# GPSDO-based Holdover (#152)

## Introduction

This document describes holdover when using an external GNSS Disciplined Oscillator (GPSDO). The GPSDO disciplines the oscillator upstream of SatPulse, so pulses continue during holdover but are no longer GNSS-locked.

Note that a suitable GPSDO needs to have PPS output aligned to top-of-second. I have three suitable GPSDOs:

- BG7TBL CM55
- tinyGTC
- Intel E810-XXVDA4T (this isn't a GPSDO but rather a PHC which gets its PPS from a DPLL that can be attached to a GPS; overall effect from SatPulse's perspective is the same)

This is a completely different implementation from PHC-based holdover (see phc-holdover.md). The two approaches share the `ModeHoldover` constant and PTP clock quality semantics, but share no code.

## Key difference from PHC-based holdover

With PHC-based holdover, PPS pulses stop arriving when GNSS lock is lost. SatPulse must free-run using a frequency model.

With GPSDO-based holdover, PPS pulses **continue** arriving—the GPSDO keeps outputting pulses from its disciplined oscillator. SatPulse continues tracking these pulses normally. The only difference is:
1. The pulses are no longer GNSS-locked (degraded accuracy)
2. PTP clock quality must reflect this (advertise holdover quality)

We detect loss/restoration of lock via time messages rather than missing pulses.

## Detecting lock loss

A `TimeMsg` indicates loss of lock when:
- `UTCTime == nil` AND `TAITime == 0`

Add method to `TimeMsgBuffer`:

```go
// IsLocked returns true if the most recent navigation-solution TimeMsg
// indicates a valid time and is not stale.
func (b *TimeMsgBuffer) IsLocked() bool
```

## State machine

- tracking mode -> holdover mode: `!timeMsgBuf.IsLocked()`
- holdover mode -> tracking mode: GPS lock restored AND GPSDO reconverged

No gap mode is needed—pulses don't go missing, so there's no brief-gap handling.

## GPSDO reconvergence

When GPS lock is restored, the GPSDO's internal discipline loop needs time to reconverge. During this period, the pulses are still degraded even though time messages indicate lock. We need to wait for GPSDO reconvergence before returning to tracking mode.

Two approaches:

**1. Wait time (simple)** - Configure a `reconvergenceDelay` and stay in holdover for that duration after GPS lock returns. Works with any GPSDO (CM55, etc.) but conservative.

**2. External program** - Exec a GPSDO-specific program (e.g., Python script for tinyGTC SCPI, or a small binary for E810 DPLL status) and read state events from its stdout. Each line is an event like `locked` or `holdover`. Returns to tracking mode when program reports GPSDO is locked.

## Holdover processor behavior

Unlike PHC-based holdover which has three phases (no-sample, recovery, reconvergence), GPSDO holdover is simple:

- Continue running the PI servo normally
- Continue processing samples normally
- The only change is PTP clock quality (advertise holdover)

The servo never stops, so no SatPulse-side reconvergence is needed. We just wait for the GPSDO to reconverge.

## Configuration

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `gps.disciplined` | bool | false | True if using external GPSDO |
| `gps.reconvergenceDelay` | float | 60 | Seconds to wait after GPS lock before exiting holdover (wait time approach) |

The `gps.disciplined` setting selects the GPSDO holdover implementation instead of PHC-based holdover.

## Implementation steps

1. Add `IsLocked()` to `TimeMsgBuffer`
2. New `gpsdoHoldoverSampleGenerator`: checks lock status instead of pulse presence
3. New `gpsdoHoldoverSampleProcessor`: continues tracking (essentially identical to tracking processor)
4. Controller selects implementations based on `gps.disciplined`
5. Implement reconvergence delay timer (wait time approach)
6. External program interface for GPSDO state events.
