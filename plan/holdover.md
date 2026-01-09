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

- tracking -> holdover: too many consecutive missing samples
- holdover -> tracking: reconvergence criteria met
- holdover -> reset: holdover timer expired OR median offset of initial samples exceeds drift limit

Holdover is triggered by missing samples (lost reference), not by outliers (bad reference).

### Holdover timer

The holdover timer measures elapsed time since the last good tracking sample:

- Timer starts when entering holdover from tracking
- If timer expires, transition to reset
- Timer is cleared when transitioning back to tracking

### PTP clock quality

| Mode | Clock class |
|------|-------------|
| tracking | Configured in-sync quality |
| holdover | ClockClassHoldover (7) |
| reset/converging | ClockClassDegradedA (degraded) |

### Frequency model

During tracking, we maintain two frequency estimates:

- **Short-term EMA**: Existing `avgFreq` with `avgFreqTimeConstant` (default 30s)
- **Long-term EMA**: New `longAvgFreq` with `longAvgFreqTimeConstant` (default 300s)

Both are updated on each good sample.

### Holdover processor behaviour

The holdover processor handles three phases:

**Phase 1: No sample (pulses missing)**
- Blend short-term and long-term frequency estimates based on time in holdover
- Apply blended frequency to PHC

**Phase 2: Drift check (first samples returning)**
- Collect `recoverySamples` samples (similar to gap recovery in mad-missing-samples.md)
- Feed samples to PI servo while collecting
- After collecting enough samples, compute median offset
- If median offset exceeds `driftLimit`, exit to reset
- Otherwise proceed to phase 3

**Phase 3: Reconvergence**
- Continue running PI servo
- Track convergence (like converging mode)
- Exit to tracking when converged

### Sample generation

Holdover uses a simple sample generator similar to `convergingSampleGenerator`. When pulses arrive, it produces samples. No complex tracking state is needed.

### Relationship to gap recovery in tracking mode

There are two levels of missing-sample handling:

1. **Gap recovery in tracking** (mad-missing-samples.md): Short gaps handled within tracking mode, PTP quality unchanged
2. **Full holdover mode**: Longer gaps, transition to holdover, advertise holdover PTP quality

Parameters follow consistent naming: `gap*` in tracking section, unprefixed in holdover section.

| Tracking | Holdover |
|----------|----------|
| `gapDriftLimit` | `driftLimit` |
| `gapRecoverySamples` | `recoverySamples` |

### Triggering holdover

Modify tracking mode to distinguish between outliers and missing samples:

- **Outliers**: Increment `consecutiveBadSamples`, trigger reset when limit reached
- **Missing samples**: Increment `consecutiveMissingSamples`, trigger holdover when limit reached

New config parameter in `[phcsync.tracking]`:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `missingSampleLimit` | int | 10 | Consecutive missing samples before holdover |

Note: `missingSampleLimit` must be >= `gapMinSamples` (default 5) so gap recovery runs before holdover.

TODO: this is not OK; need to design plan that works well with tracking-limits.md.

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
| `driftLimit` | int64 | TBD | Max median offset (ns) to attempt reconvergence |
| `recoverySamples` | int | 5 | Samples to collect before checking drift |
| `kp` | float | TBD | PI servo proportional gain |
| `ki` | float | TBD | PI servo integral gain |
| `medianWindow` | int | 5 | Samples for convergence median |
| `offsetLimit` | int64 | 1000 | Max offset to declare converged (ns) |
| `stableWindow` | int | 5 | Stable samples before exit to tracking |

**Parameter semantics:**

`duration`: How long we trust the frequency model. After this time without signal recovery, the accumulated drift is too uncertain, so we give up and reset (step the clock).

`driftLimit`: When signal returns, we check how far we've drifted. If median offset of initial samples exceeds this, we've drifted too far to reconverge gracefully - reset instead. Should be slightly less than `ptp.holdover.clockAccuracy` since there's no point reconverging if we've exceeded our advertised accuracy.

`recoverySamples`: How many samples to collect before checking drift. Using median of several samples is more robust than trusting a single sample (which could be an outlier).

`kp`, `ki`: PI servo gains during reconvergence. May need different tuning than converging mode since we want to correct the drift without causing large frequency swings that would disturb PTP clients.

`medianWindow`, `offsetLimit`, `stableWindow`: Convergence criteria - same semantics as converging mode. We track the median of recent offsets and exit to tracking when the median stabilizes below `offsetLimit` for `stableWindow` consecutive samples.

### Implementation components

1. Add `SyncState` value `Holdover` in `ptpgm` package
2. Add `ModeHoldover` in `phcsync`
3. Add `longAvgFreq` to `trackingSampleProcessor`
4. New `holdoverSampleGenerator`: simple, like converging
5. New `holdoverSampleProcessor`: handles both no-sample and sample cases
6. Modify `trackingSampleProcessor`: separate missing vs outlier counting, transition to holdover
7. Add holdover timer: track time since last good sample, enforce `maxDuration`

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