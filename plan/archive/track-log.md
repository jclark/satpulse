# Track log

## Overview

Add a JSONL track log that records one trackpoint per navigation epoch, enabled by `track = true` in the `[log]` section of `satpulse.toml`. The log file is written to `track.{devname}.jsonl` in the log directory. Also provide a Python utility to convert the track log to GPX format.

## JSONL format

Each line is a JSON object with these fields:

| Field    | Type   | Description                              |
|----------|--------|------------------------------------------|
| t        | string | RFC 3339 timestamp, millisecond precision |
| lat      | number | Latitude in degrees                      |
| lon      | number | Longitude in degrees                     |
| ele      | number | Elevation (height above MSL) in meters   |
| hdop     | number | Horizontal DOP                           |
| vdop     | number | Vertical DOP                             |
| pdop     | number | Position DOP                             |
| numsat   | number | Number of satellites used in solution    |
| fixType  | string | One of: "none", "2d", "3d", "dgps"       |

Fields `lat` and `lon` are always present. All other fields use `opt.Val` with `omitzero` and are omitted when not available (`fixType` is a plain string with `omitzero` -- empty string means not provided). Lines are only written when `PosGeo` is present in the PV bundle. The `fixType` is derived from `FixDim` and `FixLevel`:

- `FixLevel >= CodeCorrected` -> "dgps"
- `FixDim == 3D` -> "3d"
- `FixDim == 2D` -> "2d"
- `FixLevel == None` or `FixDim` not set -> "none"
- otherwise omitted

## Prerequisites

Depends on [dispatcher-tick.md](dispatcher-tick.md) — centralizing `TimeTicker` in the Dispatcher and adding the `Tick` method to the Observer interface.

## Go changes

### 1. Add `Track` field to `LogConfig` (`time/app/daemon/config.go`)

Add `Track bool` field and `TrackPath` method, following the pattern of `Clock`/`Packet`/`Event`.

### 2. Add `TrackLogObserver` to `logobs` (`time/internal/logobs/tracklog.go`)

New file, under 50 lines. Structure:

```go
type TrackLogObserver struct {
    obs.DefaultObserver
    lg *slog.Logger
    lf logfile.LogFile
}
```

Implements `Tick()` to cache the latest filled `TimeMsg.UTCTime`. In `NavEpochPV`: skips if `PosGeo` is not set or no `UTCTime` has been received, otherwise formats the cached UTC time as RFC 3339 with millisecond precision, marshals a trackpoint struct to JSON, and writes a line. Implements `ReopenLog` and `Release` for log rotation and cleanup (same pattern as `ClockLogObserver`).

The trackpoint entry type:

```go
type TrackLogEntry struct {
    T       string             `json:"t"`
    Lat     float64            `json:"lat"`
    Lon     float64            `json:"lon"`
    Ele     opt.Val[float64]   `json:"ele,omitzero"`
    HDOP    opt.Val[float64]   `json:"hdop,omitzero"`
    VDOP    opt.Val[float64]   `json:"vdop,omitzero"`...
    PDOP    opt.Val[float64]   `json:"pdop,omitzero"`
    NumSat  opt.Val[uint16]    `json:"numsat,omitzero"`
    FixType string             `json:"fixType,omitzero"`
}
```

`lat` and `lon` are always present (we only write when PosGeo is set). Optional fields use `opt.Val` with `omitzero` to omit when not available. `fixType` is a plain string with `omitzero` (empty string = not provided).

### 3. Wire up in daemon (`time/app/daemon/daemon.go`)

Add `newTrackLogObserver` helper (follows `newClockLogObserver` pattern) and add to `combineObservers`.

### 4. Update example config (`configs/satpulse.toml`)

Add commented-out `# track = true` line in the `[log]` section.

## Python converter (`tools/track2gpx.py`)

Standalone Python script (no dependencies beyond stdlib) that reads a track JSONL file from stdin or a file argument and writes GPX XML to stdout.

### GPX 1.1 output format

The output is a valid GPX 1.1 file. The root element declares the GPX namespace, xsi namespace, and schema location:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="track2gpx"
     xmlns="http://www.topografix.com/GPX/1/1"
     xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
     xsi:schemaLocation="http://www.topografix.com/GPX/1/1 http://www.topografix.com/GPX/1/1/gpx.xsd">
```

The file contains a single `<trk>` with one or more `<trkseg>` elements. A new segment is started when a time gap exceeds a threshold (default 10 seconds, configurable via `--max-gap`).

### Track point element ordering

GPX 1.1 uses `xsd:sequence`, so child elements of `<trkpt>` must appear in schema order. The elements used (all optional, omitted when not present in the JSONL):

1. `<ele>` -- elevation in meters
2. `<time>` -- ISO 8601 timestamp
3. `<fix>` -- fix type enum
4. `<sat>` -- number of satellites (non-negative integer)
5. `<hdop>` -- horizontal DOP
6. `<vdop>` -- vertical DOP
7. `<pdop>` -- position DOP

### Field mapping

| JSONL field | GPX element | Notes |
|-------------|-------------|-------|
| `lat`, `lon` | `<trkpt lat= lon=>` | attributes on track point |
| `t` | `<time>` | ISO 8601, passed through |
| `ele` | `<ele>` | meters |
| `fixType` | `<fix>` | "none", "2d", "3d", "dgps" map directly to GPX fix values |
| `numsat` | `<sat>` | integer |
| `hdop` | `<hdop>` | decimal |
| `vdop` | `<vdop>` | decimal |
| `pdop` | `<pdop>` | decimal |

### Gap detection

When the time difference between consecutive trackpoints exceeds `--max-gap` seconds (default 10), the current `<trkseg>` is closed and a new one is opened. This prevents GPS outages from creating long straight-line artifacts in the track.
