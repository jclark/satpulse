# GPS configuration extensibility

This is issue #200.

Allow users to extend GPS configuration beyond what satpulsetool natively supports.

## Motivation

`satpulsetool gps` provides protocol-agnostic configuration, but:

- there will always be protocol-specific details that cannot be abstracted
- only UBX and Unicore protocols are currently supported; adding new protocols requires significant work
- technical users are comfortable reading their receiver's manual and constructing exact commands

## Design

### Message file format

Messages are defined in a TOML file with named sections. Each section defines a named message; the section name becomes the message identifier used with `-m`.

Each message has a `type` key that determines the valid keys in the section, the framing, and response handling:

```toml
[has]
type = "crlf"
line = "CONFIG PPP ENABLE E6-HAS"
```

**Types:**

| Type | Keys | Framing | Response |
|----------|------|---------|----------|
| `"raw"` | `bin` or `ascii` | none | fire and forget |
| `"crlf"` | `line` | adds `\r\n` | fire and forget |
| `"cr"` | `line` | adds `\r` | fire and forget |
| `"lf"` | `line` | adds `\n` | fire and forget |
| `"nmea"` | `data` | prepends `$` if missing; computes/validates checksum (`*XX`); appends `\r\n` | fire and forget |
| `"pqtm"` | `data` | prepends `$PQTM`; computes checksum; appends `\r\n` | expects `$PQTM{cmd},OK*XX` |
| `"ubx"` | `class`, `id`, `pack`, `payload` | UBX framing (sync bytes, length, checksums); little-endian | expects ACK/NAK |
| `"cas"` | `class`, `id`, `pack`, `payload` | CASIC framing; payload must be multiple of 4 bytes | expects ACK/NAK |
| `"multi"` | `msgs` | n/a | per-message |

**Content keys by type:**

| Key | Description |
|-----|-------------|
| `line` | ASCII string (for `crlf`, `cr`, `lf` protocols) |
| `data` | NMEA sentence content (for `nmea` type); content after `$PQTM` (for `pqtm` type) |
| `ascii` | Exact byte sequence as ASCII string (for `raw` protocol) |
| `bin` | Exact byte sequence as hex string (for `raw` protocol) |
| `class` | Message class (for `ubx`, `cas` protocols) |
| `id` | Message ID (for `ubx`, `cas` protocols) |
| `pack` | Pack format string, e.g. `"U1U2U4"` (for `ubx`, `cas` protocols) |
| `payload` | Array of values to pack (for `ubx`, `cas` protocols) |
| `msgs` | Array of message names (for `multi` type) |

**Common options:**

| Key | Description |
|-----|-------------|
| `delay` | Delay in seconds after sending this message before sending the next (default: 0) |

**Pack specifiers** (following u-blox notation):
- `U1`, `U2`, `U4` - unsigned integer (1, 2, 4 bytes)
- `I1`, `I2`, `I4` - signed integer
- `X1`, `X2`, `X4` - bitfield (same encoding as unsigned)
- `R4`, `R8` - IEEE 754 float (4 bytes) and double (8 bytes)

**Defaults:**

The special `[default]` section sets default values for all messages:

```toml
[default]
type = "nmea"

[gps-bds]
data = "PCAS04,3"
```

Per-message options override the defaults.

### CLI interface

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 \
    -f um980.toml \
    -m has,rtk-base
```

**New flags:**

| Flag | Description |
|------|-------------|
| `-f`, `--msg-file PATH` | Path to TOML file containing message definitions |
| `-m`, `--msg NAME,...` | Comma-separated list of messages to send (in order) |

### Open design issues

#### Customizable response handling for line types

For line-based types (`crlf`, `cr`, `lf`, `nmea`), consider adding optional keys to expect a response, e.g., a line matching a regexp. Currently these are fire-and-forget.

### Resolved design issues

#### Response handling

The `type` key determines response handling. Binary types (`ubx`, `cas`) expect ACK/NAK responses. Line-based types are fire-and-forget by default.

#### Framing for binary types

The `"ubx"` and `"cas"` types handle framing automatically (sync bytes, length, checksums).

#### Interaction with existing config flags

The `-m` flag cannot be combined with other config flags like `--gnss`, `--pps`, etc. This avoids issues with `--save` semantics and ambiguity about the relative ordering of manual messages versus higher-level configuration.

### Examples

**UM980 message file:**

```toml
# um980.toml
[default]
type = "crlf"

[has]
line = "CONFIG PPP ENABLE E6-HAS"

[signalgroup2]
line = "CONFIG SIGNALGROUP 2"
```

**Enable Galileo HAS:**

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -f um980.toml -m has
```

**Multiple messages in sequence:**

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 \
    -f um980.toml \
    -m has,signalgroup2
```

**u-blox message file with UBX type:**

This example sends a UBX-CFG-VALSET message to use GPS L5 signals regardless of health status. (Note: satpulsetool already does this automatically when configuring bands with `--band`, but it illustrates the UBX format.)

```toml
# ublox.toml
[default]
type = "ubx"

[gps-l5-health-override]
class = 0x06
id = 0x8A
pack = "U1U1U2U4U1"
payload = [0, 1, 0, 0x10320001, 1]
```

```
satpulsetool gps -d /dev/ttyACM0 -s 9600 -f ublox.toml -m gps-l5-health-override
```

**CASIC message file with NMEA type:**

This example enables GPS and BeiDou on a CASIC-based receiver using the NMEA-style command format with automatic checksum computation.

```toml
# casic.toml
[default]
type = "nmea"

[gps-bds]
data = "PCAS04,3"
```

```
satpulsetool gps -d /dev/ttyUSB0 -s 9600 -f casic.toml -m gps-bds
```

**Quectel message file with PQTM type:**

This example configures PPS output on a Quectel LC29H. The tool expects a `$PQTMCFGPPS,OK*XX` response.

```toml
# quectel.toml
[default]
type = "pqtm"

[pps]
data = "CFGPPS,W,1,1,100,1,1,0"
```

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -f quectel.toml -m pps
```

## Implementation

### Stage 1: Fire-and-forget types

Implement support for the simple line-based types: `raw`, `crlf`, `cr`, `lf`, `nmea`, and `multi`.

These types don't require protocol-specific response handling. `satpulsetool gps` parses the TOML file, constructs the framed bytes for each message, writes them to the serial port with appropriate delays, and displays any responses received.

Response display uses the existing `gpsio.Scan` infrastructure to read packets. Recognized packets (NMEA, etc.) are displayed in their native format; unrecognized data is shown as ASCII lines. This gives users visibility into receiver responses without programmatic success/failure determination.

This stage is self-contained within `satpulsetool gps` and doesn't require changes to the `ConfigProtocol` or `Configurator` interfaces.

#### Implementation steps

**1. Add `internal/gpscmd/msgfile.go`**

New file for TOML message file parsing, message construction, and execution.

```go
package gpscmd

type msgType string

const (
    msgTypeRaw   msgType = "raw"
    msgTypeCRLF  msgType = "crlf"
    msgTypeCR    msgType = "cr"
    msgTypeLF    msgType = "lf"
    msgTypeNMEA  msgType = "nmea"
    msgTypeMulti msgType = "multi"
)

type msgDef struct {
    Type     msgType
    Line     string
    Data     string
    ASCII    string   `toml:"ascii"`
    Bin      string   `toml:"bin"`
    Msgs     []string `toml:"msgs"`
    Delay    float64
}

type msgFile struct {
    defs map[string]*msgDef
}

type builtMsg struct {
    name  string
    data  []byte
    delay time.Duration
}

func loadMsgFile(path string) (*msgFile, error)
func (m *msgDef) build() ([]byte, error)
func (f *msgFile) buildSequence(names []string) ([]builtMsg, error)
func (f *msgFile) Run(ctx context.Context, lg *slog.Logger, conn gpsio.Conn, pCh <-chan scan.Packet, names []string) error
```

**2. TOML parsing**

Use `github.com/pelletier/go-toml/v2`. Parse into `map[string]msgDef`, extract `default` section, apply defaults to each definition. Validate required keys per type.

**3. Message building**

- `raw`: decode hex from `bin`, or use `ascii` as-is
- `crlf`/`cr`/`lf`: append appropriate line ending to `line`
- `nmea`: prepend `$` if missing, compute checksum via `nmea.Checksum`, format as `$data*XX\r\n`
- `multi`: expand recursively with cycle detection

**4. Message execution**

`Run()` takes a context, logger, `gpsio.Conn`, packet channel, and the list of message names:
1. Build message sequence via `buildSequence`
2. Start a goroutine to read from `pCh` and display responses
3. For each message: log the message name, write to port, sleep for delay
4. Wait briefly for final responses, then return

The packet channel is set up by the caller using `startScan()`, which spawns a goroutine running `gpsio.Scan`. Recognized packets (NMEA, etc.) are displayed in their native format; unrecognized data is shown as hex/ASCII.

**5. Add CLI flags to `gpsflags.go`**

Add `msgFile` and `msgNames` to `flagVars`. Add `--msg-file`/`-f` and `--msg`/`-m` flags. Validate that `-m` requires `-f` and cannot combine with other config flags.

**6. Update `gpscmd.go`**

Refactor `run()` to share setup/teardown:
1. Shared setup: defer `conn.Close()`, packet logging, `startScan()`
2. Branch: call either `runConfig()` or `runMsgs()` based on flags
3. Shared teardown: `conn.Stop()`, drain channel, `wg.Wait()`

Extract current config logic into `runConfig(ctx, lg, target, pCh, conn) (*gpscfg.Result, error)`.

Add `runMsgs(ctx, lg, mf *msgFile, pCh <-chan scan.Packet, conn gpsio.Conn, names []string) error` which calls `mf.Run()`.

**7. Tests**

Add `msgfile_test.go` with tests for parsing, building, multi expansion, and cycle detection.

#### File changes

| File | Change |
|------|--------|
| `internal/gpscmd/msgfile.go` | New: parsing, building, and execution |
| `internal/gpscmd/msgfile_test.go` | New: unit tests |
| `internal/gpscmd/gpsflags.go` | Add `-f` and `-m` flags |
| `internal/gpscmd/gpscmd.go` | Check for `-m` flag and call `msgFile.Run()` |

#### Dependencies

- `github.com/pelletier/go-toml/v2` (already in go.mod)
- `internal/nmea.Checksum` for NMEA checksum
- `internal/gpsio.Scan` for response display

### Stage 2: Protocol-specific types with ACK/NAK

Implement support for `ubx`, `cas`, and `pqtm` types that require response handling.

These types need the existing `Configurator` infrastructure for ACK/NAK processing, retries, and timeouts. A new method on `ConfigProtocol` accepts pre-built packets and returns a `Configurator` that manages them. The UBX implementation determines response expectations (ackable or not) by inspecting the packet's class/id.

This stage also requires a pack format parser to convert format strings like `"U1U2U4"` and value arrays into binary payloads, which are then wrapped with protocol framing (sync bytes, length, checksums) before being passed to the `Configurator`.
