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
	if e.SolutionDim != gpsprot.SolutionDim3D {
		t.Errorf("epoch 1 FixDim = %v, want %v", e.SolutionDim, gpsprot.SolutionDim3D)
	}
	if !e.NumSVUsed.IsSet() || e.NumSVUsed.Get() != 12 {
		t.Errorf("epoch 1 NumSVUsed = %v, want 12", e.NumSVUsed)
	}

	// Third NAV-AUTO should flush the second epoch
	auto3 := &asbin.NavAuto{FixState: asbin.NavAutoFix3D, SatInUse: 10, SatInView: 20}
	processMsg(t, pp, auto3, time.Unix(3, 0))
	epochs = handler.navEpochMsgs()
	if len(epochs) != 2 {
		t.Fatalf("got %d NavEpochMsgs after third NAV-AUTO, want 2", len(epochs))
	}
	e2 := epochs[1]
	if e2.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("epoch 2 FixLevel = %v, want %v", e2.FixLevel, gpsprot.FixLevelCode)
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
		Lat:     473977640, Lon: 85255110, Height: 467890, HMSL: 420000,
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
		Lat:     473977641, Lon: 85255111, Height: 467891, HMSL: 420001,
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
	if e.SolutionDim != gpsprot.SolutionDim3D {
		t.Errorf("FixDim = %v, want %v", e.SolutionDim, gpsprot.SolutionDim3D)
	}
	if !e.NumSVUsed.IsSet() || e.NumSVUsed.Get() != 12 {
		t.Errorf("NumSVUsed = %v, want 12", e.NumSVUsed)
	}
	if !e.Acc.Hor.IsSet() {
		t.Error("Acc.Hor not set (should come from NAV-POSLLH)")
	}
}

// TestNavTimeUTCITowOffByOne reproduces the real hardware message ordering
// where NAV-TIMEUTC has iTOW 1ms less than NAV-POSLLH/DOP/VELNED.
// This must not split the epoch: the flushed epoch should contain both
// accuracy (from NAV-POSLLH) and quality (from NAV-AUTO).
func TestNavTimeUTCITowOffByOne(t *testing.T) {
	pp := NewPacketProcessor(gpsprot.NewNavEpochManager())
	handler := &testMsgHandler{}
	pp.SetMsgHandler(handler)

	// Epoch 1: real hardware ordering from TAU1201
	// NAV-POSLLH, NAV-DOP, NAV-VELNED all have iTOW=100000
	processMsg(t, pp, &asbin.NavPosLlh{
		NavITOW: asbin.NavITOW{ITow: 100000},
		Lat:     473977640, Lon: 85255110, Height: 467890, HMSL: 420000,
		HAcc: 5000, VAcc: 4000,
	}, time.Unix(1, 0))
	processMsg(t, pp, &asbin.NavDop{
		NavITOW: asbin.NavITOW{ITow: 100000},
		GDOP:    149, PDOP: 126, TDOP: 80, VDOP: 103, HDOP: 71,
	}, time.Unix(1, 3000000))
	processMsg(t, pp, &asbin.NavVelNed{
		NavITOW: asbin.NavITOW{ITow: 100000},
		GSpeed:  5, Heading: 5517413, SAcc: 4,
	}, time.Unix(1, 6000000))
	// NAV-TIMEUTC has iTOW=99999 (1ms less, as observed on real hardware)
	utc1 := &asbin.NavTimeUTC{NavITOW: asbin.NavITOW{ITow: 99999}}
	utc1.ValidFlag = asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagWknValid | asbin.NavTimeUTCFlagUtcValid
	utc1.Year, utc1.Month, utc1.Day = 2026, 3, 4
	utc1.Hour, utc1.Min, utc1.Sec = 12, 55, 31
	processMsg(t, pp, utc1, time.Unix(1, 9000000))
	// NAV-AUTO (no iTOW)
	processMsg(t, pp, &asbin.NavAuto{
		FixState: asbin.NavAutoFix3D, SatInUse: 25, SatInView: 26,
		Lat: 473977640, Lon: 85255110, Alt: 467890,
		PDOP: 77, HDOP: 77, VDOP: 115,
	}, time.Unix(1, 12000000))

	// Epoch 2: next second's messages flush epoch 1
	processMsg(t, pp, &asbin.NavPosLlh{
		NavITOW: asbin.NavITOW{ITow: 200000},
		Lat:     473977641, Lon: 85255111, Height: 467891, HMSL: 420001,
		HAcc: 5001, VAcc: 4001,
	}, time.Unix(2, 0))

	epochs := handler.navEpochMsgs()
	// Should be exactly 1 epoch, not 2
	if len(epochs) != 1 {
		t.Fatalf("got %d NavEpochMsgs, want 1 (epoch was split by NAV-TIMEUTC iTOW off-by-one)", len(epochs))
	}
	e := epochs[0]
	// Must have accuracy from NAV-POSLLH
	if !e.Acc.Hor.IsSet() {
		t.Error("Acc.Hor not set (should come from NAV-POSLLH)")
	}
	// Must have quality from NAV-AUTO
	if e.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("FixLevel = %v, want %v (should come from NAV-AUTO)", e.FixLevel, gpsprot.FixLevelCode)
	}
	if e.SolutionDim != gpsprot.SolutionDim3D {
		t.Errorf("FixDim = %v, want %v", e.SolutionDim, gpsprot.SolutionDim3D)
	}
	if !e.NumSVUsed.IsSet() || e.NumSVUsed.Get() != 25 {
		t.Errorf("NumSVUsed = %v, want 25", e.NumSVUsed)
	}
	// Must have full DOPs from NAV-DOP
	if !e.DOP.Geom.IsSet() {
		t.Error("DOP.Geom not set (should come from NAV-DOP)")
	}
	if !e.DOP.Time.IsSet() {
		t.Error("DOP.Time not set (should come from NAV-DOP)")
	}
}
