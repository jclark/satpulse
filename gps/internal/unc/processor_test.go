package unc

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
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
	if epochMsg.StartTime != time.Unix(1, 0) {
		t.Errorf("NavEpoch.StartTime = %v, want %v", epochMsg.StartTime, time.Unix(1, 0))
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
		if ne.FixDim != gpsprot.FixDim3D {
			t.Errorf("FixDim = %v, want 3D", ne.FixDim)
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
		wantSig := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGALE1)
		if ne.SignalsUsed != wantSig {
			t.Errorf("SignalsUsed = %v, want %v", ne.SignalsUsed, wantSig)
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
		if ne.FixDim != 0 {
			t.Errorf("FixDim = %v, want 0", ne.FixDim)
		}
		if ne.Correction != 0 {
			t.Errorf("Correction = %v, want 0", ne.Correction)
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
