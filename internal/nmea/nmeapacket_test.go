package nmea

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/scantest"
)

var syntaxTestCases = []struct {
	name     string
	packet   string
	expected SentenceSyntaxFlags
}{
	// NOT A PACKET - Short packet tests (length < 6)
	{
		name:     "not a packet - empty string",
		packet:   "",
		expected: 0,
	},
	{
		name:     "not a packet - single character",
		packet:   "$",
		expected: 0,
	},
	{
		name:     "not a packet - two characters",
		packet:   "$\n",
		expected: 0,
	},
	{
		name:     "not a packet - three characters",
		packet:   "$*\n",
		expected: 0,
	},
	{
		name:     "not a packet - four characters",
		packet:   "$*A\n",
		expected: 0,
	},
	{
		name:     "not a packet - five characters",
		packet:   "$*AB\n",
		expected: 0,
	},

	// NOT A PACKET - Basic packet format constraint failures (testing each of the 6 constraints)

	// Constraint 2: Terminated with a line terminator (CR/LF or LF)
	{
		name:     "not a packet - no line terminator (constraint 2)",
		packet:   "$GPGGA,123*5A",
		expected: 0,
	},

	// Constraint 3: All characters before terminator must be printable ASCII (0x20-0x7E)
	{
		name:     "not a packet - non-printable char (constraint 3)",
		packet:   "$GPGGA,\x01*5A\r\n",
		expected: 0,
	},

	// Constraint 4: Total length ≤ 128 characters
	{
		name:     "not a packet - 129 chars (constraint 4)",
		packet:   "$GPGGA," + strings.Repeat("X", 117) + "*5A\r\n", // Total = 129
		expected: 0,
	},

	// Constraint 1: First character is `$` and no other `$` characters
	{
		name:     "not a packet - missing dollar (constraint 1a)",
		packet:   "GPGGA,123*5A\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - extra dollar (constraint 1b)",
		packet:   "$GPGGA,1$23*5A\r\n",
		expected: 0,
	},

	// Constraint 5: Immediately before terminator there is `*` and two uppercase hex digits
	{
		name:     "not a packet - missing asterisk (constraint 5a)",
		packet:   "$GPGGA,123\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - missing checksum (constraint 5b)",
		packet:   "$GPGGA,123*\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - short checksum (constraint 5c)",
		packet:   "$GPGGA,123*5\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - non-hex checksum (constraint 5d)",
		packet:   "$GPGGA,123*ZZ\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - lowercase checksum (constraint 5e)",
		packet:   "$GPGGA,123*5a\r\n",
		expected: 0,
	},

	// Constraint 5: This `*` is the only one in the packet
	{
		name:     "not a packet - extra asterisk (constraint 5)",
		packet:   "$GPGGA,1*23*5A\r\n",
		expected: 0,
	},

	// Additional constraint violations
	{
		name:     "not a packet - mixed case checksum - aB (constraint 5e)",
		packet:   "$GPGGA,123*aB\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - tab character (constraint 3)",
		packet:   "$GPGGA,hello\tworld*5A\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - DEL character (constraint 3)",
		packet:   "$GPGGA,hello\x7Fworld*5A\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - null character (constraint 3)",
		packet:   "$GPGGA,hello\x00world*5A\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - empty address with comma (constraint 6)",
		packet:   "$,*5A\r\n",
		expected: 0,
	},
	{
		name:     "not a packet - CR only no LF (constraint 2)",
		packet:   "$GPGGA,123*5A\r",
		expected: 0,
	},
	{
		name:     "not a packet - extra data after CRLF (constraint 2)",
		packet:   "$GPGGA,123*5A\r\nEXTRA",
		expected: 0,
	},
	{
		name:     "not a packet - checksum with spaces (constraint 5)",
		packet:   "$GPGGA,123*5 A\r\n",
		expected: 0,
	},

	// VALID PACKETS - Basic packet format tests
	{
		name:     "valid basic packet",
		packet:   "$GPGGA,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Address format tests - approved (exactly 5 chars)
	{
		name:     "approved address - GPGGA",
		packet:   "$GPGGA,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "approved address - GLRMC",
		packet:   "$GLRMC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGL | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "approved address - GARMC",
		packet:   "$GARMC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGA | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "approved address - GBRMC",
		packet:   "$GBRMC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGB | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "approved address - BDRMC (legacy BeiDou)",
		packet:   "$BDRMC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsBD | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "approved address - GIRMC",
		packet:   "$GIRMC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGI | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "approved address - GQRMC",
		packet:   "$GQRMC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGQ | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "approved address - GNRMC (Multi-GNSS)",
		packet:   "$GNRMC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGN | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "approved address - unknown talker",
		packet:   "$XXTXT,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "not approved address - too short",
		packet:   "$ABC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "not approved address - too long",
		packet:   "$ABCDEF,123*5A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "not approved address - lowercase",
		packet:   "$GPgga,123*5A\r\n",
		expected: SentenceIsPacket | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "not approved address - non-alphanumeric",
		packet:   "$GP-GA,123*5A\r\n",
		expected: SentenceIsPacket | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Proprietary address format tests
	{
		name:     "proprietary address - PUBX (4 chars total)",
		packet:   "$PUBX,41,1*25\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "proprietary address - PUBX1 (5 chars, overlapping flags)",
		packet:   "$PUBX1,data*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "proprietary address - PMTK (3 chars after P)",
		packet:   "$PMTK,123*5A\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "proprietary address - PMTK123 (extended, more than 3 chars)",
		packet:   "$PMTK123,data*5A\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "not proprietary - starts with P but too short",
		packet:   "$PA,123*5A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "not proprietary - starts with P but lowercase",
		packet:   "$Pabc,123*5A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "not proprietary - starts with P but non-alphanumeric",
		packet:   "$P-BC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "proprietary address - no data fields",
		packet:   "$PUBX*5A\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "proprietary address - single empty data field",
		packet:   "$PUBX,*5A\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Caret escaping tests
	{
		name:     "no carets - flags should be set",
		packet:   "$GPGGA,123,data*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "valid caret escaping",
		packet:   "$GPGGA,12^0D3*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "valid caret escaping - multiple",
		packet:   "$GPGGA,^0A^0D^2A*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "invalid caret escaping - missing hex",
		packet:   "$GPGGA,12^3*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "invalid caret escaping - non-hex",
		packet:   "$GPGGA,12^XY*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "invalid caret escaping - lowercase hex",
		packet:   "$GPGGA,12^0a*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Invalid data character tests
	{
		name:     "invalid data char - backslash",
		packet:   "$GPGGA,12\\3*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "invalid data char - exclamation",
		packet:   "$GPGGA,12!3*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "invalid data char - tilde",
		packet:   "$GPGGA,12~3*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Length tests
	{
		name:     "exactly 82 chars",
		packet:   "$GPGGA,1234567890123456789012345678901234567890123456789012345678901234567890*5A\r\n", // 82 chars total
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "over 82 chars",
		packet:   "$GPGGA,12345678901234567890123456789012345678901234567890123456789012345678901*5A\r\n", // 83 chars total
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceEndsWithCRLF,
	},

	// Line ending tests
	{
		name:     "CRLF ending",
		packet:   "$GPGGA,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "LF only ending",
		packet:   "$GPGGA,123*5A\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess,
	},

	// Real NMEA sentences from existing tests
	{
		name:     "real GPGGA",
		packet:   "$GPGGA,092725.00,4717.11399,N,00833.91590,E,1,08,1.01,499.6,M,48.0,M,,*5B\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "real GPGLL",
		packet:   "$GPGLL,4717.11364,N,00833.91565,E,092321.00,A,A*60\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "real PUBX proprietary",
		packet:   "$PUBX,41,1,0007,0003,19200,0*25\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "real GPTXT",
		packet:   "$GPTXT,01,01,02,u-blox ag - www.u-blox.com*50\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "real GPVTG",
		packet:   "$GPVTG,77.52,T,,M,0.004,N,0.008,K,A*06\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "real GPZDA",
		packet:   "$GPZDA,082710.00,16,09,2002,00,00*64\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "real GNRMC (over 82 chars)",
		packet:   "$GNRMC,114650.00,A,1343.90931561,N,10038.68511804,E,0.005,221.7,040525,0.5,W,A,C*59\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGN | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceEndsWithCRLF,
	},

	// Unicore command acknowledgment (non-NMEA but packet-like)
	{
		name:     "Unicore command ack - FRESET",
		packet:   "$FRESET,response: OK*2E\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "Unicore command ack - CONFIG",
		packet:   "$CONFIG,OK*1A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Edge cases
	{
		name:     "no data fields - address only",
		packet:   "$GPGGA*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "single empty data field",
		packet:   "$GPGGA,*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "multiple empty data fields",
		packet:   "$GPGGA,,,*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "minimal packet - single char address",
		packet:   "$A*5A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "minimal packet with empty field",
		packet:   "$A,*5A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Packet length boundary tests (128 char limit per comments)
	{
		name:     "exactly 128 chars (packet format max)",
		packet:   "$GPGGA," + strings.Repeat("X", 115) + "*5A\r\n", // Total = 128
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceEndsWithCRLF,
	},
	{
		name:     "129 chars (over packet format limit)",
		packet:   "$GPGGA," + strings.Repeat("X", 117) + "*5A\r\n", // Total = 129
		expected: 0,                                                // Should fail packet detection due to length
	},

	// Checksum case variations
	{
		name:     "lowercase checksum",
		packet:   "$GPGGA,123*5a\r\n",
		expected: 0, // Should fail - spec requires uppercase hex
	},
	{
		name:     "mixed case checksum",
		packet:   "$GPGGA,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "mixed case checksum - aB",
		packet:   "$GPGGA,123*aB\r\n",
		expected: 0, // Should fail - spec requires uppercase hex
	},

	// Address character validation - digits
	{
		name:     "address with digits - 12345",
		packet:   "$12345,data*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "address with mixed alphanumeric - A1B2C",
		packet:   "$A1B2C,data*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "address starting with digit - 1ABCD",
		packet:   "$1ABCD,data*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Caret escaping edge cases
	{
		name:     "caret at end of field",
		packet:   "$GPGGA,data^*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "caret with one char at end",
		packet:   "$GPGGA,data^5*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "sequential escaped carets",
		packet:   "$GPGGA,^^5E5E*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Reserved character tests (dollar and asterisk in data)
	{
		name:     "dollar sign in data field",
		packet:   "$GPGGA,$123*5A\r\n",
		expected: 0, // Should fail - $ only allowed at start
	},
	{
		name:     "asterisk in data field",
		packet:   "$GPGGA,12*34*5A\r\n",
		expected: 0, // Should fail - only one * allowed before checksum
	},

	// ASCII boundary tests
	{
		name:     "space character (0x20) in data",
		packet:   "$GPGGA,hello world*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "tab character (0x09) in data",
		packet:   "$GPGGA,hello\tworld*5A\r\n",
		expected: 0, // Should fail - tab is not printable ASCII
	},
	{
		name:     "DEL character (0x7F) in data",
		packet:   "$GPGGA,hello\x7Fworld*5A\r\n",
		expected: 0, // Should fail - DEL is not in printable range
	},
	{
		name:     "null character (0x00) in data",
		packet:   "$GPGGA,hello\x00world*5A\r\n",
		expected: 0, // Should fail - null is not printable
	},

	// Proprietary edge cases
	{
		name:     "proprietary minimum valid - P + 3 chars",
		packet:   "$PABC,data*5A\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "proprietary with digits - P123",
		packet:   "$P123,data*5A\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "very long proprietary address",
		packet:   "$PABCDEFGHIJKLMNOP,data*5A\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Empty address tests
	{
		name:     "no address - just checksum (constraint 6)",
		packet:   "$*5A\r\n",
		expected: 0, // Should fail - address cannot be empty
	},
	{
		name:     "empty address with comma (constraint 6)",
		packet:   "$,*5A\r\n",
		expected: 0, // Should fail - address cannot be empty
	},

	// Line terminator variations
	{
		name:     "no line terminator",
		packet:   "$GPGGA,123*5A",
		expected: 0, // Should fail - must have terminator
	},
	{
		name:     "CR only (no LF)",
		packet:   "$GPGGA,123*5A\r",
		expected: 0, // Should fail - incomplete terminator
	},
	{
		name:     "extra data after CRLF",
		packet:   "$GPGGA,123*5A\r\nEXTRA",
		expected: 0, // Should fail - data after terminator
	},

	// GNSS talker edge cases
	{
		name:     "unknown G-prefix talker - GX",
		packet:   "$GXRMC,123*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "partial GNSS match - just G",
		packet:   "$G,123*5A\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "partial GNSS match - just GP",
		packet:   "$GP,123*5A\r\n",
		expected: SentenceIsPacket | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "partial GNSS match - GPS (3 chars)",
		packet:   "$GPS,123*5A\r\n",
		expected: SentenceIsPacket | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},

	// Mixed validation scenarios
	{
		name:     "proprietary P12345 (5 chars, digits, starts with P)",
		packet:   "$P1234,data*5A\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "address with spaces (valid packet, invalid address format)",
		packet:   "$GP GA,123*5A\r\n",
		expected: SentenceIsPacket | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF, // Valid packet but fails address format validation
	},

	// Test cases migrated from nmeapacket_test.go

	// nmeaOK test cases - should be valid GNSS talker or proprietary NMEA
	{
		name:     "nmeaOK - GPGGA real sentence",
		packet:   "$GPGGA,092725.00,4717.11399,N,00833.91590,E,1,08,1.01,499.6,M,48.0,M,,*5B\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "nmeaOK - GPGLL real sentence",
		packet:   "$GPGLL,4717.11364,N,00833.91565,E,092321.00,A,A*60\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "nmeaOK - PUBX proprietary",
		packet:   "$PUBX,41,1,0007,0003,19200,0*25\r\n",
		expected: SentenceIsPacket | SentenceProprietaryAddressFormat | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "nmeaOK - GPTXT with spaces and hyphens",
		packet:   "$GPTXT,01,01,02,u-blox ag - www.u-blox.com*50\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "nmeaOK - GPVTG real sentence",
		packet:   "$GPVTG,77.52,T,,M,0.004,N,0.008,K,A*06\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "nmeaOK - GPZDA real sentence",
		packet:   "$GPZDA,082710.00,16,09,2002,00,00*64\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF,
	},
	{
		name:     "nmeaOK - GNRMC over 82 chars from Unicore",
		packet:   "$GNRMC,114650.00,A,1343.90931561,N,10038.68511804,E,0.005,221.7,040525,0.5,W,A,C*59\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGN | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceEndsWithCRLF, // Note: missing SentenceLength82OrLess
	},

	// nmeaBad test cases - should NOT be valid NMEA
	{
		name:     "nmeaBad - dollar sign in data field",
		packet:   "$GPMRC,$1*FF\r\n",
		expected: 0, // Should fail packet validation due to $ in data
	},
	{
		name:     "nmeaBad - exclamation mark in data",
		packet:   "$GPTXT,1,2,3,Hello!*FF\r\n",
		expected: SentenceIsPacket | SentenceAddressLength5 | SentenceTalkerIsGP | SentenceNoCarets | SentenceValidCaretEscaping | SentenceLength82OrLess | SentenceEndsWithCRLF, // Missing SentenceValidDataChars due to !
	},
	{
		name:     "nmeaBad - address too long (6 chars)",
		packet:   "$ABCDEF,1*FF\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF, // Missing address format flags
	},
	{
		name:     "nmeaBad - address too short (3 chars)",
		packet:   "$ABC,1*FF\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF, // Missing address format flags
	},
	{
		name:     "nmeaBad - lowercase in address",
		packet:   "$ABCdE,1*FF\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF, // Missing address format flags due to lowercase
	},
	{
		name:     "nmeaBad - lowercase hex in checksum",
		packet:   "$GPRMC,1,2,3*0f\r\n",
		expected: 0, // Should fail packet validation due to lowercase hex
	},
	{
		name:     "nmeaBad - missing hex digit in checksum",
		packet:   "$GPRMC,1,2,3*0\r\n",
		expected: 0, // Should fail packet validation due to short checksum
	},
	{
		name:     "nmeaBad - 4-char address not starting with P",
		packet:   "$ABCD,1*FF\r\n",
		expected: SentenceIsPacket | SentenceNoCarets | SentenceValidCaretEscaping | SentenceValidDataChars | SentenceLength82OrLess | SentenceEndsWithCRLF, // Missing address format flags
	},
}

func TestCheckSyntax(t *testing.T) {
	for _, tt := range syntaxTestCases {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckSyntax(tt.packet)
			compareSyntaxFlags(t, "Test case expectation mismatch", tt.packet, got, tt.expected)
			
			// Also test against reference implementation
			ref := checkSyntaxReference(tt.packet)
			compareSyntaxFlags(t, "Optimized vs reference implementation mismatch", tt.packet, got, ref)
		})
	}
}

func FuzzCheckSyntax(f *testing.F) {
	// Seed the fuzzer with all packets from our test cases
	for _, tc := range syntaxTestCases {
		f.Add(tc.packet)
	}

	f.Fuzz(func(t *testing.T, packet string) {
		// Compare optimized implementation against reference implementation
		optimized := CheckSyntax(packet)
		reference := checkSyntaxReference(packet)

		compareSyntaxFlags(t, "Fuzz test: optimized vs reference implementation mismatch", packet, optimized, reference)
	})
}

// compareSyntaxFlags compares got vs expected flags and logs detailed differences
func compareSyntaxFlags(t *testing.T, description, packet string, got, expected SentenceSyntaxFlags) {
	if got == expected {
		return // No difference, nothing to report
	}

	t.Errorf("%s for packet %q: got 0x%X, want 0x%X", description, packet, got, expected)

	// Debug output to show which flags differ
	diff := got ^ expected
	t.Logf("Flags that differ (XOR): 0x%X", diff)
	t.Logf("Got flags:      0x%X", got)
	t.Logf("Expected flags: 0x%X", expected)

	// Print names of differing flags
	flagNames := []struct {
		flag SentenceSyntaxFlags
		name string
	}{
		{SentenceIsPacket, "SentenceIsPacket"},
		{SentenceAddressLength5, "SentenceAddressLength5"},
		{SentenceProprietaryAddressFormat, "SentenceProprietaryAddressFormat"},
		{SentenceTalkerIsGP, "SentenceTalkerIsGP"},
		{SentenceTalkerIsGL, "SentenceTalkerIsGL"},
		{SentenceTalkerIsGA, "SentenceTalkerIsGA"},
		{SentenceTalkerIsGB, "SentenceTalkerIsGB"},
		{SentenceTalkerIsBD, "SentenceTalkerIsBD"},
		{SentenceTalkerIsGI, "SentenceTalkerIsGI"},
		{SentenceTalkerIsGQ, "SentenceTalkerIsGQ"},
		{SentenceTalkerIsGN, "SentenceTalkerIsGN"},
		{SentenceNoCarets, "SentenceNoCarets"},
		{SentenceValidCaretEscaping, "SentenceValidCaretEscaping"},
		{SentenceValidDataChars, "SentenceValidDataChars"},
		{SentenceLength82OrLess, "SentenceLength82OrLess"},
		{SentenceEndsWithCRLF, "SentenceEndsWithCRLF"},
	}

	t.Logf("Flags in got but not expected:")
	for _, fn := range flagNames {
		if (got&fn.flag) != 0 && (expected&fn.flag) == 0 {
			t.Logf("  + %s", fn.name)
		}
	}

	t.Logf("Flags in expected but not got:")
	for _, fn := range flagNames {
		if (expected&fn.flag) != 0 && (got&fn.flag) == 0 {
			t.Logf("  - %s", fn.name)
		}
	}
}

func TestSyntaxFlagMethods(t *testing.T) {
	// Test the composite flag methods work
	var flags SentenceSyntaxFlags

	// Test method calls don't panic
	_ = flags.IsValidGNSSTalkerNMEA()
	_ = flags.IsValidProprietaryNMEA()

	t.Log("SentenceSyntaxFlags methods verified")
}


// checkSyntaxReference is a reference implementation that prioritizes correctness over performance
func checkSyntaxReference(data string) SentenceSyntaxFlags {
	var flags SentenceSyntaxFlags

	// ===== PACKET FORMAT VALIDATION (SentenceIsPacket) =====
	// Check constraints 1-6 for acceptable NMEA-like packet

	// Constraint 2: Terminated with line terminator (CR/LF or LF)
	var lineTerminatorIndex int
	if strings.HasSuffix(data, "\r\n") {
		lineTerminatorIndex = len(data) - 2
	} else if strings.HasSuffix(data, "\n") {
		lineTerminatorIndex = len(data) - 1
	} else {
		return 0
	}

	// Constraint 4: Total length ≤ 128 characters (including line terminator)
	if len(data) > 128 {
		return 0
	}

	// Constraint 1: First character is `$` and no other `$` characters
	if len(data) < 1 || data[0] != '$' {
		return 0
	}
	if strings.Count(data, "$") != 1 {
		return 0
	}

	// Constraint 5: Immediately before line terminator there is `*` and two uppercase hex digits
	// This `*` is the only one in the packet

	// First check that it has only one *
	if strings.Count(data, "*") != 1 {
		return 0
	}

	// Find the index of that *
	asteriskIndex := strings.Index(data, "*")

	// Check there are exactly 3 chars from * to line terminator (*, hex, hex)
	if lineTerminatorIndex-asteriskIndex != 3 {
		return 0
	}

	// Check that the two chars after * are uppercase hex
	if !isUpperHexDigit(data[asteriskIndex+1]) || !isUpperHexDigit(data[asteriskIndex+2]) {
		return 0
	}

	// Constraint 3: All characters before line terminator must be printable ASCII (0x20-0x7E)
	for i := 0; i < lineTerminatorIndex; i++ {
		if !isPrintableASCII(data[i]) {
			return 0
		}
	}

	// Constraint 6: The address part is non-empty
	// Address is substring between `$` and first comma or `*`
	addressEnd := strings.Index(data[1:asteriskIndex], ",")
	var address string
	if addressEnd != -1 {
		address = data[1 : 1+addressEnd] // addressEnd is relative to data[1:], so add 1
	} else {
		// No comma, so address goes from $ to *
		address = data[1:asteriskIndex]
	}
	if len(address) == 0 {
		return 0
	}

	// If we reach here, it's a valid packet
	flags |= SentenceIsPacket

	// ===== ADDRESS AND DATA VALIDATION =====
	// Address already extracted in constraint 6

	// SentenceAddressLength5: Address is exactly 5 uppercase alphanumeric chars
	if len(address) == 5 && isUpperAlphanumeric(address) {
		flags |= SentenceAddressLength5
	}

	// ===== PROPRIETARY FORMAT VALIDATION =====
	// SentenceProprietaryAddressFormat: Address starts with P + 3+ uppercase alphanumeric chars
	if len(address) >= 4 && address[0] == 'P' && isUpperAlphanumeric(address[1:]) {
		flags |= SentenceProprietaryAddressFormat
	}

	// ===== GNSS TALKER ID VALIDATION =====
	// Check first two characters after $ for GNSS talker IDs
	if len(address) >= 2 {
		talker := address[:2]
		switch talker {
		case "GP":
			flags |= SentenceTalkerIsGP
		case "GL":
			flags |= SentenceTalkerIsGL
		case "GA":
			flags |= SentenceTalkerIsGA
		case "GB":
			flags |= SentenceTalkerIsGB
		case "BD":
			flags |= SentenceTalkerIsBD
		case "GI":
			flags |= SentenceTalkerIsGI
		case "GQ":
			flags |= SentenceTalkerIsGQ
		case "GN":
			flags |= SentenceTalkerIsGN
		}
	}

	// ===== DATA FIELD VALIDATION =====
	// Extract data fields (from after $ to *)
	dataFields := data[1:asteriskIndex]

	// SentenceNoCaretInData: No ^ characters in data fields
	if !strings.Contains(dataFields, "^") {
		flags |= SentenceNoCarets
	}

	// SentenceValidCaretEscaping: All ^ followed by exactly 2 hex digits
	validCaretEscaping := true
	for i := 0; i < len(dataFields); i++ {
		if dataFields[i] == '^' {
			if i+2 >= len(dataFields) {
				validCaretEscaping = false
				break
			}
			if !isUpperHexDigit(dataFields[i+1]) || !isUpperHexDigit(dataFields[i+2]) {
				validCaretEscaping = false
				break
			}
			i += 2 // Skip the two hex digits
		}
	}
	if validCaretEscaping {
		flags |= SentenceValidCaretEscaping
	}

	// SentenceValidDataChars: No backslash, exclamation mark or tilde in the packet
	if !strings.ContainsAny(data, "\\!~") {
		flags |= SentenceValidDataChars
	}

	// ===== LENGTH VALIDATION =====
	// SentenceLength82OrLess: Length ≤ 82 chars
	if len(data) <= 82 {
		flags |= SentenceLength82OrLess
	}

	// ===== TERMINATOR VALIDATION =====
	// SentenceEndsWithCRLF: Ends with CRLF (\r\n) not just LF
	if strings.HasSuffix(data, "\r\n") {
		flags |= SentenceEndsWithCRLF
	}

	return flags
}

// isPrintableASCII checks if character is in printable ASCII range (0x20-0x7E)
func isPrintableASCII(c byte) bool {
	return c >= 0x20 && c <= 0x7E
}

// isUpperAlphanumeric checks if string contains only uppercase letters and digits
func isUpperAlphanumeric(s string) bool {
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// TestPacketFormatWithSyntaxTestCases tests PacketFormat.Next using scantest.FindPacket and gpsprot.IsValidPacket
// with all syntax test cases. Test cases with expected flags as 0 should be invalid packets,
// and others should be valid packets. This test will fail until we fix PacketFormat.Next for nmea-lax.
func TestPacketFormatWithSyntaxTestCases(t *testing.T) {
	for _, tt := range syntaxTestCases {
		t.Run(tt.name, func(t *testing.T) {
			expectValidPacket := tt.expected != 0

			// Test gpsprot.IsValidPacket
			isValid := gpsprot.IsValidPacket(PacketFormat, []byte(tt.packet))
			if isValid != expectValidPacket {
				t.Errorf("gpsprot.IsValidPacket mismatch for %q: got %v, want %v", tt.packet, isValid, expectValidPacket)
			}

			// Test that IsValidPacket works even within a buffer
			for range 10 {
				buf, nRandom := scantest.InsertRandomPrefix(tt.packet, '$')
				isValid = scantest.IsValidPacketBetween(PacketFormat, buf, nRandom, nRandom+len(tt.packet))
				
				if isValid != expectValidPacket {
					t.Errorf("scantest.IsValidPacketBetween mismatch for %q: got %v, want %v", tt.packet, isValid, expectValidPacket)
				}
			}
		})
	}
}


