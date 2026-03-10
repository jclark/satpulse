package ubx

import (
	"encoding/hex"
	"math"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func TestPosECEF(t *testing.T) {
	m := &ubxbin.NavPosECEF{
		ECEF: [3]int32{-267173351, -402753274, 391919498}, // cm
		PAcc: 1543,                                        // cm
	}
	var ne gpsprot.NavEpochMsg
	got := posECEFNavPosECEF(&ne, m)
	if got == nil {
		t.Fatal("expected non-nil PosECEFMsg")
	}
	want := gpsprot.Point3D{
		gpsprot.Length(-267173351) * gpsprot.Centimeter,
		gpsprot.Length(-402753274) * gpsprot.Centimeter,
		gpsprot.Length(391919498) * gpsprot.Centimeter,
	}
	if got.Pos != want {
		t.Errorf("Pos = %v, want %v", got.Pos, want)
	}
	if got.NativeMsgID != "NAV-POSECEF" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "NAV-POSECEF")
	}
	wantPos := opt.Make(gpsprot.Length(1543) * gpsprot.Centimeter)
	if ne.Acc.Pos != wantPos {
		t.Errorf("Acc.Pos = %v, want %v", ne.Acc.Pos, wantPos)
	}
}

func TestVelECEF(t *testing.T) {
	m := &ubxbin.NavVelECEF{
		ECEFV: [3]int32{-15, 23, -8}, // cm/s
		SAcc:  5,                     // cm/s
	}
	var ne gpsprot.NavEpochMsg
	got := velECEFNavVelECEF(&ne, m)
	if got == nil {
		t.Fatal("expected non-nil VelECEFMsg")
	}
	wantVel := [3]gpsprot.Speed{
		gpsprot.Speed(-15) * gpsprot.CentimeterPerSecond,
		gpsprot.Speed(23) * gpsprot.CentimeterPerSecond,
		gpsprot.Speed(-8) * gpsprot.CentimeterPerSecond,
	}
	if got.Vel != wantVel {
		t.Errorf("Vel = %v, want %v", got.Vel, wantVel)
	}
	if got.NativeMsgID != "NAV-VELECEF" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "NAV-VELECEF")
	}
	wantSpeed := opt.Make(gpsprot.Speed(5) * gpsprot.CentimeterPerSecond)
	if ne.Acc.Speed != wantSpeed {
		t.Errorf("Acc.Speed = %v, want %v", ne.Acc.Speed, wantSpeed)
	}
}

func TestPosLLH(t *testing.T) {
	m := &ubxbin.NavPosLLH{
		Lat:    474900000, // 47.49 deg (1e-7 degrees)
		Lon:    85600000,  // 8.56 deg
		Height: 540123,    // mm
		HMSL:   489456,    // mm
		HAcc:   1200,      // mm
		VAcc:   1800,      // mm
	}
	var ne gpsprot.NavEpochMsg
	got := posGeoNavPosLLH(&ne, m)
	if got == nil {
		t.Fatal("expected non-nil PosGeoMsg")
	}
	wantLat := gpsprot.Angle(474900000) * 100
	wantLon := gpsprot.Angle(85600000) * 100
	if got.LatLon[0] != wantLat || got.LatLon[1] != wantLon {
		t.Errorf("LatLon = %v, want [%v, %v]", got.LatLon, wantLat, wantLon)
	}
	wantH := opt.Make(gpsprot.Length(540123) * gpsprot.Millimeter)
	if got.Height != wantH {
		t.Errorf("Height = %v, want %v", got.Height, wantH)
	}
	wantHMSL := opt.Make(gpsprot.Length(489456) * gpsprot.Millimeter)
	if got.HeightMSL != wantHMSL {
		t.Errorf("HeightMSL = %v, want %v", got.HeightMSL, wantHMSL)
	}
	if got.NativeMsgID != "NAV-POSLLH" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "NAV-POSLLH")
	}
	wantHor := opt.Make(gpsprot.Length(1200) * gpsprot.Millimeter)
	wantVert := opt.Make(gpsprot.Length(1800) * gpsprot.Millimeter)
	if ne.Acc.Hor != wantHor {
		t.Errorf("Acc.Hor = %v, want %v", ne.Acc.Hor, wantHor)
	}
	if ne.Acc.Vert != wantVert {
		t.Errorf("Acc.Vert = %v, want %v", ne.Acc.Vert, wantVert)
	}
}

func TestVelNED(t *testing.T) {
	m := &ubxbin.NavVelNED{
		VelNED:  [3]int32{10, -5, 3}, // cm/s
		Speed:   12,                  // cm/s (3D)
		GSpeed:  11,                  // cm/s (2D)
		Heading: 18045000,            // 1e-5 deg (180.45 degrees)
		SAcc:    4,                   // cm/s
		CAcc:    500000,              // 1e-5 deg (5.0 degrees)
	}
	var ne gpsprot.NavEpochMsg
	got := velGeoNavVelNED(&ne, m)
	if got == nil {
		t.Fatal("expected non-nil VelGeoMsg")
	}
	wantNED := opt.Make([3]gpsprot.Speed{
		gpsprot.Speed(10) * gpsprot.CentimeterPerSecond,
		gpsprot.Speed(-5) * gpsprot.CentimeterPerSecond,
		gpsprot.Speed(3) * gpsprot.CentimeterPerSecond,
	})
	if got.VelNED != wantNED {
		t.Errorf("VelNED = %v, want %v", got.VelNED, wantNED)
	}
	wantSpeed := opt.Make(gpsprot.Speed(12) * gpsprot.CentimeterPerSecond)
	if got.Speed3D != wantSpeed {
		t.Errorf("Speed = %v, want %v", got.Speed3D, wantSpeed)
	}
	wantGSpeed := opt.Make(gpsprot.Speed(11) * gpsprot.CentimeterPerSecond)
	if got.GroundSpeed != wantGSpeed {
		t.Errorf("GroundSpeed = %v, want %v", got.GroundSpeed, wantGSpeed)
	}
	wantHeading := opt.Make(gpsprot.Angle(18045000) * 10000)
	if got.Course != wantHeading {
		t.Errorf("Heading = %v, want %v", got.Course, wantHeading)
	}
	if got.NativeMsgID != "NAV-VELNED" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "NAV-VELNED")
	}
	wantSAcc := opt.Make(gpsprot.Speed(4) * gpsprot.CentimeterPerSecond)
	wantCourse := opt.Make(gpsprot.Angle(500000) * 10000)
	if ne.Acc.Speed != wantSAcc {
		t.Errorf("Acc.Speed = %v, want %v", ne.Acc.Speed, wantSAcc)
	}
	if ne.Acc.Course != wantCourse {
		t.Errorf("Acc.Course = %v, want %v", ne.Acc.Course, wantCourse)
	}
}

func TestPosGeoNavPVT(t *testing.T) {
	t.Run("valid_3d_fix", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT3DFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
			Lat:     474900000, // 1e-7 deg
			Lon:     85600000,
			Height:  540123, // mm
			HMSL:    489456,
			HAcc:    1200, // mm
			VAcc:    1800,
		}
		var ne gpsprot.NavEpochMsg
		got := posGeoNavPVT(&ne, m)
		if got == nil {
			t.Fatal("expected non-nil PosGeoMsg for valid 3D fix")
		}
		wantLat := gpsprot.Angle(474900000) * 100
		wantLon := gpsprot.Angle(85600000) * 100
		if got.LatLon[0] != wantLat || got.LatLon[1] != wantLon {
			t.Errorf("LatLon = %v, want [%v, %v]", got.LatLon, wantLat, wantLon)
		}
		wantH := opt.Make(gpsprot.Length(540123) * gpsprot.Millimeter)
		if got.Height != wantH {
			t.Errorf("Height = %v, want %v", got.Height, wantH)
		}
		if got.NativeMsgID != "NAV-PVT" {
			t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "NAV-PVT")
		}
		wantHor := opt.Make(gpsprot.Length(1200) * gpsprot.Millimeter)
		wantVert := opt.Make(gpsprot.Length(1800) * gpsprot.Millimeter)
		if ne.Acc.Hor != wantHor {
			t.Errorf("Acc.Hor = %v, want %v", ne.Acc.Hor, wantHor)
		}
		if ne.Acc.Vert != wantVert {
			t.Errorf("Acc.Vert = %v, want %v", ne.Acc.Vert, wantVert)
		}
	})
	t.Run("no_fix", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVTNoFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
		}
		var ne gpsprot.NavEpochMsg
		if got := posGeoNavPVT(&ne, m); got != nil {
			t.Errorf("expected nil for no fix, got %v", got)
		}
	})
	t.Run("fix_not_ok", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT3DFix,
			Flags:   0, // GNSSFixOK not set
		}
		var ne gpsprot.NavEpochMsg
		if got := posGeoNavPVT(&ne, m); got != nil {
			t.Errorf("expected nil when GNSSFixOK not set, got %v", got)
		}
	})
	t.Run("invalid_llh", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT3DFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
			Flags3:  ubxbin.NavPVTInvalidLlh,
		}
		var ne gpsprot.NavEpochMsg
		if got := posGeoNavPVT(&ne, m); got != nil {
			t.Errorf("expected nil when InvalidLlh set, got %v", got)
		}
	})
	t.Run("2d_fix_valid", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT2DFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
			Lat:     474900000,
			Lon:     85600000,
		}
		var ne gpsprot.NavEpochMsg
		got := posGeoNavPVT(&ne, m)
		if got == nil {
			t.Fatal("expected non-nil PosGeoMsg for 2D fix")
		}
	})
}

func TestVelGeoNavPVT(t *testing.T) {
	t.Run("valid_fix", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT3DFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
			VelN:    1500,     // mm/s
			VelE:    -800,     // mm/s
			VelD:    200,      // mm/s
			GSpeed:  1700,     // mm/s
			HeadMot: 18045000, // 1e-5 deg (180.45 degrees)
			SAcc:    500,      // mm/s
			HeadAcc: 100000,   // 1e-5 deg (1.0 degrees)
		}
		var ne gpsprot.NavEpochMsg
		got := velGeoNavPVT(&ne, m)
		if got == nil {
			t.Fatal("expected non-nil VelGeoMsg for valid fix")
		}
		wantNED := opt.Make([3]gpsprot.Speed{
			gpsprot.Speed(1500) * gpsprot.MillimeterPerSecond,
			gpsprot.Speed(-800) * gpsprot.MillimeterPerSecond,
			gpsprot.Speed(200) * gpsprot.MillimeterPerSecond,
		})
		if got.VelNED != wantNED {
			t.Errorf("VelNED = %v, want %v", got.VelNED, wantNED)
		}
		wantGSpeed := opt.Make(gpsprot.Speed(1700) * gpsprot.MillimeterPerSecond)
		if got.GroundSpeed != wantGSpeed {
			t.Errorf("GroundSpeed = %v, want %v", got.GroundSpeed, wantGSpeed)
		}
		wantHeading := opt.Make(gpsprot.Angle(18045000) * 10000)
		if got.Course != wantHeading {
			t.Errorf("Heading = %v, want %v", got.Course, wantHeading)
		}
		if got.NativeMsgID != "NAV-PVT" {
			t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "NAV-PVT")
		}
		wantSpeed := opt.Make(gpsprot.Speed(500) * gpsprot.MillimeterPerSecond)
		wantCourse := opt.Make(gpsprot.Angle(100000) * 10000)
		if ne.Acc.Speed != wantSpeed {
			t.Errorf("Acc.Speed = %v, want %v", ne.Acc.Speed, wantSpeed)
		}
		if ne.Acc.Course != wantCourse {
			t.Errorf("Acc.Course = %v, want %v", ne.Acc.Course, wantCourse)
		}
	})
	t.Run("no_fix", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVTNoFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
		}
		var ne gpsprot.NavEpochMsg
		if got := velGeoNavPVT(&ne, m); got != nil {
			t.Errorf("expected nil for no fix, got %v", got)
		}
	})
	t.Run("fix_not_ok", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT3DFix,
			Flags:   0,
		}
		var ne gpsprot.NavEpochMsg
		if got := velGeoNavPVT(&ne, m); got != nil {
			t.Errorf("expected nil when GNSSFixOK not set, got %v", got)
		}
	})
}

// Real packets captured from ZED-F9P (same session, London area).
var testHPPosECEFHex = "b56201131c0000000000d819331caf73b617cf14f0ff25ad9d1d1d300c00b0370000fdd0"
var testHPPosLLHHex = "b5620114240000000000d819331cf112e9ffe8e9b21ebc7901007bc70000ef0a0201891f0000e62d00003412"

func TestHPPosECEF(t *testing.T) {
	m := parseTestMsg(t, testHPPosECEFHex).(*ubxbin.NavHPPosECEF)
	var ne gpsprot.NavEpochMsg
	got := posECEFNavHPPosECEF(&ne, m)
	if got == nil {
		t.Fatal("expected non-nil PosECEFMsg")
	}
	if got.NativeMsgID != "NAV-HPPOSECEF" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "NAV-HPPOSECEF")
	}
	if got.Priority != gpsprot.PriVendorHigh {
		t.Errorf("Priority = %v, want PriVendorHigh", got.Priority)
	}
	// ECEF X ~3978 km: plausible for Earth surface
	xm := float64(got.Pos[0]) / float64(gpsprot.Meter)
	if xm < 3900000 || xm > 4100000 {
		t.Errorf("ECEF X = %.1f m, out of plausible range", xm)
	}
	if !ne.Acc.Pos.IsSet() {
		t.Error("Acc.Pos not set")
	}
}

func TestHPPosECEFInvalid(t *testing.T) {
	m := &ubxbin.NavHPPosECEF{Flags: ubxbin.NavHPPosECEFInvalidEcef}
	var ne gpsprot.NavEpochMsg
	if got := posECEFNavHPPosECEF(&ne, m); got != nil {
		t.Errorf("expected nil for invalid ECEF, got %v", got)
	}
}

func TestHPPosLLH(t *testing.T) {
	m := parseTestMsg(t, testHPPosLLHHex).(*ubxbin.NavHPPosLLH)
	var ne gpsprot.NavEpochMsg
	got := posGeoNavHPPosLLH(&ne, m)
	if got == nil {
		t.Fatal("expected non-nil PosGeoMsg")
	}
	if got.NativeMsgID != "NAV-HPPOSLLH" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "NAV-HPPOSLLH")
	}
	if got.Priority != gpsprot.PriVendorHigh {
		t.Errorf("Priority = %v, want PriVendorHigh", got.Priority)
	}
	// Lat ~51.5 deg (London area)
	lat := float64(got.LatLon[0]) / float64(gpsprot.Degrees)
	if lat < 51.0 || lat > 52.0 {
		t.Errorf("Lat = %.6f deg, out of plausible range", lat)
	}
	// Lon small negative (west of Greenwich)
	lon := float64(got.LatLon[1]) / float64(gpsprot.Degrees)
	if lon > 0 || lon < -1.0 {
		t.Errorf("Lon = %.6f deg, out of plausible range", lon)
	}
	if !got.Height.IsSet() {
		t.Error("Height not set")
	}
	if !got.HeightMSL.IsSet() {
		t.Error("HeightMSL not set")
	}
	if !ne.Acc.Hor.IsSet() {
		t.Error("Acc.Hor not set")
	}
	if !ne.Acc.Vert.IsSet() {
		t.Error("Acc.Vert not set")
	}
}

func TestHPPosLLHInvalid(t *testing.T) {
	m := &ubxbin.NavHPPosLLH{Flags: ubxbin.NavHPPosLLHInvalidLlh}
	var ne gpsprot.NavEpochMsg
	if got := posGeoNavHPPosLLH(&ne, m); got != nil {
		t.Errorf("expected nil for invalid LLH, got %v", got)
	}
}

const posConsistencyTolerance = 0.01 // meters

// Captured from a stationary ZED-F9P antenna (one hex-encoded UBX packet per line).
var testPosECEFHex = []string{
	"b5620101140008259e1b61542df9da1d4d244f2ff7084d00000009e8",
	"b56201011400f0289e1b61542df9da1d4d244f2ff7084d000000f441",
	"b56201011400d82c9e1b61542df9da1d4d244f2ff7084d000000e0ad",
	"b56201011400c0309e1b61542df9da1d4d244f2ff7084d000000cc19",
	"b56201011400a8349e1b61542df9da1d4d244f2ff7084d000000b885",
}
var testPosLLHHex = []string{
	"b56201021c0048449e1bed2afd3b25502f0870a0ffffa60b000078020000bf01000058f1",
	"b56201021c0030489e1bed2afd3b25502f0870a0ffffa60b000078020000bf01000044bd",
	"b56201021c00184c9e1bed2afd3b25502f0870a0ffffa60b000078020000bf0100003089",
	"b56201021c0000509e1bed2afd3b25502f0870a0ffffa60b000078020000bf0100001c55",
	"b56201021c00e8539e1bed2afd3b25502f0870a0ffffa60b000078020000bf0100000706",
}
var testPVTHex = []string{
	"b56201075c00586b9e1bea07020d082a1d3717000000917802000503ea1eed2afd3b25502f0870a0ffffa60b000078020000bf0100000000000000000000000000000000000000000000204e0000a671ef000f2700002c2e4c3300000000000000000f0e",
	"b56201075c00406f9e1bea07020d082a1e3717000000547702000503ea1eed2afd3b25502f0870a0ffffa60b000078020000bf0100000000000000000000000000000000000000000000204e0000a671ef000f2700002c2e4c330000000000000000bec5",
	"b56201075c0028739e1bea07020d082a1f3717000000177602000503ea1fed2afd3b25502f0870a0ffffa60b000078020000bf0100000000000000000000000000000000000000000000204e0000a671ef000f2700002c2e4c3300000000000000006ec1",
	"b56201075c0010779e1bea07020d082a203717000000d97402000503ea1fed2afd3b25502f0870a0ffffa60b000078020000bf0100000000000000000000000000000000000000000000204e0000a671ef000f2700002c2e4c3300000000000000001be1",
	"b56201075c00f87a9e1bea07020d082a2137170000009c7302000503ea1fed2afd3b25502f0870a0ffffa60b000078020000bf0100000000000000000000000000000000000000000000204e0000a671ef000f2700002c2e4c330000000000000000c93d",
}

// TestPosConsistency verifies that NAV-POSECEF, NAV-POSLLH, and NAV-PVT
// produce consistent ECEF positions when fed real captured data from a
// stationary antenna.
func TestPosConsistency(t *testing.T) {
	var ne gpsprot.NavEpochMsg
	var positions []geopos.ECEF
	for _, h := range testPosECEFHex {
		m := parseTestMsg(t, h).(*ubxbin.NavPosECEF)
		p := posECEFNavPosECEF(&ne, m)
		positions = append(positions, geopos.ECEF{
			float64(p.Pos[0]) / float64(gpsprot.Meter),
			float64(p.Pos[1]) / float64(gpsprot.Meter),
			float64(p.Pos[2]) / float64(gpsprot.Meter),
		})
	}
	for _, h := range testPosLLHHex {
		m := parseTestMsg(t, h).(*ubxbin.NavPosLLH)
		positions = append(positions, llhToECEF(posGeoNavPosLLH(&ne, m)))
	}
	for _, h := range testPVTHex {
		m := parseTestMsg(t, h).(*ubxbin.NavPVT)
		if p := posGeoNavPVT(&ne, m); p != nil {
			positions = append(positions, llhToECEF(p))
		}
	}
	if len(positions) == 0 {
		t.Fatal("no positions collected")
	}
	var cx, cy, cz float64
	for _, p := range positions {
		cx += p[0]
		cy += p[1]
		cz += p[2]
	}
	n := float64(len(positions))
	cx /= n
	cy /= n
	cz /= n
	for i, p := range positions {
		dx, dy, dz := p[0]-cx, p[1]-cy, p[2]-cz
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist > posConsistencyTolerance {
			t.Errorf("position %d: %.3f m from centroid (tolerance %.1f m)", i, dist, posConsistencyTolerance)
		}
	}
	t.Logf("checked %d positions, centroid (%.2f, %.2f, %.2f)", len(positions), cx, cy, cz)
}

func llhToECEF(p *gpsprot.PosGeoMsg) geopos.ECEF {
	return geopos.WGS84.LLHtoECEF(geopos.LLH{
		Lat:    float64(p.LatLon[0]) / float64(gpsprot.Degrees),
		Lon:    float64(p.LatLon[1]) / float64(gpsprot.Degrees),
		Height: float64(p.Height.Get()) / float64(gpsprot.Meter),
	})
}

func parseTestMsg(t *testing.T, h string) ubxbin.Msg {
	t.Helper()
	b, err := hex.DecodeString(h)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	m, err := ubxbin.ParseMsg(string(b))
	if err != nil {
		t.Fatalf("ParseMsg: %v", err)
	}
	return m
}

func TestQualityNavPVT(t *testing.T) {
	tests := []struct {
		name       string
		m          ubxbin.NavPVT
		wantLevel  gpsprot.FixLevel
		wantDim    gpsprot.SolutionDim
		wantCorr   gpsprot.CorrKind
		wantAux    gpsprot.AuxSrc
		wantNumSV  uint16
		wantPDOP   float64
		wantAge    gpsprot.Duration
		wantAgeSet bool
	}{
		{
			name:      "no fix",
			m:         ubxbin.NavPVT{FixType: ubxbin.NavPVTNoFix, NumSV: 0, PDOP: 10000},
			wantLevel: gpsprot.FixLevelNone,
			wantPDOP:  100.00,
		},
		{
			name: "3D fix, code only",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK,
				NumSV:   12, PDOP: 150,
			},
			wantLevel: gpsprot.FixLevelCode,
			wantDim:   gpsprot.SolutionDim3D,
			wantNumSV: 12, wantPDOP: 1.50,
		},
		{
			name: "3D fix, diff corrections",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK | ubxbin.NavPVTDiffSoln,
				NumSV:   10, PDOP: 120,
			},
			wantLevel: gpsprot.FixLevelCodeCorrected,
			wantDim:   gpsprot.SolutionDim3D,
			wantCorr:  gpsprot.CorrUsed,
			wantNumSV: 10, wantPDOP: 1.20,
		},
		{
			name: "3D fix, carrier float",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK | ubxbin.NavPVTCarrSolnFloat,
				NumSV:   15, PDOP: 100,
			},
			wantLevel: gpsprot.FixLevelCarrierFloat,
			wantDim:   gpsprot.SolutionDim3D,
			wantCorr:  gpsprot.CorrUsed,
			wantNumSV: 15, wantPDOP: 1.00,
		},
		{
			name: "3D fix, carrier fixed",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK | ubxbin.NavPVTCarrSolnFixed,
				NumSV:   20, PDOP: 80,
			},
			wantLevel: gpsprot.FixLevelCarrierFixed,
			wantDim:   gpsprot.SolutionDim3D,
			wantCorr:  gpsprot.CorrUsed,
			wantNumSV: 20, wantPDOP: 0.80,
		},
		{
			name: "2D fix",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT2DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK,
				NumSV:   3, PDOP: 500,
			},
			wantLevel: gpsprot.FixLevelCode,
			wantDim:   gpsprot.SolutionDim2D,
			wantNumSV: 3, wantPDOP: 5.00,
		},
		{
			name: "dead reckoning only",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVTDeadReckoningOnly,
				Flags:   ubxbin.NavPVTGNSSFixOK,
			},
			wantLevel: gpsprot.FixLevelNone,
			wantAux:   gpsprot.AuxSrcDR,
		},
		{
			name: "GNSS + dead reckoning",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVTGNSSDeadReckoning,
				Flags:   ubxbin.NavPVTGNSSFixOK,
				NumSV:   8, PDOP: 200,
			},
			wantLevel: gpsprot.FixLevelCode,
			wantDim:   gpsprot.SolutionDim3D,
			wantAux:   gpsprot.AuxSrcDR,
			wantNumSV: 8, wantPDOP: 2.00,
		},
		{
			name: "time only fix",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVTTimeOnlyFix,
				Flags:   ubxbin.NavPVTGNSSFixOK,
				NumSV:   1, PDOP: 10000,
			},
			wantLevel: gpsprot.FixLevelCode,
			wantDim:   gpsprot.SolutionDimTimeOnly,
			wantNumSV: 1, wantPDOP: 100.00,
		},
		{
			name: "HeadVehValid sets AuxSrcINS",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK | ubxbin.NavPVTHeadVehValid,
				NumSV:   10, PDOP: 150,
			},
			wantLevel: gpsprot.FixLevelCode,
			wantDim:   gpsprot.SolutionDim3D,
			wantAux:   gpsprot.AuxSrcINS,
			wantNumSV: 10, wantPDOP: 1.50,
		},
		{
			name: "GNSS+DR with HeadVehValid",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVTGNSSDeadReckoning,
				Flags:   ubxbin.NavPVTGNSSFixOK | ubxbin.NavPVTHeadVehValid,
				NumSV:   6, PDOP: 300,
			},
			wantLevel: gpsprot.FixLevelCode,
			wantDim:   gpsprot.SolutionDim3D,
			wantAux:   gpsprot.AuxSrcDR | gpsprot.AuxSrcINS,
			wantNumSV: 6, wantPDOP: 3.00,
		},
		{
			name: "dead reckoning gnssFixOK not set",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVTDeadReckoningOnly,
				Flags:   0,
			},
			wantLevel: gpsprot.FixLevelNone,
			wantAux:   gpsprot.AuxSrcDR,
		},
		{
			name: "3D gnssFixOK not set",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   0,
				NumSV:   10, PDOP: 150,
			},
			wantLevel: gpsprot.FixLevelNone,
			wantDim:   0,
			wantNumSV: 10, wantPDOP: 1.50,
		},
		{
			name: "correction age 1-2s",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK | ubxbin.NavPVTDiffSoln,
				Flags3:  ubxbin.NavPVTLastCorrectionAge1to2,
				NumSV:   10, PDOP: 120,
			},
			wantLevel: gpsprot.FixLevelCodeCorrected,
			wantDim:   gpsprot.SolutionDim3D,
			wantCorr:  gpsprot.CorrUsed,
			wantNumSV: 10, wantPDOP: 1.20,
			wantAge: 1 * gpsprot.Second, wantAgeSet: true,
		},
		{
			name: "correction age 120+",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK | ubxbin.NavPVTCarrSolnFixed,
				Flags3:  ubxbin.NavPVTLastCorrectionAge120Plus,
				NumSV:   15, PDOP: 90,
			},
			wantLevel: gpsprot.FixLevelCarrierFixed,
			wantDim:   gpsprot.SolutionDim3D,
			wantCorr:  gpsprot.CorrUsed,
			wantNumSV: 15, wantPDOP: 0.90,
			wantAge: 120 * gpsprot.Second, wantAgeSet: true,
		},
		{
			name: "correction age not available",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK,
				Flags3:  ubxbin.NavPVTLastCorrectionAgeNotAvailable,
				NumSV:   10, PDOP: 150,
			},
			wantLevel: gpsprot.FixLevelCode,
			wantDim:   gpsprot.SolutionDim3D,
			wantNumSV: 10, wantPDOP: 1.50,
		},
		{
			name: "correction age 0-1s",
			m: ubxbin.NavPVT{
				FixType: ubxbin.NavPVT3DFix,
				Flags:   ubxbin.NavPVTGNSSFixOK | ubxbin.NavPVTDiffSoln,
				Flags3:  ubxbin.NavPVTLastCorrectionAge0to1,
				NumSV:   10, PDOP: 100,
			},
			wantLevel: gpsprot.FixLevelCodeCorrected,
			wantDim:   gpsprot.SolutionDim3D,
			wantCorr:  gpsprot.CorrUsed,
			wantNumSV: 10, wantPDOP: 1.00,
			wantAge: 0, wantAgeSet: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ne gpsprot.NavEpochMsg
			qualityNavPVT(&ne, &tt.m)
			if ne.FixLevel != tt.wantLevel {
				t.Errorf("FixLevel = %v, want %v", ne.FixLevel, tt.wantLevel)
			}
			if ne.SolutionDim != tt.wantDim {
				t.Errorf("SolutionDim = %v, want %v", ne.SolutionDim, tt.wantDim)
			}
			if ne.Correction != tt.wantCorr {
				t.Errorf("Correction = %v, want %v", ne.Correction, tt.wantCorr)
			}
			if ne.AuxSrc != tt.wantAux {
				t.Errorf("AuxSrc = %v, want %v", ne.AuxSrc, tt.wantAux)
			}
			if !ne.NumSVUsed.IsSet() {
				t.Error("NumSVUsed not set")
			} else if ne.NumSVUsed.Get() != tt.wantNumSV {
				t.Errorf("NumSVUsed = %d, want %d", ne.NumSVUsed.Get(), tt.wantNumSV)
			}
			if !ne.DOP.Pos.IsSet() {
				t.Error("DOP.Pos not set")
			} else if ne.DOP.Pos.Get() != tt.wantPDOP {
				t.Errorf("DOP.Pos = %v, want %v", ne.DOP.Pos.Get(), tt.wantPDOP)
			}
			if tt.wantAgeSet {
				if !ne.DiffAge.IsSet() {
					t.Error("DiffAge not set")
				} else if ne.DiffAge.Get() != tt.wantAge {
					t.Errorf("DiffAge = %v, want %v", ne.DiffAge.Get(), tt.wantAge)
				}
			} else if ne.DiffAge.IsSet() {
				t.Errorf("DiffAge set unexpectedly: %v", ne.DiffAge.Get())
			}
		})
	}
}

func TestDOPNavDOP(t *testing.T) {
	m := &ubxbin.NavDOP{
		GDOP: 250,
		PDOP: 200,
		HDOP: 120,
		VDOP: 150,
		TDOP: 180,
		NDOP: 90,
		EDOP: 80,
	}
	var ne gpsprot.NavEpochMsg
	dopNavDOP(&ne, m)
	checks := []struct {
		name string
		got  opt.Val[float64]
		want float64
	}{
		{"Geom", ne.DOP.Geom, 2.50},
		{"Pos", ne.DOP.Pos, 2.00},
		{"Hor", ne.DOP.Hor, 1.20},
		{"Vert", ne.DOP.Vert, 1.50},
		{"Time", ne.DOP.Time, 1.80},
		{"North", ne.DOP.North, 0.90},
		{"East", ne.DOP.East, 0.80},
	}
	for _, c := range checks {
		if !c.got.IsSet() {
			t.Errorf("DOP.%s not set", c.name)
		} else if c.got.Get() != c.want {
			t.Errorf("DOP.%s = %v, want %v", c.name, c.got.Get(), c.want)
		}
	}
}
