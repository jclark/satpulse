package allan

import (
	"math"
)

// OverlapADev calculates overlapping Allan deviation of phase data.
// tau0 is the interval between samples in seconds, typically 1 for PPS data
// m is the averaging factor (tau is m*tau0)
func OverlapADev(phase []float64, tau0 float64, m int) float64 {
	nSamples := len(phase) - 2*m
	if nSamples <= 0 {
		return math.NaN()
	}
	sum := 0.0
	for i := 0; i < nSamples; i++ {
		v := phase[i+2*m] - 2*phase[i+m] + phase[i]
		sum += v * v
	}
	tau := float64(m) * tau0
	return math.Sqrt(sum/(2*float64(nSamples))) / tau
}
