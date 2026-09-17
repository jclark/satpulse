package sbfbin

import (
	"slices"
	"testing"
)

func TestRINEXSys(t *testing.T) {
	tests := []struct {
		name string
		svid uint8
		want string
	}{
		{"DNU", 0, ""},
		{"GPS first", 1, "G"},
		{"GPS last first range", 37, "G"},
		{"GLONASS first", 38, "R"},
		{"GLONASS last first range", 61, "R"},
		{"GLONASS unknown slot", 62, ""},
		{"GLONASS first second range", 63, "R"},
		{"GLONASS last second range", 68, "R"},
		{"Galileo before range", 70, ""},
		{"Galileo first", 71, "E"},
		{"Galileo last", 106, "E"},
		{"L-band", 107, ""},
		{"SBAS first first range", 120, "S"},
		{"SBAS last first range", 140, "S"},
		{"BeiDou first first range", 141, "C"},
		{"BeiDou last first range", 180, "C"},
		{"QZSS first", 181, "J"},
		{"QZSS last", 190, "J"},
		{"NavIC first first range", 191, "I"},
		{"NavIC last first range", 197, "I"},
		{"SBAS first second range", 198, "S"},
		{"SBAS last second range", 215, "S"},
		{"NavIC first second range", 216, "I"},
		{"NavIC last second range", 222, "I"},
		{"BeiDou first second range", 223, "C"},
		{"BeiDou last second range", 245, "C"},
		{"reserved gap", 249, ""},
		{"GPS first second range", 250, "G"},
		{"GPS last second range", 251, "G"},
		{"undefined", 252, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXSys(tt.svid); got != tt.want {
				t.Errorf("RINEXSys(%d) = %q, want %q", tt.svid, got, tt.want)
			}
		})
	}
}

func TestRINEXSatNum(t *testing.T) {
	tests := []struct {
		name string
		svid uint8
		want uint8
	}{
		{"DNU", 0, 0},
		{"GPS first", 1, 1},
		{"GPS last first range", 37, 37},
		{"GLONASS first", 38, 1},
		{"GLONASS last first range", 61, 24},
		{"GLONASS unknown slot", 62, 0},
		{"GLONASS first second range", 63, 25},
		{"GLONASS last second range", 68, 30},
		{"Galileo first", 71, 1},
		{"Galileo last", 106, 36},
		{"L-band", 119, 0},
		{"SBAS first first range", 120, 20},
		{"SBAS last first range", 140, 40},
		{"BeiDou first first range", 141, 1},
		{"BeiDou last first range", 180, 40},
		{"QZSS first", 181, 1},
		{"QZSS last", 190, 10},
		{"NavIC first first range", 191, 1},
		{"NavIC last first range", 197, 7},
		{"SBAS first second range", 198, 41},
		{"SBAS last second range", 215, 58},
		{"NavIC first second range", 216, 8},
		{"NavIC last second range", 222, 14},
		{"BeiDou first second range", 223, 41},
		{"BeiDou last second range", 245, 63},
		{"reserved gap", 249, 0},
		{"GPS first second range", 250, 38},
		{"GPS last second range", 251, 39},
		{"undefined", 252, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXSatNum(tt.svid); got != tt.want {
				t.Errorf("RINEXSatNum(%d) = %d, want %d", tt.svid, got, tt.want)
			}
		})
	}
}

func TestRINEXSig(t *testing.T) {
	tests := []struct {
		name    string
		sig     uint8
		flags   CommonFlags
		wantSys string
		wantSig string
	}{
		{"GPS L1CA", 0, 0, "G", "1C"},
		{"GPS L1P", 1, 0, "G", "1W"},
		{"GPS L2P", 2, 0, "G", "2W"},
		{"GPS L2C", 3, 0, "G", "2L"},
		{"GPS L5", 4, 0, "G", "5Q"},
		{"GPS L1C", 5, 0, "G", "1L"},
		{"QZSS L1CA", 6, 0, "J", "1C"},
		{"QZSS L2C", 7, 0, "J", "2L"},
		{"GLONASS L1CA", 8, 0, "R", "1C"},
		{"GLONASS L1P", 9, 0, "R", "1P"},
		{"GLONASS L2P", 10, 0, "R", "2P"},
		{"GLONASS L2CA", 11, 0, "R", "2C"},
		{"GLONASS L3", 12, 0, "R", "3Q"},
		{"BeiDou B1C", 13, 0, "C", "1P"},
		{"BeiDou B2a", 14, 0, "C", "5P"},
		{"NavIC L5", 15, 0, "I", "5A"},
		{"reserved 16", 16, 0, "", ""},
		{"Galileo E1", 17, 0, "E", "1C"},
		{"reserved 18", 18, 0, "", ""},
		{"Galileo E6 default", 19, 0, "E", "6C"},
		{"Galileo E6 E6B", 19, CommonFlagsE6BUsed, "E", "6B"},
		{"Galileo E5a", 20, 0, "E", "5Q"},
		{"Galileo E5b", 21, 0, "E", "7Q"},
		{"Galileo E5AltBOC", 22, 0, "E", "8Q"},
		{"MSS L-band", 23, 0, "", ""},
		{"SBAS L1CA", 24, 0, "S", "1C"},
		{"SBAS L5", 25, 0, "S", "5I"},
		{"QZSS L5", 26, 0, "J", "5Q"},
		{"QZSS L6", 27, 0, "J", "6S"},
		{"BeiDou B1I", 28, 0, "C", "2I"},
		{"BeiDou B2I", 29, 0, "C", "7I"},
		{"BeiDou B3I", 30, 0, "C", "6I"},
		{"extension escape", 31, 0, "", ""},
		{"QZSS L1C", 32, 0, "J", "1L"},
		{"QZSS L1S", 33, 0, "J", "1Z"},
		{"BeiDou B2b", 34, 0, "C", "7D"},
		{"reserved 35", 35, 0, "", ""},
		{"reserved 36", 36, 0, "", ""},
		{"NavIC L1", 37, 0, "I", "1P"},
		{"QZSS L1CB", 38, 0, "J", "1E"},
		{"QZSS L5S", 39, 0, "J", "5P"},
		{"undefined", 40, 0, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSys, gotSig := RINEXSig(tt.sig, tt.flags)
			if gotSys != tt.wantSys || gotSig != tt.wantSig {
				t.Errorf("RINEXSig(%d, %d) = %q, %q, want %q, %q", tt.sig, tt.flags, gotSys, gotSig, tt.wantSys, tt.wantSig)
			}
		})
	}
}

func TestRINEXChannelStatusSignals(t *testing.T) {
	tests := []struct {
		name string
		sys  string
		slot int
		want []string
	}{
		{"GPS L1CA", "G", 0, []string{"1C"}},
		{"Galileo E6 family", "E", 3, []string{"6B", "6C"}},
		{"BeiDou B1I", "C", 0, []string{"2I"}},
		{"BeiDou B1C", "C", 3, []string{"1P"}},
		{"QZSS L1CB", "J", 6, []string{"1E"}},
		{"reserved", "E", 0, nil},
		{"bad slot", "G", 8, nil},
		{"bad sys", "X", 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXChannelStatusSignals(tt.sys, tt.slot); !slices.Equal(got, tt.want) {
				t.Errorf("RINEXChannelStatusSignals(%q, %d) = %v, want %v", tt.sys, tt.slot, got, tt.want)
			}
		})
	}
}
