# Desktop GUI issues

## probe-readback: Combine probe and config readback into one operation

On connect, two separate `gpscfg.Configure` calls happen in sequence: the initial probe (to detect the receiver and get `ReceiverInfo`) and then the config readback (to get current property values for the Config tab). If the user is already on the Config tab when they reconnect, both could be combined into a single `Configure` call by adding `Get: readProps` to the initial probe target. This would halve the round-trip time for the reconnect case.

Currently the probe is initiated by the Go backend (`packetWorker`) and the readback is initiated by the frontend (`doReadback` via `ReadConfig`). Combining them would require the backend to know whether properties should be read during the probe, or a way to piggyback the readback request onto the probe.

## configure-msg-handler: Semantic messages dropped during Configure

During `gpscfg.Configure`, `init()` calls `pp.SetMsgHandler(mh)` on every packet processor, replacing the caller's handler with gpscfg's internal `msgHandler`. This handler embeds `DefaultHandler`, so all semantic methods (Time, PosGeo, VelGeo, NavEpoch, etc.) are no-ops. Messages parsed during detection and probing are silently dropped.

gpscfg's `msgHandler` uses `SetMsgHandler` for one reason only: to capture `LeapSecond` messages for `Result.LeapSecond`. The `NativeMsg` callback (line 568) just logs unused messages and is managed separately via `installNativeMsgHandlers`/`MultiNativeMsgHandler`. Detection counting (`msgCount`) happens directly from the scanner loop (line 547), before `ProcessPacket` is called, so it does not depend on `MsgHandler` at all.

### Phase 1 on master: Configure stops overwriting MsgHandler

Stop calling `SetMsgHandler` in `gpscfg.init()`. Whatever `MsgHandler` the caller installed on the packet processors stays in place throughout. Remove `Result.LeapSecond` and the `LeapSecond` method from gpscfg's `msgHandler`.

Temporary contract (superseded by phase 2): callers must call `SetMsgHandler` with a non-nil handler before processing packets. `gpscfg.Configure` requires the packet processors it is passed to be in ready-to-process state. `SetMsgHandler` panics if called with nil. Document this in the `PacketProcessor` interface.

- **daemon** (daemon.go:182-194): install a `MsgHandler` that captures `LeapSecond` on the packet processors before calling Configure. After Configure returns, pass the captured leap second to the Dispatcher, replacing the current `Result.LeapSecond` seeding (daemon.go:276-279).
- **satpulsetool** (gpscmd.go:197): install a no-op `DefaultHandler` before calling Configure.

Above is issue #210

- **ubx**: add nil guard to `flushSats` (ubx.go:135) for consistency with the other dispatch paths. Not required by the contract but nice to have.

### Phase 2 on master: PacketProcessor safe without prior SetMsgHandler

This is issue #211.

### Fix on desktop-gui: desktop (requires phase 1 only)

In `packetWorker`, the desktop's `msgHandler` is created before Configure (app.go:194) but is not installed on the packet processors until after Configure returns (line 241). Its `LeapSecond` handler (line 788) already calls `UpdateLeapSecond` and emits a frontend event. With Configure no longer overwriting, install the handler on the packet processors before calling Configure. Remove the post-config `SetMsgHandler` loop (lines 240-243) and the `Result.LeapSecond` seeding (lines 228-230). Semantic messages (time, position, velocity) will flow to the frontend during the entire probe, eliminating the visible delay. The same applies to inline config requests (line 264).

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
