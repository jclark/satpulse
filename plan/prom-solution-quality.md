# Prometheus solution quality metrics

## Context

The `NavEpochMsg` (implemented in #217 per `solution-quality.md`) provides rich per-epoch solution quality metadata: GNSS fix level, position/time solution dimensionality, satellite counts, DOPs, accuracy estimates, and correction age. The `PrometheusObserver` (`time/internal/promobs/prometheus.go`) currently exposes PHC timing, position, and satellite signal metrics but does not export any of these quality fields. This plan adds Prometheus gauges for the operationally relevant `NavEpochMsg` fields, and refactors the existing position metrics for consistency.

All metrics use the `satpulse_` prefix and are `prometheus.Gauge`. Each gauge is lazily registered on first data. Gauges backed by `opt.Val` fields are never created if the receiver does not provide that field.

Model assumptions used by this plan:
- `FixLevel` is GNSS-specific and ordered: `none`, `not_measured`, `doppler`, `code`, `code_corrected`, `carrier_float`, `carrier_fixed`.
- `SolutionDim` (renamed from `FixDim`) is position/time dimensionality only: `2d`, `3d`, `time_only`.
- `SolutionDim` is only set when `FixLevel >= code`.
- `AuxSrc` is independent and may be non-zero even when GNSS fix level is `none`/unset.

## Step 0: refactor existing position metrics

Replace the four separate position gauges with two labelled `GaugeVec`s:

| Metric | Labels | Source |
|---|---|---|
| `satpulse_position_degrees` | `coord` = `latitude` / `longitude` | `PosGeo.LatLon` |
| `satpulse_height_meters` | `type` = `ellipsoid` / `msl` | `PosGeo.Height` / `.HeightMSL` |

Each `type` label on height only appears if the receiver provides that field.

Replaces: `satpulse_position_latitude_degrees`, `satpulse_position_longitude_degrees`, `satpulse_position_height_meters`, `satpulse_position_elevation_meters`.

## Step 1: solution quality metrics

### Fix state

| Metric | Labels | Source |
|---|---|---|
| `satpulse_gnss_fix` | `level` | `msg.FixLevel` |
| `satpulse_gnss_solution` | `dim` | `msg.SolutionDim` |

Each metric value is always 1 for the active state and 0 for previously seen inactive states. `satpulse_gnss_fix` label values: `none`, `not_measured`, `doppler`, `code`, `code_corrected`, `carrier_float`, `carrier_fixed`. `satpulse_gnss_solution` label values: `2d`, `3d`, `time_only`. `satpulse_gnss_solution` is only emitted when `msg.SolutionDim` is set.

### Satellite counts

| Metric | Labels | Source |
|---|---|---|
| `satpulse_num_satellites` | `status` = `used` / `tracked` / `in_view` | `msg.NumSVUsed` / `NumSVTracked` / `NumSVInView` |

Each label value only appears if the receiver provides that field.

### Dilution of precision

| Metric | Labels | Source |
|---|---|---|
| `satpulse_dop` | `type` = `geometric` / `position` / `horizontal` / `vertical` / `time` | `msg.DOP.Geom` / `.Pos` / `.Hor` / `.Vert` / `.Time` |

Each label value only appears if the receiver provides that DOP component.

### Accuracy estimates

| Metric | Labels | Source |
|---|---|---|
| `satpulse_position_accuracy_meters` | `type` = `horizontal` / `vertical` / `3d` | `msg.Acc.Hor` / `.Vert` / `.Pos` |
| `satpulse_speed_accuracy_meters_per_second` | `type` = `3d` / `ground` | `msg.Acc.Speed` / `.GroundSpeed` |
| `satpulse_course_accuracy_degrees` | (none) | `msg.Acc.Course` |

Each label value only appears if provided.

### Corrections

| Metric | Labels | Source |
|---|---|---|
| `satpulse_gnss_correction` | `kind` | `msg.Correction` (expanded) |
| `satpulse_gnss_correction_leaf` | `kind` | `msg.Correction` (leaf only) |

Value is always 1. `satpulse_gnss_correction` emits one label value per asserted bit including all implied bits (e.g. if `PPPConverged` is set, also emit `PPP`, `SSR`, `used`). `satpulse_gnss_correction_leaf` emits only the maximally specific assertions (what `CorrKind.items()` returns). `kind` values: `used`, `osr`, `ssr`, `rtcm`, `partial_dual_freq`, `full_dual_freq`, `sbas`, `clas`, `spartn`, `ppp`, `ppp_rtk`, `ppp_converging`, `ppp_converged`, `ppp_has`, `ppp_mdc`, `ppp_b2b`.

### Correction metadata

| Metric | Source |
|---|---|
| `satpulse_correction_age_seconds` | `msg.DiffAge` |
| `satpulse_correction_base_id` | `msg.RTCMRefBaseID` |

Only appear if the receiver provides differential corrections.

### Auxiliary sources

| Metric | Labels | Source |
|---|---|---|
| `satpulse_nav_aux_source` | `kind` | `msg.AuxSrc` |

Value is always 1. One label value per asserted bit. `kind` values: `dr`, `ins`.

### Excluded fields

- **`SignalsUsed` (SignalSet)**: Per-satellite signal data already exposed via `Satellites`.
- **`Tag`**: Static per-session.

## Implementation

All changes in `time/internal/promobs/prometheus.go`.

### Absent/stale value handling

All metrics use `*prometheus.GaugeVec` (even metrics with no labels like `correction_age_seconds`). This enables a consistent scheme for handling absent values:

- **Bitmask/info metrics** (`gnss_fix`, `gnss_solution`, `gnss_correction`, `gnss_correction_leaf`, `nav_aux_source`): Register a label value on first encounter. Set to 1 when active, 0 when not. Never delete. A label value that has never been seen is never registered.
- **Numeric opt.Val metrics** (`dop`, `num_satellites`, `position_accuracy_meters`, `speed_accuracy_meters_per_second`, `course_accuracy_degrees`, `correction_age_seconds`, `correction_base_id`): Register a label value (or the no-label entry for label-free metrics) on first `IsSet()`. Set value when set. `DeleteLabelValues()` when unset. Never created if the receiver never provides that field.
- **Position metrics** (`position_degrees`, `height_meters`): Same as numeric opt.Val. `position_degrees` is always present when position is reported. `height_meters` labels appear/disappear per field availability.

This extends the existing pattern used by `height_meters`/`elevation` to all opt.Val-backed metrics. In practice, opt.Val fields are stable per receiver, so deletions will be rare.
