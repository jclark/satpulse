# Holdover

## Introduction

Holdover allows the system to maintain accurate time when the GNSS receiver loses its lock. This requires a local oscillator separate from the GNSS module's oscillator.

The implementation depends on where the holdover oscillator lives:

**Internal holdover (PHC-based)**: The oscillator is part of the PHC hardware. Examples include the [TimeHAT](https://www.tindie.com/products/timeappliances/timehat-i226-nic-with-pps-inout-for-rpi5/) and [TimeNIC](https://www.tindie.com/products/timeappliances/timenic-i226-pcie-nic-with-pps-inout-and-tcxo/), which have high-quality TCXOs. During tracking, we build a frequency model. During holdover, we apply that model to maintain the PHC frequency.

**External holdover (GPSDO-based)**: A GNSS Disciplined Oscillator (e.g., BG7TBL CM55) disciplines the oscillator upstream of SatPulse. Pulses continue during holdover but are no longer GNSS-locked. We detect loss/restoration of lock via time messages rather than missing pulses.

A third approach using an independent oscillator (e.g., Leo Bodnar LBE-1421) connected to a second SDP is possible but out of scope for this plan.

---

## Stage 1: PHC-based holdover

### State machine

- tracking -> microHoldover: any missing sample (see micro-holdover.md)
- microHoldover -> holdover: `consecutiveMissingSamples >= holdoverThreshold`
- holdover -> tracking: reconvergence criteria met
- holdover -> reset: holdover timer expired OR median shift from pre-holdover baseline exceeds drift limit

Holdover is never entered directly from tracking. Missing samples first enter micro-holdover, which handles brief gaps without PTP quality degradation. Only persistent missing samples (reaching `holdoverThreshold`) trigger transition to full holdover.

### Holdover timer

The holdover timer measures elapsed time since the last good tracking sample:

- Computed as current time minus timestamp of last good sample
- Includes time spent in micro-holdover before entering holdover (since micro-holdover also lacks good samples)
- If elapsed time exceeds `duration`, transition to reset
- Timer is implicitly cleared when a good sample arrives

**Constraint**: `duration > holdoverThreshold` (since samples are 1Hz, holdoverThreshold samples ≈ holdoverThreshold seconds; we need the timer to allow at least that long before expiring)

### PTP clock quality

| Mode | Clock class |
|------|-------------|
| tracking | Configured in-sync quality |
| microHoldover | Configured in-sync quality (same as tracking) |
| holdover | ClockClassHoldover (7) |
| reset/converging | ClockClassDegradedA (degraded) |

**Note**: Micro-holdover maintains tracking-level PTP quality because gaps are brief enough that drift remains within tracking accuracy. Only holdover degrades clock quality. Any mode→quality mapping must treat `ModeMicroHoldover` identically to `ModeTracking`.

### Frequency model

During tracking, we maintain two frequency estimates:

- **Short-term EMA**: Existing `avgFreq` with `avgFreqTimeConstant` (default 30s)
- **Long-term EMA**: New `longAvgFreq` with `longAvgFreqTimeConstant` (default 300s)

Both are updated on each good sample.

**State handoff**: Holdover receives these frequency estimates (and the preserved tracking processor reference) from micro-holdover. Micro-holdover uses only `avgFreq`; holdover blends both estimates for longer free-running periods.

### Holdover processor behaviour

The holdover processor handles three phases:

**Phase 1: No sample (pulses missing)**
- Blend short-term and long-term frequency estimates based on time in holdover
- Apply blended frequency to PHC

**Phase 2: Recovery (first samples returning)**

Recovery uses the same relaxed-MAD + window-shift approach as micro-holdover for consistency:

- Record the MAD window's median at holdover entry as the pre-holdover baseline
- Use relaxed MAD detection: add `driftLimit` to normal MAD threshold
- If sample passes relaxed check: accept, feed to servo, collect for recovery
- If sample fails relaxed check: reject as outlier, increment bad sample counter; if too many, exit to reset
- After `recoverySamples` accepted samples: compute median of collected offsets
- Compute shift = (new median) - (pre-holdover baseline median)
- If shift exceeds `driftLimit`, exit to reset
- Otherwise: shift the MAD window, add recovery samples, proceed to phase 3

This mirrors micro-holdover's window-shift logic (see micro-holdover.md Phase 3), ensuring uniform recovery behavior regardless of gap length.

**Phase 3: Reconvergence**
- Continue running PI servo
- Track convergence (like converging mode)
- Exit to tracking when converged

### Sample generation

Holdover uses a simple sample generator similar to `convergingSampleGenerator`. When pulses arrive, it produces samples.

However, the holdover processor needs access to the preserved tracking processor (passed through from micro-holdover) for:
- Reading `avgFreq` and `longAvgFreq` for frequency blending
- Accessing the MAD window for relaxed outlier detection during recovery
- Shifting the MAD window on successful recovery

### Relationship to micro-holdover

There are two levels of missing-sample handling:

1. **Micro-holdover** (micro-holdover.md): Brief gaps, uses `avgFreq`, relaxed MAD recovery, PTP quality unchanged
2. **Full holdover**: Extended gaps (≥`holdoverThreshold`), blends frequency estimates, reconvergence phase, PTP quality degraded

Both modes share recovery behavior (relaxed-MAD detection, window shift) for uniform handling of post-gap samples. The key differences are:
- Micro-holdover uses only short-term `avgFreq`; holdover blends short-term and long-term estimates
- Micro-holdover returns directly to tracking after window shift; holdover requires reconvergence
- Micro-holdover maintains in-sync PTP quality; holdover advertises holdover quality

### Triggering holdover

Holdover is triggered from micro-holdover, not directly from tracking:

1. **tracking -> microHoldover**: Any missing sample leaves tracking immediately
2. **microHoldover -> holdover**: When `consecutiveMissingSamples >= holdoverThreshold`

The micro-holdover processor increments `consecutiveMissingSamples` on each missing sample and checks against `holdoverThreshold` (configured in `[phcsync.microHoldover]`).

**What about outliers?** With the micro-holdover design, the "reference present but bad" case is handled differently:

- Missing sample → enter micro-holdover immediately
- If samples return during micro-holdover → relaxed-MAD recovery attempts to accept them
- If recovery fails (bad-run-limit exceeded, or drift-limit exceeded) → exit to reset
- Only persistent missing samples (no samples returning) → transition to holdover

This simplifies the decision logic: holdover is purely about "reference disappeared for too long", not a choice between holdover-vs-reset at tracking exit.

### Configuration

New config section `[ptp.holdover]` for PTP clock quality during holdover:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `clockAccuracy` | string | TBD | Clock accuracy during holdover |
| `offsetScaledLogVariance` | int | TBD | OSLV during holdover |

New config section `[phcsync.holdover]`:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `duration` | float | 60 | Maximum seconds in holdover before reset |
| `driftLimit` | int64 | TBD | Max allowed median shift (ns) from pre-holdover baseline; if exceeded, exit to reset |
| `recoverySamples` | int | 5 | Samples to collect before checking drift |
| `kp` | float | TBD | PI servo proportional gain |
| `ki` | float | TBD | PI servo integral gain |
| `medianWindow` | int | 5 | Samples for convergence median |
| `offsetLimit` | int64 | 1000 | Max offset to declare converged (ns) |
| `stableWindow` | int | 5 | Stable samples before exit to tracking |

**Note**: `holdoverThreshold` is configured in `[phcsync.microHoldover]`, not here, since micro-holdover owns the transition decision.

**Constraint** (from micro-holdover.md): `recoveryThreshold < holdoverThreshold < badSampleRunLimit`

This ensures:
1. Short gaps trigger MAD recovery (not holdover)
2. Holdover is reachable before bad-sample-run-limit forces reset

**Parameter semantics:**

`duration`: How long we trust the frequency model. After this time without signal recovery, the accumulated drift is too uncertain, so we give up and reset (step the clock).

`driftLimit`: Maximum allowed shift between pre-holdover baseline median and post-holdover recovery median. Uses the same semantics as micro-holdover's `driftLimit` for consistency. Should be slightly less than `ptp.holdover.clockAccuracy` since there's no point reconverging if we've exceeded our advertised accuracy.

`recoverySamples`: How many samples to collect before checking drift. Using median of several samples is more robust than trusting a single sample (which could be an outlier).

`kp`, `ki`: PI servo gains during reconvergence. May need different tuning than converging mode since we want to correct the drift without causing large frequency swings that would disturb PTP clients.

`medianWindow`, `offsetLimit`, `stableWindow`: Convergence criteria - same semantics as converging mode. We track the median of recent offsets and exit to tracking when the median stabilizes below `offsetLimit` for `stableWindow` consecutive samples.

### Implementation components

1. Add `SyncState` value `Holdover` in `ptpgm` package
2. Add `ModeHoldover` in `phcsync`
3. Add `longAvgFreq` to `trackingSampleProcessor`
4. New `holdoverSampleGenerator`: simple, like converging
5. New `holdoverSampleProcessor`: handles no-sample, recovery, and reconvergence phases
6. Modify `microHoldoverSampleProcessor`: transition to holdover when `consecutiveMissingSamples >= holdoverThreshold`
7. Controller `changeMode`: pass tracking processor reference from micro-holdover to holdover
8. Add holdover timer: track time since last good sample, enforce `duration`

---

## Stage 2: GPSDO-based holdover

This section describes modifications needed to support external holdover with a GPSDO.

### Key difference: detecting lock loss

With a GPSDO, PPS pulses continue even when GNSS lock is lost. We must detect loss of lock from time messages, not from missing pulses.

A `TimeMsg` indicates loss of lock when:
- `UTCTime == nil` AND `TAITime == 0`

### Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `gps.disciplined` | bool | false | True if using external GPSDO |

### TimeMsgBuffer lock query API

Add method to `TimeMsgBuffer`:

```go
// IsLocked returns true if the most recent navigation-solution TimeMsg
// indicates a valid time and is not stale.
func (b *TimeMsgBuffer) IsLocked() bool
```

### Accommodating multiple holdover types

The `gps.disciplined` setting selects different implementations:

**Sample generation:**
- PHC-based (`disciplined=false`): Missing sample = no PPS edge received
- GPSDO-based (`disciplined=true`): Missing sample = `!timeMsgBuf.IsLocked()`

**Holdover servo:**
- PHC-based: Apply blended frequency model (free-running)
- GPSDO-based: Continue tracking the disciplined PPS

### Implementation components

1. Add `IsLocked()` to `TimeMsgBuffer`
2. New `gpsdoHoldoverSampleGenerator`: checks lock status instead of pulse presence
3. New `gpsdoHoldoverSampleProcessor`: continues tracking with adjusted Kp/Ki
4. Controller selects implementations based on `gps.disciplined`