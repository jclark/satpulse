# Phase 3: Text panels for time and survey status

## Goal
Build text-based panels displaying time and survey status - equivalent to the cards in `web/dashboard.tsx`.

## Prerequisite
Phase 1 (panel layout). Phase 2 (structured logging) is independent but ideally done first.

## Reference documents
- [ui-workspace-panels.md](ui-workspace-panels.md) - panel layout for new panels

## What already exists

The backend message decoding pipeline is already implemented in `desktop/app.go`:

- `msgDecoderWorker` creates `gpsreg.CreatePacketProcessors()`, wires a `msgHandler` implementing `gpsprot.MsgHandler`, subscribes to the packet broadcast, and processes packets through the processors. This follows the same pattern as `time/internal/gpsevent/dispatcher.go` and `time/app/daemon/daemon.go`.
- `msgHandler` emits `"gps:msg"` Wails events with a `MsgEvent{Kind, Msg, Time}` envelope.

## Backend changes needed

### Remove DTOs, emit `*Msg` types directly

The existing `TimeMsgDTO`, `SurveyMsgDTO`, and `SatellitesMsgDTO` are unnecessary. The `gpsprot.*Msg` types are JSON-serializable as-is - the event log in `gpsevent/dispatcher.go` already serializes them directly via `json.Marshal` in production.

Replace the DTO wrappers with direct emission of the `*Msg` values. The frontend handles unit conversions (micrometers to meters, nanoseconds to seconds, etc.).

### Export ECEF-to-LLH conversion

Add an exported method on `App` that wraps `geopos.WGS84.ECEFtoLLH`. The method takes X, Y, Z as `float64` in meters and returns lat, lon (degrees) and height (meters). The frontend converts `SurveyMsg.position` from micrometers to meters before calling.

### Emit LeapSecondMsg to frontend

The `LeapSecond` handler should emit a `"gps:msg"` event with `kind: "leapSecond"` so the frontend can maintain leap second state. The frontend needs this to convert TAI to UTC (TAI - offset = UTC).

## Frontend steps

### 1. Subscribe to gps:msg events
Add a `gps:msg` event listener in `app.tsx`. Dispatch by `kind` to update per-kind state (latest time msg, latest survey msg, latest satellites msg, etc).

### 2. Time panel
New `time-panel.tsx` displaying:
- UTC time from `TimeMsg.utcTime` (RFC3339 string, when available)
- UTC time inferred from TAI using current leap second offset (TAI - offset)
- TAI time (from `TimeMsg.taiTime`, nanoseconds since epoch)
- Time source GNSS

The frontend maintains leap second state from `LeapSecondMsg` events. When `utcTime` is present in the `TimeMsg`, display it directly. Always also show the TAI-derived UTC.

Port formatting from `web/dashboard.tsx` and `web/timefmt.ts`.

### 3. Survey status panel
New `survey-panel.tsx` displaying:
- Survey accuracy (from `SurveyMsg.accuracy` in micrometers, convert to meters)
- ECEF position (from `SurveyMsg.position` array of micrometers)
- Lat/lon coordinates (via exported `ECEFtoLLH` Go method, with Google Maps link)
- Altitude
- Observation count and time
- Valid / In Progress status

Port formatting from `web/dashboard.tsx` (`surveyFormat`).

### 4. Receiver info display
Deferred to phase 4 (receiver panel). The data source is the `DetectReceiver` method, not the `*Msg` stream.

### 5. Update panel layout
Add new panels to the `PanelGroup` layout. Likely arrangement:
- Time and Survey panels in the left or right column
- Adjust default panel sizes to accommodate

## Result
The desktop app shows live GPS time and survey progress alongside the existing config and packet monitor panels. The semantic `*Msg` stream infrastructure is in place for signal graph (phase 6a) and sky view (phase 6b).

## Testing (Playwright)

Testing the semantic stream requires a connected receiver. Tests split into two categories.

### Without hardware (UI structure)
- Verify Time panel exists in the layout and shows placeholder or empty state.
- Verify Survey panel exists and shows placeholder or empty state.
- Verify panel layout accommodates the new panels without breaking resize behavior.

### With hardware (live data, manual or CI with hardware)
- Connect to a receiver.
- Verify Time panel updates with UTC time within a few seconds.
- Verify Survey panel populates if the receiver is in survey mode.
- Disconnect and verify panels show stale data or empty state (not errors).

## Files changed
- `desktop/app.go` (remove DTOs, simplify msgHandler, add ECEFtoLLH export)
- `desktop/frontend/src/app.tsx` (gps:msg event subscription, state for each msg kind)
- `desktop/frontend/src/time-panel.tsx` (new)
- `desktop/frontend/src/survey-panel.tsx` (new)
- `desktop/frontend/src/timefmt.ts` (new, ported from web/)
