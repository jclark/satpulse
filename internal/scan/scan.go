package scan

import (
	"fmt"
	"io"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/gpsreg"
)

type Packet struct {
	Kind      gpsprot.PacketKind
	ReadError error
	Data      string
	TRead     time.Time
}

type Scanner struct {
	pktFormats []gpsprot.PacketFormat
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
	s.pktFormats = gpsreg.PacketFormats
	s.r = r
	s.buf = make([]byte, 0, bufSize)
	return s
}

const stateSync = gpsprot.ScanStateSync
const Invalid = gpsprot.InvalidPacketKind

// Scan reads a packet from the underlying Reader.
// A transient error, such as a timeout, will be returned in the ReadError field of the packet
// with a Kind of Invalid, and err will be nil.
func (s *Scanner) Scan() (p Packet, err error) {
	state := stateSync
	// length of the packet so far
	// the packet is in the buffer preceding s.nextScanIndex
	packetLen := 0
	p = Packet{TRead: s.tRead}
	// this is non-nil if state != stateSync
	var curPktFormat gpsprot.PacketFormat
Loop:
	for {
		if s.nextScanIndex >= len(s.buf) {
			if state == stateSync && packetLen > 0 {
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
					p.ReadError = fmt.Errorf("error in the middle of a packet: %w", e)
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
		var nextState gpsprot.ScanState
		if state != stateSync {
			nextState = curPktFormat.Next(state, s.buf, s.nextScanIndex, packetLen)
		} else {
			for _, pf := range s.pktFormats {
				nextState = pf.Next(state, s.buf, s.nextScanIndex, packetLen)
				if nextState != stateSync {
					curPktFormat = pf
					break
				}
			}
		}
		// Looks like we may have a new packet.
		// If we have invalid data before the start of the packet, need to clear it out now.
		if nextState != stateSync && state == stateSync && packetLen > 0 {
			p.Kind = Invalid
			break Loop
		}
		if state != stateSync && nextState == stateSync && packetLen > 0 {
			// We had something that looked like the start of a packet,
			// but turned out to be invalid.
			// we need to start reprocessing it with the character that made it become invalid.
			// This is sufficient for UBX and NMEA, because the $ which starts an NMEA packet
			// isn't allowed with an NMEA packet. For UBX, the only way it's invalid is if the
			// length is wrong or it didn't have the right second sync byte.
			p.Kind = Invalid
			break Loop
		}
		// accept this character
		state = nextState
		packetLen++
		s.nextScanIndex++
		if state == stateSync {
			curPktFormat = nil
		} else if curPktFormat.IsFinal(state) {
			p.Kind = curPktFormat.Kind()
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

// LooksLike returns the packet kind that it looks like
func LooksLike(pktFormats []gpsprot.PacketFormat, buf []byte) gpsprot.PacketKind {
	for i := 0; i < len(pktFormats); i++ {
		if pktFormats[i].Next(stateSync, buf, 0, 0) != stateSync {
			return pktFormats[i].Kind()
		}
	}
	return gpsprot.InvalidPacketKind
}
