package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/serial"
	"github.com/jclark/gps2phc/tai"
	"github.com/jclark/gps2phc/ubx"
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
	flag.StringVar(&serialDev, "s", "/dev/ttyUSB0", "device for serial connection to GPS")
	flag.StringVar(&ifName, "e", "eth0", "ethernet interface of PTP hardware clock")
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
	phcIndex, err := phc.IfPhcIndex(ifName)
	if err != nil {
		return
	}
	if phcIndex < 0 {
		err = fmt.Errorf("interface %s cannot be used because it does not have a PTP hardware clock", ifName)
		return
	}
	s := Syncer{}
	fmt.Printf("PHC index %d\n", phcIndex)

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
	nSkipped := 0
	for tsCh != nil || gpsCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				if e.Epoch == phc.InitialEpoch {
					nSkipped++
				} else {
					if nSkipped > 0 {
						fmt.Printf("skipped %d stale stamps\n", nSkipped)
						nSkipped = 0
					}
					corr.ppsEdge(tai.Timespec(e.T), e.TRead, e.Epoch)
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
					corr.gpsTime(tai.GPS(data.Week, data.ITOW), g.TRead)
				case ubx.TimTPId:
					data := u.Payload().(*ubx.TimTPPayload)
					corr.ppsCorr(tai.GPS(int16(data.Week), data.TowMS), Picoseconds(data.QErr))
				default:
					//fmt.Printf("ubx-%s %+v\n", u.ClsId(), u.Payload())
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
