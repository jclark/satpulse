package clocksim

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

// OscillatorSimulator models the behavior of a hardware oscillator.
// Input: true simulation time (seconds since start)
// Output: fractional frequency error at that time
// Example: if output is +1e-6, the clock runs 1µs fast per second of true time
type OscillatorSimulator func(trueTime float64) float64

// ConstantDrift creates an oscillator with constant frequency offset.
// ppm is the frequency offset in parts per million (positive means runs fast).
func ConstantDrift(ppm float64) OscillatorSimulator {
	return func(t float64) float64 {
		return ppm / 1e6
	}
}

// Perfect creates an oscillator with no frequency error.
func Perfect() OscillatorSimulator {
	return ConstantDrift(0)
}

// WhiteFreqNoise creates an oscillator with white frequency noise.
// stddevPPM is the standard deviation of frequency noise in ppm.
func WhiteFreqNoise(stddevPPM float64, seed int64) OscillatorSimulator {
	rng := rand.New(rand.NewSource(seed))
	return func(t float64) float64 {
		return rng.NormFloat64() * stddevPPM / 1e6
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
// stddev is the standard deviation of the timing error in seconds.
func WhiteNoisePPS(stddev float64, seed int64) PPSSimulator {
	rng := rand.New(rand.NewSource(seed))
	return func(t float64) float64 {
		return rng.NormFloat64() * stddev
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
type VirtualClock struct {
	raw                  *RawClock
	ppsSimulator         PPSSimulator
	adjTimeDelaySimulator func() float64
	maxFreqOff           float64
	simTime              float64
	lastAdjTime          float64
	lastRawPhaseNs       int64
	lastVirtPhaseNs      int64
	freqOffset           float64
	nextPPSNominal       float64
	nextPPSActual        float64
	tsQueue              []time.Duration
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
func NewVirtualClock(raw *RawClock, ppsSimulator PPSSimulator, startTime float64, maxFreqOff float64) *VirtualClock {
	firstPPSNominal := float64(int(startTime) + 1)
	phaseError := ppsSimulator(firstPPSNominal)
	rawPhaseNs := raw.ReadAt(startTime)

	return &VirtualClock{
		raw:                  raw,
		ppsSimulator:         ppsSimulator,
		adjTimeDelaySimulator: defaultAdjTimeDelay(),
		maxFreqOff:           maxFreqOff,
		simTime:              startTime,
		lastAdjTime:          startTime,
		lastRawPhaseNs:       rawPhaseNs,
		lastVirtPhaseNs:      rawPhaseNs,
		freqOffset:           0,
		nextPPSNominal:       firstPPSNominal,
		nextPPSActual:        firstPPSNominal + phaseError,
	}
}

// AdvanceTo advances simulation time to newTime.
// Generates timestamps for any PPS events that occur during this interval.
// Panics if newTime < c.simTime (time cannot go backwards).
func (c *VirtualClock) AdvanceTo(newTime float64) {
	if newTime < c.simTime {
		panic("AdvanceTo: time cannot go backwards")
	}

	for newTime >= c.nextPPSActual {
		virtPhaseNs := c.computeVirtPhaseNs(c.nextPPSActual)
		c.tsQueue = append(c.tsQueue, time.Duration(virtPhaseNs))

		c.nextPPSNominal += 1.0
		phaseError := c.ppsSimulator(c.nextPPSNominal)
		c.nextPPSActual = c.nextPPSNominal + phaseError
	}

	c.simTime = newTime
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

// ReadTimestamp reads the next timestamp from the queue.
func (c *VirtualClock) ReadTimestamp() (time.Duration, bool) {
	if len(c.tsQueue) == 0 {
		return 0, false
	}
	ts := c.tsQueue[0]
	c.tsQueue = c.tsQueue[1:]
	return ts, true
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

// TestClock implements servo.Clock for testing.
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
func (c *TestClock) ReadTimestampWithEra() (ptime.ClockTime, bool) {
	ts, ok := c.VirtualClock.ReadTimestamp()
	if !ok {
		return ptime.ClockTime{}, false
	}
	return ptime.ClockTime{
		T:   ptime.Time(ts),
		Era: c.Era(),
	}, true
}

// Now returns the current PHC time.
func (c *TestClock) Now() ptime.ClockTime {
	virtPhaseNs := c.VirtualClock.computeVirtPhaseNs(c.VirtualClock.simTime)
	return ptime.ClockTime{
		T:   ptime.Time(virtPhaseNs),
		Era: c.Era(),
	}
}
