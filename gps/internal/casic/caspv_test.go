package casic

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestPosECEFNavSol(t *testing.T) {
	m := &casbin.NavSol{
		PosValid: casbin.NavPos3D,
		VelValid: casbin.NavVel3D,
		NumSV:    12,
		PDOP:     1.5,
		ECEFX:    1000.0,
		ECEFY:    -2000.0,
		ECEFZ:    3000.0,
		PAcc:     9.0, // m^2 variance -> sqrt = 3.0 m
	}
	var ne gpsprot.NavEpochMsg
	got := posECEFNavSol(&ne, m)
	want := &gpsprot.PosECEFMsg{
		Pos: gpsprot.Point3D{
			gpsprot.Meters(1000.0), gpsprot.Meters(-2000.0), gpsprot.Meters(3000.0),
		},
		NativeMsgID: "NAV-SOL",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posECEFNavSol() = %+v, want %+v", got, want)
	}
	wantAcc := gpsprot.Meters(3.0)
	if !ne.Acc.Pos.IsSet() || ne.Acc.Pos.Get() != wantAcc {
		t.Errorf("Acc.Pos = %v (set=%v); want %v", ne.Acc.Pos.Get(), ne.Acc.Pos.IsSet(), wantAcc)
	}
	if ne.FixLevel != gpsprot.FixLevelCode {
		t.Errorf("FixLevel = %v, want %v", ne.FixLevel, gpsprot.FixLevelCode)
	}
	if ne.FixDim != gpsprot.FixDim3D {
		t.Errorf("FixDim = %v, want %v", ne.FixDim, gpsprot.FixDim3D)
	}
}

func TestPosECEFNavSolInvalid(t *testing.T) {
	m := &casbin.NavSol{
		PosValid: casbin.NavPosRoughEstimate,
		NumSV:    5,
		PDOP:     99.0,
	}
	var ne gpsprot.NavEpochMsg
	got := posECEFNavSol(&ne, m)
	if got != nil {
		t.Errorf("posECEFNavSol() = %+v, want nil", got)
	}
	// Quality should still be populated
	if ne.FixLevel != gpsprot.FixLevelNone {
		t.Errorf("FixLevel = %v, want %v", ne.FixLevel, gpsprot.FixLevelNone)
	}
	if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 5 {
		t.Errorf("NumSVUsed = %v, want 5", ne.NumSVUsed.Get())
	}
}

func TestVelECEFNavSol(t *testing.T) {
	m := &casbin.NavSol{
		VelValid: casbin.NavVel3D,
		ECEFVX:   1.0,
		ECEFVY:   -2.0,
		ECEFVZ:   0.5,
		SAcc:     0.25, // (m/s)^2 -> sqrt = 0.5 m/s
	}
	var ne gpsprot.NavEpochMsg
	got := velECEFNavSol(&ne, m)
	want := &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(1.0),
			gpsprot.MetersPerSecondFromFloat(-2.0),
			gpsprot.MetersPerSecondFromFloat(0.5),
		},
		NativeMsgID: "NAV-SOL",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velECEFNavSol() = %+v, want %+v", got, want)
	}
	wantAcc := gpsprot.MetersPerSecondFromFloat(0.5)
	if !ne.Acc.Speed.IsSet() || ne.Acc.Speed.Get() != wantAcc {
		t.Errorf("Acc.Speed = %v (set=%v); want %v", ne.Acc.Speed.Get(), ne.Acc.Speed.IsSet(), wantAcc)
	}
}

func TestVelECEFNavSolInvalid(t *testing.T) {
	m := &casbin.NavSol{VelValid: casbin.NavVelRoughEstimate}
	var ne gpsprot.NavEpochMsg
	got := velECEFNavSol(&ne, m)
	if got != nil {
		t.Errorf("velECEFNavSol() = %+v, want nil", got)
	}
	if ne.Acc.Speed.IsSet() {
		t.Errorf("Acc.Speed should not be set for invalid velocity")
	}
}

func TestPosGeoNavPv(t *testing.T) {
	m := &casbin.NavPv{
		PosValid: casbin.NavPos3D,
		NumSV:    10,
		PDOP:     1.2,
		Lat:      47.0,
		Lon:      8.0,
		Height:   500.0,
		SepGeoid: 50.0,
		HAcc:     4.0, // m^2 -> sqrt = 2.0 m
		VAcc:     9.0, // m^2 -> sqrt = 3.0 m
	}
	var ne gpsprot.NavEpochMsg
	got := posGeoNavPv(&ne, m)
	want := &gpsprot.PosGeoMsg{
		LatLon:      [2]gpsprot.Angle{gpsprot.DegreesFromFloat(47.0), gpsprot.DegreesFromFloat(8.0)},
		Height:      opt.Make(gpsprot.Meters(500.0)),
		HeightMSL:   opt.Make(gpsprot.Meters(450.0)),
		NativeMsgID: "NAV-PV",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posGeoNavPv() = %+v, want %+v", got, want)
	}
	wantHor := gpsprot.Meters(2.0)
	if !ne.Acc.Hor.IsSet() || ne.Acc.Hor.Get() != wantHor {
		t.Errorf("Acc.Hor = %v (set=%v); want %v", ne.Acc.Hor.Get(), ne.Acc.Hor.IsSet(), wantHor)
	}
	wantVert := gpsprot.Meters(3.0)
	if !ne.Acc.Vert.IsSet() || ne.Acc.Vert.Get() != wantVert {
		t.Errorf("Acc.Vert = %v (set=%v); want %v", ne.Acc.Vert.Get(), ne.Acc.Vert.IsSet(), wantVert)
	}
}

func TestPosGeoNavPvSepGeoidZero(t *testing.T) {
	m := &casbin.NavPv{
		PosValid: casbin.NavPos3D,
		NumSV:    10,
		PDOP:     1.2,
		Lat:      47.0,
		Lon:      8.0,
		Height:   500.0,
		SepGeoid: 0,
		HAcc:     4.0,
		VAcc:     9.0,
	}
	var ne gpsprot.NavEpochMsg
	got := posGeoNavPv(&ne, m)
	if !got.Height.IsSet() {
		t.Fatal("Height should be set")
	}
	if got.HeightMSL.IsSet() {
		t.Errorf("HeightMSL should not be set when SepGeoid is 0, got %v", got.HeightMSL.Get())
	}
}

func TestPosGeoNavPvInvalid(t *testing.T) {
	m := &casbin.NavPv{PosValid: casbin.NavPosDeadReckoning, NumSV: 0, PDOP: 99.0}
	var ne gpsprot.NavEpochMsg
	got := posGeoNavPv(&ne, m)
	if got != nil {
		t.Errorf("posGeoNavPv() = %+v, want nil", got)
	}
	if ne.FixLevel != gpsprot.FixLevelNone {
		t.Errorf("FixLevel = %v, want %v", ne.FixLevel, gpsprot.FixLevelNone)
	}
	if ne.AuxSrc != gpsprot.AuxSrcDR {
		t.Errorf("AuxSrc = %v, want %v", ne.AuxSrc, gpsprot.AuxSrcDR)
	}
}

func TestVelGeoNavPv(t *testing.T) {
	m := &casbin.NavPv{
		VelValid: casbin.NavVel3D,
		VelN:     1.0,
		VelE:     -2.0,
		VelU:     0.5, // negated to Down = -0.5
		Speed3D:  3.0,
		Speed2D:  2.5,
		Heading:  45.0,
		SAcc:     0.25, // (m/s)^2 -> sqrt = 0.5
		CAcc:     4.0,  // deg^2 -> sqrt = 2.0
	}
	var ne gpsprot.NavEpochMsg
	got := velGeoNavPv(&ne, m)
	want := &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			gpsprot.MetersPerSecondFromFloat(1.0),
			gpsprot.MetersPerSecondFromFloat(-2.0),
			gpsprot.MetersPerSecondFromFloat(-0.5),
		}),
		Speed3D:     opt.Make(gpsprot.MetersPerSecondFromFloat(3.0)),
		GroundSpeed: opt.Make(gpsprot.MetersPerSecondFromFloat(2.5)),
		Course:      opt.Make(gpsprot.DegreesFromFloat(45.0)),
		NativeMsgID: "NAV-PV",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velGeoNavPv() = %+v, want %+v", got, want)
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

func TestVelGeoNavPvInvalid(t *testing.T) {
	m := &casbin.NavPv{VelValid: casbin.NavVelRoughEstimate}
	var ne gpsprot.NavEpochMsg
	got := velGeoNavPv(&ne, m)
	if got != nil {
		t.Errorf("velGeoNavPv() = %+v, want nil", got)
	}
}

func TestDopNavDop(t *testing.T) {
	m := &casbin.NavDop{
		PDOP: 1.5,
		HDOP: 0.8,
		VDOP: 1.2,
		NDOP: 0.6,
		EDOP: 0.5,
		TDOP: 0.9,
	}
	var ne gpsprot.NavEpochMsg
	dopNavDop(&ne, m)
	check := func(name string, got opt.Val[float64], want float64) {
		if !got.IsSet() || got.Get() != want {
			t.Errorf("DOP.%s = %v (set=%v); want %v", name, got.Get(), got.IsSet(), want)
		}
	}
	check("Pos", ne.DOP.Pos, float64(float32(1.5)))
	check("Hor", ne.DOP.Hor, float64(float32(0.8)))
	check("Vert", ne.DOP.Vert, float64(float32(1.2)))
	check("Time", ne.DOP.Time, float64(float32(0.9)))
	// NDOP and EDOP are not represented in gpsprot.DOP
}

func TestQualityFromPosValid(t *testing.T) {
	tests := []struct {
		name     string
		pv       casbin.NavPosValid
		fixLevel gpsprot.FixLevel
		fixDim   gpsprot.FixDim
		auxSrc   gpsprot.AuxSrc
	}{
		{"Invalid", casbin.NavPosInvalid, gpsprot.FixLevelNone, 0, 0},
		{"External", casbin.NavPosExternal, gpsprot.FixLevelNotMeasured, 0, 0},
		{"RoughEstimate", casbin.NavPosRoughEstimate, gpsprot.FixLevelNone, 0, 0},
		{"Maintaining", casbin.NavPosMaintaining, gpsprot.FixLevelNone, 0, 0},
		{"DeadReckoning", casbin.NavPosDeadReckoning, gpsprot.FixLevelNone, 0, gpsprot.AuxSrcDR},
		{"QuickMode", casbin.NavPosQuickMode, gpsprot.FixLevelCode, gpsprot.FixDim3D, 0},
		{"2D", casbin.NavPos2D, gpsprot.FixLevelCode, gpsprot.FixDim2D, 0},
		{"3D", casbin.NavPos3D, gpsprot.FixLevelCode, gpsprot.FixDim3D, 0},
		{"GNSSDR", casbin.NavPosGNSSDR, gpsprot.FixLevelCode, gpsprot.FixDim3D, gpsprot.AuxSrcDR},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ne gpsprot.NavEpochMsg
			qualityFromPosValid(&ne, tt.pv, 10, 1.5)
			if ne.FixLevel != tt.fixLevel {
				t.Errorf("FixLevel = %v, want %v", ne.FixLevel, tt.fixLevel)
			}
			if ne.FixDim != tt.fixDim {
				t.Errorf("FixDim = %v, want %v", ne.FixDim, tt.fixDim)
			}
			if ne.AuxSrc != tt.auxSrc {
				t.Errorf("AuxSrc = %v, want %v", ne.AuxSrc, tt.auxSrc)
			}
			if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 10 {
				t.Errorf("NumSVUsed = %v, want 10", ne.NumSVUsed.Get())
			}
			if !ne.DOP.Pos.IsSet() || ne.DOP.Pos.Get() != float64(float32(1.5)) {
				t.Errorf("DOP.Pos = %v, want %v", ne.DOP.Pos.Get(), float64(float32(1.5)))
			}
		})
	}
}

