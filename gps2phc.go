package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/tsync"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/ptime"
	"github.com/jclark/gps2phc/serio"
	"github.com/jclark/gps2phc/ubx"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

var serialDev string
var ifName string
var debugEnable bool

type Syncer struct {
	tsCh <-chan phc.TsEvent
	fCh  <-chan scan.Frame
	corr *tsync.Correlator
}

func main() {
	flag.StringVar(&serialDev, "s", "/dev/ttyUSB0", "device for serial connection to GPS")
	flag.StringVar(&ifName, "e", "eth0", "ethernet interface of PTP hardware clock")
	flag.BoolVar(&debugEnable, "d", false, "log debugging information")
	flag.Parse()
	level := slog.LevelInfo
	if debugEnable {
		level = slog.LevelDebug
	}
	lg := slog.New(slog.HandlerOptions{Level: level}.NewTextHandler(os.Stdout))
	slog.SetDefault(lg)
	ctx := context.Background()
	ctx, cancel := cancelOnSignal(ctx)
	err := run(ctx, cancel)
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cancel context.CancelFunc) error {
	clk, err := openExttsClock()
	if err != nil {
		return err
	}
	lg := slog.FromContext(ctx)

	defer func() {
		clk.Close()
		lg.Debug("closedPHC", "if", ifName)
	}()
	port, err := serio.Open(serialDev)
	if err != nil {
		return err
	}

	defer func() {
		lg.Debug("restoringSerial", "path", serialDev)
		e := port.Restore()
		if e != nil {
			lg.Error("restoredSerialErr", e, "path", serialDev)
		} else {
			lg.Debug("restoredSerial", "path", serialDev)
		}
		lg.Debug("closingSerial", "path", serialDev)
		port.Close()
		lg.Debug("closedSerial", "path", serialDev)
	}()

	lg.Debug("serial", "devKind", port.DevKind())

	fCh, err := gpsInit(ctx, port)
	defer func() {
		// gpsInit calls port.StartRead, which starts a goroutine sending to fCh.
		// We need to wait for the goroutine to close fCh, before calling port.Restore/port.Close.
		// Otherwise, there is a possibility of reading from a file descriptor that
		// is no longer valid (and so might refer to something else).
		// If fCh is nil, then the goroutine has already closed fCh.
		// If not, then we need to cancel it, to ensure the goroutine will stop reading.
		if fCh != nil {
			cancel()
			lg.Debug("waitingForFrameChannel")
			for range fCh {
			}
		}
	}()
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return nil
	}
	s, err := newSyncer(ctx, clk, fCh)
	if err != nil {
		return err
	}
	doSync(ctx, s)
	// doSync reads fCh completely, so the deferred func does not need to read it again
	fCh = nil
	lg.Debug("exiting")
	return nil
}

func cancelOnSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		slog.FromContext(ctx).Debug("cancelling")
		cancel()
	}()
	return ctx, cancel
}

func openExttsClock() (*phc.Clock, error) {
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
	if clk.ExttsChanCount() == 0 {
		clk.Close()
		return nil, fmt.Errorf("interface %s does not support external timestamping", ifName)
	}
	return clk, nil
}

func nmeaLog(lg *slog.Logger, data string) {
	fields := scan.NMEASplit(data)
	if fields.SentenceFmt == "TXT" && len(fields.DataFields) >= 4 {
		// When we open an ACM device, the GPS receiver sends TXT messages with each line of the boot screen
		lg.Debug("nmeaTxt", "s", fields.DataFields[3])
	}
}

func newSyncer(ctx context.Context, clk *phc.Clock, fCh <-chan scan.Frame) (r *Syncer, err error) {
	err = nil
	r = nil
	lg := slog.FromContext(ctx)

	servo, err := tsync.NewServo(clk, lg)
	if err != nil {
		return nil, err
	}
	s := Syncer{corr: tsync.NewCorrelator(servo), fCh: fCh}
	lg.Info("usingPHC", "path", clk.Path())
	s.tsCh, err = StartPPS(ctx, clk)
	if err != nil {
		return
	}
	r = &s
	return
}

func doSync(ctx context.Context, s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	fCh := s.fCh
	corr := s.corr
	lg := slog.FromContext(ctx)
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
				tsCh = nil
			}
		case f, ok := <-fCh:
			if ok {
				syncFrame(ctx, corr, f)
			} else {
				fCh = nil
			}
		}
	}
}

func syncFrame(ctx context.Context, corr *tsync.Correlator, f scan.Frame) {
	lg := slog.FromContext(ctx)
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
