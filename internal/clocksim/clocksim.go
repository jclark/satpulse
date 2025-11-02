package clocksim

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

// OscillatorSimulator models the behavior of a hardware oscillator.
// Input: true simulation time (seconds since start)
// Output: fractional frequency error at that time
// Example: if output is +1e-6, the clock runs 1µs fast per second of true time
type OscillatorSimulator func(trueTime float64) float64

// FreqOffset creates an oscillator with constant frequency offset.
// ppb is the frequency offset in parts per billion (positive means runs fast).
func FreqOffset(ppb float64) OscillatorSimulator {
	return func(t float64) float64 {
		return ppb / 1e9
	}
}

// Perfect creates an oscillator with no frequency error.
func Perfect() OscillatorSimulator {
	return FreqOffset(0)
}

// WhiteFreqNoise creates an oscillator with white frequency noise.
// stddevPPB is the standard deviation of frequency noise in ppb.
func WhiteFreqNoise(stddevPPB float64, seed int64) OscillatorSimulator {
	rng := rand.New(rand.NewSource(seed))
	return func(t float64) float64 {
		return rng.NormFloat64() * stddevPPB / 1e9
	}
}

// FlickerFreqNoise creates an oscillator with flicker-like frequency noise.
// This is a simplified approximation using a random walk to generate slowly-varying
// correlated noise, suitable for testing disciplining algorithms.
//
// The actual flicker FM (1/f noise) would have flat Allan deviation at stddevPPB,
// but this random walk approximation produces qualitatively similar behavior:
// frequency errors that drift slowly over time rather than changing instantly.
//
// stddevPPB is the noise level in parts per billion.
func FlickerFreqNoise(stddevPPB float64, seed int64) OscillatorSimulator {
	rng := rand.New(rand.NewSource(seed))
	var currentValue float64
	var lastTime float64

	return func(t float64) float64 {
		if lastTime > 0 {
			dt := t - lastTime
			// Random walk: frequency drifts slowly over time
			currentValue += rng.NormFloat64() * stddevPPB / 1e9 * math.Sqrt(dt)
		}
		lastTime = t
		return currentValue
	}
}

// FreqDrift creates linear frequency drift over time.
// Models oscillator frequency changing at a constant rate (quadratic phase drift).
//
// Parameters:
//
//	ratePPBPerDay: frequency change rate in ppb/day
//
// Example: ratePPBPerDay=163 means frequency increases by 163 ppb per day
// This would accumulate ~3.4 ns of phase error per minute, ~12 µs per hour
func FreqDrift(ratePPBPerDay float64) OscillatorSimulator {
	// Convert ppb/day to fractional_frequency/second
	ratePerSec := ratePPBPerDay / 86400.0 / 1e9

	return func(t float64) float64 {
		return ratePerSec * t
	}
}

// SineFM creates sinusoidal frequency modulation.
// Models periodic effects like temperature cycles or crystal resonances.
//
// Parameters:
//
//	ampPPB: amplitude in ppb (peak deviation from nominal frequency)
//	periodS: period in seconds (e.g., 86400 for daily thermal cycle)
//	phaseRad: initial phase offset in radians (0 to 2π)
func SineFM(ampPPB, periodS, phaseRad float64) OscillatorSimulator {
	omega := 2 * math.Pi / periodS
	scale := ampPPB / 1e9

	return func(t float64) float64 {
		return scale * math.Sin(omega*t+phaseRad)
	}
}

// CombineOscillators combines multiple oscillator frequency error sources.
func CombineOscillators(funcs ...OscillatorSimulator) OscillatorSimulator {
	return func(t float64) float64 {
		sum := 0.0
		for _, f := range funcs {
			sum += f(t)
		}
		return sum
	}
}

// PPSSimulator models GNSS PPS timing errors.
// Input: true time (typically integer seconds)
// Output: phase error in seconds (added to true time to get actual PPS time)
type PPSSimulator func(trueTime float64) float64

// PerfectPPS creates a PPS simulator with no timing error.
func PerfectPPS() PPSSimulator {
	return func(t float64) float64 {
		return 0
	}
}

// WhiteNoisePPS creates a PPS simulator with Gaussian timing jitter.
// stddev is the standard deviation of the timing error.
func WhiteNoisePPS(stddev time.Duration, seed int64) PPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	stddevSec := stddev.Seconds()
	return func(t float64) float64 {
		return rng.NormFloat64() * stddevSec
	}
}

// SawtoothPPS creates a PPS simulator that models GPS receiver sawtooth error.
// Sawtooth error is a characteristic ramp-and-reset pattern caused by the receiver's
// timing quantisation, typically with 2-4 second periods and amplitudes of 5-20 ns.
//
// Parameters:
//
//	periodS: median time between sawtooth resets in seconds (e.g., 2.0 for LEA-M8T, 4.0 for ZED-F9T)
//	amplitudeNs: median peak-to-peak amplitude in nanoseconds (e.g., 13.8 for M8T, 6.0 for F9T)
//
// The sawtooth rises linearly from 0 to amplitude over the period, then resets.
// Reset times are deterministic at integer multiples of the period.
func SawtoothPPS(periodS, amplitudeNs float64) PPSSimulator {
	amplitude := amplitudeNs * 1e-9 // Convert ns to seconds

	return func(trueTime float64) float64 {
		// Position within current sawtooth period [0, periodS)
		t := math.Mod(trueTime, periodS)

		// Linear ramp from 0 to amplitude
		return amplitude * (t / periodS)
	}
}

// ShiftPPS creates a PPS simulator that applies a temporary phase shift with smooth transitions.
// The shift rises with a half-sine profile, holds at the specified shift, then falls symmetrically.
// Useful for simulating ionospheric disturbances or other gradual timing biases.
func ShiftPPS(startTime float64, ramp, duration, shift time.Duration) PPSSimulator {
	startSec := startTime
	rampSec := ramp.Seconds()
	durationSec := duration.Seconds()
	shiftSec := shift.Seconds()
	// Hold period is total duration minus two ramps
	holdSec := durationSec - 2*rampSec
	if holdSec < 0 {
		holdSec = 0
	}
	return func(trueTime float64) float64 {
		t := trueTime - startSec
		switch {
		case t <= 0:
			return 0
		case t < rampSec:
			// Smooth half-sine rise: 0 to shift
			prog := t / rampSec
			return shiftSec * 0.5 * (1 - math.Cos(math.Pi*prog))
		case t < rampSec+holdSec:
			// Hold at shift
			return shiftSec
		case t < durationSec:
			// Symmetric fall: shift to 0
			prog := (t - rampSec - holdSec) / rampSec
			return shiftSec * 0.5 * (1 + math.Cos(math.Pi*prog))
		default:
			return 0
		}
	}
}

// SingleOutlierPPS creates a PPS simulator that adds a phase offset at a specific second.
// Both second and trueTime are rounded to the nearest integer for comparison.
// Useful for testing outlier detection by injecting controlled timing errors.
func SingleOutlierPPS(second float64, offset time.Duration) PPSSimulator {
	targetSecond := math.Round(second)
	return func(trueTime float64) float64 {
		if math.Round(trueTime) == targetSecond {
			return offset.Seconds()
		}
		return 0
	}
}

// CombinePPS combines multiple PPS phase error sources additively.
func CombinePPS(funcs ...PPSSimulator) PPSSimulator {
	return func(t float64) float64 {
		sum := 0.0
		for _, f := range funcs {
			sum += f(t)
		}
		return sum
	}
}

// RawClock wraps an OscillatorSimulator and integrates frequency error to produce phase.
// It represents the unadjusted hardware oscillator with an initial phase offset.
// Times are stored as int64 nanoseconds to match ptime.Time representation.
// Integration is incremental: ReadAt must be called with monotonically increasing times.
type RawClock struct {
	oscillator     OscillatorSimulator
	startPhaseNs   int64
	dt             float64 // Integration step size
	lastSimTime    float64 // Last time ReadAt was called
	lastFreq       float64 // Frequency at lastSimTime
	accumulatedSec float64 // Accumulated phase delta from t=0 to lastSimTime
}

// NewRawClock creates a RawClock with the given oscillator and initial phase in nanoseconds.
func NewRawClock(oscillator OscillatorSimulator, startPhaseNs int64) *RawClock {
	return &RawClock{
		oscillator:     oscillator,
		startPhaseNs:   startPhaseNs,
		dt:             0.001, // 1ms integration steps
		lastSimTime:    0.0,
		lastFreq:       oscillator(0.0),
		accumulatedSec: 0.0,
	}
}

// ReadAt returns the raw clock phase in nanoseconds at the given simulation time by integrating frequency error.
// Integrates: phase(t) = startPhase + integral from 0 to t of (1 + freq(tau)) dtau
// Must be called with monotonically increasing times (panics if time goes backwards).
func (r *RawClock) ReadAt(simTime float64) int64 {
	if simTime < r.lastSimTime {
		panic(fmt.Sprintf("ReadAt: time went backwards: %.9f < %.9f", simTime, r.lastSimTime))
	}

	// Integrate from lastSimTime to simTime using incremental approach
	// This ensures each time point is evaluated exactly once
	t := r.lastSimTime
	freq1 := r.lastFreq

	// Use trapezoidal rule for integration
	for t < simTime {
		dt := r.dt
		if t+dt > simTime {
			dt = simTime - t
		}

		freq2 := r.oscillator(t + dt) // Only evaluate new endpoint
		// Clock advances by dt * (1 + average frequency error)
		r.accumulatedSec += dt * (1 + (freq1+freq2)/2)

		t += dt
		freq1 = freq2 // Next iteration's freq1 is this iteration's freq2
	}

	// Update state for next call
	r.lastSimTime = simTime
	r.lastFreq = freq1

	// Convert accumulated delta to nanoseconds and add to start phase
	// This avoids precision loss when startPhase is large (e.g., GPS epoch)
	deltaNs := int64(r.accumulatedSec * 1e9)
	phaseNs := r.startPhaseNs + deltaNs
	return phaseNs
}

// VirtualClock simulates a disciplined PHC that can be adjusted via frequency offset and time steps.
// It models the Linux PHC implementation where adjustments are tracked relative to the last change.
// Timestamps are generated lazily as simulation time advances past PPS events.
// Phase values are stored as int64 nanoseconds to match ptime.Time.
type timestampEvent struct {
	phase    time.Duration
	trueTime float64
}

type VirtualClock struct {
	raw                    *RawClock
	ppsSimulator           PPSSimulator
	trailingEdgeSimulator  PPSSimulator
	adjTimeDelaySimulator  func() float64
	maxFreqOff             float64
	simTime                float64
	lastAdjTime            float64
	lastRawPhaseNs         int64
	lastVirtPhaseNs        int64
	freqOffset             float64
	nextPPSNominal         float64
	nextEdgeActual         float64
	nextTrailingEdgeActual float64 // next trailing edge when pulseWidth > 0; equals nextEdgeActual otherwise
	pulseWidth             float64
	tsQueue                []timestampEvent
}

// defaultAdjTimeDelay returns a realistic ADJ_SETOFFSET delay with jitter.
// Models kernel read-modify-write timing: ~5µs mean with ~1µs stddev.
// Uses rejection sampling to ensure delay is always non-negative.
func defaultAdjTimeDelay() func() float64 {
	rng := rand.New(rand.NewSource(42))
	return func() float64 {
		for {
			delay := 5e-6 + rng.NormFloat64()*1e-6
			if delay >= 0 {
				return delay
			}
			// Reject negative sample, resample to maintain distribution
		}
	}
}

// NewVirtualClock creates a VirtualClock starting at the given simulation time.
// The first PPS will be at the first integer second > startTime.
// For dual-edge mode, pulseWidth > 0 and trailingEdgeSimulator must be provided.
// For single-edge mode, pulseWidth = 0 and trailingEdgeSimulator can be nil.
func NewVirtualClock(raw *RawClock, ppsSimulator PPSSimulator, startTime float64, maxFreqOff float64, pulseWidth time.Duration, trailingEdgeSimulator PPSSimulator) *VirtualClock {
	firstPPSNominal := float64(int(startTime) + 1)
	rawPhaseNs := raw.ReadAt(startTime)

	c := &VirtualClock{
		raw:                   raw,
		ppsSimulator:          ppsSimulator,
		trailingEdgeSimulator: trailingEdgeSimulator,
		adjTimeDelaySimulator: defaultAdjTimeDelay(),
		maxFreqOff:            maxFreqOff,
		simTime:               startTime,
		lastAdjTime:           startTime,
		lastRawPhaseNs:        rawPhaseNs,
		lastVirtPhaseNs:       rawPhaseNs,
		freqOffset:            0,
		nextPPSNominal:        firstPPSNominal,
		pulseWidth:            pulseWidth.Seconds(),
	}
	c.generateNextEdges()
	return c
}

// AdvanceTo advances simulation time to newTime.
// Generates timestamps for any PPS events that occur during this interval.
// Panics if newTime < c.simTime (time cannot go backwards).
func (c *VirtualClock) AdvanceTo(newTime float64) {
	if newTime < c.simTime {
		panic("AdvanceTo: time cannot go backwards")
	}

	for newTime >= c.nextEdgeActual {
		virtPhaseNs := c.computeVirtPhaseNs(c.nextEdgeActual)
		c.tsQueue = append(c.tsQueue, timestampEvent{
			phase:    time.Duration(virtPhaseNs),
			trueTime: c.nextEdgeActual,
		})

		if c.nextEdgeActual == c.nextTrailingEdgeActual {
			c.nextPPSNominal += 1.0
			c.generateNextEdges()
		} else {
			c.nextEdgeActual = c.nextTrailingEdgeActual
		}
	}

	c.simTime = newTime
}

// generateNextEdges generates nextEdgeActual and nextTrailingEdgeActual based on nextPPSNominal.
func (c *VirtualClock) generateNextEdges() {
	phaseError := c.ppsSimulator(c.nextPPSNominal)
	c.nextEdgeActual = c.nextPPSNominal + phaseError
	if c.pulseWidth != 0 {
		c.nextTrailingEdgeActual = c.nextEdgeActual + c.pulseWidth + c.trailingEdgeSimulator(c.nextPPSNominal+c.pulseWidth)
	} else {
		c.nextTrailingEdgeActual = c.nextEdgeActual
	}
}

// SetFreqOffset sets the frequency offset adjustment in PPB.
func (c *VirtualClock) SetFreqOffset(f float64) error {
	if f > c.maxFreqOff {
		f = c.maxFreqOff
	} else if f < -c.maxFreqOff {
		f = -c.maxFreqOff
	}
	currentVirtNs := c.computeVirtPhaseNs(c.simTime)
	currentRawNs := c.raw.ReadAt(c.simTime)

	c.lastAdjTime = c.simTime
	c.lastRawPhaseNs = currentRawNs
	c.lastVirtPhaseNs = currentVirtNs
	c.freqOffset = f
	return nil
}

// FreqOffset returns the current frequency offset in PPB.
func (c *VirtualClock) FreqOffset() (float64, error) {
	return c.freqOffset, nil
}

// MaxFreqOffset returns the maximum allowed frequency offset in PPB.
func (c *VirtualClock) MaxFreqOffset() float64 {
	return c.maxFreqOff
}

// AdjTime steps the virtual clock by the given duration.
// Simulates ADJ_SETOFFSET kernel behavior: read-modify-write has delay,
// causing the actual time step to be slightly imperfect.
func (c *VirtualClock) AdjTime(d time.Duration) error {
	// Kernel reads current time at T1
	t1 := c.simTime
	currentVirtNs := c.computeVirtPhaseNs(t1)

	// Kernel computes target = current + offset
	targetPhaseNs := currentVirtNs + int64(d)

	// Kernel write happens after a delay (read-modify-write takes time)
	delay := c.adjTimeDelaySimulator()
	c.AdvanceTo(t1 + delay)

	// Kernel writes the target phase, but true time has advanced by delay
	// So the clock is now behind by ~delay seconds
	currentRawNs := c.raw.ReadAt(c.simTime)
	c.lastAdjTime = c.simTime
	c.lastRawPhaseNs = currentRawNs
	c.lastVirtPhaseNs = targetPhaseNs
	return nil
}

// ReadTimestamp reads the next timestamp from the queue along with the true time when it occurred.
func (c *VirtualClock) ReadTimestamp() (time.Duration, float64, bool) {
	if len(c.tsQueue) == 0 {
		return 0, 0, false
	}
	ts := c.tsQueue[0]
	c.tsQueue = c.tsQueue[1:]
	return ts.phase, ts.trueTime, true
}

// TimestampAvailable returns true if there are timestamps in the queue.
func (c *VirtualClock) TimestampAvailable() bool {
	return len(c.tsQueue) > 0
}

func (c *VirtualClock) computeVirtPhaseNs(atTime float64) int64 {
	currentRawNs := c.raw.ReadAt(atTime)
	rawDeltaNs := currentRawNs - c.lastRawPhaseNs
	// Apply frequency correction: correctedDelta = delta * (1 + freqOffset/1e9)
	correctedDeltaNs := int64(float64(rawDeltaNs) * (1 + c.freqOffset/1e9))
	return c.lastVirtPhaseNs + correctedDeltaNs
}

// TestClock implements phcsync.Clock for testing.
// It wraps VirtualClock and adds era tracking, mirroring how ts.Clock wraps phc.Clock.
// Since simulation is single-threaded, no atomics needed.
type TestClock struct {
	*VirtualClock
	era ptime.Era
}

// NewTestClock creates a TestClock for testing.
func NewTestClock(vclock *VirtualClock) *TestClock {
	return &TestClock{
		VirtualClock: vclock,
		era:          1, // Start at era 1
	}
}

// AdjTime steps the clock and updates the era.
func (c *TestClock) AdjTime(d time.Duration) (ptime.Era, error) {
	c.era++ // Era N+1 (uncertain)
	err := c.VirtualClock.AdjTime(d)
	c.era++ // Era N+2 (certain)
	if err != nil {
		return ptime.Era(0), err
	}
	return c.era, nil
}

// Era returns the current era value.
func (c *TestClock) Era() ptime.Era {
	return c.era
}

// ReadTimestampWithEra reads a timestamp from the queue and attaches the current era.
func (c *TestClock) ReadTimestampWithEra() (ptime.ClockTime, float64, bool) {
	ts, trueTime, ok := c.VirtualClock.ReadTimestamp()
	if !ok {
		return ptime.ClockTime{}, 0, false
	}
	return ptime.ClockTime{
		T:   ptime.Time(ts),
		Era: c.Era(),
	}, trueTime, true
}

// Now returns the current PHC time.
func (c *TestClock) Now() ptime.ClockTime {
	virtPhaseNs := c.VirtualClock.computeVirtPhaseNs(c.VirtualClock.simTime)
	return ptime.ClockTime{
		T:   ptime.Time(virtPhaseNs),
		Era: c.Era(),
	}
}
