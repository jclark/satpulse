# Gap mode (#188)

## Introduction

Gap mode is used when there is a loss of PPS short enough that PTP clock quality does not need to change. It is intermediate between normal tracking (where every sample is processed) and holdover (where PTP quality is degraded).

Gap mode consolidates two pieces of missing-sample handling:

1. **Frequency adjustment**: When samples go missing, the servo switches to an averaged frequency estimate (`avgFreq`). This is currently implemented inline in tracking mode.

2. **MAD recovery**: After a gap, post-gap samples may be incorrectly rejected as outliers. This requires relaxed MAD detection and window shifting, as described in `mad-missing-samples.md`.

By handling both in a dedicated mode, tracking becomes simpler (it only handles present samples) and the design is more uniform with full holdover mode.

## Why is a gap mode needed?

When GPS signal is briefly lost during tracking:

1. The servo switches to an averaged frequency estimate
2. The PHC drifts slightly (typically under 100ns for brief outages)
3. When signal returns, the first samples show offset reflecting this drift
4. The MAD detector, still comparing against pre-gap statistics, may incorrectly reject these legitimate samples as outliers

The post-gap samples are not outliers: they correctly reflect that the PHC drifted. But the MAD detector doesn't know about the gap; it just sees a sudden jump.

## Design Goals

1. **Uniform with holdover**: Gap mode and holdover mode should share conceptual structure (entry criteria, recovery phases, exit conditions)
2. **Clean separation**: Gap mode is a distinct mode, not inline logic cluttering the tracking processor
3. **Preserve state**: Unlike mode transitions that create fresh processors, gap mode preserves the tracking processor's state (MAD window, servo, avgFreq) for use during recovery and restoration
4. **Same effect**: Achieve identical behavior to `mad-missing-samples.md` gap recovery

## Comparison: Gap vs Holdover mode

| Aspect | Gap | Holdover |
|--------|---------------|----------|
| Purpose | Brief signal loss, maintain tracking quality | Extended signal loss, degrade gracefully |
| PTP clock class | Unchanged (tracking quality) | ClockClassHoldover (7) |
| Entry trigger | Any missing sample | `consecutiveMissingSamples >= holdoverThreshold` |
| Frequency control | Use tracking's `avgFreq` | Blend short-term and long-term frequency estimates |
| Recovery | Relaxed MAD detection, shift window (if gap was long enough) | Drift check, reconvergence phase |
| Exit to tracking | Directly (short gap) or after window shift (long gap) | After reconvergence criteria met |

## State Machine

```
tracking ──[any missing sample]──> gap
                                        │
                    ┌───────────────────┼───────────────────┐
                    │                   │                   │
                    ▼                   ▼                   ▼
          [holdoverThreshold]   [recovery success]   [drift > limit]
                    │                   │                   │
                    ▼                   ▼                   ▼
               holdover             tracking              reset
```

**Transitions:**

- **tracking -> gap mode**: Any missing sample
- **gap mode -> holdover**: `consecutiveMissingSamples >= holdoverThreshold` (deferred to #199; until holdover exists, a long gap exits to reset via `badSampleRunLimit`)
- **gap mode -> tracking**: Successful recovery
- **gap mode -> reset**: Recovery drift exceeds `driftLimit`

## Configuration

**Principle**: Parameters belong to the config section of the mode that uses them, not the mode they trigger transition to.

New config section `[sync.gap]`:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `recoveryThreshold` | int | 3 | Consecutive missing samples before MAD recovery is needed |
| `driftLimit` | int64 | 100 | Max allowed median shift (ns) from pre-gap baseline; if exceeded, exit to reset |
| `recoverySamples` | int | 5 | Samples to collect before shifting window |

`holdoverThreshold` (consecutive missing samples to transition to full holdover) is deferred to #199 along with the holdover mode itself; adding it before the mode exists would be dead config.

**Parameter semantics:**

- `recoveryThreshold`: Short gaps (fewer missing samples) don't cause enough drift to need MAD window adjustment. Longer gaps do. This threshold controls when the full recovery procedure (relaxed MAD detection, collect samples, shift window) is triggered.

**Shared parameters from tracking**: Gap mode is in some ways a sub-mode of tracking and uses several tracking parameters directly:
- `badSampleRunLimit`: exits to reset if too many consecutive bad samples during recovery
- MAD parameters (`MADMinSamples`, `MADMultiple`, etc.): used for relaxed outlier detection

**Constraints:**

- `recoveryThreshold < badSampleRunLimit` (enforced by `Config.Validate`), so MAD recovery can trigger before the bad sample limit forces an exit. The default of 3 sits below the default `badSampleRunLimit` of 5.
- When #199 adds `holdoverThreshold`, the full chain becomes `recoveryThreshold < holdoverThreshold < badSampleRunLimit`, so holdover is possible before the bad sample limit forces an exit. (Note the default `badSampleRunLimit` of 5 will need raising, or the holdover default choosing accordingly.)

## Phases

### Phase 1: No Samples

While samples are missing:

- Apply `avgFreq` from tracking processor (moved from tracking mode)
- Increment the missing-sample count and the shared bad-sample bookkeeping; if a bad-sample limit trips, exit to reset
- (#199) If `consecutiveMissingSamples >= holdoverThreshold`: transition to holdover

### Phase 2: Recovery

When samples return, behavior depends on gap length:

**Short gap** (`consecutiveMissingSamples < recoveryThreshold`):
- On the first present sample: process it normally via the tracking processor (servo update, MAD update) and return `ModeTracking`
- No MAD window adjustment needed (drift too small to cause issues)

**Long gap** (`consecutiveMissingSamples >= recoveryThreshold`):
- Use relaxed MAD detection: add `driftLimit` to normal MAD threshold
- If sample passes relaxed check: accept, feed to servo, collect for recovery
- If sample fails relaxed check: reject as outlier, increment bad sample counter
- If bad samples exceed `badSampleRunLimit`: exit to reset
- After collecting `recoverySamples` accepted samples: proceed to window shift

### Phase 3: Window Shift (long gaps only)

After collecting enough recovery samples:

```go
preMedian := trackingProc.madWindow.Median()
newMedian := computeMedian(p.recoveryOffsets)  // median of collected offsets
shift := newMedian - preMedian

if shift.Abs() > driftLimit {
    return ModeReset
}

trackingProc.madWindow.ShiftAll(shift)
for _, offset := range p.recoveryOffsets {
    trackingProc.madWindow.Add(offset)
}
return ModeTracking
```

## Implementation

### New Mode Constant

```go
const (
    ModeInvalid Mode = iota
    ModeReset
    ModeConverging
    ModeTracking
    ModeGap
    // ModeHoldover will be added later
    NModes
)
```

**PTP quality**: `ModeGap` must be treated as in-sync for PTP clock quality reporting. Any code that maps modes to PTP clock class or state should treat `ModeGap` identically to `ModeTracking` (i.e., no degradation of clock quality).

### Sample Generator

Gap mode reuses the tracking sample generator. The controller preserves it during the tracking -> gap mode transition.

### Sample Processor

New `gapSampleProcessor`:

```go
type gapSampleProcessor struct {
    cfg             GapConfig
    trackingProc    *trackingSampleProcessor // preserved from tracking mode
    missingSamples  int                      // consecutive missing samples, including the one that triggered gap mode
    recoveryOffsets []time.Duration          // accepted post-gap offsets collected during MAD recovery
    lg              *slog.Logger
}
```

The processor holds a pointer to the tracking processor, allowing it to:
- Access `avgFreq` for frequency control during no-sample phase
- Access the MAD window (and tracking config) for relaxed outlier detection
- Modify the MAD window on successful recovery
- Share the bad-sample bookkeeping and limit checks (`noteBadSample`/`limitExceeded`)

The `missingSamples` counter (initialized to 1: the triggering sample was processed by tracking) determines whether MAD recovery is needed when samples return. The `recoveryOffsets` slice collects post-gap offsets (distinct from the config field `recoverySamples` which specifies the count).

### Controller Changes

The `changeMode` function needs special handling for tracking <-> gap mode transitions:

```go
func (c *Controller) changeMode(mode Mode) {
    // ... existing code ...

    switch mode {
    // ... existing cases ...

    case ModeGap:
        // Entered only from tracking. Keep the live generator and wrap the
        // processor so its state survives the gap.
        c.sampleProc = newGapSampleProcessor(c.cfg.Gap, c.sampleProc.(*trackingSampleProcessor), c.lg)

    case ModeTracking:
        if gsp, ok := c.sampleProc.(*gapSampleProcessor); ok {
            // Returning from a gap: restore the preserved tracking processor.
            // The generator is already the tracking generator.
            c.sampleProc = gsp.trackingProc
        } else {
            // Normal tracking entry (from converging)
            c.sampleGen = newTrackingSampleGenerator(...)
            c.sampleProc = newTrackingSampleProcessor(...)
        }
    }
}
```

Two further controller details follow from the preserve/restore pattern: the persist-sample extraction on mode exit must reach through the gap processor (so a gap -> reset transition does not lose the drift-validation sample), and `Tick`'s pending-pulse timeout must also run in gap mode, since the tracking generator stays live there.

### Triggering Gap mode

The tracking processor triggers gap mode on any missing sample:

```go
func (p *trackingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
    if sample.Kind == SampleMissing {
        // Any missing sample -> gap mode. Record it in the shared bookkeeping
        // so the bad-sample limits carry across the transition, and switch to
        // the averaged frequency immediately (first bad sample of a run).
        action := p.noteBadSample()
        if p.limitExceeded() {
            return action, ModeReset
        }
        return action, ModeGap
    }

    // ... rest of existing logic (only handles present samples) ...
}
```

This simplifies tracking mode: it only processes present samples. The bad-sample bookkeeping and limit checks are factored into `noteBadSample`/`limitExceeded` so gap mode shares them; `noteBadSample` switches the PHC to `avgFreq` on the first bad sample of a run (preserving the existing semantics, including the `AvgFreqTimeConstant = 0` disable case), so the PHC runs at the averaged frequency from the triggering sample onwards.

### MAD Window Shift

Add `ShiftAll` method to `median.Window` (exposed through `madWindow` from tracking-limits.md):

```go
// ShiftAll adds delta to all values in the window.
// This adjusts the window's baseline while preserving relative positions.
func (w *Window[T]) ShiftAll(delta T) {
    for i := range w.values {
        w.values[i] += delta
    }
    // indices array remains valid (relative ordering unchanged)
}
```

Note: We shift all capacity entries rather than trying to identify active circular-buffer slots. This works because shifting all values by a constant preserves their relative order, so the sorted indices remain valid, and unused slots don't affect behavior.

### Relaxed MAD Detection

The gap mode processor uses a modified MAD check. This is additive to the tracking gates—samples must still pass the unconditional `OutlierThreshold` (and `MADThreshold` if configured). The relaxed check only adjusts the adaptive MAD-based detection.

Rather than duplicating the MAD computation on the gap processor, the tracking processor's `madIsOutlier` is generalized with a slack parameter; the normal check delegates with slack 0 and gap recovery calls it with `driftLimit`:

```go
func (p *trackingSampleProcessor) madIsOutlier(offset time.Duration) bool {
    return p.madIsOutlierSlack(offset, 0)
}

// madIsOutlierSlack is madIsOutlier with the threshold widened by slack.
// Gap mode uses a non-zero slack to admit samples showing legitimate drift
// accumulated while the signal was lost.
func (p *trackingSampleProcessor) madIsOutlierSlack(offset, slack time.Duration) bool {
    if p.madWindow.Len() < p.cfg.MADMinSamples {
        return false
    }
    center := p.madWindow.Median()
    mad := // ... median of absolute deviations from center, as before ...
    threshold := time.Duration(float64(mad)*p.cfg.MADMultiple) + slack
    return (offset - center).Abs() > threshold
}
```

## Implementation Steps

Implementation is divided into two steps to allow incremental testing.

### Step 1: Frequency Adjustment Only

Step 1 moves the existing `avgFreq` handling from tracking mode to gap mode. No new functionality—just reorganization into a distinct mode.

**Behavior:**
- Enter gap mode on any missing sample
- Apply `avgFreq` on each missing sample (same as current tracking behavior)
- Continue tracking's `consecutiveBadSamples` count (which may include outliers from before the missing sample); if `>= badSampleRunLimit`: exit to reset
- On first present sample: delegate to tracking processor and return `ModeTracking`
- No `holdoverThreshold` check (no transition to holdover yet)
- No MAD recovery logic (no relaxed detection, no window shift, no drift check)

**What changes:**
- Add `ModeGap` constant
- Add minimal `gapSampleProcessor` (no `recoveryOffsets`, no `recoveryThreshold` logic)
- Modify `trackingSampleProcessor`: trigger gap mode on missing sample, remove inline `avgFreq` logic
- Update `changeMode`: preserve/restore tracking processor across transitions
- Update PTP quality mapping: treat `ModeGap` as in-sync

**Test adjustments:**
- Existing tests may see `ModeGap` in mode sequences where they previously saw only tracking
- Tests with outages should expect: tracking -> gap mode -> tracking (not direct tracking recovery)
- No behavioral change to sync quality—just an additional mode in the sequence

**Files to modify (Step 1):**

| File | Changes |
|------|---------|
| `time/internal/phcsync/controller.go` | Add `ModeGap`, update `changeMode` |
| `time/internal/phcsync/tracking.go` | Trigger gap mode on missing sample, factor shared bad-sample bookkeeping |
| `time/internal/phcsync/gap.go` | New file: minimal `gapSampleProcessor` |
| `time/internal/syncsim/sync_test.go` | Adjust expected mode sequences |

### Step 2: MAD Window Adjustment

Step 2 adds the MAD recovery logic described in the Phases section above.

**New behavior:**
- Track the missing-sample count during gap mode
- On sample return with long gap (`>= recoveryThreshold`): relaxed MAD detection, collect samples, window shift
- If drift exceeds `driftLimit`: exit to reset

**What changes:**
- Add `GapConfig` with `recoveryThreshold`, `driftLimit`, `recoverySamples` (`holdoverThreshold` deferred to #199)
- Add `recoveryOffsets` slice to processor
- Add slack parameter to the tracking processor's MAD check (`madIsOutlierSlack`)
- Add `ShiftAll` method to `median.Window`
- Add recovery and window-shift logic
- Enforce `recoveryThreshold < badSampleRunLimit` in `Config.Validate`

**Files to modify (Step 2):**

| File | Changes |
|------|---------|
| `time/lib/median/median.go` | Add `ShiftAll` method |
| `time/internal/phcsync/gap.go` | Add config, recovery logic, window shift |
| `configs/config-schema.json` | Add new config section and parameters |
| `time/internal/syncsim/sync_test.go` | Add MAD recovery test scenarios |

## Test Plan

### Simulation Configuration

Use realistic parameters matching production hardware (same as mad-missing-samples.md):

```go
cfg := Config{
    PHC: PHCConfig{
        WhiteNoise: 2.5,
        RandomWalk: 2.9,
    },
    GPS: GPSConfig{
        Jitter:   0.2,
        Sawtooth: SawtoothConfig{Amp: 7.86},
        AR1:      []AR1Config{{Tau: 3400, Sigma: 3}},
    },
    Sync: phcsync.Config{
        Track: phcsync.TrackingConfig{
            Kp: 0.8,
            Ki: 0.3,
        },
    },
}
```

### Test Cases

#### Gap mode accepts legitimate post-gap samples

```go
{
    name:                     "gap mode accepts post-gap drift",
    duration:                 120.0,
    maxTrackingStdDev:        15,
    maxTrackingAbsMax:        60 * time.Nanosecond,
    expectMinTrackingSamples: 90,
    modifyConfig: func(cfg *Config) {
        // 4s outage triggers gap mode, exits back to tracking
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 4.0}}
    },
    // Verify: enters gap mode, recovers, returns to tracking
}
```

#### Gap mode rejects real outliers during recovery

```go
{
    name:                     "gap mode still rejects outliers",
    duration:                 120.0,
    maxTrackingStdDev:        15,
    maxTrackingAbsMax:        60 * time.Nanosecond,
    expectMinTrackingSamples: 90,
    modifyConfig: func(cfg *Config) {
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 4.0}}
        cfg.Fault.Outlier = []OutlierConfig{{Time: 65, Offset: 2000}}
    },
    // Verify: 2µs outlier still rejected despite relaxed threshold
}
```

#### Short gap skips MAD recovery

```go
{
    name:                     "short gap skips MAD recovery",
    duration:                 120.0,
    maxTrackingStdDev:        15,
    expectMinTrackingSamples: 95,
    modifyConfig: func(cfg *Config) {
        // 2s outage < recoveryThreshold (3) missing samples: enters gap mode
        // but returns to tracking without MAD window adjustment
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 2.0}}
    },
}
```

The short-gap and recovery paths are distinguished externally by the exact gap-mode sample count (`expectGapSamples`): a short gap attributes only the further missing sample and the gap-ending present sample to gap mode, while recovery adds the collected samples.

#### Gap mode transitions to holdover on long gap

Deferred to #199 with the holdover mode itself.

#### Excessive drift triggers reset

```go
{
    name:               "excessive drift during gap mode triggers reset",
    duration:           120.0,
    expectResetEntered: ptr(2), // initial + drift check failure after recovery
    modifyConfig: func(cfg *Config) {
        cfg.Sync.Track.BadSampleRunLimit = 15 // allow the 8s outage without a bad-sample reset
        // Soft servo gains so the step is not corrected away while recovery
        // samples are collected
        cfg.Sync.Track.Kp = 0.1
        cfg.Sync.Track.Ki = 0.02
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 8.0}}
        // 260ns step from the end of the outage, emulated by an excursion
        // lasting past the end of the simulation
        cfg.Fault.Excursion = []ExcursionConfig{{
            StartTime: 67.5,
            Duration:  60.0,
            Amplitude: 260,
            Rise:      RampConfig{Duration: 0.01},
            Fall:      RampConfig{Duration: 0.01},
        }}
    },
    // Window shift exceeds driftLimit -> triggers reset
}
```

Two empirical notes on this case. A step at the unconditional `OutlierThreshold` (500ns) never reaches the drift check: every sample is rejected outright and the bad-sample run limit forces the reset instead, so the step must sit inside the relaxed-MAD acceptance band. And at default servo gains the servo corrects ~100ns/s while recovery samples are collected, attenuating the offsets so their median rarely exceeds `driftLimit`; the soft gains keep the step visible across the collection window. The drift check is thus mainly a backstop at default gains.

## Relationship to Holdover

Gap mode is the first stage of signal loss handling. The hierarchy is:

1. **Normal tracking**: Every sample processed, no special handling
2. **Gap mode**: Brief gap, use avgFreq, relaxed recovery, no PTP quality change
3. **Holdover**: Extended gap, frequency model, reconvergence, PTP quality degraded

The holdover plan will build on gap mode:
- Entry: From gap mode when `consecutiveMissingSamples >= holdoverThreshold`
- Some frequency model code may be shared
- Recovery phases have similar structure (drift check, reconvergence)

## Design Rationale: Why Any Missing Sample Triggers Gap Mode

An alternative design would only enter gap mode after N consecutive missing samples. We chose to trigger on **any** missing sample because:

1. **Easier to understand**: The purpose of gap mode becomes obvious - you don't want to trigger full holdover for just one missing sample. Gap mode is the answer.

2. **Conceptual simplicity**: Any missing sample means we've lost the reference signal and are now holding over - just for a very short time.

3. **Uniform frequency handling**: Gap mode owns `avgFreq` application, which is more uniform with how holdover works (holdover also applies its own frequency model).

4. **Cleaner tracking mode**: Tracking only handles present samples. Missing sample = leave tracking.

5. **Efficiency is not a concern**: Missing samples are rare (perhaps once a week). Mode transition overhead is negligible.

The `recoveryThreshold` parameter still exists but serves a different purpose: it controls whether MAD window adjustment is needed, not whether to enter gap mode.

## Future Considerations

When implementing holdover.md:

1. **Shared config structure**: Both modes have `driftLimit` and `recoverySamples`. Explore using a shared embedded struct for common recovery parameters:

   ```go
   type RecoveryConfig struct {
       DriftLimit      int64 `toml:"driftLimit"`
       RecoverySamples int   `toml:"recoverySamples"`
   }

   type GapConfig struct {
       RecoveryConfig
       RecoveryThreshold int `toml:"recoveryThreshold"`
       HoldoverThreshold int `toml:"holdoverThreshold"`
   }

   type HoldoverConfig struct {
       RecoveryConfig
       Duration float64 `toml:"duration"`
       // ... other holdover-specific params
   }
   ```

2. **Tracking processor reference**: Full holdover should also hold a reference to the `trackingSampleProcessor` (passed through from `gapSampleProcessor`). When samples return after holdover, use the same MAD-based relaxed outlier detection approach as gap mode. This provides uniform recovery behavior across both modes and allows holdover to shift the MAD window on successful recovery.