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

The daemon owns the subscription lifecycle.  It should keep the bcast
pointer and call `Unsubscribe` after `Dispatcher.Run` returns.  The
subscription must be created before `startStream`; subscribing after
`Pull.Run` has exited returns a closed channel.

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
2. Add `CorReport` to the dispatcher log event.
3. Add `UBX-RXM-COR` parsing in `ubxbin`.
4. Convert `UBX-RXM-COR` to `CorReportMsg` in the u-blox packet
   processor.
5. Add Unicore `RTCMSTATUS` parsing and conversion if Unicore receiver
   status is included in this implementation.
6. Add subtype-aware RTCM message ID support in `rtcmbin`, including
   4072 u-blox proprietary subtype extraction.
7. Update `gps/internal/rtcm.packetFormat.MsgID`,
   `gps/internal/rtcm.PacketProcessor.ProcessPacket`, and the RTCM
   pull-report helper to use the new `rtcmbin` MsgID helper.
8. Add `PullSetup.Bcast()` and subscribe the dispatcher before stream
   pull starts.
9. Add the pulled-packet channel to `Dispatcher.Run` and dispatch pull
   reports through the same `gpsprot.MsgHandler` fan-out used for packet
   processors.
10. Log invalid correction checksums in `stream.pull`.
11. Add tests for source marshaling, handler fan-out, UBX-RXM-COR
    conversion, RTCM pull-report conversion, and dispatcher pull-channel
    close handling.  Add RTCMSTATUS conversion tests if Unicore support
    is included.

## Follow-ons

These are not part of this plan.

- Make existing observers consume `CorReportMsg`.
- Enable `UBX-RXM-COR` through `ConfigOpts` where needed.
- Add a protocol-independent correction-report correlator if needed.
- Enrich the generic `NativeMsg` observer path with packet length if
  receiver-output RTCM observers need it.
