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

## Proposed Solution

**Existing parameter:**
- `badSampleRunLimit`: max consecutive bad samples (missing or outlier) before transitioning to reset mode

**New parameter:**
- `badSampleRunDrift`: max expected drift (ns) over `badSampleRunLimit` missing samples

The user already needs to set `badSampleRunLimit` small enough that the PHC stays within PTP clockAccuracy during the gap. So `badSampleRunDrift` is bounded by clockAccuracy (typically 100ns), though it would usually be set somewhat smaller to leave margin.

**Algorithm:**

Enter **post-gap mode** after N consecutive missing samples. Record the pre-gap median M and the count N.

While in post-gap mode, on receiving sample X:

1. Calculate envelope (using 1.5 power law for random walk FM):
   $$U(N) = badSampleRunDrift \times \left(\frac{N}{badSampleRunLimit}\right)^{1.5}$$

2. Calculate limit:
   $$Limit = (k \times MAD) + U(N)$$

3. Calculate deviation from pre-gap median:
   $$D = |X - M|$$

4. If D ≤ Limit (sample passes):
   - Shift all samples in MAD window by (X - M)
   - Accept X, feed to servo
   - Exit post-gap mode

5. If D > Limit (sample fails):
   - Mark X as outlier, add to MAD window (don't feed to servo)
   - N = N + 1
   - If N ≥ badSampleRunLimit: go to reset mode
   - Otherwise remain in post-gap mode (same M)

**Key properties:**
- Samples explainable by drift are accepted; the MAD window is shifted to the new center
- True outliers (e.g., multipath) are rejected even after a gap
- Missing samples and post-gap outliers both count toward badSampleRunLimit
- The envelope grows with the 1.5 power of time, matching random walk FM physics
