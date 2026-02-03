// Package clocksim provides discrete-time simulation of PTP hardware clocks and GNSS PPS signals.
//
// # Time Representation
//
// The simulator uses simulation time (seconds since simulation start) rather than absolute
// TAI time. Simulation time 0 is aligned with a TAI second boundary, allowing PPS events to
// occur at integer simulation seconds (1.0, 2.0, 3.0, ...). This design:
//   - Simplifies the simulator implementation
//   - Makes test cases easier to understand
//   - Allows simulators to work with any TAI epoch
//
// If a caller needs to generate GPS time messages with absolute TAI time, they should add
// the simulation start TAI time to the simulation time values.
package clocksim

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/jclark/satpulse/time/phctime"
	"github.com/jclark/satpulse/gps/ptime"
)

// RawClock wraps an OscSimulator and integrates frequency error to produce phase.
// It represents the unadjusted hardware oscillator with an initial phase offset.
// Times are stored as int64 nanoseconds to match ptime.Time representation.
// Integration is incremental: ReadAt must be called with monotonically increasing times.
//
// The clock phase at time t is: phase(t) = startPhase + integral of (1 + freq(tau)) dtau
// which equals: startPhase + t + integral of freq(tau) dtau
//
// We split this into two accumulators:
//   - Nominal time (accumNominalNs): tracked in integer nanoseconds for exact arithmetic
//   - Frequency error (accumNoise): tracked in float64 seconds where sub-ns precision is adequate
//
// This separation is necessary because rawClockDT (0.001) cannot be represented exactly
// in float64. Accumulating it directly would cause drift: 1200 * 0.001 ≠ 1.2 in float64.
// Integer nanoseconds give exact results for nominal time.
//
// Integration uses absolute step indexing: lastStep tracks which 1ms grid steps have been
// processed. Each step's time is computed as step*rawClockDT rather than accumulated
// incrementally. This avoids floating-point error that could cause spurious micro-steps
// at grid boundaries, which would corrupt stochastic oscillator statistics by drawing
// extra samples that don't contribute meaningful phase.
type RawClock struct {
	oscillator     OscSimulator
	startPhaseNs   int64
	lastStep       int64   // Absolute step index (grid position)
	lastTime       float64 // For monotonicity check
	accumNominalNs int64   // Accumulated nominal time in nanoseconds (exact)
	accumNoise     float64 // Accumulated frequency error in seconds (float)
}

// Integration step size for RawClock discrete integration.
const rawClockStepsPerSec = 1000
const rawClockDT = 1.0 / float64(rawClockStepsPerSec)           // 1ms in seconds
const rawClockDTNs = int64(1_000_000_000 / rawClockStepsPerSec) // 1ms in nanoseconds (exact integer)

// NewRawClock creates a RawClock with the given oscillator and initial phase in nanoseconds.
func NewRawClock(oscillator OscSimulator, startPhaseNs int64) *RawClock {
	return &RawClock{
		oscillator:     oscillator,
		startPhaseNs:   startPhaseNs,
		lastStep:       0,
		lastTime:       0.0,
		accumNominalNs: 0,
		accumNoise:     0.0,
	}
}

// ReadAt returns the raw clock phase in nanoseconds at the given simulation time.
// Integrates: phase(t) = startPhase + integral from 0 to t of (1 + freq(tau)) dtau
// Must be called with monotonically increasing times (panics if time goes backwards).
// The underlying oscillator is called only with monotonically increasing time values,
// enabling stateful simulators to use incremental updates without random access.
func (r *RawClock) ReadAt(simTime float64) int64 {
	if simTime < r.lastTime {
		panic(fmt.Sprintf("ReadAt: time went backwards: %.9f < %.9f", simTime, r.lastTime))
	}

	// Compute target step index using the integer step rate to avoid undercounting at boundaries.
	targetStep := int64(simTime * rawClockStepsPerSec)

	// Process new full steps since lastStep.
	// Each step samples the oscillator at the step's start time and accumulates:
	//   - Nominal time: exactly rawClockDTNs nanoseconds (integer, no FP error)
	//   - Noise: rawClockDT * freq (float, sub-ns precision adequate)
	for step := r.lastStep; step < targetStep; step++ {
		t := float64(step) * rawClockDT
		freq := r.oscillator(t)
		r.accumNominalNs += rawClockDTNs
		r.accumNoise += rawClockDT * freq
	}
	r.lastStep = targetStep
	r.lastTime = simTime

	// Compute remainder for sub-step precision.
	// This is NOT persisted - it's only used for the return value.
	// If we persisted it, consecutive calls like ReadAt(1.0004) then ReadAt(2.0008)
	// would double-count the remainder.
	gridTime := float64(targetStep) * rawClockDT
	remNs := int64(math.Round((simTime - gridTime) * 1e9))
	if remNs < 0 {
		remNs = 0
	}

	// Combine: startPhase + nominal + noise + remainder.
	// Use math.Round for noise because decimal frequencies (e.g., 10ppm = 1e-5) cannot be
	// represented exactly in binary float64, causing sub-nanosecond accumulation errors.
	return r.startPhaseNs + r.accumNominalNs + int64(math.Round(r.accumNoise*1e9)) + remNs
}

// SawtoothCorrections holds sawtooth error corrections for current and next pulse.
type SawtoothCorrections struct {
	Current float64 // correction applied to this timestamp
	Next    float64 // correction for next pulse (for PrePulse messages)
}

// TimestampReading groups all information from a timestamp read.
type TimestampReading struct {
	Timestamp phctime.Time
	TrueTime  float64
	Sawtooth  *SawtoothCorrections // nil if no sawtooth configured
}

// VirtualClock simulates a disciplined PHC that can be adjusted via frequency offset and time steps.
// It models the Linux PHC implementation where adjustments are tracked relative to the last change.
// Timestamps are generated lazily as simulation time advances past PPS events.
// Phase values are stored as int64 nanoseconds to match ptime.Time.
type timestampEvent struct {
	phase               time.Duration
	trueTime            float64
	sawtoothCorrections SawtoothCorrections
}

type VirtualClock struct {
	raw                    *RawClock
	sawtoothGPSSimulator   GPSSimulator // separate sawtooth simulator (can be nil)
	otherGPSSimulator      GPSSimulator // all non-sawtooth errors combined
	trailingEdgeSimulator  GPSSimulator
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
	sawtoothCorrections    SawtoothCorrections // current and next sawtooth corrections
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
//
// The startTime parameter is in simulation seconds since simulation start (typically 0).
// Simulation time 0 is aligned with a TAI second boundary. The first PPS will be at
// the first integer second > startTime.
//
// For dual-edge mode, pulseWidth > 0 and trailingEdgeSimulator must be provided.
// For single-edge mode, pulseWidth = 0 and trailingEdgeSimulator can be nil.
func NewVirtualClock(raw *RawClock, sawtoothPPS GPSSimulator, otherPPS GPSSimulator, startTime float64, maxFreqOff float64, pulseWidth time.Duration, trailingEdgeSimulator GPSSimulator) *VirtualClock {
	firstPPSNominal := float64(int(startTime) + 1)
	rawPhaseNs := raw.ReadAt(startTime)

	c := &VirtualClock{
		raw:                   raw,
		sawtoothGPSSimulator:  sawtoothPPS,
		otherGPSSimulator:     otherPPS,
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

	// Initialize Next for first pulse using actual PPS epoch
	if sawtoothPPS != nil {
		c.sawtoothCorrections.Next = sawtoothPPS(firstPPSNominal)
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
			phase:               time.Duration(virtPhaseNs),
			trueTime:            c.nextEdgeActual,
			sawtoothCorrections: c.sawtoothCorrections,
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
//
// Invariant: On return, sawtoothCorrections.Current must contain the correction value
// that was used to compute nextEdgeActual. This ensures that when AdvanceTo() enqueues
// the timestamp, the captured sawtoothCorrections accurately reflects the edge timing.
func (c *VirtualClock) generateNextEdges() {
	// Rotate sawtooth: advance Current to the value for THIS pulse
	c.sawtoothCorrections.Current = c.sawtoothCorrections.Next

	// Get error from non-sawtooth sources
	otherError := 0.0
	if c.otherGPSSimulator != nil {
		otherError = c.otherGPSSimulator(c.nextPPSNominal)
	}

	// Combine with current sawtooth correction to compute edge timing
	phaseError := otherError + c.sawtoothCorrections.Current
	c.nextEdgeActual = c.nextPPSNominal + phaseError

	// Pre-compute sawtooth for the NEXT pulse (N+1)
	if c.sawtoothGPSSimulator != nil {
		c.sawtoothCorrections.Next = c.sawtoothGPSSimulator(c.nextPPSNominal + 1.0)
	}

	// Trailing edge computation
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
// Note: VirtualClock returns era=0; TestClock.ReadTimestamp() sets the proper era.
func (c *VirtualClock) ReadTimestamp() (TimestampReading, bool) {
	if len(c.tsQueue) == 0 {
		return TimestampReading{}, false
	}
	ts := c.tsQueue[0]
	c.tsQueue = c.tsQueue[1:]

	var sawtooth *SawtoothCorrections
	if c.sawtoothGPSSimulator != nil {
		sc := ts.sawtoothCorrections
		sawtooth = &sc
	}

	return TimestampReading{
		Timestamp: phctime.Time{T: ptime.Time(ts.phase), Era: 0},
		TrueTime:  ts.trueTime,
		Sawtooth:  sawtooth,
	}, true
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
	era phctime.Era
}

// NewTestClock creates a TestClock for testing.
func NewTestClock(vclock *VirtualClock) *TestClock {
	return &TestClock{
		VirtualClock: vclock,
		era:          1, // Start at era 1
	}
}

// AdjTime steps the clock and updates the era.
func (c *TestClock) AdjTime(d time.Duration) (phctime.Era, error) {
	c.era++ // Era N+1 (uncertain)
	err := c.VirtualClock.AdjTime(d)
	c.era++ // Era N+2 (certain)
	if err != nil {
		return phctime.Era(0), err
	}
	return c.era, nil
}

// Era returns the current era value.
func (c *TestClock) Era() phctime.Era {
	return c.era
}

// ReadTimestamp reads a timestamp from the queue and attaches the current era.
func (c *TestClock) ReadTimestamp() (TimestampReading, bool) {
	reading, ok := c.VirtualClock.ReadTimestamp()
	if !ok {
		return TimestampReading{}, false
	}
	reading.Timestamp.Era = c.Era()
	return reading, true
}

// Now returns the current PHC time.
func (c *TestClock) Now() phctime.Time {
	virtPhaseNs := c.VirtualClock.computeVirtPhaseNs(c.VirtualClock.simTime)
	return phctime.Time{
		T:   ptime.Time(virtPhaseNs),
		Era: c.Era(),
	}
}
