# Port lock refactor

## Motivation

The TCP proxy in `time/internal/proxy` uses a `chan gpsio.OutPort` as a write lock to coordinate exclusive access to the serial port among proxy connections. This mechanism needs to be shared with other writers (RTK correction feeder in both the desktop app and the daemon -- see issue #99). Moving the port lock into `gpsio` makes it available to any component that writes to the serial port.

## Current code

In `proxy.go`, `Start` creates the lock and seeds it with the port:

```go
var portLock chan gpsio.OutPort
if !allReadOnly {
    portLock = make(chan gpsio.OutPort, 1)
    portLock <- port
}
```

`connReadWorker` acquires by receiving from the channel, releases by sending the port back. A read deadline on the TCP connection auto-releases the lock if no data arrives within `writeLockTimeout`.

## New type in `gpsio`

```go
// OutPortLock coordinates exclusive write access to an OutPort.
// Acquire the port by receiving from the channel; release by sending it back.
type OutPortLock chan OutPort

// NewOutPortLock creates a new OutPortLock containing the given port.
func NewOutPortLock(port OutPort) OutPortLock {
	ch := make(OutPortLock, 1)
	ch <- port
	return ch
}
```

Timeout and release policy remain with each caller. The proxy continues to use its read-deadline-based timeout. Other callers (rover, config) use their own acquire/release patterns.

## Changes

### `gps/app/gpsio/outportlock.go` (new)

Define `OutPortLock` and `NewOutPortLock`.

### `time/internal/proxy/proxy.go`

- `Start` receives an `gpsio.OutPortLock` parameter instead of `gpsio.OutPort`. Remove the local `portLock chan gpsio.OutPort` creation. Pass `nil` when `allReadOnly`.
- `handleListen`, `handleConn`, `connReadWorker` change parameter type from `chan gpsio.OutPort` to `gpsio.OutPortLock`.
- No logic changes inside `connReadWorker` -- channel operations are the same since `OutPortLock` is a channel type.

### `time/app/daemon/daemon.go`

Create the `OutPortLock` in the daemon and pass it to `proxy.Start`:

```go
portLock := gpsio.NewOutPortLock(conn)
proxy.Start(ctx, lg, &wg, cfg.Proxy, pb, portLock)
```

## Testing

Existing proxy behaviour is preserved since the channel operations are identical. Verify by running `satpulsed` with a writable proxy and confirming writes still work.
