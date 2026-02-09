package gpsprot

import (
	"slices"
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

func TestSignalSetGNSSSet(t *testing.T) {
	tests := []struct {
		name     string
		signals  SignalSet
		expected GNSSSet
	}{
		{
			name:     "Empty signal set",
			signals:  0,
			expected: 0,
		},
		{
			name:     "GPS signals only",
			signals:  SignalSetOf(SigGPSL1CA, SigGPSL5),
			expected: GNSSSetOf(GPS),
		},
		{
			name:     "Multiple GNSS",
			signals:  SignalSetOf(SigGPSL1CA, SigGALE1, SigBDSB1I),
			expected: GNSSSetOf(GPS, GAL, BDS),
		},
		{
			name:     "All major GNSS",
			signals:  SignalSetOf(SigGPSL1CA, SigGALE1, SigBDSB1I, SigGLOL1),
			expected: GNSSSetOf(GPS, GAL, BDS, GLO),
		},
		{
			name:     "SBAS signals",
			signals:  SignalSetOf(SigSBASL1CA),
			expected: GNSSSetOf(SBAS),
		},
		{
			name:     "Mixed with SBAS",
			signals:  SignalSetOf(SigGPSL1CA, SigSBASL1CA),
			expected: GNSSSetOf(GPS, SBAS),
		},
		{
			name:     "All constellation types",
			signals:  SignalSetOf(SigGPSL1CA, SigGALE1, SigBDSB1I, SigGLOL1, SigQZSSL1CA, SigNAVICL1, SigSBASL1CA),
			expected: GNSSSetOf(GPS, GAL, BDS, GLO, QZSS, NAVIC, SBAS),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.signals.GNSSSet()
			if got != tt.expected {
				t.Errorf("SignalSet.GNSSSet() = %s, want %s", got, tt.expected)
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

func TestSignalSetMapRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		ss   SignalSet
	}{
		{"GPS L1+L2C", SignalSetOf(SigGPSL1CA, SigGPSL2C)},
		{"GPS+GAL", SignalSetOf(SigGPSL1CA, SigGPSL5, SigGALE1, SigGALE5b)},
		{"all major", SigSetGPS | SigSetGAL | SigSetBDS | SigSetGLO},
		{"QZSS", SigSetQZSS},
		{"NavIC", SigSetNAVIC},
		{"SBAS", SigSetSBAS},
		{"GPS+SBAS", SignalSetOf(SigGPSL1CA, SigGPSL5, SigSBASL1CA, SigSBASL5)},
		{"single signal", SignalSetOf(SigBDSB1I)},
		{"everything", SigSetAll},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.ss.GNSSSignalMap()
			got, err := ParseSignalMap(m)
			if err != nil {
				t.Fatalf("ParseSignalMap(%v): %v", m, err)
			}
			if got != tt.ss {
				t.Errorf("round-trip mismatch: got %s, want %s", got, tt.ss)
			}
		})
	}
}

func TestParseSignalMapErrors(t *testing.T) {
	tests := []struct {
		name string
		m    map[string][]string
	}{
		{"bad GNSS", map[string][]string{"BOGUS": {"L1"}}},
		{"bad signal", map[string][]string{"GPS": {"X99"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSignalMap(tt.m)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestGNSSSignalMapOrder(t *testing.T) {
	// Verify that the signal names within each GNSS are in the expected order
	ss := SigSetGPS | SigSetGAL
	m := ss.GNSSSignalMap()
	wantGPS := []string{"L1", "L1C", "L2P", "L2C", "L5"}
	wantGAL := []string{"E1", "E5a", "E5b", "E6"}
	if !slices.Equal(m["GPS"], wantGPS) {
		t.Errorf("GPS signals = %v, want %v", m["GPS"], wantGPS)
	}
	if !slices.Equal(m["GAL"], wantGAL) {
		t.Errorf("GAL signals = %v, want %v", m["GAL"], wantGAL)
	}
}
