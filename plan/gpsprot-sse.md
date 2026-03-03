# Direct gpsprot serialisation

Eliminate the intermediary `*SSE` type layer in `sseobs`. The SSE backend serialises gpsprot `*Msg` types directly, and the frontend handles their nested structure. Finishes with a replay dev server that validates the pipeline end-to-end.

An additional advantage: the frontend imports gpsprot types from `@satpulse/gps` (`gps/ts/`), which are validated against actual Go JSON output by a drift detection test (`gps/ts/gen_test.go`). If a Go type's JSON serialisation changes, `make test` fails until the TypeScript interfaces are updated. The `*SSE` intermediary types had no such validation — mismatches between Go and frontend were only caught at runtime.

## Prerequisite: remaining msg-json.md items

A few gpsprot types still need cleanup before they serialise cleanly:

### SurveyMsg geodetic fields

`SurveyMsg` has only ECEF `Position` (`Point3D`). The `SurveySSE` layer currently computes lat/lon/alt from ECEF via `geopos.WGS84.ECEFtoLLH`. Move this computation into the protocol handler that populates `SurveyMsg`:

- Add `LatLon opt.Val[[2]Angle]` and `Height opt.Val[Length]` to `SurveyMsg`.
- The protocol handler computes and fills geodetic fields from ECEF.
- Make `ObsTime` and `ObsCount` optional (`opt.Val`) -- different protocols provide different subsets.

### Replace pointer optionality with `opt.Val`

- `TimeMsg.UTCTime *ptime.UTCTime` -> `opt.Val[ptime.UTCTime]`
- `TimeMsg.PulseOffset *float64` -> `opt.Val[float64]`
- `SVInfo.LookAngles *LookAngles` -> `opt.Val[LookAngles]`

Update all producers and consumers (including `TimeMsg.Merge`, `ComputeTAITime`).

## Remove `*SSE` types from sseobs

Replace each intermediary type with direct serialisation of the gpsprot type:

### `PosVelSSE` -> `PVMsgBundle`

The `NavEpoch` handler already calls `PVMsgBundle.FillDerived()`. Instead of building a `PosVelSSE` from the bundle, send the `PVMsgBundle` directly as the `posvel` event. The frontend receives nested `posGeo`, `posECEF`, `velGeo`, `velECEF` sub-objects (each omitted when not available).

### `QualitySSE` -> `NavEpochMsg`

Send `NavEpochMsg` directly as the `quality` event. The frontend receives nested `acc` and `dop` structs, `fixLevel`/`fixDim` as strings, `correction` as a string array of leaf names, `signalsUsed` as `{GNSS: [signal, ...]}`, and `diffAge` as a decimal seconds number (via `gpsprot.Duration`).

Remove `buildFixKeywords` -- fix keyword construction moves to the frontend.

### `SurveySSE` -> `SurveyMsg`

Send `SurveyMsg` directly. After the geodetic fields are added (above), it contains everything `SurveySSE` had.

### `TimeSSE` -> `TimeMsg`

The time handler keeps its filtering (skip non-integral seconds), but sends the filled `TimeMsg` directly instead of constructing a `TimeSSE` with a pre-formatted UTC string. The frontend computes UTC from `TAITime` + `UTCOffset` via `timefmt.ts`.

### `SatellitesSSE` -> `SatellitesMsg`

Trivial -- `SatellitesSSE` was already just `{ svs: []SVInfo }`. Send `SatellitesMsg` directly (which has `Info []SVInfo` -- adjust field name in frontend).

### `InitSSE` and `SampleSSE`

`InitSSE` wraps `ReceiverInfo` -- send `ReceiverInfo` directly (or keep the wrapper if the init event needs to carry additional fields in future).

`SampleSSE` is derived from `phcsync.Sample` which is not a gpsprot type. Keep it as-is or inline the conversion.

## Frontend: path-based field mapping

The gpsprot types are defined in `@satpulse/gps` (`gps/ts/`), which the `webui/` workspace depends on via `file:../gps/ts` (see `web-toolchain.md`). Frontend code imports types from `@satpulse/gps/gpsprot`.

The gpsprot types have nested structures (`Point3D`, `Accuracy`, `DOP`, sub-messages in `PVMsgBundle`) that don't fit the current `EventFormat` model, which assumes top-level keys map 1:1 to presentation rows.

Extend `EventFormat` / `addFields` so each format entry can specify a path (as a list of field names) to reach the value in the message. For example:

- `["posGeo", "latLon"]` for position coordinates
- `["acc", "hor"]` for horizontal accuracy
- `["dop", "pos"]` for PDOP

When path resolution returns undefined (sub-message absent), the row is omitted.

### Specific card updates

- **Position card**: paths into `posGeo` (latLon, height, heightMSL) and `posECEF` (pos array)
- **Velocity card**: paths into `velGeo` (groundSpeed, speed3D, course, velNED) and `velECEF` (vel array)
- **Quality card**: paths into `acc` and `dop` nested structs, plus top-level `fixLevel`, `fixDim`, `correction`, `numSVUsed`, etc.
- **Survey card**: `position` as `Point3D` (3-element array) for ECEF rows, plus new `latLon` and `height` optional fields
- **Time card**: receives `TimeMsg` with `taiTime` and `utcOffset` instead of a pre-formatted `utc` string. Frontend formats UTC from these via `timefmt.ts`.

## Replay dev server

After the SSE backend sends gpsprot types directly, event log `.jsonl` files contain exactly the shapes the frontend consumes. Build a dev server that replays recorded event logs as SSE, enabling frontend iteration without GPS hardware.

### Event log format

`satpulsed` records event logs in JSON Lines format where each line is a `LogEvent` containing gpsprot `*Msg` types directly:

```go
type LogEvent struct {
    T          time.Time              `json:"t"`
    Nanos      time.Duration          `json:"nanos"`
    Time       *gpsprot.TimeMsg       `json:"time,omitempty"`
    PosGeo     *gpsprot.PosGeoMsg     `json:"posGeo,omitempty"`
    VelGeo     *gpsprot.VelGeoMsg     `json:"velGeo,omitempty"`
    Satellites *gpsprot.SatellitesMsg `json:"satellites,omitempty"`
    NavEpoch   *gpsprot.NavEpochMsg   `json:"navEpoch,omitempty"`
    Survey     *gpsprot.SurveyMsg     `json:"survey,omitempty"`
}
```

### Dev server setup

The dashboard Vite dev server (`npm run dev`) serves the dashboard at `localhost:5173`. A replay script reads a `.jsonl` event log and serves events as SSE, replayed at original timing (using the `nanos` field) or at accelerated speed.

### Playwright integration

The Playwright MCP server can navigate to the dev server URL and take screenshots, giving the same visual feedback loop as the Wails desktop app.

## Verify

- Web dashboard works identically with all card types (position, velocity, quality, survey, time, satellites, PHC, receiver)
- Fields gracefully absent when the receiver or fix type doesn't provide them
- Replay dev server plays back a recorded `.jsonl` and the dashboard displays correctly
