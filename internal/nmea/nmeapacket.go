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

// SentenceSyntaxFlags represents syntactic properties of an NMEA-like packet.
type SentenceSyntaxFlags uint32

// Definitions of the SentenceSyntaxFlags properties.
// If SentenceIsPacket is not set, no other flags will be set (the value will be 0).
// All other flags are only defined when SentenceIsPacket is set.
const (
	SentenceIsPacket SentenceSyntaxFlags = 1 << iota // meets all constraints for packetFormat.Next()
	// Following flags can be set only when SentenceIsPacket is set
	SentenceAddressLength5           // Address field is exactly 5 uppercase alphanumeric chars (digits + uppercase letters)
	SentenceProprietaryAddressFormat // Address field is all uppercase alphanumeric chars, starting with 'P' and length 4 or more

	// GNSS Talker ID bits (mutually exclusive)
	// Tests the first two characters after initial $
	SentenceTalkerIsGP // GPS
	SentenceTalkerIsGL // GLONASS
	SentenceTalkerIsGA // Galileo
	SentenceTalkerIsGB // BeiDou (current)
	SentenceTalkerIsBD // BeiDou (legacy)
	SentenceTalkerIsGI // NavIC
	SentenceTalkerIsGQ // QZSS
	SentenceTalkerIsGN // Multi-GNSS

	// Character validation
	SentenceNoCarets           // No ^ characters
	SentenceValidCaretEscaping // All ^ followed by exactly 2 hex digits (true even if no ^)
	SentenceValidDataChars     // No occurrences of backslash, exclamation mark or tilde in the packet.
	SentenceLength82OrLess     // Length ≤ 82 chars
	SentenceEndsWithCRLF       // Ends with CRLF (\r\n) (not just LF)

)

const SentenceMaxLength = 128 // Maximum length of a valid NMEA-like packet

// Composite flags (defined after iota sequence)
const (
	// Union of all GNSS talker flags for convenience
	SentenceTalkerIsGNSS SentenceSyntaxFlags = SentenceTalkerIsGP | SentenceTalkerIsGL | SentenceTalkerIsGA |
		SentenceTalkerIsGB | SentenceTalkerIsBD | SentenceTalkerIsGI |
		SentenceTalkerIsGQ | SentenceTalkerIsGN

	// Composite flag for approved NMEA validation (excludes GNSS talker check)
	SentenceApprovedNMEA SentenceSyntaxFlags = SentenceAddressLength5 | SentenceValidCaretEscaping |
		SentenceValidDataChars | SentenceEndsWithCRLF

	// Composite flag for proprietary NMEA validation
	SentenceProprietaryNMEA SentenceSyntaxFlags = SentenceProprietaryAddressFormat | SentenceValidDataChars |
		SentenceEndsWithCRLF
)

// SentenceSyntaxFlags Methods

// IsValidGNSSTalkerNMEA checks if the flags represent a valid approved NMEA sentence with a GNSS talker ID.
func (f SentenceSyntaxFlags) IsValidGNSSTalkerNMEA() bool {
	return f&SentenceApprovedNMEA == SentenceApprovedNMEA && f&SentenceTalkerIsGNSS != 0
}

// IsValidApprovedNMEA checks if the flags represent a valid proprietary NMEA sentence.
func (f SentenceSyntaxFlags) IsValidProprietaryNMEA() bool {
	return f&SentenceProprietaryNMEA == SentenceProprietaryNMEA
}

// CheckSyntax analyzes an NMEA-like packet and returns its syntactic properties
func CheckSyntax(data string) SentenceSyntaxFlags {
	flags := SentenceIsPacket | SentenceValidDataChars | SentenceValidCaretEscaping | SentenceNoCarets

	n := len(data)
	if n > SentenceMaxLength {
		return 0
	}
	// Smallest acceptable packet is like $_*00\n
	// Doing this early ensures no risk of out-of-bounds access later
	if n < 6 {
		return 0
	}
	if data[0] != '$' {
		return 0
	}
	if n <= 82 {
		flags |= SentenceLength82OrLess
	}
	lineTerminatorIndex := n - 1

	if data[lineTerminatorIndex] != '\n' {
		return 0
	}
	if data[lineTerminatorIndex-1] == '\r' {
		lineTerminatorIndex--
		flags |= SentenceEndsWithCRLF
	}
	asteriskIndex := lineTerminatorIndex - 3
	if data[asteriskIndex] != '*' {
		return 0
	}
	if !isUpperHexDigit(data[asteriskIndex+1]) || !isUpperHexDigit(data[asteriskIndex+2]) {
		return 0
	}
	i := 1
Loop:
	for ; i < asteriskIndex; i++ {
		switch data[i] {
		case 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
			'0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			// Valid address character
			// do nothing
		default:
			break Loop
		}
	}
	if i == asteriskIndex || data[i] == ',' {
		if i == 1 {
			return 0 // Address cannot be empty
		}
		// address part is all  uppercase alphanumric characters
		if i == 6 {
			flags |= SentenceAddressLength5 // Address is exactly 5 characters
		}
		if data[1] == 'P' && i >= 5 {
			flags |= SentenceProprietaryAddressFormat // Address starts with P + 3+ uppercase alphanumeric chars
		}
	}

	for ; i < asteriskIndex; i++ {
		ch := data[i]
		switch ch {
		case '$', '*':
			return 0 // not a valid packet
		case '!', '\\', '~':
			flags &^= SentenceValidDataChars
		case '^':
			flags &^= SentenceNoCarets
			// This is safe because we already checked that there are at least 3 characters after the asterisk
			if !isUpperHexDigit(data[i+1]) || !isUpperHexDigit(data[i+2]) {
				flags &^= SentenceValidCaretEscaping
			}
		default:
			if ch < 0x20 || ch > 0x7E { // Printable ASCII range
				return 0
			}
		}
	}
	switch data[1] {
	case 'G':
		switch data[2] {
		case 'P':
			flags |= SentenceTalkerIsGP
		case 'L':
			flags |= SentenceTalkerIsGL
		case 'A':
			flags |= SentenceTalkerIsGA
		case 'B':
			flags |= SentenceTalkerIsGB
		case 'I':
			flags |= SentenceTalkerIsGI
		case 'Q':
			flags |= SentenceTalkerIsGQ
		case 'N':
			flags |= SentenceTalkerIsGN
		}
	case 'B':
		if data[2] == 'D' {
			flags |= SentenceTalkerIsBD
		}
	}
	return flags
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