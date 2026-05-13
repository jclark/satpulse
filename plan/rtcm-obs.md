# RTCM observability (#237)

Add basic RTCM observability for satpulsed.  RTCM packets may enter
satpulsed from two directions:

- **Receiver direction:** the receiver emits RTCM on the main serial
  output, typically when acting as a base station.
- **Network direction:** `stream.pull` receives RTCM from a TCP or
  NTRIP source before writing it to the receiver correction input.

Both directions should produce the same `gpsprot.RTCMMsg` metadata
and call the same observer method.  They should not both be forced
through the same channel.  Receiver-direction RTCM is already on the
dispatcher goroutine via `pktCh -> ProcessPacket -> NativeMsg`, so
sending it to a dispatcher-owned `rtcmCh` would deadlock if the
channel is unbuffered and can still block with a bounded buffer.

The convergence point is a dispatcher helper such as `handleRTCM`,
not `rtcmCh`.

See `plan/rtcm-msm.md` for the future MSM satellite/signal detail
extension.

## RTCMMsg

Add to `gps/gpsprot/msg.go`:

```go
type RTCMSource int

const (
    RTCMReceiver RTCMSource = iota
    RTCMNetwork
)

// RTCMMsg carries metadata about a parsed RTCM packet.
type RTCMMsg struct {
    Source    RTCMSource      `json:"source"`
    MsgType  uint16          `json:"msgType"`
    StationID opt.Val[uint16] `json:"stationID,omitzero"`
}
```

No `Dispatch` method: this is not a `gpsprot.Msg` routed through
`MsgHandler`.  It goes through `obs.Observer`.

## Conversion function

Add to `gps/internal/rtcm`:

```go
// MakeRTCMMsg builds a gpsprot.RTCMMsg from a parsed rtcmbin.Msg.
func MakeRTCMMsg(msg rtcmbin.Msg, source gpsprot.RTCMSource) gpsprot.RTCMMsg
```

Extract `MsgType` and `StationID` from the `rtcmbin.Msg`.  Both
directions call this function:

- **Receiver direction:** `gps/internal/rtcm.PacketProcessor`
  already parses the packet via `rtcmbin.ParseMsg`.  It calls
  `MakeRTCMMsg` with `RTCMReceiver` and delivers the resulting
  `gpsprot.RTCMMsg` through `NativeMsgHandler.NativeMsg`.
- **Network direction:** `gps/app/stream` observes `Pull.Packets`,
  parses RTCM packets via `rtcmbin.ParseMsg`, calls `MakeRTCMMsg`
  with `RTCMNetwork`, and sends the resulting metadata event to the
  daemon dispatcher.

This keeps all rtcmbin-to-gpsprot conversion in `gps/internal/rtcm`
and avoids the dispatcher (in the `time` layer) needing to import
`gps/internal/` packages.

## Observer interface

Add to `time/internal/obs/observer.go`:

```go
RTCM(msg *gpsprot.RTCMMsg, tRead time.Time)
```

Update `DefaultObserver` with a no-op method and `MultiObserver`
with fan-out, following the existing `Tick`, `NavEpochPV`, and
`NTPSample` patterns.

## Event log

Add to `gpsevent.LogEvent`:

```go
RTCM *gpsprot.RTCMMsg `json:"rtcm,omitempty"`
```

## Dispatcher handling

Add a dispatcher helper:

```go
func (d *Dispatcher) handleRTCM(msg gpsprot.RTCMMsg, tRead time.Time) {
    d.obs.RTCM(&msg, tRead)
    d.logEvent(LogEvent{T: tRead, RTCM: &msg})
}
```

Receiver-direction RTCM reaches the dispatcher through the existing
native-message callback:

```go
func (d *Dispatcher) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) error {
    switch m := msg.(type) {
    case gpsprot.RTCMMsg:
        d.handleRTCM(m, tRead)
        return nil
    case *gpsprot.RTCMMsg:
        d.handleRTCM(*m, tRead)
        return nil
    }
    if !d.obs.NativeMsg(tag, msgID, msg, tRead) {
        d.lg.Debug("unused message from GPS receiver", "protocol", tag, "msgID", msgID)
    }
    return nil
}
```

This path must not send to `rtcmCh`, because it runs on the
dispatcher goroutine while `Run` is still inside the `pktCh` case.

## Network RTCM channel

Network-direction RTCM is produced by a stream goroutine, so a
channel into the dispatcher is appropriate there.

Define an RTCM event type in `gps/gpsprot`, which is usable by
both `gps/app/stream` and `time/internal/gpsevent`:

```go
// RTCMEvent pairs an RTCMMsg with its read time.
type RTCMEvent struct {
    Msg   RTCMMsg
    TRead time.Time
}
```

Add an optional RTCM event channel to `Dispatcher.Run`:

```go
func (d *Dispatcher) Run(
    tsCh <-chan ts.Event,
    pktCh <-chan scan.Packet,
    rtcmCh <-chan gpsprot.RTCMEvent,
)
```

Update the loop condition to include `rtcmCh`:

```go
for tsCh != nil || pktCh != nil || rtcmCh != nil {
```

Add to the select:

```go
case re, ok := <-rtcmCh:
    if ok {
        d.handleRTCM(re.Msg, re.TRead)
    } else {
        rtcmCh = nil
    }
```

When stream pull observability is not configured, `rtcmCh` is nil
from the start and does not affect the loop condition or select.

## Stream integration

Add a small observer adapter in `gps/app/stream`, where importing
`gps/internal/rtcm` is allowed.  The adapter subscribes to
`Pull.Packets`, filters RTCM packets, converts them to
`gpsprot.RTCMEvent`, and sends them to the channel passed by the
daemon.

The daemon creates `rtcmCh`, starts this adapter immediately before
starting `Pull.Run` and `Dispatcher.Run`, and performs no fallible
setup after that point.  Starting the adapter before `Pull.Run`
lets it subscribe before the stream reader can produce packets.  The
adapter owns the send side of the network RTCM channel and closes it
when the packet subscription ends.

Shutdown sequence:

1. Daemon cancels the stream context.
2. `Pull.Run` drains its internal pipeline and closes `Pull.Packets`.
3. The stream RTCM observer subscription closes, so the adapter
   closes `rtcmCh`.
4. The dispatcher sees `rtcmCh` closed and nils it out.

## Implementation order

1. `RTCMMsg`, `RTCMSource`, and `RTCMEvent` in `gps/gpsprot/msg.go`.
2. `MakeRTCMMsg` in `gps/internal/rtcm`.
3. `gps/internal/rtcm.PacketProcessor` calls `MakeRTCMMsg` for
   receiver-direction RTCM.
4. `RTCM` on `obs.Observer`, `DefaultObserver`, and
   `MultiObserver`.
5. `gpsevent.LogEvent.RTCM` and `Dispatcher.handleRTCM`.
6. Receiver path: `Dispatcher.NativeMsg` handles `gpsprot.RTCMMsg`
   directly.
7. Network path: stream RTCM observer adapter plus optional
   dispatcher `rtcmCh`.
8. Tests.

## Testing

- Unit test `RTCMMsg` JSON marshaling, including `omitzero`
  behavior for unset `StationID`.
- Unit test `MakeRTCMMsg` with representative `rtcmbin.Msg` types
  (MSM, 1005, 1230).
- Unit test receiver path: process an RTCM packet through the
  dispatcher packet path and verify observer/event-log RTCM handling
  without using `rtcmCh`.
- Unit test network path: send a `gpsprot.RTCMEvent` on `rtcmCh`
  and verify the dispatcher observer receives the expected message.
- Unit test stream adapter filtering and channel close behavior.
- `make test` for no regressions.
