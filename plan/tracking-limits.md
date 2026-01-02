# Tracking Mode Limits Redesign

Fixes: #174, #178, #187

## Problem

Current design has a single `BadSampleLimit` for consecutive bad samples (missing or outlier). This is insufficient:

1. **Issue 174**: MAD window (10) is too small. A phase step produces only 4-5 outliers before median recenters, often missing the bad sample limit (5).

2. **Issue 187**: Can't increase `BadSampleLimit` to tolerate longer outages because outliers accumulate in the MAD window and shift the median, causing a phase excursion to be absorbed rather than trigger reset.

3. **Issue 178**: Only tracking consecutive bad samples, not frequency. A system with frequent intermittent failures should exit tracking even if failures aren't consecutive.

Follow-on is issue 188.

## Solution

Four separate limits:

| # | Parameter | Purpose |
|---|-----------|---------|
| 1 | Max consecutive bad samples | Trigger reset after N consecutive bad samples (missing or outlier) |
| 2 | Max outlier fraction of MAD window | Trigger reset if outliers exceed this fraction of MAD window |
| 3 | Bad sample window size | Sliding window for tracking bad sample frequency |
| 4 | Max bad sample fraction | Trigger reset if bad samples exceed this fraction of bad window |

**Key insight**: Parameter 2 (outlier fraction limit) is the safety valve that prevents median corruption. Once in place:
- Can increase MAD window size (fixes #174)
- Can increase consecutive bad limit to tolerate longer outages (fixes #187)
- Phase steps hit outlier fraction limit before median recenters

## Parameter Names

Config field names (TOML uses camelCase):

1. `badSampleRunLimit` - max consecutive bad samples before reset
2. `outlierRatioLimit` - max ratio of MAD window that can be outliers (0.0-1.0)
3. `badSampleWindow` - window size for tracking bad sample frequency
4. `badSampleRatioLimit` - max ratio of bad sample window that can be bad (0.0-1.0)

Replaces current `badSampleLimit`.

## Default Values

| Parameter | Current | Proposed | Rationale |
|-----------|---------|----------|-----------|
| `madWindow` | 10 | 20 | Larger window, more outliers before median shifts |
| `badSampleRunLimit` | 5 | 30 | Can be generous now that outlier ratio protects us |
| `outlierRatioLimit` | N/A | 0.3 | 6 outliers in window of 20 triggers reset |
| `badSampleWindow` | N/A | 60 | 1 minute window |
| `badSampleRatioLimit` | N/A | 0.5 | 50% bad in window triggers reset |

## Implementation

### New type in tracking.go

Create a wrapper struct at bottom of `internal/phcsync/tracking.go` that embeds `median.Window` and adds outlier tracking:

```go
// madWindow tracks offsets for MAD-based outlier detection and counts
// how many samples in the window are outliers.
type madWindow struct {
    *median.Window[time.Duration]
    outliers     *circbuf.Buffer[bool]
    outlierCount int
}

func newMADTWindow(capacity int) *madWindow {
    return &madTracker{
        Window:   median.New[time.Duration](capacity),
        outliers: circbuf.New[bool](capacity),
    }
}

// Add adds an offset to the window with its outlier status.
// Must be called for every sample (including outliers) to keep median accurate.
func (w *madWindow) Add(offset time.Duration, isOutlier bool) {
    // Decrement count if evicting an outlier
    if w.outliers.Len() == w.outliers.Cap() {
        if w.outliers.Last(w.outliers.Len() - 1) {
            w.outlierCount--
        }
    }
    w.Window.Add(offset)
    w.outliers.Append(isOutlier)
    if isOutlier {
        w.outlierCount++
    }
}

```

### Bad sample window

Separate `circbuf.Buffer[bool]` for tracking bad samples (missing or outlier) over a longer window. Maintain count similarly.

### Changes to trackingSampleProcessor

- Replace `offsetWindow *median.Window[time.Duration]` with `*madTracker`
- Add `badSamples *circbuf.Buffer[bool]` and `badSampleCount int`
- Update `processSample` to check all four limits
- Update `sampleAction` to call `madTracker.Add()` with outlier status

### Changes to TrackingConfig

Remove:
- `BadSampleLimit`

Add:
- `BadSampleRunLimit int`
- `OutlierRatioLimit float64`
- `BadSampleWindow int`
- `BadSampleRatioLimit float64`

## Testing

- Unit tests for `madTracker`
- Update existing tracking tests
- syncsim scenarios for:
  - Phase step exceeding outlier ratio
  - Intermittent failures exceeding bad ratio
  - Long outage within consecutive limit
