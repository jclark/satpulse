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
	if !ok || e.gnss != gpsprot.GPS || e.band != gpsprot.BandL1 {
		t.Errorf("signal 0 = %+v", e)
	}
	e, ok = sbfSignalNumber(sbfbin.SigNumGalileoE5AltBOC)
	if !ok || e.gnss != gpsprot.GAL || e.band != (gpsprot.BandL5|gpsprot.BandE5b) {
		t.Errorf("E5AltBOC = %+v", e)
	}
}

// TestCombineChannelStatusAndMeasEpoch joins used and look-angle data from
// ChannelStatus to the MeasEpoch signal.
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
		Type2: [][]sbfbin.MeasEpochChannelType2{{}},
	}
	msg := satellitesCombine(cs, me)
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
		t.Errorf("CN0 = %d, want 60", sig.CN0)
	}
	if !sv.LookAngles.IsSet() {
		t.Error("LookAngles unset")
	}
}

func TestCombineMeasEpochType2(t *testing.T) {
	cs := &sbfbin.ChannelStatus{
		SatInfo:   []sbfbin.ChannelSatInfo{{SVID: 3, SVIDFull: 3}},
		StateInfo: [][]sbfbin.ChannelStateInfo{{}},
	}
	me := &sbfbin.MeasEpoch{
		Type1: []sbfbin.MeasEpochChannelType1{{SVID: 3, Type: sbfbin.MeasType(sbfbin.SigNumGPSL1CA), CN0: 180}},
		Type2: [][]sbfbin.MeasEpochChannelType2{{
			{Type: sbfbin.MeasType(sbfbin.SigNumGPSL5), CN0: 160},
		}},
	}
	msg := satellitesCombine(cs, me)
	if msg == nil || len(msg.SVs) != 1 {
		t.Fatalf("combine = %+v", msg)
	}
	sv := msg.SVs[0]
	if len(sv.Signals) != 2 {
		t.Fatalf("signals = %+v", sv.Signals)
	}
	sig, ok := findSignal(sv, gpsprot.SigIDGPSL5Q)
	if !ok || sig.CN0 != 50 {
		t.Errorf("L5 signal = %+v ok=%v, want CN0 50", sig, ok)
	}
}

func TestCombineMeasEpochDoesNotChangeChannelStatusSVSet(t *testing.T) {
	cs := &sbfbin.ChannelStatus{
		SatInfo: []sbfbin.ChannelSatInfo{
			{SVID: 1, SVIDFull: 1},
			{SVID: 71, SVIDFull: 71},
		},
		StateInfo: [][]sbfbin.ChannelStateInfo{{}, {{Antenna: 0, PVTStatus: sbfbin.SlotStatus(sbfbin.PVTStatusUsed)}}},
	}
	me := &sbfbin.MeasEpoch{
		Type1: []sbfbin.MeasEpochChannelType1{
			{SVID: 1, Type: sbfbin.MeasType(sbfbin.SigNumGPSL1CA), CN0: 180},
			{SVID: 2, Type: sbfbin.MeasType(sbfbin.SigNumGPSL1CA), CN0: 180},
			{SVID: 71, Type: sbfbin.MeasType(sbfbin.SigNumGalileoE5AltBOC), CN0: 180},
		},
		Type2: [][]sbfbin.MeasEpochChannelType2{{}, {}, {}},
	}
	msg := satellitesCombine(cs, me)
	if msg == nil || len(msg.SVs) != 2 {
		t.Fatalf("combine SVs = %+v, want G01 and E01", msg)
	}
	if msg.SVs[0].ID.String() != "G01" || len(msg.SVs[0].Signals) != 1 {
		t.Errorf("first SV = %+v, want G01 with one signal", msg.SVs[0])
	}
	if msg.SVs[1].ID.String() != "E01" || !msg.SVs[1].Used || msg.SVs[1].Signals == nil || len(msg.SVs[1].Signals) != 0 {
		t.Errorf("second SV = %+v, want used E01 with empty signals", msg.SVs[1])
	}
}

func TestCombineMeasEpochMainAntennaOnly(t *testing.T) {
	cs := &sbfbin.ChannelStatus{
		SatInfo:   []sbfbin.ChannelSatInfo{{SVID: 1, SVIDFull: 1}},
		StateInfo: [][]sbfbin.ChannelStateInfo{{}},
	}
	me := &sbfbin.MeasEpoch{
		Type1: []sbfbin.MeasEpochChannelType1{
			{SVID: 1, Type: sbfbin.MeasType(sbfbin.SigNumGPSL1CA), CN0: 180},
			{SVID: 1, Type: sbfbin.MeasType(1<<5) | sbfbin.MeasType(sbfbin.SigNumGPSL1CA), CN0: 220},
		},
		Type2: [][]sbfbin.MeasEpochChannelType2{
			{{Type: sbfbin.MeasType(1<<5) | sbfbin.MeasType(sbfbin.SigNumGPSL5), CN0: 220}},
			{{Type: sbfbin.MeasType(sbfbin.SigNumGPSL5), CN0: 160}},
		},
	}
	msg := satellitesCombine(cs, me)
	if msg == nil || len(msg.SVs) != 1 {
		t.Fatalf("combine = %+v", msg)
	}
	sv := msg.SVs[0]
	if len(sv.Signals) != 1 {
		t.Fatalf("signals = %+v, want main antenna L1 only", sv.Signals)
	}
	sig := sv.Signals[0]
	if sig.ID != gpsprot.SigIDGPSL1CA || sig.CN0 != 55 {
		t.Errorf("signal = %+v, want main antenna L1 CN0 55", sig)
	}
}

func TestCombineUsedRINEXCodesDistinguishBDSB1IAndB1C(t *testing.T) {
	cs := &sbfbin.ChannelStatus{
		SatInfo: []sbfbin.ChannelSatInfo{{SVID: 147, SVIDFull: 147}},
		StateInfo: [][]sbfbin.ChannelStateInfo{{{
			Antenna:   0,
			PVTStatus: sbfbin.SlotStatus(sbfbin.PVTStatusUsed),
		}}},
	}
	me := &sbfbin.MeasEpoch{
		Type1: []sbfbin.MeasEpochChannelType1{{SVID: 147, Type: sbfbin.MeasType(sbfbin.SigNumBeiDouB1C), CN0: 149}},
		Type2: [][]sbfbin.MeasEpochChannelType2{{
			{Type: sbfbin.MeasType(sbfbin.SigNumBeiDouB1I), CN0: 148},
		}},
	}
	msg := satellitesCombine(cs, me)
	if msg == nil || len(msg.SVs) != 1 {
		t.Fatalf("combine = %+v", msg)
	}
	sv := msg.SVs[0]
	b1c, ok := findSignal(sv, gpsprot.SigIDBDSB1CP)
	if !ok || b1c.Used {
		t.Errorf("B1C = %+v ok=%v, want unused", b1c, ok)
	}
	b1i, ok := findSignal(sv, gpsprot.SigIDBDSB1I)
	if !ok || !b1i.Used {
		t.Errorf("B1I = %+v ok=%v, want used", b1i, ok)
	}
	if !sv.Used {
		t.Error("SV.Used false, want true")
	}
}

func TestCombineChannelStatusOnlyEmptySignals(t *testing.T) {
	cs := &sbfbin.ChannelStatus{
		SatInfo: []sbfbin.ChannelSatInfo{{SVID: 1, SVIDFull: 1}},
		StateInfo: [][]sbfbin.ChannelStateInfo{{{
			Antenna:   0,
			PVTStatus: sbfbin.SlotStatus(sbfbin.PVTStatusUsed),
		}}},
	}
	msg := satellitesCombine(cs, nil)
	if msg == nil || msg.NativeMsgID != "ChannelStatus" || msg.UsedValidity != gpsprot.SatelliteUsedSV {
		t.Fatalf("combine = %+v", msg)
	}
	if len(msg.SVs) != 1 {
		t.Fatalf("SVs = %+v", msg.SVs)
	}
	sv := msg.SVs[0]
	if !sv.Used {
		t.Error("SV.Used false, want true")
	}
	if sv.Signals == nil || len(sv.Signals) != 0 {
		t.Fatalf("signals = %+v, want empty non-nil slice", sv.Signals)
	}
}

func TestCombineChannelStatusRealEmptySignals(t *testing.T) {
	cs := findBlock(t, "status.jsonl", "ChannelStatus").Params.(*sbfbin.ChannelStatus)
	msg := satellitesCombine(cs, nil)
	if msg == nil || len(msg.SVs) == 0 {
		t.Fatal("no SVs from real ChannelStatus")
	}
	if msg.UsedValidity != gpsprot.SatelliteUsedSV {
		t.Errorf("UsedValidity = %v", msg.UsedValidity)
	}
	for _, sv := range msg.SVs {
		if sv.Signals == nil || len(sv.Signals) != 0 {
			t.Errorf("%s signals = %+v, want empty non-nil slice", sv.ID, sv.Signals)
		}
	}
}

func TestCombineChannelStatusAndMeasEpochRealSVSet(t *testing.T) {
	cs := findBlock(t, "sats-sig.jsonl", "ChannelStatus").Params.(*sbfbin.ChannelStatus)
	me := findBlock(t, "sats-sig.jsonl", "MeasEpoch").Params.(*sbfbin.MeasEpoch)
	msg := satellitesCombine(cs, me)
	if msg == nil {
		t.Fatal("combine returned nil")
	}
	if len(msg.SVs) != len(cs.SatInfo) {
		t.Fatalf("combine SV count = %d, want ChannelStatus count %d", len(msg.SVs), len(cs.SatInfo))
	}
}

// TestChannelLookAnglesRequiresAzimuthAndElevation checks independent DNU
// values do not become valid zero-valued geometry.
func TestChannelLookAnglesRequiresAzimuthAndElevation(t *testing.T) {
	tests := []sbfbin.ChannelSatInfo{
		{AzimuthRiseSet: sbfbin.ChannelAzimuthDNU, Elevation: 30},
		{AzimuthRiseSet: 100, Elevation: sbfbin.ChannelElevationDNU},
	}
	for i := range tests {
		if _, ok := channelLookAngles(&tests[i]); ok {
			t.Errorf("test %d: channelLookAngles returned a value", i)
		}
	}
	si := sbfbin.ChannelSatInfo{AzimuthRiseSet: 100, Elevation: 30}
	la, ok := channelLookAngles(&si)
	if !ok || la.Get().Azimuth != 100 || la.Get().Elevation != 30 {
		t.Errorf("channelLookAngles = %+v, %v", la, ok)
	}
}

func findSignal(sv gpsprot.SVInfo, id gpsprot.SignalID) (gpsprot.SignalInfo, bool) {
	for _, sig := range sv.Signals {
		if sig.ID == id {
			return sig, true
		}
	}
	return gpsprot.SignalInfo{}, false
}
