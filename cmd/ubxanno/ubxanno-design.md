# UBX Packet Log Annotator Design

## Problem

Packet logs contain hex-encoded UBX packets that are difficult to interpret manually. While the hex format is compact for storage and replay, it's unreadable for verification and debugging. This is especially challenging for UBX-CFG-VALSET/VALGET messages which require multi-level decoding.

## Solution

Create a standalone `ubxanno` command that reads JSONL packet logs and annotates UBX packets with decoded payload information. The tool adds a `"payload"` field containing the JSON representation of the decoded UBX message structure, with special handling for configuration messages.

## Multi-Level Decoding Challenge

UBX packets require different levels of decoding depending on message type:

### Standard UBX Messages
- **Level 1**: `ubx/bin` → Go struct (complete decoding)

### CFG-VALSET/VALGET Messages  
The `ubx/bin` decoding only provides the outer structure:
```go
type CfgValset struct {
    Version     uint8
    Layers      uint8
    Transaction uint8
    CfgData     []byte  // Raw configuration data - needs further decoding
}
```

Additional decoding levels required:
- **Level 1**: `ubx/bin` → Go struct with raw `CfgData []byte`
- **Level 2**: `ubxcfgval.UnmarshalItems(CfgData)` → List of `Item{Key, Value}` pairs  
- **Level 3**: `schema.Unmarshal(CfgData)` → Human-readable `map[string]map[string]any`

## Input/Output Format

### Input JSONL (PacketLogEntry format)
```jsonl
{"t": "2025-01-23T10:30:45.123Z", "tag": "UBX", "msg": "CFG-VALSET", "bin": "b5620a8a10000100020001003c005a010f00", "out": true}
{"t": "2025-01-23T10:30:45.150Z", "tag": "UBX", "msg": "NAV-PVT", "bin": "b56205013400deadbeef"}
{"type": "args", "args": ["gps", "--gnss", "gps+galileo", "--band", "L1"]}
```

### Output JSONL (Stage 1)
```jsonl
{"t": "2025-01-23T10:30:45.123Z", "tag": "UBX", "msg": "CFG-VALSET", "bin": "b5620a8a10000100020001003c005a010f00", "out": true, "payload": {"Version": 1, "Layers": 1, "Transaction": 0, "CfgData": "3c005a010f00"}}
{"t": "2025-01-23T10:30:45.150Z", "tag": "UBX", "msg": "NAV-PVT", "bin": "b56205013400deadbeef", "payload": {"iTOW": 123456, "fTOW": 789, "week": 2341}}
{"type": "args", "args": ["gps", "--gnss", "gps+galileo", "--band", "L1"]}
```

### Stage 2 Enhancement (CFG messages only)
For CFG-VALSET/VALGET messages, add `cfgData` field and remove binary `CfgData` from payload:
```jsonl
{"t": "2025-01-23T10:30:45.123Z", "tag": "UBX", "msg": "CFG-VALSET", "bin": "b5620a8a10000100020001003c005a010f00", "out": true, "payload": {"Version": 1, "Layers": 1, "Transaction": 0}, "cfgData": {...}}
```

## Implementation Strategy

### Staged Implementation

**Stage 1: Basic UBX Decoding (Level 1 only)**
- Implement core JSONL processing pipeline using `PacketLogEntry` format
- For entries with `"bin"` field, decode hex and parse with `ubx/bin` package 
- Add `"payload"` field with JSON-marshaled Go struct from `ubx/bin`
- Verify basic functionality with simple UBX messages

**Stage 2: CFG Message Enhancement (Levels 2-3)**  
- Add detection for CFG-VALSET/VALGET messages
- Implement `ubxcfgval` integration for configuration decoding
- Remove binary `CfgData` from payload, add separate `"cfgData"` field
- Handle schema-based human-readable configuration output

### Core Algorithm
1. Read JSONL line by line from stdin
2. Parse each line as JSON (using `PacketLogEntry` struct)
3. For lines containing a `"bin"` field (UBX packets):
   - **Level 1**: Decode hex string to bytes, parse using `ubx/bin` (assume valid UBX)
   - **Stage 1 output**: Add `"payload"` field with JSON-marshaled Go struct
   - **Level 2/3** (Stage 2): If CFG-VALSET/VALGET message:
     - Extract `CfgData` from struct
     - Use `ubxcfgval` to decode configuration data
     - Remove binary `CfgData` from payload, add separate `"cfgData"` field
   - For other UBX messages, just include Level 1 payload
   - Add/modify fields in PacketLogEntry
4. Write (possibly modified) JSON line to stdout

### Simplified Processing
- **No sync byte validation**: Assume packet logs contain valid UBX packets
- **No checksum verification**: Trust the packet log data integrity
- **Direct `ubx/bin` parsing**: Skip validation and go straight to decoding

### Error Handling
- Invalid JSON lines: pass through unchanged with optional warning
- UBX decode failures: pass through unchanged, optionally log decode error
- Malformed hex: pass through unchanged

### Dependencies
- `ubx/bin` package for Level 1 packet parsing
- `ubxcfgval` package for Level 2/3 configuration decoding
- Standard `encoding/json` for JSON marshaling
- Standard `encoding/hex` for hex decoding

## Usage Examples

### Basic annotation
```bash
ubxanno < packets.jsonl > packets-annotated.jsonl
```

### Pipeline with test log generation
```bash
satpulsetool gps --test-log - --gnss GPS --bands L1,L5 | ubxanno | jq .
```

### Verification workflow
```bash
# Generate test log
satpulsetool gps --test-log test.jsonl --signals gps+galileo

# Verify with readable output
ubxanno < test.jsonl | jq '.payload // empty'
```

### Filter for specific message types
```bash
ubxanno < test.jsonl | jq 'select(.payload.msgID == "CFG-VALSET")'
```

## Benefits

- **Multi-level decoding**: Handles complex CFG message structure automatically
- **Human-readable config**: Shows actual configuration values, not raw hex/keys
- **Unknown key tracking**: Highlights unrecognized configuration items
- **Non-destructive**: Original packet data preserved alongside decoded payload
- **Composable**: Works with existing packet logs and standard Unix tools
- **Simplified processing**: No validation overhead, direct decoding
- **Staged implementation**: Start simple, add complexity incrementally

## Future Enhancements

- Support for other protocols (NMEA, RTCM) if needed
- Command-line flags for filtering specific message types
- Pretty-printed output format (not just JSON)
- Integration into `satpulsetool` as a subcommand

## File Structure

```
cmd/ubxanno/
  ubxanno-design.md    # this document
  ubxanno.go          # main implementation
```

The tool is standalone and can be used independently of the packet replay testing system, making it useful for general UBX packet log analysis.