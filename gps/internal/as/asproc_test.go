package as

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

type testMsgHandler struct {
	gpsprot.DefaultHandler
	msgs []struct {
		msgType string
		msg     any
		tRead   time.Time
	}
}

func (h *testMsgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     any
		tRead   time.Time
	}{"time", msg, tRead})
}

func (h *testMsgHandler) NavEpoch(msg *gpsprot.NavEpochMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     any
		tRead   time.Time
	}{"navepoch", msg, tRead})
}

func (h *testMsgHandler) PosGeo(msg *gpsprot.PosGeoMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     any
		tRead   time.Time
	}{"posgeo", msg, tRead})
}

func (h *testMsgHandler) VelGeo(msg *gpsprot.VelGeoMsg, tRead time.Time) {
	h.msgs = append(h.msgs, struct {
		msgType string
		msg     any
		tRead   time.Time
	}{"velgeo", msg, tRead})
}

func (h *testMsgHandler) navEpochMsgs() []*gpsprot.NavEpochMsg {
	var r []*gpsprot.NavEpochMsg
	for _, m := range h.msgs {
		if m.msgType == "navepoch" {
			r = append(r, m.msg.(*gpsprot.NavEpochMsg))
		}
	}
	return r
}

func processMsg(t *testing.T, pp *PacketProcessor, m asbin.Msg, tRead time.Time) {
	t.Helper()
	packet, err := asbin.Serialize(m)
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}
	if _, err := pp.ProcessPacket(string(packet), tRead); err != nil {
		t.Fatalf("failed to process: %v", err)
	}
}

func TestNavEpochMsg(t *testing.T) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	handler := &testMsgHandler{}
	pp.SetMsgHandler(handler)

	// Send messages for epoch 1 - no NavEpochMsg should be emitted
	msg := &asbin.NavTimeUTC{NavITOW: asbin.NavITOW{ITow: 100000}}
	msg.ValidFlag = asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagWknValid | asbin.NavTimeUTCFlagUtcValid
	msg.Year = 2026
	msg.Month = 2
	msg.Day = 14
	msg.Sec = 1
	processMsg(t, pp, msg, time.Unix(1, 0))
	if epochs := handler.navEpochMsgs(); len(epochs) != 0 {
		t.Fatal("NavEpochMsg emitted before first epoch boundary")
	}

	// Send first message of epoch 2 - should trigger NavEpochMsg for epoch 1
	msg2 := &asbin.NavTimeUTC{NavITOW: asbin.NavITOW{ITow: 200000}}
	msg2.ValidFlag = asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagWknValid | asbin.NavTimeUTCFlagUtcValid
	msg2.Year = 2026
	msg2.Month = 2
	msg2.Day = 14
	msg2.Sec = 2
	processMsg(t, pp, msg2, time.Unix(2, 0))
	epochs := handler.navEpochMsgs()
	if len(epochs) != 1 {
		t.Fatalf("got %d NavEpochMsgs, want 1", len(epochs))
	}
	if epochs[0].Tag != Tag {
		t.Fatalf("NavEpochMsg.Tag = %q, want %q", epochs[0].Tag, Tag)
	}
	if epochs[0].StartTime != time.Unix(1, 0) {
		t.Fatalf("NavEpochMsg.StartTime = %v, want %v", epochs[0].StartTime, time.Unix(1, 0))
	}
}

func TestNavAutoEpoch(t *testing.T) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	handler := &testMsgHandler{}
	pp.SetMsgHandler(handler)

	// First NAV-AUTO should start an epoch (no prior epoch exists)
	auto1 := &asbin.NavAuto{
		FixState:  asbin.NavAutoFix3D,
		Lat:       473977640,
		Lon:       85255110,
		Alt:       467890,
		Speed:     0,
		SatInUse:  12,
		SatInView: 24,
		PDOP:      172,
		HDOP:      125,
		VDOP:      120,
	}
	processMsg(t, pp, auto1, time.Unix(1, 0))
	if epochs := handler.navEpochMsgs(); len(epochs) != 0 {
		t.Fatal("NavEpochMsg emitted after first NAV-AUTO (no boundary yet)")
	}

	// Second NAV-AUTO should flush the first epoch and start a new one
	auto2 := &asbin.NavAuto{
		FixState:  asbin.NavAutoFixDGNSS,
		Lat:       473977641,
		Lon:       85255111,
		Alt:       467891,
		Speed:     10,
		SatInUse:  14,
		SatInView: 26,
		PDOP:      150,
		HDOP:      100,
		VDOP:      110,
	}
	processMsg(t, pp, auto2, time.Unix(2, 0))
	epochs := handler.navEpochMsgs()
	if len(epochs) != 1 {
		t.Fatalf("got %d NavEpochMsgs after second NAV-AUTO, want 1", len(epochs))
	}
	e := epochs[0]
	if e.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("epoch 1 FixLevel = %v, want %v", e.FixLevel, gpsprot.FixLevelCode)
	}
	if e.FixDim != gpsprot.FixDim3D {
		t.Errorf("epoch 1 FixDim = %v, want %v", e.FixDim, gpsprot.FixDim3D)
	}
	if !e.NumSVUsed.IsSet() || e.NumSVUsed.Get() != 12 {
		t.Errorf("epoch 1 NumSVUsed = %v, want 12", e.NumSVUsed)
	}
	if e.StartTime != time.Unix(1, 0) {
		t.Errorf("epoch 1 StartTime = %v, want %v", e.StartTime, time.Unix(1, 0))
	}

	// Third NAV-AUTO should flush the second epoch
	auto3 := &asbin.NavAuto{FixState: asbin.NavAutoFix3D, SatInUse: 10, SatInView: 20}
	processMsg(t, pp, auto3, time.Unix(3, 0))
	epochs = handler.navEpochMsgs()
	if len(epochs) != 2 {
		t.Fatalf("got %d NavEpochMsgs after third NAV-AUTO, want 2", len(epochs))
	}
	e2 := epochs[1]
	if e2.FixLevel != gpsprot.FixLevelCodeCorrected {
		t.Errorf("epoch 2 FixLevel = %v, want %v", e2.FixLevel, gpsprot.FixLevelCodeCorrected)
	}
	if e2.Correction != gpsprot.CorrUsed {
		t.Errorf("epoch 2 Correction = %v, want %v", e2.Correction, gpsprot.CorrUsed)
	}
}

func TestNavAutoWithITOWEpoch(t *testing.T) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	handler := &testMsgHandler{}
	pp.SetMsgHandler(handler)

	// iTOW message starts epoch 1
	processMsg(t, pp, &asbin.NavPosLlh{
		NavITOW: asbin.NavITOW{ITow: 100000},
		Lat: 473977640, Lon: 85255110, Height: 467890, HMSL: 420000,
		HAcc: 5000, VAcc: 4000,
	}, time.Unix(1, 0))

	// NAV-AUTO in same epoch adds quality
	processMsg(t, pp, &asbin.NavAuto{
		FixState: asbin.NavAutoFix3D, SatInUse: 12, SatInView: 24,
		Lat: 473977640, Lon: 85255110, Alt: 467890,
		PDOP: 172, HDOP: 125, VDOP: 120,
	}, time.Unix(1, 100000000))

	// Second NAV-AUTO in same epoch should NOT start a new epoch
	// (iTOW epoch is still active, hadNavAuto prevents double-counting)
	// Actually it SHOULD start a new epoch since hadNavAuto is true
	// But the iTOW epoch should be flushed first

	// iTOW message starts epoch 2, flushing epoch 1
	processMsg(t, pp, &asbin.NavPosLlh{
		NavITOW: asbin.NavITOW{ITow: 200000},
		Lat: 473977641, Lon: 85255111, Height: 467891, HMSL: 420001,
		HAcc: 5001, VAcc: 4001,
	}, time.Unix(2, 0))

	epochs := handler.navEpochMsgs()
	if len(epochs) != 1 {
		t.Fatalf("got %d NavEpochMsgs, want 1", len(epochs))
	}
	e := epochs[0]
	// Should have both accuracy (from NAV-POSLLH) and quality (from NAV-AUTO)
	if e.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("FixLevel = %v, want %v", e.FixLevel, gpsprot.FixLevelCode)
	}
	if e.FixDim != gpsprot.FixDim3D {
		t.Errorf("FixDim = %v, want %v", e.FixDim, gpsprot.FixDim3D)
	}
	if !e.NumSVUsed.IsSet() || e.NumSVUsed.Get() != 12 {
		t.Errorf("NumSVUsed = %v, want 12", e.NumSVUsed)
	}
	if !e.Acc.Hor.IsSet() {
		t.Error("Acc.Hor not set (should come from NAV-POSLLH)")
	}
}
