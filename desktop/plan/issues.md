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

## rtk-tab: RTK corrections tab (base and rover)

An RTK tab for forwarding RTCM correction data between a TCP peer and the connected GPS receiver. The user chooses a mode -- Base or Rover -- which determines the direction of data flow:

- **Rover**: Dials a remote TCP address (host + port), reads the RTCM byte stream, and writes it to the serial connection. The serial connection is already full-duplex, so injecting RTCM data is straightforward. The goroutine should reconnect on network errors and stop when the serial connection is closed.

- **Base**: Listens on a TCP port and serves RTCM packets from the connected receiver to network clients. When a client connects, subscribe to the packet broadcast (`bcast.Subscribe()`), filter for RTCM packets by tag, and forward them to the TCP connection. Multiple clients are supported since each gets its own subscription.

UI:

- Mode selector: Base / Rover
- Host field (rover only): IP or hostname of the base station
- Port field (both modes): TCP port to connect to (rover) or listen on (base)
- Start / Stop button
- Status panel below showing a count of RTCM packets by message type flowing through the connection. For base mode, also show the number of connected clients.

Write contention (rover mode): if `outportlock-desktop` is done first, the rover uses `OutPortLock` to coordinate writes with config and message file send -- no need to disable other tabs while corrections flow. The rover acquires per-packet and releases between packets, allowing config to interleave.

The rover should use `SerialConn.WritePacket` to write RTCM packets to the receiver, so they automatically appear in the packet monitor with direction and message type.

NTRIP caster support (HTTP-based, with authentication and mount points) could be added later as a separate transport option in the same tab.

## outportlock-desktop: Use OutPortLock for serial write coordination

The desktop backend currently uses `ConnState` to prevent concurrent writes to the serial port: `ReadConfig` and `WriteConfig` set `StateConfiguring`, `SendMsgFile` sets `StateSending`, and each checks `state == StateConnected` before proceeding. This means only one writing operation can run at a time, and the state machine encodes write exclusion rather than just UI status.

Replace this with `gpsio.OutPortLock`. The backend creates an `OutPortLock` on connect and each writing operation acquires it for its duration:

- `ReadConfig` / `WriteConfig`: acquire the lock for the entire `gpscfg.Configure` call. The lock is held across the full request-response sequence so that configuration packets and their acknowledgements are not interleaved with other writes.
- `SendMsgFile`: acquire the lock for the duration of the send loop. Release on completion, cancellation, or error.
- Rover (future): acquire per-packet, release between packets, allowing config to interleave.

This removes the need for `StateConfiguring` and `StateSending` as write-exclusion gates. These states can either be removed entirely or kept purely for UI feedback (e.g. showing "configuring..." in the status bar) without gating other operations. Operations that previously failed with "not connected" when another write was in progress will instead queue waiting for the lock.

The `ConnState` type simplifies to just `disconnected`, `connecting`, and `connected`. The frontend no longer needs to disable config/send tabs based on state -- the lock handles contention transparently.

## map-tile-retry: Reload failed map tiles when connectivity is restored

If the app starts without internet connectivity (or loses it), map tile `<img>` loads fail silently. When connectivity is restored the tiles remain broken because the browser does not retry failed image loads, and the tile URLs haven't changed so Preact reuses the existing DOM elements.

HTML/CSS have no native retry mechanism for failed `<img>` loads. The cleanest Preact-idiomatic fix is to keep an `epoch` counter in state and include it in each tile's `key`. The `src` stays the clean canonical tile URL. When the browser fires the `online` event, increment the epoch. The key change causes Preact to remount the `<img>` elements, which triggers a fresh fetch. Tiles that loaded successfully before the remount will likely serve from browser cache, so the cost is minimal.

```tsx
const [epoch, setEpoch] = useState(0);
useEffect(() => {
    const h = () => setEpoch(e => e + 1);
    window.addEventListener('online', h);
    return () => window.removeEventListener('online', h);
}, []);
```

Each tile img uses `key={`${tileX},${tileY}:${epoch}`}`.

## pvt-row-types: Replace pvt-panel Row types with gpsprot interfaces

The PVT panel defines its own `PosGeoRow`, `PosECEFRow`, `VelGeoRow`, `VelECEFRow`, and `TimeRow` types in `pvt-panel.tsx:38-84`. These duplicate the gpsprot message interfaces (`PosGeoMsg`, `PosECEFMsg`, etc.) but add a `kind` discriminant field that comes from the `MsgEvent` envelope, not the wire type. After `gps/ts/` provides canonical gpsprot interfaces ([gps-ts-types.md](../../plan/gps-ts-types.md)), the panel should import those directly and handle dispatch with a local tagged union (e.g. `{ kind: 'posGeo'; msg: PosGeoMsg }`).

## read-error-disconnect: Disconnect on serial read error (related: #172)

If a USB-connected GPS receiver is physically unplugged while connected, the app shows a read error in the log but remains in the connected state. The user has to manually click Disconnect to reset the UI. This is the desktop GUI counterpart of #172 (satpulsed should handle serial device disappearing); the daemon's approach is to exit and let systemd restart it, but the GUI needs to transition cleanly to disconnected state instead.

The root cause is a gap between the backend and frontend: when `Scan()` in `gpsio/conn.go` encounters a read error, it logs the error and exits the loop, closing the packet channel. The goroutines in `app.go` (`packetLogWorker`, `packetWorker`) detect the closed channel and exit, but neither calls `setEndState()` to emit a `gps:state` event. The frontend never learns the connection is dead and stays visually connected.

Fix: after the goroutines spawned by `Connect()` finish (detected via `connWg`), transition to the disconnected state and emit `gps:state` with `StateDisconnected`. This could be done with a cleanup goroutine that waits on `connWg` and calls `closeLocked()` if the connection wasn't already explicitly disconnected. The frontend already handles `gps:state` transitions and clears stale data on disconnect, so no frontend changes should be needed.

