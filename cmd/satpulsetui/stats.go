package main

import (
	"math"
	"sort"

	"github.com/jclark/satpulse/gps/lib/geopos"
)

// maxStatsPoints caps the accumulated fixes, matching the workbench
// scatter panel; beyond it the oldest point is discarded and the mean
// recomputed from the survivors.
const maxStatsPoints = 1_000_000

// posStats accumulates ECEF fixes and computes the statistics the
// workbench scatter panel shows: running mean position and CEP50 /
// CEP95 / horizontal, vertical, and 3D RMS over the accumulated fixes.
type posStats struct {
	pts   []geopos.ECEF
	mean  [3]float64
	cache statsResult
	dirty bool
}

// statsResult is a snapshot of the derived statistics.
type statsResult struct {
	N            int
	Mean         [3]float64
	CEP50, CEP95 float64
	RMSH, RMSV   float64
	RMS3D        float64
	// rotation of the local ENU frame at the mean, for ARP distances
	sinLat, cosLat, sinLon, cosLon float64
	rotOK                          bool
}

func (s *posStats) add(p geopos.ECEF) {
	if len(s.pts) >= maxStatsPoints {
		s.pts = s.pts[1:]
		var sum [3]float64
		for _, q := range s.pts {
			sum[0] += q[0]
			sum[1] += q[1]
			sum[2] += q[2]
		}
		n := float64(len(s.pts))
		s.mean = [3]float64{sum[0] / n, sum[1] / n, sum[2] / n}
	}
	s.pts = append(s.pts, p)
	n := float64(len(s.pts))
	s.mean[0] += (p[0] - s.mean[0]) / n
	s.mean[1] += (p[1] - s.mean[1]) / n
	s.mean[2] += (p[2] - s.mean[2]) / n
	s.dirty = true
}

func (s *posStats) reset() {
	s.pts = nil
	s.mean = [3]float64{}
	s.cache = statsResult{}
	s.dirty = false
}

// enu rotates an ECEF offset from the mean into the local
// east/north/up frame.
func (r *statsResult) enu(dx, dy, dz float64) (e, n, u float64) {
	e = -r.sinLon*dx + r.cosLon*dy
	n = -r.sinLat*r.cosLon*dx - r.sinLat*r.sinLon*dy + r.cosLat*dz
	u = r.cosLat*r.cosLon*dx + r.cosLat*r.sinLon*dy + r.sinLat*dz
	return
}

// compute returns the statistics for the accumulated points,
// recomputing only when points were added since the last call.
func (s *posStats) compute() statsResult {
	if !s.dirty {
		return s.cache
	}
	s.dirty = false
	r := statsResult{N: len(s.pts), Mean: s.mean}
	if r.N == 0 {
		s.cache = r
		return r
	}
	llh, err := geopos.WGS84.ECEFtoLLH(geopos.ECEF(s.mean))
	if err != nil {
		s.cache = r
		return r
	}
	lat, lon := llh.Lat*math.Pi/180, llh.Lon*math.Pi/180
	r.sinLat, r.cosLat = math.Sin(lat), math.Cos(lat)
	r.sinLon, r.cosLon = math.Sin(lon), math.Cos(lon)
	r.rotOK = true
	hDist := make([]float64, r.N)
	var sumH2, sumU2 float64
	for i, p := range s.pts {
		e, n, u := r.enu(p[0]-s.mean[0], p[1]-s.mean[1], p[2]-s.mean[2])
		h2 := e*e + n*n
		sumH2 += h2
		sumU2 += u * u
		hDist[i] = math.Sqrt(h2)
	}
	nf := float64(r.N)
	r.RMSH = math.Sqrt(sumH2 / nf)
	r.RMSV = math.Sqrt(sumU2 / nf)
	r.RMS3D = math.Sqrt((sumH2 + sumU2) / nf)
	sort.Float64s(hDist)
	r.CEP50 = hDist[min(r.N/2, r.N-1)]
	r.CEP95 = hDist[min(r.N*95/100, r.N-1)]
	s.cache = r
	return r
}

// arpDist returns the 3D, horizontal, and vertical distance from the
// mean position to a base station ARP.
func (r *statsResult) arpDist(arp [3]float64) (d3, dh, dv float64) {
	dx, dy, dz := arp[0]-r.Mean[0], arp[1]-r.Mean[1], arp[2]-r.Mean[2]
	d3 = math.Sqrt(dx*dx + dy*dy + dz*dz)
	e, n, u := r.enu(dx, dy, dz)
	dh = math.Sqrt(e*e + n*n)
	dv = math.Abs(u)
	return
}
