# NovAtel Message Support Design Document

## Problem Statement

We need to add support for NovAtel format messages in SatPulse alongside the existing Unicore support. While Unicore's protocol is historically derived from NovAtel, they have diverged in important ways that create architectural challenges for code reuse.

## Background

### Protocol Relationship
- Unicore protocol is historically derived from NovAtel OEM7 protocol
- Both support dual ASCII/binary message formats with similar structure
- Both use the same CRC32 algorithm for checksums
- Significant overlap in functionality and message semantics

### Current Implementation Status

#### Unicore Support (Complete)
- `internal/unc/` - Domain layer with PacketFormat implementations
  - `asciipacket.go` - UNCA ASCII packet format
  - `binpacket.go` - UNCB binary packet format
- `internal/uncmsg/` - Library layer for message parsing
  - Independent of `gpsprot` layer
  - Handles both ASCII and binary message parsing
  - Contains message definitions (VERSION, RECTIME, SATSINFO, etc.)

#### NovAtel Support (Started)
- `internal/nov/binpacket.go` - Binary packet format partially implemented
  - Different sync bytes: 0xAA 0x44 0x12 (vs Unicore's 0xAA 0x44 0xB5)
  - Different header structure (28 bytes vs 24 bytes)
  - Currently reuses `uncmsg.CRC32()` for checksums

## Key Differences Between Protocols

### 1. Binary Packet Headers

**NovAtel Binary (28 bytes):**
- Sync: 0xAA 0x44 0x12
- Header length field at offset 3
- Message ID at offset 4 (2 bytes)
- Port address at offset 7
- Variable header length (specified in byte 3)

**Unicore Binary (24 bytes):**
- Sync: 0xAA 0x44 0xB5
- Fixed 24-byte header
- Message ID at offset 4 (2 bytes)
- No port address field

### 2. ASCII Message Headers

**NovAtel ASCII Header Fields:**
```
#MessageName,Port,Sequence,IdleTime,TimeStatus,Week,Seconds,ReceiverStatus,Reserved,Version;data*checksum
```
Example:
```
#BESTPOSA,COM3,0,0.0,FINESTEERING,1975,393343.000,00000000,0000,113;...
```

**Unicore ASCII Header Fields:**
```
#MessageName,CPUIdle,TimeRef,TimeStatus,Week,Ms,Version,Reserved,LeapSec,OutputDelay;data*checksum
```
Example:
```
#PPSSTATUSA,93,GPS,FINE,2376,540337000,0,0,18,29;...
```

**Key Differences:**
- Field 2: Port name (NovAtel) vs CPU idle % (Unicore)
- Field 3: Sequence number (NovAtel) vs Time reference (Unicore)
- Field order and meanings differ significantly
- NovAtel uses floating point idle time, Unicore uses integer
- Both use same checksum algorithms (8-bit XOR or 32-bit CRC)

### 3. Message Identification

**Distinguishing ASCII formats:**
- Field after message name reveals protocol:
  - NovAtel: Non-numeric port identifier (e.g., "COM1", "COM3")
  - Unicore: Numeric CPU idle percentage

**Message ID namespace:**
- Binary message IDs overlap but have different meanings
- ASCII message names may overlap with different semantics
- Example: Both have "TIME" message but with different IDs and formats

### 4. Special Format: NovAtel Abbreviated ASCII

NovAtel also supports "Abbreviated ASCII" format (lines starting with `<`):
- Used by LOGLIST command output
- Not present in Unicore protocol
- Already referenced in Unicore implementation for baud rate configuration

## Detailed Header Specifications

### NovAtel Binary Header Structure

**Table: Standard Binary Header Structure (28 bytes minimum)**

| ID | Field | Description | Type | Bytes | Offset |
|----|-------|-------------|------|-------|--------|
| 1 | Sync | Hexadecimal 0xAA | Char | 1 | 0 |
| 2 | Sync | Hexadecimal 0x44 | Char | 1 | 1 |
| 3 | Protocol type | 0x12 (bits: 4=binary, 7-8=10 for standard) | Char | 1 | 2 |
| 4 | Header length | Header length in bytes | UChar | 1 | 3 |
| 5 | Message ID | Message ID | UShort | 2 | 4 |
| 6 | Message Type | Bits: 0-4=source, 5-6=format, 7=response | Char | 1 | 6 |
| 7 | Port Address | Port identifier (see port table below) | UChar | 1 | 7 |
| 8 | Message Length | Body length (excludes header and CRC) | UShort | 2 | 8 |
| 9 | Sequence | For multiple related logs (counts down) | UShort | 2 | 10 |
| 10 | Idle Time | CPU idle percentage | UChar | 1 | 12 |
| 11 | Time Status | Time quality (Unknown/Fine) | Enum | 1 | 13 |
| 12 | Week | GNSS week number | UShort | 2 | 14 |
| 13 | ms | Milliseconds from week start | GPSec | 4 | 16 |
| 14 | Receiver Status | 8-digit hex status | ULong | 4 | 20 |
| 15 | Reserved | Reserved | UShort | 2 | 24 |
| 16 | Receiver S/W Version | Build number (0-65535) | UShort | 2 | 26 |

**Port Identifiers:**

| Hex | Dec | Description |
|-----|-----|-------------|
| 0x00 | 0 | NO_PORTS |
| 0x01 | 1 | COM1 |
| 0x02 | 2 | COM2 |
| 0x03 | 3 | COM3 |
| 0x04 | 4 | THISPORT |
| 0x05 | 5 | FILE |
| 0x06 | 6 | ALL_PORTS |
| 0x11 | 17 | ETH1 |
| IMU, ICOM1-4, NCOM1-3, CCOM1-3, MCOM1-4 | Various | Additional ports |

### NovAtel ASCII Header Structure

**Format:** `#header;data_field,data_field,data_field*xxxxxxxx[CR][LF]`

**Table: ASCII Header Fields**

| ID | Field | Type | Description | Optional |
|----|-------|------|-------------|----------|
| 1 | Sync | Char | Always '#' | No |
| 2 | Message | Char | ASCII message name | No |
| 3 | Port | Char | Port name (e.g., "COM3", "COM1_1") | Yes |
| 4 | Sequence# | Long | Countdown for related logs (0 = last) | No |
| 5 | %Idle Time | Float | CPU idle percentage | Yes |
| 6 | Time Status | Enum | Time quality (Unknown/Fine/FINESTEERING) | Yes |
| 7 | Week | ULong | GPS week number | Yes |
| 8 | Seconds | GPSec | Seconds from week start (ms precision) | Yes |
| 9 | Receiver Status | ULong | 8-digit hex status | Yes |
| 10 | Reserved | ULong | Reserved | Yes |
| 11 | Receiver S/W Version | ULong | Build number (0-65535) | Yes |
| 12 | ; | Char | Header terminator | No |

**Examples:**
```
#BESTPOSA,COM3,0,0.0,FINESTEERING,1975,393343.000,00000000,0000,113;...
#HEADINGA,COM3,0,0,FINESTEERING,1975,394129.000,00000000,0000,113;...
```

## Proposed Solution

### Distinct ASCII Packet Formats

Make the ASCII packet formats distinguishable at the PacketFormat level by examining the character after the first comma:

**Unicore ASCII Detection:**
- Pattern conceptually: `#[A-Z][A-Z0-9_]*,[0-9]`
- The character after the first comma must be numeric (CPU idle percentage)
- Example: `#PPSSTATUSA,93,GPS,FINE,...`

**NovAtel ASCII Detection:**
- Pattern conceptually: `#[A-Z][A-Z0-9_]*,[A-Z]`  
- The character after the first comma must be alphabetic (port name)
- Example: `#BESTPOSA,COM3,0,0.0,...`

This allows the packet scanner to definitively identify the protocol before any parsing occurs.

### State Machine Implementation

Since the packet scanners use state machines rather than regex, we need to add states to enforce these constraints:

**Updated Unicore ASCII State Machine (`internal/unc/asciipacket.go`):**
```go
const (
    asciiStateSync        // Looking for '#'
    asciiStateStarted     // Seen '#', reading message name
    asciiStateHadComma    // NEW: Seen first comma, need to check next char
    asciiStateBeforeSemi  // NEW: Validated numeric, reading rest of header
    asciiStateHadSemi     // Found semicolon
    // ... checksum states ...
)

func (f asciiPacketFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
    b := buf[nextScanIndex]
    
    switch state {
    case asciiStateStarted:
        if b == ',' {
            return asciiStateHadComma  // Need to validate next character
        }
        if !isPrintableAscii(b) {
            return asciiStateSync
        }
        return asciiStateStarted
    
    case asciiStateHadComma:
        // CRITICAL: Must be numeric for Unicore
        if b >= '0' && b <= '9' {
            return asciiStateBeforeSemi  // Valid, continue reading header
        }
        return asciiStateSync  // Reject if not numeric
    
    case asciiStateBeforeSemi:
        if b == ';' {
            return asciiStateHadSemi
        }
        if !isPrintableAscii(b) {
            return asciiStateSync
        }
        return asciiStateBeforeSemi
    // ... rest of states ...
}
```

**NovAtel ASCII State Machine (`internal/nov/asciipacket.go`):**
```go
const (
    asciiStateSync        // Looking for '#'
    asciiStateStarted     // Seen '#', reading message name
    asciiStateHadComma    // NEW: Seen first comma, need to check next char
    asciiStateBeforeSemi  // NEW: Validated alphabetic, reading rest of header
    asciiStateHadSemi     // Found semicolon
    // ... checksum states ...
)

func (f asciiPacketFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
    b := buf[nextScanIndex]
    
    switch state {
    case asciiStateHadComma:
        // CRITICAL: Must be alphabetic for NovAtel
        if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
            return asciiStateBeforeSemi  // Valid, continue reading header
        }
        return asciiStateSync  // Reject if not alphabetic
    // ... rest of states ...
}
```

This approach:
- Maintains the efficient state machine architecture
- Adds minimal overhead (one extra state transition)
- Provides definitive protocol detection
- Rejects mis-routed packets early

### Dependency Architecture

Create a dependency hierarchy that reflects the historical derivation:

```
internal/novmsg/  (base library - NovAtel message parsing)
    ├── CRC32 algorithm
    ├── Common data structures
    ├── NovAtel message definitions
    └── Shared utilities

internal/uncmsg/  (extends novmsg - Unicore message parsing)
    ├── Depends on: novmsg
    ├── Unicore-specific message definitions
    ├── Reuses CRC32 from novmsg
    └── Unicore header parsing

internal/nov/  (domain layer - NovAtel protocol)
    ├── Depends on: novmsg
    ├── Implements gpsprot interfaces
    ├── NOVA ASCII PacketFormat
    ├── NOVB Binary PacketFormat
    └── NOVAA Abbreviated ASCII PacketFormat

internal/unc/  (domain layer - Unicore protocol)
    ├── Depends on: uncmsg, novmsg
    ├── Implements gpsprot interfaces
    ├── UNCA ASCII PacketFormat (stricter pattern)
    ├── UNCB Binary PacketFormat
    └── Can reuse NovAtel message formats where compatible
```

### Key Design Benefits

1. **Clean Protocol Detection:** ASCII formats are distinguished by regex at scan time
2. **Historical Accuracy:** Dependency structure reflects Unicore's derivation from NovAtel
3. **Maximum Code Reuse:** Shared algorithms and structures live in novmsg
4. **No Cross-Layer Violations:** Library layer (novmsg/uncmsg) remains independent of domain layer
5. **Extensibility:** Clear pattern for adding more GNSS protocols

### Implementation Steps

1. **Create `internal/nov/asciipacket.go`:**
   - Implement NOVA PacketFormat with state machine
   - Similar to existing unc implementation
   - Add `asciiStateHadComma` and `asciiStateBeforeSemi` states
   - In `asciiStateHadComma`, check if next char is alphabetic
   - If alphabetic, transition to `asciiStateBeforeSemi`; otherwise reject
   - Factor out Next into a function that takes an argument saying whether first field after message name is digit

2. **Update `internal/unc/asciipacket.go`:**
   - Use Next function from nov.
   - Test using existing test cases

3. **Create `internal/novmsg/` package:**
   - Move CRC32 from uncmsg to novmsg
   - Define NovAtel header structures
   - Implement NovAtel message parsing based on unc message parsing
   - Implement Time message
   - Factor out common functions that can be used by uncmsg

4. **Refactor `internal/uncmsg/`:**
   - Use common functions from novmsg
   - Keep Unicore-specific logic
   - Reuse novmsg.Time

5. **Implement packet processors in `internal/nov`**
   - Base on `internal/unc`
   - Factor out common functions

6. **Refactor `internal/unc/`:**
   - Use common functions in `internal/nov`


### Example: TIME Message Reuse

The TIME message demonstrates the reuse pattern with proper separation of header and payload processing:

1. **Base Definition in `novmsg`:**
   ```go
   // internal/novmsg/time.go
   type Time struct {
       ClockStatus ClockStatus
       Offset      float64
       OffsetStd   float64
       UTCOffset   float64
       UTCYear     uint32
       UTCMonth    uint8
       // ... other fields
   }
   
   func (t *Time) ID() (MsgID, string) {
       return TimeID, "TIMEA"  // NovAtel's ID
   }
   ```

2. **Unicore Override in `uncmsg`:**
   ```go
   // internal/uncmsg/time.go
   type RecTime struct {
       novmsg.Time  // Embed NovAtel's Time
   }
   
   func (r *RecTime) ID() (MsgID, string) {
       return RecTimeID, "RECTIMEA"  // Unicore's different ID
   }
   ```

3. **Shared Payload Conversion in `nov`:**
   ```go
   // internal/nov/time.go
   // Populates TimeMsg fields from Time payload (no header processing)
   func PopulateTimePayload(t *gpsprot.TimeMsg, m *novmsg.Time) error {
       if m.UTCStatus == novmsg.UTCStatusValid {
           nanos := int32(m.UTCMs%1000) * 1e6
           utc := ptime.UTC(uint16(m.UTCYear), m.UTCMonth, m.UTCDay, 
                           m.UTCHour, m.UTCMin, uint8(m.UTCMs/1000), nanos)
           t.UTCTime = &utc
           t.UTCOffset = convertUTCOffset(m.UTCOffset)
           if t.UTCOffset == 0 {
               return fmt.Errorf("invalid UTC offset %f", m.UTCOffset)
           }
       }
       t.Accuracy = convertAccuracy(m.OffsetStd)
       return nil
   }
   ```

4. **NovAtel Complete Processing in `nov`:**
   ```go
   // internal/nov/time.go
   func TimeToTimeMsg(header novmsg.MessageHeader, m *novmsg.Time, tag gpsprot.Tag) (*gpsprot.TimeMsg, error) {
       if m.ClockStatus != novmsg.ClockStatusValid {
           return nil, nil
       }
       
       t := gpsprot.TimeMsg{
           Tag:         tag,
           NativeMsgID: "TIME",
       }
       
       // Protocol-specific header processing for TAI time
       gnss, toTAI := timeRefToTAI(header.TimeStatus, header.Week)
       t.GNSS = gnss
       if toTAI != nil && header.TimeStatus == novmsg.TimeStatusFine {
           tow := time.Duration(header.Seconds) * time.Second
           t.TAITime = toTAI(int16(header.Week), tow)
       }
       
       // Populate from payload
       if err := PopulateTimePayload(&t, m); err != nil {
           return nil, err
       }
       
       return &t, nil
   }
   ```

5. **Unicore Reuse in `unc`:**
   ```go
   // internal/unc/time.go
   import "github.com/jclark/satpulse/internal/nov"
   
   func timeRecTime(header uncmsg.MessageHeader, m *uncmsg.RecTime, tag gpsprot.Tag) (*gpsprot.TimeMsg, error) {
       if m.ClockStatus != uncmsg.ClockStatusValid {
           return nil, nil
       }
       
       t := gpsprot.TimeMsg{
           Tag:         tag,
           NativeMsgID: "RECTIME",
       }
       
       // Protocol-specific header processing for TAI time
       gnss, toTAI := timeRefToTAI(header.TimeRef)  // Different header field
       t.GNSS = gnss
       if toTAI != nil && header.TimeStatus == uncmsg.TimeStatusFine {
           tow := time.Duration(header.MillisecondsOfWeek) * time.Millisecond
           t.TAITime = toTAI(int16(header.Week), tow)
       }
       
       // Reuse NovAtel's payload processing
       if err := nov.PopulateTimePayload(&t, &m.Time); err != nil {
           return nil, err
       }
       
       return &t, nil
   }
   ```

This refined pattern:
- **Separates concerns**: Header processing (protocol-specific) vs payload processing (shared)
- **Header differences**: NovAtel uses TimeStatus/Seconds, Unicore uses TimeRef/MillisecondsOfWeek
- **Shared payload logic**: UTC time, offset, and accuracy conversion in `novmsg`
- **Avoids false sharing**: TAI time generation must be protocol-specific due to different header formats
- **Maintains clarity**: Each protocol handles its own header interpretation

## Architectural Challenges (Resolved)

The proposed solution addresses all major challenges:

### 1. Packet Format Detection ✓
- Solved by distinct regex patterns at the PacketFormat level
- No ambiguity between protocols

### 2. Code Reuse vs Separation ✓
- Shared code in novmsg (CRC32, common structures)
- Protocol-specific code in respective packages
- Clear dependency hierarchy

### 3. Layer Architecture Constraints ✓
- Library layer (novmsg/uncmsg) independent of domain layer
- Domain layer (nov/unc) implements gpsprot interfaces
- Dependencies flow in one direction

### 4. Message Processing Pipeline ✓
- Each protocol has its own PacketFormat (no mis-routing)
- Each protocol has its own message parser
- Shared components where appropriate

## Design Constraints

### Must Support ✓
1. NovAtel binary packet format (NOVB) - via nov/binpacket.go
2. NovAtel ASCII packet format (NOVA) - via nov/asciipacket.go  
3. NovAtel abbreviated ASCII - via nov/abbrevasciipacket.go
4. Maximum code reuse - via novmsg base package
5. Clean architectural separation - via dependency hierarchy

### Must Maintain ✓
1. Layer independence - preserved with novmsg/uncmsg in library layer
2. Existing Unicore functionality - unchanged, just stricter detection
3. Protocol detection accuracy - improved with distinct patterns
