# Packet log refactor

## Motivation

The `gpsio.PacketLog` type currently bundles three concerns: collecting packet entries from producers (scanner for rx, serial writer for tx), formatting output bytes into log entries (protocol format discovery), and writing entries to a JSONL file. This tight coupling means the only consumer is the file writer in `doLogPackets`.

The desktop GUI needs to consume the same bidirectional packet stream to show packets in its packet monitor panel. Currently it has its own separate `packetEventWorker` that subscribes to the scanner broadcast and manually constructs `PacketEvent` structs -- a near-duplicate of `PacketLogEntry` but missing the `Out` field. This means outgoing packets (configuration, message file sends) are invisible in the desktop packet monitor.

Refactoring `PacketLog` to expose a clean consumer channel lets both the daemon file writer and the desktop event emitter consume the same bidirectional packet stream without duplicating the entry construction logic.

## Current architecture

```
                        LogInput(scan.Packet)
gpsio.Scan ──────────────────────────────────┐
                                             ▼
                                    PacketLog {
                                      ch chan<- PacketLogEntry
                                      pktFormats []PacketFormat   <-- used by LogOutput
                                      closeCount atomic.Int32
                                    }
                                             │
SerialConn.Write ── logWrite ── LogOutput ───┘
  (raw []byte)       (format discovery)      │
                                             ▼
                                      doLogPackets goroutine
                                      (writes JSONL to file)
```

Problems:
- `PacketLog` knows about `gpsprot.PacketFormat` (only needed for tx format discovery)
- `LogInput` and `LogOutput` have different signatures for the same operation (send a `PacketLogEntry`)
- No way to get a consumer channel without also starting a file writer
- Desktop duplicates the rx-to-entry conversion in `packetEventWorker`

## New architecture

```
                     LogInput(scan.Packet)
gpsio.Scan ──────────────────────────────────┐
                                             ▼
                                         PacketLog {
                                           pl *packetLog
                                           pktFormats []PacketFormat
                                         }
                                             │
SerialConn.Write ── PacketLog.logWrite ──────┘   <-chan PacketLogEntry
  (raw []byte)    (format discovery,             │
                   builds entry)                 ├──> doLogPackets (daemon: JSONL file)
                                                 └──> EventsEmit (desktop: gps:packet)
Rover ── SerialConn.WritePacket ─────────┘
  (scan.Packet   (writes + logs with known
   from TCP)      format, no discovery)
```

### `packetLog` (unexported)

Channel wrapper. Handles the `SemiClose` two-caller protocol for closing the channel.

```go
type packetLog struct {
	ch         chan<- PacketLogEntry
	closeCount atomic.Int32
}

func newPacketLog() (*packetLog, <-chan PacketLogEntry) {
	ch := make(chan PacketLogEntry, 2)
	return &packetLog{ch: ch}, ch
}

func (pl *packetLog) log(entry PacketLogEntry) {
	pl.ch <- entry
}

// semiClose must be called twice: once when input logging is done,
// once when output logging is done. Closes the channel on the second call.
func (pl *packetLog) semiClose()
```

### `PacketLog` (exported)

Handles conversion from `scan.Packet` and raw bytes to `PacketLogEntry`. Holds the packet formats for output format discovery.

```go
type PacketLog struct {
	pl         *packetLog
	pktFormats []gpsprot.PacketFormat
}

// NewPacketLog creates a PacketLog and returns the consumer channel.
func NewPacketLog(fmts []gpsprot.PacketFormat) (*PacketLog, <-chan PacketLogEntry)

// LogInput logs an incoming packet from the scanner.
// Builds a PacketLogEntry from the scan.Packet (format already parsed).
func (pl *PacketLog) LogInput(pkt scan.Packet)

// SemiClose must be called twice: once when input logging is done,
// once when output logging is done.
func (pl *PacketLog) SemiClose()
```

```go
// LogOutput logs an outgoing write. If fmt is non-nil, it is used directly
// for the tag and message ID. If fmt is nil, pktFormats is iterated to
// discover the format from the raw bytes.
func (pl *PacketLog) LogOutput(tWrite time.Time, bytes []byte, speed int, fmt gpsprot.PacketFormat)
```

### `SerialConn` changes

`SerialConn` holds `*PacketLog` (same field name, but now the new exported type). `SetPacketLog` signature simplifies since `PacketLog` already has formats:

```go
func (c *SerialConn) SetPacketLog(pl *PacketLog)
```

The existing `logWrite` method on `SerialConn` calls `pl.LogOutput(tWrite, bytes, speed, nil)`.

New method for writes where the caller already knows the packet format:

```go
// WritePacket writes bytes to the serial port and logs the write
// using the provided format instead of doing format discovery.
// Used by the rover, which has already scanned the packet from TCP
// and knows its format.
func (c *SerialConn) WritePacket(p []byte, fmt gpsprot.PacketFormat) (int, error)
```

Same as `Write` but calls `pl.LogOutput(tWrite, bytes, 0, fmt)`.

### `gpsio.Scan`

`Scan` already takes `*PacketLog`. Calls `pLog.LogInput(pkt)` -- same method name as today, no change needed in `Scan`.

```go
func Scan(ctx context.Context, lg *slog.Logger, conn Conn, ch chan<- scan.Packet, pLog *PacketLog)
```

### `LogPackets`

Signature changes to take packet formats. Internally calls `NewPacketLog` and starts `doLogPackets` on the consumer channel.

```go
func LogPackets(lg *slog.Logger, wg *sync.WaitGroup, logPath string, fmts []gpsprot.PacketFormat) (*PacketLog, *logfile.LogFile, error) {
	// ... file setup unchanged ...
	pl, ch := NewPacketLog(fmts)
	wg.Go(func() { doLogPackets(lg, lf, ch) })
	return pl, lf, nil
}
```

`doLogPackets` changes signature to take `<-chan PacketLogEntry` instead of the old unexported channel.

### Daemon caller (`daemon.go`)

Passes packet formats to `LogPackets`:

```go
pLog, lf, err := gpsio.LogPackets(lg, &wg, path, gpsreg.PacketFormats)
conn.SetPacketLog(pLog)
pCh := startScan(ctx, lg, &wg, conn, pLog)
```

### Desktop caller

```go
pLog, ch := gpsio.NewPacketLog(gpsreg.PacketFormats)
conn.SetPacketLog(pLog)
// pass pLog to gpsio.Scan for rx logging
// consumer goroutine:
go func() {
	for entry := range ch {
		runtime.EventsEmit(a.ctx, "gps:packet", entry)
	}
}()
```

### Rover usage (desktop, future daemon)

```go
// pkt scanned from TCP connection
_, err := conn.WritePacket([]byte(pkt.Data), pkt.Format)  // write + log with known format
```

## `PacketLogEntry` type

Unchanged:

```go
type PacketLogEntry struct {
	T     TimeMicro   `json:"t"`
	Tag   gpsprot.Tag `json:"tag,omitempty"`
	Msg   string      `json:"msg,omitempty"`
	Bin   HexString   `json:"bin,omitempty"`
	Ascii string      `json:"ascii,omitempty"`
	Speed *int        `json:"speed,omitempty"`
	Out   bool        `json:"out"`
}
```

## Testing

The existing daemon packet logging behaviour must be preserved. Test by running `satpulsed` with `--packet-log` and verifying the JSONL output contains both rx and tx entries with correct tags, message IDs, and direction.

Unit tests for the refactored types: create a `PacketLog` via `NewPacketLog`, send entries via `LogInput`, verify they arrive on the consumer channel with correct fields and direction. Test `SemiClose` closes the channel after two calls.

## Files changed

- `gps/app/gpsio/log.go` -- split into unexported `packetLog` and exported `PacketLog`, add `NewPacketLog`, `LogOutput`
- `gps/app/gpsio/serial.go` -- update `SetPacketLog` signature, add `WritePacket`, update `logWrite` to call `PacketLog.LogOutput`
- `gps/app/gpsio/conn.go` -- no change (`Scan` already calls `pLog.LogInput`)
- `time/app/daemon/daemon.go` -- pass `gpsreg.PacketFormats` to `LogPackets`, remove formats arg from `SetPacketLog`
