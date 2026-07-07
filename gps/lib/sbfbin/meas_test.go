package sbfbin

import (
	"reflect"
	"testing"
)

type valOK struct {
	Val float64
	OK  bool
}

func TestType1Accessors(t *testing.T) {
	tests := []struct {
		name          string
		t1            MeasEpochChannelType1
		expectPR      valOK
		expectDo      valOK
		expectCarrier valOK
	}{
		{
			name: "all valid",
			t1: MeasEpochChannelType1{
				Misc: 0xF5, CodeLSB: 123456789, Doppler: -12345678,
				CarrierMSB: 2, CarrierLSB: 1500,
			},
			expectPR:      valOK{float64(5*4294967296+123456789) * 0.001, true},
			expectDo:      valOK{float64(-12345678) * 0.0001, true},
			expectCarrier: valOK{float64(2*65536+1500) * 0.001, true},
		},
		{
			name: "all DNU",
			t1: MeasEpochChannelType1{
				Misc: 0xF0, CodeLSB: 0, Doppler: MeasType1DopplerDNU,
				CarrierMSB: -128, CarrierLSB: 0,
			},
		},
		{
			name: "compound DNU needs both fields",
			t1: MeasEpochChannelType1{
				Misc: 0, CodeLSB: 1, Doppler: 0,
				CarrierMSB: -128, CarrierLSB: 1,
			},
			expectPR:      valOK{0.001, true},
			expectDo:      valOK{0, true},
			expectCarrier: valOK{float64(-128*65536+1) * 0.001, true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got [3]valOK
			got[0].Val, got[0].OK = tc.t1.Pseudorange()
			got[1].Val, got[1].OK = tc.t1.DopplerHz()
			got[2].Val, got[2].OK = tc.t1.CarrierOffsetCycles()
			expect := [3]valOK{tc.expectPR, tc.expectDo, tc.expectCarrier}
			if !reflect.DeepEqual(got, expect) {
				t.Errorf("got  %+v\nwant %+v", got, expect)
			}
		})
	}
}

func TestType2Accessors(t *testing.T) {
	tests := []struct {
		name          string
		t2            MeasEpochChannelType2
		expectPR      valOK
		expectDo      valOK
		expectCarrier valOK
	}{
		{
			name: "negative offsets sign-extended",
			t2: MeasEpochChannelType2{
				// CodeOffsetMSB -1 (bits 0-2 = 7), DopplerOffsetMSB -2 (bits 3-7 = 30)
				OffsetsMSB: 7 | 30<<3, CodeOffsetLSB: 500, DopplerOffsetLSB: 400,
				CarrierMSB: -1, CarrierLSB: 200,
			},
			expectPR:      valOK{float64(-1*65536+500) * 0.001, true},
			expectDo:      valOK{float64(-2*65536+400) * 0.0001, true},
			expectCarrier: valOK{float64(-1*65536+200) * 0.001, true},
		},
		{
			name: "positive offsets",
			t2: MeasEpochChannelType2{
				OffsetsMSB: 3 | 15<<3, CodeOffsetLSB: 500, DopplerOffsetLSB: 400,
				CarrierMSB: 1, CarrierLSB: 200,
			},
			expectPR:      valOK{float64(3*65536+500) * 0.001, true},
			expectDo:      valOK{float64(15*65536+400) * 0.0001, true},
			expectCarrier: valOK{float64(1*65536+200) * 0.001, true},
		},
		{
			name: "all DNU",
			t2: MeasEpochChannelType2{
				// CodeOffsetMSB -4 (bits 0-2 = 4), DopplerOffsetMSB -16 (bits 3-7 = 16)
				OffsetsMSB: 4 | 16<<3, CodeOffsetLSB: 0, DopplerOffsetLSB: 0,
				CarrierMSB: -128, CarrierLSB: 0,
			},
		},
		{
			name: "compound DNU needs both fields",
			t2: MeasEpochChannelType2{
				OffsetsMSB: 4 | 16<<3, CodeOffsetLSB: 1, DopplerOffsetLSB: 1,
				CarrierMSB: -128, CarrierLSB: 1,
			},
			expectPR:      valOK{float64(-4*65536+1) * 0.001, true},
			expectDo:      valOK{float64(-16*65536+1) * 0.0001, true},
			expectCarrier: valOK{float64(-128*65536+1) * 0.001, true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got [3]valOK
			got[0].Val, got[0].OK = tc.t2.PseudorangeOffset()
			got[1].Val, got[1].OK = tc.t2.DopplerOffsetHz()
			got[2].Val, got[2].OK = tc.t2.CarrierOffsetCycles()
			expect := [3]valOK{tc.expectPR, tc.expectDo, tc.expectCarrier}
			if !reflect.DeepEqual(got, expect) {
				t.Errorf("got  %+v\nwant %+v", got, expect)
			}
		})
	}
}

func TestGLONASSFreqNr(t *testing.T) {
	tests := []struct {
		name     string
		typ      MeasType
		obsInfo  ObsInfo
		expect   int8
		expectOK bool
	}{
		{name: "L1CA freq -7", typ: 8, obsInfo: 1 << 3, expect: -7, expectOK: true},
		{name: "L2CA freq +6", typ: 11, obsInfo: 14 << 3, expect: 6, expectOK: true},
		{name: "zero wire value", typ: 8, obsInfo: 0, expectOK: false},
		{name: "non-FDMA GLONASS L3", typ: 12, obsInfo: 1 << 3, expectOK: false},
		{name: "non-GLONASS signal", typ: 0, obsInfo: 1 << 3, expectOK: false},
		{name: "half-cycle bit ignored", typ: 9, obsInfo: 1<<3 | 1<<2, expect: -7, expectOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t1 := MeasEpochChannelType1{Type: tc.typ, ObsInfo: tc.obsInfo}
			t2 := MeasEpochChannelType2{Type: tc.typ, ObsInfo: tc.obsInfo}
			got1, ok1 := t1.GLONASSFreqNr()
			got2, ok2 := t2.GLONASSFreqNr()
			if got1 != tc.expect || ok1 != tc.expectOK || got2 != tc.expect || ok2 != tc.expectOK {
				t.Errorf("got %d,%v (type1) %d,%v (type2), want %d,%v", got1, ok1, got2, ok2, tc.expect, tc.expectOK)
			}
		})
	}
}

func TestMeasExtraAccessors(t *testing.T) {
	var s MeasExtraChannelSub
	s.Type = 31
	s.Misc = 5 | 1<<3 // CN0HighRes 5, signal extension 32+1
	if got := s.SignalNumber(); got != 33 {
		t.Errorf("SignalNumber = %d, want 33", got)
	}
	if got := s.CN0HighRes(); got != 5*0.03125 {
		t.Errorf("CN0HighRes = %v, want %v", got, 5*0.03125)
	}
	s.Type = 3
	if got := s.SignalNumber(); got != 3 {
		t.Errorf("SignalNumber = %d, want 3", got)
	}
	var m MeasExtra
	for _, tc := range []struct {
		sbLength uint8
		expect   bool
	}{{16, true}, {20, true}, {15, false}, {0, false}} {
		m.SBLength = tc.sbLength
		if got := m.HasCN0HighRes(); got != tc.expect {
			t.Errorf("HasCN0HighRes with SBLength %d = %v, want %v", tc.sbLength, got, tc.expect)
		}
	}
}

func TestObsInfoHalfCycleAmbiguity(t *testing.T) {
	if ObsInfo(0).HalfCycleAmbiguity() || !ObsInfo(1<<2).HalfCycleAmbiguity() {
		t.Errorf("HalfCycleAmbiguity does not select bit 2")
	}
}

func TestMeasEpochSignalAccessors(t *testing.T) {
	t1 := MeasEpochChannelType1{Type: MeasType(2<<5) | MeasType(MeasSigIdxExtension), ObsInfo: 6 << 3}
	if got := t1.AntennaID(); got != 2 {
		t.Errorf("Type1 AntennaID = %d, want 2", got)
	}
	if got := t1.SignalNumber(); got != 38 {
		t.Errorf("Type1 SignalNumber = %d, want 38", got)
	}
	t2 := MeasEpochChannelType2{Type: MeasType(3<<5) | MeasType(SigNumGPSL5)}
	if got := t2.AntennaID(); got != 3 {
		t.Errorf("Type2 AntennaID = %d, want 3", got)
	}
	if got := t2.SignalNumber(); got != SigNumGPSL5 {
		t.Errorf("Type2 SignalNumber = %d, want %d", got, SigNumGPSL5)
	}
}
