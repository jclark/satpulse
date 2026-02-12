package casic

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// mockMsgHandler captures SatellitesMsg for test verification.
type mockMsgHandler struct {
	gpsprot.DefaultHandler
	msgs []*gpsprot.SatellitesMsg
}

func (m *mockMsgHandler) Satellites(msg *gpsprot.SatellitesMsg, _ time.Time) {
	m.msgs = append(m.msgs, msg)
}

// makeSVInfo creates a bin.NavSVInfo for testing.
func makeSVInfo(svid, cno uint8, used bool, elev int8, azim int16) casbin.NavSVInfo {
	var flags casbin.NavSVFlags
	if used {
		flags = casbin.NavSVUsed
	}
	return casbin.NavSVInfo{SVID: svid, CNO: cno, Flags: flags, Elev: elev, Azim: azim}
}

// makeFixed creates a bin.NavSatInfoFixed for testing.
func makeFixed(system casbin.GNSSID) casbin.NavSatInfoFixed {
	return casbin.NavSatInfoFixed{System: system}
}

func TestConvertNavSatInfo(t *testing.T) {
	tests := []struct {
		name    string
		system  casbin.GNSSID
		svs     []casbin.NavSVInfo
		wantLen int
		check   func(t *testing.T, result []gpsprot.SVInfo)
	}{
		{
			name:    "empty",
			system:  casbin.GPS,
			svs:     nil,
			wantLen: 0,
		},
		{
			name:    "single GPS",
			system:  casbin.GPS,
			svs:     []casbin.NavSVInfo{makeSVInfo(5, 40, true, 45, 180)},
			wantLen: 1,
			check: func(t *testing.T, result []gpsprot.SVInfo) {
				sv := result[0]
				if sv.ID.GNSS != gpsprot.GPS || sv.ID.Num != 5 {
					t.Errorf("ID = %v, want GPS:5", sv.ID)
				}
				if len(sv.Signals) != 1 || sv.Signals[0].ID != gpsprot.SigIDGPSL1CA {
					t.Errorf("signal = %v, want L1CA", sv.Signals)
				}
				if sv.Signals[0].CN0 != 40 {
					t.Errorf("CN0 = %d, want 40", sv.Signals[0].CN0)
				}
				if !sv.Used || !sv.Signals[0].Used {
					t.Errorf("Used = %v/%v, want true/true", sv.Used, sv.Signals[0].Used)
				}
				if sv.LookAngles.Azimuth != 180 || sv.LookAngles.Elevation != 45 {
					t.Errorf("LookAngles = %v, want {180, 45}", sv.LookAngles)
				}
			},
		},
		{
			name:    "single BDS",
			system:  casbin.BDS,
			svs:     []casbin.NavSVInfo{makeSVInfo(10, 35, false, 30, 90)},
			wantLen: 1,
			check: func(t *testing.T, result []gpsprot.SVInfo) {
				sv := result[0]
				if sv.ID.GNSS != gpsprot.BDS || sv.ID.Num != 10 {
					t.Errorf("ID = %v, want BDS:10", sv.ID)
				}
				if sv.Signals[0].ID != gpsprot.SigIDBDSB1I {
					t.Errorf("signal = %v, want B1I", sv.Signals[0].ID)
				}
				if sv.Used || sv.Signals[0].Used {
					t.Errorf("Used = %v/%v, want false/false", sv.Used, sv.Signals[0].Used)
				}
			},
		},
		{
			name:    "single GLN",
			system:  casbin.GLN,
			svs:     []casbin.NavSVInfo{makeSVInfo(3, 30, true, 60, 270)},
			wantLen: 1,
			check: func(t *testing.T, result []gpsprot.SVInfo) {
				sv := result[0]
				if sv.ID.GNSS != gpsprot.GLO || sv.ID.Num != 3 {
					t.Errorf("ID = %v, want GLO:3", sv.ID)
				}
				if sv.Signals[0].ID != gpsprot.SigIDGLOL1 {
					t.Errorf("signal = %v, want L1", sv.Signals[0].ID)
				}
			},
		},
		{
			name:   "filter CNO zero",
			system: casbin.GPS,
			svs: []casbin.NavSVInfo{
				makeSVInfo(1, 0, false, 10, 100),
				makeSVInfo(2, 40, true, 20, 200),
				makeSVInfo(3, 0, false, 30, 300),
			},
			wantLen: 1,
			check: func(t *testing.T, result []gpsprot.SVInfo) {
				if result[0].ID.Num != 2 {
					t.Errorf("got SV %d, want SV 2", result[0].ID.Num)
				}
			},
		},
		{
			name:    "look angles",
			system:  casbin.GPS,
			svs:     []casbin.NavSVInfo{makeSVInfo(7, 25, false, -5, 359)},
			wantLen: 1,
			check: func(t *testing.T, result []gpsprot.SVInfo) {
				la := result[0].LookAngles
				if la.Elevation != -5 || la.Azimuth != 359 {
					t.Errorf("LookAngles = {%d, %d}, want {359, -5}", la.Azimuth, la.Elevation)
				}
			},
		},
		{
			name:    "SBAS in GPS message",
			system:  casbin.GPS,
			svs:     []casbin.NavSVInfo{makeSVInfo(120, 35, true, 40, 200)}, // PRN 120 = SBAS
			wantLen: 1,
			check: func(t *testing.T, result []gpsprot.SVInfo) {
				sv := result[0]
				if sv.ID.GNSS != gpsprot.SBAS || sv.ID.Num != 20 {
					t.Errorf("ID = %v, want SBAS:20 (PRN 120 - 100)", sv.ID)
				}
			},
		},
		{
			name:    "QZSS in GPS message",
			system:  casbin.GPS,
			svs:     []casbin.NavSVInfo{makeSVInfo(193, 42, true, 70, 150)}, // PRN 193 = QZSS
			wantLen: 1,
			check: func(t *testing.T, result []gpsprot.SVInfo) {
				sv := result[0]
				if sv.ID.GNSS != gpsprot.QZSS || sv.ID.Num != 1 {
					t.Errorf("ID = %v, want QZSS:1 (PRN 193 - 192)", sv.ID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixed := makeFixed(tt.system)
			result := convertNavSatInfo(&fixed, tt.svs)
			if len(result) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(result), tt.wantLen)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestSatAccumEpochMerging(t *testing.T) {
	tRead := time.Now()

	// Message types for test sequences.
	type msg struct {
		system casbin.GNSSID
		svs    []casbin.NavSVInfo
	}

	// Action in a test step: either a message or an epoch change.
	type step struct {
		msg         *msg // nil means epoch change
		expectFlush int  // expected number of SVs in flush, -1 means no flush
	}

	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "single epoch GPS only",
			steps: []step{
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(1, 40, true, 45, 180)}}, expectFlush: -1},
				{msg: nil, expectFlush: 1}, // epoch change flushes
			},
		},
		{
			name: "single epoch multi-GNSS",
			steps: []step{
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(1, 40, true, 45, 180)}}, expectFlush: -1},
				{msg: &msg{casbin.BDS, []casbin.NavSVInfo{makeSVInfo(5, 35, false, 30, 90)}}, expectFlush: -1},
				{msg: &msg{casbin.GLN, []casbin.NavSVInfo{makeSVInfo(3, 30, true, 60, 270)}}, expectFlush: -1},
				{msg: nil, expectFlush: 3}, // epoch change flushes all 3
			},
		},
		{
			name: "no early flush during first epoch",
			steps: []step{
				// First epoch with all 3 GNSS - should NOT early flush (nEpochs < 2)
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(1, 40, true, 45, 180)}}, expectFlush: -1},
				{msg: &msg{casbin.BDS, []casbin.NavSVInfo{makeSVInfo(5, 35, false, 30, 90)}}, expectFlush: -1},
				{msg: &msg{casbin.GLN, []casbin.NavSVInfo{makeSVInfo(3, 30, true, 60, 270)}}, expectFlush: -1},
				{msg: nil, expectFlush: 3}, // epoch change
			},
		},
		{
			name: "early flush after warmup all three GNSS",
			steps: []step{
				// Epoch 1: warmup
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(1, 40, true, 45, 180)}}, expectFlush: -1},
				{msg: &msg{casbin.BDS, []casbin.NavSVInfo{makeSVInfo(5, 35, false, 30, 90)}}, expectFlush: -1},
				{msg: &msg{casbin.GLN, []casbin.NavSVInfo{makeSVInfo(3, 30, true, 60, 270)}}, expectFlush: -1},
				{msg: nil, expectFlush: 3},
				// Epoch 2: warmup complete
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(2, 42, true, 50, 120)}}, expectFlush: -1},
				{msg: &msg{casbin.BDS, []casbin.NavSVInfo{makeSVInfo(6, 38, true, 35, 95)}}, expectFlush: -1},
				{msg: &msg{casbin.GLN, []casbin.NavSVInfo{makeSVInfo(4, 32, false, 65, 275)}}, expectFlush: -1},
				{msg: nil, expectFlush: 3},
				// Epoch 3: should early flush when GLN arrives (all 3 GNSS)
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(1, 41, true, 46, 181)}}, expectFlush: -1},
				{msg: &msg{casbin.BDS, []casbin.NavSVInfo{makeSVInfo(5, 36, false, 31, 91)}}, expectFlush: -1},
				{msg: &msg{casbin.GLN, []casbin.NavSVInfo{makeSVInfo(3, 31, true, 61, 271)}}, expectFlush: 3}, // early flush!
			},
		},
		{
			name: "early flush on predicted GNSS subset",
			steps: []step{
				// Epoch 1: GPS + BDS only
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(1, 40, true, 45, 180)}}, expectFlush: -1},
				{msg: &msg{casbin.BDS, []casbin.NavSVInfo{makeSVInfo(5, 35, false, 30, 90)}}, expectFlush: -1},
				{msg: nil, expectFlush: 2},
				// Epoch 2: warmup complete, predicted = GPS|BDS
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(2, 42, true, 50, 120)}}, expectFlush: -1},
				{msg: &msg{casbin.BDS, []casbin.NavSVInfo{makeSVInfo(6, 38, true, 35, 95)}}, expectFlush: -1},
				{msg: nil, expectFlush: 2},
				// Epoch 3: should early flush when BDS arrives (matches predicted)
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{makeSVInfo(1, 41, true, 46, 181)}}, expectFlush: -1},
				{msg: &msg{casbin.BDS, []casbin.NavSVInfo{makeSVInfo(5, 36, false, 31, 91)}}, expectFlush: 2}, // early flush!
			},
		},
		{
			name: "accumulate multiple SVs per GNSS",
			steps: []step{
				{msg: &msg{casbin.GPS, []casbin.NavSVInfo{
					makeSVInfo(1, 40, true, 45, 180),
					makeSVInfo(2, 38, true, 30, 90),
					makeSVInfo(3, 0, false, 10, 50), // filtered (CNO=0)
				}}, expectFlush: -1},
				{msg: nil, expectFlush: 2}, // only 2 SVs (CNO>0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a satAccum
			h := &mockMsgHandler{}

			for i, step := range tt.steps {
				h.msgs = nil // reset for this step

				if step.msg != nil {
					fixed := makeFixed(step.msg.system)
					a.accum(&fixed, step.msg.svs, h, tRead)
				} else {
					a.epochChange(h, tRead)
				}

				if step.expectFlush == -1 {
					if len(h.msgs) != 0 {
						t.Errorf("step %d: unexpected flush with %d SVs", i, len(h.msgs[0].SVs))
					}
				} else {
					if len(h.msgs) != 1 {
						t.Fatalf("step %d: expected 1 flush, got %d", i, len(h.msgs))
					}
					if len(h.msgs[0].SVs) != step.expectFlush {
						t.Errorf("step %d: flush had %d SVs, want %d", i, len(h.msgs[0].SVs), step.expectFlush)
					}
					// Verify message metadata
					if h.msgs[0].Tag != Tag {
						t.Errorf("step %d: Tag = %v, want %v", i, h.msgs[0].Tag, Tag)
					}
					if h.msgs[0].NativeMsgID != "NAV-SATINFO" {
						t.Errorf("step %d: NativeMsgID = %v, want NAV-SATINFO", i, h.msgs[0].NativeMsgID)
					}
					if h.msgs[0].UsedValidity != gpsprot.SatelliteUsedSignal {
						t.Errorf("step %d: UsedValidity = %v, want SatelliteUsedSignal", i, h.msgs[0].UsedValidity)
					}
				}
			}
		})
	}
}

func TestSatAccumBitmasks(t *testing.T) {
	tRead := time.Now()
	h := &mockMsgHandler{}
	var a satAccum

	// Initial state
	if a.received != 0 || a.predicted != 0 {
		t.Errorf("initial: received=%d predicted=%d, want 0/0", a.received, a.predicted)
	}

	// Add GPS
	fixed := makeFixed(casbin.GPS)
	a.accum(&fixed, []casbin.NavSVInfo{makeSVInfo(1, 40, true, 45, 180)}, h, tRead)
	if a.received != 1<<casbin.GPS {
		t.Errorf("after GPS: received=%d, want %d", a.received, 1<<casbin.GPS)
	}

	// Add BDS
	fixed = makeFixed(casbin.BDS)
	a.accum(&fixed, []casbin.NavSVInfo{makeSVInfo(5, 35, false, 30, 90)}, h, tRead)
	if a.received != (1<<casbin.GPS | 1<<casbin.BDS) {
		t.Errorf("after BDS: received=%d, want %d", a.received, 1<<casbin.GPS|1<<casbin.BDS)
	}

	// Epoch change - predicted should capture received, received should reset
	a.epochChange(h, tRead)
	if a.received != 0 {
		t.Errorf("after epoch: received=%d, want 0", a.received)
	}
	if a.predicted != (1<<casbin.GPS | 1<<casbin.BDS) {
		t.Errorf("after epoch: predicted=%d, want %d", a.predicted, 1<<casbin.GPS|1<<casbin.BDS)
	}
}

func TestSatAccumNEpochsCounter(t *testing.T) {
	tRead := time.Now()
	h := &mockMsgHandler{}
	var a satAccum

	// Initial state
	if a.nEpochs != 0 {
		t.Errorf("initial: nEpochs=%d, want 0", a.nEpochs)
	}

	// First epoch change
	a.epochChange(h, tRead)
	if a.nEpochs != 1 {
		t.Errorf("after 1st epoch: nEpochs=%d, want 1", a.nEpochs)
	}

	// Second epoch change
	a.epochChange(h, tRead)
	if a.nEpochs != 2 {
		t.Errorf("after 2nd epoch: nEpochs=%d, want 2", a.nEpochs)
	}

	// Third epoch change
	a.epochChange(h, tRead)
	if a.nEpochs != 3 {
		t.Errorf("after 3rd epoch: nEpochs=%d, want 3", a.nEpochs)
	}
}

func TestSatAccumEmptyFlush(t *testing.T) {
	tRead := time.Now()
	h := &mockMsgHandler{}
	var a satAccum

	// Add message with only CNO=0 satellites (all filtered)
	fixed := makeFixed(casbin.GPS)
	a.accum(&fixed, []casbin.NavSVInfo{makeSVInfo(1, 0, false, 45, 180)}, h, tRead)

	// Verify received is still set even though no SVs accumulated
	if a.received != 1<<casbin.GPS {
		t.Errorf("received=%d, want %d", a.received, 1<<casbin.GPS)
	}
	if len(a.svs) != 0 {
		t.Errorf("svs len=%d, want 0", len(a.svs))
	}

	// Epoch change - should not emit (no SVs) but should update predicted
	a.epochChange(h, tRead)
	if len(h.msgs) != 0 {
		t.Errorf("unexpected flush with empty SVs")
	}
	if a.predicted != 1<<casbin.GPS {
		t.Errorf("predicted=%d, want %d", a.predicted, 1<<casbin.GPS)
	}
}

func TestSatAccumNilHandler(t *testing.T) {
	tRead := time.Now()
	var a satAccum

	// Should not panic with nil handler
	fixed := makeFixed(casbin.GPS)
	a.accum(&fixed, []casbin.NavSVInfo{makeSVInfo(1, 40, true, 45, 180)}, nil, tRead)
	a.epochChange(nil, tRead)

	// State should still be updated
	if a.nEpochs != 1 {
		t.Errorf("nEpochs=%d, want 1", a.nEpochs)
	}
}
