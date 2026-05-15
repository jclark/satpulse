# Event log format (#277)

Clean up the daemon event log JSONL format. The current `LogEvent` shape is a
sparse object with one optional field per possible event type, which is not a
natural JSON representation and is awkward to describe with a schema.

The goal is to use a natural JSON event shape with a `type` discriminator and a
`data` payload. This shape is schema friendly, is already used for decoded GPS
messages by `satpulsetool replay`, and is also suitable for a future JSON
socket interface.

This is separate from the SSE observability payload work: the event log is a
replay input capture, not daemon observability.

This plan depends on [gpsprot-json.md](gpsprot-json.md) for the stable JSON
shape of `gpsprot.Msg` payloads.

`satpulsetool replay` already emits decoded GPS messages using this envelope
shape. That format is a subset of the full daemon event log format described
here: the daemon log also needs pulse-edge records for PHC replay.

The daemon event log should use this single top-level JSON envelope shape, with
GPS message records carrying `gpsprot.Msg` payloads and pulse-edge records
carrying daemon-specific `PulseEdge` payloads. Replay must continue to have all
inputs it needs.

## Event envelope

`MsgType() string` has been added to the `Msg` interface, `Event` struct and
`NewEvent` constructor added to `gpsprot`, `GenericHandler` added, and
`NavEpochMsg.Dispatch` added. `satpulsetool replay` uses this envelope for
decoded GPS messages.

Change `gpsprot.Event` into a generic envelope and define a GPS-message
specialization:

Add `MsgType() string` to the existing `Msg` interface:

```go
type Msg interface {
    Dispatch(MsgHandler, time.Time)
    MsgType() string
}
```

Each concrete type returns its canonical name: `PosGeoMsg.MsgType()` returns
`"posGeo"`, `TimeMsg.MsgType()` returns `"time"`, etc. These names match the
SSE event names and the JSON `type` field.

Update `gpsprot`:

```go
// Event is a JSON event envelope.
type Event[T any] struct {
    Type string    `json:"type"`
    T    time.Time `json:"t"`
    Mono Duration  `json:"mono"`
    Data T         `json:"data"`
}

// MsgEvent is an Event carrying a gpsprot Msg.
type MsgEvent Event[Msg]
```

- `Type` -- message type name, returned by `Msg.MsgType()`.
- `T` -- wall-clock time of the system clock when the event was read. For `TimeMsg` events, the difference `T - epochTime(TAITime/UTCTime)` gives the wall-clock delay from navigation epoch to message receipt, enabling coarse estimation of system clock error. Combined with #198's delay fields, this supports serial-stream time synchronization.
- `Mono` -- monotonic elapsed time since session start as `gpsprot.Duration` (serialises as decimal seconds), rounded to microseconds. Independent of wall clock.
- `Data` -- the event payload.

Rename or wrap the current constructor so decoded GPS messages produce
`gpsprot.MsgEvent`:

```go
func NewMsgEvent(msg Msg, t time.Time, mono Duration) MsgEvent {
    return MsgEvent{Type: msg.MsgType(), T: t, Mono: mono, Data: msg}
}
```

`NewEvent` can remain as a compatibility wrapper if useful.

## Message registry

Keep a package-level map from type name to factory function:

```go
var msgRegistry = map[string]func() Msg{
    "posGeo":     func() Msg { return &PosGeoMsg{} },
    "time":       func() Msg { return &TimeMsg{} },
    // ...
}
```

Expose a helper that decodes a GPS message payload using the registry:

```go
func UnmarshalMsg(typeName string, data json.RawMessage) (Msg, error)
```

The daemon event-log unmarshaller uses this helper for GPS message records.
`gpsprot.MsgEvent` does not need custom unmarshalling for this plan.

## Event log `.jsonl` files

Event log records use one JSON envelope shape, one JSON object per line. No
filtering or bundling.

Before:

```json
{"t":"...","nanos":1234567890,"posGeo":{"latLon":[...]}}
```

After:

```json
{"type":"posGeo","t":"...","mono":1.234567,"data":{"latLon":[...]}}
```

The daemon event log records two kinds of payloads:

- GPS message records: `gpsprot.MsgEvent`
- Pulse-edge records: `gpsevent.LogEvent` with `type:"pulseEdge"` and `Data`
  decoded as `gpsevent.PulseEdge`

`PulseEdge` carries `ptime.Time` and `phctime.Era`, which are types from the
`time/` layer that `gpsprot` cannot import. The mixed event-log unmarshal path
therefore belongs in `gpsevent`, not `gpsprot`.

## Event log migration

Define the daemon event-log entry type as a concrete instantiation of the
generic envelope:

```go
type LogEvent gpsprot.Event[any]
```

Add custom unmarshalling for `gpsevent.LogEvent`. It decodes the top-level
`type` first:

- `type == "pulseEdge"`: unmarshal `data` as `gpsevent.PulseEdge`
- otherwise: use `gpsprot.UnmarshalMsg(type, data)` and store the resulting
  `gpsprot.Msg` in `Data`

Update `LogEvent` writing in `gpsevent/dispatcher.go`:

- GPS message records use the same envelope shape as `gpsprot.MsgEvent`.
- Pulse-edge records use `type:"pulseEdge"` with `Data` set to `PulseEdge`.

Update `gpsevent/replay.go` to unmarshal `gpsevent.LogEvent`. The monotonic
`Mono` field replaces `Nanos` for reconstructing replay timing.

Update `gpsevent/migrate_log.go` to handle migration from the old `LogEvent`
format (one-field-per-type with `nanos` as integer nanoseconds) to the new
`Event` format.

Regenerate test data files that use the `LogEvent` format:

- `time/internal/gpsevent/testdata/fast.jsonl` -- used by `TestReplayFast()`
- `time/internal/promobs/testdata/session.jsonl` -- used by `TestReplay()` with `readLogEvents()`

Update `promobs/prometheus_replay_test.go` `readLogEvents()` to unmarshal
`gpsevent.LogEvent`.

## Verify

- `gpsprot.MsgEvent` marshals correctly for all message types.
- `gpsprot.UnmarshalMsg` decodes all registered GPS message payloads.
- `gpsevent.LogEvent` unmarshals GPS message records and pulse-edge records.
- Event log files use the new envelope format for GPS message and pulse-edge records.
- Event log replay still handles pulse edges and GPS time/leap-second records correctly.
- Existing event-log replay and Prometheus replay tests pass with regenerated test data.

## Follow-on: socket API

Once the GPS payload model and event envelope are stable, expose the GPS data
model to external applications with a JSONL socket API.

### JSONL socket API

Streams `Event` objects as JSON Lines. Same envelope as event log GPS message
records, but live.

Query parameters select optional processing:

- `?tick` -- pass `TimeMsg` events through `TimeTicker` (one per epoch, filled fields) instead of raw time messages.
- `?pvbundle` -- emit `PVMsgBundle` events instead of individual `PosGeo`/`PosECEF`/`VelGeo`/`VelECEF` messages.

Without query parameters, the stream is the same as the GPS message part of the
event log: every raw message.

### `satpulsetool` event output (#215)

Once `gpsprot.Event` exists, `satpulsetool gps` can gain an option to run
packet processing after configuration and emit events as JSONL on stdout. This
supersedes the `--event-log path` option envisaged in #215 -- writing to stdout
is more composable: an application in another language can `popen`
`satpulsetool` and consume the event stream directly, without coordinating
around a file path.

Concretely: a new flag (e.g. `--events`) keeps the session open after
configuration, runs `NavEpochManager` packet processing, and writes one
`gpsprot.Event` JSON object per line to stdout. The existing `--packet-log`
writes raw packets to a file; `--events` writes decoded, structured events to
stdout. Both can be active simultaneously.

The `--show-nav` option from #215 (collect events for one epoch and display a
summary) is orthogonal and can be done separately.

Verify:

- JSONL socket API streams raw GPS message events.
- `?tick` and `?pvbundle` processing produce the expected GPS derived events.

## Appendix

### JSON Schema shape

JSON Schema can describe this event shape with `oneOf` and a `const` constraint
on the `type` field. Each branch fixes one `type` value and gives `data` the
corresponding schema.

For example:

```json
{
  "oneOf": [
    {
      "type": "object",
      "required": ["type", "t", "mono", "data"],
      "properties": {
        "type": { "const": "time" },
        "t": { "type": "string", "format": "date-time" },
        "mono": { "type": "number" },
        "data": { "$ref": "#/$defs/TimeMsg" }
      },
      "additionalProperties": false
    },
    {
      "type": "object",
      "required": ["type", "t", "mono", "data"],
      "properties": {
        "type": { "const": "pulseEdge" },
        "t": { "type": "string", "format": "date-time" },
        "mono": { "type": "number" },
        "data": { "$ref": "#/$defs/PulseEdge" }
      },
      "additionalProperties": false
    }
  ]
}
```

The full schema would have one branch for each GPS message type
(`time`, `posGeo`, `posECEF`, `velGeo`, `velECEF`, `leapSecond`, `survey`,
`satellites`, `navEpoch`) plus one branch for `pulseEdge`.
