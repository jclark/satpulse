# Plan: Issue #193 - Prevent Impossible Time Shifts in Reset Mode

## Problem

When GPS PPS signal issues cause the system to exit tracking mode and enter reset mode, it can re-lock to a bad phase. This happens because reset mode has no memory of where the clock was previously synchronized. The result: PTP grandmaster advertises an impossible time shift.

## Solution

Record the last good tracking sample and validate reset mode candidates against it using a drift rate limit. If the implied drift rate exceeds the limit, reject the step and remain in reset mode.

The key insight: compare elapsed **system monotonic time** vs elapsed **GPS reference time**. This handles the case where another process stepped the PHC (which would affect PHC-GPS offset but not sys-ref comparison).

## Files to Modify

1. **internal/syncsim/sync_test.go** - Add failing test first (TDD)
2. **internal/phcsync/sampler.go** - Add `SysSample()` method
3. **internal/phcsync/tracking.go** - Track persist sample in `trackingSampleProcessor`
4. **internal/phcsync/reset.go** - Add `DriftRateLimit` config, modify `resetSampleProcessor`
5. **internal/phcsync/controller.go** - Extract persist sample from tracking, pass to reset
6. **internal/phcsync/reset_test.go** - Add unit tests

## Implementation Steps (TDD)

### Step 0: Add failing test (internal/syncsim/sync_test.go)

Add a test case that demonstrates the problem. The test simulates:
1. Normal tracking for ~30s
2. Large phase shift (500ms) for 60s, causing reset
3. Currently: system re-locks to bad phase (enters tracking during shift) ❌
4. Phase shift ends, causing another reset
5. System re-locks to correct phase

**Failing test (current buggy behavior):**

```go
{
    name:                "rejects impossible phase shift (issue #193)",
    duration:            180.0,
    maxTrackingStdDev:   30,
    expectTrackingEntered: ptr(2),  // WANT: only initial + after shift ends
    // With bug: would be ptr(3) - also enters tracking during the shift
    modifyConfig: func(cfg *Config) {
        // Use shorter persist threshold for test (default 15 min would make test too long)
        cfg.Sync.Track.PersistThreshold = 30.0
        // Large 500ms phase shift from t=60 to t=120 (after persist threshold)
        cfg.Fault.Excursion = []ExcursionConfig{{
            StartTime: 60.0,
            Duration:  60.0,  // shift persists for 60 seconds
            Amplitude: 500_000_000,  // 500ms in nanoseconds
            Rise:      RampConfig{Duration: 0.01},  // instant rise
            Fall:      RampConfig{Duration: 0.01},  // instant fall
        }}
    },
},
```

**Expected behavior with fix:**
- Reset mode computes drift rate: `(sysDelta - refDelta) / refDelta * 1e9`
- During shift: sysDelta ≈ refDelta + 500ms → drift ≈ 50,000,000 PPB (50000 ppm)
- This exceeds 100 ppm limit → reject step, stay in reset
- When shift ends: drift returns to normal → accept step, proceed to tracking

### Step 1: Add `SysSample()` method to `Sample` (sampler.go)

Add method to generate a `ptime.Sample` from `phcsync.Sample`:

```go
// SysSample returns a ptime.Sample pairing the GPS reference time with
// the estimated system time of the pulse.
func (s *Sample) SysSample() ptime.Sample {
    return ptime.Sample{
        Clock: ptime.ClockTime{T: s.Ref, Era: s.Era},
        Sys:   s.Sys,
    }
}
```

### Step 2: Add config parameters

**Add to `ResetConfig` (reset.go:15-57):**

```go
DriftRateLimit float64 `toml:"driftRateLimit" check:">=0,<1_000_000_000" comment:"Max drift rate to accept step (PPB)"`
```

Add to `defaultResetConfig()`:

```go
DriftRateLimit: 100_000.0, // 100 ppm
```

**Add to `TrackingConfig` (tracking.go):**

```go
PersistThreshold float64 `toml:"persistThreshold" check:">=0,<86400" comment:"Min time in tracking before sample persists (s)"`
```

Add to `defaultTrackingConfig()`:

```go
PersistThreshold: 900.0, // 15 minutes
```

### Step 3: Track persist sample in `trackingSampleProcessor` (tracking.go:304-333)

Add fields to struct:

```go
type trackingSampleProcessor struct {
    // ... existing fields ...
    persistThreshold  time.Duration // min time in tracking before sample persists
    trackingStartTime time.Time     // system time when tracking started
    persistSample     *Sample       // persisted sample for drift validation
}
```

Initialize in constructor:

```go
persistThreshold: time.Duration(cfg.PersistThreshold * float64(time.Second)),
```

Add unexported getter method (matching `getPulseInfo` pattern):

```go
func (p *trackingSampleProcessor) getPersistSample() *Sample {
    return p.persistSample
}
```

Update `processSample()` (around line 349) - when sample is good (not bad):

```go
} else {
    p.consecutiveBadSamples = 0
    // Track start time on first good sample
    if p.trackingStartTime.IsZero() {
        p.trackingStartTime = sample.Sys
    }
    // Only persist after sufficient time in tracking mode
    if sample.Sys.Sub(p.trackingStartTime) >= p.persistThreshold {
        p.persistSample = sample
    }
}
```

This ensures we only trust the reference sample after being in tracking mode for `PersistThreshold` seconds. If tracking exits before persist threshold, `persistSample` remains nil and reset mode won't apply the drift rate check.

### Step 4: Extract persist sample when leaving tracking mode (controller.go:327-365)

In `changeMode()`, before initializing new mode processors, extract from tracking:

```go
// Extract persist sample when leaving tracking mode (c.f. getPulseInfo pattern)
var persistSample *Sample
if c.mode == ModeTracking {
    if tsp, ok := c.sampleProc.(*trackingSampleProcessor); ok {
        persistSample = tsp.getPersistSample()
    }
}
```

Then pass to reset processor:

```go
case ModeReset:
    c.pt.PulseWidth = 0
    c.sampleGen = newResetSampleGenerator(c.timeMsgBuffer, c.cfg.Reset, c.pt.EdgesPerPulse, c.freq, c.maxFreq, c.lg)
    c.sampleProc = newResetSampleProcessor(c.cfg.Reset, persistSample, c.lg)
```

### Step 5: Modify `resetSampleProcessor` (reset.go:740-774)

Update struct and constructor:

```go
type resetSampleProcessor struct {
    minStep       time.Duration
    driftRateLimit float64
    persistSample  *Sample
    lg             *slog.Logger
}

func newResetSampleProcessor(cfg ResetConfig, persistSample *Sample, lg *slog.Logger) *resetSampleProcessor {
    return &resetSampleProcessor{
        minStep:        time.Duration(cfg.StepThreshold),
        driftRateLimit: cfg.DriftRateLimit,
        persistSample:  persistSample,
        lg:             lg,
    }
}
```

### Step 6: Implement drift rate check in `processSample` (reset.go:752)

Add check before the existing offset check:

```go
// Check drift rate against persist sample from tracking
if p.persistSample != nil && p.driftRateLimit > 0 {
    driftPPB, ok := p.computeDriftRate(sample)
    if ok && abs64(driftPPB) > p.driftRateLimit {
        p.lg.Info("reset sample rejected: drift rate exceeds limit",
            "driftPPB", driftPPB,
            "limit", p.driftRateLimit,
            "elapsedSys", sample.Sys.Sub(p.persistSample.Sys),
            "elapsedRef", sample.Ref.Sub(p.persistSample.Ref))
        return phcAction{actionType: phcNoAction}, ModeReset
    }
}
```

Add helper method:

```go
func (p *resetSampleProcessor) computeDriftRate(sample *Sample) (float64, bool) {
    last := p.persistSample
    sysDelta := sample.Sys.Sub(last.Sys)
    refDelta := sample.Ref.Sub(last.Ref)

    // Need sufficient elapsed time (1 second minimum)
    if refDelta < time.Second {
        return 0, false
    }

    // Drift = (sysDelta - refDelta) / refDelta, converted to PPB
    driftNanos := float64(sysDelta - refDelta)
    return driftNanos * 1e9 / float64(refDelta), true
}
```

### Step 7: Verify failing test now passes

Run the simulation test from Step 0:
```
go test -v ./internal/syncsim -run "rejects_impossible_phase_shift"
```

The test should now pass with `expectTrackingEntered: ptr(2)`:
- Initial tracking entry after startup
- Second tracking entry after phase shift ends
- No tracking entry during the phase shift (reset mode rejects bad samples)

### Step 8: Add unit tests (reset_test.go)

Test cases for `computeDriftRate` and drift validation:
- No persist sample → accept (first boot)
- Zero drift → accept
- Small drift within limit → accept
- Large drift exceeding limit → reject
- Disabled limit (0) → accept all
- Short elapsed time (<1s) → skip check, accept

## Notes

- Default `DriftRateLimit`: 100,000 PPB (100 ppm) - catches impossible shifts while allowing normal oscillator drift
- Default `PersistThreshold`: 900 seconds (15 minutes) - must be in tracking this long before the reference sample is trusted
- When `persistSample` is nil (first boot, or tracking exited before persist threshold), skip the drift rate check entirely
- The comparison uses system monotonic time, so it's immune to PHC adjustments by other processes
- The persist mechanism prevents us from rejecting valid corrections to an erroneous startup phase
