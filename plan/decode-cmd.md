# Plan: Unified GPS Packet Decode Subcommand

## Goal

Replace standalone `ubxanno` and `uncanno` tools with a unified `satpulsetool decode` subcommand that decodes GPS packets. Supports two modes:

1. **Single packet mode**: Decode a hex packet from command line
2. **Packet log mode**: Annotate a JSONL packet log (stdin → stdout)

## Supported Formats

| Tag | Package | Parse Function | Output Fields |
|-----|---------|----------------|---------------|
| UBX | ubx/bin | `ParseMsg(string)` | `payload` (+`cfgData` for CFG messages) |
| CASBIN | casic/bin | `ParseMsg(string)` | `payload` |
| ASBIN | asbin | `ParseMsg(string)` | `payload` |
| UNCB | uncmsg | `ParseBinMsg([]byte)` | `header`, `payload` |
| NOVB | novmsg | `ParseBinMsg([]byte)` | `header`, `payload` |

ASCII formats (NMEA, UNCA, NOVA) and RTCM are passed through unchanged.

## Design

### Package: `internal/gpsdecode` (Domain layer, GPS group)

Domain layer package that decodes binary packets. Returns a strongly-typed struct for predictable JSON field ordering.

**Imports**:
- `internal/gpsreg` (for `TagUBX`, `TagCASICBin`, `TagAllystarBin`, etc.)
- `internal/ubx/bin`
- `internal/ubxcfgval` (for UBX CFG-VAL* cfgData decoding)
- `internal/casic/bin`
- `internal/asbin`
- `internal/uncmsg`
- `internal/novmsg`

**Does not import**: `ubx`, `casic`, `as`, `unc`, `nov` (domain layer protocol packages)

### Core API

```go
package gpsdecode

var (
    ErrUnknownFormat = errors.New("unknown packet format")
    ErrInvalidPacket = errors.New("invalid packet structure")
    ErrUnknownMsg    = errors.New("unknown message type")
)

// DecodeResult holds decoded packet fields in a fixed order for JSON serialization.
type DecodeResult struct {
    Payload any `json:"payload,omitempty"`
    Header  any `json:"header,omitempty"`
    CfgData any `json:"cfgData,omitempty"`
}

// Decode parses a packet and returns the PacketFormat and decoded fields.
// It uses scan.LooksLike to identify the format and gpsprot.IsValidPacket to validate.
// Returns PacketFormat so caller can get both Tag() and MsgID(data).
func Decode(pktFormats []gpsprot.PacketFormat, data []byte, out bool) (gpsprot.PacketFormat, *DecodeResult, error)
```

Returns:
- `(pf, result, nil)` on success
- `(nil, nil, ErrUnknownFormat)` when no PacketFormat matches the data
- `(nil, nil, ErrInvalidPacket)` when format matches but packet structure is invalid
- `(pf, nil, ErrUnknownMsg)` for unknown message types within a supported protocol
- `(pf, nil, err)` for parse errors from the underlying protocol package

### Implementation Pattern

```go
func Decode(pktFormats []gpsprot.PacketFormat, data []byte, out bool) (gpsprot.PacketFormat, *DecodeResult, error) {
    pf := scan.LooksLike(pktFormats, data)
    if pf == nil {
        return nil, nil, ErrUnknownFormat
    }
    if !gpsprot.IsValidPacket(pf, data) {
        return nil, nil, ErrInvalidPacket
    }
    switch pf.Tag() {
    case gpsreg.TagUBX:
        r, err := ubxbinDecode(data, out)
        return pf, r, err
    // ... other formats
    default:
        return pf, nil, ErrUnknownFormat
    }
}
```

Each format helper returns `*DecodeResult`:

```go
func ubxbinDecode(data []byte, out bool) (*DecodeResult, error) {
    msg, err := ubxbin.ParseMsg(string(data))
    if err != nil {
        return nil, err
    }
    if _, isUnknown := msg.(*ubxbin.UnknownMsg); isUnknown {
        return nil, ErrUnknownMsg
    }
    result := &DecodeResult{Payload: msg}
    // Handle cfgData for CFG-VAL* messages
    if cfgData := encodeCfgData(msg, out); cfgData != nil {
        result.CfgData = cfgData
    }
    return result, nil
}

func uncbinDecode(data []byte) (*DecodeResult, error) {
    msg, err := uncmsg.ParseBinMsg(data)
    if err != nil {
        return nil, err
    }
    if _, isUnknown := msg.Body.(*uncmsg.UnknownBinMsgBody); isUnknown {
        return nil, ErrUnknownMsg
    }
    return &DecodeResult{
        Payload: msg.Body,
        Header:  msg.Hdr,
    }, nil
}
```

### Command: `satpulsetool decode`

**Single packet mode**:
```bash
satpulsetool decode <hex-packet>
satpulsetool decode --out <hex-packet>   # for outgoing packets (affects CFG-VAL* decoding)
```

Flags:
- `--out` - Treat packet as outgoing (default: incoming). Affects CFG-VAL* decoding: outgoing has keys only, incoming has key-value items.

Output struct embeds DecodeResult for consistent field order:

```go
type output struct {
    Tag gpsprot.Tag `json:"tag"`
    Msg string      `json:"msg"`
    gpsdecode.DecodeResult
}
```

JSON field order: `tag`, `msg`, `payload`, `header`, `cfgData`

Errors are printed to stderr, not included in JSON output.

**Packet log mode** (with `--packet-log` flag):
```bash
satpulsetool decode --packet-log packet.jsonl > annotated.jsonl
satpulsetool decode --packet-log - < packet.jsonl > annotated.jsonl
```

Reads JSONL packet log from file (or stdin with `-`), annotates with decoded fields, writes to stdout. Uses `out` field from each log entry for direction.

Output preserves original field order by inserting decoded fields before the closing `}` (like uncanno), appending: `header`, `payload`, `cfgData` as applicable.

## Implementation Steps

### Step 1: Fix gpsdecode API

Modify `internal/gpsdecode/gpsdecode.go`:
- Add `DecodeResult` struct with fields in order: `Payload`, `Header`, `CfgData`
- Change `Decode` return type from `(gpsprot.Tag, map[string]any, error)` to `(gpsprot.PacketFormat, *DecodeResult, error)`
- Update all decode helpers to return `*DecodeResult`

Update `internal/gpsdecode/gpsdecode_test.go`:
- Update tests for new return type (use `pf.Tag()` instead of `tag`)

### Step 2: Implement decode for hex string

Create `internal/decodecmd/decodecmd.go` with single packet mode:
- `Cmd()` entry point with standard signature
- Flags: `--out` (bool, default false)
- Parse hex string argument
- Call `gpsdecode.Decode(gpsreg.PacketFormats, data, out)`
- Output struct embeds `DecodeResult`:
  ```go
  type output struct {
      Tag gpsprot.Tag `json:"tag"`
      Msg string      `json:"msg"`
      gpsdecode.DecodeResult
  }
  ```
- Marshal and print to stdout
- Errors printed to stderr using `cmd.ErrPrintln(progName, err)`

Update `cmd/satpulsetool/satpulsetool.go`:
- Add `case "decode":` to switch

### Step 3: Implement decode for packet log

Add `--packet-log PATH` flag to `internal/decodecmd/decodecmd.go`:
- Accept file path or `-` for stdin
- Read JSONL line by line
- Parse each line to extract `tag`, `bin`, `out` fields
- Call `gpsdecode.Decode()` with `entry.Out` for direction
- Preserve original field order by inserting decoded fields before closing `}` (like uncanno approach)
- Append fields in order: `header`, `payload`, `cfgData` (as applicable)
- On checksum error, append `checksumError` field:
  ```json
  {"t":"...","tag":"UBX","msg":"NAV-PVT","bin":"...","checksumError":{"inPacket":"deadbeef","computed":"beefdead"}}
  ```
- Pass through lines unchanged on other errors or for unsupported formats

## Files to Modify

- `internal/gpsdecode/gpsdecode.go`
- `internal/gpsdecode/gpsdecode_test.go`
- `cmd/satpulsetool/satpulsetool.go`

## Files to Create

- `internal/decodecmd/decodecmd.go`

## Verification

After Step 1:
```bash
go test ./internal/gpsdecode/...
```

After Step 2:
```bash
go build ./cmd/satpulsetool
satpulsetool decode b5620120...   # UBX packet
```

After Step 3:
```bash
satpulsetool decode --packet-log packet.ttyUSB0.jsonl | head
satpulsetool decode --packet-log - < packet.ttyUSB0.jsonl | head
```
