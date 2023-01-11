package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/gps2phc/tsync"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/ptime"
	"github.com/jclark/gps2phc/serial"
	"github.com/jclark/gps2phc/ubx"
	"golang.org/x/exp/slog"
)

var serialDev string
var ifName string
var debugEnable bool

type Syncer struct {
	clk   *phc.Clock
	port  *serial.Port
	tsCh  <-chan phc.TsEvent
	gpsCh chan GpsMsg
	corr  *tsync.Correlator
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
	ctx = cancelOnInterrupt(ctx)
	s, err := newSyncer(ctx)
	if err != nil {
		slog.Error("exiting", err)
	} else {
		doSync(ctx, s)
		slog.Debug("exiting")
		s.port.Close()
		slog.Debug("closed", "serial", serialDev)
		s.clk.Close()
		slog.Debug("closed", "if", ifName)
	}
}

func cancelOnInterrupt(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		slog.FromContext(ctx).Debug("cancelling")
		cancel()
	}()
	return ctx
}

func newSyncer(ctx context.Context) (r *Syncer, err error) {
	err = nil
	r = nil
	phcIndex, err := phc.IfPhcIndex(ifName)
	if err != nil {
		return
	}
	if phcIndex < 0 {
		err = fmt.Errorf("interface %s cannot be used because it does not have a PTP hardware clock", ifName)
		return
	}
	s := Syncer{}
	lg := slog.FromContext(ctx)
	lg.Info("usingPHC", "index", phcIndex)

	s.clk, err = phc.New(phc.ClockPath(phcIndex))
	if err != nil {
		return
	}
	s.tsCh, err = StartPPS(ctx, s.clk)
	if err != nil {
		return
	}
	/*
		// XXX errors after this need to deal with running PPS goroutine
		err = SkipStale(s.clk, s.tsCh)
		if err != nil {
			return nil, err
		}
	*/
	s.port, s.gpsCh, err = serStart(ctx, serialDev)
	if err != nil {
		return
	}
	servo, err := tsync.NewServo(s.clk, lg)
	if err != nil {
		return nil, err
	}
	s.corr = tsync.NewCorrelator(servo)
	r = &s
	return
}

func doSync(ctx context.Context, s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	gpsCh := s.gpsCh
	corr := s.corr
	lg := slog.FromContext(ctx)
	nSkipped := 0
	for tsCh != nil || gpsCh != nil {
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
		case g, ok := <-gpsCh:
			if ok {
				u := g.U
				switch u.ClsId() {
				case ubx.NavTimeGPSId:
					data := u.Payload().(*ubx.NavTimeGPSPayload)
					corr.GPSTime(ptime.GPS(data.Week, data.ITOW), g.TRead)
				case ubx.TimTPId:
					data := u.Payload().(*ubx.TimTPPayload)
					corr.PulseCorrection(ptime.GPS(int16(data.Week), data.TowMS), Picoseconds(data.QErr))
				default:
					lg.Debug("ubx", "type", u.ClsId().String(), "payload", u.Payload())
				}
			} else {
				gpsCh = nil
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

func serStart(ctx context.Context, path string) (*serial.Port, chan GpsMsg, error) {
	p, err := serial.Raw(path)
	if err != nil {
		return nil, nil, err
	}
	err = p.Flush()
	if err != nil {
		return nil, nil, err
	}
	c := make(chan GpsMsg, 1)
	go serReadWorker(ctx, p, c)
	return p, c, nil
}
