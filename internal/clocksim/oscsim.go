package clocksim

import (
	"math"
	"math/rand"
)

// OscSimulator models the behavior of a hardware oscillator.
//
// Input: t - simulation time in seconds since simulation start
// Output: fractional frequency error at that time
// Example: if output is +1e-6, the clock runs 1µs fast per second of true time
//
// IMPORTANT: When used with RawClock or SawtoothGPS, the simulator will be called with
// monotonically increasing simulation time values. This allows stateful simulators
// (like WhiteNoiseOsc, FlickerNoiseOsc) to use incremental random number
// generation without needing random access to arbitrary time points.
type OscSimulator func(t float64) float64

// FreqOffsetOsc creates an oscillator with constant frequency offset.
// ppb is the frequency offset in parts per billion (positive means runs fast).
func FreqOffsetOsc(ppb float64) OscSimulator {
	return func(t float64) float64 {
		return ppb / 1e9
	}
}

// PerfectOsc creates an oscillator with no frequency error.
func PerfectOsc() OscSimulator {
	return FreqOffsetOsc(0)
}

// WhiteNoiseOsc creates an oscillator with white frequency noise.
// stddevPPB is the standard deviation of frequency noise in ppb.
// Relies on RawClock's monotonic time guarantee: each call advances the RNG state
// incrementally without needing to seek to arbitrary time points.
func WhiteNoiseOsc(stddevPPB float64, seed int64) OscSimulator {
	rng := rand.New(rand.NewSource(seed))
	return func(t float64) float64 {
		return rng.NormFloat64() * stddevPPB / 1e9
	}
}

// FlickerNoiseOsc creates an oscillator with flicker frequency noise (1/f noise).
// Flicker FM produces a flat Allan deviation (τ^0 slope) at the specified stddevPPB level.
//
// This implementation uses the Kasdin-Walter algorithm: a sum of first-order recursive
// filters that approximates 1/f power spectral density. The Allan deviation is flat
// across tau values, matching the behavior extracted by phc_model.py.
//
// stddevPPB is the Allan deviation level in parts per billion (flat across tau).
//
// IMPORTANT: This implementation maintains state and requires monotonically increasing
// time values. RawClock guarantees this.
func FlickerNoiseOsc(stddevPPB float64, seed int64) OscSimulator {
	rng := rand.New(rand.NewSource(seed))

	// Kasdin-Walter recursive filter coefficients for 1/f noise
	// Flicker FM region is ~10-1000s based on clock-model ADEV analysis
	const tau0 = 10.0     // Start of flicker region (seconds)
	const tauMax = 1000.0 // End of flicker region (seconds)
	const ratio = 2.0     // Geometric spacing ratio

	// Build pole locations dynamically, stopping when tau exceeds tauMax
	var b []float64
	for i := 0; ; i++ {
		tau := tau0 * math.Pow(ratio, float64(i))
		if tau > tauMax {
			break
		}
		b = append(b, math.Exp(-1.0/tau))
	}
	numStages := len(b)
	states := make([]float64, numStages)

	// Scale factor to achieve target Allan deviation
	// For flicker FM, ADEV = h_minus1 (constant with tau)
	// The sum of recursive filters needs normalization
	scale := stddevPPB / 1e9 / math.Sqrt(float64(numStages))

	var lastTime float64

	return func(t float64) float64 {
		if lastTime > 0 {
			dt := t - lastTime
			// For dt = 1.0 (normal case), coefficients work as designed
			// For other dt, scale time constants appropriately
			dtNorm := dt // Assume dt ≈ 1.0 for integration steps

			// Update each recursive filter stage
			// Each stage: y[n] = b[i] * y[n-1] + noise
			for i := 0; i < numStages; i++ {
				// Adjust pole location for time step
				pole := math.Pow(b[i], dtNorm)
				noise := rng.NormFloat64() * scale * math.Sqrt(1-pole*pole)
				states[i] = pole*states[i] + noise
			}
		}
		lastTime = t

		// Sum all stages to get 1/f output
		sum := 0.0
		for i := 0; i < numStages; i++ {
			sum += states[i]
		}
		return sum
	}
}

// RandomWalkOsc creates an oscillator with random walk frequency modulation.
// Random walk FM models long-term drift where frequency undergoes Brownian motion.
// In Allan deviation, produces τ^(+1/2) slope at long averaging times.
//
// stddevPPB is the random walk FM coefficient in ppb/√s.
//
// IMPORTANT: This implementation maintains state (currentFreq, lastTime) and requires
// monotonically increasing time values. RawClock guarantees this.
func RandomWalkOsc(stddevPPB float64, seed int64) OscSimulator {
	rng := rand.New(rand.NewSource(seed))
	var currentFreq float64
	var lastTime float64

	return func(t float64) float64 {
		if lastTime > 0 {
			dt := t - lastTime
			step := rng.NormFloat64() * stddevPPB / 1e9 * math.Sqrt(dt)
			currentFreq += step
		}
		lastTime = t
		return currentFreq
	}
}

// DriftOsc creates linear frequency drift over time.
// Models oscillator frequency changing at a constant rate (quadratic phase drift).
//
// Parameters:
//
//	ratePPBPerDay: frequency change rate in ppb/day
//
// Example: ratePPBPerDay=163 means frequency increases by 163 ppb per day
// This would accumulate ~3.4 ns of phase error per minute, ~12 µs per hour
func DriftOsc(ratePPBPerDay float64) OscSimulator {
	// Convert ppb/day to fractional_frequency/second
	ratePerSec := ratePPBPerDay / 86400.0 / 1e9

	return func(t float64) float64 {
		return ratePerSec * t
	}
}

// SinusoidOsc creates sinusoidal frequency modulation.
// Models periodic effects like temperature cycles or crystal resonances.
//
// Parameters:
//
//	ampPPB: amplitude in ppb (peak deviation from nominal frequency)
//	periodS: period in seconds (e.g., 86400 for daily thermal cycle)
//	phaseInit: initial phase in [0,1) (0.5 = middle of cycle)
func SinusoidOsc(ampPPB, periodS, phaseInit float64) OscSimulator {
	omega := 2 * math.Pi / periodS
	scale := ampPPB / 1e9
	phase := phaseInit * 2 * math.Pi

	return func(t float64) float64 {
		return scale * math.Sin(omega*t+phase)
	}
}

// CombineOsc combines multiple oscillator frequency error sources.
func CombineOsc(funcs ...OscSimulator) OscSimulator {
	return func(t float64) float64 {
		sum := 0.0
		for _, f := range funcs {
			sum += f(t)
		}
		return sum
	}
}
