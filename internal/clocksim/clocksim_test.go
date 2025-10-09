package clocksim

import (
	"testing"
	"time"
)

func TestVirtualClockBasic(t *testing.T) {
	// Create oscillator running 10ppm fast
	osc := ConstantDrift(10.0)
	raw := NewRawClock(osc, 0)

	// Perfect PPS (no jitter)
	pps := PerfectPPS()

	// Create virtual clock starting at t=0
	vc := NewVirtualClock(raw, pps, 0, 500000)

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

	ts, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// At t=1.0, raw clock reads 1.00001 (10ppm fast after 1 second)
	// With no freq adjustment, virtual clock should match raw clock
	expected := time.Duration(1.00001 * 1e9)
	if ts != expected {
		t.Errorf("timestamp = %v, want %v", ts, expected)
	}
}

func TestVirtualClockFreqAdjustment(t *testing.T) {
	// Create oscillator running 10ppm fast
	osc := ConstantDrift(10.0)
	raw := NewRawClock(osc, 0)
	pps := PerfectPPS()

	vc := NewVirtualClock(raw, pps, 0, 500000)

	// Apply -10e3 PPB correction to compensate for 10ppm fast oscillator
	// PPB = parts per billion, ppm = parts per million, so 10ppm = 10e3 PPB
	vc.SetFreqOffset(-10e3)

	// Advance past first PPS at t=1.0
	vc.AdvanceTo(1.1)

	ts, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// Raw clock reads 1.00001 at t=1.0 (10ppm = 10e-6 after 1 second)
	// With -10e3 PPB = -10ppm adjustment: correctedDelta = 1.00001 * (1 + (-10e3/1e9))
	// Should be very close to 1.0 (perfect)
	expectedSeconds := 1.0
	actualSeconds := ts.Seconds()
	diff := actualSeconds - expectedSeconds
	if diff > 1e-8 || diff < -1e-8 {
		t.Errorf("timestamp = %v seconds, want ~%v, diff = %v", actualSeconds, expectedSeconds, diff)
	}
}

func TestVirtualClockTimeStep(t *testing.T) {
	osc := Perfect()
	raw := NewRawClock(osc, 1000*1e9) // Start at t=1000 seconds = 1000e9 nanoseconds
	pps := PerfectPPS()

	vc := NewVirtualClock(raw, pps, 0, 500000)

	// Step clock by -1000 seconds to sync it
	vc.AdjTime(-1000 * time.Second)

	// Advance past first PPS (AdjTime advanced simTime by ~5µs, so start from there)
	vc.AdvanceTo(vc.simTime + 1.1)

	ts, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// After step, virtual clock should read ~1.0 instead of ~1001.0
	// Allow for ADJ_SETOFFSET delay error (~5µs)
	expectedSeconds := 1.0
	actualSeconds := ts.Seconds()
	diff := actualSeconds - expectedSeconds
	if diff > 1e-5 || diff < -1e-5 {
		t.Errorf("timestamp after step = %v seconds, want ~%v, diff = %v", actualSeconds, expectedSeconds, diff)
	}
}

func TestVirtualClockMultiplePPS(t *testing.T) {
	osc := Perfect()
	raw := NewRawClock(osc, 0)
	pps := PerfectPPS()

	vc := NewVirtualClock(raw, pps, 0, 500000)

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
	osc := Perfect()
	raw := NewRawClock(osc, 0)

	// PPS with +1ns jitter (avoids floating point precision issues)
	pps := func(t float64) float64 {
		return 1e-9
	}

	vc := NewVirtualClock(raw, pps, 0, 500000)

	// Advance past first PPS
	vc.AdvanceTo(1.1)

	ts, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// PPS occurs at 1.0 + 1e-9 (true time), virtual clock reads that value
	// Since oscillator is perfect and no freq adjustment, virtual time = true time
	expected := time.Duration(1000000001) // 1 second + 1 nanosecond
	if ts != expected {
		t.Errorf("timestamp = %v, want %v", ts, expected)
	}
}

// TestPrecisionAtLargeTime verifies that int64 nanosecond representation
// maintains precision even at large PHC values (like GPS epoch ~1.4e9 seconds).
// This test catches the float64 precision bug we had earlier where 10ns
// variations were quantized to ±256ns steps at large time values.
func TestPrecisionAtLargeTime(t *testing.T) {
	// Use realistic oscillator with small frequency noise
	osc := WhiteFreqNoise(0.1, 42) // 0.1 ppb stddev

	// PHC starts at GPS epoch (1.4e9 seconds = large!)
	gpsEpochNs := int64(1.4e9 * 1e9)
	raw := NewRawClock(osc, gpsEpochNs)

	// PPS with 10ns jitter (realistic GNSS)
	pps := WhiteNoisePPS(10e-9, 42)

	// Simulation starts at t=0 (small!)
	vc := NewVirtualClock(raw, pps, 0, 500000)

	// Collect timestamps from 20 PPS events over 20 simulation seconds
	timestamps := make([]time.Duration, 0, 20)
	for i := 0; i < 20; i++ {
		simTime := float64(i + 1)
		vc.AdvanceTo(simTime + 0.1)
		ts, ok := vc.ReadTimestamp()
		if !ok {
			t.Fatalf("failed to read timestamp %d", i)
		}
		timestamps = append(timestamps, ts)
	}

	// Verify timestamps are in the GPS epoch range (large values!)
	for i, ts := range timestamps {
		tsSeconds := ts.Seconds()
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
	// Create oscillator running 10ppm fast
	osc := ConstantDrift(10.0)

	// PHC starts at 0, GPS is at 1000 seconds (huge offset)
	raw := NewRawClock(osc, 0)
	pps := PerfectPPS()

	vc := NewVirtualClock(raw, pps, 0, 500000)
	tc := NewTestClock(vc)

	gpsStartTime := 1000.0

	// Step 1: Large time step to bring PHC close to GPS
	// GPS is at 1000s, PHC is at 0s, so offset is ~1000s
	vc.AdvanceTo(1.0)
	ts1, _ := tc.ReadTimestampWithEra()
	phcTime := float64(ts1.T) / 1e9
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
	ts2, _ := tc.ReadTimestampWithEra()
	phcTime = float64(ts2.T) / 1e9
	gpsTime = gpsStartTime + 2.0
	offset = phcTime - gpsTime

	// Offset should be < 10µs (ADJ_SETOFFSET delay error)
	if offset > 10e-6 || offset < -10e-6 {
		t.Errorf("offset after step = %v seconds, expected < 10µs", offset)
	}

	// Step 3: Apply frequency correction to compensate for 10ppm drift
	tc.SetFreqOffset(-10e3) // -10ppm = -10e3 ppb

	// Record offset after freq correction is applied
	offsetAfterFreqAdj := offset

	// Step 4: After several seconds with freq correction, offset should stay roughly constant
	vc.AdvanceTo(10.0)
	for vc.TimestampAvailable() {
		tc.ReadTimestampWithEra()
	}

	// Read timestamp at t=10
	vc.AdvanceTo(11.0)
	ts3, _ := tc.ReadTimestampWithEra()
	phcTime = float64(ts3.T) / 1e9
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
	osc := Perfect()
	raw := NewRawClock(osc, 1000*1e9) // Start at 1000 seconds
	pps := PerfectPPS()

	vc := NewVirtualClock(raw, pps, 0, 500000)

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
	ts, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// Timestamp at t=1.0 should be ~1 second (slightly less due to ADJ_SETOFFSET delay)
	// Because: clock was set to target=(1000-1000)=0 at time=delay, then advanced to t=1
	// So virtual clock = 0 + (1.0 - delay) = 1.0 - delay ≈ 0.999995
	expectedSeconds := 1.0
	actualSeconds := ts.Seconds()
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
	// Create oscillator running 10ppm fast
	osc := ConstantDrift(10.0)
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
// This catches the bug where WhiteFreqNoise was evaluated multiple times at the same t,
// causing different random values and making the oscillator non-deterministic.
func TestIncrementalIntegration(t *testing.T) {
	// Use WhiteFreqNoise which would expose the bug if integration re-evaluates
	osc := WhiteFreqNoise(1.0, 42)
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
	raw2 := NewRawClock(WhiteFreqNoise(1.0, 42), 0)

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
	osc := Perfect()
	raw := NewRawClock(osc, 0)

	// PPS arrives 1ms early (negative jitter)
	pps := func(t float64) float64 {
		return -1e-3
	}

	vc := NewVirtualClock(raw, pps, 0, 500000)

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

	ts, ok := vc.ReadTimestamp()
	if !ok {
		t.Fatal("failed to read timestamp")
	}

	// Timestamp should be 0.999 seconds (1ms early)
	expected := time.Duration(0.999 * 1e9)
	if ts != expected {
		t.Errorf("timestamp = %v, want %v", ts, expected)
	}
}
