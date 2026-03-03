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
                                           ch chan<- PacketLogEntry
                                           pktFormats []PacketFormat
                                         }
                                             │
SerialConn.Write ── logWrite ── LogOutput ───┘   <-chan PacketLogEntry
  (raw []byte)       (fmt=nil: discovery)        │
                                                 ├──> doLogPackets (daemon: JSONL file)
                                                 └──> EventsEmit (desktop: gps:packet)
Rover ── SerialConn.WritePacket ─────────┘
  (scan.Packet   (fmt=known: no discovery)
   from TCP)
```

### `PacketLog`

`PacketLog` struct is unchanged. New `NewPacketLog` constructor takes formats and returns the consumer channel, replacing the hardcoded `gpsreg.PacketFormats` reference:

```go
// NewPacketLog creates a PacketLog and returns the consumer channel.
func NewPacketLog(fmts []gpsprot.PacketFormat) (*PacketLog, <-chan PacketLogEntry)
```

`LogInput` and `SemiClose` are unchanged.

`LogOutput` gets a new `fmt` parameter. If non-nil, it is used directly. If nil, `pktFormats` is iterated to discover the format. Once the format is resolved, `useBinary` decides binary vs ascii encoding:

```go
func (pl *PacketLog) LogOutput(tWrite time.Time, bytes []byte, speed int, fmt gpsprot.PacketFormat)
```

### `SerialConn` changes

`SetPacketLog` signature is unchanged:

```go
func (c *SerialConn) SetPacketLog(pl *PacketLog)
```

`WriteThenChangeSpeed` becomes a thin wrapper around an unexported `writeThenChangeSpeed` that takes an additional `pktFmt gpsprot.PacketFormat` parameter, threaded through `logWrite` to `LogOutput`:

```go
func (c *SerialConn) Write(p []byte) (int, error) {
	return c.writeThenChangeSpeed(p, 0, nil)
}

func (c *SerialConn) WritePacket(p []byte, fmt gpsprot.PacketFormat) (int, error) {
	return c.writeThenChangeSpeed(p, 0, fmt)
}

func (c *SerialConn) WriteThenChangeSpeed(p []byte, speed int) (int, error) {
	return c.writeThenChangeSpeed(p, speed, nil)
}
```

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
