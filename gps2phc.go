package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jclark/gps2phc/phc"
	"github.com/jclark/gps2phc/serial"
	"github.com/jclark/gps2phc/tai"
	"github.com/jclark/gps2phc/ubx"
)

const DEV = "/dev/ttyUSB0"

type Sync struct {
	clk   *phc.Clock
	tsCh  <-chan phc.TsEvent
	ubxCh chan ubx.Msg
}

func main() {
	s, err := newSync()
	if err != nil {
		log.Printf("%+v\n", err)
	} else {
		doSync(s.tsCh, s.ubxCh)
	}
}

func newSync() (r *Sync, err error) {
	err = nil
	r = nil
	s := Sync{}
	s.clk, err = phc.New(phc.ClockPath(0))
	if err != nil {
		return
	}
	s.tsCh, err = StartPPS(s.clk)
	if err != nil {
		return
	}
	s.ubxCh, err = trySerial(DEV)
	if err != nil {
		return
	}
	r = &s
	return
}

func doSync(tsCh <-chan phc.TsEvent, ubxCh chan ubx.Msg) {
	for {
		select {
		case e := <-tsCh:
			fmt.Printf("extts event: %d.%09d\n", e.T.Sec, e.T.Nsec)
		case u := <-ubxCh:
			fmt.Printf("ubx event: %s %+v\n", u.ClsId(), u.Payload())
			if u.ClsId() == ubx.NavTimeGPSId {
				data := u.Payload().(*ubx.NavTimeGPSPayload)
				fmt.Printf("TAI time: %v\n", tai.GPS(data.Week, data.ITOW))
			}
		}
	}
}

func trySerial(path string) (chan ubx.Msg, error) {
	f, err := serial.Open(DEV)
	if err != nil {
		return nil, err
	}
	// defer f.Close()
	s, err := serial.GetState(f)
	if err != nil {
		return nil, err
	}
	r := s.Copy()
	r.SetRaw()
	err = serial.SetState(f, r)
	if err != nil {
		return nil, err
	}
	// defer serial.SetState(f, s)
	err = serial.Flush(f)
	if err != nil {
		return nil, err
	}
	c := make(chan ubx.Msg, 1)
	go doRead(f, c)
	return c, nil
}

func doRead(f *os.File, c chan ubx.Msg) {
	buf := make([]byte, 255)
	msg := make([]byte, 0, 90)
	var state scanState
	var msgTime time.Time
	for {
		n, err := f.Read(buf)
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
