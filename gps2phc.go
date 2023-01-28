package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

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
	fCh  chan scan.Frame
	corr *tsync.Correlator
}

func main() {
	flag.StringVar(&serialDev, "s", "/dev/ttyUSB0", "device for serial connection to GPS")
	flag.StringVar(&ifName, "e", "eth0", "ethernet interface of PTP hardware clock")
	flag.BoolVar(&debugEnable, "d", false, "log debuggging information")
	flag.Parse()
	level := slog.LevelInfo
	if debugEnable {
		level = slog.LevelDebug
	}
	lg := slog.New(slog.HandlerOptions{Level: level}.NewTextHandler(os.Stdout))
	slog.SetDefault(lg)
	ctx := context.Background()
	ctx = cancelOnSignal(ctx)
	clk, err := openExttsClock()
	var port *serio.Port
	if err == nil {
		port, err = serio.Open(serialDev)
	}
	var fCh chan scan.Frame
	if err == nil {
		lg.Debug("serial", "devKind", port.DevKind())
		fCh, err = gpsInit(ctx, port)
	}
	var s *Syncer
	if err == nil {
		s, err = newSyncer(ctx, clk, fCh)
	}
	// XXX if fCh is nil and err is non-nil,
	// we should receive from fCh until it is closed
	// At that point, we can cleanup and exit
	if err != nil {
		slog.Error("exiting", err)
	} else {
		doSync(ctx, s)
		slog.Debug("exiting")
		err = port.Restore()
		if err != nil {
			slog.Error("could not restore terminal settings", err)
		}
		port.Close()
		slog.Debug("closed", "serial", serialDev)
		clk.Close()
		slog.Debug("closed", "if", ifName)
	}
}

func cancelOnSignal(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		slog.FromContext(ctx).Debug("cancelling")
		cancel()
	}()
	return ctx
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

func newSyncer(ctx context.Context, clk *phc.Clock, fCh chan scan.Frame) (r *Syncer, err error) {
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
		} else if u != nil {
			switch data := u.(type) {
			case *ubx.NavTimeGPS:
				corr.GPSTime(ptime.GPS(data.Week, data.ITOW), f.TRead)
			case *ubx.TimTP:
				corr.PulseCorrection(ptime.GPS(int16(data.Week), data.TowMS), Picoseconds(data.QErr))
			default:
				lg.Debug("ubx", "type", u.ID().String(), "payload", u)
			}
		}
	}
}

func Picoseconds(ps int32) time.Duration {
	if ps < 0 {
		return -Picoseconds(-ps)
	}
	return time.Duration(((ps + 500) / 1000))
}
