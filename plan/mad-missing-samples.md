# How Should MAD-Based Outlier Detection Handle Missing Samples?

## Context

We have a PI servo synchronizing a PHC (PTP Hardware Clock) to GPS PPS pulses. The servo operates in "tracking mode" once synchronized, receiving 1Hz samples showing the offset between PHC and GPS.

We use MAD (Median Absolute Deviation) based outlier detection to reject bad samples (e.g., multipath errors, interference). The MAD window stores recent offsets and computes:
- Median of offsets (the "center")
- MAD: median of absolute deviations from center
- A sample is an outlier if its deviation from center exceeds k × MAD

During normal tracking, offsets are typically 5-20ns with occasional outliers of 100s of ns.

Some samples may be missing (GPS signal lost). During missing samples:
- The servo switches to an averaged frequency estimate computed before the missing samples began
- The PHC continues running at this frequency
- Phase error accumulates if the frequency estimate doesn't perfectly match the true frequency

## Scope

This problem concerns runs of missing samples short enough that the PHC remains within its advertised clockAccuracy (typically 100ns for PTP applications). For longer runs where drift would exceed clockAccuracy, we will transition to a separate holdover mode with different handling. This document focuses only on cases where we want to remain in tracking mode and maintain sub-100ns accuracy.

## The Problem

We observed unexpected behavior in simulation. After a run of missing samples, legitimate samples were being rejected as outliers:

| Missing Samples | TrackingAbsMax | Outliers Rejected |
|-----------------|----------------|-------------------|
| 30              | 21ns           | 0                 |
| 35              | 71ns           | 0                 |
| 45              | 188ns          | 4                 |

With 45 missing samples:
1. During the gap, the PHC drifts ~180ns from GPS
2. When samples return, the first samples show offset of ~155-180ns
3. The MAD detector compares this to the pre-gap window (median ~5ns, MAD ~3ns)
4. The 180ns deviation is flagged as an outlier (exceeds k × MAD threshold)
5. The legitimate post-gap samples are rejected
6. The PHC stays drifted until "outliers" shift the median or we hit the bad sample limit

The post-gap samples are **not** outliers - they correctly reflect that the PHC drifted during the missing samples. But the MAD detector doesn't know about the gap; it just sees a sudden jump.

## The Question

How should MAD-based outlier detection handle missing samples?

Key constraints:
- We want to stay in tracking mode and quickly resume accurate synchronization
- The drift during missing samples is expected to be small (under 100ns) given our hardware and limits on consecutive missing samples
- We still need protection against actual outliers (multipath, interference) that might occur immediately after missing samples
- The solution should be simple and robust

## Solution

Builds on: tracking-limits.md

### Overview

1. Detect significant gaps (>= GapMinSamples consecutive missing samples)
2. Use relaxed MAD detection for post-gap samples (add GapDriftLimit to threshold)
3. Accumulate GapRecoverySamples without adding to MAD window
4. Shift window by (new_median - old_median), capped by GapDriftLimit (reset if exceeded)
5. Add recovery samples to window, resume normal MAD detection

### New Configuration Parameters

Add to `TrackingConfig`:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `GapMinSamples` | 5 | Min consecutive missing samples to trigger gap handling |
| `GapDriftLimit` | 100ns | Added to MAD threshold during gap recovery |
| `GapRecoverySamples` | 5 | Samples to collect before shifting window |

### State Variables

Add to `trackingSampleProcessor` (2 new fields):

```go
consecutiveMissingSamples int              // count of consecutive missing samples
gapPostOffsets            []time.Duration  // post-gap samples; len > 0 means in recovery
```

During recovery, don't add samples to the MAD window—just accumulate in `gapPostOffsets`. The window still reflects pre-gap state, so we can compute median/MAD from it directly.

### Gap Recovery Detection

Enhance existing `madIsOutlier` with a `postGap bool` parameter. When true, add GapDriftLimit to the threshold:

```go
func (p *trackingSampleProcessor) madIsOutlier(offset time.Duration, postGap bool) bool {
    center := p.offsetWindow.Median()
    mad := ... // existing MAD computation
    threshold := time.Duration(float64(max(mad, madFloor)) * p.cfg.MADMultiple)
    if postGap {
        threshold += time.Duration(p.cfg.GapDriftLimit)
    }
    deviation := (offset - center).Abs()
    return deviation > threshold
}
```

With GapDriftLimit=100ns, pre-gap median=5ns, pre-gap MAD=3ns, MADMultiple=25: threshold = 100ns + 75ns = 175ns.

### Logic Flow

**On missing sample:**
- Increment `consecutiveMissingSamples`

**On good sample:**
- If `consecutiveMissingSamples >= GapMinSamples` and not in recovery:
  - Only enter recovery if `offsetWindow.Len() >= MADMinSamples` (need meaningful stats)
  - If so, initialize `gapPostOffsets` slice (entering recovery)
  - Otherwise, stay on normal path (PreMAD detection handles it)
- If in recovery (`len(gapPostOffsets) > 0`):
  - Use `madIsOutlier(offset, true)` (adds GapDriftLimit to threshold)
  - Don't add to MAD window (keep it reflecting pre-gap state)
  - If accepted: append to `gapPostOffsets`, feed to servo, reset `consecutiveBadSamples`
  - If rejected: increment `consecutiveBadSamples`; if >= BadSampleRunLimit, reset mode
  - If `len(gapPostOffsets) >= GapRecoverySamples`: shift window, clear slice
- If not in recovery:
  - Use `madIsOutlier(offset, false)`, add to window as usual
- Reset `consecutiveMissingSamples = 0`

**Window shift:**
```go
preGapMedian := p.offsetWindow.Median()  // window unchanged during recovery
newMedian := median.Median(gapPostOffsets)
shift := newMedian - preGapMedian
// If shift exceeds GapDriftLimit, drop to reset mode
if shift.Abs() > time.Duration(p.cfg.GapDriftLimit) {
    return ModeReset
}
offsetWindow.ShiftAll(shift)
// Add recovery samples to window so MAD statistics reflect post-gap behavior
for _, offset := range gapPostOffsets {
    offsetWindow.Add(offset)
}
gapPostOffsets = nil  // exit recovery
```

### Edge Cases

- **Entering tracking mode**: initialize `consecutiveMissingSamples = 0`
- **Gap during MAD warmup**: don't enter recovery (window.Len() < MADMinSamples); use normal PreMAD detection path
- **Missing sample during recovery**: keep accumulated samples, continue recovery
- **Interaction with BadSampleRunLimit**: reset gap state when entering reset mode

## Files to Modify

| File | Changes |
|------|---------|
| `internal/median/median.go` | Add `ShiftAll` method |
| `internal/phcsync/tracking.go` | Gap state, config, detection logic |
| `configs/config-schema.json` | Add new parameters |
| `internal/syncsim/sync_test.go` | Gap recovery test scenarios |
