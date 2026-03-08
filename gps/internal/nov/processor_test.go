package nov

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
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

func makeCommon(week uint16, ms uint32) novmsg.CommonHdr {
	return novmsg.CommonHdr{
		Week:               week,
		MillisecondsOfWeek: novmsg.GPSec(ms),
	}
}

func TestDispatchPsrDop(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// Send BESTPOS to start an epoch
	common := makeCommon(2350, 100000)
	pp.dispatch(&common, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.SolComputed,
		PosType:    novmsg.PosSingle,
		Lat:        47.0,
		Lon:        8.0,
		Hgt:        400.0,
		LatSigma:   1.0,
		LonSigma:   1.0,
		HgtSigma:   2.0,
	}}, time.Unix(1, 0), TagBinary)

	// Send PSRDOP in same epoch
	handled, err := pp.dispatch(&common, &novmsg.PsrDop{PsrDopInitChunk: novmsg.PsrDopInitChunk{
		GDOP: 2.5,
		PDOP: 2.1,
		HDOP: 1.1,
		TDOP: 1.3,
	}}, time.Unix(1, 0), TagBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected PSRDOP to be handled")
	}

	// Trigger flush with new epoch
	common2 := makeCommon(2350, 101000)
	pp.dispatch(&common2, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.InsufficientObs,
	}}, time.Unix(2, 0), TagBinary)

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
		if !ne.DOP.Time.IsSet() || ne.DOP.Time.Get() != float64(float32(1.3)) {
			t.Errorf("DOP.Time = %v, want 1.3", ne.DOP.Time)
		}
		if ne.DOP.Vert.IsSet() {
			t.Error("DOP.Vert should not be set")
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestDispatchEpochQuality(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// Epoch 1: BESTPOS with RTK fixed (NARROW_INT=50)
	common := makeCommon(2350, 100000)
	pp.dispatch(&common, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus:    novmsg.SolComputed,
		PosType:       novmsg.PosNarrowInt,
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
	}}, time.Unix(1, 0), TagBinary)

	// Flush with new epoch
	common2 := makeCommon(2350, 101000)
	pp.dispatch(&common2, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.InsufficientObs,
	}}, time.Unix(2, 0), TagBinary)

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
		wantGNSS := gpsprot.GNSSSetOf(gpsprot.GPS, gpsprot.GAL)
		if ne.GNSSUsed != wantGNSS {
			t.Errorf("GNSSUsed = %v, want %v", ne.GNSSUsed, wantGNSS)
		}
		wantBand := gpsprot.BandL1
		if ne.BandsUsed != wantBand {
			t.Errorf("BandsUsed = %v, want %v", ne.BandsUsed, wantBand)
		}
		return
	}
	t.Fatal("no NavEpoch emitted")
}

func TestDispatchEpochQualityNotComputed(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h

	// BESTPOS with InsufficientObs should set FixLevelNone
	common := makeCommon(2350, 100000)
	pp.dispatch(&common, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.InsufficientObs,
		PosType:    novmsg.PosSingle,
		NumSVs:     5,
		NumSolnSVs: 0,
	}}, time.Unix(1, 0), TagBinary)

	// Flush
	common2 := makeCommon(2350, 101000)
	pp.dispatch(&common2, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.InsufficientObs,
	}}, time.Unix(2, 0), TagBinary)

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
