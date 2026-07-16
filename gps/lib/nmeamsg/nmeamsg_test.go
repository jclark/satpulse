package nmeamsg

import (
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/lib/ascii"
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
		{SentenceApprovedAddressFormat, "SentenceApprovedAddressFormat"},
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

func TestClassificationMethods(t *testing.T) {
	cases := []struct {
		packet                    string
		approved, gnss, propriety bool
	}{
		{"$GPGGA,123*5A\r\n", true, true, false},
		{"$XXTXT,123*5A\r\n", true, false, false},   // approved, unregistered talker
		{"$PUBX,41,1*25\r\n", false, false, true},   // proprietary
		{"$PUBX1,data*5A\r\n", false, false, true},  // 5-char P address: proprietary, not approved
		{"$P1234,data*5A\r\n", false, false, false}, // P + digits: neither approved nor proprietary
	}
	for _, tt := range cases {
		f := CheckSyntax(tt.packet)
		if got := f.IsValidApprovedNMEA(); got != tt.approved {
			t.Errorf("IsValidApprovedNMEA(%q) = %v, want %v", tt.packet, got, tt.approved)
		}
		if got := f.IsValidGNSSTalkerNMEA(); got != tt.gnss {
			t.Errorf("IsValidGNSSTalkerNMEA(%q) = %v, want %v", tt.packet, got, tt.gnss)
		}
		if got := f.IsValidProprietaryNMEA(); got != tt.propriety {
			t.Errorf("IsValidProprietaryNMEA(%q) = %v, want %v", tt.packet, got, tt.propriety)
		}
	}
}

// checkSyntaxReference is a reference implementation that prioritizes correctness over performance
func checkSyntaxReference(data string) SentenceSyntaxFlags {
	var flags SentenceSyntaxFlags

	// ===== PACKET FORMAT VALIDATION (SentenceIsPacket) =====
	// Check constraints 1-6 for acceptable NMEA-like packet

	// Constraint 2: Terminated with line terminator (CR/LF or LF)
	var lineTerminatorIndex int
	var endsWithCRLF bool
	if strings.HasSuffix(data, "\r\n") {
		lineTerminatorIndex = len(data) - 2
		endsWithCRLF = true
	} else if strings.HasSuffix(data, "\n") {
		lineTerminatorIndex = len(data) - 1
	} else {
		return 0
	}

	// Constraint 4: Total length including a canonical CRLF is at most SentenceMaxLength characters
	if len(data) > SentenceMaxLength || (!endsWithCRLF && len(data) == SentenceMaxLength) {
		return 0
	}

	// Constraint 1: First character is `$` and no other `$` characters
	if len(data) < 3 || data[0] != '$' {
		return 0
	}
	if strings.Count(data, "$") != 1 {
		return 0
	}
	if !ascii.IsAlnum(data[1]) || !ascii.IsAlnum(data[2]) {
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
	if !ascii.IsUpperHexDigit(data[asteriskIndex+1]) || !ascii.IsUpperHexDigit(data[asteriskIndex+2]) {
		return 0
	}

	// Constraint 3: All characters before line terminator must be printable ASCII (0x20-0x7E)
	for i := 0; i < lineTerminatorIndex; i++ {
		if !ascii.IsPrint(data[i]) {
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

	// SentenceApprovedAddressFormat: Address is exactly 5 uppercase alphanumeric chars, not starting with P
	if len(address) == 5 && isUpperAlphanumeric(address) && address[0] != 'P' {
		flags |= SentenceApprovedAddressFormat
	}

	// ===== PROPRIETARY FORMAT VALIDATION =====
	// SentenceProprietaryAddressFormat: Address starts with P + 3 ASCII letters
	if len(address) >= 4 && address[0] == 'P' &&
		ascii.IsLetter(address[1]) && ascii.IsLetter(address[2]) && ascii.IsLetter(address[3]) {
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
			if !ascii.IsUpperHexDigit(dataFields[i+1]) || !ascii.IsUpperHexDigit(dataFields[i+2]) {
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

// isUpperAlphanumeric checks if string contains only uppercase letters and digits
func isUpperAlphanumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		if !ascii.IsUpper(s[i]) && !ascii.IsDigit(s[i]) {
			return false
		}
	}
	return true
}
