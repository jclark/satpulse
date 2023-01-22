package main

import (
	"context"
	"errors"
	"io"
	"time"

	"golang.org/x/sys/unix"
)

const (
	ubxSync1 = 0xB5
	ubxSync2 = 0x62
)

type FrameKind int

const (
	FrameInvalid = iota
	FrameUBX
	FrameNMEA
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

func NewScanner(r io.Reader, bufSize int) *Scanner {
	s := new(Scanner)
	s.r = r
	s.buf = make([]byte, 0, bufSize)
	return s
}

// When the error is non-nil, the packet may be of kind Invalid
func (s *Scanner) Read(ctx context.Context) (f Frame, err error) {
	var state scanState

	fStartIndex := s.nextScanIndex
	f = Frame{TRead: s.tRead}
Loop:
	for {
		if s.nextScanIndex >= len(s.buf) {
			if state == frameScan && fStartIndex < s.nextScanIndex {
				f.Kind = FrameInvalid
				break Loop
			}
			err = s.fill(ctx, fStartIndex)
			fStartIndex = 0
			if s.nextScanIndex == 0 {
				f.TRead = s.tRead
			}
			if err != nil {
				f.Kind = FrameInvalid
				break Loop
			}
		}
		state = s.nextState(state, s.nextScanIndex-fStartIndex, s.buf[s.nextScanIndex])
		s.nextScanIndex++
		switch state {
		case nmeaComplete:
			f.Kind = FrameNMEA
			break Loop
		case ubxExpectN:
			f.Kind = FrameUBX
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
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EINTR) {
			return err
		}
		// only loop on EINTR
	}
	return nil
}

type scanState int

const (
	frameScan scanState = iota
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

func (p *Scanner) nextState(state scanState, frameLen int, b byte) scanState {
	switch state {
	case frameScan:
		if b == '$' {
			return nmeaStarted
		}
		if b == ubxSync1 {
			return ubxStarted
		}
	case nmeaStarted:
		if b == ',' {
			if frameLen >= 4 { // $XYZ
				return nmeaHadComma
			}
		} else if isAsciiAlnum(b) && frameLen < 6 { // $GPRMC
			return nmeaStarted
		}
	case nmeaHadComma:
		if b == '*' {
			return nmeaHadStar
		}
		if isNmeaDataByte(b) && frameLen < 82-5 { // 82 is total excluding 3-byte checksum and CRLF
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
	default:
		if state > ubxExpectN {
			return state - 1
		}
	}
	return frameScan
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
