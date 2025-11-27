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
//
// The stddevPPB parameter represents the Allan deviation at τ=1s (IEEE standard h₀).
// This function generates discrete white FM samples at integration step size dt that produce the
// desired Allan deviation when integrated and sampled at 1-second intervals.
//
// Theory:
//
//	White FM has constant one-sided PSD: S_y(f) = 2h₀²
//	Allan deviation: σ_y(τ) = h₀/√τ (IEEE 1139-2008)
//
//	For discrete samples y_k ~ N(0, σ²) at interval dt:
//	  - Discrete Allan variance at τ: σ_y²(τ) = σ² · (dt/τ)
//	  - To match continuous-time: σ² · (dt/τ) = h₀²/τ
//	  - Therefore: σ = h₀/√dt
//
// The dt dependence is correct and necessary: it ensures that the observable
// Allan deviation σ_y(τ) remains independent of the integration step size.
// Smaller dt increases bandwidth and total power, requiring proportionally
// smaller per-sample variance to maintain the same PSD level.
//
// Relies on RawClock's monotonic time guarantee: each call advances the RNG state
// incrementally without needing to seek to arbitrary time points.
func WhiteNoiseOsc(stddevPPB float64, seed int64) OscSimulator {
	rng := rand.New(rand.NewSource(seed))
	// Convert ADEV at τ=1s to per-sample stddev at integration rate
	// σ = h₀ · √(τ₀/dt) where τ₀ = 1s (matching Python's Integrator)
	const tau0 = 1.0 // Reference averaging time in seconds
	h0 := stddevPPB / 1e9
	stddev := h0 * math.Sqrt(tau0/rawClockDT)

	return func(t float64) float64 {
		return rng.NormFloat64() * stddev
	}
}

// Generate flicker noise constants (tau parameters and Allan-weighted scale factor).
//go:generate uv --directory flickerconsts run flicker_scale.py --tau0 10.0 --tau-max 1000.0 --ratio 2.0 --fudge-factor 0.774 --package clocksim --prefix flicker --go-const --output ../flickerconsts.go

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
	// Build pole locations dynamically, stopping when tau exceeds flickerTauMax
	var b []float64
	for i := 0; ; i++ {
		tau := flickerTau0 * math.Pow(flickerRatio, float64(i))
		if tau > flickerTauMax {
			break
		}
		b = append(b, math.Exp(-1.0/tau))
	}
	numStages := len(b)
	states := make([]float64, numStages)

	// Apply Allan-weighted scaling factor correction.
	// Naive 1/sqrt(N) scaling produces incorrect Allan deviation because it doesn't
	// account for how the Allan variance filter weights different frequencies.
	scale := stddevPPB / 1e9 / math.Sqrt(float64(numStages)) * flickerScaleFactor

	// Precompute poles and noise scales for a given dt.
	// This avoids expensive math.Pow calls in the hot path when dt is constant.
	poles := make([]float64, numStages)
	noiseScales := make([]float64, numStages)
	computePoles := func(dt float64) {
		for i := range poles {
			poles[i] = math.Pow(b[i], dt)
			noiseScales[i] = scale * math.Sqrt(1-poles[i]*poles[i])
		}
	}
	computePoles(rawClockDT)

	var lastTime float64
	// Track last dt to detect when recomputation is needed. Floating point
	// accumulation causes t-lastTime to drift by ~1 ULP occasionally, so we
	// can't assume dt is always exactly rawClockDT.
	lastDT := rawClockDT

	return func(t float64) float64 {
		if lastTime > 0 {
			dt := t - lastTime
			if dt != lastDT {
				computePoles(dt)
				lastDT = dt
			}
			for i := range numStages {
				states[i] = poles[i]*states[i] + rng.NormFloat64()*noiseScales[i]
			}
		}
		lastTime = t

		// Sum all stages to get 1/f output
		sum := 0.0
		for i := range numStages {
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
	if periodS == 0 {
		panic("SinusoidOsc: period cannot be zero")
	}
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
