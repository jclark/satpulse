package uncmsg

import "testing"

func TestTrackingStatusFields(t *testing.T) {
	s := TrackingStatus(uint32(SysBDS)<<16 | 13<<21 | 1<<26)
	if got := s.SysID(); got != SysBDS {
		t.Errorf("SysID() = %d, want %d", got, SysBDS)
	}
	if got := s.SignalType(); got != 13 {
		t.Errorf("SignalType() = %d, want 13", got)
	}
	if !s.L2C() {
		t.Errorf("L2C() = false, want true")
	}
}

func TestRINEXSys(t *testing.T) {
	tests := []struct {
		name   string
		system SysID
		want   string
	}{
		{"GPS", SysGPS, "G"},
		{"GLONASS", SysGLO, "R"},
		{"SBAS", SysSBAS, "S"},
		{"Galileo", SysGAL, "E"},
		{"BDS", SysBDS, "C"},
		{"QZSS", SysQZSS, "J"},
		{"NavIC", SysNAVIC, "I"},
		{"unknown", SysID(99), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXSys(tt.system); got != tt.want {
				t.Errorf("RINEXSys(%d) = %q, want %q", tt.system, got, tt.want)
			}
		})
	}
}

func TestRINEXSatNum(t *testing.T) {
	tests := []struct {
		name    string
		system  SysID
		prnSlot uint16
		want    uint8
	}{
		{"GPS first", SysGPS, 1, 1},
		{"GPS last", SysGPS, 32, 32},
		{"GPS reserved", SysGPS, 33, 0},
		{"GLONASS first", SysGLO, 38, 1},
		{"GLONASS last", SysGLO, 61, 24},
		{"GLONASS reserved", SysGLO, 37, 0},
		{"SBAS first", SysSBAS, 120, 20},
		{"SBAS last", SysSBAS, 158, 58},
		{"SBAS reserved", SysSBAS, 159, 0},
		{"Galileo first", SysGAL, 1, 1},
		{"Galileo last", SysGAL, 36, 36},
		{"Galileo reserved", SysGAL, 37, 0},
		{"BDS first", SysBDS, 1, 1},
		{"BDS last", SysBDS, 63, 63},
		{"BDS reserved", SysBDS, 64, 0},
		{"QZSS first", SysQZSS, 193, 1},
		{"QZSS last", SysQZSS, 202, 10},
		{"QZSS reserved", SysQZSS, 203, 0},
		{"NavIC first", SysNAVIC, 1, 1},
		{"NavIC last", SysNAVIC, 14, 14},
		{"NavIC reserved", SysNAVIC, 15, 0},
		{"zero", SysGPS, 0, 0},
		{"unknown system", SysID(99), 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXSatNum(tt.system, tt.prnSlot); got != tt.want {
				t.Errorf("RINEXSatNum(%d, %d) = %d, want %d", tt.system, tt.prnSlot, got, tt.want)
			}
		})
	}
}

func TestRINEXObsSig(t *testing.T) {
	tests := []struct {
		name    string
		system  SysID
		sigType FreqID
		l2c     bool
		want    string
	}{
		{"GPS L1 C/A", SysGPS, 0, false, "1C"},
		{"GPS L2P(Y)", SysGPS, 9, false, "2W"},
		{"GPS L2C M", SysGPS, 9, true, "2S"},
		{"GPS L1C pilot", SysGPS, 3, false, "1L"},
		{"GPS L1C data", SysGPS, 11, false, "1S"},
		{"GPS L5 data", SysGPS, 6, false, "5I"},
		{"GPS L5 pilot", SysGPS, 14, false, "5Q"},
		{"GPS L2C L", SysGPS, 17, false, "2L"},
		{"GLONASS L1 C/A", SysGLO, 0, false, "1C"},
		{"GLONASS L2 C/A", SysGLO, 5, false, "2C"},
		{"GLONASS G3I", SysGLO, 6, false, "3I"},
		{"GLONASS G3Q", SysGLO, 7, false, "3Q"},
		{"Galileo E1B", SysGAL, 1, false, "1B"},
		{"Galileo E1C", SysGAL, 2, false, "1C"},
		{"Galileo E5a pilot", SysGAL, 12, false, "5Q"},
		{"Galileo E5b pilot", SysGAL, 17, false, "7Q"},
		{"Galileo E6B", SysGAL, 18, false, "6B"},
		{"Galileo E6C", SysGAL, 22, false, "6C"},
		{"BDS B1I", SysBDS, 0, false, "2I"},
		{"BDS B1Q", SysBDS, 4, false, "2Q"},
		{"BDS B1C pilot", SysBDS, 8, false, "1P"},
		{"BDS B1C data", SysBDS, 23, false, "1D"},
		{"BDS B2Q", SysBDS, 5, false, "7Q"},
		{"BDS B2I", SysBDS, 17, false, "7I"},
		{"BDS B2a pilot", SysBDS, 12, false, "5P"},
		{"BDS B2a data", SysBDS, 28, false, "5D"},
		{"BDS B3Q", SysBDS, 6, false, "6Q"},
		{"BDS B3I", SysBDS, 21, false, "6I"},
		{"BDS B2b I", SysBDS, 13, false, "7D"},
		{"QZSS L1 C/A", SysQZSS, 0, false, "1C"},
		{"QZSS L1C/B", SysQZSS, 1, false, "1E"},
		{"QZSS L1C pilot", SysQZSS, 3, false, "1L"},
		{"QZSS L1S", SysQZSS, 4, false, "1Z"},
		{"QZSS L5 data", SysQZSS, 6, false, "5I"},
		{"QZSS L1C data", SysQZSS, 11, false, "1S"},
		{"QZSS L5 pilot", SysQZSS, 14, false, "5Q"},
		{"QZSS L2C L", SysQZSS, 17, false, "2L"},
		{"QZSS L6D", SysQZSS, 21, false, "6S"},
		{"QZSS L6E", SysQZSS, 27, false, "6E"},
		{"SBAS L1 C/A", SysSBAS, 0, false, "1C"},
		{"SBAS L5 I", SysSBAS, 6, false, "5I"},
		{"NavIC L5 data", SysNAVIC, 6, false, "5A"},
		{"QZSS sig 9 unmapped", SysQZSS, 9, false, ""},
		{"QZSS sig 9 l2c unmapped", SysQZSS, 9, true, ""},
		{"NavIC L5 pilot unmapped", SysNAVIC, 14, false, ""},
		{"unknown GPS signal", SysGPS, FreqID(31), false, ""},
		{"unknown system", SysID(99), 0, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXObsSig(tt.system, tt.sigType, tt.l2c); got != tt.want {
				t.Errorf("RINEXObsSig(%d, %d, %t) = %q, want %q", tt.system, tt.sigType, tt.l2c, got, tt.want)
			}
		})
	}
}
