# Dedicated correction-input serial port (#291)

Allow `[stream.pull]` to drive a second serial port dedicated to
corrections, separate from the main receiver port.  This is useful
for receivers with a dedicated correction input channel.

The base stream-pull daemon integration is described in
`plan/stream-pull-daemon.md` (#221, #99); that plan shares the main
receiver port for corrections.  This plan extends it to allow a
separate port.

## TOML

```toml
[stream.pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
serial.device = "/dev/ttyUSB1"
serial.speed = 115200
```

`serial.device` is required when `[stream.pull.serial]` is present;
`serial.speed` follows the same `0 = use current speed` semantics as
the main `[serial]` table.

## Configuration

Add a `Serial` sub-table to `stream.PullConfig`.  Reject configs
where `serial.device` equals the main `[serial]` device (would
conflict with the main port lock).

## Startup

Replace the shared-port wiring in `plan/stream-pull-daemon.md`:
if `[stream.pull.serial]` is present, open a second
`*gpsio.SerialConn` via `gpsio.OpenSerial(device, speed)`, build a
fresh `OutPortLock` for it, and use that connection as both the
`PacketWriter` and the locked port.  The daemon `run()` defers
`Close` on this connection.  Otherwise, fall back to the shared-port
wiring.

## Shutdown

Adds one step to the base shutdown sequence: after `Pull.Run`
returns, the daemon closes the separate `SerialConn` (via the
deferred close).

## Testing

- Config parsing: separate `serial.{device,speed}` fields, conflict
  with main `serial.device`.
- Daemon helper: separate `SerialConn` is opened when
  `serial.device` is set and closed on shutdown.
