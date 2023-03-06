package scan

import (
	"context"
	"io"
	"time"

	"github.com/jclark/crc24q/crc24q"
)

type FrameKind int

const (
	Invalid FrameKind = iota
	UBX
	NMEA
	RTCM
)

type Frame struct {
	Kind  FrameKind
	Data  string
	TRead time.Time
}

type Scanner struct {
	r io.Reader
	// len(buf) says how much data was read
	buf []byte
	// nextScanIndex is first index not yet returned in a packet
	nextScanIndex int
	// time at which byte at nextScanIndex was read
	tRead time.Time
}

func New(r io.Reader, bufSize int) *Scanner {
	s := new(Scanner)
	s.r = r
	s.buf = make([]byte, 0, bufSize)
	return s
}

// When the error is non-nil, the packet may be of kind Invalid
func (s *Scanner) Scan(ctx context.Context) (f Frame, err error) {
	var state scanState

	fStartIndex := s.nextScanIndex
	f = Frame{TRead: s.tRead}
Loop:
	for {
		if s.nextScanIndex >= len(s.buf) {
			if state == frameScan && fStartIndex < s.nextScanIndex {
				f.Kind = Invalid
				break Loop
			}
			err = s.fill(ctx, fStartIndex)
			fStartIndex = 0
			if s.nextScanIndex == 0 {
				f.TRead = s.tRead
			}
			if err != nil {
				f.Kind = Invalid
				break Loop
			}
		}
		state = s.nextState(state, s.nextScanIndex-fStartIndex, s.buf[s.nextScanIndex])
		s.nextScanIndex++
		switch state {
		case nmeaComplete:
			f.Kind = NMEA
			break Loop
		case ubxExpectN:
			f.Kind = UBX
			break Loop
		case rtcmExpectN:
			f.Kind = RTCM
			break Loop
		}
	}
	f.Data = string(s.buf[fStartIndex:s.nextScanIndex])
	return
}

// This returns the error it got from the Read, except in the case of EINTR
func (s *Scanner) fill(ctx context.Context, fStartIndex int) error {
	// move the partial packet to the start of the buffer
	// and grow the buffer if the partial packet uses more than half the buffer
	keep := s.buf[fStartIndex:s.nextScanIndex]
	nKeep := len(keep)
	if nKeep <= cap(s.buf)/2 {
		s.buf = s.buf[0:nKeep]
	} else {
		s.buf = make([]byte, nKeep, cap(s.buf)*2)
	}
	s.nextScanIndex = nKeep
	copy(s.buf, keep)
	rBuf := s.buf[nKeep:cap(s.buf)]
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := s.r.Read(rBuf)
		if n > 0 {
			s.buf = s.buf[0 : len(s.buf)+n]
			s.tRead = time.Now()
		}
		if err != nil {
			return err
		}
		if n > 0 {
			break
		}
		// loop on zero bytes and no error
	}
	return nil
}

type scanState int

const (
	frameScan scanState = iota
	nmeaStarted
	nmeaHadCaret
	nmeaHadCaretDigit1 // we depend on nmeaHadComma being after nmeaHadCaretDigit1
	nmeaHadComma
	nmeaHadStar
	nmeaHadChecksum1
	nmeaHadChecksum2
	nmeaHadCR
	nmeaComplete
	ubxStarted
	rtcmStarted
	ubxExpectN
	rtcmExpectN = ubxExpectN + 0x10000 + 2
)

const (
	ubxSync1     = 0xB5
	ubxSync2     = 0x62
	rtcmPreamble = 0xD3
)

func (p *Scanner) nextState(state scanState, frameLen int, b byte) scanState {
	switch state {
	case frameScan:
		switch b {
		case '$':
			return nmeaStarted
		case ubxSync1:
			return ubxStarted
		case rtcmPreamble:
			return rtcmStarted
		}
	case nmeaStarted:
		if b == ',' || b == '*' {
			if frameLen >= 5 { // $PUBX
				if frameLen == 6 || p.buf[p.nextScanIndex-4] == 'P' {
					// allowed to have just address field
					if b == '*' {
						return nmeaHadStar
					}
					return nmeaHadComma
				}
			}
		} else if isAsciiUpperAlnum(b) && frameLen < 6 { // $GPRMC
			return nmeaStarted
		}
	case nmeaHadComma:
		if b == '*' {
			return nmeaHadStar
		}
		if b == '^' {
			if frameLen+2 < 82-5 {
				return nmeaHadCaret
			}
		} else if isNmeaDataByte(b) && frameLen < 82-5 { // 82 is total excluding 3-byte checksum and CRLF
			return nmeaHadComma
		}
	case nmeaHadCaret, nmeaHadCaretDigit1:
		if isUpperHexDigit(b) {
			return state + 1
		}
	case nmeaHadStar, nmeaHadChecksum1:
		if isUpperHexDigit(b) {
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
		switch frameLen {
		case 1:
			if b == ubxSync2 {
				return ubxStarted
			}
		case 5:
			payloadLen := int(p.buf[p.nextScanIndex-1]) + int(b)*0x100
			return scanState(int(ubxExpectN) + payloadLen + 2)
		default:
			return ubxStarted
		}
	case rtcmStarted:
		switch frameLen {
		case 2:
			payloadLen := int(b) + int(p.buf[p.nextScanIndex-1]&0x3)*0x100
			return scanState(int(rtcmExpectN) + payloadLen + 3)
		case 1:
			return rtcmStarted
		}
	default:
		if state > ubxExpectN {
			return state - 1
		}
	}
	return frameScan
}

func isNmeaDataByte(b byte) bool {
	if b < ' ' || b >= 0x7f {
		return false
	}
	switch b {
	case '*', '$', '^', '!':
		return false
	default:
		return true
	}
}

func isUpperHexDigit(b byte) bool {
	if '0' <= b && b <= '9' {
		return true
	}
	// NMEA requires checksum to use upper-case hex digits
	if 'A' <= b && b <= 'F' {
		return true
	}
	return false
}

func isAsciiUpperAlnum(b byte) bool {
	if 'A' <= b && b <= 'Z' {
		return true
	}
	if '0' <= b && b <= '9' {
		return true
	}
	return false
}

func RTCMMsg(frame string) (msg string, checksumOK bool, msgType uint16) {
	n := len(frame) - 3
	checksumOK = crc24q.Checksum(frame[0:n]) == crc24q.Extract(frame, n)
	msg = frame[3:n]
	// treat 0-length message as type 0
	if n != 3 {
		msgType = (uint16(msg[0]) << 4) | uint16(msg[1]>>4)
	}
	return
}
