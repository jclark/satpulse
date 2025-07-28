# uncanno Design Document

## Overview

`uncanno` is a command-line tool that annotates packet log files by decoding Unicore (UNCA and UNCB) packets. It reads JSONL packet log files, decodes Unicore packets found within them, and adds the decoded payload as a new field in the JSON output.

## Goals

1. Decode UNCA (ASCII) and UNCB (binary) Unicore packets from packet log files
2. Preserve the original JSON structure and field order by manually inserting the decoded payload field
3. Handle both known message types (PPSSTATUS, SATSINFO) and unknown message types
4. Stream processing - read from stdin, write to stdout

## Design Differences from ubxanno

Unlike `ubxanno`, which uses `json.Unmarshal` into a `map[string]interface{}` (which can reorder fields), `uncanno` will:

1. Read packet log entries using the `PacketLogEntry` struct from `internal/gpsio/log.go`
2. Manually insert the new `payload` field into the JSON byte slice to preserve field order
3. Avoid the complexity of handling configuration data (no equivalent to ubxcfgval)

## Implementation Strategy

### 1. Reading Packet Log Entries

Use `PacketLogEntry` struct to parse each JSON line:
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

### 2. Packet Identification

- UNCA packets: `tag == "UNCA"` with non-empty `ascii` field
- UNCB packets: `tag == "UNCB"` with non-empty `bin` field

### 3. Packet Decoding

Use existing functions from `internal/uncmsg/`:
- `ParseAsciiMessage()` for UNCA packets
- `ParseBinMsg()` for UNCB packets

Both return:
- `MessageHeader` - contains timing and CPU idle info
- `Msg` - the decoded message payload

### 4. JSON Field Insertion

To preserve field order and avoid json.Marshal/Unmarshal:
1. Find the position to insert the new fields (after existing fields, before closing `}`)
2. Build the new field strings: `,"header":{...},"payload":{...}`
3. Insert the new fields into the original JSON byte slice

Only add fields for known message types. For unknown messages (UnknownBinMsg or UnknownAsciiMsg), pass through unchanged as they don't provide any new information.

### 5. Handling Unknown Messages

For unknown message types:
- Do not add any fields to the JSON output
- Pass through unchanged

## Message Types

Currently supported Unicore message types:
- **PPSSTATUS** (ID: 9000) - PPS timing status information
- **SATSINFO** (ID: 2124) - Satellite tracking information (variable-length)

## Error Handling

- Invalid JSON lines are passed through unchanged
- Packets that fail to decode are passed through unchanged
- Non-UNCA/UNCB packets are passed through unchanged

## Example Input/Output

Input:
```json
{"t":"2024-01-01T12:00:00.123456Z","tag":"UNCB","bin":"aa44b50a0000...","msg":"PPSSTATUS"}
```

Output (known message):
```json
{"t":"2024-01-01T12:00:00.123456Z","tag":"UNCB","bin":"aa44b50a0000...","msg":"PPSSTATUS","header":{"CPUIdlePercent":95,"TimeRef":0,...},"payload":{"Status":3,"Week":2294,...}}
```

Output (unknown message):
```json
{"t":"2024-01-01T12:00:00.123456Z","tag":"UNCB","bin":"aa44b50a0000...","msg":"1234"}
```
(Passed through unchanged)

## Testing Considerations

1. Test with various message types (known and unknown)
2. Verify field order preservation
3. Test with malformed packets
4. Test with non-Unicore packets (should pass through)
5. Performance testing with large log files