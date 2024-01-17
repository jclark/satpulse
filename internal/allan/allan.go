package allan

import (
	"math"

	"golang.org/x/exp/constraints"
)

type Numeric interface {
	constraints.Integer | constraints.Float
}

// OverlapADev calculates overlapping Allan deviation of phase data.
// tau0 is the interval between samples in seconds, typically 1 second for PPS data.
// m is the averaging factor (tau is m*tau0).
// The length of phase should be greater than 2*m; if not, NaN is returned.
// T will typically be float64 or time.Duration; in the latter case,
// tau0 should be time.Second for PPS data.
func OverlapADev[T Numeric](phase []T, tau0 T, m int) float64 {
	if tau0 <= 0 {
		panic("tau0 must be positive")
	}
	if m <= 0 {
		panic("m must be positive")
	}
	// See Handbook of Frequency Stability Analysis, section 5.2.4
	nSamples := len(phase) - 2*m
	if nSamples <= 0 {
		return math.NaN()
	}
	sum := 0.0
	for i := 0; i < nSamples; i++ {
		v := float64(phase[i+2*m] - 2*phase[i+m] + phase[i])
		sum += v * v
	}
	tau := float64(m) * float64(tau0)
	return math.Sqrt(sum/(2*float64(nSamples))) / tau
}
