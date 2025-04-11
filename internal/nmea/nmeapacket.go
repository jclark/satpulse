package nmea

import (
	"github.com/jclark/satpulse/internal/gpsprot"
)

// PacketKind for NMEA packets
const PacketKind gpsprot.PacketKind = "NMEA"

// PacketFormat returns the NMEA packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

// packetFormat implements the gpsprot.PacketFormat interface for NMEA packets
type packetFormat struct{}

func (f packetFormat) Kind() gpsprot.PacketKind {
	return PacketKind
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
