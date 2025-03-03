package mon

import (
	"github.com/jclark/satpulse/internal/ptime"
)

// avgFilter implements an Exponential Moving Average filter with an adaptive
// smoothing factor during initialization.
//
// It starts with minimal smoothing (high alpha values) when there are few samples,
// gradually increasing the amount of smoothing until reaching the target alpha.
// This approach prevents overweighting the first sample and provides a smooth
// transition to the desired level of filtering.
//
// The filter automatically resets when the era changes.
//
// The warmup period is automatically calculated based on the provided alpha value
// using the relationship: warmupCount = (2/alpha)-1, which corresponds to the
// effective window size of the EMA.
type avgFilter struct {
	alpha      float64   // Target smoothing factor after warmup
	value      float64   // Current EMA value
	initVals []float64 // Raw values during warmup phase
	era        ptime.Era // Current era
}

func newAvgFilter(alpha float64) *avgFilter {
	return &avgFilter{
		alpha:      alpha,
		initVals: make([]float64, 0, warmupCount(alpha)),
	}
}

func (af *avgFilter) reset() {
	af.initVals = make([]float64, 0, warmupCount(af.alpha))
}

func warmupCount(alpha float64) int {
	count := int((2.0 / alpha) - 1)
	if count < 3 {
		count = 3
	}
	return count
}

func (af *avgFilter) add(v float64, era ptime.Era) float64 {
	if era != af.era {
		af.era = era
		af.reset()
	} else if af.initVals == nil {
		// After warmup: use target alpha directly
		af.value = af.alpha*v + (1-af.alpha)*af.value
		return af.value
	}

	af.initVals = append(af.initVals, v)

	count := len(af.initVals)

	if count == 1 {
		return v
	}

	a := af.alpha

	// If we haven't completed warmup, compute an appropriate alpha
	// by inverting the formula in warmupCount.
	if count < cap(af.initVals) {
		a = 2.0 / (float64(len(af.initVals)) + 1)
	}

	// Compute the EMA using the alpha.
	ema := af.initVals[0]
	for i := 1; i < count; i++ {
		ema = a*af.initVals[i] + (1-a)*ema
	}

	if count == cap(af.initVals) {
		af.initVals = nil
	}
	af.value = ema
	return af.value
}
