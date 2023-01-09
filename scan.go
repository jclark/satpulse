package main

import (
	"context"
	"time"

	"github.com/jclark/gps2phc/serial"
	"github.com/jclark/gps2phc/ubx"
	"golang.org/x/exp/slog"
)

type GpsMsg struct {
	U     ubx.Msg
	TRead time.Time
}

func serReadWorker(ctx context.Context, p *serial.Port, c chan GpsMsg) {
	buf := make([]byte, 255)
	msg := make([]byte, 0, 90)
	var state scanState
	var msgReadTime time.Time
Loop:
	for {
		select {
		case <-ctx.Done():
			break Loop
		default:
		}
		n, err := p.Read(buf)
		if err != nil {
			slog.FromContext(ctx).Error("readError", err)
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
				finishNmeaMsg(msg, msgReadTime)
				state = msgScan
				msg = msg[:0]
			case nmeaHadCR:
				// do nothing
			case ubxExpectN:
				msg = append(msg, b)
				ubxMsg, err := ubx.ParseMsg(msg)
				if err != nil {
					slog.FromContext(ctx).Error("ubxParseError", err)
				} else if ubxMsg != nil {
					c <- GpsMsg{U: ubxMsg, TRead: msgReadTime}
				}
				state = msgScan
				msg = msg[:0]
			default:
				msg = append(msg, b)
				if prevState == msgScan {
					msgReadTime = readTime
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
		//fmt.Printf("NMEA %s: %s\n", t.Format(time.RFC3339Nano), string(msg))
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
