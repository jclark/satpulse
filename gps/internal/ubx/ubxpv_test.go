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
	got := posECEFNavPosECEF(m)
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
	wantPAcc := opt.Make(gpsprot.Length(1543) * gpsprot.Centimeter)
	if got.PAcc != wantPAcc {
		t.Errorf("PAcc = %v, want %v", got.PAcc, wantPAcc)
	}
	if got.NativeMsgID != "UBX-NAV-POSECEF" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "UBX-NAV-POSECEF")
	}
}

func TestVelECEF(t *testing.T) {
	m := &ubxbin.NavVelECEF{
		ECEFV: [3]int32{-15, 23, -8}, // cm/s
		SAcc:  5,                      // cm/s
	}
	got := velECEFNavVelECEF(m)
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
	wantSAcc := opt.Make(gpsprot.Speed(5) * gpsprot.CentimeterPerSecond)
	if got.SAcc != wantSAcc {
		t.Errorf("SAcc = %v, want %v", got.SAcc, wantSAcc)
	}
	if got.NativeMsgID != "UBX-NAV-VELECEF" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "UBX-NAV-VELECEF")
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
	got := posGeoNavPosLLH(m)
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
	wantHAcc := opt.Make(gpsprot.Length(1200) * gpsprot.Millimeter)
	if got.HAcc != wantHAcc {
		t.Errorf("HAcc = %v, want %v", got.HAcc, wantHAcc)
	}
	wantVAcc := opt.Make(gpsprot.Length(1800) * gpsprot.Millimeter)
	if got.VAcc != wantVAcc {
		t.Errorf("VAcc = %v, want %v", got.VAcc, wantVAcc)
	}
	if got.NativeMsgID != "UBX-NAV-POSLLH" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "UBX-NAV-POSLLH")
	}
}

func TestVelNED(t *testing.T) {
	m := &ubxbin.NavVelNED{
		VelNED:  [3]int32{10, -5, 3},  // cm/s
		Speed:   12,                    // cm/s (3D)
		GSpeed:  11,                    // cm/s (2D)
		Heading: 18045000,              // 1e-5 deg (180.45 degrees)
		SAcc:    4,                     // cm/s
		CAcc:    500000,               // 1e-5 deg (5.0 degrees)
	}
	got := velGeoNavVelNED(m)
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
	if got.Speed != wantSpeed {
		t.Errorf("Speed = %v, want %v", got.Speed, wantSpeed)
	}
	wantGSpeed := opt.Make(gpsprot.Speed(11) * gpsprot.CentimeterPerSecond)
	if got.GroundSpeed != wantGSpeed {
		t.Errorf("GroundSpeed = %v, want %v", got.GroundSpeed, wantGSpeed)
	}
	wantHeading := opt.Make(gpsprot.Angle(18045000) * 10000)
	if got.Heading != wantHeading {
		t.Errorf("Heading = %v, want %v", got.Heading, wantHeading)
	}
	wantSAcc := opt.Make(gpsprot.Speed(4) * gpsprot.CentimeterPerSecond)
	if got.SAcc != wantSAcc {
		t.Errorf("SAcc = %v, want %v", got.SAcc, wantSAcc)
	}
	wantHeadAcc := opt.Make(gpsprot.Angle(500000) * 10000)
	if got.HeadAcc != wantHeadAcc {
		t.Errorf("HeadAcc = %v, want %v", got.HeadAcc, wantHeadAcc)
	}
	if got.NativeMsgID != "UBX-NAV-VELNED" {
		t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "UBX-NAV-VELNED")
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
		got := posGeoNavPVT(m)
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
		if got.NativeMsgID != "UBX-NAV-PVT" {
			t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "UBX-NAV-PVT")
		}
	})
	t.Run("no_fix", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVTNoFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
		}
		if got := posGeoNavPVT(m); got != nil {
			t.Errorf("expected nil for no fix, got %v", got)
		}
	})
	t.Run("fix_not_ok", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT3DFix,
			Flags:   0, // GNSSFixOK not set
		}
		if got := posGeoNavPVT(m); got != nil {
			t.Errorf("expected nil when GNSSFixOK not set, got %v", got)
		}
	})
	t.Run("invalid_llh", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT3DFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
			Flags3:  ubxbin.NavPVTInvalidLlh,
		}
		if got := posGeoNavPVT(m); got != nil {
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
		got := posGeoNavPVT(m)
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
			VelN:    1500,    // mm/s
			VelE:    -800,    // mm/s
			VelD:    200,     // mm/s
			GSpeed:  1700,    // mm/s
			HeadMot: 18045000, // 1e-5 deg (180.45 degrees)
			SAcc:    500,     // mm/s
			HeadAcc: 100000,  // 1e-5 deg (1.0 degrees)
		}
		got := velGeoNavPVT(m)
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
		if got.Heading != wantHeading {
			t.Errorf("Heading = %v, want %v", got.Heading, wantHeading)
		}
		wantSAcc := opt.Make(gpsprot.Speed(500) * gpsprot.MillimeterPerSecond)
		if got.SAcc != wantSAcc {
			t.Errorf("SAcc = %v, want %v", got.SAcc, wantSAcc)
		}
		wantHeadAcc := opt.Make(gpsprot.Angle(100000) * 10000)
		if got.HeadAcc != wantHeadAcc {
			t.Errorf("HeadAcc = %v, want %v", got.HeadAcc, wantHeadAcc)
		}
		if got.NativeMsgID != "UBX-NAV-PVT" {
			t.Errorf("NativeMsgID = %q, want %q", got.NativeMsgID, "UBX-NAV-PVT")
		}
	})
	t.Run("no_fix", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVTNoFix,
			Flags:   ubxbin.NavPVTGNSSFixOK,
		}
		if got := velGeoNavPVT(m); got != nil {
			t.Errorf("expected nil for no fix, got %v", got)
		}
	})
	t.Run("fix_not_ok", func(t *testing.T) {
		m := &ubxbin.NavPVT{
			FixType: ubxbin.NavPVT3DFix,
			Flags:   0,
		}
		if got := velGeoNavPVT(m); got != nil {
			t.Errorf("expected nil when GNSSFixOK not set, got %v", got)
		}
	})
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
	var positions []geopos.ECEF
	for _, h := range testPosECEFHex {
		m := parseTestMsg(t, h).(*ubxbin.NavPosECEF)
		p := posECEFNavPosECEF(m)
		positions = append(positions, geopos.ECEF{
			float64(p.Pos[0]) / float64(gpsprot.Meter),
			float64(p.Pos[1]) / float64(gpsprot.Meter),
			float64(p.Pos[2]) / float64(gpsprot.Meter),
		})
	}
	for _, h := range testPosLLHHex {
		m := parseTestMsg(t, h).(*ubxbin.NavPosLLH)
		positions = append(positions, llhToECEF(posGeoNavPosLLH(m)))
	}
	for _, h := range testPVTHex {
		m := parseTestMsg(t, h).(*ubxbin.NavPVT)
		if p := posGeoNavPVT(m); p != nil {
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
