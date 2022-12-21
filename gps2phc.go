package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"example/gps2phc/serial"
	"example/gps2phc/ubx"
)

const DEV = "/dev/ttyUSB0"

func main() {
	err := StartExtts(0, 0, 0)
	//if err != nil {
	//	err = trySerial(DEV)
	//}
	if err != nil {
		log.Printf("%+v\n", err)
	}
}

func trySerial(path string) error {
	f, err := serial.Open(DEV)
	if err != nil {
		return err
	}
	defer f.Close()
	s, err := serial.GetState(f)
	if err != nil {
		return err
	}
	r := s.Copy()
	r.SetRaw()
	err = serial.SetState(f, r)
	if err != nil {
		return err
	}
	defer serial.SetState(f, s)
	err = serial.Flush(f)
	if err != nil {
		return err
	}
	return tryRead(f)
}

const NREADS = 500

func tryRead(f *os.File) error {
	buf := make([]byte, 255)
	msg := make([]byte, 0, 90)
	var state scanState
	var msgTime time.Time
	for i := 0; i < NREADS; i++ {
		n, err := f.Read(buf)
		if err != nil {
			return err
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
				finishUbxMsg(msg, msgTime)
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
	fmt.Println("Finished")
	return nil
}

func finishNmeaMsg(msg []byte, t time.Time) {
	s := string(msg)
	if s[3:7] == "RMC," {
		fmt.Printf("%s: %s\n", t.Format(time.RFC3339Nano), string(msg))
	}
}

func finishUbxMsg(msg []byte, t time.Time) error {
	ubxMsg, err := ubx.ParseMsg(msg)
	if err != nil {
		return err
	}
	if ubxMsg == nil {
		return nil
	}
	fmt.Printf("%s@%s: %+v\n", ubxMsg.ClsId(), t.Format(time.RFC3339Nano), ubxMsg.Payload())
	return nil
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
