# Native Message Observability

Add a small observer hook for parsed protocol-specific GPS messages that do not
map to SatPulse's protocol-independent `gpsprot.MsgHandler` messages.

This is intended as enabling plumbing for OSNMA and trusted-time status, such as
u-blox `UBX-NAV-TIMETRUSTED` and `UBX-SEC-OSNMA`. It should not duplicate
messages that are already handled as protocol-independent time, position,
velocity, satellite, survey, leap-second, or navigation-epoch messages.

## Design

Add a notification-style method to `time/internal/obs.Observer`:

```go
NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) bool
```

Do not embed `gpsprot.NativeMsgHandler` in `obs.Observer`. The packet processor
interface returns an `error` because it is also used by configuration and probing
code. Observability is just notification, so the observer callback should not
return an error.

The observer returns `true` if it recognized or used the native message, and
`false` if it ignored it. This lets the dispatcher keep the existing debug log
for unused native messages without logging messages that are consumed by an
observer.

## Plumbing

1. Add `NativeMsg` to `obs.Observer`.
2. Add a no-op `NativeMsg` implementation to `obs.DefaultObserver` that returns
   `false`.
3. Add `MultiObserver.NativeMsg` fan-out, following the existing `Tick`,
   `NavEpochPV`, and `NTPSample` pattern. It should call every child observer
   and return `true` if any child returns `true`.
4. Update `time/internal/gpsevent.Dispatcher.NativeMsg` to call
   `d.obs.NativeMsg(tag, msgID, msg, tRead)`. If the observer returns `false`,
   log the existing unused-native-message debug message. Always return `nil`.

The packet processors already enforce the desired filtering. For u-blox,
`PacketProcessor.ProcessPacket` calls `Dispatch` first. Only when `Dispatch`
returns false does it invoke the installed `gpsprot.NativeMsgHandler`. Therefore
native observers see unhandled parsed messages such as `NAV-TIMETRUSTED` and
`SEC-OSNMA`, but not handled messages such as `NAV-PVT`, `NAV-TIME*`,
`NAV-SAT`, or `NAV-SIG`.

## Tests

Add focused tests for:

- `MultiObserver.NativeMsg` fans out to all child observers.
- `MultiObserver.NativeMsg` returns `true` if any child observer returns `true`,
  while still calling all child observers.
- `DefaultObserver.NativeMsg` is a no-op and returns `false`.
- `Dispatcher.NativeMsg` forwards to its observer and returns `nil`.
- `Dispatcher.NativeMsg` logs an unused native message only when the observer
  returns `false`.
- u-blox filtering: an unhandled parsed message such as `NAV-TIMETRUSTED`
  reaches the native observer, while a handled message such as `NAV-PVT` does
  not.

No event-log schema change is part of this step. Native message persistence,
SSE exposure, Prometheus metrics, and OSNMA-specific interpretation can build on
this hook later.
