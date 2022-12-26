package main

import (
	"context"
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

const DEV = "/dev/ttyUSB0"

type Syncer struct {
	clk   *phc.Clock
	port  *serial.Port
	tsCh  <-chan phc.TsEvent
	ubxCh chan ubx.Msg
}

func main() {
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
	s.port, s.ubxCh, err = serStart(cx, DEV)
	if err != nil {
		return
	}
	r = &s
	return
}

func doSync(s *Syncer) {
	// loop until both channels are closed
	tsCh := s.tsCh
	ubxCh := s.ubxCh
	for tsCh != nil || ubxCh != nil {
		select {
		case e, ok := <-tsCh:
			if ok {
				fmt.Printf("extts event: %d.%09d\n", e.T.Sec, e.T.Nsec)
			} else {
				tsCh = nil
			}
		case u, ok := <-ubxCh:
			if ok {
				fmt.Printf("ubx event: %s %+v\n", u.ClsId(), u.Payload())
				if u.ClsId() == ubx.NavTimeGPSId {
					data := u.Payload().(*ubx.NavTimeGPSPayload)
					fmt.Printf("TAI time: %v\n", tai.GPS(data.Week, data.ITOW))
				}
			} else {
				ubxCh = nil
			}
		}
	}
}

func serStart(cx context.Context, path string) (*serial.Port, chan ubx.Msg, error) {
	p, err := serial.Raw(DEV)
	if err != nil {
		return nil, nil, err
	}
	err = p.Flush()
	if err != nil {
		return nil, nil, err
	}
	c := make(chan ubx.Msg, 1)
	go serReadWorker(cx, p, c)
	return p, c, nil
}

func serReadWorker(cx context.Context, p *serial.Port, c chan ubx.Msg) {
	buf := make([]byte, 255)
	msg := make([]byte, 0, 90)
	var state scanState
	var msgTime time.Time
Loop:
	for {
		select {
		case <-cx.Done():
			break Loop
		default:
		}
		n, err := p.Read(buf)
		if err != nil {
			log.Printf("Read error %+v", err)
			break
		}
		// fmt.Printf("read %d bytes\n", n)
		readTime := time.Now()
		for j := 0; j < n; j++ {
			b := buf[j]
			prevState := state
			state = nextState(state, msg, b)
			switch state {
			case msgScan:
				if prevState != msgScan {
					msg = msg[:0]
				}
			case nmeaComplete:
				finishNmeaMsg(msg, msgTime)
				state = msgScan
				msg = msg[:0]
			case nmeaHadCR:
				// do nothing
			case ubxExpectN:
				msg = append(msg, b)
				ubxMsg, err := ubx.ParseMsg(msg)
				if err != nil {
					log.Printf("UBX parse error %v", err)
				} else if ubxMsg != nil {
					c <- ubxMsg
				}
				state = msgScan
				msg = msg[:0]
			default:
				msg = append(msg, b)
				if prevState == msgScan {
					msgTime = readTime
				}
			}
		}
	}
	p.Close()
	close(c)
}

func finishNmeaMsg(msg []byte, t time.Time) {
	s := string(msg)
	if s[3:7] == "RMC," {
		fmt.Printf("NMEA %s: %s\n", t.Format(time.RFC3339Nano), string(msg))
	}
}

type scanState int

const (
	msgScan scanState = iota
	nmeaStarted
	nmeaHadComma
	nmeaHadStar
	nmeaHadChecksum1
	nmeaHadChecksum2
	nmeaHadCR
	nmeaComplete
	ubxStarted
	ubxExpectN
)

func nextState(state scanState, msg []byte, b byte) scanState {
	switch state {
	case msgScan:
		if b == '$' {
			return nmeaStarted
		}
		if b == ubx.Sync1 {
			return ubxStarted
		}
	case nmeaStarted:
		if b == ',' {
			if len(msg) >= 4 { // $XYZ
				return nmeaHadComma
			}
		} else if isAsciiAlnum(b) && len(msg) < 6 { // $GPRMC
			return nmeaStarted
		}
	case nmeaHadComma:
		if b == '*' {
			return nmeaHadStar
		}
		if isNmeaDataByte(b) && len(msg) < 82-5 { // 82 is total excluding 3-byte checksum and CRLF
			return nmeaHadComma
		}
	case nmeaHadStar, nmeaHadChecksum1:
		if isHexDigit(b) {
			return state + 1
		}
	case nmeaHadChecksum2:
		if b == '\r' {
			return nmeaHadCR
		}
		if b == '\n' {
			return nmeaComplete
		}
	case nmeaHadCR:
		if b == '\n' {
			return nmeaComplete
		}
	case ubxStarted:
		switch len(msg) {
		case 1:
			if b == ubx.Sync2 {
				return ubxStarted
			}
		case 5:
			payloadLen := int(msg[4]) + int(b)*0x100
			return scanState(int(ubxExpectN) + payloadLen + 2)
		default:
			return ubxStarted
		}
	default:
		if state > ubxExpectN {
			return state - 1
		}
	}
	return msgScan
}

func isNmeaDataByte(b byte) bool {
	if b <= ' ' || b >= 0x7f {
		return false
	}
	switch b {
	case '*':
		return false
	default:
		return true
	}
}

func isHexDigit(b byte) bool {
	if '0' <= b && b <= '9' {
		return true
	}
	if 'a' <= b && b <= 'f' {
		return true
	}
	if 'A' <= b && b <= 'F' {
		return true
	}
	return false
}

func isAsciiAlnum(b byte) bool {
	if 'a' <= b && b <= 'z' {
		return true
	}
	if 'A' <= b && b <= 'Z' {
		return true
	}
	if 'A' <= b && b <= '9' {
		return true
	}
	return false
}
