package scan

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

type Packet struct {
	Format        gpsprot.PacketFormat
	ReadError     error
	Data          string
	TRead         time.Time
	ChecksumValid bool
}

func (pkt Packet) IsInterPacketTimeout() bool {
	if pkt.Format == nil && pkt.ReadError != nil && len(pkt.Data) == 0 {
		_, ok := pkt.ReadError.(TimeoutError)
		return ok
	}
	return false
}

func (pkt Packet) Tag() gpsprot.Tag {
	if pkt.Format == nil {
		return gpsprot.EmptyTag
	}
	return pkt.Format.Tag()
}

func (pkt Packet) HasTag(tag gpsprot.Tag) bool {
	return pkt.Format != nil && pkt.Format.Tag() == tag
}

// AltChecksumValid checks whether the packet has a valid alternate checksum,
// working around firmware bugs like the UM980 including $ in the NMEA checksum.
func (pkt Packet) AltChecksumValid() bool {
	if apf, ok := pkt.Format.(gpsprot.AltChecksumPacketFormat); ok {
		data := []byte(pkt.Data)
		return bytes.Equal(apf.ComputeAltChecksum(data), apf.ExtractChecksum(data))
	}
	return false
}

func (pkt Packet) ChecksumError() error {
	if pkt.ChecksumValid {
		return nil
	}
	buf := []byte(pkt.Data)
	return fmt.Errorf("incorrect checksum: in packet %x, computed %x", pkt.Format.ExtractChecksum(buf), pkt.Format.ComputeChecksum(buf))
}

type Scanner struct {
	pktFormats []gpsprot.PacketFormat
	r          io.Reader
	// there's valid data in buf up to len(buf); the capacity may be more
	buf []byte
	// nextScanIndex is first index not yet returned in a packet
	nextScanIndex int
	// time at which byte at nextScanIndex was read
	tRead        time.Time
	prevPktValid bool
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
func New(r io.Reader, bufSize int, pktFormats []gpsprot.PacketFormat) *Scanner {
	s := new(Scanner)
	s.pktFormats = pktFormats
	s.r = r
	s.buf = make([]byte, 0, bufSize)
	return s
}

const stateSync = gpsprot.ScanStateSync

const minInvalidPacketLen = 32

// Scan reads a packet from the underlying Reader.
// A transient error, such as a timeout, will be returned in the ReadError field of the packet
// with a Tag of Invalid, and err will be nil.
func (s *Scanner) Scan() (p Packet, err error) {
	state := stateSync
	// length of the packet so far
	// the packet is in the buffer preceding s.nextScanIndex
	packetLen := 0
	p = Packet{TRead: s.tRead}
	// this is non-nil if state != stateSync
	var curPktFormat gpsprot.PacketFormat
	var curPktFormatIndex int
Loop:
	for {
		if s.nextScanIndex >= len(s.buf) {
			if state == stateSync && packetLen >= minInvalidPacketLen {
				break Loop
			}
			e := s.fill(packetLen)
			if packetLen == 0 {
				p.TRead = s.tRead
			}
			if e != nil {
				if timeout, ok := e.(TimeoutError); ok && timeout.Timeout() {
					if packetLen == 0 && len(s.buf) == 0 {
						// don't wrap it, so it can be tested by Packet.IsInterPacketTimeout
						p.ReadError = e
					} else if packetLen > 0 && state == stateSync {
						// timeout in middle of invalid packet - not an error
						e = nil
					} else {
						n := s.nextScanIndex
						packetLen = s.restart(packetLen)
						if s.nextScanIndex == n {
							p.ReadError = fmt.Errorf("error in the middle of a packet: %w", e)
						} else {
							e = nil
						}
					}
				} else if temp, ok := e.(TemporaryError); ok && temp.Temporary() {
					p.ReadError = e
				}
				if p.ReadError == nil {
					// not a transient error
					err = e
				}
				break Loop
			}
		}
		var nextState gpsprot.ScanState
		if state != stateSync {
			nextState = curPktFormat.Next(state, s.buf, s.nextScanIndex, packetLen)
		} else {
			for i, pf := range s.pktFormats {
				nextState = pf.Next(state, s.buf, s.nextScanIndex, packetLen)
				if nextState != stateSync {
					curPktFormat = pf
					curPktFormatIndex = i
					break
				}
			}
		}
		// Looks like we may have a new packet.
		// If we have invalid data before the start of the packet, need to clear it out now.
		if nextState != stateSync && state == stateSync && packetLen > 0 {
			break Loop
		}
		if state != stateSync && nextState == stateSync && packetLen > 0 {
			// We had something that looked like the start of a packet,
			// but turned out to be invalid.

			// Check if a subsequent packet format would also match the first byte
			startIndex := s.nextScanIndex - packetLen
			for i := curPktFormatIndex + 1; i < len(s.pktFormats); i++ {
				pf := s.pktFormats[i]
				nextState = pf.Next(stateSync, s.buf, startIndex, 0)
				if nextState != stateSync {
					// It did, so rescan using that packet format from the second byte
					curPktFormat = pf
					curPktFormatIndex = i
					state = nextState
					s.nextScanIndex = startIndex + 1
					packetLen = 1
					goto Loop
				}
			}
			// No more formats match the first byte, so return bytes only up
			// to the next possible packet start within the failed candidate.
			packetLen = s.restart(packetLen)
			break Loop
		}
		// accept this character
		state = nextState
		packetLen++
		s.nextScanIndex++
		if state == stateSync {
			curPktFormat = nil
		} else if curPktFormat.IsFinal(state) {
			p.Format = curPktFormat
			break Loop
		}
	}
	pkt := s.buf[s.nextScanIndex-packetLen : s.nextScanIndex]
	if fmt := p.Format; fmt != nil {
		p.ChecksumValid = bytes.Equal(fmt.ExtractChecksum(pkt), fmt.ComputeChecksum(pkt))
		if !p.ChecksumValid && fmt.RescanOnBadChecksum(s.prevPktValid, pkt) {
			p.Format = nil
			state = stateSync
			packetLen = s.restart(packetLen)
			goto Loop
		}
	}
	s.prevPktValid = p.Format != nil && p.ChecksumValid
	p.Data = string(pkt)
	return
}

// restart moves the next scan point back to the first possible packet start
// inside an abandoned candidate, so invalid data cannot hide a valid packet.
func (s *Scanner) restart(packetLen int) int {
	start := s.nextScanIndex - packetLen
	for i := start + 1; i < s.nextScanIndex; i++ {
		for _, pf := range s.pktFormats {
			if pf.Next(stateSync, s.buf, i, 0) != stateSync {
				s.nextScanIndex = i
				return i - start
			}
		}
	}
	return packetLen
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
		}
		// we return packets of size 0, and we still want them to have a tRead
		s.tRead = time.Now()
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
