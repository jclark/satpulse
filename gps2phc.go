package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"

	"github.com/jclark/gps2phc/logctx"
	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/tsync"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/ptime"
	"github.com/jclark/gps2phc/serio"
	"github.com/jclark/gps2phc/ubx"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

type Config struct {
	Serial SerialConfig
	Pulse  TimePulseConfig
	TCP    TCPConfig
}

type SerialConfig struct {
	Device string
	Speed  int
}

type TimePulseConfig struct {
	Interface string
	Pin       uint8
	Channel   uint8
}

type TCPConfig struct {
	Port uint16
}

const scanBufSize = 16

type Syncer struct {
	tsCh <-chan phc.TsEvent
	fCh  <-chan scan.Frame
	corr *tsync.Correlator
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
	serialDev := cfg.Serial.Device
	t, err := serio.OpenTerm(serialDev)
	if err != nil {
		return err
	}

	defer func() {
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
	s, err := newSyncer(ctx, clk, cfg, fCh)
	if err != nil {
		return err
	}
	wg.Add(1)
	go func() {
		syncWorker(ctx, s)
		wg.Done()
	}()

	return nil
}

func loadConfig(configFile string) (*Config, error) {
	var config Config
	f, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	err = toml.NewDecoder(f).DisallowUnknownFields().Decode(&config)
	if err != nil {
		return nil, err
	}
	return &config, nil
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

func openExttsClock(cfg TimePulseConfig) (*phc.Clock, error) {
	ifName := cfg.Interface
	phcIndex, err := phc.IfPhcIndex(ifName)
	if err != nil {
		return nil, err
	}
	if phcIndex < 0 {
		return nil, fmt.Errorf("interface %s cannot be used because it does not have a PTP hardware clock", ifName)
	}
	clk, err := phc.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, err
	}
	err = validateTimePulseConfig(clk, cfg)
	if err != nil {
		clk.Close()
		return nil, err
	}
	return clk, nil
}

func validateTimePulseConfig(clk *phc.Clock, cfg TimePulseConfig) error {
	var msg string
	if clk.ExttsChanCount() == 0 || clk.PinCount() == 0 {
		msg = fmt.Sprintf("PTP clock %s does not support external timestamping", clk.Path())
	} else if int(cfg.Pin) >= clk.PinCount() {
		msg = fmt.Sprintf("pin index %d is out of range for PTP clock %s: maximum index is %d", cfg.Pin, clk.Path(), clk.PinCount()-1)
	} else if int(cfg.Channel) >= clk.ExttsChanCount() {
		msg = fmt.Sprintf("channel index %d is out of range for PTP clock %s: maximum index is %d", cfg.Channel, clk.Path(), clk.ExttsChanCount()-1)
	} else {
		return nil
	}
	return errors.New(msg)
}

func nmeaLog(lg *slog.Logger, data string) {
	fields := scan.NMEASplit(data)
	if fields.SentenceFmt == "TXT" && len(fields.DataFields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Debug("nmeaTxt", "s", fields.DataFields[3])
	}
}

func newSyncer(ctx context.Context, clk *phc.Clock, cfg *Config, fCh <-chan scan.Frame) (r *Syncer, err error) {
	err = nil
	r = nil
	lg := logctx.FromContext(ctx)

	servo, err := tsync.NewServo(clk, lg)
	if err != nil {
		return nil, err
	}
	s := Syncer{corr: tsync.NewCorrelator(servo), fCh: fCh}
	lg.Info("usingPHC", "path", clk.Path())
	s.tsCh, err = StartPPS(ctx, clk, cfg.Pulse)
	if err != nil {
		return
	}
	r = &s
	return
}

func syncWorker(ctx context.Context, s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	fCh := s.fCh
	corr := s.corr
	lg := logctx.FromContext(ctx)
	lg.Debug("syncWorker", "event", "started")

	nSkipped := 0
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
				syncFrame(ctx, corr, f)
			} else {
				lg.Debug("syncWorker", "event", "fChClosed")
				fCh = nil
			}
		}
	}
}

func syncFrame(ctx context.Context, corr *tsync.Correlator, f scan.Frame) {
	lg := logctx.FromContext(ctx)
	switch f.Kind {
	case scan.NMEA:
		nmeaLog(lg, f.Data)
	case scan.UBX:
		u, err := ubx.ParseMsg(f.Data)
		if err != nil {
			lg.Error("ubxParseError", err)
			break
		}
		switch data := u.(type) {
		case *ubx.NavTimeGPS:
			corr.GPSTime(ptime.GPS(data.Week, data.ITOW), f.TRead)
		case *ubx.TimTP:
			if (data.Flags & ubx.TimTPQErrInvalid) == 0 {
				corr.PulseOffset(ptime.GPS(int16(data.Week), data.TOWMS), ptime.Picoseconds(data.QErr))
			}
		default:
			lg.Debug("ubx", "type", u.ID().String(), "payload", u)
		}
	}
}
