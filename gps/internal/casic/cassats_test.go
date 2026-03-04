package casic

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
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

func TestSatsNav2Sig(t *testing.T) {
	m := &casbin.Nav2Sig{
		Nav2SigFixed: casbin.Nav2SigFixed{NumTrkTot: 4, NumFixTot: 3},
		Sigs: []casbin.Nav2SigInfo{
			{GNSSID: 0, SVID: 5, SigID: casbin.SigGPSL1CA, CNO: 40, Elev: 45, Azim: 180, SolFlags: 0x01},
			{GNSSID: 0, SVID: 5, SigID: casbin.SigGPSL5, CNO: 35, Elev: 45, Azim: 180, SolFlags: 0x01},
			{GNSSID: 3, SVID: 12, SigID: casbin.SigGALE1, CNO: 38, Elev: 60, Azim: 270, SolFlags: 0x01},
			{GNSSID: 1, SVID: 10, SigID: casbin.SigBDSB1IMEO, CNO: 30, Elev: 30, Azim: 90, SolFlags: 0x00},
		},
	}
	msg := satsNav2Sig(m)
	if msg == nil {
		t.Fatal("satsNav2Sig() returned nil")
	}
	if msg.Tag != Tag {
		t.Errorf("Tag = %v, want %v", msg.Tag, Tag)
	}
	if msg.NativeMsgID != "NAV2-SIG" {
		t.Errorf("NativeMsgID = %v, want NAV2-SIG", msg.NativeMsgID)
	}
	if msg.UsedValidity != gpsprot.SatelliteUsedSignal {
		t.Errorf("UsedValidity = %v, want SatelliteUsedSignal", msg.UsedValidity)
	}
	// GPS:5 has two signals grouped, GAL:12 has one, BDS:10 has one = 3 SVs
	if len(msg.SVs) != 3 {
		t.Fatalf("len(SVs) = %d, want 3", len(msg.SVs))
	}
	// Check GPS:5 grouping (2 signals)
	gps5 := msg.SVs[0]
	if gps5.ID.GNSS != gpsprot.GPS || gps5.ID.Num != 5 {
		t.Errorf("SVs[0].ID = %v, want GPS:5", gps5.ID)
	}
	if len(gps5.Signals) != 2 {
		t.Fatalf("SVs[0] signals = %d, want 2", len(gps5.Signals))
	}
	if gps5.Signals[0].ID != gpsprot.SigIDGPSL1CA || gps5.Signals[1].ID != gpsprot.SigIDGPSL5I {
		t.Errorf("GPS:5 signals = %v/%v, want L1CA/L5I", gps5.Signals[0].ID, gps5.Signals[1].ID)
	}
	if !gps5.Used {
		t.Error("GPS:5 Used = false, want true")
	}
	// Check BDS:10 not used
	bds10 := msg.SVs[2]
	if bds10.ID.GNSS != gpsprot.BDS || bds10.ID.Num != 10 {
		t.Errorf("SVs[2].ID = %v, want BDS:10", bds10.ID)
	}
	if bds10.Used {
		t.Error("BDS:10 Used = true, want false")
	}
}

func TestSatsNav2SigEmpty(t *testing.T) {
	m := &casbin.Nav2Sig{
		Nav2SigFixed: casbin.Nav2SigFixed{NumTrkTot: 1},
		Sigs:         []casbin.Nav2SigInfo{{GNSSID: 0, SVID: 1, CNO: 0}},
	}
	msg := satsNav2Sig(m)
	if msg != nil {
		t.Errorf("satsNav2Sig() = %+v, want nil (all CNO=0)", msg)
	}
}

func TestCorrFromNav2Sig(t *testing.T) {
	tests := []struct {
		name     string
		corFlags uint8
		want     gpsprot.CorrKind
	}{
		{"NULL", 0x00, 0},
		{"SBAS", 0x01, gpsprot.CorrSBAS | gpsprot.CorrUsed},
		{"BDS_PPP", 0x02, gpsprot.CorrSSR | gpsprot.CorrUsed},
		{"RTCM2", 0x03, gpsprot.CorrRTCM | gpsprot.CorrUsed},
		{"OSR", 0x04, gpsprot.CorrOSR | gpsprot.CorrRTCM | gpsprot.CorrUsed},
		{"SSR", 0x05, gpsprot.CorrSSR | gpsprot.CorrRTCM | gpsprot.CorrUsed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ne gpsprot.NavEpochMsg
			m := &casbin.Nav2Sig{
				Sigs: []casbin.Nav2SigInfo{{
					SolFlags: 0x01, // used
					CorFlags: tt.corFlags,
					SigID:    casbin.SigGPSL1CA,
				}},
			}
			corrFromNav2Sig(&ne, m)
			if ne.Correction != tt.want {
				t.Errorf("Correction = %v, want %v", ne.Correction, tt.want)
			}
		})
	}
}

func TestCorrFromNav2SigNotUsed(t *testing.T) {
	var ne gpsprot.NavEpochMsg
	m := &casbin.Nav2Sig{
		Sigs: []casbin.Nav2SigInfo{{
			SolFlags: 0x00, // NOT used in solution
			CorFlags: 0x01, // SBAS
		}},
	}
	corrFromNav2Sig(&ne, m)
	if ne.Correction != 0 {
		t.Errorf("Correction = %v, want 0 (signal not used)", ne.Correction)
	}
}

func TestCorrFromNav2SigNilNe(t *testing.T) {
	m := &casbin.Nav2Sig{
		Sigs: []casbin.Nav2SigInfo{{SolFlags: 0x01, CorFlags: 0x01}},
	}
	corrFromNav2Sig(nil, m) // should not panic
}

func TestCorrFromNav2SigSignalsUsed(t *testing.T) {
	var ne gpsprot.NavEpochMsg
	m := &casbin.Nav2Sig{
		Sigs: []casbin.Nav2SigInfo{
			{SolFlags: 0x01, SigID: casbin.SigGPSL1CA},
			{SolFlags: 0x01, SigID: casbin.SigGALE1},
			{SolFlags: 0x00, SigID: casbin.SigBDSB1IMEO}, // not used
		},
	}
	corrFromNav2Sig(&ne, m)
	if !ne.SignalsUsed.Contains(gpsprot.SigGPSL1CA) {
		t.Error("SignalsUsed missing GPS L1 C/A")
	}
	if !ne.SignalsUsed.Contains(gpsprot.SigGALE1) {
		t.Error("SignalsUsed missing GAL E1")
	}
	if ne.SignalsUsed.Contains(gpsprot.SigBDSB1I) {
		t.Error("SignalsUsed should not contain BDS B1I (not used)")
	}
}

// TestEpochManagerIntegration verifies that the CASIC processor registers
// with NavEpochManager on the first epoch and that epoch boundaries trigger
// satellite flushing via FlushNavEpoch.
func TestEpochManagerIntegration(t *testing.T) {
	mgr := gpsprot.NewNavEpochManager()
	pp := NewPacketProcessor(mgr)
	h := &mockMsgHandler{}
	pp.SetMsgHandler(h)

	// Helper: build a NAV-GPSINFO packet with the given RunTime and one satellite.
	makePacket := func(runTime uint32, svid, cno uint8) string {
		m := &casbin.NavGPSInfo{
			NavSatInfoFixed: casbin.NavSatInfoFixed{
				NavRunTime: casbin.NavRunTime{RunTime: runTime},
				NumViewSV:  1,
				NumFixSV:   1,
				System:     casbin.GPS,
			},
			SVs: []casbin.NavSVInfo{makeSVInfo(svid, cno, true, 45, 180)},
		}
		pkt, err := casbin.Serialize(m)
		if err != nil {
			t.Fatalf("serialize: %v", err)
		}
		return string(pkt)
	}

	tRead := time.Unix(1, 0)

	// Epoch 1: first NAV-GPSINFO with RunTime=1000.
	// Processor should register with manager (enter active set).
	pp.ProcessPacket(makePacket(1000, 1, 40), tRead)
	if len(h.msgs) != 0 {
		t.Fatalf("epoch 1: unexpected satellite flush")
	}

	// Epoch 2: NAV-GPSINFO with RunTime=2000.
	// Manager detects processor already in active set, triggers FlushNavEpoch
	// for epoch 1, which flushes the accumulated satellite data.
	tRead = time.Unix(2, 0)
	pp.ProcessPacket(makePacket(2000, 2, 35), tRead)
	if len(h.msgs) != 1 {
		t.Fatalf("epoch 2: expected 1 satellite flush, got %d", len(h.msgs))
	}
	if len(h.msgs[0].SVs) != 1 || h.msgs[0].SVs[0].ID.Num != 1 {
		t.Errorf("epoch 2: flushed SV = %v, want GPS:1", h.msgs[0].SVs)
	}

	// Epoch 3: RunTime=3000. Flushes epoch 2 satellites via FlushNavEpoch,
	// then the new NAV-GPSINFO triggers an early flush from satAccum
	// (nEpochs >= 2 and predicted GNSS set matches).
	h.msgs = nil
	tRead = time.Unix(3, 0)
	pp.ProcessPacket(makePacket(3000, 3, 30), tRead)
	if len(h.msgs) != 2 {
		t.Fatalf("epoch 3: expected 2 satellite flushes (epoch + early), got %d", len(h.msgs))
	}
	if len(h.msgs[0].SVs) != 1 || h.msgs[0].SVs[0].ID.Num != 2 {
		t.Errorf("epoch 3 flush: SV = %v, want GPS:2", h.msgs[0].SVs)
	}
	if len(h.msgs[1].SVs) != 1 || h.msgs[1].SVs[0].ID.Num != 3 {
		t.Errorf("epoch 3 early flush: SV = %v, want GPS:3", h.msgs[1].SVs)
	}
}

// TestEpochManagerRunTimeZero verifies that the first epoch is correctly
// handled even when RunTime starts at 0 (e.g. immediately after boot).
func TestEpochManagerRunTimeZero(t *testing.T) {
	mgr := gpsprot.NewNavEpochManager()
	pp := NewPacketProcessor(mgr)
	h := &mockMsgHandler{}
	pp.SetMsgHandler(h)

	makePacket := func(runTime uint32, svid, cno uint8) string {
		m := &casbin.NavGPSInfo{
			NavSatInfoFixed: casbin.NavSatInfoFixed{
				NavRunTime: casbin.NavRunTime{RunTime: runTime},
				NumViewSV:  1,
				NumFixSV:   1,
				System:     casbin.GPS,
			},
			SVs: []casbin.NavSVInfo{makeSVInfo(svid, cno, true, 45, 180)},
		}
		pkt, err := casbin.Serialize(m)
		if err != nil {
			t.Fatalf("serialize: %v", err)
		}
		return string(pkt)
	}

	// Epoch with RunTime=0: must still register with manager.
	pp.ProcessPacket(makePacket(0, 1, 40), time.Unix(1, 0))
	if len(h.msgs) != 0 {
		t.Fatalf("RunTime=0: unexpected satellite flush")
	}
	// Next epoch triggers flush of RunTime=0 epoch.
	pp.ProcessPacket(makePacket(1000, 2, 35), time.Unix(2, 0))
	if len(h.msgs) != 1 {
		t.Fatalf("after RunTime=0: expected 1 satellite flush, got %d", len(h.msgs))
	}
	if len(h.msgs[0].SVs) != 1 || h.msgs[0].SVs[0].ID.Num != 1 {
		t.Errorf("flushed SV = %v, want GPS:1", h.msgs[0].SVs)
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
