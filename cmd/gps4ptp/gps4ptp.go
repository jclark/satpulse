package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/jclark/gps4ptp/internal/bcast"
	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/logctx"
	"github.com/jclark/gps4ptp/internal/mon"
	"github.com/jclark/gps4ptp/internal/nmea"
	"github.com/jclark/gps4ptp/internal/phc"
	"github.com/jclark/gps4ptp/internal/pmc"
	"github.com/jclark/gps4ptp/internal/ptime"
	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/internal/serio"
	"github.com/jclark/gps4ptp/internal/sse"
	"github.com/jclark/gps4ptp/internal/tsync"
	"github.com/jclark/gps4ptp/internal/ubx"

	"golang.org/x/sys/unix"
)

const scanBufSize = 16

type SyncRunner struct {
	gpsmsg.DefaultHandler

	sseCh    chan<- sse.Event
	corr     *tsync.Correlator
	m        *mon.Monitor
	ls       ptime.LeapSecond
	lg       *slog.Logger
	lastTime ptime.Time
}

func main() {
	var configFile string
	var debugEnable bool
	var sdLog bool
	var showVersion bool

	flag.StringVar(&configFile, "f", defaultConfigFile, "configuration file")
	flag.BoolVar(&showVersion, "version", false, "show version information")
	flag.BoolVar(&debugEnable, "debug", false, "log debugging information")
	flag.BoolVar(&sdLog, "sdlog", false, "log to stdout with priorities in systemd-compatible format")
	flag.Parse()
	if showVersion {
		fmt.Println(versionInfo())
		os.Exit(0)
	}
	level := slog.LevelInfo
	if debugEnable {
		level = slog.LevelDebug
	}
	var handler slog.Handler
	if sdLog {
		handler = NewSdHandler(level, os.Stdout)
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	lg := slog.New(handler)
	slog.SetDefault(lg)
	ctx := logctx.NewContext(context.Background(), lg)
	ctx, cancel := cancelOnSignal(ctx)
	err := run(ctx, cancel, configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		s := configErrorDetail(err)
		if s != "" {
			fmt.Fprintln(os.Stderr, s)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, cancel context.CancelFunc, cfgFile string) error {
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		return err
	}

	clk, err := openExttsClock(cfg.PHC)
	if err != nil {
		return err
	}
	lg := logctx.FromContext(ctx)
	lg.Info("selected PTP hardware clock", "path", clk.Path())

	defer func() {
		clk.Close()
		lg.Debug("closed the PHC", "interface", cfg.PHC.Interface)
	}()
	t, err := serio.OpenTerm(cfg.Serial.Device, cfg.Serial.Speed)
	if err != nil {
		return err
	}

	defer func() {
		serialDev := cfg.Serial.Device
		lg.Debug("restoring the serial port settings", "path", serialDev)
		e := t.Restore()
		if e != nil {
			lg.Error("error while restoring the serial port settings", e, "path", serialDev)
		} else {
			lg.Debug("successfully restored the serial port settings", "path", serialDev)
		}
		lg.Debug("closing the serial port", "path", serialDev)
		t.Close()
		lg.Debug("successfully closed the serial port", "path", serialDev)
	}()

	lg.Debug("detecting kind of serial port", "devKind", t.DevKind())

	scanner := scan.New(t, scanBufSize)
	var wg sync.WaitGroup

	fCh := startScan(ctx, &wg, scanner)

	fb := startBcast(ctx, &wg, fCh)

	var sseCh chan sse.Event
	var eb *bcast.Bcast[sse.Event]
	if len(cfg.HTTP) > 0 {
		sseCh = make(chan sse.Event, 1)
		eb = startBcast(ctx, &wg, sseCh)
	}
	// Shut down the broadcast goroutines when the context is cancelled.
	waitGroupGo(&wg, func() {
		<-ctx.Done()
		fb.Close()
		if eb != nil {
			eb.Close()
		}
	})
	fCh = fb.Subscribe()
	defer func() {
		// Avoid calling wg.Wait() if we panic
		if r := recover(); r != nil {
			panic(r)
		}
		// startScan starts a goroutine sending to fCh.
		// We need to wait for the goroutine to close fCh, before calling port.Restore/port.Close.
		// Otherwise, there is a possibility of reading from a file descriptor that
		// is no longer valid (and so might refer to something else).
		if err != nil {
			cancel()
		}
		// ensure that the sseCh gets closed
		// even if we don't reach the point where the syncWorker does this
		if sseCh != nil {
			close(sseCh)
		}
		wg.Wait()
		lg.Debug("wait group counter dropped to zero")
	}()

	initData, err := gpsInit(ctx, fCh, t)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}

	err = startTCP(ctx, lg, &wg, cfg.TCP, fb, t)
	if err != nil {
		return err
	}

	if eb != nil {
		initEvent, err := sse.Make("init", initData)
		if err != nil {
			return err
		}
		err = startHTTP(ctx, lg, &wg, cfg.HTTP, eb, initEvent)
		if err != nil {
			return err
		}
	}

	gmUpdateCh := make(chan mon.GrandmasterUpdateRequest)
	var pmcClient *pmc.Client
	switch cfg.PTP.Impl {
	case PTPImplNone:
		gmUpdateCh = nil
	case PTPImplPTP4L:
		pmcClient, err = cfg.PTP.NewClient()
		if err != nil {
			return err
		}
	}
	s, err := NewSyncRunner(lg, clk, cfg, gmUpdateCh, sseCh)
	if err != nil {
		return err
	}

	tsCh, err := StartPPS(ctx, clk, cfg.PHC)
	if err != nil {
		return err
	}
	if pmcClient != nil {
		waitGroupGo(&wg, func() { mon.PTP4LWorker(ctx, pmcClient, gmUpdateCh, lg) })
	}
	// the SyncRunner assumes responsibility for closing the sseCh
	sseCh = nil
	waitGroupGo(&wg, func() {
		s.run(tsCh, fCh)
		close(gmUpdateCh)
	})

	return nil
}

func waitGroupGo(wg *sync.WaitGroup, f func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		f()
	}()
}

func cancelOnSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		logctx.FromContext(ctx).Debug("received signal, initiating cancellation")
		cancel()
	}()
	return ctx, cancel
}

func startScan(ctx context.Context, wg *sync.WaitGroup, scanner *scan.Scanner) <-chan scan.Frame {
	msg := make(chan scan.Frame, 1)
	waitGroupGo(wg, func() { serio.ScanWorker(ctx, scanner, msg) })
	return msg
}

func startBcast[T any](ctx context.Context, wg *sync.WaitGroup, msg <-chan T) *bcast.Bcast[T] {
	b := bcast.New(msg)
	waitGroupGo(wg, func() { b.Run(ctx) })
	return b
}

func nmeaLog(lg *slog.Logger, msg *nmea.Message) {
	if msg.SentenceFmt == "TXT" && len(msg.Fields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Debug("received NMEA TXT message", "s", msg.Fields[3])
	}
}

func NewSyncRunner(lg *slog.Logger, clk *phc.Clock, cfg *Config, guCh chan<- mon.GrandmasterUpdateRequest, sseCh chan<- sse.Event) (*SyncRunner, error) {
	servo, err := tsync.NewServo(clk, lg, sseCh)
	if err != nil {
		return nil, err
	}
	ls := cfg.LeapSecond.leapSecond()
	m := mon.NewMonitor(ls, guCh, lg)
	s := SyncRunner{
		corr:  tsync.NewCorrelator(tsync.MultiSampler(servo, m), lg),
		m:     m,
		ls:    ls,
		lg:    lg,
		sseCh: sseCh,
	}
	return &s, nil
}

const tickPeriod = time.Second / 4

func (s *SyncRunner) run(tsCh <-chan phc.TsEvent, fCh <-chan scan.Frame) {
	// loop until both channels are closed
	sseCh := s.sseCh
	if sseCh != nil {
		defer close(sseCh)
	}
	ticker := time.NewTicker(tickPeriod)
	defer ticker.Stop()
	lg := s.lg
	lg.Debug("sync worker goroutine started")

	nSkipped := 0

	for tsCh != nil || fCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				if e.Era == phc.StaleEra {
					if nSkipped == 0 {
						lg.Debug("detected a stale PTP hardware clock timestamp", "t", e.T)
					}
					nSkipped++
				} else {
					if nSkipped > 0 {
						lg.Info("skipped stale PTP hardware clock timestamps", "n", nSkipped)
						nSkipped = 0
					}
					s.corr.PulseEdge(e.ClockTime, e.TRead)
				}
			} else {
				lg.Debug("timestamp channel of sync worker goroutine was closed")
				tsCh = nil
			}
		case f, ok := <-fCh:
			if ok {
				s.handleFrame(f)
			} else {
				lg.Debug("frame channel of sync worker goroutine was closed")
				fCh = nil
			}
		case t := <-ticker.C:
			s.m.Tick(t)
		}
	}
}

type TimeEvent struct {
	UTC string `json:"utc"`
	TAI int64  `json:"tai"`
}

func (s *SyncRunner) handleFrame(f scan.Frame) {
	// TODO: handle leapsecond messages
	lg := s.lg
	switch f.Kind {
	case scan.NMEA:
		err := nmea.ProcessFrameData(f.Data, f.TRead, s, s)
		if err != nil {
			lg.Error("failed to parse NMEA message", "err", err)
		}
	case scan.UBX:
		err := ubx.ProcessFrameData(f.Data, f.TRead, s, nil)
		if err != nil {
			lg.Error("failed to parse UBX message", "err", err)
		}
	case scan.Invalid:
		lg.Info("received data from GPS in unknown protocol (serial communication problem?)", "len", len(f.Data), "data", f.Data)
	}
}

func (s *SyncRunner) Time(mt *gpsmsg.Time, tRead time.Time) {
	if false {
		bytes, err := json.Marshal(mt)
		if err == nil {
			fmt.Println(string(bytes))
		}
	}
	var sec ptime.Time
	if !mt.TAITime.IsZero() {
		sec = mt.TAITime
	} else {
		u := mt.UTCTime
		if u == nil {
			return
		}
		sec = s.ls.UTCtoTime(*u)
		s.lg.Debug("computed TAI time from UTC time", "tai", sec)
	}
	secRnd := sec.Round(time.Second)
	if mt.PrecedesPulse {
		s.corr.PulseOffset(secRnd, tRead, mt.PulseOffset)
	} else if secRnd > s.lastTime {
		s.corr.GPSTime(secRnd, tRead)
		if s.sseCh != nil {
			te := TimeEvent{
				UTC: s.ls.FormatTime(secRnd),
				TAI: int64(secRnd) / 1e9,
			}
			event, err := sse.Make("time", te)
			if err != nil {
				s.lg.Error("failed to create SSE event", "err", err)
			} else {
				s.sseCh <- event
			}
		}
		s.lastTime = secRnd
	}
}

func (s *SyncRunner) NMEA(msg *nmea.Message, tRead time.Time) {
	nmeaLog(s.lg, msg)
}
