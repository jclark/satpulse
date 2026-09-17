package clocksim

import (
	"math"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

func TestVirtualClockBasic(t *testing.T) {
	// Create oscillator running 10000ppb fast
	osc := FreqOffsetOsc(10000.0)
	raw := NewRawClock(osc, 0)

	// Perfect PPS (no jitter)
	pps := PerfectGPS()

	// Create virtual clock starting at t=0
	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)

	// Advance to just before first PPS at t=1.0
	vc.AdvanceTo(0.5)
	if vc.TimestampAvailable() {
		t.Error("should not have timestamp before PPS")
	}

	// Advance past first PPS
	vc.AdvanceTo(1.1)
	if !vc.TimestampAvailable() {
		t.Fatal("should have timestamp after PPS")
	}

	reading, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// At t=1.0, raw clock reads 1.00001 (10ppm fast after 1 second)
	// With no freq adjustment, virtual clock should match raw clock
	expected := time.Duration(1.00001 * 1e9)
	if reading.Timestamp.T != ptime.Time(expected) {
		t.Errorf("timestamp = %v, want %v", reading.Timestamp.T, ptime.Time(expected))
	}
}

func TestVirtualClockFreqAdjustment(t *testing.T) {
	// Create oscillator running 10000ppb fast
	osc := FreqOffsetOsc(10000.0)
	raw := NewRawClock(osc, 0)
	pps := PerfectGPS()

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)

	// Apply -10e3 PPB correction to compensate for 10ppm fast oscillator
	// PPB = parts per billion, ppm = parts per million, so 10ppm = 10e3 PPB
	vc.SetFreqOffset(-10e3)

	// Advance past first PPS at t=1.0
	vc.AdvanceTo(1.1)

	reading, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// Raw clock reads 1.00001 at t=1.0 (10ppm = 10e-6 after 1 second)
	// With -10e3 PPB = -10ppm adjustment: correctedDelta = 1.00001 * (1 + (-10e3/1e9))
	// Should be very close to 1.0 (perfect)
	expectedSeconds := 1.0
	actualSeconds := float64(reading.Timestamp.T) / 1e9
	diff := actualSeconds - expectedSeconds
	if diff > 1e-8 || diff < -1e-8 {
		t.Errorf("timestamp = %v seconds, want ~%v, diff = %v", actualSeconds, expectedSeconds, diff)
	}
}

func TestVirtualClockTimeStep(t *testing.T) {
	osc := PerfectOsc()
	raw := NewRawClock(osc, 1000*1e9) // Start at t=1000 seconds = 1000e9 nanoseconds
	pps := PerfectGPS()

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)

	// Step clock by -1000 seconds to sync it
	vc.AdjTime(-1000 * time.Second)

	// Advance past first PPS (AdjTime advanced simTime by ~5µs, so start from there)
	vc.AdvanceTo(vc.simTime + 1.1)

	reading, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// After step, virtual clock should read ~1.0 instead of ~1001.0
	// Allow for ADJ_SETOFFSET delay error (~5µs)
	expectedSeconds := 1.0
	actualSeconds := float64(reading.Timestamp.T) / 1e9
	diff := actualSeconds - expectedSeconds
	if diff > 1e-5 || diff < -1e-5 {
		t.Errorf("timestamp after step = %v seconds, want ~%v, diff = %v", actualSeconds, expectedSeconds, diff)
	}
}

func TestVirtualClockMultiplePPS(t *testing.T) {
	osc := PerfectOsc()
	raw := NewRawClock(osc, 0)
	pps := PerfectGPS()

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)

	// Advance past 3 PPS events
	vc.AdvanceTo(3.5)

	// Should have 3 timestamps
	count := 0
	for vc.TimestampAvailable() {
		_, ok := vc.ReadTimestamp()
		if !ok {
			break
		}
		count++
	}

	if count != 3 {
		t.Errorf("got %d timestamps, want 3", count)
	}
}

func TestVirtualClockPPSJitter(t *testing.T) {
	osc := PerfectOsc()
	raw := NewRawClock(osc, 0)

	// PPS with +1ns jitter (avoids floating point precision issues)
	pps := func(t float64) float64 {
		return 1e-9
	}

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)

	// Advance past first PPS
	vc.AdvanceTo(1.1)

	reading, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// PPS occurs at 1.0 + 1e-9 (true time), virtual clock reads that value
	// Since oscillator is perfect and no freq adjustment, virtual time = true time
	expected := ptime.Time(1000000001) // 1 second + 1 nanosecond
	if reading.Timestamp.T != expected {
		t.Errorf("timestamp = %v, want %v", reading.Timestamp.T, expected)
	}
}

// TestPrecisionAtLargeTime verifies that int64 nanosecond representation
// maintains precision even at large PHC values (like GPS epoch ~1.4e9 seconds).
// This test catches the float64 precision bug we had earlier where 10ns
// variations were quantized to ±256ns steps at large time values.
func TestPrecisionAtLargeTime(t *testing.T) {
	// Use realistic oscillator with small frequency noise
	osc := WhiteNoiseOsc(3.5, 42) // 100 ppb stddev

	// PHC starts at GPS epoch (1.4e9 seconds = large!)
	gpsEpochNs := int64(1.4e9 * 1e9)
	raw := NewRawClock(osc, gpsEpochNs)

	// PPS with 10ns jitter (realistic GNSS)
	pps := JitterGPS(10, 42) // 10 nanoseconds

	// Simulation starts at t=0 (small!)
	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)

	// Collect timestamps from 20 PPS events over 20 simulation seconds
	timestamps := make([]ptime.Time, 0, 20)
	for i := range 20 {
		simTime := float64(i + 1)
		vc.AdvanceTo(simTime + 0.1)
		reading, ok := vc.ReadTimestamp()
		if !ok {
			t.Fatalf("failed to read timestamp %d", i)
		}
		timestamps = append(timestamps, reading.Timestamp.T)
	}

	// Verify timestamps are in the GPS epoch range (large values!)
	for i, ts := range timestamps {
		tsSeconds := float64(ts) / 1e9
		// Should be near 1.4e9 + i+1 seconds
		expected := 1.4e9 + float64(i+1)
		diff := tsSeconds - expected
		// Allow for oscillator drift and PPS jitter (should be < 1µs)
		if diff > 1e-6 || diff < -1e-6 {
			t.Errorf("timestamp %d = %.9f seconds, expected ~%.9f, diff=%e",
				i, tsSeconds, expected, diff)
		}
	}

	// Critical test: verify nanosecond-level variation at large PHC values
	// Calculate deltas between consecutive timestamps
	deltas := make([]int64, len(timestamps)-1)
	for i := 1; i < len(timestamps); i++ {
		deltas[i-1] = int64(timestamps[i] - timestamps[i-1])
	}

	// Check that deltas are all close to 1 second
	for i, delta := range deltas {
		if delta < 999999900 || delta > 1000000100 {
			t.Errorf("delta %d = %v ns, should be ~1e9", i, delta)
		}
	}

	// Verify we see nanosecond-level variation, not quantization
	// Count unique delta values (grouped into 5ns buckets)
	uniqueOffsets := make(map[int64]bool)
	for _, delta := range deltas {
		offset := (delta - 1000000000) / 5
		uniqueOffsets[offset] = true
	}

	// With 10ns jitter over 19 samples, we should see multiple unique offsets
	// If we only see 1-2 values, that indicates quantization
	if len(uniqueOffsets) < 5 {
		t.Errorf("only %d unique offset buckets, indicates quantization", len(uniqueOffsets))
	}

	// Verify no systematic quantization to 256ns boundaries
	// (this was the symptom of the float64 precision bug)
	quantizedCount := 0
	for _, delta := range deltas {
		offset := delta - 1000000000
		if offset%256 == 0 {
			quantizedCount++
		}
	}
	// With random 10ns jitter, probability of landing on 256ns boundary is ~1/256
	// Expect ~19/256 ≈ 0.07, allow up to 3
	if quantizedCount > 3 {
		t.Errorf("%d/%d deltas are multiples of 256ns, indicates precision loss",
			quantizedCount, len(deltas))
	}
}

// TestServoConvergence simulates realistic servo behavior with large initial offset.
// This catches issues with offset tracking, frequency adjustment, and time stepping.
func TestServoConvergence(t *testing.T) {
	// Create oscillator running 10000ppb fast
	osc := FreqOffsetOsc(10000.0)

	// PHC starts at 0, GPS is at 1000 seconds (huge offset)
	raw := NewRawClock(osc, 0)
	pps := PerfectGPS()

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)
	tc := NewTestClock(vc)

	gpsStartTime := 1000.0

	// Step 1: Large time step to bring PHC close to GPS
	// GPS is at 1000s, PHC is at 0s, so offset is ~1000s
	vc.AdvanceTo(1.0)
	reading1, _ := tc.ReadTimestamp()
	phcTime := float64(reading1.Timestamp.T) / 1e9
	gpsTime := gpsStartTime + 1.0
	offset := phcTime - gpsTime

	// Offset should be ~-999 seconds (PHC behind GPS)
	if offset > -998 || offset < -1000 {
		t.Errorf("initial offset = %v seconds, expected ~-999", offset)
	}

	// Step clock to reduce offset
	tc.AdjTime(time.Duration(offset * -1e9))

	// Step 2: After step, offset should be small (microseconds)
	vc.AdvanceTo(2.0)
	reading2, _ := tc.ReadTimestamp()
	phcTime = float64(reading2.Timestamp.T) / 1e9
	gpsTime = gpsStartTime + 2.0
	offset = phcTime - gpsTime

	// Offset should be < 10µs (ADJ_SETOFFSET delay error)
	if offset > 10e-6 || offset < -10e-6 {
		t.Errorf("offset after step = %v seconds, expected < 10µs", offset)
	}

	// Step 3: Apply frequency correction to compensate for 10000ppb drift
	tc.SetFreqOffset(-10e3) // -10000ppb

	// Record offset after freq correction is applied
	offsetAfterFreqAdj := offset

	// Step 4: After several seconds with freq correction, offset should stay roughly constant
	vc.AdvanceTo(10.0)
	for vc.TimestampAvailable() {
		_, _ = tc.ReadTimestamp()
	}

	// Read timestamp at t=10
	vc.AdvanceTo(11.0)
	reading3, _ := tc.ReadTimestamp()
	phcTime = float64(reading3.Timestamp.T) / 1e9
	gpsTime = gpsStartTime + 11.0
	offsetFinal := phcTime - gpsTime

	// With proper freq compensation, offset should not grow significantly
	// Allow offset to change by < 1µs over 9 seconds (much better than 90µs without compensation)
	offsetChange := offsetFinal - offsetAfterFreqAdj
	if offsetChange > 1e-6 || offsetChange < -1e-6 {
		t.Errorf("offset changed by %v seconds over 9s (initial=%v, final=%v), expected < 1µs drift",
			offsetChange, offsetAfterFreqAdj, offsetFinal)
	}
}

// TestAdjTimeDelay verifies that AdjTime() simulates realistic kernel delay.
// This catches issues with ADJ_SETOFFSET implementation.
func TestAdjTimeDelay(t *testing.T) {
	osc := PerfectOsc()
	raw := NewRawClock(osc, 1000*1e9) // Start at 1000 seconds
	pps := PerfectGPS()

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)

	// Record time before AdjTime
	timeBefore := vc.simTime

	// Step clock back by -1000 seconds to sync it to ~0
	vc.AdjTime(-1000 * time.Second)

	// Time should have advanced by ~5µs (delay)
	timeAfter := vc.simTime
	delay := timeAfter - timeBefore

	// Delay should be between 1µs and 10µs (5µs ± jitter)
	if delay < 1e-6 || delay > 10e-6 {
		t.Errorf("AdjTime delay = %v seconds, expected ~5µs", delay)
	}

	// After the step, advance to get the first PPS timestamp
	vc.AdvanceTo(1.1)
	reading, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// Timestamp at t=1.0 should be ~1 second (slightly less due to ADJ_SETOFFSET delay)
	// Because: clock was set to target=(1000-1000)=0 at time=delay, then advanced to t=1
	// So virtual clock = 0 + (1.0 - delay) = 1.0 - delay ≈ 0.999995
	expectedSeconds := 1.0
	actualSeconds := float64(reading.Timestamp.T) / 1e9
	diff := actualSeconds - expectedSeconds

	// Should be within ~10µs of 1 second (clock lags due to ADJ_SETOFFSET delay)
	if diff > 0 || diff < -10e-6 {
		t.Errorf("timestamp after AdjTime = %v seconds, want ~%v, diff = %v",
			actualSeconds, expectedSeconds, diff)
	}
}

// TestRawClockIntegration verifies that RawClock correctly integrates
// oscillator frequency error over time.
func TestRawClockIntegration(t *testing.T) {
	// Create oscillator running 10000ppb fast
	osc := FreqOffsetOsc(10000.0)
	raw := NewRawClock(osc, 0)

	// After 1 second, clock should read 1.00001 (10µs fast)
	phase1 := raw.ReadAt(1.0)
	expected1 := int64(1.00001 * 1e9)
	// Allow for 1-2ns rounding error from trapezoidal integration
	if phase1 < expected1-2 || phase1 > expected1+2 {
		t.Errorf("phase at t=1 = %v ns, want %v ns (±2ns)", phase1, expected1)
	}

	// After 100 seconds, clock should read 100.001 (1ms fast)
	phase100 := raw.ReadAt(100.0)
	expected100 := int64(100.001 * 1e9)
	// Allow for small rounding error from trapezoidal integration
	if phase100 < expected100-10 || phase100 > expected100+10 {
		t.Errorf("phase at t=100 = %v ns, want %v ns (±10ns)", phase100, expected100)
	}

	// Verify linearity (should be proportional to time)
	ratio := float64(phase100) / float64(phase1)
	expectedRatio := 100.001 / 1.00001
	if ratio < expectedRatio-0.01 || ratio > expectedRatio+0.01 {
		t.Errorf("phase ratio = %v, want %v", ratio, expectedRatio)
	}
}

// TestIncrementalIntegration verifies that RawClock integration is truly incremental.
// This catches the bug where WhiteNoiseOsc was evaluated multiple times at the same t,
// causing different random values and making the oscillator non-deterministic.
func TestIncrementalIntegration(t *testing.T) {
	// Use WhiteNoiseOsc which would expose the bug if integration re-evaluates
	osc := WhiteNoiseOsc(1000.0, 42)
	raw := NewRawClock(osc, 0)

	// Read at t=2.0 - this integrates from 0→2
	phase1 := raw.ReadAt(2.0)

	// Read at t=3.0 - with incremental integration, this only integrates 2→3
	// With the old bug, this would re-integrate 0→3 with different random values
	phase2 := raw.ReadAt(3.0)

	// The phase difference should be ~1 second (with some noise)
	deltaNs := phase2 - phase1
	diffSeconds := float64(deltaNs) / 1e9

	// Should be close to 1.0 second, allow ±1ms for noise
	if diffSeconds < 0.999 || diffSeconds > 1.001 {
		t.Errorf("phase delta = %.9f seconds, expected ~1.0", diffSeconds)
	}

	// More importantly: verify determinism by reading at same point again
	// Create a new RawClock with same seed
	raw2 := NewRawClock(WhiteNoiseOsc(1000.0, 42), 0)

	// Should get exact same values
	phase1b := raw2.ReadAt(2.0)
	phase2b := raw2.ReadAt(3.0)

	if phase1 != phase1b {
		t.Errorf("non-deterministic at t=2: %v != %v", phase1, phase1b)
	}
	if phase2 != phase2b {
		t.Errorf("non-deterministic at t=3: %v != %v", phase2, phase2b)
	}
}

// TestNegativePPSJitter verifies that negative PPS jitter (early arrival) works.
// This was a source of bugs in the lazy PPS generation logic.
func TestNegativePPSJitter(t *testing.T) {
	osc := PerfectOsc()
	raw := NewRawClock(osc, 0)

	// PPS arrives 1ms early (negative jitter)
	pps := func(t float64) float64 {
		return -1e-3
	}

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, 0, nil)

	// Advance to t=0.995 (before nominal PPS at t=1.0, but after actual PPS at t=0.999)
	vc.AdvanceTo(0.995)

	// Should not have timestamp yet (actual PPS is at 0.999)
	if vc.TimestampAvailable() {
		t.Error("should not have timestamp before actual PPS time")
	}

	// Advance past actual PPS time
	vc.AdvanceTo(1.0)

	// Should have timestamp now
	if !vc.TimestampAvailable() {
		t.Fatal("should have timestamp after actual PPS time")
	}

	reading, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// Timestamp should be 0.999 seconds (1ms early)
	expected := ptime.Time(0.999 * 1e9)
	if reading.Timestamp.T != expected {
		t.Errorf("timestamp = %v, want %v", reading.Timestamp.T, expected)
	}
}

// TestAdjTimeDelayNonNegative verifies that ADJ_SETOFFSET delay is always non-negative.
// This catches the bug where Gaussian tail could produce negative delays causing panic.
func TestAdjTimeDelayNonNegative(t *testing.T) {
	delayGen := defaultAdjTimeDelay()

	// Sample many times to catch tail events
	for i := range 10000 {
		delay := delayGen()
		if delay < 0 {
			t.Errorf("sample %d: delay = %v, must be non-negative", i, delay)
		}
	}
}

// TestAdjTimeDelayMean verifies that rejection sampling preserves the ~5µs mean.
func TestAdjTimeDelayMean(t *testing.T) {
	delayGen := defaultAdjTimeDelay()

	// Sample to compute mean
	sum := 0.0
	n := 10000
	for range n {
		sum += delayGen()
	}
	mean := sum / float64(n)

	// Should be close to 5µs (allow ±10% for statistical variation)
	if mean < 4.5e-6 || mean > 5.5e-6 {
		t.Errorf("mean delay = %v, expected ~5µs", mean)
	}
}

// TestDualEdgeMode verifies that dual-edge mode generates two timestamps per second.
func TestDualEdgeMode(t *testing.T) {
	// Perfect oscillator for predictable behavior
	osc := PerfectOsc()
	raw := NewRawClock(osc, 0)

	// Perfect PPS (no jitter on rising edge)
	pps := PerfectGPS()

	// Perfect trailing edge (no noise)
	trailingEdge := PerfectGPS()

	// 200ms pulse width
	pulseWidth := 200 * time.Millisecond

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, pulseWidth, trailingEdge)

	// Advance past 3 seconds
	vc.AdvanceTo(3.5)

	// Should have 6 timestamps (2 per second)
	timestamps := []ptime.Time{}
	for vc.TimestampAvailable() {
		reading, ok := vc.ReadTimestamp()
		if !ok {
			break
		}
		timestamps = append(timestamps, reading.Timestamp.T)
	}

	if len(timestamps) != 6 {
		t.Fatalf("got %d timestamps, want 6", len(timestamps))
	}

	// Verify timestamps alternate: rising, trailing, rising, trailing, ...
	for i := range 3 {
		risingIdx := i * 2
		trailingIdx := i*2 + 1

		rising := timestamps[risingIdx]
		trailing := timestamps[trailingIdx]

		// Rising edge should be at ~(i+1) seconds.
		// Multiply in time.Duration (int64) to avoid int overflow on 32-bit.
		expectedRising := ptime.Time(time.Duration(i+1) * time.Second)
		if rising != expectedRising {
			t.Errorf("rising edge %d = %v, want %v", i, rising, expectedRising)
		}

		// Trailing edge should be 200ms after rising edge
		expectedTrailing := expectedRising + ptime.Time(pulseWidth)
		if trailing != expectedTrailing {
			t.Errorf("trailing edge %d = %v, want %v", i, trailing, expectedTrailing)
		}

		// Pulse width should be exactly 200ms (perfect oscillator)
		actualPulseWidth := trailing - rising
		if actualPulseWidth != ptime.Time(pulseWidth) {
			t.Errorf("pulse width %d = %v, want %v", i, actualPulseWidth, pulseWidth)
		}
	}
}

// TestDualEdgeModeWithDrift verifies pulse width is affected by oscillator drift.
func TestDualEdgeModeWithDrift(t *testing.T) {
	// Oscillator running 10000ppb fast
	osc := FreqOffsetOsc(10000.0)
	raw := NewRawClock(osc, 0)

	pps := PerfectGPS()
	trailingEdge := PerfectGPS()

	// 200ms pulse width
	pulseWidth := 200 * time.Millisecond

	vc := NewVirtualClock(raw, nil, pps, 0, 500000, pulseWidth, trailingEdge)

	// Advance past first second
	vc.AdvanceTo(1.5)

	// Get rising and trailing edge
	risingReading, ok1 := vc.ReadTimestamp()
	if !ok1 {
		t.Fatal("failed to read rising edge")
	}
	trailingReading, ok2 := vc.ReadTimestamp()
	if !ok2 {
		t.Fatal("failed to read trailing edge")
	}

	// Measured pulse width by the PHC
	measuredWidth := trailingReading.Timestamp.T - risingReading.Timestamp.T

	// With 10000ppb drift over 200ms: 200ms * 10ppm = 2µs extra
	// So PHC measures: 200ms + 2µs = 200.002ms
	expectedWidthNs := int64(200_002_000) // 200.002ms in nanoseconds
	actualWidthNs := int64(measuredWidth)

	// Allow 1ns tolerance for rounding
	diff := actualWidthNs - expectedWidthNs
	if diff < -1 || diff > 1 {
		t.Errorf("measured pulse width = %v ns, want %v ns (±1ns)", actualWidthNs, expectedWidthNs)
	}
}

// TestSawtoothMetadataConsistency verifies that sawtooth metadata matches actual edge timing.
// This tests the fix for the invariant violation where sawtoothCorrections.Current
// was rotated before computing edge timing, causing metadata mismatch.
func TestSawtoothMetadataConsistency(t *testing.T) {
	// Create oscillator with small drift
	osc := FreqOffsetOsc(100.0) // 100ppb
	raw := NewRawClock(osc, 0)

	// Create sawtooth PPS with 8ns amplitude
	sawtoothPPS := SawtoothGPS(osc, 8e-9, 0.5)
	otherPPS := PerfectGPS()

	vc := NewVirtualClock(raw, sawtoothPPS, otherPPS, 0, 500000, 0, nil)

	// Advance through several PPS events
	for i := 1; i <= 5; i++ {
		vc.AdvanceTo(float64(i) + 0.1)

		reading, ok := vc.ReadTimestamp()
		if !ok {
			t.Fatalf("failed to read timestamp at second %d", i)
		}

		if reading.Sawtooth == nil {
			t.Fatalf("sawtooth metadata missing at second %d", i)
		}

		// The sawtooth correction should have been applied to the edge timing
		// Verify that Current is what was actually used
		actualTime := reading.TrueTime
		nominalTime := float64(i)
		actualCorrection := actualTime - nominalTime

		// The reported Current should match what was actually applied (within tolerance)
		reportedCurrent := reading.Sawtooth.Current
		diff := actualCorrection - reportedCurrent

		// Allow small difference for other noise sources, but should be very close
		if diff > 1e-9 || diff < -1e-9 {
			t.Errorf("second %d: actual correction = %.12f, reported Current = %.12f, diff = %.12f",
				i, actualCorrection, reportedCurrent, diff)
		}
	}
}

// TestSawtoothArbitraryStartTime verifies sawtooth works correctly when simulation
// starts at arbitrary times (not t=0). This tests the fix where NewVirtualClock
// was initializing sawtooth with epoch 1.0 instead of firstPPSNominal.
func TestSawtoothArbitraryStartTime(t *testing.T) {
	// Start simulation at a large time (e.g., 124.5 seconds)
	startTime := 124.5

	osc := FreqOffsetOsc(1000.0) // 1000ppb = 1ppm
	raw := NewRawClock(osc, int64(startTime*1e9))

	// Create sawtooth PPS with 8ns amplitude
	sawtoothPPS := SawtoothGPS(osc, 8e-9, 0.5)
	otherPPS := PerfectGPS()

	vc := NewVirtualClock(raw, sawtoothPPS, otherPPS, startTime, 500000, 0, nil)

	// Collect sawtooth corrections over several seconds
	corrections := []float64{}
	for i := range 5 {
		targetTime := startTime + float64(i) + 1.0
		vc.AdvanceTo(targetTime + 0.1)

		reading, ok := vc.ReadTimestamp()
		if !ok {
			t.Fatalf("failed to read timestamp at second %d", i)
		}

		if reading.Sawtooth == nil {
			t.Fatalf("sawtooth metadata missing at second %d", i)
		}

		corrections = append(corrections, reading.Sawtooth.Current)
	}

	// Verify sawtooth corrections are reasonable (within amplitude bounds)
	// With 8ns amplitude and zero-mean, corrections should be in [-4ns, +4ns]
	for i, corr := range corrections {
		if corr > 4e-9 || corr < -4e-9 {
			t.Errorf("second %d: sawtooth correction = %.12f, exceeds amplitude bounds ±4ns", i, corr)
		}
	}

	// Verify sawtooth shows proper phase evolution (not stuck at one value)
	// With 1000ppb frequency offset over 8ns amplitude, period is 8ns/1e-6 = 8000s
	// Over 5 seconds, phase should advance by 5/8000 = 0.000625 cycles
	// That's tiny, so we mainly check corrections aren't all identical
	allSame := true
	for i := 1; i < len(corrections); i++ {
		if corrections[i] != corrections[0] {
			allSame = false
			break
		}
	}

	// With numerical integration and RNG, values should vary
	if allSame {
		t.Errorf("all sawtooth corrections are identical: %.12f (expected variation)", corrections[0])
	}
}

// TestRawClockWhiteFMStatistics verifies that RawClock integration produces correct
// statistics for white FM noise. This test catches the FP accumulation bug where
// extra micro-steps create spurious lag-1 correlation and inflated variance.
func TestRawClockWhiteFMStatistics(t *testing.T) {
	// White FM with h0 = 4.5e-9 (4.5 ppb ADEV at τ=1s)
	// Expected 1-second phase error stddev: 4.5 ns
	// Expected lag-1 correlation: ~0
	const (
		h0PPB    = 4.5
		nSeconds = 100000
		seed     = 12345
	)

	osc := WhiteNoiseOsc(h0PPB, seed)
	raw := NewRawClock(osc, 0)

	// Collect 1-second phase deltas
	var phases []float64
	for sec := 0; sec <= nSeconds; sec++ {
		phaseNs := raw.ReadAt(float64(sec))
		phases = append(phases, float64(phaseNs))
	}

	// Compute deltas (phase change per second, minus nominal 1e9 ns)
	deltas := make([]float64, nSeconds)
	for i := range nSeconds {
		deltas[i] = phases[i+1] - phases[i] - 1e9
	}

	// Statistics
	mean, stddev := testStats(deltas)
	lag1 := testLag1Corr(deltas)

	// Check stddev: should be close to h0 * 1e9 = 4.5 ns
	// Bug causes ~5.38 ns (+20%), so allow 10% tolerance for correct behavior
	expectedStddev := h0PPB
	if stddev < expectedStddev*0.9 || stddev > expectedStddev*1.1 {
		t.Errorf("stddev = %.3f ns, want %.3f ± 10%%", stddev, expectedStddev)
	}

	// Check lag-1 correlation: should be near 0
	// Bug causes ~0.30, so fail if |lag1| > 0.1
	if math.Abs(lag1) > 0.1 {
		t.Errorf("lag-1 correlation = %.4f, want |lag1| < 0.1", lag1)
	}

	// Mean should be near 0
	if math.Abs(mean) > 1.0 {
		t.Errorf("mean = %.3f ns, want near 0", mean)
	}
}

func testStats(data []float64) (mean, stddev float64) {
	n := float64(len(data))
	for _, v := range data {
		mean += v
	}
	mean /= n
	for _, v := range data {
		d := v - mean
		stddev += d * d
	}
	stddev = math.Sqrt(stddev / n)
	return
}

func testLag1Corr(data []float64) float64 {
	n := len(data)
	if n < 2 {
		return 0
	}
	mean, _ := testStats(data)
	var num, denom float64
	for i := 0; i < n-1; i++ {
		d1 := data[i] - mean
		d2 := data[i+1] - mean
		num += d1 * d2
		denom += d1 * d1
	}
	return num / denom
}

// TestSawtoothPhaseAlignment verifies that sawtooth phase advances correctly
// based on oscillator frequency error without artificial jumps.
func TestSawtoothPhaseAlignment(t *testing.T) {
	// Frequency offset chosen to give non-integer phase advancement
	// With 8ns amplitude: deltaTicks = 750 * 1e-9 * 1.0 / 8e-9 = 93.75 ticks per second
	// After 8 seconds: 750 ticks accumulated, phase wraps multiple times
	osc := FreqOffsetOsc(750) // 750 ppb
	raw := NewRawClock(osc, 0)

	sawtoothPPS := SawtoothGPS(osc, 8e-9, 0.25) // Start at phase 0.25
	otherPPS := PerfectGPS()

	vc := NewVirtualClock(raw, sawtoothPPS, otherPPS, 0, 5e6, 0, nil)

	// Collect corrections over 10 seconds
	// Phase advances by 93.75 ticks/sec * 10 sec = 937.5 ticks = 937 full cycles + 0.5 tick
	corrections := []float64{}
	for i := 1; i <= 10; i++ {
		vc.AdvanceTo(float64(i) + 0.1)
		reading, _ := vc.ReadTimestamp()
		if reading.Sawtooth != nil {
			corrections = append(corrections, reading.Sawtooth.Current)
		}
	}

	if len(corrections) != 10 {
		t.Fatalf("expected 10 corrections, got %d", len(corrections))
	}

	// Verify corrections wrap through the full range
	// Should see both positive and negative values as phase cycles
	hasPositive := false
	hasNegative := false
	for _, corr := range corrections {
		if corr > 1e-9 {
			hasPositive = true
		}
		if corr < -1e-9 {
			hasNegative = true
		}
	}

	if !hasPositive || !hasNegative {
		t.Errorf("sawtooth should cycle through positive and negative, got: %v", corrections)
	}
}
