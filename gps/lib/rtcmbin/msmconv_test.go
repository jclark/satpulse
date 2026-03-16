package rtcmbin

import "testing"

func TestLockTimeDF407ToDF402(t *testing.T) {
	// Verify monotonicity: as DF407 increases, the converted DF402 never decreases.
	var prevDF402 uint8
	for ind := uint16(0); ind <= 704; ind++ {
		ms, ok := lockTimeMsFromDF407(ind)
		if !ok {
			t.Fatalf("lockTimeMsFromDF407(%d) returned false", ind)
		}
		df402 := lockTimeToDF402(ms)
		if df402 < prevDF402 {
			t.Errorf("DF402 decreased: ind=%d ms=%d df402=%d, prev=%d", ind, ms, df402, prevDF402)
		}
		prevDF402 = df402
	}
	// Verify that DF402 threshold <= DF407 min lock time for all valid values.
	df402Thresholds := [16]uint32{0, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072, 262144, 524288}
	for ind := uint16(0); ind <= 704; ind++ {
		ms, _ := lockTimeMsFromDF407(ind)
		df402 := lockTimeToDF402(ms)
		if df402Thresholds[df402] > ms {
			t.Errorf("ind=%d: ms=%d but DF402=%d has threshold %d", ind, ms, df402, df402Thresholds[df402])
		}
		// Check that the next DF402 threshold is > ms (tightest fit).
		if df402 < 15 && df402Thresholds[df402+1] <= ms {
			t.Errorf("ind=%d: ms=%d mapped to DF402=%d but threshold[%d]=%d also fits", ind, ms, df402, df402+1, df402Thresholds[df402+1])
		}
	}
	// Reserved values must return false.
	for ind := uint16(705); ind <= 1023; ind++ {
		if _, ok := lockTimeMsFromDF407(ind); ok {
			t.Errorf("lockTimeMsFromDF407(%d) should be reserved", ind)
		}
	}
}

func TestLockTimeMsFromDF407Spot(t *testing.T) {
	// Spot-check against table values.
	tests := []struct {
		ind uint16
		ms  uint32
	}{
		{0, 0},
		{63, 63},
		{64, 64},
		{95, 126},
		{96, 128},
		{127, 252},
		{128, 256},
		{704, 67108864},
	}
	for _, tt := range tests {
		ms, ok := lockTimeMsFromDF407(tt.ind)
		if !ok {
			t.Errorf("lockTimeMsFromDF407(%d) returned false", tt.ind)
		} else if ms != tt.ms {
			t.Errorf("lockTimeMsFromDF407(%d) = %d, want %d", tt.ind, ms, tt.ms)
		}
	}
}

func TestRoundShift(t *testing.T) {
	tests := []struct {
		v    int32
		n    uint
		want int32
	}{
		{32, 5, 1},   // exact
		{16, 5, 1},   // half rounds up
		{15, 5, 0},   // below half
		{-32, 5, -1}, // exact negative
		{-16, 5, -1}, // half rounds away from zero (to -1)
		{-15, 5, 0},  // above half (toward zero)
		{0, 5, 0},
		{100, 2, 25}, // exact
		{5, 2, 1},    // 5/4 rounds to 1
		{6, 2, 2},    // 6/4 rounds to 2 (half rounds up)
		{-6, 2, -2},  // -6/4 rounds to -2 (half away from zero)
		{-5, 2, -1},
	}
	for _, tt := range tests {
		got := roundShift(tt.v, tt.n)
		if got != tt.want {
			t.Errorf("roundShift(%d, %d) = %d, want %d", tt.v, tt.n, got, tt.want)
		}
	}
}

func TestMSM7Convert(t *testing.T) {
	m7 := &MSMHiRes{
		MSMHeader: MSMHeader{
			MsgHdr:    MsgHdr{MsgNum: 1077},
			StationID: 1234,
			SatMask:   1<<63 | 1<<62, // 2 sats
			SigMask:   1 << 31,        // 1 sig
		},
		CellMask: 1<<1 | 1<<0, // 2 cells
		Sat: MSMSatData{
			RangeInt:  Uint8Slice{100, 200},
			ExtInfo:   Uint8Slice{1, 2},
			RangeMod:  []uint16{500, 600},
			PhaseRate: []int16{10, 20},
		},
		Sig: MSMHiResSigData{
			Pseudorange: []int32{1000, -524288},
			PhaseRange:  []int32{2000, -8388608},
			LockTime:    []uint16{100, 0},
			HalfCycle:   []bool{true, false},
			CNR:         []uint16{160, 0},
			PhaseRate:   []int16{100, -100},
		},
	}
	m4, err := MSM7Convert(m7, 4)
	if err != nil {
		t.Fatal(err)
	}
	if m4.MsgNum != 1074 {
		t.Errorf("MsgNum = %d, want 1074", m4.MsgNum)
	}
	if m4.StationID != 1234 {
		t.Errorf("StationID = %d, want 1234", m4.StationID)
	}
	if m4.MSMLevel() != 4 {
		t.Errorf("MSMLevel = %d, want 4", m4.MSMLevel())
	}
	if m4.CellMask != m7.CellMask {
		t.Errorf("CellMask = %d, want %d", m4.CellMask, m7.CellMask)
	}
	// Satellite data
	if len(m4.Sat.ExtInfo) != 0 {
		t.Errorf("ExtInfo len = %d, want 0", len(m4.Sat.ExtInfo))
	}
	if len(m4.Sat.PhaseRate) != 0 {
		t.Errorf("Sat.PhaseRate len = %d, want 0", len(m4.Sat.PhaseRate))
	}
	if len(m4.Sat.RangeInt) != 2 || m4.Sat.RangeInt[0] != 100 {
		t.Errorf("RangeInt = %v, want [100 200]", m4.Sat.RangeInt)
	}
	// Pseudorange: roundShift(1000, 5) = (1000+16)/32 = 31
	if m4.Sig.Pseudorange[0] != 31 {
		t.Errorf("Pseudorange[0] = %d, want 31", m4.Sig.Pseudorange[0])
	}
	if m4.Sig.Pseudorange[1] != -16384 {
		t.Errorf("Pseudorange[1] = %d, want -16384 (invalid sentinel)", m4.Sig.Pseudorange[1])
	}
	// Phase range: roundShift(2000, 2) = (2000+2)/4 = 500
	if m4.Sig.PhaseRange[0] != 500 {
		t.Errorf("PhaseRange[0] = %d, want 500", m4.Sig.PhaseRange[0])
	}
	if m4.Sig.PhaseRange[1] != -2097152 {
		t.Errorf("PhaseRange[1] = %d, want -2097152 (invalid sentinel)", m4.Sig.PhaseRange[1])
	}
	// CNR: (160+8)/16 = 10
	if m4.Sig.CNR[0] != 10 {
		t.Errorf("CNR[0] = %d, want 10", m4.Sig.CNR[0])
	}
	if m4.Sig.CNR[1] != 0 {
		t.Errorf("CNR[1] = %d, want 0", m4.Sig.CNR[1])
	}
	// Lock time: ind=100 -> ms=144 -> DF402=3 (threshold 128)
	if m4.Sig.LockTime[0] != 3 {
		t.Errorf("LockTime[0] = %d, want 3", m4.Sig.LockTime[0])
	}
	if m4.Sig.LockTime[1] != 0 {
		t.Errorf("LockTime[1] = %d, want 0", m4.Sig.LockTime[1])
	}
	// HalfCycle copied
	if m4.Sig.HalfCycle[0] != true || m4.Sig.HalfCycle[1] != false {
		t.Errorf("HalfCycle = %v, want [true false]", m4.Sig.HalfCycle)
	}
	// No signal phase rate in MSM4
	if len(m4.Sig.PhaseRate) != 0 {
		t.Errorf("Sig.PhaseRate len = %d, want 0", len(m4.Sig.PhaseRate))
	}
}

func TestMSM7ConvertClamp(t *testing.T) {
	m7 := &MSMHiRes{
		MSMHeader: MSMHeader{
			MsgHdr:  MsgHdr{MsgNum: 1077},
			SatMask: 1 << 63,
			SigMask: 1 << 31,
		},
		CellMask: 1,
		Sat: MSMSatData{
			RangeInt:  Uint8Slice{100},
			ExtInfo:   Uint8Slice{0},
			RangeMod:  []uint16{500},
			PhaseRate: []int16{0},
		},
		Sig: MSMHiResSigData{
			Pseudorange: []int32{524287},  // max int20-1, >> 5 = 16384, clamped to 16383
			PhaseRange:  []int32{8388607}, // max int24-1, >> 2 = 2097152, clamped to 2097151
			LockTime:    []uint16{0},
			HalfCycle:   []bool{false},
			CNR:         []uint16{1023}, // max uint10, (1023+8)/16=64, clamped to 63
			PhaseRate:   []int16{0},
		},
	}
	m4, err := MSM7Convert(m7, 4)
	if err != nil {
		t.Fatal(err)
	}
	if m4.Sig.Pseudorange[0] != 16383 {
		t.Errorf("Pseudorange = %d, want 16383 (clamped)", m4.Sig.Pseudorange[0])
	}
	if m4.Sig.PhaseRange[0] != 2097151 {
		t.Errorf("PhaseRange = %d, want 2097151 (clamped)", m4.Sig.PhaseRange[0])
	}
	if m4.Sig.CNR[0] != 63 {
		t.Errorf("CNR = %d, want 63 (clamped)", m4.Sig.CNR[0])
	}
}

func TestMSM7ConvertErrorNotMSM7(t *testing.T) {
	m6 := &MSMHiRes{MSMHeader: MSMHeader{MsgHdr: MsgHdr{MsgNum: 1076}}}
	if _, err := MSM7Convert(m6, 4); err == nil {
		t.Error("expected error for MSM6 input")
	}
}

func TestMSM7ConvertErrorBadLevel(t *testing.T) {
	m7 := &MSMHiRes{MSMHeader: MSMHeader{MsgHdr: MsgHdr{MsgNum: 1077}}}
	if _, err := MSM7Convert(m7, 5); err == nil {
		t.Error("expected error for unsupported target level 5")
	}
}

func TestMSM7ConvertErrorReservedLockTime(t *testing.T) {
	m7 := &MSMHiRes{
		MSMHeader: MSMHeader{
			MsgHdr:  MsgHdr{MsgNum: 1077},
			SatMask: 1 << 63,
			SigMask: 1 << 31,
		},
		CellMask: 1,
		Sat: MSMSatData{
			RangeInt:  Uint8Slice{100},
			ExtInfo:   Uint8Slice{0},
			RangeMod:  []uint16{500},
			PhaseRate: []int16{0},
		},
		Sig: MSMHiResSigData{
			Pseudorange: []int32{0},
			PhaseRange:  []int32{0},
			LockTime:    []uint16{705},
			HalfCycle:   []bool{false},
			CNR:         []uint16{0},
			PhaseRate:   []int16{0},
		},
	}
	if _, err := MSM7Convert(m7, 4); err == nil {
		t.Error("expected error for reserved DF407 value 705")
	}
}

func TestMSM7ConvertErrorBadSatSlices(t *testing.T) {
	m7 := &MSMHiRes{
		MSMHeader: MSMHeader{
			MsgHdr:  MsgHdr{MsgNum: 1077},
			SatMask: 1<<63 | 1<<62, // 2 sats
			SigMask: 1 << 31,
		},
		CellMask: 1<<1 | 1<<0,
		Sat: MSMSatData{
			RangeInt:  Uint8Slice{100}, // wrong: 1 instead of 2
			ExtInfo:   Uint8Slice{0, 0},
			RangeMod:  []uint16{500, 600},
			PhaseRate: []int16{0, 0},
		},
	}
	if _, err := MSM7Convert(m7, 4); err == nil {
		t.Error("expected error for mismatched satellite slice lengths")
	}
}

func TestMSM7ConvertErrorBadSigSlices(t *testing.T) {
	m7 := &MSMHiRes{
		MSMHeader: MSMHeader{
			MsgHdr:  MsgHdr{MsgNum: 1077},
			SatMask: 1 << 63,
			SigMask: 1 << 31,
		},
		CellMask: 1,
		Sat: MSMSatData{
			RangeInt:  Uint8Slice{100},
			ExtInfo:   Uint8Slice{0},
			RangeMod:  []uint16{500},
			PhaseRate: []int16{0},
		},
		Sig: MSMHiResSigData{
			Pseudorange: []int32{0, 0}, // wrong: 2 instead of 1
			PhaseRange:  []int32{0},
			LockTime:    []uint16{0},
			HalfCycle:   []bool{false},
			CNR:         []uint16{0},
			PhaseRate:   []int16{0},
		},
	}
	if _, err := MSM7Convert(m7, 4); err == nil {
		t.Error("expected error for mismatched signal slice lengths")
	}
}

func TestMSM7ConvertErrorBadCellMask(t *testing.T) {
	m7 := &MSMHiRes{
		MSMHeader: MSMHeader{
			MsgHdr:  MsgHdr{MsgNum: 1077},
			SatMask: 1 << 63, // 1 sat
			SigMask: 1 << 31, // 1 sig -> 1 cell mask bit allowed
		},
		CellMask: 1<<1 | 1<<0, // 2 bits set, but only 1 bit allowed
		Sat: MSMSatData{
			RangeInt:  Uint8Slice{100},
			ExtInfo:   Uint8Slice{0},
			RangeMod:  []uint16{500},
			PhaseRate: []int16{0},
		},
	}
	if _, err := MSM7Convert(m7, 4); err == nil {
		t.Error("expected error for cell mask with bits beyond sat*sig count")
	}
}
