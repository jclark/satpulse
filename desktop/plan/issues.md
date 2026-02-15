# Desktop GUI issues

## probe-readback: Combine probe and config readback into one operation

On connect, two separate `gpscfg.Configure` calls happen in sequence: the initial probe (to detect the receiver and get `ReceiverInfo`) and then the config readback (to get current property values for the Config tab). If the user is already on the Config tab when they reconnect, both could be combined into a single `Configure` call by adding `Get: readProps` to the initial probe target. This would halve the round-trip time for the reconnect case.

Currently the probe is initiated by the Go backend (`packetWorker`) and the readback is initiated by the frontend (`doReadback` via `ReadConfig`). Combining them would require the backend to know whether properties should be read during the probe, or a way to piggyback the readback request onto the probe.

## config-speed-change: Handle receiver serial speed change in GUI

The CLI supports `--speed` to configure the GPS receiver's serial port speed (as opposed to `--device-speed` which sets the host serial port speed). The desktop GUI needs equivalent support so users can change the receiver's baud rate from the Config tab.

The backend already handles this correctly: `gpscfg.configure()` detects `ConfigAction.Speed != 0` and calls `SerialConn.WriteThenChangeSpeed()`, which sends the command, waits for drain, then changes the host port speed. The `Scan` goroutine continues reading at the new speed. The missing piece is propagating the new speed back to the frontend so the connection bar reflects reality.

Outline:

1. Add a `Speed int` field to `gpscfg.Result`. In `gpscfg.configure()`, record the speed from `ConfigActionSendRequest` when `action.Speed != 0`.
2. In `app.go`, after `gpscfg.Configure` returns with a non-zero `Result.Speed`, emit a `gps:speed` event to the frontend with the new speed value.
3. Frontend listens for `gps:speed` and updates the speed state in `App`, which flows down to the connection bar dropdown.

## connected-speed-change: Allow changing host serial speed while connected

When first connecting to a receiver at an unknown baud rate, or after a speed change via a message file command, the user may need to change the host serial port speed without disconnecting and reconnecting. The speed dropdown should remain enabled while connected and changing it should call `term.Change()` on the live `SerialConn` to switch the host port speed in place.

## disable-inputs: Disable device and speed inputs while connected

The device text input and speed dropdown in the connection bar remain editable while connected, but changing them has no effect on the live connection. They should be disabled when `connected` is true to avoid confusion.

## tcp-connect: Allow connecting to a GPS receiver via TCP

The desktop app only supports serial connections (`gpsio.OpenSerial`). Adding TCP support would allow managing a receiver attached to a headless machine via satpulsed's TCP proxy.

The `gpsio.Conn` interface is transport-agnostic, and `NetConn` already handles unix sockets, so adding TCP is straightforward at the connection level. The main complication is inter-packet idle detection: the scanner relies on serial read timeouts to generate `Idle()` calls, which the NMEA satellite buffer uses as its primary flush trigger. Over TCP, timing is unreliable due to network latency and TCP buffering, so `Idle()` cannot be generated reliably. The satellite buffer's fallback (repeated GNSS/signal key detection) would still work but lags one cycle behind.

## pvt-staleness: Detect and indicate stale PVT message rows

The PVT Messages panel shows position, velocity, and time rows keyed by `nativeMsgID`, but there is no indication when a particular message type stops arriving. Time staleness is somewhat visible because the displayed time stops updating, but stale position or velocity rows look identical to fresh ones. If a receiver stops sending a particular message (e.g. after a configuration change or signal loss), the old values linger indefinitely with no visual cue.

Two possible approaches:

1. **Age column**: Track the timestamp of the last update for each row and display a "last update" column showing seconds since last refresh. Rows older than a threshold (e.g. 5s) could be dimmed or flagged. This gives the user precise visibility into update rates.

2. **Remove on epoch boundary**: Use `NavEpochMsg` (the `gps:epochPVT` event) as a heartbeat. On each epoch, remove any PVT rows that were not updated during that epoch. This keeps the table clean automatically but means rows flicker in/out if a message arrives intermittently.

A hybrid might work best: dim rows that missed the last epoch, remove rows that have been stale for several epochs.

