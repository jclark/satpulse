 # Correction report observability (#237)

Add correction observability for `satpulsed` as a protocol-independent
`gpsprot.Msg`.  The initial producers are:

- `stream.pull`, which reports correction packets received from the
  configured network source.
- u-blox `UBX-RXM-COR`, which reports correction input status from the
  receiver.

Unicore `RTCMSTATUS` also fits this model as another receiver-status
producer, although it populates fewer common fields.

This plan is for issue #237.

## Message Shape

Add a new `gpsprot.Msg`:

```go
type CorReportSource uint8

const (
	CorReportSourcePull CorReportSource = iota
	CorReportSourceReceiver
)

type CorReportMsg struct {
	Source CorReportSource `json:"source"`

	Tag   Tag    `json:"tag"`
	MsgID string `json:"msgID"`

	NativeMsg any `json:"nativeMsg,omitempty"`

	NBytes        opt.Val[int]    `json:"nBytes,omitzero"`
	ChecksumOK    opt.Val[bool]   `json:"checksumOK,omitzero"`
	Used          opt.Val[bool]   `json:"used,omitzero"`
	RTCMRefBaseID opt.Val[uint16] `json:"rtcmRefBaseID,omitzero"`
}
```

`CorReportSource` should have `String()` values `pull` and
`receiver`, and `MarshalText()` should be implemented in terms of
`String()`.

`CorReportMsg` should implement `gpsprot.Msg`:

- `MsgType()` returns `corReport`.
- `Dispatch` calls `MsgHandler.CorReport`.

Add a small interface for consumers that only need correction reports:

```go
type CorReporter interface {
	CorReport(msg *CorReportMsg, tRead time.Time)
}
```

Embed `CorReporter` in `MsgHandler`.  This keeps correction
reports on the normal `gpsprot.Msg` path while allowing a future
protocol-independent correlator to consume only correction reports.

Update `DefaultHandler`, `GenericHandler`, and `MultiHandler` for
`CorReport`.

## Semantics

`CorReportSourcePull` means `stream.pull` scanned a correction
packet from the network source.  It does not mean the packet was used by
the receiver.

For pulled RTCM reports:

- `Tag` is `RTCM`.
- `MsgID` is the RTCM message ID string, e.g. `1077` or `4072.1`.
- `NBytes` is the complete packet length.
- `ChecksumOK` is always set.
- `NativeMsg`, when present, is the parsed `rtcmbin.Msg`.

`CorReportSourceReceiver` means the receiver reported correction
input status.  It does not mean the receiver emitted an RTCM packet.

For u-blox `UBX-RXM-COR` reports:

- Convert only the correction protocol into the existing `gpsprot.Tag`
  space; initially this is `RTCM`.
- `MsgID` uses the same RTCM message ID formatting as pull reports,
  including the subtype for proprietary messages such as `4072.1`.
- `Used` maps `msgUsed`: unknown is unset, not used is `false`, used is
  `true`.
- `ChecksumOK` maps RTCM `errStatus`: unknown is unset, error-free is
  `true`, erroneous is `false`.
- `RTCMRefBaseID` maps the RTCM reference station ID when present.
- Do not put the `UBX-RXM-COR` struct into `NativeMsg`; for `Tag ==
  RTCM`, non-nil `NativeMsg` is an `rtcmbin.Msg`.

For Unicore `RTCMSTATUS` reports:

- `Source` is `CorReportSourceReceiver`.
- `Tag` is `RTCM`.
- `MsgID` uses the same RTCM message ID formatting as other reports
  when the available message fields support it.
- `RTCMRefBaseID` maps the Base ID field.
- Leave `NBytes`, `ChecksumOK`, `Used`, and `NativeMsg` unset.  The
  documented checksum is the Unicore log checksum, not an RTCM
  correction-message checksum, and the message does not explicitly say
  whether the receiver used the correction in its solution.

Receiver-emitted RTCM remains on the existing native-message path as
`NativeMsg(tag == RTCM, msgID, rtcmbin.Msg, tRead)`.  Do not add
`RTCMOutput`.

## Emission Path

Add `UBX-RXM-COR` support to `gps/lib/ubxbin`, limited to parsing the
message fields needed by the packet processor.

In `gps/internal/ubx.PacketProcessor.Dispatch`, convert parsed
`*ubxbin.RxmCor` into `*gpsprot.CorReportMsg` and emit it through
the configured `gpsprot.MsgHandler`.

Unicore can use the same receiver-source emission path by adding typed
`RTCMSTATUS` parsing in `gps/lib/uncmsg` and converting it in
`gps/internal/unc.packetProcessor.dispatch`.  The repository already
knows the `RTCMSTATUS` message ID/name, but unregistered messages are
currently parsed as unknown bodies.

Add subtype-aware RTCM message ID support in `gps/lib/rtcmbin`.  It
should continue to return the plain message type for normal RTCM
messages, but for RTCM 4072 u-blox proprietary messages it should
extract the subtype and format IDs such as `4072.0` and `4072.1`.

Update `gps/internal/rtcm` to use the new `rtcmbin` MsgID helper
where it currently formats IDs with `rtcmbin.ExtractMsgType(...).String()`:

- `packetFormat.MsgID`
- `PacketProcessor.ProcessPacket`

Add an API in `gps/app/stream` that converts a scanned correction-source
RTCM packet into a pull-source `*gpsprot.CorReportMsg`.  This keeps
packet length, checksum status, reference station extraction, and
`rtcmbin.Msg` parsing out of the dispatcher, while keeping the API in
the package that owns pulled correction packets.

The dispatcher should emit both pull-derived and receiver-derived
reports as separate `CorReportMsg` observations.  It should not
try to combine them or prefer one source over the other.  Correlation is
protocol-independent observer policy, because both sides have the same
`gpsprot` representation.

Add `CorReport` to `time/internal/gpsevent.LogEvent` so reports
are visible in the JSONL event log like other `gpsprot.Msg` values.

## SSE and Web Dashboard

Surface RTCM correction traffic in the web dashboard through a
correction-report SSE event and an RTCM card.  This is intentionally a
presentation path, not a correlator.

The SSE event wire format is protocol-generic (it carries `tag`) so
non-RTCM correction protocols can flow over the same event in the
future.  The dashboard is RTCM-centric: it consumes only events with
`tag == "RTCM"`.

### Backend

Add a correction-report SSE event in `time/internal/sseobs`:

```go
type CorReportSSE struct {
	Tag           gpsprot.Tag             `json:"tag"`
	MsgID         string                  `json:"msgID"`
	Source        gpsprot.CorReportSource `json:"source"`
	ChecksumOK    opt.Val[bool]           `json:"checksumOK,omitzero"`
	Used          opt.Val[bool]           `json:"used,omitzero"`
	RTCMRefBaseID opt.Val[uint16]         `json:"rtcmRefBaseID,omitzero"`
}
```

JSON field names mirror `CorReportMsg` so that when the sse-data.md
migration lands and SSE payloads become direct `*CorReportMsg`
serializations, the frontend changes minimally.

`SSEObserver.CorReport` does not filter on content (tag, msgID,
checksum); the dashboard handles that.  It does apply a source
preference: at any moment it emits events from only one source (pull
or receiver) so the dashboard never has to mix two views of correction
traffic.  Pull and receiver are independent observations and may even
describe different streams (e.g. another app could feed RTCM into a
separate serial port on the receiver), so we cannot assume one is a
subset of the other.

Receiver mode is preferred because it describes what the receiver
actually reports about correction input.  The transition rules are:

- Start in pull mode.
- pull -> receiver: any receiver-source event.  Emit the triggering
  event and update `lastReceiverTime`.
- In receiver mode, emit receiver-source events and update
  `lastReceiverTime` on each one.
- receiver -> pull: lazy.  When a pull-source event arrives, if
  `now - lastReceiverTime > T`, switch to pull and emit that event;
  otherwise drop it.  No background timer.

T should be long enough that brief gaps in UBX-RXM-COR do not flap the
mode but short enough that operator-visible config changes (e.g.
disabling UBX-RXM-COR) recover quickly.  Start with T = 30 s.

`CorReportSSE` is built from every emitted `CorReportMsg`, omitting
`NativeMsg` and `NBytes`.  Event name is `corReport`.

### Frontend

In `web/dashboard.tsx`:

- Add `corReport` to the subscribed event types.
- In `validateEvent`, drop events with `tag != "RTCM"`, empty
  `msgID`, `checksumOK === false`, or an unknown `source`.
- Accumulate per-msgID counts in an `RTCMState` class with a
  mutating `update(ev)` method.  The state holds two maps:
  `totalCount[msgID]` is the number of accepted events for the
  current source session, and `unusedCount[msgID]` counts events
  with `used === false`.  `unusedCount` is `null` until at least one
  event in the current session has carried a `used` field; once any
  event in the session reports `used` (true or false),
  `unusedCount` becomes a (possibly empty) map.
- When an event's `source` differs from the current state's source,
  `update` clears both counters and switches to the new source.
  This picks up the backend's mode switches without duplicating the
  latch logic in the frontend.
- Add an RTCM dashboard card with one row per message ID.  Row
  display:
  - `unusedCount === null` -- show the integer `totalCount[msgID]`.
  - otherwise -- show `M/N`, where `N = totalCount[msgID]` and
    `M = N - unusedCount[msgID]` (i.e. the count of `used: true`
    events for that message).
- Card title:
  - Source `pull`: `RTCM Messages Received`.
  - Source `receiver` with `unusedCount === null` (no `used` info
    ever observed, e.g. Unicore `RTCMSTATUS`): `RTCM Messages Used`.
  - Source `receiver` with `used` info available: `RTCM Messages
    Used/Received`, matching the M/N column.
- Sort rows by numeric message ID where possible, with subtype IDs
  such as `4072.1` sorted naturally after `4072.0`.
- Do not add backend aggregation, persisted counters, frequency
  estimates, or reconnect recovery for this pass; a page reload or
  new SSE connection starts the dashboard counts from zero.

When corrections first start flowing the dashboard will typically see
one or a few pull-source events before the first receiver-source event
arrives (network delivery beats UBX-RXM-COR reporting by tens to
hundreds of milliseconds), so the initial card state will briefly show
pull-counted rows before the source change clears them.  Keep the
source-change handler simple -- update the title and reset the counts
-- so this transient is not visually distracting.  Do not animate row
removal or otherwise draw attention to the reset.

`RTCMState.update` is mutating, so the SSE handler clones the
previous state (`prev.clone()`) before calling `update` to keep
React/Preact's state-identity comparison happy.

Add focused tests:

- `SSEObserver.CorReport` starts in pull mode and emits pull-source
  events.
- A receiver-source event switches the observer to receiver mode; the
  triggering event is emitted, and subsequent pull-source events
  within T are dropped.
- A pull-source event arriving more than T after the last
  receiver-source event switches the observer back to pull and is
  emitted.
- `NativeMsg` is absent from the emitted payload.
- `RTCMState.update` accumulates pull-source events by msgID and
  reports the right title and row values.
- A receiver-source event after pull-source events clears the
  counters and switches the title/format appropriately.
- Receiver-source events with `used: false` increment `totalCount`
  and `unusedCount` so the M/N display reads correctly.
- `validateEvent('corReport', ...)` rejects non-RTCM tag, empty
  message ID, failed checksum, and unknown source.
- `sortRTCMMsgIDs` orders plain and subtype message IDs numerically.

## Prometheus

Add `PrometheusObserver.CorReport` so RTCM correction observations are
also visible through `/metrics`.

Prometheus should not apply the SSE/dashboard receiver-preference rule;
export both sources with labels so operators can compare the pull-side
and receiver-side streams.

Use bounded labels:

- `satpulse_rtcm_messages_total{source,msg_id,usage}` increments for
  each RTCM `CorReportMsg` with a non-empty message ID and no explicit
  checksum failure.
- `satpulse_rtcm_checksum_errors_total{source}` increments when
  `ChecksumOK` is set and false.  Do not trust a corrupted packet's
  message ID for this error counter.

The `source` label should use `CorReportSource.String()` values
(`pull`, `receiver`).  The `usage` label is a bounded enum:

- `used` when `Used` is set and true.
- `not_used` when `Used` is set and false.
- `unknown` when `Used` is unset, including pull-source reports.

This lets operators query total valid RTCM observations by summing over
`usage`, or receiver-used traffic with `source="receiver",usage="used"`.
Do not include receiver base ID or byte length as labels in this first
metric surface; those either have higher cardinality risk or are better
suited to later gauges after there is a clear consumer.

Add Prometheus tests that exercise valid RTCM report counters, source
labels, usage labels, non-RTCM filtering, and checksum-error counting.

## Pull Packet Path

The dispatcher should receive pulled packets by subscribing to
`stream.Pull.Packets`.  Expose this from `PullSetup` with:

```go
func (s *PullSetup) Bcast() *bcast.Bcast[scan.Packet]
```

The daemon should call `pullSetup.Bcast().Subscribe()` before
`startStream`, then pass the subscribed channel to `Dispatcher.Run`.
Do not add a separate puller-to-dispatcher queue; the existing bcast
queuing is sufficient.

The daemon does not need to unsubscribe this dispatcher subscription.
This is a daemon-lifetime subscriber, analogous to the existing
receiver packet subscription used by the dispatcher.  `Unsubscribe` is
needed for dynamic subscribers that can go away while the bcast remains
live, such as HTTP SSE, proxy, or NTRIP client streams; otherwise the
bcast would retain a non-reading subscriber.  Here `Pull.Run` owns the
bcast and calls `Bcast.Close()` when it exits, which closes the
dispatcher subscription channel.  The subscription must be created
before `startStream`; subscribing after `Pull.Run` has exited returns a
closed channel.

`Dispatcher.Run` should take the subscribed pulled-packet channel as a
third input.  Its loop condition should include this channel, and its
select should nil the channel when it closes.  `Pull.Run` calls
`Bcast.Close()` when it exits, so the dispatcher subscription closes
naturally on stream-pull shutdown.

Do not require checksum-valid packets for the dispatcher channel.  A
pull report can carry `ChecksumOK=false`, so observability does not
depend on dropping the packet.  `stream.pull` should log invalid
correction checksums when it sees them.

The dispatcher should ignore non-RTCM packets on the pull channel for
now.

## Implementation Order

1. Add `CorReportMsg` and `CorReportSource` to
   `gpsprot`, including handler plumbing and source text marshaling.
2. Add `UBX-RXM-COR` parsing in `ubxbin`.
3. Convert `UBX-RXM-COR` to `CorReportMsg` in the u-blox packet
   processor.
4. Add `CorReport` to the dispatcher log event.
5. Use subtype-aware RTCM message IDs consistently: add a
   subtype-aware MsgID helper in `rtcmbin` (including 4072 u-blox
   proprietary subtype extraction), and switch existing
   `gps/internal/rtcm` call sites (`packetFormat.MsgID`,
   `PacketProcessor.ProcessPacket`) to use it.
6. Wire pulled correction packets into the dispatcher: add the
   `gps/app/stream` API that converts a scanned correction-source RTCM
   packet into a pull-source `*gpsprot.CorReportMsg`; expose `PullSetup.Bcast()`; subscribe
   the dispatcher before stream pull starts; take the subscribed
   channel in `Dispatcher.Run`; and emit pull-source `CorReportMsg`
   through the existing `gpsprot.MsgHandler` fan-out.
7. Log invalid correction checksums in `stream.pull`.
8. Add the `corReport` SSE event in `time/internal/sseobs`.
   `SSEObserver.CorReport` applies the source-preference latch (start
   in pull; switch to receiver on any receiver-source event; lazy
   switch back to pull on a pull-source event after T seconds of
   receiver silence) and emits the chosen events as `CorReportSSE`
   under event name `corReport`.  No content filtering.
9. Add the `RTCM` dashboard card in `web/`, filtering `corReport`
   events to `tag == "RTCM"` with valid checksum and non-empty
   `msgID`, and maintaining per-message-ID counts that reset whenever
   an event's `source` differs from the previous one.
10. Add Prometheus RTCM counters for valid reports and checksum
    failures.
11. Add tests for source marshaling, handler fan-out, UBX-RXM-COR
    conversion, RTCM pull-report conversion, dispatcher pull-channel
    close handling, SSE filtering, dashboard counting, and Prometheus
    metrics.
12. Add Unicore `RTCMSTATUS` parsing and conversion.

## Follow-ons

These are not part of this plan.

- Enable `UBX-RXM-COR` through `ConfigOpts` where needed.
- Add a protocol-independent correction-report correlator if needed.
- Enrich the generic `NativeMsg` observer path with packet length if
  receiver-output RTCM observers need it.
- Add web-dashboard frequency estimates, source splits, persisted counts,
  or invalid-packet displays if the simple RTCM count card proves too
  limited.
