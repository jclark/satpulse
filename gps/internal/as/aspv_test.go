package as

import (
	"reflect"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/opt"
)

func TestPosEcefNavPosEcef(t *testing.T) {
	m := &asbin.NavPosEcef{
		EcefX: -267173351, // cm
		EcefY: -402753274,
		EcefZ: 391919498,
		PAcc:  1543, // cm
	}
	var ne gpsprot.NavEpochMsg
	got := posEcefNavPosEcef(&ne, m)
	want := &gpsprot.PosECEFMsg{
		Pos: gpsprot.Point3D{
			gpsprot.Length(-267173351) * gpsprot.Centimeter,
			gpsprot.Length(-402753274) * gpsprot.Centimeter,
			gpsprot.Length(391919498) * gpsprot.Centimeter,
		},
		NativeMsgID: "NAV-POSECEF",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posEcefNavPosEcef() = %+v, want %+v", got, want)
	}
	wantAcc := gpsprot.Length(1543) * gpsprot.Centimeter
	if !ne.Acc.Pos.IsSet() || ne.Acc.Pos.Get() != wantAcc {
		t.Errorf("Acc.Pos = %v (set=%v); want %v", ne.Acc.Pos.Get(), ne.Acc.Pos.IsSet(), wantAcc)
	}
}

func TestPosGeoNavPosLlh(t *testing.T) {
	m := &asbin.NavPosLlh{
		Lat:    473977640, // 1e-7 deg (47.3977640)
		Lon:    85255110,  // 1e-7 deg (8.5255110)
		Height: 467890,    // mm
		HMSL:   420123,    // mm
		HAcc:   12345,     // mm
		VAcc:   23456,     // mm
	}
	var ne gpsprot.NavEpochMsg
	got := posGeoNavPosLlh(&ne, m)
	want := &gpsprot.PosGeoMsg{
		LatLon: [2]gpsprot.Angle{
			gpsprot.Angle(473977640) * 100,
			gpsprot.Angle(85255110) * 100,
		},
		Height:      opt.Make(gpsprot.Length(467890) * gpsprot.Millimeter),
		HeightMSL:   opt.Make(gpsprot.Length(420123) * gpsprot.Millimeter),
		NativeMsgID: "NAV-POSLLH",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posGeoNavPosLlh() = %+v, want %+v", got, want)
	}
	wantHor := gpsprot.Length(12345) * gpsprot.Millimeter
	if !ne.Acc.Hor.IsSet() || ne.Acc.Hor.Get() != wantHor {
		t.Errorf("Acc.Hor = %v (set=%v); want %v", ne.Acc.Hor.Get(), ne.Acc.Hor.IsSet(), wantHor)
	}
	wantVert := gpsprot.Length(23456) * gpsprot.Millimeter
	if !ne.Acc.Vert.IsSet() || ne.Acc.Vert.Get() != wantVert {
		t.Errorf("Acc.Vert = %v (set=%v); want %v", ne.Acc.Vert.Get(), ne.Acc.Vert.IsSet(), wantVert)
	}
}

func TestVelEcefNavVelEcef(t *testing.T) {
	m := &asbin.NavVelEcef{
		EcefVX: -150, // cm/s
		EcefVY: 230,
		EcefVZ: -45,
		SAcc:   12, // cm/s
	}
	var ne gpsprot.NavEpochMsg
	got := velEcefNavVelEcef(&ne, m)
	want := &gpsprot.VelECEFMsg{
		Vel: [3]gpsprot.Speed{
			gpsprot.Speed(-150) * gpsprot.CentimeterPerSecond,
			gpsprot.Speed(230) * gpsprot.CentimeterPerSecond,
			gpsprot.Speed(-45) * gpsprot.CentimeterPerSecond,
		},
		NativeMsgID: "NAV-VELECEF",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velEcefNavVelEcef() = %+v, want %+v", got, want)
	}
	wantAcc := gpsprot.Speed(12) * gpsprot.CentimeterPerSecond
	if !ne.Acc.Speed.IsSet() || ne.Acc.Speed.Get() != wantAcc {
		t.Errorf("Acc.Speed = %v (set=%v); want %v", ne.Acc.Speed.Get(), ne.Acc.Speed.IsSet(), wantAcc)
	}
}

func TestVelGeoNavVelNed(t *testing.T) {
	m := &asbin.NavVelNed{
		VelN:    -50,     // cm/s
		VelE:    120,     // cm/s
		VelD:    -5,      // cm/s
		Speed:   131,     // cm/s (3D)
		GSpeed:  130,     // cm/s (2D)
		Heading: 1125000, // 1e-5 deg (11.25 degrees)
		SAcc:    8,       // cm/s
		CAcc:    500000,  // 1e-5 deg (5.0 degrees)
	}
	var ne gpsprot.NavEpochMsg
	got := velGeoNavVelNed(&ne, m)
	want := &gpsprot.VelGeoMsg{
		VelNED: opt.Make([3]gpsprot.Speed{
			gpsprot.Speed(-50) * gpsprot.CentimeterPerSecond,
			gpsprot.Speed(120) * gpsprot.CentimeterPerSecond,
			gpsprot.Speed(-5) * gpsprot.CentimeterPerSecond,
		}),
		Speed3D:     opt.Make(gpsprot.Speed(131) * gpsprot.CentimeterPerSecond),
		GroundSpeed: opt.Make(gpsprot.Speed(130) * gpsprot.CentimeterPerSecond),
		Course:      opt.Make(gpsprot.Angle(1125000) * 10000),
		NativeMsgID: "NAV-VELNED",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velGeoNavVelNed() = %+v, want %+v", got, want)
	}
	wantSpeedAcc := gpsprot.Speed(8) * gpsprot.CentimeterPerSecond
	if !ne.Acc.Speed.IsSet() || ne.Acc.Speed.Get() != wantSpeedAcc {
		t.Errorf("Acc.Speed = %v (set=%v); want %v", ne.Acc.Speed.Get(), ne.Acc.Speed.IsSet(), wantSpeedAcc)
	}
	wantCourseAcc := gpsprot.Angle(500000) * 10000
	if !ne.Acc.Course.IsSet() || ne.Acc.Course.Get() != wantCourseAcc {
		t.Errorf("Acc.Course = %v (set=%v); want %v", ne.Acc.Course.Get(), ne.Acc.Course.IsSet(), wantCourseAcc)
	}
}
