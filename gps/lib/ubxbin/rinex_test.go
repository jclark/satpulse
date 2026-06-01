package ubxbin

import (
	"testing"
)

func TestRINEXSys(t *testing.T) {
	tests := []struct {
		name   string
		gnssId GNSSID
		expect string
	}{
		{"GPS", GPS, "G"},
		{"SBAS", SBAS, "S"},
		{"GAL", GAL, "E"},
		{"BDS", BDS, "C"},
		{"IMES", IMES, ""},
		{"QZSS", QZSS, "J"},
		{"GLO", GLO, "R"},
		{"NavIC", NavIC, "I"},
		{"unknown", GNSSID(99), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RINEXSys(tc.gnssId); got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestRINEXSatNum(t *testing.T) {
	tests := []struct {
		name   string
		gnssId GNSSID
		svId   byte
		expect uint8
	}{
		{"GPS 7", GPS, 7, 7},
		{"GAL 11", GAL, 11, 11},
		{"QZSS 4", QZSS, 4, 4},
		{"BDS 16", BDS, 16, 16},
		{"NavIC 3", NavIC, 3, 3},
		{"SBAS PRN 120", SBAS, 120, 20},
		{"SBAS PRN 158", SBAS, 158, 58},
		{"SBAS below 120 passes through", SBAS, 119, 119},
		{"GLO slot 3", GLO, 3, 3},
		{"GLO unknown 255", GLO, 255, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RINEXSatNum(tc.gnssId, tc.svId); got != tc.expect {
				t.Errorf("got %d, want %d", got, tc.expect)
			}
		})
	}
}

func TestRINEXSig(t *testing.T) {
	tests := []struct {
		name   string
		gnssId GNSSID
		sigId  byte
		expect string
	}{
		{"GPS L1 C/A", GPS, 0, "1C"},
		{"GPS L2 CL", GPS, 3, "2L"},
		{"GPS L2 CM", GPS, 4, "2S"},
		{"GPS L5 I", GPS, 6, "5I"},
		{"GPS L5 Q", GPS, 7, "5Q"},
		{"SBAS L1 C/A", SBAS, 0, "1C"},
		{"GAL E1 C", GAL, 0, "1C"},
		{"GAL E1 B", GAL, 1, "1B"},
		{"GAL E5aI", GAL, 3, "5I"},
		{"GAL E5aQ", GAL, 4, "5Q"},
		{"GAL E5bI", GAL, 5, "7I"},
		{"GAL E5bQ", GAL, 6, "7Q"},
		{"GAL E6 B", GAL, 8, "6B"},
		{"GAL E6 C", GAL, 9, "6C"},
		{"GAL E6 A", GAL, 10, "6A"},
		{"BDS B1I D1", BDS, 0, "2I"},
		{"BDS B1I D2", BDS, 1, "2I"},
		{"BDS B2I D1", BDS, 2, "7I"},
		{"BDS B2I D2", BDS, 3, "7I"},
		{"BDS B3I D1", BDS, 4, "6I"},
		{"BDS B3I D2", BDS, 10, "6I"},
		{"BDS B1Cp", BDS, 5, "1P"},
		{"BDS B1Cd", BDS, 6, "1D"},
		{"BDS B2ap", BDS, 7, "5P"},
		{"BDS B2ad", BDS, 8, "5D"},
		{"QZSS L1 C/A", QZSS, 0, "1C"},
		{"QZSS L1S", QZSS, 1, "1Z"},
		{"QZSS L2 CM", QZSS, 4, "2S"},
		{"QZSS L2 CL", QZSS, 5, "2L"},
		{"QZSS L5 I", QZSS, 8, "5I"},
		{"QZSS L5 Q", QZSS, 9, "5Q"},
		{"QZSS L1C/B", QZSS, 12, "1E"},
		{"GLO L1 OF", GLO, 0, "1C"},
		{"GLO L2 OF", GLO, 2, "2C"},
		{"NavIC L5 A", NavIC, 0, "5A"},
		{"unknown GPS sigId", GPS, 99, ""},
		{"IMES has no signals", IMES, 0, ""},
		{"unknown GNSS", GNSSID(99), 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RINEXSig(tc.gnssId, tc.sigId); got != tc.expect {
				t.Errorf("got %q, want %q", got, tc.expect)
			}
		})
	}
}
