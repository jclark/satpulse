# Fix RawClock Integration for Correct Stochastic Simulation

## Summary

Replace the FP-based loop in `RawClock.ReadAt` with absolute step indexing. Remainder is computed for return value but not persisted. No API changes needed.

## Problem

The integration loop uses `t += 0.001` which accumulates floating-point error. After ~1000 iterations, `t` may be slightly less than `simTime`, triggering an extra iteration with tiny dt (~1e-13s). This extra step:
1. Draws a full-variance freq sample but contributes ~0 phase
2. Saves that freq as `lastFreq` for the next second's first step
3. Creates 0.30 lag-1 correlation between adjacent 1-second intervals

Combined effect: +20% variance, 0.30 lag-1 correlation, 4ms accumulated phase drift over 524k seconds.

## TDD: Write Failing Test First

**File**: `internal/clocksim/clocksim_test.go`

Create a test that captures the bug. It should FAIL with current code, PASS after fix.

```go
func TestRawClockWhiteFMStatistics(t *testing.T) {
    // White FM with h0 = 4.5e-9 (4.5 ppb ADEV at τ=1s)
    // Expected 1-second phase error stddev: 4.5 ns
    // Expected lag-1 correlation: ~0
    const (
        h0       = 4.5e-9
        nSeconds = 10000
        seed     = 12345
    )

    osc := WhiteFMSimulator(h0, seed)
    raw := NewRawClock(osc, 0)

    // Collect 1-second phase deltas
    var phases []float64
    for sec := 0; sec <= nSeconds; sec++ {
        phaseNs := raw.ReadAt(float64(sec))
        phases = append(phases, float64(phaseNs))
    }

    // Compute deltas (phase change per second, minus nominal 1e9 ns)
    deltas := make([]float64, nSeconds)
    for i := 0; i < nSeconds; i++ {
        deltas[i] = phases[i+1] - phases[i] - 1e9
    }

    // Statistics
    mean, stddev := stats(deltas)
    lag1 := lag1Corr(deltas)

    // Check stddev: should be close to h0 * 1e9 = 4.5 ns
    // Bug causes ~5.38 ns (+20%), so allow 10% tolerance for correct behavior
    expectedStddev := h0 * 1e9
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

func stats(data []float64) (mean, stddev float64) {
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

func lag1Corr(data []float64) float64 {
    n := len(data)
    if n < 2 {
        return 0
    }
    mean, _ := stats(data)
    var num, denom float64
    for i := 0; i < n-1; i++ {
        d1 := data[i] - mean
        d2 := data[i+1] - mean
        num += d1 * d2
        denom += d1 * d1
    }
    return num / denom
}
```

**Expected behavior:**
- Current buggy code: FAILS (stddev ~5.38, lag1 ~0.30)
- Fixed code: PASSES (stddev ~4.5, lag1 ~0)

Run test:
```bash
go test -v ./internal/clocksim -run TestRawClockWhiteFMStatistics
```

## Solution

Use absolute step indexing. Track which grid steps have been processed. Remainder is computed for return value but NOT added to accumulatedSec.

### Key Points

1. **Absolute step index**: `lastStep` tracks processed steps (int64)
2. **Grid-aligned oscillator calls**: `oscillator(step * dt)` for each new step
3. **Remainder not persisted**: computed for return value only
4. **Monotonicity check**: `lastTime` ensures time never goes backwards

### Why Remainder Must Not Be Persisted

If remainder is added to accumulatedSec:
- Call 1 at t=1.0004: steps 0-999 + rem 0.0004 → accum=1.0004, grid=1.000
- Call 2 at t=2.0008: elapsed from grid=1.0008, adds 1.0008 → total=2.0012

But real interval was 1.0004, so we double-counted 0.0004s!

Fix: accumulatedSec tracks only full steps. Remainder is added to return value, not persisted.

### Rewrite RawClock

**File**: [internal/clocksim/clocksim.go](internal/clocksim/clocksim.go)

```go
type RawClock struct {
    oscillator     OscSimulator
    startPhaseNs   int64
    lastStep       int64   // Absolute step index (grid position)
    lastTime       float64 // For monotonicity check
    accumulatedSec float64 // Phase through lastStep (excludes remainder)
}

func NewRawClock(oscillator OscSimulator, startPhaseNs int64) *RawClock {
    return &RawClock{
        oscillator:     oscillator,
        startPhaseNs:   startPhaseNs,
        lastStep:       0,
        lastTime:       0.0,
        accumulatedSec: 0.0,
    }
}

func (r *RawClock) ReadAt(simTime float64) int64 {
    if simTime < r.lastTime {
        panic(fmt.Sprintf("ReadAt: time went backwards: %.9f < %.9f", simTime, r.lastTime))
    }

    // Target step index (floor)
    targetStep := int64(simTime / rawClockDT)

    // Process new full steps
    for step := r.lastStep; step < targetStep; step++ {
        t := float64(step) * rawClockDT
        freq := r.oscillator(t)
        r.accumulatedSec += rawClockDT * (1 + freq)
    }
    r.lastStep = targetStep
    r.lastTime = simTime

    // Remainder: computed for return, NOT persisted
    gridTime := float64(targetStep) * rawClockDT
    rem := simTime - gridTime

    // Return accumulated + remainder
    totalSec := r.accumulatedSec + rem
    return r.startPhaseNs + int64(totalSec*1e9)
}
```

### Example Trace

| Call | simTime | lastStep | targetStep | steps processed | gridTime | rem    | accumulatedSec | returned |
|------|---------|----------|------------|-----------------|----------|--------|----------------|----------|
| 1    | 1.0004  | 0        | 1000       | 0-999           | 1.000    | 0.0004 | ~1.000         | ~1.0004  |
| 2    | 2.0008  | 1000     | 2000       | 1000-1999       | 2.000    | 0.0008 | ~2.000         | ~2.0008  |
| 3    | 3.0012  | 2000     | 3001       | 2000-3000       | 3.001    | 0.0002 | ~3.001         | ~3.0012  |

- accumulatedSec advances by exactly 1000 or 1001 steps worth
- Remainder is always simTime - gridTime (not cumulative)
- No double-counting

### Edge Cases

**Same grid point called twice:**
- Call at 1.0004: targetStep=1000, rem=0.0004
- Call at 1.0006: targetStep=1000, no new steps, rem=0.0006
- Returns slightly more phase (correct)

**Time at exact grid point:**
- Call at 1.000: targetStep=1000, rem=0
- Works correctly

## Implementation Steps

1. **Write failing test** (`internal/clocksim/clocksim_test.go`)
   - Add `TestRawClockWhiteFMStatistics`
   - Run: should FAIL with current code

2. **Implement fix** (`internal/clocksim/clocksim.go`)
   - Update RawClock struct
   - Rewrite ReadAt method
   - Update NewRawClock

3. **Verify test passes**
   - Run: should now PASS

4. **Remove debug code**
   - Lines 82-96: per-second frequency tracking
   - Lines 112-115: spike region debug output
   - Lines 360-372: computeVirtPhaseNs debug output

5. **Run full simulation**
   ```bash
   make && out/amd64/syncsim --hw timehat-f10t.toml --duration 600000
   ```
   - Verify no spike at t=524289

## Files Modified

1. `internal/clocksim/clocksim_test.go` - Add failing test (step 1)
2. `internal/clocksim/clocksim.go` - Implement fix (step 2)

## Verification

```bash
# Step 1: Test should FAIL
go test -v ./internal/clocksim -run TestRawClockWhiteFMStatistics

# Step 3: Test should PASS after fix
go test -v ./internal/clocksim -run TestRawClockWhiteFMStatistics

# Step 5: Full simulation
make && out/amd64/syncsim --hw timehat-f10t.toml --duration 600000
```

Expected after fix:
- Stddev: 4.50 ns (was 5.38 ns)
- Lag-1 correlation: ~0 (was 0.30)
- No spike at t=524289
