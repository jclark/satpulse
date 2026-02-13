# Phase 5g: PVT messages panel

## Goal
Add a PVT Messages section to the Monitor tab that shows three tables (Position, Velocity, Time) with one row per source message. Each row shows native fields (bold) alongside computed/derived fields (not bold), enabling cross-checking that different messages give consistent results.

## Prerequisite
Phase 5b (tab-based layout). Position/velocity message types in `gps/gpsprot/msg.go` (implemented for UBX).

## Motivation
The backend now emits `PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg` and multiple `TimeMsg` variants from different protocol messages. There is no way to see this data in the desktop UI. When developing and testing protocol extractors, you need to verify that NAV-PVT, NAV-POSLLH, NAV-POSECEF, etc. all report consistent positions. A cross-reference table with native vs computed fields makes inconsistencies immediately visible.

## Concept

### One row per source message
Each table is keyed by `nativeMsgID` (e.g. `NAV-PVT`, `NAV-POSLLH`, `NAV-POSECEF`). When a new message arrives, it updates the row for that `nativeMsgID`. This gives a live snapshot of the most recent value from each source.

### Native vs computed fields
Fields that come directly from the message are displayed in **bold**. Fields that are derived (e.g. ECEF computed from LLH via `LLHtoECEF`, or UTC computed from TAI minus leap seconds) are displayed in normal weight. Fields that cannot be derived are shown as `—`.

### Position table

| Column | PosGeoMsg | PosECEFMsg |
|--------|-----------|------------|
| Message | tag + nativeMsgID (e.g. "UBX NAV-PVT") | tag + nativeMsgID |
| Lat | **native** | computed via ECEFtoLLH |
| Lon | **native** | computed via ECEFtoLLH |
| Height | **native** (if present) | computed via ECEFtoLLH |
| Height MSL | **native** (if present) | — |
| ECEF X | computed via LLHtoECEF | **native** |
| ECEF Y | computed via LLHtoECEF | **native** |
| ECEF Z | computed via LLHtoECEF | **native** |
| H Acc | **native** (if present) | — |
| V Acc | **native** (if present) | — |
| P Acc | — | **native** (if present) |

The LLH→ECEF conversion requires height. When a PosGeoMsg has no height (e.g. NMEA RMC), the ECEF columns are `—`.

### Velocity table

| Column | VelGeoMsg | VelECEFMsg |
|--------|-----------|------------|
| Message | tag + nativeMsgID | tag + nativeMsgID |
| N | **native** (if velNED set) | — |
| E | **native** (if velNED set) | — |
| D | **native** (if velNED set) | — |
| Ground speed | **native** (if set) | — |
| 3D speed | **native** (if set) | — |
| Heading | **native** (if set) | — |
| ECEF VX | — | **native** |
| ECEF VY | — | **native** |
| ECEF VZ | — | **native** |
| Speed acc | **native** (if set) | **native** (if set) |
| Heading acc | **native** (if set) | — |

No cross-conversion between geo and ECEF velocity (would need the position for the rotation matrix).

### Time table

| Column | Source |
|--------|--------|
| Message | tag + nativeMsgID |
| Ref | **native** — TimeRef value (NavSolution, PrePulse, PostPulse) |
| UTC | **native** if `utcTime` present; computed from TAI − leap seconds otherwise |
| TAI | **native** if `taiTime` present; computed from UTC + leap seconds otherwise |
| Leap seconds | **native** if `utcOffset` non-zero in message; from global LeapSecondMsg otherwise |
| Accuracy | **native** (if present) |
| GNSS | **native** (if present) |
| Local | always computed from UTC (not bold) |

TAI is displayed in ISO 8601 format (like UTC), differing by the leap second offset.

### Coordinate conversions
The frontend calls existing Go backend methods for ECEF↔LLH conversion:
- `ECEFtoLLH(x, y, z float64)` — already bound in `desktop/app.go`
- `LLHtoECEF(lat, lon, height float64)` — new binding needed

These are async calls. Results are cached per row and updated when the source message changes. Follow the same `useEffect` pattern as `desktop/frontend/src/survey-panel.tsx`.

### JSON serialization reference
The Go types serialize to JSON as follows (the frontend receives these raw values):
- `Length` (int64) → raw number in **micrometers**
- `Angle` (int64) → raw number in **nanodegrees**
- `Speed` (int64) → raw number in **micrometers per second**
- `ptime.Time` → string `"seconds.nanoseconds"` (via `MarshalText`)
- `ptime.UTCTime` → ISO 8601 string (via `MarshalJSON`)
- `opt.Val[T]` → omitted from JSON when unset (via `omitzero` tag); inner value when set
- `Point3D` (`[3]Length`) → `[number, number, number]` in micrometers
- `Tag` → string (e.g. `"UBX"`, `"NMEA"`)
- `time.Duration` → number in nanoseconds
- `TimeRef` (int) → number (0=NavSolution, 1=PrePulse, 2=PostPulse)

---

## Steps

### 1. Backend: add LLHtoECEF binding
Add to `desktop/app.go`:

```go
func (a *App) LLHtoECEF(lat, lon, height float64) [3]float64 {
	ecef := geopos.WGS84.LLHtoECEF(geopos.LLH{Lat: lat, Lon: lon, Height: height})
	return [3]float64(ecef)
}
```

### 2. Backend: add PVT message handlers
Override `DefaultHandler` methods on `msgHandler` in `desktop/app.go` to emit `gps:msg` events:
- `PosGeo` → kind `"posGeo"`
- `PosECEF` → kind `"posECEF"`
- `VelGeo` → kind `"velGeo"`
- `VelECEF` → kind `"velECEF"`

Each handler simply emits the raw `gpsprot` message struct as `Msg` with no filtering or deduplication.

### 3. Backend: remove time deduplication
Remove the dedup logic (PrePulse filter, same-second skip) from the `Time` handler in `msgHandler`. Emit every `TimeMsg` as kind `"time"`. The frontend handles deduplication.

### 4. Frontend: time deduplication in app.tsx
In the `gps:msg` event handler in `app.tsx`, the `"time"` case must do two things:
1. **PVT time rows map** — update unconditionally (every TimeMsg, including PrePulse).
2. **`timeMsg` state** (for the existing TimePanel) — apply the old dedup logic in JS: skip if `ref` is PrePulse (value 1), compute TAI seconds, round to nearest second, skip if same as last emitted. This keeps the existing TimePanel showing one-per-second updates without flooding.

### 5. Frontend: PVT panel component
Create `desktop/frontend/src/pvt-panel.tsx`:
- Props: position rows map, velocity rows map, time rows map, leap second state.
- Three sub-sections (Position, Velocity, Time), each a compact `<table>`.
- Bold class (`font-bold`) for native fields, normal weight for computed.
- Unit conversion helpers (can be in the same file or a shared utils file):
  - `ndegToDeg(nd: number): number` — nanodegrees to degrees (`nd / 1e9`)
  - `umToM(um: number): number` — micrometers to meters (`um / 1e6`)
  - `umsToMs(ums: number): number` — micrometers/sec to m/s (`ums / 1e6`)
- Async ECEF↔LLH conversion via `useEffect` watching row changes, following the pattern in `survey-panel.tsx` (call Go backend, store results in component state).
- Monospace tabular numbers (`tabular-nums font-mono`), compact rows (`text-xs`), minimal padding.

### 6. Frontend: wire up in app.tsx
- Add state: `posRows`, `velRows`, `timeRows` as `Map<string, ...>` (keyed by `nativeMsgID`).
- In the `gps:msg` event handler, add cases for `"posGeo"`, `"posECEF"`, `"velGeo"`, `"velECEF"`.
- Add `<PVTPanel>` to the Monitor tab in a `CollapsibleSection` titled "PVT Messages".
- Import `LLHtoECEF` from `wailsjs/go/main/App`.
- Clear PVT state on disconnect (when `gps:state` transitions to `"disconnected"`).

### 7. Update README.md
Add phase5g to the phase table and dependency graph in `desktop/plan/README.md`.

---

## Result
The Monitor tab gains a PVT Messages section with three tables showing live position, velocity, and time data from every source message. Native fields are bold; computed fields are normal weight. This makes it easy to verify cross-message consistency during protocol development and testing.

## Files changed
- `desktop/app.go` — add `LLHtoECEF` method, add PosGeo/PosECEF/VelGeo/VelECEF handlers, remove time dedup
- `desktop/frontend/src/pvt-panel.tsx` — new component
- `desktop/frontend/src/app.tsx` — add PVT state, wire up events, add PVT section to Monitor tab, add time dedup in JS
- `desktop/frontend/src/time-panel.tsx` — may need minor adjustment if it currently relies on backend dedup behavior
- `desktop/plan/README.md` — add phase5g entry

## Testing -- Playwright

### Position table
- Connect to a UBX receiver; verify rows appear for NAV-PVT, NAV-POSLLH, NAV-POSECEF.
- Verify lat/lon/height are bold in NAV-PVT and NAV-POSLLH rows; ECEF is bold in NAV-POSECEF row.
- Verify computed ECEF values in geo rows are close to native ECEF values in the POSECEF row.

### Velocity table
- Verify rows appear for NAV-PVT, NAV-VELNED, NAV-VELECEF.
- Verify NED components are bold in geo rows; ECEF components are bold in VELECEF row.

### Time table
- Verify rows appear for NAV-TIMEGPS, NAV-TIMEGAL, NAV-PVT, etc.
- Verify Ref column shows the time reference type.
- Verify UTC/TAI are shown in ISO 8601; difference matches leap seconds column.
- Verify bold/non-bold matches which fields the message natively provides.

### TimePanel compatibility
- Verify the existing TimePanel still shows one-per-second updates (no flooding).
- Verify PrePulse messages do not appear in the TimePanel.
