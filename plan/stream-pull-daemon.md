# Stream pull daemon integration (#221, #99, #237)

Integrate `stream.pull` into satpulsed.  The `gps/app/stream`
package handles the network/serial plumbing; this plan covers
daemon wiring and RTCM observability.

## Prerequisite

- `plan/corrsink-rename.md` (rename corrsink to stream).
- `plan/stream-backoff.md` (adaptive backoff).
- `plan/ntrip-client.md` (NTRIPSource and ntriphdr library) if
  NTRIP transport is needed.

## TOML configuration

`stream.pull` is a single table (one correction source per
receiver).  Transport is selected by which dotted keys are present
(`tcp.address` vs `ntrip.*`), mutually exclusive.

```toml
# Plain TCP
[stream.pull]
tcp.address = "10.56.65.82:2006"

# OR NTRIP client
[stream.pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
ntrip.username = "user"
ntrip.password = "pass"
```

### stream.pull with separate serial port

If the receiver has a second serial input for corrections:

```toml
[stream.pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
serial.device = "/dev/ttyUSB1"
serial.speed = 115200
```

## RTCM observability (#237)

RTCM observability works in both directions: packets from the
receiver and packets from the network (stream.pull).  Both
directions produce the same `gpsprot.RTCMMsg` type and go through
the same observer method.

See `plan/rtcm-msm.md` for the future MSM satellite/signal detail
extension.

### RTCMMsg

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

No `Dispatch` method -- this is not a `gpsprot.Msg` routed through
`MsgHandler`.  It goes through Observer only.

### Conversion function

Add to `gps/internal/rtcm`:

```go
// MakeRTCMMsg builds a gpsprot.RTCMMsg from a parsed rtcmbin.Msg.
func MakeRTCMMsg(msg rtcmbin.Msg, source gpsprot.RTCMSource) gpsprot.RTCMMsg
```

Extracts `MsgType` and `StationID` from the `rtcmbin.Msg`.  Both
directions call this function:

- **Receiver direction:** `gps/internal/rtcm.PacketProcessor`
  already parses the packet via `rtcmbin.ParseMsg`.  It calls
  `MakeRTCMMsg` with `RTCMReceiver` and delivers the result
  through `NativeMsgHandler.NativeMsg`.
- **Network direction:** `gps/app/stream` subscribes to
  `Pull.Packets`, parses RTCM packets via `rtcmbin.ParseMsg`,
  calls `MakeRTCMMsg` with `RTCMNetwork`, and sends the result
  over a channel to the dispatcher.

This keeps all rtcmbin-to-gpsprot conversion in `gps/internal/rtcm`
and avoids the dispatcher (in the `time` layer) needing to import
`gps/internal/` packages.

### Observer interface

Add to `time/internal/obs/observer.go`:

```go
RTCM(msg *gpsprot.RTCMMsg, tRead time.Time)
```

Update `DefaultObserver` (no-op), `MultiObserver` (fan-out with
type assertion, same pattern as `Tick` / `NavEpochPV`).

### Event log

Add to `gpsevent.LogEvent`:

```go
RTCM *gpsprot.RTCMMsg `json:"rtcm,omitempty"`
```

## Dispatcher

Add an optional RTCM event channel to `Dispatcher.Run`:

```go
func (d *Dispatcher) Run(tsCh <-chan ts.Event, pktCh <-chan scan.Packet,
    rtcmCh <-chan RTCMEvent)
```

where

```go
// RTCMEvent pairs an RTCMMsg with its read time.
type RTCMEvent struct {
    Msg   gpsprot.RTCMMsg
    TRead time.Time
}
```

Update the loop condition to include `rtcmCh`:

```go
for tsCh != nil || pktCh != nil || rtcmCh != nil {
```

Add to the select:

```go
case re, ok := <-rtcmCh:
    if ok {
        d.obs.RTCM(&re.Msg, re.TRead)
        d.logEvent(LogEvent{T: re.TRead, RTCM: &re.Msg})
    } else {
        rtcmCh = nil
    }
```

When stream.pull is not configured, `rtcmCh` is nil from the start
and does not affect the loop condition or select.

Note: receiver-direction RTCM events also arrive via `rtcmCh`.
The `PacketProcessor` delivers through `NativeMsgHandler`, and the
daemon routes these into the same channel.  Both directions
converge in the dispatcher.

## Daemon wiring

### Configuration

Add `Stream stream.Config` to `Config` in
`time/app/daemon/config.go`.  The `stream.Config` type is defined
in `gps/app/stream` and matches the `[stream.pull]` TOML table
described above.

Put stream-specific daemon code in a new file
`time/app/daemon/stream.go`.

### Startup in `run()`

Stream setup is split into two phases to avoid a startup deadlock.
`run()` has many fallible steps between early setup and the final
`d.Run(...)` call that starts consuming channels.  If the adapter
started producing before the dispatcher was running, it could block
on `rtcmCh <-` with no consumer, and a subsequent error-path
`cancel()` + `wg.Wait()` would hang.

**Phase 1 -- prepare (before any fallible steps that follow):**

If `[stream.pull]` is configured:

1. Create `stream.NewPull()`.
2. Create the appropriate `Source` (`stream.TCPSource` or
   `stream.NTRIPSource`) based on which transport keys are present.
3. Determine serial port: if `serial.device` is set, open a
   separate serial connection and create a new `OutPortLock`;
   otherwise reuse the main `portLock`.
4. Create `rtcmCh := make(chan RTCMEvent)`.

**Phase 2 -- start (immediately before `d.Run`, after all fallible
steps):**

5. Start `pull.Run` in a goroutine (via `wg.Go`).  `gps/app/stream`
   subscribes to `pull.Packets` internally, parses RTCM packets,
   calls `gps/internal/rtcm.MakeRTCMMsg`, and sends `RTCMEvent`
   on `rtcmCh`.  Stream owns `rtcmCh` and closes it on shutdown.
6. Pass `rtcmCh` to `Dispatcher.Run`.

### Shutdown ordering

Channel ownership: `gps/app/stream` owns `rtcmCh` and is
responsible for closing it.

Shutdown sequence:

1. Daemon cancels the stream context.
2. `Pull.Run` drains its internal pipeline (reader, queue, writer),
   then calls `Packets.Close()` which closes all bcast subscriber
   channels.
3. Stream's internal subscriber sees its channel close, closes
   `rtcmCh`, and exits.
4. The dispatcher sees `rtcmCh` closed, nils it out, and (once
   `tsCh` and `pktCh` are also nil) exits its loop.

The dispatcher loop condition `tsCh != nil || pktCh != nil ||
rtcmCh != nil` ensures it stays alive to drain `rtcmCh` even if
the GPS channels close first.  Conversely, because `Pull.Run` is
ctx-based, the daemon must cancel that context before or
concurrently with closing the GPS channels to avoid the dispatcher
blocking indefinitely on a never-closed `rtcmCh`.

### Connection state logging

`stream.Pull.Run` takes an `onState func(State, error)` callback.
The daemon passes a callback that logs state transitions via slog:

```go
func(st stream.State, err error) {
    switch st {
    case stream.Connecting:
        lg.Info("stream pull connecting", "addr", addr)
    case stream.Connected:
        lg.Info("stream pull connected", "addr", addr)
    case stream.Reconnecting:
        lg.Warn("stream pull reconnecting", "addr", addr, "err", err)
    }
}
```

## Implementation order

1. `RTCMMsg` and `RTCMSource` in `gps/gpsprot/msg.go`.
2. `MakeRTCMMsg` in `gps/internal/rtcm`.
3. `PacketProcessor` calls `MakeRTCMMsg` for receiver direction.
4. `RTCM` on Observer interface + DefaultObserver + MultiObserver.
5. `RTCMEvent` + dispatcher changes.
6. `stream.Config` + daemon wiring.
7. Tests.

## Testing

- Unit test `RTCMMsg` JSON marshaling (verify `omitzero` omits
  unset `StationID`).
- Unit test `MakeRTCMMsg` with various rtcmbin.Msg types (MSM,
  1005, 1230).
- Unit test dispatcher: send `RTCMEvent`, verify observer receives
  `RTCMMsg` with correct source and fields.
- `make test` for no regressions.
- Manual: add `[stream.pull]` to `/etc/satpulse.toml` pointing at
  a base station, verify `rtcm` entries in the event log.
