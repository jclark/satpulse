# Unified event model (#230)

Establish a single event envelope (`gpsprot.Event`) used across all transports: event log files, SSE to the web frontend, and a new JSONL HTTP endpoint for external consumers. Replace the per-field-per-type `LogEvent` struct and the `*SSE` intermediary types with one model.

An additional advantage: the frontend imports gpsprot types from `@satpulse/gps` (`gps/ts/`), which are validated against actual Go JSON output by a drift detection test (`gps/ts/gen_test.go`). If a Go type's JSON serialisation changes, `make test` fails until the TypeScript interfaces are updated. The `*SSE` intermediary types had no such validation — mismatches between Go and frontend were only caught at runtime.

## Prerequisite: gpsprot type cleanup

A few gpsprot types still need cleanup before they serialise cleanly:

### SurveyMsg geodetic fields

`SurveyMsg` has only ECEF `Position` (`Point3D`). The `SurveySSE` layer currently computes lat/lon/alt from ECEF via `geopos.WGS84.ECEFtoLLH`. Move this computation into the protocol handler that populates `SurveyMsg`:

- Add `LatLon opt.Val[[2]Angle]` and `Height opt.Val[Length]` to `SurveyMsg`.
- The protocol handler computes and fills geodetic fields from ECEF.
- Make `ObsTime` and `ObsCount` optional (`opt.Val`) -- different protocols provide different subsets.

### LeapSecondMsg cleanup

`LeapSecondMsg` embeds `ptime.LeapSecond` which does not serialise cleanly: `ptime.Time` fields render as raw `int64`, and the embedded fields lack JSON tags. More broadly, GNSS satellites transmit leap second info as part of a time-correction block that describes how UTC relates to the GNSS system time (equivalent to `ptime.CorrectionParams`). The cleanup should consider generalising `LeapSecondMsg` to cover this GNSS-to-UTC conversion as a whole, not just the leap second. Details to be fleshed out.

### Replace pointer optionality with `opt.Val`

- `TimeMsg.UTCTime *ptime.UTCTime` -> `opt.Val[ptime.UTCTime]`
- `TimeMsg.PulseOffset *float64` -> `opt.Val[float64]`
- `SVInfo.LookAngles *LookAngles` -> `opt.Val[LookAngles]`

Update all producers and consumers (including `TimeMsg.Merge`, `ComputeTAITime`).

## Event envelope

Add `MsgType() string` to the existing `Msg` interface:

```go
type Msg interface {
    Dispatch(MsgHandler, time.Time)
    MsgType() string
}
```

Each concrete type returns its canonical name: `PosGeoMsg.MsgType()` returns `"posGeo"`, `TimeMsg.MsgType()` returns `"time"`, etc. These names match the SSE event names and the JSON `type` field.

Add to `gpsprot`:

```go
// Event is the universal envelope for all gpsprot messages.
type Event struct {
    Type string    `json:"type"`
    T    time.Time `json:"t"`
    Mono Duration  `json:"mono"`
    Data Msg       `json:"data"`
}
```

- `Type` -- message type name, returned by `Msg.MsgType()`.
- `T` -- wall-clock time of the system clock when the event was read. For `TimeMsg` events, the difference `T - epochTime(TAITime/UTCTime)` gives the wall-clock delay from navigation epoch to message receipt, enabling coarse estimation of system clock error. Combined with #198's delay fields, this supports serial-stream time synchronization.
- `Mono` -- monotonic elapsed time since session start as `gpsprot.Duration` (serialises as decimal seconds), rounded to microseconds. Independent of wall clock.
- `Data` -- the concrete `Msg` value. Marshalling works as-is (Go sees the concrete type). Unmarshalling uses a custom `Event.UnmarshalJSON` that decodes `type` first, looks up the concrete type in a registry, then unmarshals `data` into it.

### Unmarshal registry

A package-level map from type name to factory function:

```go
var msgRegistry = map[string]func() Msg{
    "posGeo":     func() Msg { return &PosGeoMsg{} },
    "time":       func() Msg { return &TimeMsg{} },
    // ...
}
```

`Event.UnmarshalJSON` decodes into a helper struct with `Data json.RawMessage`, switches on `Type`, and unmarshals `Data` into the concrete type from the registry.

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

The SSE time handler uses `TimeTicker` to produce one filled `TimeMsg` per epoch with integral-second filtering, then sends it directly. The frontend computes UTC from `TAITime` + `UTCOffset` via `timefmt.ts`.

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

## Transports

All three transports carry the same `Event` content with transport-specific framing.

### Event log (`.jsonl` files)

One `Event` JSON object per line, every raw message as received. No filtering or bundling. Replaces the current `LogEvent` struct in `gpsevent/dispatcher.go`.

Before:
```json
{"t":"...","nanos":1234567890,"posGeo":{"latLon":[...]}}
```

After:
```json
{"type":"posGeo","t":"...","mono":1.234567,"data":{"latLon":[...]}}
```

### SSE (`/sse` endpoint)

`Event.Type` maps to the SSE `event:` field. `Event.Data` is JSON-serialised as the SSE `data:` field. The `t` and `mono` envelope fields are omitted -- the frontend doesn't need them.

SSE applies processing before sending:
- **TimeTicker** -- one filled `TimeMsg` per nav epoch (integral-second filtering).
- **PVMsgBundle** -- accumulates position/velocity messages per epoch into a single `posvel` event.

These are SSE-specific presentation concerns, not part of the base event model.

### JSONL HTTP endpoint (`/events`)

Streams `Event` objects as JSON Lines over plain HTTP. Same envelope as event log files but live.

Query parameters select optional processing:
- `?tick` -- pass `TimeMsg` events through `TimeTicker` (one per epoch, filled fields) instead of raw time messages.
- `?pvbundle` -- emit `PVMsgBundle` events instead of individual `PosGeo`/`PosECEF`/`VelGeo`/`VelECEF` messages.

Without query parameters, the stream is the same as the event log: every raw message.

## Event log migration

### Dispatcher update

Replace `LogEvent` in `gpsevent/dispatcher.go` with `gpsprot.Event`. The `logEvent()` method constructs an `Event` with `MsgType()`, wall-clock time, and monotonic duration (microsecond-rounded `gpsprot.Duration` since session start).

### Replay code

Update `gpsevent/replay.go` to unmarshal `gpsprot.Event` using the custom `UnmarshalJSON`. The monotonic `Mono` field replaces `Nanos` for reconstructing replay timing.

Update `gpsevent/migrate_log.go` to handle migration from the old `LogEvent` format (one-field-per-type with `nanos` as integer nanoseconds) to the new `Event` format.

### Test data

Regenerate test data files that use the `LogEvent` format:
- `time/internal/gpsevent/testdata/fast.jsonl` -- used by `TestReplayFast()`
- `time/internal/promobs/testdata/session.jsonl` -- used by `TestReplay()` with `readLogEvents()`

Update `promobs/prometheus_replay_test.go` `readLogEvents()` to unmarshal `gpsprot.Event`.

## Replay dev server

After the SSE backend sends gpsprot types directly, event log `.jsonl` files contain exactly the shapes the frontend consumes. Build a dev server that replays recorded event logs as SSE, enabling frontend iteration without GPS hardware.

The replay server reads `.jsonl` event log files and serves events as SSE, replayed at original timing (using the `mono` field) or at accelerated speed. Since both event log and SSE now use `gpsprot.Event`, replay is a direct translation: read line, unmarshal `Event`, map to SSE framing, send.

### Playwright integration

The Playwright MCP server can navigate to the dev server URL and take screenshots, giving the same visual feedback loop as the Wails desktop app.

## Verify

- `gpsprot.Event` marshals and unmarshals correctly for all message types (round-trip test)
- Event log files use the new envelope format
- Web dashboard works identically with all card types (position, velocity, quality, survey, time, satellites, PHC, receiver)
- Fields gracefully absent when the receiver or fix type doesn't provide them
- `/events` JSONL endpoint streams raw events; `?tick` and `?pvbundle` query params apply processing
- Replay dev server plays back a recorded `.jsonl` and the dashboard displays correctly
- Existing tests pass with regenerated test data
