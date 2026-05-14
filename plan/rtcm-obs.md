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

Add an API in `gps/internal/rtcm` that converts an RTCM packet into a
`*gpsprot.CorReportMsg` for the pull path.  This keeps packet
length, checksum status, reference station extraction, and `rtcmbin.Msg`
parsing out of the dispatcher.

The dispatcher should emit both pull-derived and receiver-derived
reports as separate `CorReportMsg` observations.  It should not
try to combine them or prefer one source over the other.  Correlation is
protocol-independent observer policy, because both sides have the same
`gpsprot` representation.

Add `CorReport` to `time/internal/gpsevent.LogEvent` so reports
are visible in the JSONL event log like other `gpsprot.Msg` values.

## SSE and Web Dashboard

Surface RTCM correction traffic in the web dashboard through a dedicated
RTCM SSE event and card.  This is intentionally a presentation path, not
a correlator.

Add an RTCM-specific SSE event in `time/internal/sseobs`:

```go
type RTCMSSE struct {
	MsgID  string                      `json:"msgID"`
	Source gpsprot.CorReportSource     `json:"source"`
	Used   opt.Val[bool]               `json:"used,omitzero"`
}
```

`SSEObserver.CorReport` should filter `CorReportMsg` before sending:

- Require `Tag == RTCM`.
- Require a non-empty, subtype-aware `MsgID`.
- Exclude explicitly invalid packets: when `ChecksumOK` is set and
  false, emit no SSE event.
- Do not include `NativeMsg` in the SSE payload.

Emit the filtered report as SSE event name `rtcm`.  Keep the payload
small: message ID, source, and optional used status are enough for the
first pass.

The SSE observer should prefer receiver-source reports over pull-source
reports.  It starts in pull mode and emits valid pull reports so the UI
can show that corrections are arriving from the configured source.  Once
it observes any valid `CorReportSourceReceiver` RTCM report, it switches
to receiver mode and suppresses all subsequent pull-source reports.  The
receiver report that caused the switch is emitted.  Do not switch back
to pull mode during the daemon session; this avoids mixing two views of
the same correction stream and keeps the dashboard counts easy to
interpret.

Receiver mode is the better user-facing signal because it describes
what the receiver reports about correction input, not merely what
`stream.pull` downloaded.  Be precise about wording: call the receiver
view `RTCM messages used` only when the UI is counting receiver reports
with `Used == true`.  If a receiver-source producer does not provide
used/not-used status, title the card `RTCM messages reported by
receiver`.

In `web/dashboard.tsx`:

- Add `rtcm` to the subscribed event types.
- Maintain RTCM counts in the frontend, separate from the existing
  latest-event state.  Track the current RTCM view (`pull` or
  `receiver`) and a map from `msgID` to count.
- On each valid pull-source `rtcm` event, increment that message ID's
  count while the UI is still in pull mode.
- On the first receiver-source `rtcm` event, switch the UI to receiver
  mode and clear any pull-mode counts so the card never mixes source
  observations with receiver observations.
- In receiver mode, increment counts for receiver-source events.  If
  the event includes `used: false`, do not increment the `used` count;
  that event still establishes receiver mode.
- Add an RTCM dashboard card with one row per message ID and the count
  observed since the page connected.  Use a header that reflects the
  current view, for example `RTCM messages received` in pull mode and
  `RTCM messages used` in receiver mode when counting `used: true`
  events.
- Sort rows by numeric message ID where possible, with subtype IDs such
  as `4072.1` sorted naturally after `4072.0`.
- Do not add backend aggregation, persisted counters, frequency
  estimates, or reconnect recovery for this pass; a page reload or new
  SSE connection starts the dashboard counts from zero.

Add focused tests:

- `SSEObserver.CorReport` emits `event: rtcm` for a valid RTCM
  `CorReportMsg`.
- It emits nothing for non-RTCM reports, empty message IDs, and
  explicitly failed checksums.
- It suppresses pull-source reports after the first valid receiver-source
  report.
- The dashboard test EventSource can send multiple `rtcm` events and
  the rendered RTCM card shows the expected per-message counts.
- The dashboard resets pull-mode counts when the first receiver-source
  event arrives and updates the card header to match the receiver view.

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
   `gps/internal/rtcm` API that converts an RTCM packet into a
   `*gpsprot.CorReportMsg`; expose `PullSetup.Bcast()`; subscribe
   the dispatcher before stream pull starts; take the subscribed
   channel in `Dispatcher.Run`; and emit pull-source `CorReportMsg`
   through the existing `gpsprot.MsgHandler` fan-out.
7. Log invalid correction checksums in `stream.pull`.
8. Add the `rtcm` SSE event in `time/internal/sseobs`, filtered to
   valid RTCM reports with non-empty message IDs.
9. Add the `RTCM` dashboard card in `web/`, maintaining per-message-ID
   counts in frontend state.
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
