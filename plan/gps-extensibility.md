# GPS configuration extensibility

This is issue #200.

Allow users to extend GPS configuration beyond what satpulsetool natively supports.

## Motivation

`satpulsetool gps` provides protocol-agnostic configuration, but:

- there will always be protocol-specific details that cannot be abstracted
- only UBX and Unicore protocols are currently supported; adding new protocols requires significant work
- technical users are comfortable reading their receiver's manual and constructing exact commands

## Stage 1: Fire-and-forget message types

Stage 1 provides simple message sending without response validation. Users can send raw bytes or line-based commands with automatic framing. The tool displays any responses received but doesn't programmatically verify success.

### Design

#### Message file format

Messages are defined in a TOML file with named sections. Each section defines a named message; the section name becomes the message identifier used with `-m`.

Each message has a `type` key that determines the valid keys in the section and the framing applied:

```toml
[has]
type = "crlf"
line = "CONFIG PPP ENABLE E6-HAS"
```

**Types:**

| Type | Keys | Framing |
|----------|------|---------|
| `"raw"` | `bin` or `ascii` | none |
| `"crlf"` | `line` | adds `\r\n` |
| `"cr"` | `line` | adds `\r` |
| `"lf"` | `line` | adds `\n` |
| `"nmea"` | `data` | prepends `$` if missing; computes checksum (`*XX`); appends `\r\n` |
| `"multi"` | `msgs` | n/a (expands to referenced messages) |

**Content keys by type:**

| Key | Description |
|-----|-------------|
| `line` | ASCII string (for `crlf`, `cr`, `lf` types) |
| `data` | NMEA sentence content after `$` (for `nmea` type) |
| `ascii` | Exact byte sequence as ASCII string (for `raw` type) |
| `bin` | Exact byte sequence as hex string (for `raw` type) |
| `msgs` | Array of message names (for `multi` type) |

**Common options:**

| Key | Description |
|-----|-------------|
| `delay` | Delay in seconds after sending this message before sending the next (default: 0) |

**Defaults:**

The special `[default]` section sets default values for all messages:

```toml
[default]
type = "nmea"

[gps-bds]
data = "PCAS04,3"
```

Per-message options override the defaults. Only `type` and `delay` are allowed in `[default]`.

#### CLI interface

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

**Constraints:**

The `-m` flag cannot be combined with other config flags like `--gnss`, `--pps`, etc. This avoids issues with `--save` semantics and ambiguity about the relative ordering of manual messages versus higher-level configuration.

When `-m` is used without `--capture`, the tool defaults to `--capture 2` to show receiver responses.

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

**CASIC receiver with NMEA-style commands:**

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

**Raw binary message:**

```toml
# raw-example.toml
[wake]
type = "raw"
bin = "B562060001"

[text-cmd]
type = "raw"
ascii = "HELLO"
```

**Multi-message grouping:**

```toml
# multi-example.toml
[default]
type = "crlf"

[enable-has]
line = "CONFIG PPP ENABLE E6-HAS"

[enable-sg2]
line = "CONFIG SIGNALGROUP 2"

[full-setup]
type = "multi"
msgs = ["enable-has", "enable-sg2"]
```

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -f multi-example.toml -m full-setup
```

### Implementation

These types don't require protocol-specific response handling. There is no ACK/NAK, retries, or programmatic success/failure determination. `satpulsetool gps` parses the TOML file, constructs the framed bytes for each message, writes them to the serial port with appropriate delays, and displays any responses received.

Response display uses the existing `gpsio.Scan` infrastructure to read packets. Recognized packets (NMEA, etc.) are displayed in their native format; unrecognized data is shown as ASCII lines. This gives users some visibility into receiver responses.

This stage is self-contained within `satpulsetool gps` and doesn't require changes to the `ConfigProtocol` or `Configurator` interfaces.

#### Implementation steps

**1. Add `internal/gpscmd/msgfile.go`**

New file for TOML message file parsing and message construction. Keeps parsing/building separate from execution.

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

type BuiltMsg struct {
    Name  string
    Data  []byte
    Delay time.Duration
}

func loadMsgFile(path string) (*msgFile, error)
func (m *msgDef) build() ([]byte, error)
func (f *msgFile) BuildSequence(names []string) ([]BuiltMsg, error)
```

**2. TOML parsing**

Use `github.com/pelletier/go-toml/v2`. Parse into `map[string]msgDef`, extract the special `[default]` section, and apply defaults to each definition. Pragmatic constraint: only `type` and `delay` are allowed in `[default]`. Merge rule: defaults only fill unset fields; per-message values always take precedence. Validate required keys per type.

**3. Message building**

- `raw`: require exactly one of `bin` or `ascii`; `bin` is an even-length hex string (no spaces, no `0x` prefixes) to decode, and `ascii` is used as-is
- `crlf`/`cr`/`lf`: append appropriate line ending to `line`
- `nmea`: prepend `$` if missing, compute checksum via `nmea.Checksum`, format as `$data*XX\r\n`
- `multi`: expand recursively with cycle detection

**4. Add CLI flags to `gpsflags.go`**

Add `msgFilePath` and `msgNames` to `flagVars`. Add `--msg-file`/`-f` and `--msg`/`-m` flags. When `-m` is used without `--capture`, default to `--capture 2`.

In message mode (`-m` specified), only connection and logging flags are allowed:
- Allowed: `-d`/`--serial-device`, `-s`/`--device-speed`, `--socket`, `--packet-log`, `--capture`
- Rejected: any flag that mutates receiver config (`configChanged`) or triggers actions (`--save`, `--save-all`, `--reset`, `--reload`, `--factory-reset`, `--show-config`)

Use `flags.Changed()` to detect if any disallowed flag was explicitly set. The existing `configChanged` variable already tracks config-mutating flags.

**5. Update `gpscmd.go`**

Add `runMsgs()` to handle the `-m` branch:

```go
func runMsgs(ctx context.Context, lg *slog.Logger, conn gpsio.Conn, msgs []BuiltMsg) error
```

For each message: log the name, write to port, sleep for delay. Response display is handled by `--capture` (issue #197).

**6. Tests**

Add `msgfile_test.go` with tests for parsing, building, multi expansion, and cycle detection.

#### File changes

| File | Change |
|------|--------|
| `internal/gpscmd/msgfile.go` | New: parsing and building |
| `internal/gpscmd/msgfile_test.go` | New: unit tests |
| `internal/gpscmd/gpsflags.go` | Add `-f` and `-m` flags |
| `internal/gpscmd/gpscmd.go` | Add `runMsgs()`, branch on `-m` flag |

#### Dependencies

- Issue #197 (`--capture` flag) must be implemented first
- `github.com/pelletier/go-toml/v2` (already in go.mod)
- `internal/nmea.Checksum` for NMEA checksum

## Stage 2: Protocol-specific types with ACK/NAK

Stage 2 extends the message file format with types that require response validation: UBX, CASIC, and Quectel PQTM protocols. These types leverage the existing `Configurator` infrastructure for ACK/NAK processing, retries, and timeouts.

### Design

Stage 2 adds three new message types to the format defined in Stage 1:

| Type | Keys | Framing | Response |
|----------|------|---------|----------|
| `"ubx"` | `class`, `id`, `pack`, `payload` | UBX framing (sync bytes, length, checksums); little-endian | expects ACK/NAK |
| `"cas"` | `class`, `id`, `pack`, `payload` | CASIC framing; payload must be multiple of 4 bytes | expects ACK/NAK |
| `"pqtm"` | `data` | prepends `$PQTM`; computes checksum; appends `\r\n` | expects `$PQTM{cmd},OK*XX` |

**Additional content keys:**

| Key | Description |
|-----|-------------|
| `class` | Message class (for `ubx`, `cas` types) |
| `id` | Message ID (for `ubx`, `cas` types) |
| `pack` | Pack format string, e.g. `"U1U2U4"` (for `ubx`, `cas` types) |
| `payload` | Array of values to pack (for `ubx`, `cas` types) |
| `data` | Content after `$PQTM` (for `pqtm` type) |

**Pack specifiers** (following u-blox notation):
- `U1`, `U2`, `U4` - unsigned integer (1, 2, 4 bytes)
- `I1`, `I2`, `I4` - signed integer
- `X1`, `X2`, `X4` - bitfield (same encoding as unsigned)
- `R4`, `R8` - IEEE 754 float (4 bytes) and double (8 bytes)

### Examples

**u-blox UBX message file:**

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

**Quectel PQTM message file:**

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

**Mixed message file:**

A single file can contain both fire-and-forget and ACK/NAK message types:

```toml
# mixed.toml
[nmea-reset]
type = "nmea"
data = "PMTK104"

[ubx-config]
type = "ubx"
class = 0x06
id = 0x8A
pack = "U1U1U2U4U1"
payload = [0, 1, 0, 0x10320001, 1]
```

### Implementation

These types need the existing `Configurator` infrastructure for ACK/NAK processing, retries, and timeouts. A new method on `ConfigProtocol` accepts pre-built packets and returns a `Configurator` that manages them. The UBX implementation determines response expectations (ackable or not) by inspecting the packet's class/id.

This stage requires:

1. **Pack format parser**: Convert format strings like `"U1U2U4"` and value arrays into binary payloads
2. **Protocol framing**: Wrap payloads with sync bytes, length, and checksums
3. **Configurator integration**: Use existing retry/timeout logic for ACK/NAK handling

#### Open design issues

##### Customizable response handling for line types

For line-based types (`crlf`, `cr`, `lf`, `nmea`), consider adding optional keys to expect a response, e.g., a line matching a regexp. Currently these are fire-and-forget.
