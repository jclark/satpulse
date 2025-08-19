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
	stateHadDollar
	stateNormal
	stateHadStar
	stateHadChecksum1
	stateHadChecksum2
	stateHadCR
	stateComplete
)

// maxSentenceLength is max length of NMEA sentence (including checksum and CRLF)
// NMEA specifies this as 82 characters, but modern receivers exceed this,
// because they need more precision particularly in the latitude and longitude fields.
// I am seeing this with the Unicore UM980: there is no option to limit the length of the sentence.
// U-blox also has a Limit82 flag to limit the length of the sentence to 82 characters,
// which implies that sentences will exceed this without the flag.
// The Rust NMEA crate has a limit of 102 characters, so let's follow that.
// Unicore receivers use something that is not strictly NMEA; and this packet format
// is designed to accept this, and it is documented as having a maximum length of 128 characters.
const maxSentenceLength = 128

// Next implements the gpsprot.PacketFormat interface for NMEA-like packets.
// Constraints for acceptable NMEA-like packet (not necessarily completely NMEA compliant)
// 1. First character is `$` and the packet does not contain any other `$` characters.
// 2. Terminated with a line terminator (CR/LF or LF)
// 3. All character before the line terminator must be printable ASCII (0x20-0x7E).
// 4. Total length of packet does not exceed 128 characters (including the line terminator).
// 5. Immediately before the line terminator there is a `*` and two uppercase hex digits;
//    this `*` is the only one in the packet.
// 6. The address field is non-empty, where the address is the substring between `$` and the first comma or `*`.
func (f packetFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]

	switch state {
	case stateSync:
		if b == '$' {
			return stateHadDollar
		}
	case stateHadDollar:
		if b == '*' || b == ',' {
			break
		}
		fallthrough
	case stateNormal:
		switch b {
		case '*':
			return stateHadStar
		case '$':
			break
		default:
			if b >= ' ' && b < 0x7f && packetLen < maxSentenceLength-5 { // excluding 3-byte checksum and CRLF
				return stateNormal
			}
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

func (f packetFormat) MsgID(pkt []byte) string {
	if pkt[1] == 'P' {
		return string(pkt[1:5])
	}
	return string(pkt[1:6])
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

var _ gpsprot.AltChecksumPacketFormat = packetFormat{}

// ComputeAltChecksum is designed to work around UM980 firmware quirk.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ComputeAltChecksum(pkt []byte) []byte {
	// This won't get called often, so no need to be efficient.
	flags := CheckSyntax(string(pkt))
	// If it looks like an NMEA packet with 5-char address and GNSS talker ID, then do not allow alternate checksum.
	if flags&SentenceAddressLength5 != 0 && flags&SentenceTalkerIsGNSS != 0 {
		return nil
	}
	// Since manufacturers using NMEA-like packets are purposefully ignoring the NMEA proprietary extension mechanism,
	// I think it's better no to do anything special for something that starts with P.
	// Possibly Unicore NMEA-like packet - compute checksum including the $
	return []byte{Checksum(pkt[0:starIndex(pkt)])}
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
