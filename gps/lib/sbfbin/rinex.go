package sbfbin

// RINEXSys returns the RINEX satellite system letter for an SBF SVID.
// It returns "" for Do-Not-Use, reserved, non-GNSS, and unrecognised values.
func RINEXSys(svid uint8) string {
	if svid >= 1 && svid <= 37 || svid >= 250 && svid <= 251 {
		return "G"
	}
	if svid >= 38 && svid <= 61 || svid >= 63 && svid <= 68 {
		return "R"
	}
	if svid >= 71 && svid <= 106 {
		return "E"
	}
	if svid >= 120 && svid <= 140 || svid >= 198 && svid <= 215 {
		return "S"
	}
	if svid >= 141 && svid <= 180 || svid >= 223 && svid <= 245 {
		return "C"
	}
	if svid >= 181 && svid <= 190 {
		return "J"
	}
	if svid >= 191 && svid <= 197 || svid >= 216 && svid <= 222 {
		return "I"
	}
	return ""
}

// RINEXSatNum converts an SBF SVID to a RINEX satellite number.
// It returns 0 for Do-Not-Use, reserved, non-GNSS, and unrecognised values.
func RINEXSatNum(svid uint8) uint8 {
	if svid >= 1 && svid <= 37 {
		return svid
	}
	if svid >= 38 && svid <= 61 {
		return svid - 37
	}
	if svid >= 63 && svid <= 68 {
		return svid - 38
	}
	if svid >= 71 && svid <= 106 {
		return svid - 70
	}
	if svid >= 120 && svid <= 140 {
		return svid - 100
	}
	if svid >= 141 && svid <= 180 {
		return svid - 140
	}
	if svid >= 181 && svid <= 190 {
		return svid - 180
	}
	if svid >= 191 && svid <= 197 {
		return svid - 190
	}
	if svid >= 198 && svid <= 215 {
		return svid - 157
	}
	if svid >= 216 && svid <= 222 {
		return svid - 208
	}
	if svid >= 223 && svid <= 245 {
		return svid - 182
	}
	if svid >= 250 && svid <= 251 {
		return svid - 212
	}
	return 0
}

// RINEXSig returns the RINEX system letter and two-character signal identifier
// for an SBF signal number. It returns "", "" if the signal is reserved or not
// representable.
// The mapping follows mosaic-G5 4.1.10 and RINEX 4.02 Tables 10-16.
func RINEXSig(sig uint8, flags CommonFlags) (string, string) {
	if sig == 19 && flags.E6BUsed() {
		return "E", "6B"
	}
	i := int(sig) * 3
	if i+3 > len(rinexSigMap) {
		return "", ""
	}
	s := rinexSigMap[i : i+3]
	if s == "   " {
		return "", ""
	}
	return s[:1], s[1:]
}

// RINEXChannelStatusSignals returns the RINEX signal identifiers represented
// by a ChannelStatus PVTStatus slot for a RINEX system letter.
func RINEXChannelStatusSignals(sys string, slot int) []string {
	if slot < 0 || slot >= 8 {
		return nil
	}
	sigs, ok := rinexChannelStatusMap[sys]
	if !ok {
		return nil
	}
	return sigs[slot]
}

// rinexSigMap maps SBF signal numbers to fixed-width RINEX system+signal entries.
// Each line holds five 3-byte entries; "   " means reserved or unmapped.
// QZSS L6 uses the RINEX L6D code, filling the blank cell in the SBF table.
const rinexSigMap = "" +
	// 0
	"G1CG1WG2WG2LG5Q" +
	// 5
	"G1LJ1CJ2LR1CR1P" +
	// 10
	"R2PR2CR3QC1PC5P" +
	// 15
	"I5A   E1C   E6C" +
	// 20
	"E5QE7QE8Q   S1C" +
	// 25
	"S5IJ5QJ6SC2IC7I" +
	// 30
	"C6I   J1LJ1ZC7D" +
	// 35
	"      I1PJ1EJ5P"

var rinexChannelStatusMap = map[string][8][]string{
	"G": {
		{"1C"},
		{"1W"},
		{"2W"},
		{"2L"},
		{"5Q"},
		{"1L"},
	},
	"R": {
		{"1C"},
		{"1P"},
		{"2P"},
		{"2C"},
		{"3Q"},
	},
	"E": {
		nil,
		{"1C"},
		nil,
		{"6B", "6C"},
		{"5Q"},
		{"7Q"},
		{"8Q"},
	},
	"S": {
		{"1C"},
		{"5I"},
	},
	"C": {
		{"2I"},
		{"7I"},
		{"6I"},
		{"1P"},
		{"5P"},
		{"7D"},
	},
	"J": {
		{"1C"},
		{"2L"},
		{"5Q"},
		{"6S"},
		{"1L"},
		{"1Z"},
		{"1E"},
		{"5P"},
	},
	"I": {
		{"5A"},
		{"1P"},
	},
}
