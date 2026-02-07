# Desktop GUI: API gaps and future work

Issues discovered while building the Wails GUI prototype against the `gps/` public API.

## Refactoring

### Move bcast to gps/app/

`time/internal/bcast` is a general-purpose channel multiplexer with no
time-specific dependencies. Move it to `gps/app/bcast` so it can be used by
both `time/app/daemon` and the desktop app.

### Message file support (msgfile.go)

`internal/gpscmd/msgfile.go` contains the TOML message file loader and types
(`MsgFile`, `LineMsg`, `BinaryMsg`, `NMEAMsg`, `UBXMsg`, `CASBINMsg`, `ASBINMsg`).
This needs to move to a package under `gps/` so the desktop app can load and
send message files without depending on `internal/`.

### Response formatting (response.go)

`internal/gpscmd/response.go` contains `responsePrinter` which formats raw
packets into human-readable text (protocol-aware formatting for UBX, CASBIN,
ASBIN, NMEA, with hex dump fallback). The desktop packet monitor currently
shows raw `pkt.Data` strings; it would benefit from this formatting. Factor
into a package under `gps/`.

### ConfigProps serialization to map

The desktop app has a `configPropsToMap` function that manually extracts each
property with its getter and converts to a `map[string]any`. This duplicates
logic in `ConfigProps.serializableMap()` which is unexported. Either export
`serializableMap` or provide a public `ConfigProps.Map() map[string]any`.

## Platform support

### Windows serial port (gps/lib/term)

`gps/lib/term` is Linux-only. Needs a Windows implementation for the desktop
app to work on Windows. macOS support may also need work since term currently
uses Linux-specific terminal ioctls.

### Serial port enumeration

The desktop app currently requires the user to type a device path manually.
Need a way to discover available serial ports. This is best done in the
desktop package itself using platform-specific APIs:
- macOS: scan `/dev/cu.usbmodem*` and `/dev/cu.usbserial*`
- Windows: registry or SetupDi API
- Linux: `/dev/ttyACM*`, `/dev/ttyUSB*`, or udev

## Receiver capabilities

### Supported signals

`ReceiverInfo` has `SupportedGNSS` (which constellations) but no supported
signals. The desktop signal picker currently shows all signals defined in
gpsprot for every constellation. It should show only signals the receiver
actually supports. Need a `SupportedSignals SignalSet` field in
`ReceiverInfo` so the GUI can filter the picker and prevent users from
selecting signals the hardware cannot track.

## Architecture

### Continuous scanning with broadcast

The current architecture starts and stops scanning for each operation (detect,
configure, capture), which causes problems:

- `DetectReceiver`, `ApplyConfig`, etc. call `conn.Stop()` when done
- Once stopped, `SerialConn.Read()` returns EOF permanently
- Subsequent operations (like packet capture) fail silently

The daemon (`time/app/daemon`) has the right pattern:

1. Start `gpsio.Scan` once on connect, producing packets to a channel
2. Wrap channel in `bcast.Bcast` to multiplex to subscribers
3. Each consumer (configurator, packet monitor, proxy) calls `Subscribe()`
4. On disconnect, cancel context and everything shuts down

The desktop app should follow this:

- **Connect**: start scan goroutine, create broadcast, start emitting
  `"gps:packet"` events from a dedicated subscriber
- **Packet Monitor tab**: just displays events (no Start/Stop button needed)
- **DetectReceiver/ApplyConfig**: get a `Subscribe()` channel, pass to
  `gpscfg.Configure()`, then `Unsubscribe()` when done
- **Disconnect**: cancel context, broadcast closes all subscribers

This eliminates the `conn.Stop()` problem and matches how the daemon works.

