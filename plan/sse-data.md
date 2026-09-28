# SSE data (#278)

Make SSE `data:` payloads the JSON serialization of the relevant daemon
observability bus types. SSE `event:` names remain transport framing; the
goal is to remove web-specific GPS DTOs where the daemon already has a
canonical observability payload.

The main motivation is React component sharing with the `desktop-gui` branch.
The desktop GUI already feeds GPS payloads directly from the GPS subsystem into
the UI; the web UI currently feeds SSE-specific GPS DTOs. The transport frames
can differ, but the React components should receive the same payload shape from
both transports. The existing `gps/ts` drift-test contract is a secondary
benefit: it gives that shared payload shape generated TypeScript types checked
against actual Go JSON output.

This plan depends on [gpsprot-json.md](gpsprot-json.md), which handles the
transport-independent JSON cleanup for `gpsprot.Msg` payloads.

Phase 2 changes the backend to emit gpsprot payloads over SSE and changes the
frontend to consume them. Phase 3 covers non-GPS payloads.

The daemon event log and the future JSONL socket API are related, but they are
covered by [event-log-format.md](event-log-format.md), not by these phases. The
replay dev server remains as an SSE follow-on below.

## Event categories

These names describe disjoint event categories. Transports such as SSE, Wails,
JSONL files, and the future socket API are a separate axis. Consumers are a
transport plus a selected sum of these categories.

- **GPS Message Events**: `TimeMsg`, `PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg`, `LeapSecondMsg`, `SurveyMsg`, `SatellitesMsg`, `NavEpochMsg`
- **GPS Derived Events**: `TimeTicker` tick, `PVMsgBundle`
- **Pulse Edge Events**: `PulseEdge`
- **PHC Sync Events**: `Sample`, `ModeChanged`
- **NTP Refclock Events**: `NTPSample`
- **Native GPS Protocol Events**: `NativeMsg`
- **Init Session Events**: `InitSSE`

`NTPSample` is not currently provided to the Web UI; issue #274 tracks adding
it.

For the GPS SSE payload work, the web SSE stream keeps its existing SSE
framing, but its GPS payloads should be the same GPS observability payloads the
daemon already uses: `gpsprot` message types for GPS Message Events and
`gpsprot` derived types for GPS Derived Events.

The frontend imports gpsprot types from `@satpulse/gps` (`gps/ts/`), which are
validated against actual Go JSON output by a drift detection test
(`gps/ts/gen_test.go`). If a Go type's JSON serialisation changes, `make test`
fails until the TypeScript interfaces are updated. The GPS SSE payload work
should preserve that contract and remove the unvalidated web-specific GPS DTO
shapes.

## Phase 2: GPS SSE payload migration

Phase 2 has two parts:

- Backend: change `sseobs` to emit gpsprot GPS message and GPS derived payloads over SSE.
- Frontend: change the web UI to consume those gpsprot payloads.

### Backend: remove GPS `*SSE` types from `sseobs`

Replace each GPS-specific intermediary type with direct serialisation of the
corresponding daemon observability payload. The SSE observer may still decide
which events to emit and may still prepare payloads before serialization, but
it should not introduce a separate web-only GPS DTO shape.

#### `PosVelSSE` -> `PVMsgBundle`

The `NavEpoch` handler already calls `PVMsgBundle.FillDerived()`. Instead of
building a `PosVelSSE` from the bundle, send the `PVMsgBundle` directly as the
`posvel` event. The frontend receives nested `posGeo`, `posECEF`, `velGeo`,
`velECEF` sub-objects (each omitted when not available).

#### `QualitySSE` -> `NavEpochMsg`

Send `NavEpochMsg` directly as the `quality` event. The frontend receives
nested `acc` and `dop` structs, `fixLevel`/`fixDim` as strings, `correction` as
a string array of leaf names, `signalsUsed` as `{GNSS: [signal, ...]}`, and
`diffAge` as a decimal seconds number (via `gpsprot.Duration`).

Remove `buildFixKeywords` -- fix keyword construction moves to the frontend.

#### `SurveySSE` -> `SurveyMsg`

Send `SurveyMsg` directly. After the geodetic fields from
[gpsprot-json.md](gpsprot-json.md) are added, it contains everything
`SurveySSE` had. When ECEF is present and LLH is absent, `sseobs` fills the
optional LLH fields from ECEF before sending the SSE data.

This means the existing `sseobs` survey handling still needs an explicit
update: it must handle the new optional ECEF and LLH fields correctly, and it
must preserve the current display behavior by filling LLH from ECEF when that
conversion is possible.

#### `TimeSSE` -> `TimeMsg`

The SSE time handler uses `TimeTicker` to produce one filled `TimeMsg` per
epoch with integral-second filtering, then sends it directly. The frontend
computes UTC from `TAITime` + `UTCOffset` via `timefmt.ts`.

The payload type for raw `TimeMsg` and `TimeTicker` output is the same:
`TimeTicker` emits a filled `*gpsprot.TimeMsg`. The distinction is producer
semantics, not payload shape.

#### `SatellitesSSE` -> `SatellitesMsg`

Send `SatellitesMsg` directly (which has `Info []SVInfo` -- adjust field name
in frontend).

### Frontend: path-based field mapping

The gpsprot types are defined in `@satpulse/gps` (`gps/ts/`), which the web
workspace depends on via `file:../gps/ts` (see `archive/web-toolchain.md`). Frontend
code imports types from `@satpulse/gps/gpsprot`.

The gpsprot types have nested structures (`Point3D`, `Accuracy`, `DOP`,
sub-messages in `PVMsgBundle`) that don't fit the current `EventFormat` model,
which assumes top-level keys map 1:1 to presentation rows.

Extend `EventFormat` / `addFields` so each format entry can specify a path (as
a list of field names) to reach the value in the message. For example:

- `["posGeo", "latLon"]` for position coordinates
- `["acc", "hor"]` for horizontal accuracy
- `["dop", "pos"]` for PDOP

When path resolution returns undefined (sub-message absent), the row is
omitted.

#### Specific card updates

- **Position card**: paths into `posGeo` (latLon, height, heightMSL) and `posECEF` (pos array)
- **Velocity card**: paths into `velGeo` (groundSpeed, speed3D, course, velNED) and `velECEF` (vel array)
- **Quality card**: paths into `acc` and `dop` nested structs, plus top-level `fixLevel`, `fixDim`, `correction`, `numSVUsed`, etc.
- **Survey card**: `position` as `Point3D` (3-element array) for ECEF rows, plus new `latLon` and `height` optional fields
- **Time card**: receives `TimeMsg` with `taiTime` and `utcOffset` instead of a pre-formatted `utc` string. Frontend formats UTC from these via `timefmt.ts`.

## Phase 3: non-GPS payloads

Phase 3 covers SSE payloads that are not GPS Message Events or GPS Derived
Events. Keep this phase less detailed until the GPS payload work has settled.

### `SampleSSE` cleanup

`SampleSSE` is a flattened view of `phcsync.Sample`. Send `phcsync.Sample`
directly instead. Some fields need serialization cleanup (e.g. `Kind`, `Mode`
should serialise as strings), but the structure should be sent as-is rather
than flattened.

### `ModeSSE` cleanup

`ModeChanged` should ideally be represented by serialization of the phcsync
observability type used on the daemon observability bus. The current callback
is `ModeChanged(oldMode, newMode phcsync.Mode)`, so this may require a named
payload type before it can replace `ModeSSE`.

### Init session events

`InitSSE` wraps `ReceiverInfo`. It is not a gpsprot `Msg` -- it carries static
configuration data from the configuration handshake, not a decoded GPS message.
The SSE handler sends it once on connection as the first event, so the frontend
can display receiver info immediately.

Leave `InitSSE` as an init session event for now. Revisit whether this should
become a transport-neutral type after the GPS payload work.

## Follow-on: replay dev server

Build a dev server that replays recorded event logs as SSE, enabling frontend
iteration without GPS hardware.

The replay server reads `.jsonl` event log files and serves events as SSE,
replayed at original timing or at accelerated speed.

### Playwright integration

The Playwright MCP server can navigate to the dev server URL and take
screenshots, giving the same visual feedback loop as the Wails desktop app.

## Verify

Prerequisite:

- [gpsprot-json.md](gpsprot-json.md) is complete.

Phase 2:

- Web dashboard works identically with all GPS card types (position, velocity, quality, survey, time, satellites, receiver).
- Frontend code consumes gpsprot-shaped GPS payloads instead of SSE-specific GPS DTOs.
- Fields gracefully absent when the receiver or fix type doesn't provide them.
- Survey SSE data preserves current geodetic display behavior when only ECEF is provided.
- Existing tests pass.
