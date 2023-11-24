package scan

import (
	"fmt"
	"io"
	"time"

	"github.com/jclark/crc24q"
)

type PacketKind uint16

const (
	Invalid PacketKind = iota
	UBX
	NMEA
	RTCM
)

type Packet struct {
	Kind      PacketKind
	ReadError error
	Data      string
	TRead     time.Time
}

type Scanner struct {
	r io.Reader
	// there's valid data in buf up to len(buf); the capacity may be more
	buf []byte
	// nextScanIndex is first index not yet returned in a packet
	nextScanIndex int
	// time at which byte at nextScanIndex was read
	tRead time.Time
}

// A TimeoutError indicates a read timed out.
// A Reader should produce this error for a read that timed out.
// Timeouts are allowed in between packets, but not in the middle of a packet.
type TimeoutError interface {
	error
	Timeout() bool
}

type TemporaryError interface {
	error
	Temporary() bool
}

// New returns a new Scanner to read from r.
func New(r io.Reader, bufSize int) *Scanner {
	s := new(Scanner)
	s.r = r
	s.buf = make([]byte, 0, bufSize)
	return s
}

// Scan reads a packet from the underlying Reader.
// A transient error, such as a timeout, will be returned in the ReadError field of the packet
// with a Kind of Invalid, and err will be nil.
func (s *Scanner) Scan() (p Packet, err error) {
	state := syncScan
	// length of the packet so far
	// the packet is in the buffer preceding s.nextScanIndex
	packetLen := 0
	p = Packet{TRead: s.tRead}
Loop:
	for {
		if s.nextScanIndex >= len(s.buf) {
			if state == syncScan && packetLen > 0 {
				p.Kind = Invalid
				break Loop
			}
			e := s.fill(packetLen)
			if packetLen == 0 {
				p.TRead = s.tRead
			}
			if e != nil {
				if timeout, ok := e.(TimeoutError); ok && timeout.Timeout() {
					if packetLen == 0 && len(s.buf) == 0 {
						// a timeout in between packets is OK, just keep on going
						continue Loop
					}
					p.ReadError = fmt.Errorf("%w in the middle of a packet", err)
				} else if temp, ok := e.(TemporaryError); ok && temp.Temporary() {
					p.ReadError = e
				}
				if p.ReadError == nil {
					// not a transient error
					err = e
				}
				p.Kind = Invalid
				break Loop
			}
		}
		nextState := state.next(s.buf, s.nextScanIndex, packetLen)
		// Looks like we may have a new packet.
		// If we have invalid data before the start of the packet, need to clear it out now.
		if nextState != syncScan && state == syncScan && packetLen > 0 {
			p.Kind = Invalid
			break Loop
		}
		// accept this character
		state = nextState
		packetLen++
		s.nextScanIndex++
		switch state {
		case nmeaComplete:
			p.Kind = NMEA
			break Loop
		case ubxExpectN:
			p.Kind = UBX
			break Loop
		case rtcmExpectN:
			p.Kind = RTCM
			break Loop
		}
	}
	p.Data = string(s.buf[s.nextScanIndex-packetLen : s.nextScanIndex])
	return
}

// This returns the error it got from the Read, except in the case of EINTR.
// The packetLen bytes up to nextScanIndex must be kept.
func (s *Scanner) fill(packetLen int) error {
	// move the partial packet to the start of the buffer
	// and grow the buffer if the partial packet uses more than half the buffer
	packetData := s.buf[s.nextScanIndex-packetLen : s.nextScanIndex]
	if packetLen <= cap(s.buf)/2 {
		s.buf = s.buf[0:packetLen]
	} else {
		// reallocate buffer
		s.buf = make([]byte, packetLen, cap(s.buf)*2)
	}
	// store current packet at the beginning of buffer
	copy(s.buf, packetData)
	s.nextScanIndex = packetLen
	for {
		rBuf := s.buf[packetLen:cap(s.buf)]
		n, err := s.r.Read(rBuf)
		if n > 0 {
			s.buf = s.buf[0 : packetLen+n]
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
	syncScan scanState = iota
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

// Return a new state based on the byte at nextScanIndex.
// packetLen is number of bytes in the packet not including the one at nextScanIndex
func (state scanState) next(buf []byte, nextScanIndex int, packetLen int) scanState {
	b := buf[nextScanIndex]
	switch state {
	case syncScan:
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
			if packetLen >= 5 { // $PUBX
				if packetLen == 6 || buf[nextScanIndex-4] == 'P' {
					// allowed to have just address field
					if b == '*' {
						return nmeaHadStar
					}
					return nmeaHadComma
				}
			}
		} else if isAsciiUpperAlnum(b) && packetLen < 6 { // $GPRMC
			return nmeaStarted
		}
	case nmeaHadComma:
		if b == '*' {
			return nmeaHadStar
		}
		if b == '^' {
			if packetLen+2 < 82-5 {
				return nmeaHadCaret
			}
		} else if isNmeaDataByte(b) && packetLen < 82-5 { // 82 is total excluding 3-byte checksum and CRLF
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
		switch packetLen {
		case 1:
			if b == ubxSync2 {
				return ubxStarted
			}
			// XXX better handle the case where b is ubxSync1
		case 5:
			payloadLen := int(buf[nextScanIndex-1]) + int(b)*0x100
			return scanState(int(ubxExpectN) + payloadLen + 2)
		default:
			return ubxStarted
		}
	case rtcmStarted:
		switch packetLen {
		case 2:
			payloadLen := int(b) + int(buf[nextScanIndex-1]&0x3)*0x100
			return scanState(int(rtcmExpectN) + payloadLen + 3)
		case 1:
			return rtcmStarted
		}
	default:
		if state > ubxExpectN {
			return state - 1
		}
	}
	return syncScan
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

func RTCMMsg(packet string) (msg string, checksumOK bool, msgType uint16) {
	n := len(packet) - 3
	checksumOK = crc24q.Checksum(packet[0:n]) == crc24q.Extract(packet, n)
	msg = packet[3:n]
	// treat 0-length message as type 0
	if n != 3 {
		msgType = (uint16(msg[0]) << 4) | uint16(msg[1]>>4)
	}
	return
}
