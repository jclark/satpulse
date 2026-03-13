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

func TestDopNavDop(t *testing.T) {
	m := &asbin.NavDop{
		GDOP: 199, // 1.99
		PDOP: 172, // 1.72
		TDOP: 103, // 1.03
		VDOP: 120, // 1.20
		HDOP: 125, // 1.25
		NDOP: 55,  // 0.55
		EDOP: 45,  // 0.45
	}
	var ne gpsprot.NavEpochMsg
	dopNavDop(&ne, m)
	check := func(name string, got opt.Val[float64], want float64) {
		t.Helper()
		if !got.IsSet() {
			t.Errorf("DOP.%s not set", name)
		} else if got.Get() != want {
			t.Errorf("DOP.%s = %v, want %v", name, got.Get(), want)
		}
	}
	check("Geom", ne.DOP.Geom, 1.99)
	check("Pos", ne.DOP.Pos, 1.72)
	check("Time", ne.DOP.Time, 1.03)
	check("Vert", ne.DOP.Vert, 1.20)
	check("Hor", ne.DOP.Hor, 1.25)
	check("North", ne.DOP.North, 0.55)
	check("East", ne.DOP.East, 0.45)
}

func TestPosGeoNavAuto(t *testing.T) {
	m := &asbin.NavAuto{
		FixState: 4, // 3D fix
		Lat:      473977640,
		Lon:      85255110,
		Alt:      467890, // mm
	}
	got := posGeoNavAuto(m)
	want := &gpsprot.PosGeoMsg{
		LatLon: [2]gpsprot.Angle{
			gpsprot.Angle(473977640) * 100,
			gpsprot.Angle(85255110) * 100,
		},
		Height:      opt.Make(gpsprot.Length(467890) * gpsprot.Millimeter),
		NativeMsgID: "NAV-AUTO",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("posGeoNavAuto() = %+v, want %+v", got, want)
	}
}

func TestPosGeoNavAutoNoFix(t *testing.T) {
	for _, fs := range []uint8{0, 1, 2} {
		if got := posGeoNavAuto(&asbin.NavAuto{FixState: fs}); got != nil {
			t.Errorf("posGeoNavAuto(fixState=%d) = %+v, want nil", fs, got)
		}
	}
}

func TestVelGeoNavAuto(t *testing.T) {
	m := &asbin.NavAuto{
		FixState: 4,
		Speed:    350,  // cm/s
		Heading:  4520, // 1e-2 deg (45.20 degrees)
	}
	got := velGeoNavAuto(m)
	want := &gpsprot.VelGeoMsg{
		GroundSpeed: opt.Make(gpsprot.Speed(350) * gpsprot.CentimeterPerSecond),
		Course:      opt.Make(gpsprot.Angle(4520) * 10_000_000),
		NativeMsgID: "NAV-AUTO",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("velGeoNavAuto() = %+v, want %+v", got, want)
	}
}

func TestVelGeoNavAutoNoFix(t *testing.T) {
	for _, fs := range []uint8{0, 1, 2} {
		if got := velGeoNavAuto(&asbin.NavAuto{FixState: fs}); got != nil {
			t.Errorf("velGeoNavAuto(fixState=%d) = %+v, want nil", fs, got)
		}
	}
}

func TestQualityNavAuto(t *testing.T) {
	tests := []struct {
		fixState   uint8
		fixLevel   gpsprot.FixLevel
		fixDim     gpsprot.SolutionDim
		correction gpsprot.CorrKind
	}{
		{0, gpsprot.FixLevelNone, 0, 0},
		{1, gpsprot.FixLevelNone, 0, 0},
		{2, gpsprot.FixLevelCode, gpsprot.SolutionDimTimeOnly, 0},
		{3, gpsprot.FixLevelCode, gpsprot.SolutionDim2D, 0},
		{4, gpsprot.FixLevelCode, gpsprot.SolutionDim3D, 0},
		{5, gpsprot.FixLevelCode, gpsprot.SolutionDim3D, gpsprot.CorrUsed},
		{6, gpsprot.FixLevelCarrierFloat, gpsprot.SolutionDim3D, gpsprot.CorrUsed},
		{7, gpsprot.FixLevelCarrierFixed, gpsprot.SolutionDim3D, gpsprot.CorrUsed},
	}
	for _, tt := range tests {
		var ne gpsprot.NavEpochMsg
		m := &asbin.NavAuto{
			FixState:  tt.fixState,
			SatInUse:  12,
			SatInView: 24,
			PDOP:      172,
			HDOP:      125,
			VDOP:      120,
		}
		qualityNavAuto(&ne, m)
		if ne.FixLevel != tt.fixLevel {
			t.Errorf("fixState=%d: FixLevel=%v, want %v", tt.fixState, ne.FixLevel, tt.fixLevel)
		}
		if ne.SolutionDim != tt.fixDim {
			t.Errorf("fixState=%d: SolutionDim=%v, want %v", tt.fixState, ne.SolutionDim, tt.fixDim)
		}
		if ne.Correction != tt.correction {
			t.Errorf("fixState=%d: Correction=%v, want %v", tt.fixState, ne.Correction, tt.correction)
		}
		if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 12 {
			t.Errorf("fixState=%d: NumSVUsed=%v, want 12", tt.fixState, ne.NumSVUsed)
		}
		if !ne.NumSVTracked.IsSet() || ne.NumSVTracked.Get() != 24 {
			t.Errorf("fixState=%d: NumSVTracked=%v, want 24", tt.fixState, ne.NumSVTracked)
		}
		if !ne.DOP.Pos.IsSet() || ne.DOP.Pos.Get() != 1.72 {
			t.Errorf("fixState=%d: DOP.Pos=%v, want 1.72", tt.fixState, ne.DOP.Pos)
		}
		if !ne.DOP.Hor.IsSet() || ne.DOP.Hor.Get() != 1.25 {
			t.Errorf("fixState=%d: DOP.Hor=%v, want 1.25", tt.fixState, ne.DOP.Hor)
		}
		if !ne.DOP.Vert.IsSet() || ne.DOP.Vert.Get() != 1.20 {
			t.Errorf("fixState=%d: DOP.Vert=%v, want 1.20", tt.fixState, ne.DOP.Vert)
		}
	}
}

func TestQualityNavAutoDOPPrecedence(t *testing.T) {
	var ne gpsprot.NavEpochMsg
	ne.DOP.Pos.Set(1.50)
	ne.DOP.Hor.Set(0.90)
	ne.DOP.Vert.Set(1.10)
	m := &asbin.NavAuto{
		FixState: 4,
		PDOP:     172,
		HDOP:     125,
		VDOP:     120,
	}
	qualityNavAuto(&ne, m)
	if ne.DOP.Pos.Get() != 1.50 {
		t.Errorf("DOP.Pos=%v, want 1.50 (NAV-DOP precedence)", ne.DOP.Pos.Get())
	}
	if ne.DOP.Hor.Get() != 0.90 {
		t.Errorf("DOP.Hor=%v, want 0.90 (NAV-DOP precedence)", ne.DOP.Hor.Get())
	}
	if ne.DOP.Vert.Get() != 1.10 {
		t.Errorf("DOP.Vert=%v, want 1.10 (NAV-DOP precedence)", ne.DOP.Vert.Get())
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
