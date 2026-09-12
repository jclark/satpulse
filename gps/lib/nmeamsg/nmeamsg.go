package nmeamsg

import "github.com/jclark/satpulse/gps/lib/ascii"

// SentenceSyntaxFlags represents syntactic properties of an NMEA-like packet.
type SentenceSyntaxFlags uint32

// Definitions of the SentenceSyntaxFlags properties.
// If SentenceIsPacket is not set, no other flags will be set (the value will be 0).
// All other flags are only defined when SentenceIsPacket is set.
const (
	SentenceIsPacket SentenceSyntaxFlags = 1 << iota // meets all constraints for packetFormat.Next()
	// Following flags can be set only when SentenceIsPacket is set
	SentenceApprovedAddressFormat    // Address field is exactly 5 uppercase alphanumeric chars (digits + uppercase letters) and does not start with 'P'
	SentenceProprietaryAddressFormat // Address field starts with 'P' and three ASCII letters

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

// SentenceMaxLength is the maximum length of a valid NMEA-like packet,
// including checksum and CRLF. A packet ending in LF alone can be at most
// SentenceMaxLength-1 bytes, since the limit is defined for a CRLF ending.
//
// The NMEA 0183 standard specifies 82 characters, but modern receivers
// exceed this because they need more precision in latitude/longitude fields.
// U-blox has a Limit82 flag implying sentences exceed 82 without it.
// Unicore documents a maximum of 128 characters.
//
// Quectel PQTMNAV sentences are much longer: ~190 characters with current
// firmware fields, and up to ~340 if the 16 reserved fields are populated
// in future firmware. We use 400 to allow headroom.
const SentenceMaxLength = 400

// MaxGGANumSats is the largest satellite count representable by GGA field 7's
// two decimal digits.
const MaxGGANumSats = 99

// Composite flags (defined after iota sequence)
const (
	// Union of all GNSS talker flags for convenience
	SentenceTalkerIsGNSS SentenceSyntaxFlags = SentenceTalkerIsGP | SentenceTalkerIsGL | SentenceTalkerIsGA |
		SentenceTalkerIsGB | SentenceTalkerIsBD | SentenceTalkerIsGI |
		SentenceTalkerIsGQ | SentenceTalkerIsGN

	// Composite flag for approved NMEA validation (excludes GNSS talker check)
	SentenceApprovedNMEA SentenceSyntaxFlags = SentenceApprovedAddressFormat | SentenceValidCaretEscaping |
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

// IsValidApprovedNMEA checks if the flags represent a valid approved NMEA sentence.
// This does not check whether the talker ID and format are registered with NMEA.
// It does check that the address does not start with 'P'.
func (f SentenceSyntaxFlags) IsValidApprovedNMEA() bool {
	return f&SentenceApprovedNMEA == SentenceApprovedNMEA
}

// CheckSyntax analyzes an NMEA-like packet and returns its syntactic properties
func CheckSyntax(data string) SentenceSyntaxFlags {
	flags := SentenceIsPacket | SentenceValidDataChars | SentenceValidCaretEscaping | SentenceNoCarets

	n := len(data)
	if n > SentenceMaxLength {
		return 0
	}
	// Smallest acceptable packet is like $AA*00\n
	// Doing this early ensures no risk of out-of-bounds access later
	if n < 7 {
		return 0
	}
	if data[0] != '$' || !ascii.IsAlnum(data[1]) || !ascii.IsAlnum(data[2]) {
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
	} else if n == SentenceMaxLength {
		return 0
	}
	asteriskIndex := lineTerminatorIndex - 3
	if data[asteriskIndex] != '*' {
		return 0
	}
	if !ascii.IsUpperHexDigit(data[asteriskIndex+1]) || !ascii.IsUpperHexDigit(data[asteriskIndex+2]) {
		return 0
	}
	i := 3
	if asteriskIndex == 6 || data[6] == ',' {
		for {
			if !ascii.IsUpper(data[i]) && !ascii.IsDigit(data[i]) {
				break
			}
			if i == 5 {
				if data[1] != 'P' && !ascii.IsLower(data[1]) && !ascii.IsLower(data[2]) {
					flags |= SentenceApprovedAddressFormat
				}
				i++
				break
			}
			i++
		}
	}
	if data[1] == 'P' && ascii.IsLetter(data[2]) && ascii.IsLetter(data[3]) && ascii.IsLetter(data[4]) {
		flags |= SentenceProprietaryAddressFormat
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
			if !ascii.IsUpperHexDigit(data[i+1]) || !ascii.IsUpperHexDigit(data[i+2]) {
				flags &^= SentenceValidCaretEscaping
			}
		default:
			if !ascii.IsPrint(ch) {
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

func Checksum(data []byte) byte {
	var c byte
	for i := range data {
		c ^= data[i]
	}
	return c
}
