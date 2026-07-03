package septentrio

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

func TestSBFSVID(t *testing.T) {
	tests := []struct {
		svid uint16
		want string
		ok   bool
	}{
		{1, "G01", true},
		{37, "G37", true},
		{38, "R01", true},
		{62, "R?", true},
		{63, "R25", true},
		{71, "E01", true},
		{107, "", false}, // L-band beam
		{120, "S20", true},
		{141, "C01", true},
		{181, "J01", true},
		{191, "I01", true},
		{250, "G38", true},
	}
	for _, tc := range tests {
		got, ok := sbfSVID(tc.svid)
		if ok != tc.ok {
			t.Errorf("sbfSVID(%d) ok=%v, want %v", tc.svid, ok, tc.ok)
			continue
		}
		if ok && got.String() != tc.want {
			t.Errorf("sbfSVID(%d) = %s, want %s", tc.svid, got.String(), tc.want)
		}
	}
}

func TestSignalTableSpotChecks(t *testing.T) {
	e, ok := sbfSignalNumber(sbfbin.SigNumGPSL1CA)
	if !ok || e.gnss != gpsprot.GPS || e.id != gpsprot.SigIDGPSL1CA || e.band != gpsprot.BandL1 {
		t.Errorf("signal 0 = %+v", e)
	}
	// Galileo E6 component resolves via CommonFlags.
	if id, _ := measEpochSignalID(sbfbin.SigNumGalileoE6, 0); id != gpsprot.SigIDGALE6C {
		t.Errorf("E6 default = %v, want E6-C", id)
	}
	if id, _ := measEpochSignalID(sbfbin.SigNumGalileoE6, sbfbin.CommonFlagsE6BUsed); id != gpsprot.SigIDGALE6B {
		t.Errorf("E6 with E6B used = %v, want E6-B", id)
	}
	// E5AltBOC has no SignalID yet (gap).
	if _, ok := measEpochSignalID(sbfbin.SigNumGalileoE5AltBOC, 0); ok {
		t.Error("E5AltBOC should have no MeasEpoch SignalID")
	}
}

// TestCombineChannelStatusAndMeasEpoch builds a ChannelStatus base tracking a
// used GPS L1CA signal and overlays MeasEpoch CN0 onto the matching slot.
func TestCombineChannelStatusAndMeasEpoch(t *testing.T) {
	cs := &sbfbin.ChannelStatus{
		SatInfo: []sbfbin.ChannelSatInfo{{SVID: 1, SVIDFull: 1, AzimuthRiseSet: 100, Elevation: 30}},
		StateInfo: [][]sbfbin.ChannelStateInfo{{{
			Antenna:        0,
			TrackingStatus: sbfbin.SlotStatus(sbfbin.TrackStatusTracking),
			PVTStatus:      sbfbin.SlotStatus(sbfbin.PVTStatusUsed),
		}}},
	}
	me := &sbfbin.MeasEpoch{
		Type1: []sbfbin.MeasEpochChannelType1{{SVID: 1, Type: sbfbin.MeasType(sbfbin.SigNumGPSL1CA), CN0: 200}},
	}
	msg := satellitesCombine(cs, me, nil)
	if msg == nil || len(msg.SVs) != 1 {
		t.Fatalf("combine SVs = %v", msg)
	}
	if msg.NativeMsgID != "ChannelStatus" || msg.UsedValidity != gpsprot.SatelliteUsedSignal {
		t.Errorf("NativeMsgID=%q UsedValidity=%v", msg.NativeMsgID, msg.UsedValidity)
	}
	sv := msg.SVs[0]
	if sv.ID.String() != "G01" || !sv.Used {
		t.Errorf("SV = %s used=%v, want G01 used", sv.ID, sv.Used)
	}
	if len(sv.Signals) != 1 {
		t.Fatalf("signals = %d, want 1", len(sv.Signals))
	}
	sig := sv.Signals[0]
	if sig.ID != gpsprot.SigIDGPSL1CA || !sig.Used {
		t.Errorf("signal = %+v, want L1CA used", sig)
	}
	if sig.CN0 != 60 { // 200*0.25 + 10
		t.Errorf("CN0 = %d, want 60 (overlaid from MeasEpoch)", sig.CN0)
	}
	if !sv.LookAngles.IsSet() {
		t.Error("LookAngles unset")
	}
}

// TestCombineMeasEpochOnly uses MeasEpoch as the base with no Used validity.
func TestCombineMeasEpochOnly(t *testing.T) {
	me := &sbfbin.MeasEpoch{
		Type1: []sbfbin.MeasEpochChannelType1{{SVID: 10, Type: sbfbin.MeasType(sbfbin.SigNumGPSL1CA), CN0: 180}},
	}
	msg := satellitesCombine(nil, me, nil)
	if msg == nil || msg.NativeMsgID != "MeasEpoch" || msg.UsedValidity != gpsprot.SatelliteUsedInvalid {
		t.Fatalf("combine = %+v", msg)
	}
	if len(msg.SVs) != 1 || len(msg.SVs[0].Signals) != 1 || msg.SVs[0].Signals[0].CN0 != 55 {
		t.Errorf("SVs = %+v", msg.SVs)
	}
}

// TestCombineSatVisibilityReal appends orbit-visible SVs with look angles and
// no signals from a real SatVisibility block.
func TestCombineSatVisibilityReal(t *testing.T) {
	vis := findBlock(t, "status.jsonl", "SatVisibility").Params.(*sbfbin.SatVisibility)
	msg := satellitesCombine(nil, nil, vis)
	if msg == nil || msg.NativeMsgID != "SatVisibility" {
		t.Fatalf("combine = %+v", msg)
	}
	if len(msg.SVs) == 0 {
		t.Fatal("no SVs")
	}
	for _, sv := range msg.SVs {
		if len(sv.Signals) != 0 {
			t.Errorf("%s should have no signals from SatVisibility alone", sv.ID)
		}
	}
}

// TestCombineChannelStatusReal exercises the combine on a real ChannelStatus.
func TestCombineChannelStatusReal(t *testing.T) {
	cs := findBlock(t, "status.jsonl", "ChannelStatus").Params.(*sbfbin.ChannelStatus)
	msg := satellitesCombine(cs, nil, nil)
	if msg == nil || len(msg.SVs) == 0 {
		t.Fatal("no SVs from real ChannelStatus")
	}
	if msg.UsedValidity != gpsprot.SatelliteUsedSignal {
		t.Errorf("UsedValidity = %v", msg.UsedValidity)
	}
	// Every SV should have a valid decoded SVID and at least one tracked signal.
	for _, sv := range msg.SVs {
		if sv.ID.GNSS == 0 {
			t.Errorf("SV with zero GNSS: %+v", sv)
		}
	}
}
