# RTCM observability (#237)

Add RTCM observability for `satpulsed` using the existing observer
fan-out in `time/internal/obs`.  This is packet observability, not a
new protocol-independent GPS solution message.

This plan is for issue #237.

RTCM packets can be observed in two directions:

- **Pulled RTCM:** `stream.pull` receives RTCM from a TCP or NTRIP
  source.  This is the correction feed being pulled from the network.
- **RTCM output:** the receiver emits RTCM on its output stream,
  typically when acting as a base station.

Add a dedicated observer callback only for pulled RTCM.  Receiver RTCM
output already goes through the generic native-message observability
path as `tag == RTCM`, with `msg` carrying the parsed `rtcmbin.Msg`.
Do not introduce a generic `gpsprot.RTCMMsg`, `RTCMSource`, source
enum, or `RTCMOutput` observer callback.

## RTCMPulled

`RTCMPulled` means:

> `stream.pull` received an RTCM packet from the configured network
> source and the packet passed RTCM checksum validation.

It does not mean the packet was written to the receiver, accepted by
the receiver, or used in the receiver's solution.

### Observer Interface

Add to `time/internal/obs/observer.go`:

```go
RTCMPulled(msg rtcmbin.Msg, msgID string, nBytes int, tRead time.Time)
```

where:

- `msg` is the parsed RTCM message.
- `msgID` is the cheap display/label string from the packet format,
  e.g. `"1077"`.
- `nBytes` is the length of the complete RTCM packet in bytes.
- `tRead` is the time the network scanner read the packet.

Update `DefaultObserver` with a no-op method and `MultiObserver` with
fan-out, following the existing `Tick`, `NavEpochPV`, and `NTPSample`
patterns.

### Stream Pull Filtering

`stream.pull` already scans packets and records checksum validity in
`scan.Packet.ChecksumValid`.  The pruning queue should explicitly drop
invalid checksum packets before they can be enqueued or written.

Log checksum failures at `Info` level:

```go
lg.Info("correction packet has invalid checksum",
    "tag", pkt.Format.Tag(),
    "msg", pkt.Format.MsgID([]byte(pkt.Data)),
    "len", len(pkt.Data),
)
```

Only packets with `Format != nil` and `ChecksumValid == true` should
be reported as `RTCMPulled`.  Packets dropped by the pruning queue
should be logged.

### Packet Path

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

`stream.Pull` should not parse RTCM itself.  The dispatcher parses the
packet with `rtcmbin.ParseMsg` and calls `obs.RTCMPulled`.

This keeps `stream.Pull` focused on network read, validation, queueing,
and serial write mechanics.  It also ensures RTCM parsing happens in
one place before observer fan-out, rather than inside each observer.

The dispatcher should treat parse errors as unexpected but non-fatal:
log the packet problem and do not call `RTCMPulled` for that packet.

## Implementation Order

1. Add `RTCMPulled` to `obs.Observer`, `DefaultObserver`, and
   `MultiObserver`.
2. Add checksum filtering and `Info` logging for invalid correction
   packets in `stream.Pull`.
3. Add `PullSetup.Bcast()` and have the daemon subscribe the dispatcher
   to `stream.Pull.Packets` before starting stream pull.  The daemon
   should unsubscribe after `Dispatcher.Run` returns.
4. Add the pulled-packet channel to `Dispatcher.Run` and handle channel
   close by niling it in the select loop.
5. Parse pulled RTCM packets in the dispatcher and call
   `obs.RTCMPulled`.
6. Add tests for observer fan-out, invalid-checksum drop/logging, and
   dispatcher parsing for `RTCMPulled`.

## Follow-ons

These are not part of this plan.

- Make existing observers use `RTCMPulled` and native `RTCM` messages.
- Enrich the generic `NativeMsg` observer path to include packet
  length, if receiver-output RTCM observers need it.
- Add protocol-specific correction-input status observers, including
  correlating `RTCMPulled` with u-blox RXM correction-input status in
  `UBXLogObserver`.
