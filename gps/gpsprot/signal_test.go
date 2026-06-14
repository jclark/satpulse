package gpsprot

import (
	"encoding/json"
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

func TestSignalSetBands(t *testing.T) {
	tests := []struct {
		name     string
		signals  SignalSet
		expected Band
	}{
		{
			name:     "empty signal set",
			signals:  0,
			expected: 0,
		},
		{
			name:     "L1 only across multiple GNSS",
			signals:  SignalSetOf(SigGPSL1CA, SigGALE1, SigGLOL1),
			expected: BandL1,
		},
		{
			name:     "L1 augment (SBAS)",
			signals:  SignalSetOf(SigSBASL1CA),
			expected: BandL1,
		},
		{
			name:     "L1 + L2",
			signals:  SignalSetOf(SigGPSL1CA, SigGPSL2C),
			expected: BandL1 | BandL2,
		},
		{
			name:     "L5 only",
			signals:  SignalSetOf(SigGPSL5, SigGALE5a, SigBDSB2a),
			expected: BandL5,
		},
		{
			name:     "E5b only",
			signals:  SignalSetOf(SigGALE5b, SigBDSB2I, SigGLOL3),
			expected: BandE5b,
		},
		{
			name:     "E6 only",
			signals:  SignalSetOf(SigGALE6, SigBDSB3I, SigQZSSL6),
			expected: BandE6,
		},
		{
			name:     "all bands",
			signals:  SignalSetOf(SigGPSL1CA, SigGPSL2C, SigGPSL5, SigGALE5b, SigGALE6),
			expected: BandL1 | BandL2 | BandL5 | BandE5b | BandE6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.signals.Bands()
			if got != tt.expected {
				t.Errorf("SignalSet.Bands() = %s, want %s", got, tt.expected)
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

func TestSignalString(t *testing.T) {
	tests := []struct {
		sig        Signal
		expect     string
		expectName string
	}{
		{SigGPSL1CA, "GPSL1", "L1"},
		{SigGPSL1C, "GPSL1C", "L1C"},
		{SigGPSL2P, "GPSL2P", "L2P"},
		{SigGPSL5, "GPSL5", "L5"},
		{SigGLOL2, "GLOL2", "L2"},
		{SigGLOL1OC, "GLOL1OC", "L1OC"},
		{SigGALE1, "E1", "E1"},
		{SigGALE5b, "E5b", "E5b"},
		{SigBDSB1C, "B1C", "B1C"},
		{SigBDSB2a, "B2a", "B2a"},
		{SigQZSSL1CA, "QZSSL1", "L1"},
		{SigQZSSL1S, "QZSSL1S", "L1S"},
		{SigNAVICL5, "NAVICL5", "L5"},
		{SigSBASL1CA, "SBASL1", "L1"},
	}
	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			if got := tt.sig.String(); got != tt.expect {
				t.Errorf("Signal.String() = %q, want %q", got, tt.expect)
			}
			if got := tt.sig.UnqualifiedName(); got != tt.expectName {
				t.Errorf("Signal.UnqualifiedName() = %q, want %q", got, tt.expectName)
			}
		})
	}
}

func TestParseSignal(t *testing.T) {
	tests := []struct {
		s         string
		expect    Signal
		expectErr bool
	}{
		{s: "GPSL1", expect: SigGPSL1CA},
		{s: "gpsl1c", expect: SigGPSL1C},
		{s: "GPSL5", expect: SigGPSL5},
		{s: "GALE1", expect: SigGALE1},
		{s: "GALILEOE1", expect: SigGALE1},
		{s: "E5b", expect: SigGALE5b},
		{s: "e5A", expect: SigGALE5a},
		{s: "B2b", expect: SigBDSB2b},
		{s: "BEIDOUB1C", expect: SigBDSB1C},
		{s: "GLONASSL2", expect: SigGLOL2},
		{s: "glol1oc", expect: SigGLOL1OC},
		{s: "QZSSL5S", expect: SigQZSSL5S},
		{s: "NAVICL1", expect: SigNAVICL1},
		{s: "SBASL5", expect: SigSBASL5},
		{s: "L1", expectErr: true},
		{s: "L2", expectErr: true},
		{s: "L2C", expectErr: true},
		{s: "L5", expectErr: true},
		{s: "GPSE1", expectErr: true},
		{s: "GALL1", expectErr: true},
		{s: "X99", expectErr: true},
		{s: "GPS", expectErr: true},
		{s: "", expectErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got, err := ParseSignal(tt.s)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("ParseSignal(%q) = %s, expected error", tt.s, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSignal(%q): %v", tt.s, err)
			}
			if got != tt.expect {
				t.Errorf("ParseSignal(%q) = %s, want %s", tt.s, got, tt.expect)
			}
		})
	}
}

func TestParseSignalStringRoundTrip(t *testing.T) {
	for sig := range SigSetAll.Signals() {
		got, err := ParseSignal(sig.String())
		if err != nil {
			t.Fatalf("ParseSignal(%q): %v", sig.String(), err)
		}
		if got != sig {
			t.Errorf("ParseSignal(%q) = %s, want %s", sig.String(), got, sig)
		}
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

func TestBandItems(t *testing.T) {
	tests := []struct {
		name string
		band Band
		want []string
	}{
		{"zero", 0, nil},
		{"L1", BandL1, []string{"L1"}},
		{"L1+L5", BandL1 | BandL5, []string{"L1", "L5"}},
		{"L5+E5b", BandL5 | BandE5b, []string{"L5", "E5b"}},
		{"all five", BandL1 | BandL2 | BandL5 | BandE5b | BandE6, []string{"L1", "L2", "L5", "E5b", "E6"}},
		{"L5 only", BandL5, []string{"L5"}},
		{"E5b only", BandE5b, []string{"E5b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.band.Items()
			if !slices.Equal(got, tt.want) {
				t.Errorf("Band.Items() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBandString(t *testing.T) {
	tests := []struct {
		band Band
		want string
	}{
		{0, ""},
		{BandL1, "L1"},
		{BandL1 | BandL5, "L1,L5"},
		{BandL5 | BandE5b, "L5,E5b"},
		{BandL1 | BandE6, "L1,E6"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.band.String(); got != tt.want {
				t.Errorf("Band.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBandJSONRoundTrip(t *testing.T) {
	tests := []Band{
		0,
		BandL1,
		BandL1 | BandL5,
		BandL5 | BandE5b,
		BandL1 | BandL2 | BandE6,
	}
	for _, b := range tests {
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", b, err)
		}
		var got Band
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", data, err)
		}
		if got != b {
			t.Errorf("round-trip: got %s, want %s", got, b)
		}
	}
}

func TestParseBandName(t *testing.T) {
	tests := []struct {
		name string
		want Band
		ok   bool
	}{
		{"L1", BandL1, true},
		{"L2", BandL2, true},
		{"L5", BandL5, true},
		{"E5", BandL5 | BandE5b, true},
		{"E5b", BandE5b, true},
		{"E6", BandE6, true},
		{"L6", BandE6, true},
		{"l1", BandL1, true},
		{"BOGUS", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseBandName(tt.name)
			if ok != tt.ok || got != tt.want {
				t.Errorf("ParseBandName(%q) = (%s, %v), want (%s, %v)", tt.name, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSignalIDBand(t *testing.T) {
	tests := []struct {
		gnss GNSS
		id   SignalID
		want Band
	}{
		{GPS, SigIDGPSL1CA, BandL1},
		{GPS, SigIDGPSL1C, BandL1},
		{GPS, SigIDGPSL2P, BandL2},
		{GPS, SigIDGPSL2CM, BandL2},
		{GPS, SigIDGPSL5I, BandL5},
		{GLO, SigIDGLOL1, BandL1},
		{GLO, SigIDGLOL2, BandL2},
		{GLO, SigIDGLOL3I, BandE5b},
		{GAL, SigIDGALE1, BandL1},
		{GAL, SigIDGALE5a, BandL5},
		{GAL, SigIDGALE5b, BandE5b},
		{GAL, SigIDGALE5, BandL5 | BandE5b},
		{GAL, SigIDGALE6, BandE6},
		{BDS, SigIDBDSB1I, BandL1},
		{BDS, SigIDBDSB1C, BandL1},
		{BDS, SigIDBDSB2a, BandL5},
		{BDS, SigIDBDSB2I, BandE5b},
		{BDS, SigIDBDSB2b, BandE5b},
		{BDS, SigIDBDSB2ab, BandL5 | BandE5b},
		{BDS, SigIDBDSB3I, BandE6},
		{QZSS, SigIDQZSSL1CA, BandL1},
		{QZSS, SigIDQZSSL6, BandE6},
		{NAVIC, SigIDNAVICS, 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.id), func(t *testing.T) {
			got := SignalIDBand(tt.gnss, tt.id)
			if got != tt.want {
				t.Errorf("SignalIDBand(%s, %q) = %s, want %s", tt.gnss, tt.id, got, tt.want)
			}
		})
	}
}

func TestSatellitesMsgGNSSUsedSignal(t *testing.T) {
	msg := SatellitesMsg{
		UsedValidity: SatelliteUsedSignal,
		SVs: []SVInfo{
			{ID: SVID{GNSS: GPS, Num: 1}, Signals: []SignalInfo{
				{ID: SigIDGPSL1CA, Used: true},
				{ID: SigIDGPSL5I, Used: false},
			}},
			{ID: SVID{GNSS: GAL, Num: 1}, Signals: []SignalInfo{
				{ID: SigIDGALE1, Used: true},
			}},
			{ID: SVID{GNSS: BDS, Num: 1}, Signals: []SignalInfo{
				{ID: SigIDBDSB1I, Used: false},
			}},
		},
	}
	wantGNSS := GNSSSetOf(GPS, GAL)
	if got := msg.GNSSUsed(); got != wantGNSS {
		t.Errorf("GNSSUsed() = %s, want %s", got, wantGNSS)
	}
	wantBands := BandL1
	if got := msg.BandsUsed(); got != wantBands {
		t.Errorf("BandsUsed() = %s, want %s", got, wantBands)
	}
}

func TestSatellitesMsgGNSSUsedSV(t *testing.T) {
	msg := SatellitesMsg{
		UsedValidity: SatelliteUsedSV,
		SVs: []SVInfo{
			{ID: SVID{GNSS: GPS, Num: 1}, Used: true, Signals: []SignalInfo{
				{ID: SigIDGPSL1CA},
				{ID: SigIDGPSL5I},
			}},
			{ID: SVID{GNSS: GLO, Num: 1}, Used: false, Signals: []SignalInfo{
				{ID: SigIDGLOL1},
			}},
		},
	}
	wantGNSS := GNSSSetOf(GPS)
	if got := msg.GNSSUsed(); got != wantGNSS {
		t.Errorf("GNSSUsed() = %s, want %s", got, wantGNSS)
	}
	wantBands := BandL1 | BandL5
	if got := msg.BandsUsed(); got != wantBands {
		t.Errorf("BandsUsed() = %s, want %s", got, wantBands)
	}
}

func TestSatellitesMsgUsedInvalid(t *testing.T) {
	msg := SatellitesMsg{
		UsedValidity: SatelliteUsedInvalid,
		SVs: []SVInfo{
			{ID: SVID{GNSS: GPS, Num: 1}, Used: true, Signals: []SignalInfo{
				{ID: SigIDGPSL1CA, Used: true},
			}},
		},
	}
	if got := msg.GNSSUsed(); got != 0 {
		t.Errorf("GNSSUsed() = %s, want 0", got)
	}
	if got := msg.BandsUsed(); got != 0 {
		t.Errorf("BandsUsed() = %s, want 0", got)
	}
}
