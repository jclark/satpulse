# Desktop GUI issues

## probe-readback: Combine probe and config readback into one operation

On connect, two separate `gpscfg.Configure` calls happen in sequence: the initial probe (to detect the receiver and get `ReceiverInfo`) and then the config readback (to get current property values for the Config tab). If the user is already on the Config tab when they reconnect, both could be combined into a single `Configure` call by adding `Get: readProps` to the initial probe target. This would halve the round-trip time for the reconnect case.

Currently the probe is initiated by the Go backend (`packetWorker`) and the readback is initiated by the frontend (`doReadback` via `ReadConfig`). Combining them would require the backend to know whether properties should be read during the probe, or a way to piggyback the readback request onto the probe.

## connected-speed-change: Allow changing host serial speed while connected

When first connecting to a receiver at an unknown baud rate, or after a speed change via a message file command, the user may need to change the host serial port speed without disconnecting and reconnecting.

Changing the host speed also requires re-running the receiver probe at the new speed. Without re-probing, the receiver-info display and the Config tab gating stay stuck on whatever the last successful probe found.

## tcp-connect: Allow connecting to a GPS receiver via TCP

The desktop app only supports serial connections (`gpsio.OpenSerial`). Adding TCP support would allow managing a receiver attached to a headless machine via satpulsed's TCP proxy.

The `gpsio.Conn` interface is transport-agnostic, and `NetConn` already handles unix sockets, so adding TCP is straightforward at the connection level. The main complication is inter-packet idle detection: the scanner relies on serial read timeouts to generate `Idle()` calls, which the NMEA satellite buffer uses as its primary flush trigger. Over TCP, timing is unreliable due to network latency and TCP buffering, so `Idle()` cannot be generated reliably. The satellite buffer's fallback (repeated GNSS/signal key detection) would still work but lags one cycle behind.

## pvt-staleness: Detect and indicate stale PVT message rows

The PVT Messages panel shows position, velocity, and time rows keyed by `nativeMsgID`, but there is no indication when a particular message type stops arriving. Time staleness is somewhat visible because the displayed time stops updating, but stale position or velocity rows look identical to fresh ones. If a receiver stops sending a particular message (e.g. after a configuration change or signal loss), the old values linger indefinitely with no visual cue.

Two possible approaches:

1. **Age column**: Track the timestamp of the last update for each row and display a "last update" column showing seconds since last refresh. Rows older than a threshold (e.g. 5s) could be dimmed or flagged. This gives the user precise visibility into update rates.

2. **Remove on epoch boundary**: Use `NavEpochMsg` (the `gps:epochPVT` event) as a heartbeat. On each epoch, remove any PVT rows that were not updated during that epoch. This keeps the table clean automatically but means rows flicker in/out if a message arrives intermittently.

A hybrid might work best: dim rows that missed the last epoch, remove rows that have been stale for several epochs.

## provide-corrections: Provide corrections to network clients

Extend the Corrections tab with a "provide corrections" mode (base use case) for serving correction packets (RTCM, or in future PPP-RTK/PPP-AR streams) from the connected receiver to network clients over TCP. The "consume corrections" mode is already implemented.

The backend listens on a TCP port. When a client connects, subscribe to the packet broadcast (`bcast.Subscribe()`), filter for correction packets (e.g. RTCM) by tag, and forward them to the TCP connection. Multiple clients are supported since each gets its own subscription.

UI additions:

- Mode selector to switch between consume and provide
- Port field: TCP port to listen on
- Start / Stop button (shared with consume mode)
- Status panel showing a count of correction packets by message type flowing out, plus the number of connected clients.

NTRIP caster support (HTTP-based, with authentication and mount points) could be added later as a separate transport option in the same tab.

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

## packet-fix-rate: Packet panel assumes 1 Hz fix rate

The packet panel groups messages into epochs using `EPOCH_GAP_MS = 900` (in `packet-panel.tsx`). When a new packet arrives, `recentEntries` is filtered to keep only entries whose timestamps fall within 900ms of the incoming packet. At 1 Hz this cleanly separates consecutive epochs, but at higher fix rates (e.g. 10 Hz, where epochs are 100ms apart) packets from ~9 epochs pile up in a single group. Expanding a row shows entries from multiple epochs rather than just the latest one, and the snapshot feature (which uses `ACTIVE_WINDOW_MS = 1500`) captures multiple epochs too.

The `isActive` check (dimming rows older than 1500ms) is similarly tuned for 1 Hz and would keep rows lit for many epochs at higher rates.

One approach: let the user control the assumed fix rate (expressed in Hz) via a dropdown or input in the packet panel toolbar. The grouping window (`EPOCH_GAP_MS`) and active window (`ACTIVE_WINDOW_MS`) would derive from this value -- e.g. for rate R Hz, `EPOCH_GAP_MS = 0.9 * (1000/R)` and `ACTIVE_WINDOW_MS = 1.5 * (1000/R)`. A sensible default is 1 Hz; common choices would be 1, 2, 5, 10 Hz.

## read-error-disconnect: Disconnect on serial read error (related: #172)

If a USB-connected GPS receiver is physically unplugged while connected, the app shows a read error in the log but remains in the connected state. The user has to manually click Disconnect to reset the UI. This is the desktop GUI counterpart of #172 (satpulsed should handle serial device disappearing); the daemon's approach is to exit and let systemd restart it, but the GUI needs to transition cleanly to disconnected state instead.

The root cause is a gap between the backend and frontend: when `Scan()` in `gpsio/conn.go` encounters a read error, it logs the error and exits the loop, closing the packet channel. The goroutines in `app.go` (`packetLogWorker`, `packetWorker`) detect the closed channel and exit, but neither calls `setEndState()` to emit a `gps:state` event. The frontend never learns the connection is dead and stays visually connected.

Fix: after the goroutines spawned by `Connect()` finish (detected via `connWg`), transition to the disconnected state and emit `gps:state` with `StateDisconnected`. This could be done with a cleanup goroutine that waits on `connWg` and calls `closeLocked()` if the connection wasn't already explicitly disconnected. The frontend already handles `gps:state` transitions and clears stale data on disconnect, so no frontend changes should be needed.

## stationary-hide-speed-acc: Hide speed accuracies when stationary

When the receiver is stationary (ground speed < 0.1 m/s), the speed accuracy and ground speed accuracy values in the status panel are meaningless noise. Track whether the receiver is stationary somewhere accessible to the status panel rendering, and when it is, hide or omit the speed accuracy and ground speed accuracy rows rather than displaying misleading values.

