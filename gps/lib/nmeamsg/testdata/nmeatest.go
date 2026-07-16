package testdata

import "strings"

// These have to match nmeamsg.go
const (
	sentenceIsPacket sentenceSyntaxFlags = 1 << iota
	sentenceAddressLength5
	sentenceProprietaryAddressFormat
	sentenceTalkerIsGP         // GPS
	sentenceTalkerIsGL         // GLONASS
	sentenceTalkerIsGA         // Galileo
	sentenceTalkerIsGB         // BeiDou (current)
	sentenceTalkerIsBD         // BeiDou (legacy)
	sentenceTalkerIsGI         // NavIC
	sentenceTalkerIsGQ         // QZSS
	sentenceTalkerIsGN         // Multi-GNSS
	sentenceNoCarets           // No ^ characters
	sentenceValidCaretEscaping // All ^ followed by exactly 2 hex digits (true even if no ^)
	sentenceValidDataChars     // No occurrences of backslash, exclamation mark or tilde in the packet.
	sentenceLength82OrLess     // Length ≤ 82 chars
	sentenceEndsWithCRLF       // Ends with CRLF (\r\n) (not just LF)
)

type sentenceSyntaxFlags uint32

var SyntaxTestCases = []struct {
	Name     string
	Packet   string
	Expected sentenceSyntaxFlags
}{
	// NOT A PACKET - Short packet tests (length < 6)
	{
		Name:     "not a packet - empty string",
		Packet:   "",
		Expected: 0,
	},
	{
		Name:     "not a packet - single character",
		Packet:   "$",
		Expected: 0,
	},
	{
		Name:     "not a packet - two characters",
		Packet:   "$\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - three characters",
		Packet:   "$*\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - four characters",
		Packet:   "$*A\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - five characters",
		Packet:   "$*AB\n",
		Expected: 0,
	},

	// NOT A PACKET - Basic packet format constraint failures (testing each of the 6 constraints)

	// Constraint 2: Terminated with a line terminator (CR/LF or LF)
	{
		Name:     "not a packet - no line terminator (constraint 2)",
		Packet:   "$GPGGA,123*5A",
		Expected: 0,
	},

	// Constraint 3: All characters before terminator must be printable ASCII (0x20-0x7E)
	{
		Name:     "not a packet - non-printable char (constraint 3)",
		Packet:   "$GPGGA,\x01*5A\r\n",
		Expected: 0,
	},

	// Constraint 4: Total length ≤ 400 characters
	{
		Name:     "not a packet - 401 chars (constraint 4)",
		Packet:   "$GPGGA," + strings.Repeat("X", 389) + "*5A\r\n", // Total = 401
		Expected: 0,
	},

	// Constraint 1: First character is `$` and no other `$` characters
	{
		Name:     "not a packet - missing dollar (constraint 1a)",
		Packet:   "GPGGA,123*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - extra dollar (constraint 1b)",
		Packet:   "$GPGGA,1$23*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - SBF sync prefix",
		Packet:   "$@AB*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - Septentrio reply prefix",
		Packet:   "$R*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - Septentrio command prefix",
		Packet:   "$----*5A\r\n",
		Expected: 0,
	},

	// Constraint 5: Immediately before terminator there is `*` and two uppercase hex digits
	{
		Name:     "not a packet - missing asterisk (constraint 5a)",
		Packet:   "$GPGGA,123\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - missing checksum (constraint 5b)",
		Packet:   "$GPGGA,123*\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - short checksum (constraint 5c)",
		Packet:   "$GPGGA,123*5\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - non-hex checksum (constraint 5d)",
		Packet:   "$GPGGA,123*ZZ\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - lowercase checksum (constraint 5e)",
		Packet:   "$GPGGA,123*5a\r\n",
		Expected: 0,
	},

	// Constraint 5: This `*` is the only one in the packet
	{
		Name:     "not a packet - extra asterisk (constraint 5)",
		Packet:   "$GPGGA,1*23*5A\r\n",
		Expected: 0,
	},

	// Additional constraint violations
	{
		Name:     "not a packet - mixed case checksum - aB (constraint 5e)",
		Packet:   "$GPGGA,123*aB\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - tab character (constraint 3)",
		Packet:   "$GPGGA,hello\tworld*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - DEL character (constraint 3)",
		Packet:   "$GPGGA,hello\x7Fworld*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - null character (constraint 3)",
		Packet:   "$GPGGA,hello\x00world*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - empty address with comma (constraint 6)",
		Packet:   "$,*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not a packet - CR only no LF (constraint 2)",
		Packet:   "$GPGGA,123*5A\r",
		Expected: 0,
	},
	{
		Name:     "not a packet - extra data after CRLF (constraint 2)",
		Packet:   "$GPGGA,123*5A\r\nEXTRA",
		Expected: 0,
	},
	{
		Name:     "not a packet - checksum with spaces (constraint 5)",
		Packet:   "$GPGGA,123*5 A\r\n",
		Expected: 0,
	},

	// VALID PACKETS - Basic packet format tests
	{
		Name:     "valid basic packet",
		Packet:   "$GPGGA,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "valid Unicore OK packet",
		Packet:   "$OK*5A\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Address format tests - approved (exactly 5 chars)
	{
		Name:     "approved address - GPGGA",
		Packet:   "$GPGGA,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "approved address - GLRMC",
		Packet:   "$GLRMC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGL | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "approved address - GARMC",
		Packet:   "$GARMC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGA | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "approved address - GBRMC",
		Packet:   "$GBRMC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGB | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "approved address - BDRMC (legacy BeiDou)",
		Packet:   "$BDRMC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsBD | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "approved address - GIRMC",
		Packet:   "$GIRMC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGI | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "approved address - GQRMC",
		Packet:   "$GQRMC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGQ | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "approved address - GNRMC (Multi-GNSS)",
		Packet:   "$GNRMC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGN | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "approved address - unknown talker",
		Packet:   "$XXTXT,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "not approved address - too short",
		Packet:   "$ABC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "not approved address - too long",
		Packet:   "$ABCDEF,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "not approved address - lowercase",
		Packet:   "$GPgga,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "not approved address - non-alphanumeric",
		Packet:   "$GP-GA,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Proprietary address format tests
	{
		Name:     "proprietary address - PUBX (4 chars total)",
		Packet:   "$PUBX,41,1*25\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "proprietary address - PUBX1 (5 chars, overlapping flags)",
		Packet:   "$PUBX1,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "proprietary address - PMTK (3 chars after P)",
		Packet:   "$PMTK,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "proprietary address - PMTK123 (extended, more than 3 chars)",
		Packet:   "$PMTK123,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "not proprietary - starts with P but too short",
		Packet:   "$PA,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "proprietary address - lowercase manufacturer code",
		Packet:   "$Pabc,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "not a packet - non-letter after P",
		Packet:   "$P-BC,123*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "not proprietary - digit in manufacturer code",
		Packet:   "$PA1C,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "proprietary address - unrestricted suffix",
		Packet:   "$PABC-1,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "proprietary address - no data fields",
		Packet:   "$PUBX*5A\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "proprietary address - single empty data field",
		Packet:   "$PUBX,*5A\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Caret escaping tests
	{
		Name:     "no carets - flags should be set",
		Packet:   "$GPGGA,123,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "valid caret escaping",
		Packet:   "$GPGGA,12^0D3*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "valid caret escaping - multiple",
		Packet:   "$GPGGA,^0A^0D^2A*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "invalid caret escaping - missing hex",
		Packet:   "$GPGGA,12^3*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "invalid caret escaping - non-hex",
		Packet:   "$GPGGA,12^XY*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "invalid caret escaping - lowercase hex",
		Packet:   "$GPGGA,12^0a*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Invalid data character tests
	{
		Name:     "invalid data char - backslash",
		Packet:   "$GPGGA,12\\3*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "invalid data char - exclamation",
		Packet:   "$GPGGA,12!3*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "invalid data char - tilde",
		Packet:   "$GPGGA,12~3*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Length tests
	{
		Name:     "exactly 82 chars",
		Packet:   "$GPGGA,1234567890123456789012345678901234567890123456789012345678901234567890*5A\r\n", // 82 chars total
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "over 82 chars",
		Packet:   "$GPGGA,12345678901234567890123456789012345678901234567890123456789012345678901*5A\r\n", // 83 chars total
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceEndsWithCRLF,
	},

	// Line ending tests
	{
		Name:     "CRLF ending",
		Packet:   "$GPGGA,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "LF only ending",
		Packet:   "$GPGGA,123*5A\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess,
	},

	// Real NMEA sentences from existing tests
	{
		Name:     "real GPGGA",
		Packet:   "$GPGGA,092725.00,4717.11399,N,00833.91590,E,1,08,1.01,499.6,M,48.0,M,,*5B\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "real GPGLL",
		Packet:   "$GPGLL,4717.11364,N,00833.91565,E,092321.00,A,A*60\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "real PUBX proprietary",
		Packet:   "$PUBX,41,1,0007,0003,19200,0*25\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "real GPTXT",
		Packet:   "$GPTXT,01,01,02,u-blox ag - www.u-blox.com*50\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "real GPVTG",
		Packet:   "$GPVTG,77.52,T,,M,0.004,N,0.008,K,A*06\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "real GPZDA",
		Packet:   "$GPZDA,082710.00,16,09,2002,00,00*64\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "real GNRMC (over 82 chars)",
		Packet:   "$GNRMC,114650.00,A,1343.90931561,N,10038.68511804,E,0.005,221.7,040525,0.5,W,A,C*59\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGN | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceEndsWithCRLF,
	},

	// Unicore command acknowledgment (non-NMEA but packet-like)
	{
		Name:     "Unicore command ack - FRESET",
		Packet:   "$FRESET,response: OK*2E\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "Unicore command ack - CONFIG",
		Packet:   "$CONFIG,OK*1A\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Edge cases
	{
		Name:     "no data fields - address only",
		Packet:   "$GPGGA*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "single empty data field",
		Packet:   "$GPGGA,*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "multiple empty data fields",
		Packet:   "$GPGGA,,,*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "minimal packet - single char address",
		Packet:   "$A*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "minimal packet with empty field",
		Packet:   "$AA,*5A\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Packet length boundary tests (400 char limit including CRLF)
	{
		Name:     "exactly 400 chars with CRLF",
		Packet:   "$GPGGA," + strings.Repeat("X", 388) + "*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceEndsWithCRLF,
	},
	{
		Name:     "exactly 399 chars with LF",
		Packet:   "$GPGGA," + strings.Repeat("X", 388) + "*5A\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars,
	},
	{
		Name:     "401 chars with CRLF",
		Packet:   "$GPGGA," + strings.Repeat("X", 389) + "*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "400 chars with LF",
		Packet:   "$GPGGA," + strings.Repeat("X", 389) + "*5A\n",
		Expected: 0,
	},

	// Checksum case variations
	{
		Name:     "lowercase checksum",
		Packet:   "$GPGGA,123*5a\r\n",
		Expected: 0, // Should fail - spec requires uppercase hex
	},
	{
		Name:     "mixed case checksum",
		Packet:   "$GPGGA,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "mixed case checksum - aB",
		Packet:   "$GPGGA,123*aB\r\n",
		Expected: 0, // Should fail - spec requires uppercase hex
	},

	// Address character validation - digits
	{
		Name:     "address with digits - 12345",
		Packet:   "$12345,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "address with mixed alphanumeric - A1B2C",
		Packet:   "$A1B2C,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "address starting with digit - 1ABCD",
		Packet:   "$1ABCD,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "address with digits after two letters - AB1C2",
		Packet:   "$AB1C2,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Caret escaping edge cases
	{
		Name:     "caret at end of field",
		Packet:   "$GPGGA,data^*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "caret with one char at end",
		Packet:   "$GPGGA,data^5*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "sequential escaped carets",
		Packet:   "$GPGGA,^^5E5E*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Reserved character tests (dollar and asterisk in data)
	{
		Name:     "dollar sign in data field",
		Packet:   "$GPGGA,$123*5A\r\n",
		Expected: 0, // Should fail - $ only allowed at start
	},
	{
		Name:     "asterisk in data field",
		Packet:   "$GPGGA,12*34*5A\r\n",
		Expected: 0, // Should fail - only one * allowed before checksum
	},

	// ASCII boundary tests
	{
		Name:     "space character (0x20) in data",
		Packet:   "$GPGGA,hello world*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "tab character (0x09) in data",
		Packet:   "$GPGGA,hello\tworld*5A\r\n",
		Expected: 0, // Should fail - tab is not printable ASCII
	},
	{
		Name:     "DEL character (0x7F) in data",
		Packet:   "$GPGGA,hello\x7Fworld*5A\r\n",
		Expected: 0, // Should fail - DEL is not in printable range
	},
	{
		Name:     "null character (0x00) in data",
		Packet:   "$GPGGA,hello\x00world*5A\r\n",
		Expected: 0, // Should fail - null is not printable
	},

	// Proprietary edge cases
	{
		Name:     "proprietary minimum valid - P + 3 chars",
		Packet:   "$PABC,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "not proprietary - digit after P",
		Packet:   "$P123,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "very long proprietary address",
		Packet:   "$PABCDEFGHIJKLMNOP,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Empty address tests
	{
		Name:     "no address - just checksum (constraint 6)",
		Packet:   "$*5A\r\n",
		Expected: 0, // Should fail - address cannot be empty
	},
	{
		Name:     "empty address with comma (constraint 6)",
		Packet:   "$,*5A\r\n",
		Expected: 0, // Should fail - address cannot be empty
	},

	// Line terminator variations
	{
		Name:     "no line terminator",
		Packet:   "$GPGGA,123*5A",
		Expected: 0, // Should fail - must have terminator
	},
	{
		Name:     "CR only (no LF)",
		Packet:   "$GPGGA,123*5A\r",
		Expected: 0, // Should fail - incomplete terminator
	},
	{
		Name:     "extra data after CRLF",
		Packet:   "$GPGGA,123*5A\r\nEXTRA",
		Expected: 0, // Should fail - data after terminator
	},

	// GNSS talker edge cases
	{
		Name:     "unknown G-prefix talker - GX",
		Packet:   "$GXRMC,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "partial GNSS match - just G",
		Packet:   "$G,123*5A\r\n",
		Expected: 0,
	},
	{
		Name:     "partial GNSS match - just GP",
		Packet:   "$GP,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "partial GNSS match - GPS (3 chars)",
		Packet:   "$GPS,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},

	// Mixed validation scenarios
	{
		Name:     "not proprietary - P1234",
		Packet:   "$P1234,data*5A\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "address with spaces (valid packet, invalid address format)",
		Packet:   "$GP GA,123*5A\r\n",
		Expected: sentenceIsPacket | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF, // Valid packet but fails address format validation
	},

	// Test cases migrated from nmeapacket_test.go

	// nmeaOK test cases - should be valid GNSS talker or proprietary NMEA
	{
		Name:     "nmeaOK - GPGGA real sentence",
		Packet:   "$GPGGA,092725.00,4717.11399,N,00833.91590,E,1,08,1.01,499.6,M,48.0,M,,*5B\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "nmeaOK - GPGLL real sentence",
		Packet:   "$GPGLL,4717.11364,N,00833.91565,E,092321.00,A,A*60\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "nmeaOK - PUBX proprietary",
		Packet:   "$PUBX,41,1,0007,0003,19200,0*25\r\n",
		Expected: sentenceIsPacket | sentenceProprietaryAddressFormat | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "nmeaOK - GPTXT with spaces and hyphens",
		Packet:   "$GPTXT,01,01,02,u-blox ag - www.u-blox.com*50\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "nmeaOK - GPVTG real sentence",
		Packet:   "$GPVTG,77.52,T,,M,0.004,N,0.008,K,A*06\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "nmeaOK - GPZDA real sentence",
		Packet:   "$GPZDA,082710.00,16,09,2002,00,00*64\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF,
	},
	{
		Name:     "nmeaOK - GNRMC over 82 chars from Unicore",
		Packet:   "$GNRMC,114650.00,A,1343.90931561,N,10038.68511804,E,0.005,221.7,040525,0.5,W,A,C*59\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGN | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceEndsWithCRLF, // Note: missing sentenceLength82OrLess
	},

	// nmeaBad test cases - should NOT be valid NMEA
	{
		Name:     "nmeaBad - dollar sign in data field",
		Packet:   "$GPMRC,$1*FF\r\n",
		Expected: 0, // Should fail packet validation due to $ in data
	},
	{
		Name:     "nmeaBad - exclamation mark in data",
		Packet:   "$GPTXT,1,2,3,Hello!*FF\r\n",
		Expected: sentenceIsPacket | sentenceAddressLength5 | sentenceTalkerIsGP | sentenceNoCarets | sentenceValidCaretEscaping | sentenceLength82OrLess | sentenceEndsWithCRLF, // Missing sentenceValidDataChars due to !
	},
	{
		Name:     "nmeaBad - address too long (6 chars)",
		Packet:   "$ABCDEF,1*FF\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF, // Missing address format flags
	},
	{
		Name:     "nmeaBad - address too short (3 chars)",
		Packet:   "$ABC,1*FF\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF, // Missing address format flags
	},
	{
		Name:     "nmeaBad - lowercase in address",
		Packet:   "$ABCdE,1*FF\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF, // Missing address format flags due to lowercase
	},
	{
		Name:     "nmeaBad - lowercase hex in checksum",
		Packet:   "$GPRMC,1,2,3*0f\r\n",
		Expected: 0, // Should fail packet validation due to lowercase hex
	},
	{
		Name:     "nmeaBad - missing hex digit in checksum",
		Packet:   "$GPRMC,1,2,3*0\r\n",
		Expected: 0, // Should fail packet validation due to short checksum
	},
	{
		Name:     "nmeaBad - 4-char address not starting with P",
		Packet:   "$ABCD,1*FF\r\n",
		Expected: sentenceIsPacket | sentenceNoCarets | sentenceValidCaretEscaping | sentenceValidDataChars | sentenceLength82OrLess | sentenceEndsWithCRLF, // Missing address format flags
	},
}
