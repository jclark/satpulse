package nmea

import (
	"github.com/jclark/satpulse/internal/gpsprot"
)

// PacketFormat returns the NMEA packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

// packetFormat implements the gpsprot.PacketFormat interface for NMEA packets
type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

// Constants for NMEA packet scanning (private)
const (
	stateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	stateStarted
	stateHadCaret
	stateHadCaretDigit1
	stateHadComma
	stateHadStar
	stateHadChecksum1
	stateHadChecksum2
	stateHadCR
	stateComplete
)

func (f packetFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]

	switch state {
	case stateSync:
		if b == '$' {
			return stateStarted
		}
	case stateStarted:
		if b == ',' || b == '*' {
			if packetLen >= 5 { // $PUBX
				if packetLen == 6 || buf[nextScanIndex-4] == 'P' {
					// allowed to have just address field
					if b == '*' {
						return stateHadStar
					}
					return stateHadComma
				}
			}
		} else if isAsciiUpperAlnum(b) && packetLen < 6 { // $GPRMC
			return stateStarted
		}
	case stateHadComma:
		if b == '*' {
			return stateHadStar
		}
		if b == '^' {
			if packetLen+2 < 82-5 {
				return stateHadCaret
			}
		} else if isDataByte(b) && packetLen < 82-5 { // 82 is total excluding 3-byte checksum and CRLF
			return stateHadComma
		}
	case stateHadCaret, stateHadCaretDigit1:
		if isUpperHexDigit(b) {
			return state + 1
		}
	case stateHadStar, stateHadChecksum1:
		if isUpperHexDigit(b) {
			return state + 1
		}
	case stateHadChecksum2:
		if b == '\r' {
			return stateHadCR
		}
		if b == '\n' {
			return stateComplete
		}
	case stateHadCR:
		if b == '\n' {
			return stateComplete
		}
	}

	return stateSync
}

func (f packetFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == stateComplete
}

// ExtractChecksum extracts the checksum from the NMEA packet.
// Precondition: the packet must be valid according to Next().
// We represent the checksum as a single byte in the expectation that when a checksum error is described the bytes will be printed as hex.
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	i := starIndex(pkt) + 1
	return []byte{(hexWeight(pkt[i]) << 4) | hexWeight(pkt[i+1])}
}

// ComputeChecksum computes the checksum for the NMEA packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	return []byte{Checksum(pkt[1:starIndex(pkt)])}
}

func (f packetFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	// no point in rescanning because valid packet constraints are quite strict
	return false
}

func Checksum(data []byte) byte {
	var c byte
	for i := 0; i < len(data); i++ {
		c ^= data[i]
	}
	return c
}

func starIndex(pkt []byte) int {
	starOffset := len(pkt) - 5
	if pkt[starOffset] != '*' {
		starOffset++
		if pkt[starOffset] != '*' {
			panic("Invalid NMEA packet passed to PacketFormat ComputeChecksum or ExtractChecksum")
		}
	}
	return starOffset
}

// Helper functions (private)
func isDataByte(b byte) bool {
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
