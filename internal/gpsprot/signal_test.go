package gpsprot

import (
	"testing"
)

func TestGNSSSignalSet(t *testing.T) {
	tests := []struct {
		name     string
		gnss     GNSS
		expected SignalSet
	}{
		{"GPS", GPS, SigSetGPS},
		{"GLONASS", GLO, SigSetGLO},
		{"Galileo", GAL, SigSetGAL},
		{"BeiDou", BDS, SigSetBDS},
		{"QZSS", QZSS, SigSetQZSS},
		{"NavIC", NAVIC, SigSetNAVIC},
		{"SBAS", SBAS, SigSetSBAS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BandAll.SignalSet(tt.gnss)
			if got != tt.expected {
				t.Errorf("GNSS.SignalSet() for %s = %x, want %x", tt.name, got, tt.expected)
			}
		})
	}
}

func TestSignalSetString(t *testing.T) {
	tests := []struct {
		name     string
		set      SignalSet
		expected string
	}{
		{
			name:     "Empty set",
			set:      0,
			expected: "None",
		},
		{
			name:     "GPS L1 only",
			set:      1 << SigGPSL1CA,
			expected: "GPS[L1]",
		},
		{
			name:     "Multiple GPS signals",
			set:      (1 << SigGPSL1CA) | (1 << SigGPSL5),
			expected: "GPS[L1,L5]",
		},
		{
			name:     "Multiple constellations",
			set:      (1 << SigGPSL1CA) | (1 << SigGALE1) | (1 << SigGALE5b),
			expected: "GPS[L1],GAL[E1,E5b]",
		},
		{
			name:     "Predefined sets",
			set:      SigSetGPS | SigSetGAL,
			expected: "GPS[L1,L1C,L2P,L2C,L5],GAL[E1,E5a,E5b,E6]",
		},
		{
			name:     "SBAS signals",
			set:      SigSetSBAS,
			expected: "SBAS[L1,L5]",
		},
		{
			name:     "Mixed GPS and SBAS",
			set:      (1 << SigGPSL1CA) | (1 << SigSBASL1CA),
			expected: "GPS[L1],SBAS[L1]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.set.String()
			if result != tt.expected {
				t.Errorf("SignalSet.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}
