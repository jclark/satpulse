package septentrio

import (
	"math"
	"testing"

	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

func approx(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (tol %v)", name, got, want, tol)
	}
}

// TestPosGeoPVTGeodetic checks the geodetic position, height, and the
// height-minus-undulation MSL height from a real PVTGeodetic block.
func TestPosGeoPVTGeodetic(t *testing.T) {
	b := findBlock(t, "pvt-cov.jsonl", "PVTGeodetic")
	m := b.Params.(*sbfbin.PVTGeodetic)
	msg := posGeoPVTGeodetic(m)
	if msg == nil {
		t.Fatal("posGeoPVTGeodetic returned nil")
	}
	approx(t, "lat", msg.LatLon[0].Degrees(), m.Latitude*180/math.Pi, 1e-6)
	approx(t, "lon", msg.LatLon[1].Degrees(), m.Longitude*180/math.Pi, 1e-6)
	if !msg.Height.IsSet() || !msg.HeightMSL.IsSet() {
		t.Fatal("Height/HeightMSL unset")
	}
	approx(t, "height", msg.Height.Get().Meters(), m.Height, 1e-3)
	approx(t, "heightMSL", msg.HeightMSL.Get().Meters(), m.Height-float64(m.Undulation), 1e-3)
}

// TestPosECEFPVTCartesian checks the ECEF position from a real PVTCartesian.
func TestPosECEFPVTCartesian(t *testing.T) {
	b := findBlock(t, "pvt-cov.jsonl", "PVTCartesian")
	m := b.Params.(*sbfbin.PVTCartesian)
	msg := posECEFPVTCartesian(m)
	if msg == nil {
		t.Fatal("nil")
	}
	approx(t, "X", msg.Pos[0].Meters(), m.X, 1e-3)
	approx(t, "Y", msg.Pos[1].Meters(), m.Y, 1e-3)
	approx(t, "Z", msg.Pos[2].Meters(), m.Z, 1e-3)
}

// TestVelGeoPVTGeodetic checks the NED velocity (Down = -Vu) and that Course
// stays unset when COG is at its do-not-use sentinel.
func TestVelGeoPVTGeodetic(t *testing.T) {
	b := findBlock(t, "pvt-cov.jsonl", "PVTGeodetic")
	m := b.Params.(*sbfbin.PVTGeodetic)
	msg := velGeoPVTGeodetic(m)
	if msg == nil || !msg.VelNED.IsSet() {
		t.Fatal("VelNED unset")
	}
	ned := msg.VelNED.Get()
	approx(t, "Vn", ned[0].MetersPerSecond(), float64(m.Vn), 1e-6)
	approx(t, "Ve", ned[1].MetersPerSecond(), float64(m.Ve), 1e-6)
	approx(t, "Vd", ned[2].MetersPerSecond(), float64(-m.Vu), 1e-6)
	if !msg.GroundSpeed.IsSet() {
		t.Error("GroundSpeed unset")
	}
	approx(t, "groundSpeed", msg.GroundSpeed.Get().MetersPerSecond(),
		math.Hypot(float64(m.Vn), float64(m.Ve)), 1e-6)
	if msg.Course.IsSet() { // COG is DNU in this capture
		t.Error("Course should be unset when COG is DNU")
	}
}

// TestVelECEFPVTCartesian checks the ECEF velocity from a real PVTCartesian.
func TestVelECEFPVTCartesian(t *testing.T) {
	b := findBlock(t, "pvt-cov.jsonl", "PVTCartesian")
	m := b.Params.(*sbfbin.PVTCartesian)
	msg := velECEFPVTCartesian(m)
	if msg == nil {
		t.Fatal("nil")
	}
	approx(t, "Vx", msg.Vel[0].MetersPerSecond(), float64(m.Vx), 1e-6)
	approx(t, "Vy", msg.Vel[1].MetersPerSecond(), float64(m.Vy), 1e-6)
	approx(t, "Vz", msg.Vel[2].MetersPerSecond(), float64(m.Vz), 1e-6)
}

// TestPosVelNoPVTSuppressed suppresses all pos/vel messages when Mode has no
// GNSS PVT and the fields are at their -2e10 sentinel.
func TestPosVelNoPVTSuppressed(t *testing.T) {
	var g sbfbin.PVTGeodetic
	g.Mode = sbfbin.ModeNoPVT
	g.Latitude, g.Longitude, g.Height = sbfbin.PVTCoordinateDNU, sbfbin.PVTCoordinateDNU, sbfbin.PVTCoordinateDNU
	g.Vn, g.Ve, g.Vu = sbfbin.PVTVelocityDNU, sbfbin.PVTVelocityDNU, sbfbin.PVTVelocityDNU
	if posGeoPVTGeodetic(&g) != nil || velGeoPVTGeodetic(&g) != nil {
		t.Error("no-PVT PVTGeodetic should suppress pos/vel")
	}
	var c sbfbin.PVTCartesian
	c.Mode = sbfbin.ModeNoPVT
	c.X, c.Y, c.Z = sbfbin.PVTCoordinateDNU, sbfbin.PVTCoordinateDNU, sbfbin.PVTCoordinateDNU
	c.Vx, c.Vy, c.Vz = sbfbin.PVTVelocityDNU, sbfbin.PVTVelocityDNU, sbfbin.PVTVelocityDNU
	if posECEFPVTCartesian(&c) != nil || velECEFPVTCartesian(&c) != nil {
		t.Error("no-PVT PVTCartesian should suppress pos/vel")
	}
}
