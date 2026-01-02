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

// Accum is a specialized incremental ADEV calculator for m=1.
type Accum[T Numeric] struct {
	tau0     float64
	prev     float64
	prevPrev float64
	samples  int64
	sumSq    float64
}

// NewAccum creates a stream for consecutive samples (m=1).
func NewAccum[T Numeric](tau0 T) *Accum[T] {
	return &Accum[T]{
		tau0: float64(tau0),
	}
}

func (s *Accum[T]) Update(val T) {
	current := float64(val)

	// We need 2 prior samples (index 0 and 1) to form the first 2nd-difference
	if s.samples >= 2 {
		// D = x[i] - 2x[i-1] + x[i-2]
		v := current - (2 * s.prev) + s.prevPrev
		s.sumSq += v * v
	}

	// Shift history
	s.prevPrev = s.prev
	s.prev = current
	s.samples++
}

func (s *Accum[T]) ADev() float64 {
	count := s.samples - 2
	if count <= 0 {
		return math.NaN()
	}
	// ADEV formula for m=1
	return math.Sqrt(s.sumSq/(2.0*float64(count))) / s.tau0
}