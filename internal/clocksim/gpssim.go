package clocksim

import (
	"math"
	"math/rand"
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
// stddev is the standard deviation of the timing error in nanoseconds.
func JitterGPS(stddev Nanoseconds, seed int64) GPSSimulator {
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
//	amp: amplitude in nanoseconds (peak deviation)
//	periodS: period in seconds (e.g., 3000 for thermal cycle)
//	phaseInit: initial phase in [0,1) (0.5 = middle of cycle)
func SinusoidGPS(amp Nanoseconds, periodS, phaseInit float64) GPSSimulator {
	omega := 2 * math.Pi / periodS
	ampSec := amp.Seconds()
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
//	noiseStddev: standard deviation of driving noise in nanoseconds (e.g., 0.49)
//	seed: random seed for reproducibility
//
// Assumes calls at unit time steps (t = 1.0, 2.0, 3.0, ...).
// This means it is suitable for leading edge (ppsSimulator) but not trailing edge (trailingEdgeSimulator).
// Correlation time constant: tau_c = -1 / ln(alpha) seconds (e.g., ~750s for alpha=0.9987)
func AR1ColoredNoiseGPS(alpha float64, noiseStddev Nanoseconds, seed int64) GPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	noiseStddevSec := noiseStddev.Seconds()

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
func ShiftPPS(startTime, rampSec, durationSec float64, shift Nanoseconds) GPSSimulator {
	shiftSec := shift.Seconds()
	// Hold period is total duration minus two ramps
	holdSec := durationSec - 2*rampSec
	if holdSec < 0 {
		holdSec = 0
	}
	return func(t float64) float64 {
		relTime := t - startTime
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
func SingleOutlierPPS(second float64, offset Nanoseconds) GPSSimulator {
	targetSecond := math.Round(second)
	offsetSec := offset.Seconds()
	return func(t float64) float64 {
		if math.Round(t) == targetSecond {
			return offsetSec
		}
		return 0
	}
}

// AR1FMGPS creates a GPS simulator with AR(1) frequency modulation (OU on frequency).
// Models mean-reverting coloured frequency bias integrated to phase.
// The frequency state x_k evolves as AR(1): x_{k+1} = ρ x_k + ε_k
// Phase is the cumulative sum: φ_k = Σ x_i · Δt
//
// Parameters:
//
//	tauS: correlation time T in seconds
//	sigma: stationary RMS of frequency bias in ppb
//	seed: random seed for reproducibility
//
// Returns GPS simulator function producing phase error in seconds.
func AR1FMGPS(tauS float64, sigma PPB, seed int64) GPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	// Convert parameters (Δt = 1 s)
	rho := math.Exp(-1.0 / tauS)
	rho = min(rho, 1.0-1e-12) // numerical safety
	sigmaX := sigma.Fractional()
	sigmaEps := sigmaX * math.Sqrt(1.0-rho*rho)
	// Initial state from stationary distribution
	xState := rng.NormFloat64() * sigmaX // frequency bias (dimensionless)
	var phiState float64                 // accumulated phase (seconds)
	return func(t float64) float64 {
		// AR(1) update on frequency
		eps := rng.NormFloat64() * sigmaEps
		xState = rho*xState + eps
		// Integrate to phase (Δt = 1 s)
		phiState += xState * 1.0
		return phiState
	}
}

// RandomWalkFMGPS creates a GPS simulator with random walk frequency modulation.
// Uses IEEE Std 1139-2008 convention: σ_y(τ) = h₊₁ · √(τ/3).
// Implements a discrete random walk on fractional frequency with step
// variance σ_step = h₊₁ · √dt, integrated to phase at 1 Hz.
//
// Parameters:
//
//	hPlus1: random walk FM coefficient (dimensionless/√s)
//	seed: random seed for reproducibility
//
// Returns GPS simulator function producing phase error in seconds.
//
// IMPORTANT: This implementation maintains state and requires monotonically
// increasing time values with unit increments (1.0, 2.0, 3.0, ...).
// VirtualClock guarantees this for leading edge simulators.
func RandomWalkFMGPS(hPlus1 float64, seed int64) GPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	var freqState float64  // fractional frequency offset (dimensionless)
	var phaseState float64 // accumulated phase error (seconds)
	var lastTime float64

	return func(t float64) float64 {
		if lastTime > 0 {
			dt := t - lastTime
			stepSigma := hPlus1 * math.Sqrt(dt)
			freqState += rng.NormFloat64() * stepSigma
			phaseState += freqState * dt
		}
		lastTime = t
		return phaseState
	}
}

// DriftGPS creates a GPS simulator with Carpenter-Lee 2nd-order Gauss-Markov drift.
// Implements a bounded drift process where the phase variance remains finite
// over long simulations. The state vector is [x, v] where x is phase error
// and v is the rate of change. The process is driven by white noise on v.
//
// Parameters:
//
//	omegaN: natural frequency (1/s)
//	zeta: damping ratio (dimensionless, typically 0.7)
//	sigmaDrift: continuous-time noise intensity driving v_drift (seconds)
//	seed: random seed for reproducibility
//
// Returns GPS simulator function producing drift phase error in seconds.
func DriftGPS(omegaN, zeta, sigmaDrift float64, seed int64) GPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	// dt = 1.0 for 1 Hz sampling
	dt := 1.0
	// Compute discrete Phi matrix elements for underdamped case (zeta < 1)
	// For critically/overdamped, clamp zeta
	zetaClamped := min(zeta, 0.99)
	omegaD := omegaN * math.Sqrt(1-zetaClamped*zetaClamped)
	decay := math.Exp(-zetaClamped * omegaN * dt)
	cosWd := math.Cos(omegaD * dt)
	sinWd := math.Sin(omegaD * dt)
	phi11 := decay * (cosWd + (zetaClamped*omegaN/omegaD)*sinWd)
	phi12 := decay * (1.0 / omegaD) * sinWd
	phi21 := decay * (-(omegaN * omegaN / omegaD) * sinWd)
	phi22 := decay * (cosWd - (zetaClamped*omegaN/omegaD)*sinWd)
	// Process noise covariance Q - first order approximation
	// Q_c = [[0, 0], [0, sigma_drift^2]], Q ≈ Q_c * dt
	// Cholesky factor L of Q: L[0,0] = 0, L[1,0] = 0, L[1,1] = sigma_drift * sqrt(dt)
	noiseStddev := sigmaDrift * math.Sqrt(dt)
	// State vector [x, v] - initialise from stationary distribution
	// For simplicity, start at zero (could initialise from stationary variance)
	var xState, vState float64
	return func(t float64) float64 {
		// Generate noise (only drives velocity)
		w := rng.NormFloat64() * noiseStddev
		// State transition: X_{k+1} = Phi * X_k + [0, w]
		xNew := phi11*xState + phi12*vState
		vNew := phi21*xState + phi22*vState + w
		xState = xNew
		vState = vNew
		return xState
	}
}

// ResonatorUserToInternal converts user-facing resonator parameters to internal simulator parameters.
//
// Uses discrete Lyapunov calibration so that sigmaNs matches the actual RMS
// of the 1 Hz discrete resonator simulator output.
//
// Parameters:
//
//	period: Resonator period in seconds (ω₀ = 2π/period)
//	sigmaNs: RMS phase in nanoseconds
//	zeta: Damping ratio (dimensionless, typically 0.1-0.5)
//
// Returns (omegaN, zeta, sigmaNoise) where:
//
//	omegaN: Natural frequency (rad/s)
//	zeta: Damping ratio (unchanged)
//	sigmaNoise: Noise intensity for simulator (seconds)
func ResonatorUserToInternal(period, sigmaNs, zeta float64) (omegaN, zetaOut, sigmaNoise float64) {
	if period <= 0 || sigmaNs <= 0 {
		return 0, 0, 0
	}
	omegaN = 2 * math.Pi / period
	zetaOut = zeta
	if zetaOut <= 0 {
		zetaOut = 0.3 // default damping ratio for resonator
	}
	sigmaSec := sigmaNs * 1e-9
	dt := 1.0
	varXUnit := DriftUnitPhaseVariance(omegaN, zetaOut, dt)
	// actual_variance = sigma_noise² * dt * var_x_unit = sigma_sec²
	// sigma_noise = sigma_sec / sqrt(dt * var_x_unit)
	if varXUnit > 0 {
		sigmaNoise = sigmaSec / math.Sqrt(dt*varXUnit)
	}
	return omegaN, zetaOut, sigmaNoise
}

// ResonatorGPS creates a GPS simulator with a damped harmonic oscillator on phase.
// Unlike resonator FM which integrates bounded frequency to unbounded phase, this
// directly oscillates phase around zero, guaranteeing bounded phase.
//
// State: [x, ẋ] where x is phase error (seconds)
// Dynamics: ẍ + 2ζω₀ẋ + ω₀²x = ξ(t)
// Output: x (phase directly)
//
// Parameters:
//
//	omegaN: natural frequency (rad/s), related to period by ω₀ = 2π/T
//	zeta: damping ratio (0.1-0.5 for pronounced resonance)
//	sigmaNoise: process noise intensity (seconds), calibrated via Lyapunov equation
//	seed: random seed for reproducibility
//
// Returns GPS simulator producing phase error in seconds.
func ResonatorGPS(omegaN, zeta, sigmaNoise float64, seed int64) GPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	dt := 1.0 // 1 Hz sampling
	// Compute discrete Phi matrix elements for underdamped case
	zetaClamped := min(zeta, 0.99)
	omegaD := omegaN * math.Sqrt(1-zetaClamped*zetaClamped)
	decay := math.Exp(-zetaClamped * omegaN * dt)
	cosWd := math.Cos(omegaD * dt)
	sinWd := math.Sin(omegaD * dt)
	phi11 := decay * (cosWd + (zetaClamped*omegaN/omegaD)*sinWd)
	phi12 := decay * (1.0 / omegaD) * sinWd
	phi21 := decay * (-(omegaN * omegaN / omegaD) * sinWd)
	phi22 := decay * (cosWd - (zetaClamped*omegaN/omegaD)*sinWd)
	// Process noise stddev
	noiseStddev := sigmaNoise * math.Sqrt(dt)
	// State vector [x, ẋ] - phase and phase rate
	var xState, xdotState float64
	return func(t float64) float64 {
		// Generate noise (only drives ẋ)
		w := rng.NormFloat64() * noiseStddev
		// State transition: X_{k+1} = Phi * X_k + [0, w]
		xNew := phi11*xState + phi12*xdotState
		xdotNew := phi21*xState + phi22*xdotState + w
		xState = xNew
		xdotState = xdotNew
		return xState
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