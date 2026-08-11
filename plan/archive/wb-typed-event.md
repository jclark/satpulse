# Typed session event payloads (no issue yet)

## Goal

Replace the untyped session event envelope

    type Event struct {
        Name EventName
        Data any // one of the payload types documented on the EventName constants
    }

in `gps/app/session` with a sealed union, so that the name-payload
correspondence is code instead of comments, an emit site cannot pair
a name with the wrong payload, and consumers dispatch with a type
switch over a closed, discoverable set instead of guarded type
assertions on `any`.

This is an improvement to the session API in its own right,
independent of any particular shell: the name-payload contract is
currently enforced by nothing, and `emit(EventTime, wrongThing)`
compiles. The gap was surfaced by the TUI exercise (see
`plan/wb-tui.md` on the `wb-tui` branch), whose in-process shell consumes
payloads directly and got no compile-time help; the serializing
shells never noticed because they JSON-encode `Data` one line after
receiving it.

## Design

The `Event` struct is deleted and its name is taken by the union:

    // Event is a session event payload. The implementing types are
    // exactly the payload types listed on the EventName constants.
    type Event interface {
        EventName() EventName
    }

    type Sink interface {
        Emit(Event)
        Wants(EventName) bool
    }

`EventName` and its constants stay: the hub still needs the name for
its sticky-event cache and as the SSE event name, `Wants` still gates
on it, and the TUI sink coalesces by it. The name is now derived
from the value rather than carried beside it.

This is a pre-1.0 breaking change with no compatibility shim; every
consumer updates in the same change (see Consumers).

### Payload types

Each event's payload becomes a type in `session` implementing
`Event`. Three cases:

1. Already a session-local type: add the `EventName` method.
   `ReceiverEvent`, `LogEvent`, `MsgEvent`, `NMEAPositionEvent`,
   `MsgSendEvent`, `ResponseEvent`, `CorrEvent`, `BaseARPEvent`,
   and `ConnState` (already a defined type).

2. Bare built-in types: replace with defined types that marshal
   identically. `gps:speed` `int` becomes `type SpeedEvent int`;
   `gps:initialPos` `[2]float64` becomes
   `type InitialPosEvent [2]float64`.

3. Types owned by other packages, which cannot receive methods:
   wrap by embedding, which preserves the JSON encoding exactly:

       type TimeEvent struct{ *gpsprot.TimeMsg }
       type EpochPVTEvent struct{ gpsprot.PVMsgBundle }
       type CorrPacketEvent struct{ *gpsprot.CorReportMsg }
       type PacketEvent struct{ gpsio.PacketLogEntry }

The internal `emit(sink, name, data)` helper and every emit site in
session.go change to construct the typed value; the name argument
disappears.

`MsgEvent.Msg` stays `any` with its string `Kind`: the nine gpsprot
message types it carries are dispatched the same way one layer down
(`gpsprot.MsgHandler`), and typing that envelope is a separate
decision. Revisit only if it falls out nearly free.

## Wire compatibility

The SSE stream must be byte-identical: struct embedding marshals to
the same JSON as the embedded value, defined types marshal as their
underlying type, and the SSE event name still comes from the same
constants. The web and desktop frontends and their TypeScript are
untouched; no asset rebuild.

Guard this with a marshaling test: for each payload type, marshal
the new typed value and the value it wraps and require equality.

## Consumers

On master, two:

- `cmd/satpulsewb/sink.go` (sseHub): `ev.Name` comparisons become
  `ev.EventName()`; the payload inspections (the `MsgEvent` kind for
  sticky caching, the `StateDisconnected` cache flush, the
  `CorrEvent` base-ARP drop) become type switches. Mechanical, no
  behavior change.
- `gps/app/session` tests.

On other branches, to adapt when this merges into them:

- `wb-tui` branch (`cmd/satpulsewb/tui`): the main beneficiary.
  Handlers
  drop their `if x, ok := ev.Data.(T); ok` guards for plain type
  switches; the sink keeps coalescing by `EventName()`; test
  fixtures become typed, so invalid name/payload pairings are
  unrepresentable.
- `desktop-gui` branch: same mechanical update as the hub.

## Testing

- The wire-equality marshaling test above.
- Full `make test`; the workbench e2e suite and the smoketest wb
  scenarios confirm the SSE stream end to end.
