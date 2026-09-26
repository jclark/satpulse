package nov

import (
	"encoding/hex"
	"reflect"
	"slices"
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

func (h *testMsgHandler) Time(msg *gpsprot.TimeMsg, tRead time.Time) {
	h.msgs = append(h.msgs, testHandledMsg{"time", msg, tRead})
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
	pp.dispatch(&common, &novmsg.BestPos{
		Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
			PSolStatus: novmsg.SolComputed,
			PosType:    novmsg.PosNarrowInt,
			Lat:        47.0,
			Lon:        8.0,
			Hgt:        400.0,
			LatSigma:   0.01,
			LonSigma:   0.01,
			HgtSigma:   0.02,
			DiffAge:    1.5,
			StnID:      novmsg.StationID{'1', '2', '3', 0},
			NumSVs:     20,
			NumSolnSVs: 15,
		},
		PosFlags: novmsg.PosFlags{
			GPSGLOBDS2Sig: 0x01, // GPS L1CA
			GalBDS3Sig:    0x01, // GAL E1
		},
	}, time.Unix(1, 0), TagBinary)

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

func TestDispatchEpochRTCMBaseIDNonOSR(t *testing.T) {
	var pp packetProcessor
	pp.mgr = gpsprot.NewNavEpochManager()
	h := &testMsgHandler{}
	pp.mh = h
	// Epoch 1: BESTPOS with Single fix and a station ID.
	// Station ID should not appear in output since Single is not OSR.
	common := makeCommon(2350, 100000)
	pp.dispatch(&common, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.SolComputed,
		PosType:    novmsg.PosSingle,
		Lat:        47.0,
		Lon:        8.0,
		Hgt:        400.0,
		StnID:      novmsg.StationID{'1', '2', '3', 0},
		NumSVs:     10,
		NumSolnSVs: 8,
	}}, time.Unix(1, 0), TagBinary)
	// Flush with new epoch.
	common2 := makeCommon(2350, 101000)
	pp.dispatch(&common2, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.InsufficientObs,
	}}, time.Unix(2, 0), TagBinary)
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

func TestByNavBestXYZNotDecoded(t *testing.T) {
	// BESTXYZA and BESTXYZB from a ByNav M10 in the same epoch.
	const ascii = "#BESTXYZA,COM1,0,99.9,FINESTEERING,2437,420882.000,00000000,0000,782;SOL_COMPUTED,NONE,0.0000,0.0000,0.0000,0.0000,0.0000,0.0000,SOL_COMPUTED,NONE,0.0000,0.0000,0.0000,0.0000,0.0000,0.0000,\"\",0.000,0.000,0.000,0,0,0,0,0,00,00,00*6f907c54\r\n"
	bin, err := hex.DecodeString("aa44121cf100002070000000c7b48509502616190000000000000e03000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008fd3ee5a")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		variant Variant
		expect  int // position and velocity messages published
	}{
		{VariantOEM7, 4},
		{VariantByNav, 0},
	}
	for _, tc := range tests {
		h := &testMsgHandler{}
		ap := NewAsciiPacketProcessor(gpsprot.NewNavEpochManager())
		ap.SetVariant(tc.variant)
		ap.SetMsgHandler(h)
		bp := NewBinPacketProcessor(gpsprot.NewNavEpochManager())
		bp.SetVariant(tc.variant)
		bp.SetMsgHandler(h)
		if _, err := ap.ProcessPacket(ascii, time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		if _, err := bp.ProcessPacket(string(bin), time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		got := 0
		for _, m := range h.msgs {
			if m.msgType == "posecef" || m.msgType == "velecef" {
				got++
			}
		}
		if got != tc.expect {
			t.Errorf("variant %d: %d position and velocity messages, want %d", tc.variant, got, tc.expect)
		}
	}
}

func TestDispatchPosGeoNativeMsgID(t *testing.T) {
	pos := novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{PSolStatus: novmsg.SolComputed, PosType: novmsg.PosSingle}
	sinoPos := novmsg.Pos[novmsg.SolStatus, novmsg.SinoPosType]{PSolStatus: novmsg.SolComputed, PosType: novmsg.PosSingle}
	tests := []struct {
		name   string
		body   novmsg.MsgBody
		expect string
	}{
		{"BESTPOS", &novmsg.BestPos{Pos: pos}, "BESTPOS"},
		{"BESTGNSSPOS", &novmsg.BestGNSSPos{Pos: pos}, "BESTGNSSPOS"},
		{"PSRPOS", &novmsg.PsrPos{Pos: pos}, "PSRPOS"},
		{"SinoGNSS BESTPOS", &novmsg.SinoBestPos{Pos: sinoPos}, "BESTPOS"},
		{"SinoGNSS PSRPOS", &novmsg.SinoPsrPos{Pos: sinoPos}, "PSRPOS"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var pp packetProcessor
			pp.mgr = gpsprot.NewNavEpochManager()
			h := &testMsgHandler{}
			pp.mh = h
			common := makeCommon(2350, 100000)
			if _, err := pp.dispatch(&common, tc.body, time.Unix(1, 0), TagBinary); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range h.msgs {
				if m.msgType == "posgeo" {
					got = append(got, m.msg.(*gpsprot.PosGeoMsg).NativeMsgID)
				}
			}
			if !reflect.DeepEqual(got, []string{tc.expect}) {
				t.Errorf("got %v, want [%s]", got, tc.expect)
			}
		})
	}
}

type testNativeHandler struct{ msgs []any }

func (h *testNativeHandler) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) error {
	h.msgs = append(h.msgs, msg)
	return nil
}

func TestByCheckOnlyForByNav(t *testing.T) {
	const ascii = "#BYCHECKA,COM1,0,99.9,FINESTEERING,2437,429495.000,00000000,0000,782;7843,2437,429495.000,1,1,1,1,1,1,1,1,1,1,1,1*8c0596c3\r\n"
	bin, err := hex.DecodeString("aa44121c20a500203c000000c7b48509d89299190000000000000e03a31e000085090000e0b6d148010000000100000001000000010000000100000001000000010000000100000001000000010000000100000001000000689c42f5")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		variant Variant
		expect  bool
	}{
		{VariantOEM7, false},
		{VariantSinoGNSS, false},
		{VariantUnicore, false},
		{VariantByNav, true},
	}
	for _, tc := range tests {
		ap := NewAsciiPacketProcessor(gpsprot.NewNavEpochManager())
		ap.SetVariant(tc.variant)
		bp := NewBinPacketProcessor(gpsprot.NewNavEpochManager())
		bp.SetVariant(tc.variant)
		h := &testNativeHandler{}
		ap.SetNativeMsgHandler(h)
		bp.SetNativeMsgHandler(h)
		// Other variants may reject the packet; only the decoded body matters.
		ap.ProcessPacket(ascii, time.Unix(1, 0))
		bp.ProcessPacket(string(bin), time.Unix(1, 0))
		var got []bool
		for _, m := range h.msgs {
			switch m := m.(type) {
			case *novmsg.Msg[novmsg.Port]:
				_, ok := m.Body.(*novmsg.ByCheck)
				got = append(got, ok)
			case *novmsg.Msg[novmsg.UnicorePort]:
				_, ok := m.Body.(*novmsg.ByCheck)
				got = append(got, ok)
			}
		}
		if (tc.expect && !reflect.DeepEqual(got, []bool{true, true})) || (!tc.expect && slices.Contains(got, true)) {
			t.Errorf("variant %d: decoded as BYCHECK %v, want %v", tc.variant, got, tc.expect)
		}
	}
}

func TestSinoGNSSAsciiPortName(t *testing.T) {
	const ascii = "#TIMEA,COM1,0,60.0,FINESTEERING,2437,548371.000,00000000,0000,1114;VALID,-4.078056768e-08,0.000000000e+00,-18.00002289175,2026,9,26,8,19,13000,VALID*ce2e7ae6\r\n"
	ap := NewAsciiPacketProcessor(gpsprot.NewNavEpochManager())
	ap.SetVariant(VariantSinoGNSS)
	h := &testMsgHandler{}
	ap.SetMsgHandler(h)
	if _, err := ap.ProcessPacket(ascii, time.Unix(1, 0)); err != nil {
		t.Fatalf("ProcessPacket: %v", err)
	}
	if !slices.ContainsFunc(h.msgs, func(m testHandledMsg) bool { _, ok := m.msg.(*gpsprot.TimeMsg); return ok }) {
		t.Errorf("no TimeMsg from SinoGNSS TIMEA with port COM1")
	}
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

func TestFlushResetsEpochStateForLateSameEpochPacket(t *testing.T) {
	mgr := gpsprot.NewNavEpochManager()
	bp := NewBinPacketProcessor(mgr)
	ap := NewAsciiPacketProcessor(mgr)
	h := &testMsgHandler{}
	bp.SetMsgHandler(h)
	ap.SetMsgHandler(h)

	epoch1 := makeCommon(2350, 100000)
	epoch2 := makeCommon(2350, 101000)

	bestPos := &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.SolComputed,
		PosType:    novmsg.PosSingle,
		Lat:        47.0,
		Lon:        8.0,
		Hgt:        400.0,
	}}
	if handled, err := bp.dispatch(&epoch1, bestPos, time.Unix(1, 0), TagBinary); err != nil || !handled {
		t.Fatalf("binary epoch 1 BESTPOS handled=%v err=%v", handled, err)
	}
	if handled, err := ap.dispatch(&epoch1, bestPos, time.Unix(1, 0), TagAscii); err != nil || !handled {
		t.Fatalf("ascii epoch 1 BESTPOS handled=%v err=%v", handled, err)
	}

	// Starting a new binary epoch flushes all active processors, including the
	// ASCII processor. A late ASCII packet from the previous epoch should start
	// a fresh accumulation instead of dereferencing a nil curEpochMsg.
	if _, err := bp.dispatch(&epoch2, &novmsg.BestPos{Pos: novmsg.Pos[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.InsufficientObs,
	}}, time.Unix(2, 0), TagBinary); err != nil {
		t.Fatalf("binary epoch 2 BESTPOS err=%v", err)
	}

	if handled, err := ap.dispatch(&epoch1, &novmsg.BestXYZ{XYZ: novmsg.XYZ[novmsg.SolStatus, novmsg.PosType]{
		PSolStatus: novmsg.SolComputed,
		PosType:    novmsg.PosSingle,
		PX:         1,
		PY:         2,
		PZ:         3,
	}}, time.Unix(3, 0), TagAscii); err != nil || !handled {
		t.Fatalf("late ascii epoch 1 BESTXYZ handled=%v err=%v", handled, err)
	}
	if ap.curEpoch == 0 || ap.curEpochMsg == nil {
		t.Fatalf("late ascii packet should restart epoch accumulation, got curEpoch=%d curEpochMsg=%v", ap.curEpoch, ap.curEpochMsg)
	}
}
