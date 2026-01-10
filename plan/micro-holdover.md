# Micro-Holdover Mode

Fixes: #188

Assumes: tracking-limits.md is implemented

## Introduction

Micro-holdover is a brief holdover period short enough that PTP clock quality does not need to change. It bridges the gap between normal tracking (where every sample is processed) and full holdover (where PTP quality is degraded).

Micro-holdover consolidates two pieces of missing-sample handling:

1. **Frequency adjustment**: When samples go missing, the servo switches to an averaged frequency estimate (`avgFreq`). This is currently implemented inline in tracking mode.

2. **MAD recovery**: After a gap, post-gap samples may be incorrectly rejected as outliers. This requires relaxed MAD detection and window shifting, as described in `mad-missing-samples.md`.

By handling both in a dedicated mode, tracking becomes simpler (it only handles present samples) and the design is more uniform with full holdover mode.

## Why is Micro-Holdover needed?

When GPS signal is briefly lost during tracking:

1. The servo switches to an averaged frequency estimate
2. The PHC drifts slightly (typically under 100ns for brief outages)
3. When signal returns, the first samples show offset reflecting this drift
4. The MAD detector, still comparing against pre-gap statistics, may incorrectly reject these legitimate samples as outliers

The post-gap samples are **not** outliers—they correctly reflect that the PHC drifted. But the MAD detector doesn't know about the gap; it just sees a sudden jump.

## Design Goals

1. **Uniform with holdover**: Micro-holdover and holdover should share conceptual structure (entry criteria, recovery phases, exit conditions)
2. **Clean separation**: Micro-holdover is a distinct mode, not inline logic cluttering the tracking processor
3. **Preserve state**: Unlike mode transitions that create fresh processors, micro-holdover preserves the tracking processor's state (MAD window, servo, avgFreq) for use during recovery and restoration
4. **Same effect**: Achieve identical behavior to `mad-missing-samples.md` gap recovery

## Comparison: Micro-Holdover vs Holdover

| Aspect | Micro-Holdover | Holdover |
|--------|---------------|----------|
| Purpose | Brief signal loss, maintain tracking quality | Extended signal loss, degrade gracefully |
| PTP clock class | Unchanged (tracking quality) | ClockClassHoldover (7) |
| Entry trigger | Any missing sample | `consecutiveMissingSamples >= holdoverThreshold` |
| Frequency control | Use tracking's `avgFreq` | Blend short-term and long-term frequency estimates |
| Recovery | Relaxed MAD detection, shift window (if gap was long enough) | Drift check, reconvergence phase |
| Exit to tracking | Directly (short gap) or after window shift (long gap) | After reconvergence criteria met |

## State Machine

```
tracking ──[any missing sample]──> microHoldover
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

- **tracking -> microHoldover**: Any missing sample
- **microHoldover -> holdover**: `consecutiveMissingSamples >= holdoverThreshold`
- **microHoldover -> tracking**: Successful recovery
- **microHoldover -> reset**: Recovery drift exceeds `driftLimit`

## Configuration

**Principle**: Parameters belong to the config section of the mode that uses them, not the mode they trigger transition to.

New config section `[phcsync.microHoldover]`:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `recoveryThreshold` | int | 5 | Consecutive missing samples before MAD recovery is needed |
| `holdoverThreshold` | int | 10 | Consecutive missing samples to transition to full holdover |
| `driftLimit` | int64 | 100 | Max allowed median shift (ns) from pre-gap baseline; if exceeded, exit to reset |
| `recoverySamples` | int | 5 | Samples to collect before shifting window |

**Parameter semantics:**

- `recoveryThreshold`: Short gaps (fewer missing samples) don't cause enough drift to need MAD window adjustment. Longer gaps do. This threshold controls when the full recovery procedure (relaxed MAD detection, collect samples, shift window) is triggered.

**Shared parameters from tracking**: Micro-holdover is in some ways a sub-mode of tracking and uses several tracking parameters directly:
- `badSampleRunLimit`: exits to reset if too many consecutive bad samples during recovery
- MAD parameters (`MADMinSamples`, `MADMultiple`, etc.): used for relaxed outlier detection

**Constraints:**

- `recoveryThreshold < holdoverThreshold < badSampleRunLimit`

This ensures:
1. MAD recovery triggers before holdover consideration
2. Holdover is possible before the bad sample limit forces an exit

## Phases

### Phase 1: No Samples

While samples are missing:

- Apply `avgFreq` from tracking processor (moved from tracking mode)
- Increment `consecutiveMissingSamples`
- If `consecutiveMissingSamples >= holdoverThreshold`: transition to holdover

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
preMedian := trackingProc.madTracker.Median()
newMedian := computeMedian(p.recoveryOffsets)  // median of collected offsets
shift := newMedian - preMedian

if shift.Abs() > driftLimit {
    return ModeReset
}

trackingProc.madTracker.ShiftAll(shift)
for _, offset := range p.recoveryOffsets {
    trackingProc.madTracker.Add(offset, false)  // not outliers
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
    ModeMicroHoldover
    // ModeHoldover will be added later
    NModes
)
```

**PTP quality**: `ModeMicroHoldover` must be treated as in-sync for PTP clock quality reporting. Any code that maps modes to PTP clock class or state should treat `ModeMicroHoldover` identically to `ModeTracking` (i.e., no degradation of clock quality).

### Sample Generator

Micro-holdover reuses the tracking sample generator. The controller preserves it during the tracking -> microHoldover transition.

### Sample Processor

New `microHoldoverSampleProcessor`:

```go
type microHoldoverSampleProcessor struct {
    cfg                      MicroHoldoverConfig
    trackingCfg              TrackingConfig
    trackingProc             *trackingSampleProcessor  // preserved from tracking mode
    consecutiveMissingSamples int                      // count of missing samples in this micro-holdover
    recoveryOffsets          []time.Duration          // collected post-gap offsets (long gaps only)
    lg                       *slog.Logger
}
```

The processor holds a pointer to the tracking processor, allowing it to:
- Access `avgFreq` for frequency control during no-sample phase
- Access the MAD window for relaxed outlier detection
- Modify the MAD window on successful recovery

The `consecutiveMissingSamples` counter determines whether MAD recovery is needed when samples return. The `recoveryOffsets` slice collects post-gap offsets (distinct from the config field `recoverySamples` which specifies the count).

### Controller Changes

The `changeMode` function needs special handling for tracking <-> microHoldover transitions:

```go
func (c *Controller) changeMode(mode Mode) {
    // ... existing code ...

    switch mode {
    // ... existing cases ...

    case ModeMicroHoldover:
        // Preserve tracking generator and processor
        trackingGen := c.sampleGen.(*trackingSampleGenerator)
        trackingProc := c.sampleProc.(*trackingSampleProcessor)
        c.sampleGen = trackingGen  // reuse
        c.sampleProc = newMicroHoldoverSampleProcessor(
            c.cfg.MicroHoldover,
            c.cfg.Track,
            trackingProc,
            c.lg,
        )

    case ModeTracking:
        if c.mode == ModeMicroHoldover {
            // Restore tracking processor from micro-holdover
            mhProc := c.sampleProc.(*microHoldoverSampleProcessor)
            c.sampleProc = mhProc.trackingProc
            // Generator already correct (was preserved)
        } else {
            // Normal tracking entry (from converging)
            c.sampleGen = newTrackingSampleGenerator(...)
            c.sampleProc = newTrackingSampleProcessor(...)
        }
    }
}
```

### Triggering Micro-Holdover

The tracking processor triggers micro-holdover on any missing sample:

```go
func (p *trackingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
    // ... existing outlier detection ...

    if sample.Kind == SampleMissing {
        // Any missing sample -> micro-holdover
        // Return avgFreq action immediately so PHC uses holdover frequency
        // starting from this sample (not the next one)
        return phcAction{actionType: phcAdjustFrequency, freq: p.avgFreq}, ModeMicroHoldover
    }

    // ... rest of existing logic (only handles present samples) ...
}
```

This simplifies tracking mode: it only processes present samples. The `avgFreq` action on the triggering sample ensures the PHC switches to holdover frequency immediately, maintaining parity with current behavior. Subsequent missing samples in micro-holdover continue using `avgFreq`.

### MAD Window Shift

Add `ShiftAll` method to `median.Window` (exposed through `madTracker` from tracking-limits.md):

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

The micro-holdover processor uses a modified MAD check. This is additive to the tracking gates—samples must still pass the unconditional `OutlierThreshold` (and `MADThreshold` if configured). The relaxed check only adjusts the adaptive MAD-based detection:

```go
func (p *microHoldoverSampleProcessor) relaxedMADIsOutlier(offset time.Duration) bool {
    // Unconditional gates still apply (checked before this function):
    // - OutlierThreshold: absolute hard limit
    // - MADThreshold: optional secondary gate
    // This function only relaxes the adaptive MAD-based detection.

    tw := p.trackingProc.madTracker
    if tw.Len() < p.trackingCfg.MADMinSamples {
        return false
    }

    center := tw.Median()
    mad := median.Median(func(yield func(time.Duration) bool) {
        tw.Iterate(func(i int, v time.Duration) bool {
            return yield((v - center).Abs())
        })
    })

    // Relaxed threshold: normal + driftLimit
    threshold := time.Duration(float64(mad) * p.trackingCfg.MADMultiple)
    threshold += time.Duration(p.cfg.DriftLimit)

    deviation := (offset - center).Abs()
    return deviation > threshold
}
```

## Files to Modify

| File | Changes |
|------|---------|
| `internal/median/median.go` | Add `ShiftAll` method |
| `internal/phcsync/controller.go` | Add `ModeMicroHoldover`, update `changeMode` |
| `internal/phcsync/tracking.go` | Trigger micro-holdover on missing sample, remove `avgFreq` logic |
| `internal/phcsync/microholdover.go` | New file: `MicroHoldoverConfig`, `microHoldoverSampleProcessor` |
| `configs/config-schema.json` | Add new config section and parameters |
| `internal/syncsim/sync_test.go` | Test scenarios |

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

#### Micro-holdover accepts legitimate post-gap samples

```go
{
    name:                     "micro-holdover accepts post-gap drift",
    duration:                 120.0,
    maxTrackingStdDev:        15,
    maxTrackingAbsMax:        60 * time.Nanosecond,
    expectMinTrackingSamples: 90,
    modifyConfig: func(cfg *Config) {
        // 4s outage triggers micro-holdover, exits back to tracking
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 4.0}}
    },
    // Verify: enters micro-holdover, recovers, returns to tracking
}
```

#### Micro-holdover rejects real outliers during recovery

```go
{
    name:                     "micro-holdover still rejects outliers",
    duration:                 120.0,
    maxTrackingStdDev:        15,
    maxTrackingAbsMax:        60 * time.Nanosecond,
    expectMinTrackingSamples: 90,
    modifyConfig: func(cfg *Config) {
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 4.0}}
        cfg.Fault.Outlier = []OutlierConfig{{Time: 64.5, Offset: 2000}}
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
    expectMinTrackingSamples: 100,
    modifyConfig: func(cfg *Config) {
        // 3s outage < recoveryThreshold (5): enters micro-holdover
        // but returns to tracking without MAD window adjustment
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 3.0}}
    },
}
```

#### Micro-holdover transitions to holdover on long gap

```go
{
    name:               "long gap transitions micro-holdover to holdover",
    duration:           180.0,
    expectModeSequence: []Mode{ModeReset, ModeConverging, ModeTracking,
                               ModeMicroHoldover, ModeHoldover},
    modifyConfig: func(cfg *Config) {
        // Outage exceeds holdoverThreshold while in micro-holdover
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 15.0}}
    },
}
```

#### Excessive drift triggers reset

```go
{
    name:               "excessive drift during micro-holdover triggers reset",
    duration:           180.0,
    expectResetSamples: 2,
    modifyConfig: func(cfg *Config) {
        // Long enough to cause > 100ns drift, but inject samples
        // that show excessive offset
        cfg.Fault.Outage = []OutageConfig{{StartTime: 60.0, Duration: 8.0}}
        cfg.Fault.PhaseStep = []PhaseStepConfig{{Time: 68.0, Offset: 500}}
    },
    // Window shift exceeds driftLimit -> triggers reset
}
```

## Relationship to Holdover

Micro-holdover is the first stage of signal loss handling. The hierarchy is:

1. **Normal tracking**: Every sample processed, no special handling
2. **Micro-holdover**: Brief gap, use avgFreq, relaxed recovery, no PTP quality change
3. **Holdover**: Extended gap, frequency model, reconvergence, PTP quality degraded

The holdover plan will build on micro-holdover:
- Entry: From micro-holdover when `consecutiveMissingSamples >= holdoverThreshold`
- Some frequency model code may be shared
- Recovery phases have similar structure (drift check, reconvergence)

## Design Rationale: Why Any Missing Sample Triggers Micro-Holdover

An alternative design would only enter micro-holdover after N consecutive missing samples. We chose to trigger on **any** missing sample because:

1. **Easier to understand**: The purpose of micro-holdover becomes obvious - you don't want to trigger full holdover for just one missing sample. Micro-holdover is the answer.

2. **Conceptual simplicity**: Any missing sample means we've lost the reference signal and are now holding over - just for a very short time.

3. **Uniform frequency handling**: Micro-holdover owns `avgFreq` application, which is more uniform with how holdover works (holdover also applies its own frequency model).

4. **Cleaner tracking mode**: Tracking only handles present samples. Missing sample = leave tracking.

5. **Efficiency is not a concern**: Missing samples are rare (perhaps once a week). Mode transition overhead is negligible.

The `recoveryThreshold` parameter still exists but serves a different purpose: it controls whether MAD window adjustment is needed, not whether to enter micro-holdover.

## Future Considerations

When implementing holdover.md:

1. **Shared config structure**: Both modes have `driftLimit` and `recoverySamples`. Explore using a shared embedded struct for common recovery parameters:

   ```go
   type RecoveryConfig struct {
       DriftLimit      int64 `toml:"driftLimit"`
       RecoverySamples int   `toml:"recoverySamples"`
   }

   type MicroHoldoverConfig struct {
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

2. **Tracking processor reference**: Full holdover should also hold a reference to the `trackingSampleProcessor` (passed through from `microHoldoverSampleProcessor`). When samples return after holdover, use the same MAD-based relaxed outlier detection approach as micro-holdover. This provides uniform recovery behavior across both modes and allows holdover to shift the MAD window on successful recovery.