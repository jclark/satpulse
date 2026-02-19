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
