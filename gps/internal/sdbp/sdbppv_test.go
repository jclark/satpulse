package sdbp

import (
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

func TestPosGeoDatLLA3(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatLLA3{
		Valid: 1, FixSats: 12,
		Lat: 13.731845, Lon: 100.644758,
		AltMSL: 6.5, GeoidSep: -28.5,
		HAcc: 1500, VAcc: 2500,
	}
	msg := posGeoDatLLA3(ne, m)
	if msg == nil {
		t.Fatal("expected non-nil PosGeoMsg")
	}
	if msg.NativeMsgID != "DAT-LLA3" {
		t.Errorf("NativeMsgID = %q", msg.NativeMsgID)
	}
	// Lat should be ~13.731845 degrees = 13731845000 nanodegrees
	lat := float64(msg.LatLon[0]) / float64(gpsprot.Degrees)
	if lat < 13.73 || lat > 13.74 {
		t.Errorf("lat = %f, want ~13.73", lat)
	}
	lon := float64(msg.LatLon[1]) / float64(gpsprot.Degrees)
	if lon < 100.64 || lon > 100.65 {
		t.Errorf("lon = %f, want ~100.64", lon)
	}
	if !msg.HeightMSL.IsSet() {
		t.Error("HeightMSL not set")
	}
	// NumSVUsed should be set on ne
	if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 12 {
		t.Errorf("NumSVUsed = %v, want 12", ne.NumSVUsed)
	}
}

func TestPosGeoDatLLA3Invalid(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatLLA3{Valid: 0}
	if msg := posGeoDatLLA3(ne, m); msg != nil {
		t.Error("expected nil for Valid=0")
	}
}

func TestVelGeoDatLLA3(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatLLA3{
		Valid: 1, GroundSpeed: 0.5, Heading: 90.0,
		SpeedAcc: 10, HeadingAcc: 500,
	}
	msg := velGeoDatLLA3(ne, m)
	if msg == nil {
		t.Fatal("expected non-nil VelGeoMsg")
	}
	if !msg.GroundSpeed.IsSet() {
		t.Error("GroundSpeed not set")
	}
	if !msg.Course.IsSet() {
		t.Error("Course not set")
	}
}

func TestPosECEFDatECEF2(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatECEF2{
		Valid: 1, FixSats: 10,
		X: -1106227.0, Y: 5650780.0, Z: 1502567.0,
		XAcc: 2000, YAcc: 2000, ZAcc: 3000,
	}
	msg := posECEFDatECEF2(ne, m)
	if msg == nil {
		t.Fatal("expected non-nil PosECEFMsg")
	}
	// X should be ~ -1106227 m = -1106227000000 um
	xm := float64(msg.Pos[0]) / float64(gpsprot.Meter)
	if xm > -1e6 || xm < -1.2e6 {
		t.Errorf("X = %f m, want ~-1106227", xm)
	}
}

func TestVelECEFDatECEF2(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatECEF2{
		Valid: 1,
		VX: 0.01, VY: -0.02, VZ: 0.03,
		VXAcc: 5, VYAcc: 5, VZAcc: 5,
	}
	msg := velECEFDatECEF2(ne, m)
	if msg == nil {
		t.Fatal("expected non-nil VelECEFMsg")
	}
}

func TestVelGeoDatNED3(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatNED3{
		Valid: 1,
		VN: 0.01, VE: -0.02, VD: 0.005,
		VNAcc: 5, VEAcc: 5, VDAcc: 5,
	}
	msg := velGeoDatNED3(ne, m)
	if msg == nil {
		t.Fatal("expected non-nil VelGeoMsg")
	}
	if !msg.VelNED.IsSet() {
		t.Error("VelNED not set")
	}
	if !msg.GroundSpeed.IsSet() {
		t.Error("GroundSpeed not set")
	}
	if !msg.Speed3D.IsSet() {
		t.Error("Speed3D not set")
	}
}

func TestDopDatDOP(t *testing.T) {
	ne := &gpsprot.NavEpochMsg{}
	m := &sdbpbin.DatDOP{
		Valid: 1, FixSats: 12,
		GDOP: 150, PDOP: 120, HDOP: 80, VDOP: 90, TDOP: 110,
	}
	dopDatDOP(ne, m)
	if !ne.DOP.Pos.IsSet() || ne.DOP.Pos.Get() != 1.2 {
		t.Errorf("PDOP = %v, want 1.2", ne.DOP.Pos)
	}
	if !ne.DOP.Hor.IsSet() || ne.DOP.Hor.Get() != 0.8 {
		t.Errorf("HDOP = %v, want 0.8", ne.DOP.Hor)
	}
	if !ne.NumSVUsed.IsSet() || ne.NumSVUsed.Get() != 12 {
		t.Errorf("NumSVUsed = %v, want 12", ne.NumSVUsed)
	}
}

func TestDopDatDOPNilEpoch(t *testing.T) {
	m := &sdbpbin.DatDOP{Valid: 1, FixSats: 12, PDOP: 120}
	dopDatDOP(nil, m) // should not panic
}
