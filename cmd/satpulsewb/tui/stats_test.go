package tui

import (
	"math"
	"testing"

	"github.com/jclark/satpulse/gps/lib/geopos"
)

func TestPosStats(t *testing.T) {
	// Points at the equator/prime meridian, offset +-1 m along the
	// ECEF Y axis, which is due east there: horizontal distances are
	// exactly 1 m, vertical 0.
	center := geopos.ECEF{6378137, 0, 0}
	var s posStats
	s.add(geopos.ECEF{center[0], center[1] + 1, center[2]})
	s.add(geopos.ECEF{center[0], center[1] - 1, center[2]})
	r := s.compute()
	if r.N != 2 || !r.rotOK {
		t.Fatalf("N = %d rotOK = %v, want 2 true", r.N, r.rotOK)
	}
	approx := func(name string, got, want float64) {
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("%s = %g, want %g", name, got, want)
		}
	}
	approx("mean X", r.Mean[0], center[0])
	approx("mean Y", r.Mean[1], 0)
	approx("mean Z", r.Mean[2], 0)
	approx("RMSH", r.RMSH, 1)
	approx("RMSV", r.RMSV, 0)
	approx("RMS3D", r.RMS3D, 1)
	approx("CEP50", r.CEP50, 1)
	approx("CEP95", r.CEP95, 1)
	// An ARP 2 m east and 3 m up of the mean.
	d3, dh, dv := r.arpDist([3]float64{center[0] + 3, 2, 0})
	approx("ARP 3D", d3, math.Sqrt(13))
	approx("ARP horiz", dh, 2)
	approx("ARP vert", dv, 3)
}

func TestPosStatsCache(t *testing.T) {
	var s posStats
	s.add(geopos.ECEF{6378137, 0, 0})
	r1 := s.compute()
	r2 := s.compute()
	if !r1.rotOK || r1 != r2 {
		t.Errorf("cached result differs: %+v vs %+v", r1, r2)
	}
	s.reset()
	if r := s.compute(); r.N != 0 {
		t.Errorf("N after reset = %d, want 0", r.N)
	}
}
