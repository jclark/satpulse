package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/jclark/gps2phc/internal/gpsmsg"
	"github.com/jclark/gps2phc/internal/logctx"
	"github.com/jclark/gps2phc/internal/mon"
	"github.com/jclark/gps2phc/internal/nmea"
	"github.com/jclark/gps2phc/internal/phc"
	"github.com/jclark/gps2phc/internal/pmc"
	"github.com/jclark/gps2phc/internal/ptime"
	"github.com/jclark/gps2phc/internal/scan"
	"github.com/jclark/gps2phc/internal/serio"
	"github.com/jclark/gps2phc/internal/tsync"
	"github.com/jclark/gps2phc/internal/ubx"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

type Config struct {
	Serial     SerialConfig
	Pulse      TimePulseConfig
	TCP        TCPConfig
	LeapSecond LeapSecondConfig
}

type SerialConfig struct {
	Device string
	Speed  *int
}

type TCPConfig struct {
	Port uint16
}

type LeapSecondConfig struct {
	Date          toml.LocalDate
	Before, After uint8
}

var leapSecondDefault = LeapSecondConfig{
	Date:   toml.LocalDate{Year: 2016, Month: int(time.December), Day: 31},
	Before: 36,
	After:  37,
}

const scanBufSize = 16

type Syncer struct {
	tsCh <-chan phc.TsEvent
	fCh  <-chan scan.Frame
	corr *tsync.Correlator
	gm   *mon.Grandmaster
	ls   ptime.LeapSecond
}

func main() {
	var configFile string
	var debugEnable bool

	flag.StringVar(&configFile, "f", "gps2phc.toml", "configuration file")
	flag.BoolVar(&debugEnable, "d", false, "log debugging information")
	flag.Parse()
	level := slog.LevelInfo
	if debugEnable {
		level = slog.LevelDebug
	}

	lg := slog.New(slog.HandlerOptions{Level: level}.NewTextHandler(os.Stdout))
	slog.SetDefault(lg)
	ctx := logctx.NewContext(context.Background(), lg)
	ctx, cancel := cancelOnSignal(ctx)
	err := run(ctx, cancel, configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		var derr *toml.DecodeError
		if errors.As(err, &derr) {
			fmt.Fprintln(os.Stderr, derr.String())
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
		lg.Debug("closedPHC", "if", cfg.Pulse.Interface)
	}()
	t, err := serio.OpenTerm(cfg.Serial.Device, cfg.Serial.Speed)
	if err != nil {
		return err
	}

	defer func() {
		serialDev := cfg.Serial.Device
		lg.Debug("restoringSerial", "path", serialDev)
		e := t.Restore()
		if e != nil {
			lg.Error("restoredSerialErr", e, "path", serialDev)
		} else {
			lg.Debug("restoredSerial", "path", serialDev)
		}
		lg.Debug("closingSerial", "path", serialDev)
		t.Close()
		lg.Debug("closedSerial", "path", serialDev)
	}()

	lg.Debug("serial", "devKind", t.DevKind())

	scanner := scan.New(t, scanBufSize)
	var wg sync.WaitGroup

	fCh := startScan(ctx, &wg, scanner)
	b := startBcast(ctx, &wg, fCh)
	// Shut down the broadcast goroutine when the context is cancelled.
	wg.Add(1)
	go func() {
		<-ctx.Done()
		b.Close()
		wg.Done()
	}()
	fCh = b.Subscribe()
	defer func() {
		// startScan starts a goroutine sending to fCh.
		// We need to wait for the goroutine to close fCh, before calling port.Restore/port.Close.
		// Otherwise, there is a possibility of reading from a file descriptor that
		// is no longer valid (and so might refer to something else).
		if err != nil {
			cancel()
		}
		wg.Wait()
		lg.Debug("waitDone")
	}()

	err = gpsInit(ctx, fCh, t)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}
	tcpPort := cfg.TCP.Port
	if tcpPort != 0 {
		err = startTCP(ctx, &wg, fmt.Sprintf(":%d", tcpPort), b, t)
		if err != nil {
			return err
		}
	}
	pmcClient, err := pmc.NewClient(nil)
	if err != nil {
		return err
	}
	gmUpdateCh := make(chan mon.GrandmasterUpdateRequest)

	s, err := newSyncer(ctx, clk, cfg, fCh, gmUpdateCh)
	if err != nil {
		return err
	}
	wg.Add(1)
	go func() {
		mon.PTP4LWorker(ctx, pmcClient, gmUpdateCh, lg)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		syncWorker(ctx, s)
		close(gmUpdateCh)
		wg.Done()
	}()

	return nil
}

func loadConfig(configFile string) (*Config, error) {
	f, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	cfg := new(Config)
	cfg.LeapSecond = leapSecondDefault
	err = toml.NewDecoder(f).DisallowUnknownFields().Decode(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func cancelOnSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		logctx.FromContext(ctx).Debug("cancelling")
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

func startBcast(ctx context.Context, wg *sync.WaitGroup, msg <-chan scan.Frame) *serio.Bcast {
	b := serio.NewBcast(msg)
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
		lg.Debug("nmeaTxt", "s", fields.DataFields[3])
	}
}

func newSyncer(ctx context.Context, clk *phc.Clock, cfg *Config, fCh <-chan scan.Frame,
	guCh chan<- mon.GrandmasterUpdateRequest) (r *Syncer, err error) {
	err = nil
	r = nil
	lg := logctx.FromContext(ctx)

	sa := mon.NewSyncAnalyzer()
	servo, err := tsync.NewServo(clk, lg)
	if err != nil {
		return nil, err
	}
	ls := leapSecondFromConfig(cfg.LeapSecond)
	s := Syncer{
		corr: tsync.NewCorrelator(tsync.MultiSampler(servo, sa), lg),
		fCh:  fCh,
		gm:   mon.NewGrandmaster(sa, ls, guCh, lg),
		ls:   ls,
	}
	lg.Info("usingPHC", "path", clk.Path())
	s.tsCh, err = StartPPS(ctx, clk, cfg.Pulse)
	if err != nil {
		return
	}
	r = &s
	return
}

func leapSecondFromConfig(cfg LeapSecondConfig) ptime.LeapSecond {
	return ptime.LeapSecondOnDate(cfg.Date.AsTime((time.UTC)), int16(cfg.Before), int16(cfg.After))
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
	corr := s.corr
	lg := logctx.FromContext(ctx)
	lg.Debug("syncWorker", "event", "started")

	nSkipped := 0
	var state SyncState
	state.leapSecond = s.ls
	for tsCh != nil || fCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				if e.Epoch == ptime.InitialEpoch {
					if nSkipped == 0 {
						lg.Info("stalePHCTimestamps", "t", e.T)
					}
					nSkipped++
				} else {
					if nSkipped > 0 {
						lg.Info("skippedStalePHCTimestamps", "n", nSkipped)
						nSkipped = 0
					}
					corr.PulseEdge(e.ClockTime, e.TRead)
				}
			} else {
				lg.Debug("syncWorker", "event", "tsChClosed")
				tsCh = nil
			}
		case f, ok := <-fCh:
			if ok {
				syncFrame(ctx, &state, corr, s.gm, f)
			} else {
				lg.Debug("syncWorker", "event", "fChClosed")
				fCh = nil
			}
		}
	}
}

func syncFrame(ctx context.Context, state *SyncState, corr *tsync.Correlator, gm *mon.Grandmaster, f scan.Frame) {
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
		lg.Debug("timeFromUTC", "t", sec)
	}
	secRnd := sec.Round(time.Second)
	if mt.PrecedesPulse {
		corr.PulseOffset(secRnd, f.TRead, mt.PulseOffset)
	} else if secRnd > state.lastTime {
		corr.GPSTime(secRnd, f.TRead)
		// do corr first so that samples are updated in the SyncAnalyzer
		gm.GPSTime(sec)
		state.lastTime = secRnd
	}
}
