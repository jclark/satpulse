package casic

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestPosECEFNav2Sol(t *testing.T) {
	m := &casbin.Nav2Sol{
		FixFlags:  casbin.Nav2Fix3D,
		VelFlags:  casbin.Nav2Vel3D,
		NumFixTot: 12,
		PDOP:      1.5,
		X:         1000.0,
		Y:         -2000.0,
		Z:         3000.0,
		PAcc:      3.0, // std dev, NOT variance
	}
	var ne gpsprot.NavEpochMsg
	got := posECEFNav2Sol(&ne, m)
	want := &gpsprot.PosECEFMsg{
		Pos: gpsprot.Point3D{
			gpsprot.Meters(1000.0), gpsprot.Meters(-2000.0), gpsprot.Meters(3000.0),
		},
		NativeMsgID: "NAV2-SOL",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posECEFNav2Sol() = %+v, want %+v", got, want)
	}
	// V6: PAcc is std dev, used directly (no sqrt)
	wantAcc := gpsprot.Meters(3.0)
	if !ne.Acc.Pos.IsSet() || ne.Acc.Pos.Get() != wantAcc {
		t.Errorf("Acc.Pos = %v (set=%v); want %v", ne.Acc.Pos.Get(), ne.Acc.Pos.IsSet(), wantAcc)
	}
	if ne.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("FixLevel = %v, want %v", ne.FixLevel, gpsprot.FixLevelCode)
	}
	if ne.SolutionDim != gpsprot.SolutionDim3D {
		t.Errorf("SolutionDim = %v, want %v", ne.SolutionDim, gpsprot.SolutionDim3D)
	}
	if !ne.DOP.Pos.IsSet() || ne.DOP.Pos.Get() != float64(float32(1.5)) {
		t.Errorf("DOP.Pos = %v, want %v", ne.DOP.Pos.Get(), float64(float32(1.5)))
	}
}

func TestPosECEFNav2SolInvalid(t *testing.T) {
	m := &casbin.Nav2Sol{
		FixFlags:  casbin.Nav2FixRoughEstimate,
		NumFixTot: 5,
		PDOP:      99.0,
	}
	var ne gpsprot.NavEpochMsg
	got := posECEFNav2Sol(&ne, m)
	if got != nil {
		t.Errorf("posECEFNav2Sol() = %+v, want nil", got)
	}
	if ne.FixLevel != gpsprot.FixLevelNone {
		t.Errorf("FixLevel = %v, want %v", ne.FixLevel, gpsprot.FixLevelNone)
	}
	if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 5 {
		t.Errorf("NumSVUsed = %v, want 5", ne.NumSVUsed.Get())
	}
}

func TestVelECEFNav2Sol(t *testing.T) {
	m := &casbin.Nav2Sol{
		VelFlags: casbin.Nav2Vel3D,
		VX:       1.0,
		VY:       -2.0,
		VZ:       0.5,
		SAcc:     0.5, // std dev, NOT variance
	}
	var ne gpsprot.NavEpochMsg
	got := velECEFNav2Sol(&ne, m)
	want := &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(1.0),
			gpsprot.MetersPerSecondFromFloat(-2.0),
			gpsprot.MetersPerSecondFromFloat(0.5),
		},
		NativeMsgID: "NAV2-SOL",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velECEFNav2Sol() = %+v, want %+v", got, want)
	}
	// V6: SAcc is std dev, used directly
	wantAcc := gpsprot.MetersPerSecondFromFloat(0.5)
	if !ne.Acc.Speed.IsSet() || ne.Acc.Speed.Get() != wantAcc {
		t.Errorf("Acc.Speed = %v (set=%v); want %v", ne.Acc.Speed.Get(), ne.Acc.Speed.IsSet(), wantAcc)
	}
}

func TestVelECEFNav2SolInvalid(t *testing.T) {
	m := &casbin.Nav2Sol{VelFlags: casbin.Nav2VelRoughEstimate}
	var ne gpsprot.NavEpochMsg
	got := velECEFNav2Sol(&ne, m)
	if got != nil {
		t.Errorf("velECEFNav2Sol() = %+v, want nil", got)
	}
}

func TestPosGeoNav2Pvh(t *testing.T) {
	m := &casbin.Nav2Pvh{
		FixFlags:  casbin.Nav2Fix3D,
		NumFixTot: 10,
		Lat:       47.0,
		Lon:       8.0,
		Height:    500.0,
		SepGeoid:  50.0,
		HAcc:      2.0, // std dev
		VAcc:      3.0, // std dev
	}
	var ne gpsprot.NavEpochMsg
	got := posGeoNav2Pvh(&ne, m)
	want := &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(47.0), gpsprot.DegreesFromFloat(8.0)},
		Height:      opt.Make(gpsprot.Meters(500.0)),
		HeightMSL:   opt.Make(gpsprot.Meters(450.0)),
		NativeMsgID: "NAV2-PVH",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posGeoNav2Pvh() = %+v, want %+v", got, want)
	}
	// V6: accuracy is std dev, used directly (no sqrt)
	wantHor := gpsprot.Meters(2.0)
	if !ne.Acc.Hor.IsSet() || ne.Acc.Hor.Get() != wantHor {
		t.Errorf("Acc.Hor = %v (set=%v); want %v", ne.Acc.Hor.Get(), ne.Acc.Hor.IsSet(), wantHor)
	}
	wantVert := gpsprot.Meters(3.0)
	if !ne.Acc.Vert.IsSet() || ne.Acc.Vert.Get() != wantVert {
		t.Errorf("Acc.Vert = %v (set=%v); want %v", ne.Acc.Vert.Get(), ne.Acc.Vert.IsSet(), wantVert)
	}
}

func TestPosGeoNav2PvhInvalid(t *testing.T) {
	m := &casbin.Nav2Pvh{FixFlags: casbin.Nav2FixDeadReckoning, NumFixTot: 0}
	var ne gpsprot.NavEpochMsg
	got := posGeoNav2Pvh(&ne, m)
	if got != nil {
		t.Errorf("posGeoNav2Pvh() = %+v, want nil", got)
	}
	if ne.FixLevel != gpsprot.FixLevelNone {
		t.Errorf("FixLevel = %v, want %v", ne.FixLevel, gpsprot.FixLevelNone)
	}
	if ne.AuxSrc != gpsprot.AuxSrcDR {
		t.Errorf("AuxSrc = %v, want %v", ne.AuxSrc, gpsprot.AuxSrcDR)
	}
}

func TestVelGeoNav2Pvh(t *testing.T) {
	m := &casbin.Nav2Pvh{
		VelFlags: casbin.Nav2Vel3D,
		VelN:     1.0,
		VelE:     -2.0,
		VelU:     0.5, // negated to Down = -0.5
		Speed3D:  3.0,
		Speed2D:  2.5,
		Heading:  45.0,
		SAcc:     0.5, // std dev
		CAcc:     2.0, // std dev in deg
	}
	var ne gpsprot.NavEpochMsg
	got := velGeoNav2Pvh(&ne, m)
	want := &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(1.0),
			gpsprot.MetersPerSecondFromFloat(-2.0),
			gpsprot.MetersPerSecondFromFloat(-0.5),
		}),
		Speed3D:     opt.Make(gpsprot.MetersPerSecondFromFloat(3.0)),
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(2.5)),
		Course:      opt.Make(gpsprot.DegreesFromFloat(45.0)),
		NativeMsgID: "NAV2-PVH",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velGeoNav2Pvh() = %+v, want %+v", got, want)
	}
	wantSpeedAcc := gpsprot.MetersPerSecondFromFloat(0.5)
	if !ne.Acc.Speed.IsSet() || ne.Acc.Speed.Get() != wantSpeedAcc {
		t.Errorf("Acc.Speed = %v (set=%v); want %v", ne.Acc.Speed.Get(), ne.Acc.Speed.IsSet(), wantSpeedAcc)
	}
	wantCourseAcc := gpsprot.DegreesFromFloat(2.0)
	if !ne.Acc.Course.IsSet() || ne.Acc.Course.Get() != wantCourseAcc {
		t.Errorf("Acc.Course = %v (set=%v); want %v", ne.Acc.Course.Get(), ne.Acc.Course.IsSet(), wantCourseAcc)
	}
}

func TestVelGeoNav2PvhInvalid(t *testing.T) {
	m := &casbin.Nav2Pvh{VelFlags: casbin.Nav2VelRoughEstimate}
	var ne gpsprot.NavEpochMsg
	got := velGeoNav2Pvh(&ne, m)
	if got != nil {
		t.Errorf("velGeoNav2Pvh() = %+v, want nil", got)
	}
}

func TestDopNav2Dop(t *testing.T) {
	m := &casbin.Nav2Dop{
		PDOP: 1.5,
		HDOP: 0.8,
		VDOP: 1.2,
		NDOP: 0.6,
		EDOP: 0.5,
		TDOP: 0.9,
	}
	var ne gpsprot.NavEpochMsg
	dopNav2Dop(&ne, m)
	check := func(name string, got opt.Val[float64], want float64) {
		if !got.IsSet() || got.Get() != want {
			t.Errorf("DOP.%s = %v (set=%v); want %v", name, got.Get(), got.IsSet(), want)
		}
	}
	check("Pos", ne.DOP.Pos, float64(float32(1.5)))
	check("Hor", ne.DOP.Hor, float64(float32(0.8)))
	check("Vert", ne.DOP.Vert, float64(float32(1.2)))
	check("Time", ne.DOP.Time, float64(float32(0.9)))
	check("North", ne.DOP.North, float64(float32(0.6)))
	check("East", ne.DOP.East, float64(float32(0.5)))
}

func TestQualityFromNav2FixFlags(t *testing.T) {
	tests := []struct {
		name       string
		ff         casbin.Nav2FixFlags
		fixLevel   gpsprot.FixLevel
		fixDim     gpsprot.SolutionDim
		auxSrc     gpsprot.AuxSrc
		correction gpsprot.CorrKind
	}{
		{"Invalid", casbin.Nav2FixInvalid, gpsprot.FixLevelNone, 0, 0, 0},
		{"External", casbin.Nav2FixExternal, gpsprot.FixLevelNotMeasured, 0, 0, 0},
		{"RoughEstimate", casbin.Nav2FixRoughEstimate, gpsprot.FixLevelNone, 0, 0, 0},
		{"Hold", casbin.Nav2FixHold, gpsprot.FixLevelNone, 0, 0, 0},
		{"DeadReckoning", casbin.Nav2FixDeadReckoning, gpsprot.FixLevelNone, 0, gpsprot.AuxSrcDR, 0},
		{"QuickMode", casbin.Nav2FixQuickMode, gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, 0},
		{"2D", casbin.Nav2Fix2D, gpsprot.FixLevelCode, gpsprot.SolutionDim2D, 0, 0},
		{"3D", casbin.Nav2Fix3D, gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0, 0},
		{"DGPS", casbin.Nav2FixDGPS, gpsprot.FixLevelCodeCorrected, gpsprot.SolutionDim3D, 0, gpsprot.CorrUsed},
		{"RTKFloat", casbin.Nav2FixRTKFloat, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, 0, gpsprot.CorrOSR | gpsprot.CorrUsed},
		{"RTKFixed", casbin.Nav2FixRTKFixed, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, 0, gpsprot.CorrOSR | gpsprot.CorrUsed},
		{"TimingFixed", casbin.Nav2FixTimingFixed, gpsprot.FixLevelCode, gpsprot.SolutionDimTimeOnly, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ne gpsprot.NavEpochMsg
			qualityFromNav2FixFlags(&ne, tt.ff, 10)
			if ne.FixLevel != tt.fixLevel {
				t.Errorf("FixLevel = %v, want %v", ne.FixLevel, tt.fixLevel)
			}
			if ne.SolutionDim != tt.fixDim {
				t.Errorf("SolutionDim = %v, want %v", ne.SolutionDim, tt.fixDim)
			}
			if ne.AuxSrc != tt.auxSrc {
				t.Errorf("AuxSrc = %v, want %v", ne.AuxSrc, tt.auxSrc)
			}
			if ne.Correction != tt.correction {
				t.Errorf("Correction = %v, want %v", ne.Correction, tt.correction)
			}
			if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 10 {
				t.Errorf("NumSVUsed = %v, want 10", ne.NumSVUsed.Get())
			}
		})
	}
}
