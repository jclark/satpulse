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

type Syncer struct {
	tsCh  <-chan phc.TsEvent
	fCh   <-chan scan.Frame
	sseCh chan<- sse.Event
	corr  *tsync.Correlator
	gm    *mon.Grandmaster
	ls    ptime.LeapSecond
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

	clk, err := openExttsClock(cfg.Pulse)
	if err != nil {
		return err
	}
	lg := logctx.FromContext(ctx)

	defer func() {
		clk.Close()
		lg.Debug("closed the PHC", "interface", cfg.Pulse.Interface)
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
	wg.Add(1)
	go func() {
		<-ctx.Done()
		fb.Close()
		if eb != nil {
			eb.Close()
		}
		wg.Done()
	}()
	fCh = fb.Subscribe()
	defer func() {
		// startScan starts a goroutine sending to fCh.
		// We need to wait for the goroutine to close fCh, before calling port.Restore/port.Close.
		// Otherwise, there is a possibility of reading from a file descriptor that
		// is no longer valid (and so might refer to something else).
		if err != nil {
			cancel()
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
	s, err := newSyncer(ctx, clk, cfg, fCh, gmUpdateCh, sseCh)
	if err != nil {
		return err
	}
	if pmcClient != nil {
		wg.Add(1)
		go func() {
			mon.PTP4LWorker(ctx, pmcClient, gmUpdateCh, lg)
			wg.Done()
		}()
	}
	wg.Add(1)
	go func() {
		syncWorker(ctx, s)
		close(gmUpdateCh)
		wg.Done()
	}()

	return nil
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
	wg.Add(1)
	go func() {
		serio.ScanWorker(ctx, scanner, msg)
		wg.Done()
	}()
	return msg
}

func startBcast[T any](ctx context.Context, wg *sync.WaitGroup, msg <-chan T) *bcast.Bcast[T] {
	b := bcast.New(msg)
	wg.Add(1)
	go func() {
		b.Run(ctx)
		wg.Done()
	}()
	return b
}

func nmeaLog(lg *slog.Logger, msg *nmea.Message) {
	fields := msg.Fields()
	if fields.SentenceFmt == "TXT" && len(fields.DataFields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Debug("received NMEA TXT message", "s", fields.DataFields[3])
	}
}

func newSyncer(ctx context.Context, clk *phc.Clock, cfg *Config, fCh <-chan scan.Frame,
	guCh chan<- mon.GrandmasterUpdateRequest, sseCh chan<- sse.Event) (r *Syncer, err error) {
	err = nil
	r = nil
	lg := logctx.FromContext(ctx)

	sa := mon.NewSyncAnalyzer()
	servo, err := tsync.NewServo(clk, lg, sseCh)
	if err != nil {
		return nil, err
	}
	ls := cfg.LeapSecond.leapSecond()
	s := Syncer{
		corr:  tsync.NewCorrelator(tsync.MultiSampler(servo, sa), lg),
		fCh:   fCh,
		gm:    mon.NewGrandmaster(sa, ls, guCh, lg),
		ls:    ls,
		sseCh: sseCh,
	}
	lg.Info("selected PTP hardware clock", "path", clk.Path())
	s.tsCh, err = StartPPS(ctx, clk, cfg.Pulse)
	if err != nil {
		return
	}
	r = &s
	return
}

type SyncState struct {
	lastTime   ptime.Time
	leapSecond ptime.LeapSecond
}

// XXX Need a better name. This handles input from the GPS over the serial and timestammp channel.
func syncWorker(ctx context.Context, s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	fCh := s.fCh
	sseCh := s.sseCh
	if sseCh != nil {
		defer close(sseCh)
	}
	corr := s.corr
	lg := logctx.FromContext(ctx)
	lg.Debug("sync worker goroutine started")

	nSkipped := 0
	var state SyncState
	state.leapSecond = s.ls
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
					corr.PulseEdge(e.ClockTime, e.TRead)
				}
			} else {
				lg.Debug("timestamp channel of sync worker goroutine was closed")
				tsCh = nil
			}
		case f, ok := <-fCh:
			if ok {
				syncFrame(ctx, &state, corr, s.gm, s.sseCh, f)
			} else {
				lg.Debug("frame channel of sync worker goroutine was closed")
				fCh = nil
			}
		}
	}
}

type TimeEvent struct {
	UTC string `json:"utc"`
	TAI int64  `json:"tai"`
}

func syncFrame(ctx context.Context, state *SyncState, corr *tsync.Correlator, gm *mon.Grandmaster, sseCh chan<- sse.Event, f scan.Frame) {
	lg := logctx.FromContext(ctx)
	// TODO: handle leapsecond messages
	var mt *gpsmsg.Time
	switch f.Kind {
	case scan.NMEA:
		m, err := nmea.Parse(f.Data)
		if err != nil {
			lg.Error("nmeaParseError", err)
			break
		}
		nmeaLog(lg, m)
		mt = m.Time()
	case scan.UBX:
		m, err := ubx.Parse(f.Data)
		if err != nil {
			lg.Error("ubxParseError", err)
			break
		}
		mt = m.Time()
	}
	if mt == nil {
		return
	}
	if false {
		bytes, err := json.Marshal(mt)
		if err == nil {
			fmt.Println(string(bytes))
		}
	}
	// TODO: make calls on s.gm when no valid time is received from GPS
	// Can do this by having allowing Time to represent an invalid Time
	var sec ptime.Time
	if !mt.TAITime.IsZero() {
		sec = mt.TAITime
	} else {
		u := mt.UTCTime
		if u == nil {
			return
		}
		sec = state.leapSecond.UTCtoTime(*u)
		lg.Debug("computed TAI time from UTC time", "tai", sec)
	}
	secRnd := sec.Round(time.Second)
	if mt.PrecedesPulse {
		corr.PulseOffset(secRnd, f.TRead, mt.PulseOffset)
	} else if secRnd > state.lastTime {
		corr.GPSTime(secRnd, f.TRead)
		// do corr first so that samples are updated in the SyncAnalyzer
		gm.GPSTime(sec)
		if sseCh != nil {
			te := TimeEvent{
				UTC: state.leapSecond.FormatTime(secRnd),
				TAI: int64(secRnd) / 1e9,
			}
			event, err := sse.Make("time", te)
			if err != nil {
				lg.Error("failed to create SSE event", err)
			} else {
				sseCh <- event
			}
		}
		state.lastTime = secRnd
	}
}
