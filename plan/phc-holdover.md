# PHC-based holdover

## Introduction

This document describes holdover when using a PHC (PTP Hardware Clock) with a high-quality oscillator, such as the TCXO on the [TimeHAT](https://www.tindie.com/products/timeappliances/timehat-i226-nic-with-pps-inout-for-rpi5/) or [TimeNIC](https://www.tindie.com/products/timeappliances/timenic-i226-pcie-nic-with-pps-inout-and-tcxo/). When the GPS loses lock and the PPS signal stops, we enter holdover mode and advertise reduced PTP clock quality until the signal returns.

This can also work without any special hardware, using a normal PHC with an inexpensive crystal oscillator, but the holdover period will be much shorter.

Other holdover approaches are possible but covered separately:
- **GPSDO-based**: An external GNSS Disciplined Oscillator continues providing pulses during holdover (see gpsdo-holdover.md)
- **Independent oscillator**: A separate frequency reference (e.g., Leo Bodnar LBE-1421) connected to a second SDP

## Entering and exiting holdover

State transitions:

- tracking mode -> gap mode: any missing sample (see gap-mode.md)
- gap mode -> holdover mode: `consecutiveMissingSamples >= holdoverThreshold`
- holdover mode -> tracking mode: reconvergence criteria met
- holdover mode -> reset: holdover timer expired OR median shift from pre-holdover baseline exceeds drift limit

Holdover mode is never entered directly from tracking mode. Missing samples first enter gap mode, which handles brief gaps without PTP quality degradation. Only persistent missing samples (reaching `holdoverThreshold`) trigger transition to holdover mode.

There are two levels of missing-sample handling:

1. **Gap mode**: Brief gaps, uses `avgFreq`, relaxed MAD recovery, PTP quality unchanged
2. **Holdover mode**: Extended gaps (≥`holdoverThreshold`), blends frequency estimates, reconvergence phase, PTP quality degraded

PTP clock quality by mode:

| Mode | Clock class |
|------|-------------|
| tracking mode | Configured in-sync quality |
| gap mode | Configured in-sync quality (same as tracking mode) |
| holdover mode | ClockClassHoldover (7) |
| reset/converging | ClockClassDegradedA (degraded) |

Both modes share recovery behavior (relaxed-MAD detection, window shift) for uniform handling of post-gap samples.

## Holdover behavior

During tracking, we maintain two frequency estimates:

- **Short-term EMA**: Existing `avgFreq` with `avgFreqTimeConstant` (default 30s)
- **Long-term EMA**: New `longAvgFreq` with `longAvgFreqTimeConstant` (default 300s)

Both are updated on each good sample. Holdover mode receives these frequency estimates (and the preserved tracking processor reference) from gap mode, and blends both estimates for longer free-running periods.

The holdover processor handles three phases:

**Phase 1: No sample (pulses missing)**
- Blend short-term and long-term frequency estimates based on time in holdover
- Apply blended frequency to PHC

**Phase 2: Recovery (first samples returning)**

Recovery uses the same relaxed-MAD + window-shift approach as gap mode for consistency:

- Record the MAD window's median at holdover entry as the pre-holdover baseline
- Use relaxed MAD detection: add `driftLimit` to normal MAD threshold
- If sample passes relaxed check: accept, feed to servo, collect for recovery
- If sample fails relaxed check: reject as outlier, increment bad sample counter; if too many, exit to reset
- After `recoverySamples` accepted samples: compute median of collected offsets
- Compute shift = (new median) - (pre-holdover baseline median)
- If shift exceeds `driftLimit`, exit to reset
- Otherwise: shift the MAD window, add recovery samples, proceed to phase 3

This mirrors gap mode's window-shift logic (see gap-mode.md Phase 3), ensuring uniform recovery behavior regardless of gap length.

**Phase 3: Reconvergence**
- Continue running PI servo
- Track convergence (like converging mode)
- Exit to tracking mode when converged

The holdover timer measures elapsed time since the last good tracking sample. It is computed as current time minus timestamp of last good sample, and includes time spent in gap mode before entering holdover mode (since gap mode also lacks good samples). If elapsed time exceeds the configured maximum duration, transition to reset. The timer is implicitly cleared when a good sample arrives.

Holdover uses a simple sample generator similar to `convergingSampleGenerator`. When pulses arrive, it produces samples. The holdover processor needs access to the preserved tracking processor (passed through from gap mode) for reading `avgFreq` and `longAvgFreq` for frequency blending, accessing the MAD window for relaxed outlier detection during recovery, and shifting the MAD window on successful recovery.

## Configuration

Config section `[ptp.holdover]` for PTP clock quality during holdover:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `clockAccuracy` | string | TBD | Clock accuracy during holdover |
| `offsetScaledLogVariance` | int | TBD | OSLV during holdover |

Config section `[sync.holdover]`:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `duration` | float | 60 | Maximum seconds in holdover before reset |
| `driftLimit` | int64 | TBD | Max allowed median shift (ns) from pre-holdover baseline; if exceeded, exit to reset |
| `recoverySamples` | int | 5 | Samples to collect before checking drift |
| `kp` | float | TBD | PI servo proportional gain |
| `ki` | float | TBD | PI servo integral gain |
| `medianWindow` | int | 5 | Samples for convergence median |
| `offsetLimit` | int64 | 1000 | Max offset to declare converged (ns) |
| `stableWindow` | int | 5 | Stable samples before exit to tracking mode |

`holdoverThreshold` is configured in `[sync.gap]`, not here, since gap mode owns the transition decision.

**Constraint:** `sync.holdover.duration > sync.gap.holdoverThreshold` (since samples are 1Hz, holdoverThreshold samples ≈ holdoverThreshold seconds; we need the timer to allow at least that long before expiring)

**Parameter semantics:**

`duration`: How long we trust the frequency model. After this time without signal recovery, the accumulated drift is too uncertain, so we give up and reset (step the clock).

`driftLimit`: Maximum allowed shift between pre-holdover baseline median and post-holdover recovery median. Uses the same semantics as gap mode's `driftLimit` for consistency. Should be slightly less than `ptp.holdover.clockAccuracy` since there's no point reconverging if we've exceeded our advertised accuracy.

`recoverySamples`: How many samples to collect before checking drift. Using median of several samples is more robust than trusting a single sample (which could be an outlier).

`kp`, `ki`: PI servo gains during reconvergence. May need different tuning than converging mode since we want to correct the drift without causing large frequency swings that would disturb PTP clients.

`medianWindow`, `offsetLimit`, `stableWindow`: Convergence criteria - same semantics as converging mode. We track the median of recent offsets and exit to tracking mode when the median stabilizes below `offsetLimit` for `stableWindow` consecutive samples.

## Implementation steps

1. Add `SyncState` value `Holdover` in `ptpgm` package
2. Add `ModeHoldover` in `phcsync`
3. Add `longAvgFreq` to `trackingSampleProcessor`
4. New `holdoverSampleGenerator`: simple, like converging
5. New `holdoverSampleProcessor`: handles no-sample, recovery, and reconvergence phases
6. Modify `gapSampleProcessor`: transition to holdover mode when `consecutiveMissingSamples >= holdoverThreshold`
7. Controller `changeMode`: pass tracking processor reference from gap mode to holdover mode
8. Add holdover timer: track time since last good sample, enforce `duration`
