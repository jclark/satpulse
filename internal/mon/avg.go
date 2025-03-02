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
	alpha       float64   // Target smoothing factor after warmup
	value       float64   // Current EMA value
	count       int       // Number of samples seen in current era
	warmupCount int       // Number of samples for warmup
	era         ptime.Era // Current era
}

func newAvgFilter(alpha float64) *avgFilter {
	// Calculate warmup period based on alpha
	warmupCount := int((2.0 / alpha) - 1)
	if warmupCount < 3 {
		warmupCount = 3
	}

	return &avgFilter{
		alpha:       alpha,
		warmupCount: warmupCount,
	}
}

func (af *avgFilter) reset() {
	af.count = 0
}

func (af *avgFilter) add(v float64, era ptime.Era) float64 {
	// First sample after reset: just use the raw value
	if af.count == 0 || era != af.era {
		af.era = era
		af.count = 1
		af.value = v
		return v
	}

	// Calculate current alpha based on our position in the warmup period
	var a float64
	if af.count < af.warmupCount {
		// Using the inverse of the warmupCount formula: a = 2/(n+1)
		// This gives higher alpha values for smaller count values
		a = 2.0 / (float64(af.count) + 1)
	} else {
		// After warmup: use target alpha
		a = af.alpha
	}

	// Apply EMA with current alpha
	af.value = a*v + (1-a)*af.value

	af.count++
	return af.value
}
