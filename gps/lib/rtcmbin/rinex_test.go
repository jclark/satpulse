package rtcmbin

import "testing"

func TestRINEXSys(t *testing.T) {
	tests := []struct {
		name string
		gnss GNSS
		want string
	}{
		{"GPS", GPS, "G"},
		{"GLONASS", GLONASS, "R"},
		{"Galileo", GALILEO, "E"},
		{"SBAS", SBAS, "S"},
		{"QZSS", QZSS, "J"},
		{"BeiDou", BEIDOU, "C"},
		{"IRNSS", IRNSS, "I"},
		{"unknown", GNSS("UNKNOWN"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXSys(tt.gnss); got != tt.want {
				t.Errorf("RINEXSys(%q) = %q, want %q", tt.gnss, got, tt.want)
			}
		})
	}
}

func TestRINEXSatNum(t *testing.T) {
	tests := []struct {
		name  string
		gnss  GNSS
		satID uint8
		want  uint8
	}{
		{"GPS first", GPS, 1, 1},
		{"GPS last", GPS, 63, 63},
		{"GPS reserved", GPS, 64, 0},
		{"GLONASS first", GLONASS, 1, 1},
		{"GLONASS last", GLONASS, 24, 24},
		{"GLONASS reserved", GLONASS, 25, 0},
		{"Galileo first", GALILEO, 1, 1},
		{"Galileo last", GALILEO, 50, 50},
		{"Galileo GIOVE-A", GALILEO, 51, 0},
		{"Galileo GIOVE-B", GALILEO, 52, 0},
		{"Galileo reserved", GALILEO, 53, 0},
		{"SBAS first", SBAS, 1, 20},
		{"SBAS last", SBAS, 39, 58},
		{"SBAS reserved", SBAS, 40, 0},
		{"QZSS first", QZSS, 1, 1},
		{"QZSS last", QZSS, 10, 10},
		{"QZSS reserved", QZSS, 11, 0},
		{"BeiDou first", BEIDOU, 1, 1},
		{"BeiDou last", BEIDOU, 63, 63},
		{"BeiDou reserved", BEIDOU, 64, 0},
		{"IRNSS first", IRNSS, 1, 1},
		{"IRNSS last", IRNSS, 14, 14},
		{"IRNSS reserved", IRNSS, 15, 0},
		{"zero ID", GPS, 0, 0},
		{"above mask", GPS, 65, 0},
		{"unknown GNSS", GNSS("UNKNOWN"), 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXSatNum(tt.gnss, tt.satID); got != tt.want {
				t.Errorf("RINEXSatNum(%q, %d) = %d, want %d", tt.gnss, tt.satID, got, tt.want)
			}
		})
	}
}

func TestRINEXSig(t *testing.T) {
	tests := []struct {
		name  string
		gnss  GNSS
		sigID uint8
		want  string
	}{
		{"GPS 2", GPS, 2, "1C"},
		{"GPS 3", GPS, 3, "1P"},
		{"GPS 4", GPS, 4, "1W"},
		{"GPS 8", GPS, 8, "2C"},
		{"GPS 9", GPS, 9, "2P"},
		{"GPS 10", GPS, 10, "2W"},
		{"GPS 15", GPS, 15, "2S"},
		{"GPS 16", GPS, 16, "2L"},
		{"GPS 17", GPS, 17, "2X"},
		{"GPS 22", GPS, 22, "5I"},
		{"GPS 23", GPS, 23, "5Q"},
		{"GPS 24", GPS, 24, "5X"},
		{"GPS 30", GPS, 30, "1S"},
		{"GPS 31", GPS, 31, "1L"},
		{"GPS 32", GPS, 32, "1X"},
		{"GLONASS 2", GLONASS, 2, "1C"},
		{"GLONASS 3", GLONASS, 3, "1P"},
		{"GLONASS 8", GLONASS, 8, "2C"},
		{"GLONASS 9", GLONASS, 9, "2P"},
		{"Galileo 2", GALILEO, 2, "1C"},
		{"Galileo 3", GALILEO, 3, "1A"},
		{"Galileo 4", GALILEO, 4, "1B"},
		{"Galileo 5", GALILEO, 5, "1X"},
		{"Galileo 6", GALILEO, 6, "1Z"},
		{"Galileo 8", GALILEO, 8, "6C"},
		{"Galileo 9", GALILEO, 9, "6A"},
		{"Galileo 10", GALILEO, 10, "6B"},
		{"Galileo 11", GALILEO, 11, "6X"},
		{"Galileo 12", GALILEO, 12, "6Z"},
		{"Galileo 14", GALILEO, 14, "7I"},
		{"Galileo 15", GALILEO, 15, "7Q"},
		{"Galileo 16", GALILEO, 16, "7X"},
		{"Galileo 18", GALILEO, 18, "8I"},
		{"Galileo 19", GALILEO, 19, "8Q"},
		{"Galileo 20", GALILEO, 20, "8X"},
		{"Galileo 22", GALILEO, 22, "5I"},
		{"Galileo 23", GALILEO, 23, "5Q"},
		{"Galileo 24", GALILEO, 24, "5X"},
		{"SBAS 2", SBAS, 2, "1C"},
		{"SBAS 22", SBAS, 22, "5I"},
		{"SBAS 23", SBAS, 23, "5Q"},
		{"SBAS 24", SBAS, 24, "5X"},
		{"QZSS 2", QZSS, 2, "1C"},
		{"QZSS 9", QZSS, 9, "6S"},
		{"QZSS 10", QZSS, 10, "6L"},
		{"QZSS 11", QZSS, 11, "6X"},
		{"QZSS 15", QZSS, 15, "2S"},
		{"QZSS 16", QZSS, 16, "2L"},
		{"QZSS 17", QZSS, 17, "2X"},
		{"QZSS 22", QZSS, 22, "5I"},
		{"QZSS 23", QZSS, 23, "5Q"},
		{"QZSS 24", QZSS, 24, "5X"},
		{"QZSS 30", QZSS, 30, "1S"},
		{"QZSS 31", QZSS, 31, "1L"},
		{"QZSS 32", QZSS, 32, "1X"},
		{"BeiDou 2", BEIDOU, 2, "2I"},
		{"BeiDou 3", BEIDOU, 3, "2Q"},
		{"BeiDou 4", BEIDOU, 4, "2X"},
		{"BeiDou 8", BEIDOU, 8, "6I"},
		{"BeiDou 9", BEIDOU, 9, "6Q"},
		{"BeiDou 10", BEIDOU, 10, "6X"},
		{"BeiDou 14", BEIDOU, 14, "7I"},
		{"BeiDou 15", BEIDOU, 15, "7Q"},
		{"BeiDou 16", BEIDOU, 16, "7X"},
		{"BeiDou 22", BEIDOU, 22, "5D"},
		{"BeiDou 23", BEIDOU, 23, "5P"},
		{"BeiDou 24", BEIDOU, 24, "5X"},
		{"BeiDou 25", BEIDOU, 25, "7D"},
		{"BeiDou 30", BEIDOU, 30, "1D"},
		{"BeiDou 31", BEIDOU, 31, "1P"},
		{"BeiDou 32", BEIDOU, 32, "1X"},
		{"IRNSS 8", IRNSS, 8, "9A"},
		{"IRNSS 22", IRNSS, 22, "5A"},
		{"reserved GPS", GPS, 1, ""},
		{"unknown signal ID", GPS, 33, ""},
		{"unknown GNSS", GNSS("UNKNOWN"), 2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RINEXSig(tt.gnss, tt.sigID); got != tt.want {
				t.Errorf("RINEXSig(%q, %d) = %q, want %q", tt.gnss, tt.sigID, got, tt.want)
			}
		})
	}
}
