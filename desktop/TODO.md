# Desktop GUI: API gaps and future work

Issues discovered while building the Wails GUI prototype against the `gps/` public API.

## Factor out from gpscmd

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

## API ergonomics

### Configure/scan boilerplate

Every operation (detect, configure, save, reset, capture) requires the same
boilerplate: create channel, spawn `gpsio.Scan` in a goroutine, call
`gpscfg.Configure`, call `conn.Stop()`, drain channel, wait. The desktop app
has this pattern in `runConfig`, `DetectReceiver`, and `StartCapture`.
Consider a higher-level helper in `gps/app/` that wraps this into a single
call, e.g.:

    gpscfg.Run(ctx, lg, conn, target) (*Result, error)

### ConfigProps serialization to map

The desktop app has a `configPropsToMap` function that manually extracts each
property with its getter and converts to a `map[string]any`. This duplicates
logic in `ConfigProps.serializableMap()` which is unexported. Either export
`serializableMap` or provide a public `ConfigProps.Map() map[string]any`.

