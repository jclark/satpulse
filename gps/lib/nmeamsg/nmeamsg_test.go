package nmeamsg

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/lib/nmeamsg/testdata"
)

func TestCheckSyntax(t *testing.T) {
	for _, tt := range testdata.SyntaxTestCases {
		t.Run(tt.Name, func(t *testing.T) {
			got := CheckSyntax(tt.Packet)
			compareSyntaxFlags(t, "Test case expectation mismatch", tt.Packet, got, SentenceSyntaxFlags(tt.Expected))

			// Also test against reference implementation
			ref := checkSyntaxReference(tt.Packet)
			compareSyntaxFlags(t, "Optimized vs reference implementation mismatch", tt.Packet, got, ref)
		})
	}
}

func FuzzCheckSyntax(f *testing.F) {
	// Seed the fuzzer with all packets from our test cases
	for _, tc := range testdata.SyntaxTestCases {
		f.Add(tc.Packet)
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

	// Constraint 4: Total length ≤ SentenceMaxLength characters (including line terminator)
	if len(data) > SentenceMaxLength {
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
