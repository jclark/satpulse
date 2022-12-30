package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/serial"
	"github.com/jclark/gps2phc/tai"
	"github.com/jclark/gps2phc/ubx"
)

var serialDev string

type Syncer struct {
	clk   *phc.Clock
	port  *serial.Port
	tsCh  <-chan phc.TsEvent
	gpsCh chan GpsMsg
	corr  *Correlator
}

func main() {
	flag.StringVar(&serialDev, "s", "/dev/ttyUSB0", "device for serial connection to GPS")
	flag.Parse()
	cx := cancelOnInterrupt()
	s, err := newSyncer(cx)
	if err != nil {
		log.Printf("%+v\n", err)
	} else {
		doSync(s)
		fmt.Println("sync ended")
		s.port.Close()
		fmt.Println("serial port closed")
		s.clk.Close()
		fmt.Println("clock closed")

	}
}

func cancelOnInterrupt() context.Context {
	cx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		fmt.Println("cancelling")
		cancel()
	}()
	return cx
}

func newSyncer(cx context.Context) (r *Syncer, err error) {
	err = nil
	r = nil
	s := Syncer{}
	s.clk, err = phc.New(phc.ClockPath(0))
	if err != nil {
		return
	}
	s.tsCh, err = StartPPS(cx, s.clk)
	if err != nil {
		return
	}
	// XXX errors after this need to deal with running PPS goroutine
	err = SkipStale(s.clk, s.tsCh)
	if err != nil {
		return nil, err
	}
	s.port, s.gpsCh, err = serStart(cx, serialDev)
	if err != nil {
		return
	}
	servo, err := NewServo(s.clk)
	if err != nil {
		return nil, err
	}
	corr := new(Correlator)
	corr.servo = servo

	s.corr = corr
	r = &s
	return
}

func doSync(s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	gpsCh := s.gpsCh
	corr := s.corr
	for tsCh != nil || gpsCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				corr.ppsEdge(TimeReading{T: tai.Timespec(e.T), TRead: e.TRead})
			} else {
				tsCh = nil
			}
		case g, ok := <-gpsCh:
			if ok {
				u := g.U
				switch u.ClsId() {
				case ubx.NavTimeGPSId:
					data := u.Payload().(*ubx.NavTimeGPSPayload)
					corr.gpsTime(TimeReading{T: tai.GPS(data.Week, data.ITOW), TRead: g.TRead})
				default:
					//fmt.Printf("ubx-%s %+v\n", u.ClsId(), u.Payload())
				}
			} else {
				gpsCh = nil
			}
		}
	}
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
