package unc

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

type testMsgHandler struct {
	gpsprot.DefaultHandler
	msgs []testHandledMsg
}

type testHandledMsg struct {
	msgType string
	msg     any
	tRead   time.Time
}

func (h *testMsgHandler) PosGeo(msg *gpsprot.PosGeoMsg, tRead time.Time) {
	h.msgs = append(h.msgs, testHandledMsg{"posgeo", msg, tRead})
}

func (h *testMsgHandler) VelGeo(msg *gpsprot.VelGeoMsg, tRead time.Time) {
	h.msgs = append(h.msgs, testHandledMsg{"velgeo", msg, tRead})
}

func (h *testMsgHandler) PosECEF(msg *gpsprot.PosECEFMsg, tRead time.Time) {
	h.msgs = append(h.msgs, testHandledMsg{"posecef", msg, tRead})
}

func (h *testMsgHandler) VelECEF(msg *gpsprot.VelECEFMsg, tRead time.Time) {
	h.msgs = append(h.msgs, testHandledMsg{"velecef", msg, tRead})
}

func (h *testMsgHandler) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	h.msgs = append(h.msgs, testHandledMsg{"navepoch", msg, tRead})
}

func (h *testMsgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	h.msgs = append(h.msgs, testHandledMsg{"time", msg, tRead})
}

func (h *testMsgHandler) Satellites(msg *gpsprot.SatellitesMsg, tRead time.Time) {
	h.msgs = append(h.msgs, testHandledMsg{"satellites", msg, tRead})
}

func makeMsg(week uint16, ms uint32, body uncmsg.MsgBody) *uncmsg.Msg {
	return &uncmsg.Msg{
		Hdr: uncmsg.MsgHdr{
			TimingHdr: uncmsg.TimingHdr{
				Week:               week,
				MillisecondsOfWeek: ms,
			},
		},
		Body: body,
	}
}

func TestDispatchBestNav(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	pp.mh = &gpsprot.DefaultHandler{}
	h := &testMsgHandler{}
	pp.mh = h

	msg := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.49,
			Lon:        8.56,
			Hgt:        489.0,
			Undulation: 50.0,
			LatSigma:   1.0,
			LonSigma:   1.0,
			HgtSigma:   2.0,
		},
		VSolStatus: uncmsg.SolComputed,
		VelType:    uncmsg.PosVelDopplerVelocity,
		HorSpd:     1.5,
		TrkGnd:     90.0,
	})
	handled, err := pp.dispatch(msg, time.Unix(1, 0), TagBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected handled")
	}
	var gotPosGeo, gotVelGeo bool
	for _, m := range h.msgs {
		switch m.msgType {
		case "posgeo":
			gotPosGeo = true
			pg := m.msg.(*gpsprot.PosGeoMsg)
			if pg.Tag != TagBinary {
				t.Errorf("PosGeo.Tag = %q, want %q", pg.Tag, TagBinary)
			}
			if pg.NativeMsgID != "BESTNAV" {
				t.Errorf("NativeMsgID = %q, want %q", pg.NativeMsgID, "BESTNAV")
			}
		case "velgeo":
			gotVelGeo = true
			vg := m.msg.(*gpsprot.VelGeoMsg)
			if vg.Tag != TagBinary {
				t.Errorf("VelGeo.Tag = %q, want %q", vg.Tag, TagBinary)
			}
		}
	}
	if !gotPosGeo {
		t.Error("no PosGeo dispatched")
	}
	if !gotVelGeo {
		t.Error("no VelGeo dispatched")
	}
}

func TestDispatchBestNavXYZ(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	msg := makeMsg(2350, 100000, &uncmsg.BestNavXYZ{XYZ: novmsg.XYZ[uncmsg.SolStatus, uncmsg.PosVelType]{
		PSolStatus: uncmsg.SolComputed,
		PosType:    uncmsg.PosVelSingle,
		PX:         -2671733.51,
		PY:         -4027532.74,
		PZ:         3919194.98,
		PXSigma:    1.5,
		PYSigma:    2.0,
		PZSigma:    1.8,
		VSolStatus: uncmsg.SolComputed,
		VelType:    uncmsg.PosVelDopplerVelocity,
		VX:         -0.15,
		VY:         0.23,
		VZ:         -0.08,
		VXSigma:    0.05,
		VYSigma:    0.04,
		VZSigma:    0.06,
	}})
	handled, err := pp.dispatch(msg, time.Unix(1, 0), TagBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected handled")
	}
	var gotPosECEF, gotVelECEF bool
	for _, m := range h.msgs {
		switch m.msgType {
		case "posecef":
			gotPosECEF = true
			pe := m.msg.(*gpsprot.PosECEFMsg)
			if pe.Tag != TagBinary {
				t.Errorf("PosECEF.Tag = %q, want %q", pe.Tag, TagBinary)
			}
			if pe.NativeMsgID != "BESTNAVXYZ" {
				t.Errorf("NativeMsgID = %q, want %q", pe.NativeMsgID, "BESTNAVXYZ")
			}
		case "velecef":
			gotVelECEF = true
			ve := m.msg.(*gpsprot.VelECEFMsg)
			if ve.Tag != TagBinary {
				t.Errorf("VelECEF.Tag = %q, want %q", ve.Tag, TagBinary)
			}
		}
	}
	if !gotPosECEF {
		t.Error("no PosECEF dispatched")
	}
	if !gotVelECEF {
		t.Error("no VelECEF dispatched")
	}
}

func TestDispatchBestNavNotComputed(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	msg := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
		},
		VSolStatus: uncmsg.NoConvergence,
	})
	handled, err := pp.dispatch(msg, time.Unix(1, 0), TagBinary)
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Error("expected not handled when neither computed")
	}
	for _, m := range h.msgs {
		if m.msgType == "posgeo" || m.msgType == "velgeo" {
			t.Errorf("unexpected %s dispatched", m.msgType)
		}
	}
}

func TestEpochTracking(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// Epoch 1: week=2350, ms=100000
	msg1 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.0,
			Lon:        8.0,
			Hgt:        400.0,
			LatSigma:   1.0,
			LonSigma:   1.0,
			HgtSigma:   2.0,
		},
		VSolStatus: uncmsg.SolComputed,
		VelType:    uncmsg.PosVelDopplerVelocity,
		HorSpd:     1.0,
		TrkGnd:     90.0,
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)

	// No NavEpoch should be emitted yet (first epoch, nothing to flush)
	for _, m := range h.msgs {
		if m.msgType == "navepoch" {
			t.Fatal("NavEpoch emitted before first epoch boundary")
		}
	}

	// Epoch 2: week=2350, ms=101000 - triggers flush of epoch 1
	msg2 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.1,
			Lon:        8.1,
			Hgt:        401.0,
			LatSigma:   1.0,
			LonSigma:   1.0,
			HgtSigma:   2.0,
		},
		VSolStatus: uncmsg.SolComputed,
		VelType:    uncmsg.PosVelDopplerVelocity,
		HorSpd:     1.1,
		TrkGnd:     91.0,
	})
	beforeCount := len(h.msgs)
	pp.dispatch(msg2, time.Unix(2, 0), TagBinary)

	var epochMsg *gpsprot.NavEpochMsg
	var epochTRead time.Time
	for _, m := range h.msgs[beforeCount:] {
		if m.msgType == "navepoch" {
			epochMsg = m.msg.(*gpsprot.NavEpochMsg)
			epochTRead = m.tRead
		}
	}
	if epochMsg == nil {
		t.Fatal("no NavEpoch emitted at epoch boundary")
	}
	if epochMsg.Tag != TagBinary {
		t.Errorf("NavEpoch.Tag = %q, want %q", epochMsg.Tag, TagBinary)
	}

	if epochTRead != time.Unix(2, 0) {
		t.Errorf("NavEpoch tRead = %v, want %v", epochTRead, time.Unix(2, 0))
	}
	// Epoch 1 should have accumulated accuracy from the BESTNAV message
	if !epochMsg.Acc.Hor.IsSet() {
		t.Error("NavEpoch.Acc.Hor should be set")
	}
	if !epochMsg.Acc.Vert.IsSet() {
		t.Error("NavEpoch.Acc.Vert should be set")
	}
}

func TestEpochTagFromFirstMessage(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// First message in epoch uses TagAscii
	msg1 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.0,
			Lon:        8.0,
			Hgt:        400.0,
			LatSigma:   1.0,
			LonSigma:   1.0,
			HgtSigma:   2.0,
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagAscii)

	// Second message in same epoch uses TagBinary - should not change epoch tag
	msg2 := makeMsg(2350, 100000, &uncmsg.BestNavXYZ{XYZ: novmsg.XYZ[uncmsg.SolStatus, uncmsg.PosVelType]{
		PSolStatus: uncmsg.SolComputed,
		PosType:    uncmsg.PosVelSingle,
		PX:         1.0,
		PY:         2.0,
		PZ:         3.0,
		PXSigma:    1.0,
		PYSigma:    1.0,
		PZSigma:    1.0,
	}})
	pp.dispatch(msg2, time.Unix(1, 0), TagBinary)

	// Trigger flush with new epoch
	msg3 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
		},
		VSolStatus: uncmsg.NoConvergence,
	})
	pp.dispatch(msg3, time.Unix(2, 0), TagBinary)

	for _, m := range h.msgs {
		if m.msgType == "navepoch" {
			ne := m.msg.(*gpsprot.NavEpochMsg)
			if ne.Tag != TagAscii {
				t.Errorf("NavEpoch.Tag = %q, want %q (from first message)", ne.Tag, TagAscii)
			}
			return
		}
	}
	t.Fatal("no NavEpoch emitted")
}

func TestSameEpochNoFlush(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// Two messages in the same epoch
	msg1 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.0,
			Lon:        8.0,
			Hgt:        400.0,
			LatSigma:   1.0,
			LonSigma:   1.0,
			HgtSigma:   2.0,
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)

	msg2 := makeMsg(2350, 100000, &uncmsg.BestNavXYZ{XYZ: novmsg.XYZ[uncmsg.SolStatus, uncmsg.PosVelType]{
		PSolStatus: uncmsg.SolComputed,
		PosType:    uncmsg.PosVelSingle,
		PX:         1.0,
		PY:         2.0,
		PZ:         3.0,
		PXSigma:    1.0,
		PYSigma:    1.0,
		PZSigma:    1.0,
	}})
	pp.dispatch(msg2, time.Unix(1, 0), TagBinary)

	for _, m := range h.msgs {
		if m.msgType == "navepoch" {
			t.Fatal("NavEpoch should not be emitted within same epoch")
		}
	}
}

func TestDispatchStaDOP(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// First send a BESTNAV to start an epoch
	msg1 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.0,
			Lon:        8.0,
			Hgt:        400.0,
			LatSigma:   1.0,
			LonSigma:   1.0,
			HgtSigma:   2.0,
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)

	// Send STADOP in same epoch
	msg2 := makeMsg(2350, 100000, &uncmsg.StaDOP{
		StaDOPFixed: uncmsg.StaDOPFixed{
			GDOP: 2.5,
			PDOP: 2.1,
			TDOP: 1.3,
			VDOP: 1.8,
			HDOP: 1.1,
		},
	})
	handled, err := pp.dispatch(msg2, time.Unix(1, 0), TagBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected STADOP to be handled")
	}

	// Trigger flush with new epoch
	msg3 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
		},
		VSolStatus: uncmsg.NoConvergence,
	})
	pp.dispatch(msg3, time.Unix(2, 0), TagBinary)

	for _, m := range h.msgs {
		if m.msgType != "navepoch" {
			continue
		}
		ne := m.msg.(*gpsprot.NavEpochMsg)
		if !ne.DOP.Geom.IsSet() || ne.DOP.Geom.Get() != float64(float32(2.5)) {
			t.Errorf("DOP.Geom = %v, want 2.5", ne.DOP.Geom)
		}
		if !ne.DOP.Pos.IsSet() || ne.DOP.Pos.Get() != float64(float32(2.1)) {
			t.Errorf("DOP.Pos = %v, want 2.1", ne.DOP.Pos)
		}
		if !ne.DOP.Hor.IsSet() || ne.DOP.Hor.Get() != float64(float32(1.1)) {
			t.Errorf("DOP.Hor = %v, want 1.1", ne.DOP.Hor)
		}
		if !ne.DOP.Vert.IsSet() || ne.DOP.Vert.Get() != float64(float32(1.8)) {
			t.Errorf("DOP.Vert = %v, want 1.8", ne.DOP.Vert)
		}
		if !ne.DOP.Time.IsSet() || ne.DOP.Time.Get() != float64(float32(1.3)) {
			t.Errorf("DOP.Time = %v, want 1.3", ne.DOP.Time)
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestEpochQualityFields(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// Epoch 1: BESTNAV with RTK fixed (NARROW_INT=50)
	msg1 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus:    uncmsg.SolComputed,
			PosType:       uncmsg.PosVelNarrowInt,
			Lat:           47.0,
			Lon:           8.0,
			Hgt:           400.0,
			LatSigma:      0.01,
			LonSigma:      0.01,
			HgtSigma:      0.02,
			DiffAge:       1.5,
			StnID:         novmsg.StationID{'1', '2', '3', 0},
			NumSVs:        20,
			NumSolnSVs:    15,
			GPSGLOBDS2Sig: 0x01, // GPS L1CA
			GalBDS3Sig:    0x01, // GAL E1
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)

	// Flush with new epoch
	msg2 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
		},
	})
	pp.dispatch(msg2, time.Unix(2, 0), TagBinary)

	for _, m := range h.msgs {
		if m.msgType != "navepoch" {
			continue
		}
		ne := m.msg.(*gpsprot.NavEpochMsg)
		if ne.FixLevel != gpsprot.FixLevelCarrierFixed {
			t.Errorf("FixLevel = %v, want CarrierFixed", ne.FixLevel)
		}
		if ne.SolutionDim != gpsprot.SolutionDim3D {
			t.Errorf("SolutionDim = %v, want 3D", ne.SolutionDim)
		}
		if ne.Correction != gpsprot.CorrFullDualFreq.Expand() {
			t.Errorf("Correction = %v, want FullDualFreq expanded", ne.Correction)
		}
		if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 15 {
			t.Errorf("NumSVUsed = %v, want 15", ne.NumSVUsed)
		}
		if !ne.NumSVTracked.IsSet() || ne.NumSVTracked.Get() != 20 {
			t.Errorf("NumSVTracked = %v, want 20", ne.NumSVTracked)
		}
		if !ne.DiffAge.IsSet() || ne.DiffAge.Get() != gpsprot.Seconds(1.5) {
			t.Errorf("DiffAge = %v, want 1.5s", ne.DiffAge)
		}
		if !ne.RTCMRefBaseID.IsSet() || ne.RTCMRefBaseID.Get() != 123 {
			t.Errorf("RTCMRefBaseID = %v, want 123", ne.RTCMRefBaseID)
		}
		// GNSSUsed and BandsUsed are not set from BESTNAV signal mask
		// bytes (unreliable on Unicore firmware); only BESTSAT sets them.
		if ne.GNSSUsed != 0 {
			t.Errorf("GNSSUsed = %v, want 0 (BESTNAV should not set it)", ne.GNSSUsed)
		}
		if ne.BandsUsed != 0 {
			t.Errorf("BandsUsed = %v, want 0 (BESTNAV should not set it)", ne.BandsUsed)
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestEpochRTCMBaseIDNonOSR(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h
	// Epoch 1: BESTNAV with Single fix and a station ID.
	// Station ID should not appear in output since Single is not OSR.
	msg1 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.0,
			Lon:        8.0,
			Hgt:        400.0,
			StnID:      novmsg.StationID{'1', '2', '3', 0},
			NumSVs:     10,
			NumSolnSVs: 8,
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)
	// Flush with new epoch.
	msg2 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
		},
	})
	pp.dispatch(msg2, time.Unix(2, 0), TagBinary)
	for _, m := range h.msgs {
		if m.msgType != "navepoch" {
			continue
		}
		ne := m.msg.(*gpsprot.NavEpochMsg)
		if ne.RTCMRefBaseID.IsSet() {
			t.Errorf("RTCMRefBaseID = %d, want unset for non-OSR fix", ne.RTCMRefBaseID.Get())
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestEpochQualityNotComputed(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// BESTNAV with InsufficientObs should set FixLevelNone
	msg1 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
			PosType:    uncmsg.PosVelSingle,
			NumSVs:     5,
			NumSolnSVs: 0,
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)

	// Flush
	msg2 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.InsufficientObs,
		},
	})
	pp.dispatch(msg2, time.Unix(2, 0), TagBinary)

	for _, m := range h.msgs {
		if m.msgType != "navepoch" {
			continue
		}
		ne := m.msg.(*gpsprot.NavEpochMsg)
		if ne.FixLevel != gpsprot.FixLevelNone {
			t.Errorf("FixLevel = %v, want None", ne.FixLevel)
		}
		if ne.SolutionDim != 0 {
			t.Errorf("SolutionDim = %v, want 0", ne.SolutionDim)
		}
		if ne.Correction != 0 {
			t.Errorf("Correction = %v, want 0", ne.Correction)
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestBestSatGNSSBands(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h
	// BESTSAT with GPS L1+L2, GAL E1+E5a, BDS B1+B3
	msg1 := makeMsg(2350, 100000, &uncmsg.BestSat{
		BestSatInitChunk: uncmsg.BestSatInitChunk{NumEntries: 4},
		Sats: []uncmsg.BestSatEntry{
			{System: uncmsg.SatSysGPS, SatID: uncmsg.MakeSatID(1), SigMask: uncmsg.SigUsedGPSL1 | uncmsg.SigUsedGPSL2},
			{System: uncmsg.SatSysGPS, SatID: uncmsg.MakeSatID(3), SigMask: uncmsg.SigUsedGPSL1},
			{System: uncmsg.SatSysGALILEO, SatID: uncmsg.MakeSatID(5), SigMask: uncmsg.SigUsedGALE1 | uncmsg.SigUsedGALE5A},
			{System: uncmsg.SatSysBEIDOU, SatID: uncmsg.MakeSatID(10), SigMask: uncmsg.SigUsedBDSB1 | uncmsg.SigUsedBDSB3},
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)
	// Flush
	msg2 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{PSolStatus: uncmsg.InsufficientObs},
	})
	pp.dispatch(msg2, time.Unix(2, 0), TagBinary)
	for _, m := range h.msgs {
		if m.msgType != "navepoch" {
			continue
		}
		ne := m.msg.(*gpsprot.NavEpochMsg)
		wantGNSS := gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.GAL, gpsprot.BDS)
		if ne.GNSSUsed != wantGNSS {
			t.Errorf("GNSSUsed = %v, want %v", ne.GNSSUsed, wantGNSS)
		}
		wantBand := gpsprot.BandL1 | gpsprot.BandL2 | gpsprot.BandL5 | gpsprot.BandE6
		if ne.BandsUsed != wantBand {
			t.Errorf("BandsUsed = %v, want %v", ne.BandsUsed, wantBand)
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestBestSatWithBestNav(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h
	// BESTNAV arrives first (does not set GNSSUsed/BandsUsed)
	msg1 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.0, Lon: 8.0, Hgt: 400.0,
			LatSigma:   1.0, LonSigma: 1.0, HgtSigma: 2.0,
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)
	// BESTSAT arrives second with GPS+BDS
	msg2 := makeMsg(2350, 100000, &uncmsg.BestSat{
		BestSatInitChunk: uncmsg.BestSatInitChunk{NumEntries: 2},
		Sats: []uncmsg.BestSatEntry{
			{System: uncmsg.SatSysGPS, SatID: uncmsg.MakeSatID(1), SigMask: uncmsg.SigUsedGPSL1 | uncmsg.SigUsedGPSL5},
			{System: uncmsg.SatSysBEIDOU, SatID: uncmsg.MakeSatID(10), SigMask: uncmsg.SigUsedBDSB1},
		},
	})
	pp.dispatch(msg2, time.Unix(1, 0), TagBinary)
	// Flush
	msg3 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{PSolStatus: uncmsg.InsufficientObs},
	})
	pp.dispatch(msg3, time.Unix(2, 0), TagBinary)
	for _, m := range h.msgs {
		if m.msgType != "navepoch" {
			continue
		}
		ne := m.msg.(*gpsprot.NavEpochMsg)
		wantGNSS := gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.BDS)
		if ne.GNSSUsed != wantGNSS {
			t.Errorf("GNSSUsed = %v, want %v", ne.GNSSUsed, wantGNSS)
		}
		wantBand := gpsprot.BandL1 | gpsprot.BandL5
		if ne.BandsUsed != wantBand {
			t.Errorf("BandsUsed = %v, want %v", ne.BandsUsed, wantBand)
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestBestSatBeforeBestNav(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h
	// BESTSAT arrives first
	msg1 := makeMsg(2350, 100000, &uncmsg.BestSat{
		BestSatInitChunk: uncmsg.BestSatInitChunk{NumEntries: 1},
		Sats: []uncmsg.BestSatEntry{
			{System: uncmsg.SatSysGALILEO, SatID: uncmsg.MakeSatID(5), SigMask: uncmsg.SigUsedGALE1 | uncmsg.SigUsedGALE5B},
		},
	})
	pp.dispatch(msg1, time.Unix(1, 0), TagBinary)
	// BESTNAV arrives second (does not set GNSSUsed/BandsUsed)
	msg2 := makeMsg(2350, 100000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{
			PSolStatus: uncmsg.SolComputed,
			PosType:    uncmsg.PosVelSingle,
			Lat:        47.0, Lon: 8.0, Hgt: 400.0,
			LatSigma:   1.0, LonSigma: 1.0, HgtSigma: 2.0,
		},
	})
	pp.dispatch(msg2, time.Unix(1, 0), TagBinary)
	// Flush
	msg3 := makeMsg(2350, 101000, &uncmsg.BestNav{
		Pos: novmsg.Pos[uncmsg.SolStatus, uncmsg.PosVelType]{PSolStatus: uncmsg.InsufficientObs},
	})
	pp.dispatch(msg3, time.Unix(2, 0), TagBinary)
	for _, m := range h.msgs {
		if m.msgType != "navepoch" {
			continue
		}
		ne := m.msg.(*gpsprot.NavEpochMsg)
		wantGNSS := gpsprot.GNSSSetOf(gpsprot.GAL)
		if ne.GNSSUsed != wantGNSS {
			t.Errorf("GNSSUsed = %v, want %v", ne.GNSSUsed, wantGNSS)
		}
		wantBand := gpsprot.BandL1 | gpsprot.BandE5b
		if ne.BandsUsed != wantBand {
			t.Errorf("BandsUsed = %v, want %v", ne.BandsUsed, wantBand)
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestDefaultHandlerInvariant(t *testing.T) {
	mgr := gpsprot.NewNavEpochManager()
	bp := NewBinPacketProcessor(mgr)
	if bp.mh == nil {
		t.Error("BinPacketProcessor.mh should not be nil after construction")
	}
	ap := NewAsciiPacketProcessor(mgr)
	if ap.mh == nil {
		t.Error("AsciiPacketProcessor.mh should not be nil after construction")
	}
}

func TestSatellitesMerge(t *testing.T) {
	la := opt.Make(gpsprot.LookAngles{Azimuth: 100, Elevation: 45})
	tests := []struct {
		name     string
		sats     *gpsprot.SatellitesMsg
		bestSat  []uncmsg.BestSatEntry
		expected *gpsprot.SatellitesMsg
	}{
		{
			name:     "bestSat nil returns sats unchanged",
			sats:     &gpsprot.SatellitesMsg{Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedInvalid},
			bestSat:  nil,
			expected: &gpsprot.SatellitesMsg{Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedInvalid},
		},
		{
			name:     "sats nil returns nil",
			sats:     nil,
			bestSat:  []uncmsg.BestSatEntry{{System: uncmsg.SatSysGPS, SatID: uncmsg.MakeSatID(1), SigMask: uncmsg.SigUsedGPSL1}},
			expected: nil,
		},
		{
			name: "GPS L1+L2 used marks correct signals",
			sats: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedInvalid,
				SVs: []gpsprot.SVInfo{{
					ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5}, LookAngles: la,
					Signals: []gpsprot.SignalInfo{
						{ID: gpsprot.SigIDGPSL1CA, CN0: 40},
						{ID: gpsprot.SigIDGPSL2P, CN0: 35},
					},
				}},
			},
			bestSat: []uncmsg.BestSatEntry{
				{System: uncmsg.SatSysGPS, SatID: uncmsg.MakeSatID(5), SigMask: uncmsg.SigUsedGPSL1 | uncmsg.SigUsedGPSL2},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedSignal,
				SVs: []gpsprot.SVInfo{{
					ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5}, LookAngles: la, Used: true,
					Signals: []gpsprot.SignalInfo{
						{ID: gpsprot.SigIDGPSL1CA, CN0: 40, Used: true},
						{ID: gpsprot.SigIDGPSL2P, CN0: 35, Used: true},
					},
				}},
			},
		},
		{
			name: "L1 only does not mark L5 used",
			sats: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedInvalid,
				SVs: []gpsprot.SVInfo{{
					ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 13}, LookAngles: la,
					Signals: []gpsprot.SignalInfo{
						{ID: gpsprot.SigIDGPSL1CA, CN0: 42},
						{ID: gpsprot.SigIDGPSL5I, CN0: 38},
					},
				}},
			},
			bestSat: []uncmsg.BestSatEntry{
				{System: uncmsg.SatSysGPS, SatID: uncmsg.MakeSatID(13), SigMask: uncmsg.SigUsedGPSL1},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedSignal,
				SVs: []gpsprot.SVInfo{{
					ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 13}, LookAngles: la, Used: true,
					Signals: []gpsprot.SignalInfo{
						{ID: gpsprot.SigIDGPSL1CA, CN0: 42, Used: true},
						{ID: gpsprot.SigIDGPSL5I, CN0: 38, Used: false},
					},
				}},
			},
		},
		{
			name: "SV not in BESTSAT remains unused",
			sats: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedInvalid,
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5}, LookAngles: la,
						Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 40}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 20}, LookAngles: la,
						Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 30}}},
				},
			},
			bestSat: []uncmsg.BestSatEntry{
				{System: uncmsg.SatSysGPS, SatID: uncmsg.MakeSatID(5), SigMask: uncmsg.SigUsedGPSL1},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedSignal,
				SVs: []gpsprot.SVInfo{
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 5}, LookAngles: la, Used: true,
						Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 40, Used: true}}},
					{ID: gpsprot.SVID{GNSS: gpsprot.GPS, Num: 20}, LookAngles: la,
						Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGPSL1CA, CN0: 30}}},
				},
			},
		},
		{
			name: "Galileo E1+E5a",
			sats: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedInvalid,
				SVs: []gpsprot.SVInfo{{
					ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 5}, LookAngles: la,
					Signals: []gpsprot.SignalInfo{
						{ID: gpsprot.SigIDGALE1C, CN0: 45},
						{ID: gpsprot.SigIDGALE5aQ, CN0: 40},
					},
				}},
			},
			bestSat: []uncmsg.BestSatEntry{
				{System: uncmsg.SatSysGALILEO, SatID: uncmsg.MakeSatID(5), SigMask: uncmsg.SigUsedGALE1 | uncmsg.SigUsedGALE5A},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedSignal,
				SVs: []gpsprot.SVInfo{{
					ID: gpsprot.SVID{GNSS: gpsprot.GAL, Num: 5}, LookAngles: la, Used: true,
					Signals: []gpsprot.SignalInfo{
						{ID: gpsprot.SigIDGALE1C, CN0: 45, Used: true},
						{ID: gpsprot.SigIDGALE5aQ, CN0: 40, Used: true},
					},
				}},
			},
		},
		{
			name: "GLONASS PRN conversion",
			sats: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedInvalid,
				SVs: []gpsprot.SVInfo{{
					ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 5}, LookAngles: la,
					Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 42}},
				}},
			},
			bestSat: []uncmsg.BestSatEntry{
				// GLONASS PRN 42 = slot 5 (42-37)
				{System: uncmsg.SatSysGLONASS, SatID: uncmsg.MakeSatID(42), SigMask: uncmsg.SigUsedGLOL1},
			},
			expected: &gpsprot.SatellitesMsg{
				Tag: TagBinary, NativeMsgID: "SATSINFO", UsedValidity: gpsprot.SatelliteUsedSignal,
				SVs: []gpsprot.SVInfo{{
					ID: gpsprot.SVID{GNSS: gpsprot.GLO, Num: 5}, LookAngles: la, Used: true,
					Signals: []gpsprot.SignalInfo{{ID: gpsprot.SigIDGLOL1, CN0: 42, Used: true}},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := satellitesMerge(tt.sats, tt.bestSat)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("satellitesMerge() =\n%+v\nexpected\n%+v", result, tt.expected)
			}
		})
	}
}

// simpleSatsInfo creates a SATSINFO message with a few satellites for testing.
func simpleSatsInfo(week uint16, ms uint32) *uncmsg.Msg {
	return makeMsg(week, ms, &uncmsg.SatsInfo{
		SatsInfoInitChunk: uncmsg.SatsInfoInitChunk{SatNumber: 2},
		Sats: []uncmsg.SatsInfoSat{
			{SatsInfoSatChunk: uncmsg.SatsInfoSatChunk{PRN: 5, Azimuth: 100, Elevation: 45}, Freqs: []uncmsg.SatsInfoFreq{
				{SysStatus: uncmsg.SysGPS, FreqStatus: uncmsg.FreqGPSL1CA, SNR: 40},
			}},
			{SatsInfoSatChunk: uncmsg.SatsInfoSatChunk{PRN: 5, Azimuth: 150, Elevation: 60}, Freqs: []uncmsg.SatsInfoFreq{
				{SysStatus: uncmsg.SysGAL, FreqStatus: uncmsg.FreqGALE1C, SNR: 45},
			}},
		},
	})
}

// simpleBestSat creates a BESTSAT message for testing.
func simpleBestSat(week uint16, ms uint32) *uncmsg.Msg {
	return makeMsg(week, ms, &uncmsg.BestSat{
		BestSatInitChunk: uncmsg.BestSatInitChunk{NumEntries: 2},
		Sats: []uncmsg.BestSatEntry{
			{System: uncmsg.SatSysGPS, SatID: uncmsg.MakeSatID(5), SigMask: uncmsg.SigUsedGPSL1},
			{System: uncmsg.SatSysGALILEO, SatID: uncmsg.MakeSatID(5), SigMask: uncmsg.SigUsedGALE1},
		},
	})
}

func getSatsMsg(h *testMsgHandler) *gpsprot.SatellitesMsg {
	for _, m := range h.msgs {
		if m.msgType == "satellites" {
			return m.msg.(*gpsprot.SatellitesMsg)
		}
	}
	return nil
}

func TestSatsDispatchMerge(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h
	// Establish pattern: send both SATSINFO+BESTSAT for 3 epochs
	for i := range 3 {
		ms := uint32(100000 + i*1000)
		pp.dispatch(simpleSatsInfo(2350, ms), time.Unix(int64(i+1), 0), TagBinary)
		pp.dispatch(simpleBestSat(2350, ms), time.Unix(int64(i+1), 0), TagBinary)
	}
	h.msgs = nil
	// Epoch 4: SATSINFO should wait, then merge when BESTSAT arrives
	pp.dispatch(simpleSatsInfo(2350, 103000), time.Unix(10, 0), TagBinary)
	if getSatsMsg(h) != nil {
		t.Fatal("SATSINFO should wait for BESTSAT")
	}
	pp.dispatch(simpleBestSat(2350, 103000), time.Unix(10, 0), TagBinary)
	sm := getSatsMsg(h)
	if sm == nil {
		t.Fatal("expected merged SatellitesMsg")
	}
	if sm.UsedValidity != gpsprot.SatelliteUsedSignal {
		t.Errorf("UsedValidity = %v, want SatelliteUsedSignal", sm.UsedValidity)
	}
}

func TestSatsDispatchSatsInfoOnly(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h
	// Send SATSINFO in epoch 1 -- during first epochs, maybeFlushSats waits
	pp.dispatch(simpleSatsInfo(2350, 100000), time.Unix(1, 0), TagBinary)
	if getSatsMsg(h) != nil {
		t.Error("expected no SatellitesMsg during first epoch")
	}
	// Epoch boundary flushes buffered SATSINFO
	pp.dispatch(makeMsg(2350, 101000, &uncmsg.StaDOP{StaDOPFixed: uncmsg.StaDOPFixed{GDOP: 2.0}}), time.Unix(2, 0), TagBinary)
	sm := getSatsMsg(h)
	if sm == nil {
		t.Fatal("expected SatellitesMsg from FlushNavEpoch backstop")
	}
	if sm.UsedValidity != gpsprot.SatelliteUsedInvalid {
		t.Errorf("UsedValidity = %v, want SatelliteUsedInvalid", sm.UsedValidity)
	}
}

func TestSatsDispatchBestSatOnly(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h
	// Advance past 3 epochs with no sat messages
	ms := uint32(100000)
	for i := range 3 {
		pp.dispatch(makeMsg(2350, ms, &uncmsg.StaDOP{StaDOPFixed: uncmsg.StaDOPFixed{GDOP: 2.0}}), time.Unix(int64(i+1), 0), TagBinary)
		ms += 1000
	}
	h.msgs = nil
	// BESTSAT alone produces no SatellitesMsg (but sets GNSSUsed on NavEpochMsg)
	pp.dispatch(simpleBestSat(2350, ms), time.Unix(10, 0), TagBinary)
	if getSatsMsg(h) != nil {
		t.Error("expected no SatellitesMsg when only BESTSAT (no SATSINFO)")
	}
	if pp.curEpochMsg.GNSSUsed == 0 {
		t.Error("GNSSUsed should be set from BESTSAT")
	}
}
