package clocksim

import (
	"math"
	"math/rand"
	"time"
)


// GPSSimulator models GNSS PPS timing errors.
//
// Input: t - simulation time in seconds since simulation start. The simulation is
// assumed to start aligned with a TAI second boundary. This is NOT TAI time itself,
// but rather an offset from the simulation start (which could be at any TAI epoch).
//
// Output: phase error in seconds (added to nominal PPS time to get actual PPS time)
//
// Contract when used with VirtualClock:
//   - Leading edge simulators: called with consecutive integers 1.0, 2.0, 3.0, ...
//     (first PPS is at simulation second 1)
//   - Trailing edge simulators: called with integers plus pulseWidth
//     (e.g., 1.1, 2.1, 3.1, ... for 0.1s pulse width)
//
// This monotonic calling pattern allows stateful simulators to use incremental
// updates without needing random access to arbitrary time points.
type GPSSimulator func(t float64) float64

// PerfectGPS creates a GPS simulator with no timing error.
func PerfectGPS() GPSSimulator {
	return func(t float64) float64 {
		return 0
	}
}

// JitterGPS creates a GPS simulator with Gaussian timing jitter.
// stddev is the standard deviation of the timing error.
func JitterGPS(stddev time.Duration, seed int64) GPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	stddevSec := stddev.Seconds()
	return func(t float64) float64 {
		return rng.NormFloat64() * stddevSec
	}
}

// SawtoothGPS creates a stateful GPS simulator that models GPS receiver sawtooth error.
// Sawtooth error is quantization caused by aligning PPS edges to internal ticks.
// The phase advances based on the oscillator's frequency error, coupling sawtooth
// behavior to the PHC oscillator.
//
// Parameters:
//
//	osc: OscSimulator returning fractional frequency error (dimensionless)
//	ampPPS: peak-to-peak amplitude in seconds (typically 8ns = 8e-9)
//	phaseInit: initial phase in [0,1) (0.5 = middle of cycle)
//
// The sawtooth period varies with oscillator frequency error: Ts = ampPPS / |y|
// where y is the fractional frequency offset. Returns nearest-tick zero-mean error.
//
// Assumes consecutive calls with unit time increments (dt = 1.0 second).
// This is satisfied when used as a leading edge simulator with VirtualClock,
// which calls with 1.0, 2.0, 3.0, ... The absolute value of t is passed to the
// coupled oscillator, but phase advancement only depends on the unit increment.
func SawtoothGPS(osc OscSimulator, ampPPS, phaseInit float64) GPSSimulator {
	phase := phaseInit
	return func(t float64) float64 {
		// Get fractional frequency error from oscillator at simulation time t
		y := osc(t)
		// Advance phase by one tick (dt = 1.0 second by assumption)
		deltaTicks := y * 1.0 / ampPPS
		phase = phase + deltaTicks
		phase = phase - math.Floor(phase)
		// Map to quantization error (nearest-tick, zero-mean)
		rawSaw := (phase - 0.5) * ampPPS
		return rawSaw
	}
}

// SinusoidGPS creates a GPS simulator with sinusoidal phase modulation.
// Models periodic GPS errors like multipath or atmospheric effects.
//
// Parameters:
//
//	ampNs: amplitude in nanoseconds (peak deviation)
//	periodS: period in seconds (e.g., 3000 for thermal cycle)
//	phaseInit: initial phase in [0,1) (0.5 = middle of cycle)
func SinusoidGPS(ampNs, periodS, phaseInit float64) GPSSimulator {
	omega := 2 * math.Pi / periodS
	ampSec := ampNs * 1e-9
	phase := phaseInit * 2 * math.Pi
	return func(t float64) float64 {
		return ampSec * math.Sin(omega*t+phase)
	}
}

// AR1ColoredNoiseGPS creates a GPS simulator with AR(1) colored noise.
// Models tropospheric delay and multipath effects as slowly varying correlated drift.
//
// The process evolves as: Y_t = alpha * Y_{t-1} + epsilon_t
// where epsilon_t is Gaussian white noise with standard deviation noiseStddev.
//
// Parameters:
//
//	alpha: autocorrelation coefficient (e.g., 0.998671 from GPS measurements)
//	noiseStddevNs: standard deviation of driving noise in nanoseconds (e.g., 0.49)
//	seed: random seed for reproducibility
//
// Assumes calls at unit time steps (t = 1.0, 2.0, 3.0, ...).
// This means it is suitable for leading edge (ppsSimulator) but not trailing edge (trailingEdgeSimulator).
// Correlation time constant: tau_c = -1 / ln(alpha) seconds (e.g., ~750s for alpha=0.9987)
func AR1ColoredNoiseGPS(alpha float64, noiseStddevNs float64, seed int64) GPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	noiseStddevSec := noiseStddevNs * 1e-9 // Convert ns to seconds

	// Initialize from stationary distribution
	// For AR(1), stationary variance = sigma_epsilon^2 / (1 - alpha^2)
	stationaryStddev := noiseStddevSec / math.Sqrt(1-alpha*alpha)
	state := rng.NormFloat64() * stationaryStddev

	return func(t float64) float64 {
		// AR(1) evolution: Y_t = alpha * Y_{t-1} + epsilon_t
		epsilon := rng.NormFloat64() * noiseStddevSec
		state = alpha*state + epsilon
		return state
	}
}

// ShiftPPS creates a GPS simulator that applies a temporary phase shift with smooth transitions.
// The shift rises with a half-sine profile, holds at the specified shift, then falls symmetrically.
// Useful for simulating ionospheric disturbances or other gradual timing biases.
func ShiftPPS(startTime float64, ramp, duration, shift time.Duration) GPSSimulator {
	startSec := startTime
	rampSec := ramp.Seconds()
	durationSec := duration.Seconds()
	shiftSec := shift.Seconds()
	// Hold period is total duration minus two ramps
	holdSec := durationSec - 2*rampSec
	if holdSec < 0 {
		holdSec = 0
	}
	return func(t float64) float64 {
		relTime := t - startSec
		switch {
		case relTime <= 0:
			return 0
		case relTime < rampSec:
			// Smooth half-sine rise: 0 to shift
			prog := relTime / rampSec
			return shiftSec * 0.5 * (1 - math.Cos(math.Pi*prog))
		case relTime < rampSec+holdSec:
			// Hold at shift
			return shiftSec
		case relTime < durationSec:
			// Symmetric fall: shift to 0
			prog := (relTime - rampSec - holdSec) / rampSec
			return shiftSec * 0.5 * (1 + math.Cos(math.Pi*prog))
		default:
			return 0
		}
	}
}

// SingleOutlierPPS creates a GPS simulator that adds a phase offset at a specific second.
// Both second and t are rounded to the nearest integer for comparison.
// Useful for testing outlier detection by injecting controlled timing errors.
func SingleOutlierPPS(second float64, offset time.Duration) GPSSimulator {
	targetSecond := math.Round(second)
	return func(t float64) float64 {
		if math.Round(t) == targetSecond {
			return offset.Seconds()
		}
		return 0
	}
}

// CombineGPS combines multiple PPS phase error sources additively.
func CombineGPS(funcs ...GPSSimulator) GPSSimulator {
	return func(t float64) float64 {
		sum := 0.0
		for _, f := range funcs {
			sum += f(t)
		}
		return sum
	}
}