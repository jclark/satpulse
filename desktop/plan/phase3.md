# Phase 3: Semantic data stream and text panels

## Goal
Add backend decoder that produces semantic `*Msg` events from raw packets. Build text-based panels displaying time, survey status, and receiver info - equivalent to the cards in `web/dashboard.tsx`.

## Prerequisite
Phase 1 (panel layout). Phase 2 (structured logging) is independent but ideally done first.

## Reference documents
- [backend.md](backend.md) - semantic `*Msg` stream definition and DTO design
- [ui-panel-connection.md](ui-panel-connection.md) - receiver info display options
- [ui-workspace-panels.md](ui-workspace-panels.md) - panel layout for new panels

## Steps

### 1. Backend: message decoder goroutine
Add a new goroutine that:
- subscribes to the packet broadcast (`pb.Subscribe()`)
- runs packets through `gpsreg.CreatePacketProcessors()` to decode into `gpsprot.*Msg` values
- emits decoded messages as `gps:msg` Wails events

The decoder starts when a connection is established (in `Connect`) and stops on disconnect.

Event payload:
```go
type MsgEvent struct {
    Kind string `json:"kind"` // e.g. "time", "satellites", "survey"
    Msg  any    `json:"msg"`
    Time string `json:"time"`
}
```

The `Msg` field contains a DTO appropriate to the kind. Map `gpsprot.*Msg` types to JSON-friendly DTOs at the Wails boundary.

### 2. Define message DTOs
Create DTO structs for each message kind the frontend needs:
- `TimeMsg` - UTC string, TAI string, leap seconds, time source GNSS
- `SurveyMsg` - accuracy, ECEF position, lat/lon, altitude, observation count/time, valid, in-progress
- `SatellitesMsg` - array of satellite info (id, constellation, look angles, signal strengths, usage)

These map from `gpsprot` message types. Use the same field names and structure as `web/dashboard.tsx` expects where possible, to make porting the display code easier.

### 3. Frontend: subscribe to gps:msg events
Add a `gps:msg` event listener in `app.tsx`. Dispatch by `kind` to update per-kind state (latest time msg, latest survey msg, latest satellites msg, etc).

### 4. Time panel
New `time-panel.tsx` displaying:
- Local time and date
- UTC
- TAI (if available)

Port formatting from `web/dashboard.tsx` (`formatUTC`, `formatTAI`, `formatDateTime`). Also port `web/timefmt.ts` helper functions to the desktop frontend.

### 5. Survey Status panel
New `survey-panel.tsx` displaying:
- Survey accuracy
- ECEF position (X, Y, Z)
- Lat/lon coordinates (with Google Maps link)
- Altitude
- Observation count and time
- Valid / In Progress status

Port formatting from `web/dashboard.tsx` (`surveyFormat`).

### 6. Receiver Info display
Receiver identity (vendor, hardware, firmware, supported GNSS) is handled by the receiver panel (phase 4), which auto-probes on connect via the `Configure` method with `forceProbe: true`. Two display options:
- Fold into the connection strip (compact, always visible when connected)
- Keep as a small panel

Decide based on how much space the connection strip has. The data source is the `gps:receiver` event from the auto-probe, not the `*Msg` stream.

### 7. Update panel layout
Add new panels to the `PanelGroup` layout. Likely arrangement:
- Time and Survey panels in the left or right column
- Adjust default panel sizes to accommodate

## Result
The desktop app shows live GPS time, survey progress, and receiver info alongside the existing config and packet monitor panels. The semantic `*Msg` stream infrastructure is in place for Sky View and Signal View in phase 6.

## Testing (Playwright)

Testing the semantic stream requires a connected receiver. Tests split into two categories.

### Without hardware (UI structure)
- Verify Time panel exists in the layout and shows placeholder or empty state.
- Verify Survey panel exists and shows placeholder or empty state.
- Verify receiver info area exists (in connection strip or separate panel).
- Verify panel layout accommodates the new panels without breaking resize behavior.

### With hardware (live data, manual or CI with hardware)
- Connect to a receiver.
- Verify Time panel updates with UTC time within a few seconds.
- Verify Time panel shows local time, UTC, and TAI fields.
- Verify Survey panel populates if the receiver is in survey mode.
- Verify receiver info shows vendor/hardware/firmware after detect.
- Disconnect and verify panels show stale data or empty state (not errors).

## Files changed
- `desktop/app.go` (decoder goroutine, MsgEvent DTOs, Connect/Disconnect changes)
- `desktop/frontend/src/app.tsx` (gps:msg event subscription, state for each msg kind)
- `desktop/frontend/src/time-panel.tsx` (new)
- `desktop/frontend/src/survey-panel.tsx` (new)
- `desktop/frontend/src/timefmt.ts` (new, ported from web/)
- `desktop/frontend/src/receiver-panel.tsx` or `connection-panel.tsx` (receiver info moved)
