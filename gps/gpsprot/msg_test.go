package gpsprot

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

func TestGNSSSet(t *testing.T) {
	// Create a new GNSSSet
	set := GNSSSetOf(GPS, GLO)

	// Check that Contains works properly for values in the set
	if !set.Contains(GPS) {
		t.Errorf("expected set to contain GPS")
	}
	if !set.Contains(GLO) {
		t.Errorf("expected set to contain GLONASS")
	}

	// Check that Contains works properly for values not in the set
	if set.Contains(BDS) {
		t.Errorf("expected set to not contain BeiDou")
	}
	if set.Contains(GAL) {
		t.Errorf("expected set to not contain Galileo")
	}

	// Check that Contains works properly for the 0 value
	if set.Contains(0) {
		t.Errorf("expected set to not contain 0")
	}

	// Check that Items returns the correct values
	items := set.Items()
	if len(items) != 2 {
		t.Errorf("expected Items to return 2 items, got %d", len(items))
	}
	if items[0] != GPS {
		t.Errorf("expected first item to be GPS, got %v", items[0])
	}
	if items[1] != GLO {
		t.Errorf("expected second item to be GLONASS, got %v", items[1])
	}
}

func TestGNSSSetMarshalJSON(t *testing.T) {
	gnssSet := GNSSSetOf(GPS) | GNSSSetOf(GAL)

	marshaledJSON, err := gnssSet.MarshalJSON()

	if err != nil {
		t.Errorf("MarshalJSON returned error: %v", err)
	}

	expectedJSON := `["GPS","GAL"]`
	if string(marshaledJSON) != expectedJSON {
		t.Errorf("Expected marshaled JSON to be %v, got %v", expectedJSON, string(marshaledJSON))
	}
}

func TestPosGeoMergeHigherOverwrites(t *testing.T) {
	m := &PosGeoMsg{
		LatLon:   [2]Angle{10 * Degrees, 20 * Degrees},
		Height:   opt.Make(100 * Meter),
		Priority: PriGenericLow,
		Tag:      "NMEA",
	}
	other := &PosGeoMsg{
		LatLon:    [2]Angle{11 * Degrees, 21 * Degrees},
		HeightMSL: opt.Make(95 * Meter),
		Priority:  PriVendorHigh,
		Tag:       "UBX",
	}
	m.Merge(other)
	if m.LatLon[0] != 11*Degrees {
		t.Errorf("LatLon[0] = %v, want %v", m.LatLon[0], 11*Degrees)
	}
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	if m.Priority != PriVendorHigh {
		t.Errorf("Priority = %v, want %v", m.Priority, PriVendorHigh)
	}
	// Height kept (other did not set it)
	if !m.Height.IsSet() || m.Height.Get() != 100*Meter {
		t.Errorf("Height = %v, want 100m set", m.Height)
	}
	// HeightMSL filled from other
	if !m.HeightMSL.IsSet() || m.HeightMSL.Get() != 95*Meter {
		t.Errorf("HeightMSL = %v, want 95m set", m.HeightMSL)
	}
}

func TestPosGeoMergeLowerFillsOnly(t *testing.T) {
	m := &PosGeoMsg{
		LatLon:   [2]Angle{10 * Degrees, 20 * Degrees},
		Height:   opt.Make(100 * Meter),
		Priority: PriVendorHigh,
		Tag:      "UBX",
	}
	other := &PosGeoMsg{
		LatLon:    [2]Angle{11 * Degrees, 21 * Degrees},
		Height:    opt.Make(200 * Meter),
		HeightMSL: opt.Make(95 * Meter),
		Priority:  PriGenericLow,
		Tag:       "NMEA",
	}
	m.Merge(other)
	// LatLon NOT overwritten (lower priority)
	if m.LatLon[0] != 10*Degrees {
		t.Errorf("LatLon[0] = %v, want %v", m.LatLon[0], 10*Degrees)
	}
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	if m.Priority != PriVendorHigh {
		t.Errorf("Priority = %v, want %v", m.Priority, PriVendorHigh)
	}
	// Height NOT overwritten (already set, lower priority)
	if m.Height.Get() != 100*Meter {
		t.Errorf("Height = %v, want 100m", m.Height.Get())
	}
	// HeightMSL filled (was unset)
	if !m.HeightMSL.IsSet() || m.HeightMSL.Get() != 95*Meter {
		t.Errorf("HeightMSL = %v, want 95m set", m.HeightMSL)
	}
}

func TestPosGeoMergeEqualOverwrites(t *testing.T) {
	m := &PosGeoMsg{
		LatLon:   [2]Angle{10 * Degrees, 20 * Degrees},
		Priority: PriVendorLow,
	}
	other := &PosGeoMsg{
		LatLon:   [2]Angle{11 * Degrees, 21 * Degrees},
		Priority: PriVendorLow,
		Tag:      "UBX2",
	}
	m.Merge(other)
	if m.LatLon[0] != 11*Degrees {
		t.Errorf("LatLon[0] = %v, want %v", m.LatLon[0], 11*Degrees)
	}
	if m.Tag != "UBX2" {
		t.Errorf("Tag = %v, want UBX2", m.Tag)
	}
}

func TestVelGeoMergeHigherOverwrites(t *testing.T) {
	m := &VelGeoMsg{
		GroundSpeed: opt.Make(5 * MeterPerSecond),
		Course:      opt.Make(90 * Degrees),
		Priority:    PriGenericLow,
		Tag:         "NMEA",
	}
	other := &VelGeoMsg{
		VelNED:      opt.Make([3]Speed{1, 2, 3}),
		GroundSpeed: opt.Make(6 * MeterPerSecond),
		Priority:    PriVendorHigh,
		Tag:         "UBX",
	}
	m.Merge(other)
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	// GroundSpeed overwritten
	if m.GroundSpeed.Get() != 6*MeterPerSecond {
		t.Errorf("GroundSpeed = %v, want 6", m.GroundSpeed.Get())
	}
	// VelNED filled from other
	if !m.VelNED.IsSet() {
		t.Error("VelNED not set")
	}
	// Course kept (other did not set it)
	if !m.Course.IsSet() || m.Course.Get() != 90*Degrees {
		t.Errorf("Course = %v, want 90deg", m.Course)
	}
}

func TestVelGeoMergeLowerFillsOnly(t *testing.T) {
	m := &VelGeoMsg{
		GroundSpeed: opt.Make(5 * MeterPerSecond),
		Priority:    PriVendorHigh,
	}
	other := &VelGeoMsg{
		GroundSpeed: opt.Make(6 * MeterPerSecond),
		Course:      opt.Make(90 * Degrees),
		Priority:    PriGenericLow,
	}
	m.Merge(other)
	// GroundSpeed NOT overwritten
	if m.GroundSpeed.Get() != 5*MeterPerSecond {
		t.Errorf("GroundSpeed = %v, want 5", m.GroundSpeed.Get())
	}
	// Course filled
	if !m.Course.IsSet() || m.Course.Get() != 90*Degrees {
		t.Errorf("Course = %v, want 90deg", m.Course)
	}
}

func TestPosECEFMerge(t *testing.T) {
	m := &PosECEFMsg{
		Pos:      Point3D{1, 2, 3},
		Priority: PriVendorLow,
		Tag:      "A",
	}
	other := &PosECEFMsg{
		Pos:      Point3D{4, 5, 6},
		Priority: PriVendorHigh,
		Tag:      "B",
	}
	m.Merge(other)
	if m.Pos != (Point3D{4, 5, 6}) {
		t.Errorf("Pos = %v, want {4,5,6}", m.Pos)
	}
	if m.Tag != "B" {
		t.Errorf("Tag = %v, want B", m.Tag)
	}
	// Lower priority does not overwrite
	low := &PosECEFMsg{Pos: Point3D{7, 8, 9}, Priority: PriGenericLow, Tag: "C"}
	m.Merge(low)
	if m.Pos != (Point3D{4, 5, 6}) {
		t.Errorf("Pos = %v, want {4,5,6} (lower priority)", m.Pos)
	}
}

func TestVelECEFMerge(t *testing.T) {
	m := &VelECEFMsg{
		Vel:      [3]Speed{1, 2, 3},
		Priority: PriVendorLow,
	}
	other := &VelECEFMsg{
		Vel:      [3]Speed{4, 5, 6},
		Priority: PriVendorHigh,
	}
	m.Merge(other)
	if m.Vel != [3]Speed{4, 5, 6} {
		t.Errorf("Vel = %v, want {4,5,6}", m.Vel)
	}
	// Lower priority does not overwrite
	low := &VelECEFMsg{Vel: [3]Speed{7, 8, 9}, Priority: PriGenericLow}
	m.Merge(low)
	if m.Vel != [3]Speed{4, 5, 6} {
		t.Errorf("Vel = %v, want {4,5,6} (lower priority)", m.Vel)
	}
}

func TestTimeMsgMergeHigher(t *testing.T) {
	utc := ptime.UTCTime{TimeOfDay: 100 * time.Second}
	offset := 1.5
	m := &TimeMsg{
		TAITime:     1000,
		Accuracy:    time.Millisecond,
		UTCTime:     &utc,
		PulseOffset: &offset,
		Priority:    PriGenericLow,
		Tag:         "NMEA",
	}
	other := &TimeMsg{
		TAITime:  2000,
		Accuracy: time.Microsecond,
		GNSS:     GPS,
		Priority: PriVendorHigh,
		Tag:      "UBX",
	}
	m.Merge(other)
	if m.TAITime != 2000 {
		t.Errorf("TAITime = %v, want 2000", m.TAITime)
	}
	if m.Accuracy != time.Microsecond {
		t.Errorf("Accuracy = %v, want 1us", m.Accuracy)
	}
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	// UTCTime preserved (other had nil, should not clear)
	if m.UTCTime == nil || m.UTCTime.TimeOfDay != 100*time.Second {
		t.Errorf("UTCTime cleared unexpectedly")
	}
	// PulseOffset preserved
	if m.PulseOffset == nil || *m.PulseOffset != 1.5 {
		t.Errorf("PulseOffset cleared unexpectedly")
	}
}

func TestTimeMsgMergeHigherOverwritesPointers(t *testing.T) {
	utc1 := ptime.UTCTime{TimeOfDay: 100 * time.Second}
	utc2 := ptime.UTCTime{TimeOfDay: 200 * time.Second}
	m := &TimeMsg{
		UTCTime:  &utc1,
		Priority: PriGenericLow,
	}
	other := &TimeMsg{
		UTCTime:  &utc2,
		Priority: PriVendorHigh,
	}
	m.Merge(other)
	if m.UTCTime.TimeOfDay != 200*time.Second {
		t.Errorf("UTCTime.TimeOfDay = %v, want 200s", m.UTCTime.TimeOfDay)
	}
}

func TestTimeMsgMergeLowerFillsPointers(t *testing.T) {
	utc := ptime.UTCTime{TimeOfDay: 100 * time.Second}
	offset := 2.5
	m := &TimeMsg{
		TAITime:  1000,
		Priority: PriVendorHigh,
		Tag:      "UBX",
	}
	other := &TimeMsg{
		TAITime:     2000,
		UTCTime:     &utc,
		PulseOffset: &offset,
		Priority:    PriGenericLow,
		Tag:         "NMEA",
	}
	m.Merge(other)
	// Non-optional fields NOT overwritten
	if m.TAITime != 1000 {
		t.Errorf("TAITime = %v, want 1000", m.TAITime)
	}
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	// Pointer fields filled
	if m.UTCTime == nil || m.UTCTime.TimeOfDay != 100*time.Second {
		t.Errorf("UTCTime not filled, got %v", m.UTCTime)
	}
	if m.PulseOffset == nil || *m.PulseOffset != 2.5 {
		t.Errorf("PulseOffset not filled, got %v", m.PulseOffset)
	}
}

func TestTimeMsgMergeLowerDoesNotOverwritePointers(t *testing.T) {
	utc1 := ptime.UTCTime{TimeOfDay: 100 * time.Second}
	utc2 := ptime.UTCTime{TimeOfDay: 200 * time.Second}
	m := &TimeMsg{
		UTCTime:  &utc1,
		Priority: PriVendorHigh,
	}
	other := &TimeMsg{
		UTCTime:  &utc2,
		Priority: PriGenericLow,
	}
	m.Merge(other)
	if m.UTCTime.TimeOfDay != 100*time.Second {
		t.Errorf("UTCTime.TimeOfDay = %v, want 100s (lower priority should not overwrite)", m.UTCTime.TimeOfDay)
	}
}

func TestNavEpochAccumFirstStored(t *testing.T) {
	var a NavEpochAccum
	now := time.Now()
	p := &PosGeoMsg{LatLon: [2]Angle{1, 2}, Priority: PriGenericLow}
	a.PosGeo(p, now)
	if a.Bundle.PosGeo != p {
		t.Fatal("first PosGeo should be stored directly")
	}
}

func TestNavEpochAccumMerge(t *testing.T) {
	var a NavEpochAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{
		LatLon:   [2]Angle{1 * Degrees, 2 * Degrees},
		Priority: PriGenericLow,
		Tag:      "NMEA",
	}, now)
	a.PosGeo(&PosGeoMsg{
		LatLon:   [2]Angle{3 * Degrees, 4 * Degrees},
		Height:   opt.Make(50 * Meter),
		Priority: PriVendorHigh,
		Tag:      "UBX",
	}, now)
	if a.Bundle.PosGeo.LatLon[0] != 3*Degrees {
		t.Errorf("LatLon[0] = %v, want %v", a.Bundle.PosGeo.LatLon[0], 3*Degrees)
	}
	if a.Bundle.PosGeo.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", a.Bundle.PosGeo.Tag)
	}
	if !a.Bundle.PosGeo.Height.IsSet() {
		t.Error("Height not set after merge")
	}
}

func TestNavEpochAccumClear(t *testing.T) {
	var a NavEpochAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{1, 2}}, now)
	a.VelGeo(&VelGeoMsg{GroundSpeed: opt.Make(Speed(5))}, now)
	a.Time(&TimeMsg{TAITime: 1000}, now)
	a.NavEpoch(&NavEpochMsg{}, now)
	if a.Bundle.PosGeo != nil {
		t.Error("PosGeo not cleared")
	}
	if a.Bundle.VelGeo != nil {
		t.Error("VelGeo not cleared")
	}
	if a.Bundle.Time != nil {
		t.Error("Time not cleared")
	}
}

func TestNavEpochAccumIndependent(t *testing.T) {
	var a NavEpochAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{1, 2}, Priority: PriGenericLow}, now)
	a.VelGeo(&VelGeoMsg{GroundSpeed: opt.Make(Speed(5)), Priority: PriVendorLow}, now)
	a.PosECEF(&PosECEFMsg{Pos: Point3D{10, 20, 30}, Priority: PriVendorHigh}, now)
	a.VelECEF(&VelECEFMsg{Vel: [3]Speed{1, 2, 3}, Priority: PriGenericHigh}, now)
	if a.Bundle.PosGeo == nil || a.Bundle.VelGeo == nil || a.Bundle.PosECEF == nil || a.Bundle.VelECEF == nil {
		t.Error("not all message types accumulated")
	}
}

func TestNavEpochAccumAfterClear(t *testing.T) {
	var a NavEpochAccum
	now := time.Now()
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{1, 2}, Priority: PriGenericLow, Tag: "old"}, now)
	a.NavEpoch(&NavEpochMsg{}, now)
	a.PosGeo(&PosGeoMsg{LatLon: [2]Angle{3, 4}, Priority: PriVendorHigh, Tag: "new"}, now)
	if a.Bundle.PosGeo.Tag != "new" {
		t.Errorf("Tag = %v, want new (fresh after clear)", a.Bundle.PosGeo.Tag)
	}
}

func TestAccuracyMergeHigherOverwrites(t *testing.T) {
	a := Accuracy{
		Pos:   opt.Make(10 * Meter),
		Hor:   opt.Make(5 * Meter),
		Speed: opt.Make(2 * MeterPerSecond),
	}
	b := Accuracy{
		Pos:         opt.Make(20 * Meter),
		Vert:        opt.Make(8 * Meter),
		GroundSpeed: opt.Make(3 * MeterPerSecond),
	}
	a.Merge(&b, PriGenericHigh, PriVendorLow)
	// Higher priority overwrites Pos
	if a.Pos.Get() != 20*Meter {
		t.Errorf("Pos = %v, want 20m", a.Pos.Get())
	}
	// Hor kept (src didn't set it)
	if a.Hor.Get() != 5*Meter {
		t.Errorf("Hor = %v, want 5m", a.Hor.Get())
	}
	// Vert filled from src
	if a.Vert.Get() != 8*Meter {
		t.Errorf("Vert = %v, want 8m", a.Vert.Get())
	}
	// Speed kept (src didn't set it)
	if a.Speed.Get() != 2*MeterPerSecond {
		t.Errorf("Speed = %v, want 2m/s", a.Speed.Get())
	}
	// GroundSpeed filled from src
	if a.GroundSpeed.Get() != 3*MeterPerSecond {
		t.Errorf("GroundSpeed = %v, want 3m/s", a.GroundSpeed.Get())
	}
}

func TestAccuracyMergeLowerFillsOnly(t *testing.T) {
	a := Accuracy{
		Pos: opt.Make(10 * Meter),
	}
	b := Accuracy{
		Pos:  opt.Make(20 * Meter),
		Vert: opt.Make(8 * Meter),
	}
	a.Merge(&b, PriVendorLow, PriGenericHigh)
	// Lower priority does not overwrite Pos
	if a.Pos.Get() != 10*Meter {
		t.Errorf("Pos = %v, want 10m", a.Pos.Get())
	}
	// Vert filled (was unset)
	if a.Vert.Get() != 8*Meter {
		t.Errorf("Vert = %v, want 8m", a.Vert.Get())
	}
}

func TestMergeNavEpochNils(t *testing.T) {
	msg := &NavEpochMsg{Tag: "UBX"}
	m, p := MergeNavEpoch(nil, 0, msg, PriVendorLow)
	if m != msg || p != PriVendorLow {
		t.Errorf("MergeNavEpoch(nil, msg) = %v, %v; want msg, PriVendorLow", m, p)
	}
	m, p = MergeNavEpoch(msg, PriVendorLow, nil, 0)
	if m != msg || p != PriVendorLow {
		t.Errorf("MergeNavEpoch(msg, nil) = %v, %v; want msg, PriVendorLow", m, p)
	}
	m, _ = MergeNavEpoch(nil, 0, nil, 0)
	if m != nil {
		t.Errorf("MergeNavEpoch(nil, nil) = %v; want nil", m)
	}
}

func TestMergeNavEpochTagFromHigherPriority(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Millisecond)
	a := &NavEpochMsg{Tag: "NMEA", StartTime: t1, Acc: Accuracy{Hor: opt.Make(5 * Meter)}}
	b := &NavEpochMsg{Tag: "UBX", StartTime: t0, Acc: Accuracy{Pos: opt.Make(10 * Meter)}}
	m, _ := MergeNavEpoch(a, PriGenericHigh, b, PriVendorLow)
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX (higher priority)", m.Tag)
	}
	if m.StartTime != t0 {
		t.Errorf("StartTime = %v, want %v (earliest)", m.StartTime, t0)
	}
	if m.Acc.Hor.Get() != 5*Meter {
		t.Errorf("Acc.Hor = %v, want 5m (filled from NMEA)", m.Acc.Hor.Get())
	}
	if m.Acc.Pos.Get() != 10*Meter {
		t.Errorf("Acc.Pos = %v, want 10m (from UBX)", m.Acc.Pos.Get())
	}
}

// testFlusher is a fake EpochFlusher for testing NavEpochManager.
type testFlusher struct {
	msg *NavEpochMsg
	pri MsgPriority
	mh  MsgHandler
}

func (f *testFlusher) FlushNavEpoch(tRead time.Time) (*NavEpochMsg, MsgPriority, MsgHandler) {
	msg := f.msg
	f.msg = nil
	return msg, f.pri, f.mh
}

// epochRecorder records NavEpoch calls.
type epochRecorder struct {
	DefaultHandler
	epochs []NavEpochMsg
	tReads []time.Time
}

func (r *epochRecorder) NavEpoch(msg *NavEpochMsg, tRead time.Time) {
	r.epochs = append(r.epochs, *msg)
	r.tReads = append(r.tReads, tRead)
}

func TestNavEpochManagerSingleProtocol(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)
	f := &testFlusher{pri: PriVendorLow, mh: rec}
	// First epoch start: f added to active set, no flush
	mgr.EpochStarted(f, t0)
	if len(rec.epochs) != 0 {
		t.Fatalf("expected no epochs after first EpochStarted, got %d", len(rec.epochs))
	}
	// Set up what will be flushed
	f.msg = &NavEpochMsg{Tag: "UBX", StartTime: t0, Acc: Accuracy{Pos: opt.Make(10 * Meter)}}
	// Second epoch start: triggers flush of first epoch
	mgr.EpochStarted(f, t1)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after second EpochStarted, got %d", len(rec.epochs))
	}
	if rec.epochs[0].Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", rec.epochs[0].Tag)
	}
	if rec.tReads[0] != t1 {
		t.Errorf("tRead = %v, want %v", rec.tReads[0], t1)
	}
	// Set up second epoch data
	f.msg = &NavEpochMsg{Tag: "UBX", StartTime: t1}
	// Third epoch start: flushes second epoch
	mgr.EpochStarted(f, t2)
	if len(rec.epochs) != 2 {
		t.Fatalf("expected 2 epochs, got %d", len(rec.epochs))
	}
}

func TestNavEpochManagerMultiProtocol(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Millisecond)
	t2 := t0.Add(time.Second)
	binary := &testFlusher{pri: PriVendorLow, mh: rec}
	nmeaF := &testFlusher{pri: PriGenericHigh, mh: rec}
	// Epoch 1: both protocols register
	mgr.EpochStarted(binary, t0)
	mgr.EpochStarted(nmeaF, t1)
	if len(rec.epochs) != 0 {
		t.Fatalf("no flush expected in first epoch, got %d", len(rec.epochs))
	}
	// Set up epoch 1 data for flush
	binary.msg = &NavEpochMsg{Tag: "UBX", StartTime: t0, Acc: Accuracy{Pos: opt.Make(10 * Meter)}}
	nmeaF.msg = &NavEpochMsg{Tag: "NMEA", StartTime: t1, Acc: Accuracy{Hor: opt.Make(5 * Meter)}}
	// Epoch 2: binary starts first, triggering flush
	mgr.EpochStarted(binary, t2)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after binary starts epoch 2, got %d", len(rec.epochs))
	}
	m := rec.epochs[0]
	// Tag should come from binary (higher priority)
	if m.Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", m.Tag)
	}
	// StartTime should be earliest
	if m.StartTime != t0 {
		t.Errorf("StartTime = %v, want %v", m.StartTime, t0)
	}
	// Accuracy should be merged
	if m.Acc.Pos.Get() != 10*Meter {
		t.Errorf("Acc.Pos = %v, want 10m", m.Acc.Pos.Get())
	}
	if m.Acc.Hor.Get() != 5*Meter {
		t.Errorf("Acc.Hor = %v, want 5m", m.Acc.Hor.Get())
	}
}

func TestNavEpochManagerEndOfEpochSingle(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	f := &testFlusher{pri: PriVendorLow, mh: rec}
	mgr.EpochStarted(f, t0)
	f.msg = &NavEpochMsg{Tag: "UBX", StartTime: t0}
	// EndOfEpoch with one active processor: flushes immediately
	mgr.EndOfEpoch(t1)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after EndOfEpoch, got %d", len(rec.epochs))
	}
	if rec.epochs[0].Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", rec.epochs[0].Tag)
	}
}

func TestNavEpochManagerEndOfEpochMultiple(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	f1 := &testFlusher{pri: PriVendorLow, mh: rec}
	f2 := &testFlusher{pri: PriGenericHigh, mh: rec}
	mgr.EpochStarted(f1, t0)
	mgr.EpochStarted(f2, t0)
	f1.msg = &NavEpochMsg{Tag: "UBX"}
	f2.msg = &NavEpochMsg{Tag: "NMEA"}
	// EndOfEpoch with multiple active: flushes all processors
	mgr.EndOfEpoch(t0)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch after EndOfEpoch, got %d", len(rec.epochs))
	}
	// UBX (PriVendorLow=3) wins over NMEA (PriGenericHigh=2) for Tag
	if rec.epochs[0].Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", rec.epochs[0].Tag)
	}
}

func TestNavEpochManagerNilContribution(t *testing.T) {
	mgr := NewNavEpochManager()
	rec := &epochRecorder{}
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)
	binary := &testFlusher{pri: PriVendorLow, mh: rec}
	casicF := &testFlusher{pri: PriVendorLow, mh: rec}
	// Epoch 1: both register
	mgr.EpochStarted(binary, t0)
	mgr.EpochStarted(casicF, t0)
	// Only binary has data; CASIC returns nil
	binary.msg = &NavEpochMsg{Tag: "UBX", StartTime: t0}
	casicF.msg = nil
	// Epoch 2: triggers flush
	mgr.EpochStarted(binary, t1)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(rec.epochs))
	}
	if rec.epochs[0].Tag != "UBX" {
		t.Errorf("Tag = %v, want UBX", rec.epochs[0].Tag)
	}
	// Now test all-nil: no emission
	binary.msg = nil
	casicF.msg = nil
	mgr.EpochStarted(casicF, t1)
	mgr.EpochStarted(binary, t2)
	if len(rec.epochs) != 1 {
		t.Fatalf("expected still 1 epoch (all nil), got %d", len(rec.epochs))
	}
}
