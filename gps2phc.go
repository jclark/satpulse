package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/ptime"
	"github.com/jclark/gps2phc/serial"
	"github.com/jclark/gps2phc/ubx"
	"golang.org/x/exp/slog"
)

var serialDev string
var ifName string

type Syncer struct {
	clk   *phc.Clock
	port  *serial.Port
	tsCh  <-chan phc.TsEvent
	gpsCh chan GpsMsg
	corr  *Correlator
}

func main() {
	lg := slog.New(slog.HandlerOptions{Level: slog.LevelDebug}.NewTextHandler(os.Stdout))
	slog.SetDefault(lg)

	flag.StringVar(&serialDev, "s", "/dev/ttyUSB0", "device for serial connection to GPS")
	flag.StringVar(&ifName, "e", "eth0", "ethernet interface of PTP hardware clock")
	flag.Parse()
	cx := context.Background()
	cx = cancelOnInterrupt(cx)
	s, err := newSyncer(cx)
	if err != nil {
		slog.Error("exiting", err)
	} else {
		doSync(cx, s)
		slog.Debug("exiting")
		s.port.Close()
		slog.Debug("closed", "serial", serialDev)
		s.clk.Close()
		slog.Debug("closed", "if", ifName)
	}
}

func cancelOnInterrupt(cx context.Context) context.Context {
	cx, cancel := context.WithCancel(cx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		slog.FromContext(cx).Debug("cancelling")
		cancel()
	}()
	return cx
}

func newSyncer(cx context.Context) (r *Syncer, err error) {
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
	lg := slog.FromContext(cx)
	lg.Info("usingPHC", "index", phcIndex)

	s.clk, err = phc.New(phc.ClockPath(phcIndex))
	if err != nil {
		return
	}
	s.tsCh, err = StartPPS(cx, s.clk)
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
	s.port, s.gpsCh, err = serStart(cx, serialDev)
	if err != nil {
		return
	}
	servo, err := NewServo(s.clk, lg)
	if err != nil {
		return nil, err
	}
	corr := new(Correlator)
	corr.servo = servo

	s.corr = corr
	r = &s
	return
}

func doSync(cx context.Context, s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	gpsCh := s.gpsCh
	corr := s.corr
	lg := slog.FromContext(cx)
	nSkipped := 0
	for tsCh != nil || gpsCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				t := ptime.TimespecToTime(e.T)
				if e.Epoch == phc.InitialEpoch {
					if nSkipped == 0 {
						lg.Info("stalePHCTimestamps", "t", t)
					}
					nSkipped++
				} else {
					if nSkipped > 0 {
						lg.Info("skippedStalePHCTimestamps", "n", nSkipped)
						nSkipped = 0
					}
					corr.ppsEdge(t, e.TRead, e.Epoch)
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
					corr.gpsTime(ptime.GPS(data.Week, data.ITOW), g.TRead)
				case ubx.TimTPId:
					data := u.Payload().(*ubx.TimTPPayload)
					corr.ppsCorr(ptime.GPS(int16(data.Week), data.TowMS), Picoseconds(data.QErr))
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

func serStart(cx context.Context, path string) (*serial.Port, chan GpsMsg, error) {
	p, err := serial.Raw(path)
	if err != nil {
		return nil, nil, err
	}
	err = p.Flush()
	if err != nil {
		return nil, nil, err
	}
	c := make(chan GpsMsg, 1)
	go serReadWorker(cx, p, c)
	return p, c, nil
}
