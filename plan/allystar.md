# Allystar Binary Protocol Integration Plan

## Current State

### On master (after merge):
- `internal/as/asnmea.go` - NMEA satellite numbering for Allystar (already integrated with gpsreg)
- `internal/asbin/asbin.go` - Message parsing/serialization (498 lines)
- `internal/asbin/asbin_test.go` - Tests with example packets from spec (641 lines)

### What asbin.go already has:
- Sync bytes: `0xF1 0xD9`
- Message ID encoding: `MsgID = class | (id << 8)`
- Fletcher-8 checksum (same as UBX)
- `ParseMsg()` - parse packet to message struct
- `Serialize()` - message struct to packet
- Message types: NavTime, NavClock, AckAck, AckNak, CfgPps, CfgPrt, CfgMsg, CfgNavSat, CfgSurvey, CfgFixedECEF, CfgCfg, CfgSimpleRst, MonVer
- Poll functions: `Poll()`, `PollNavTime()`, `PollPrt()`, `PollCfgMsg()`, `SetCfgMsg()`

### What's missing (need to add):
1. `PacketFormat` - for packet scanning/recognition by gpsio
2. `PacketProcessor` - for integration with gpsevent
3. Time conversion - NavTime → gpsprot.TimeMsg
4. Registration in gpsreg

## Packet Format

From the Allystar spec:
```
| Start    | Message ID      | Payload  | Payload    | Checksum |
| sequence | Group + Sub     | length   |            |          |
| F1 D9    | class(1)+id(1)  | len(2)   | 0-N bytes  | CK1,CK2  |
```

Compared to UBX:
```
| Sync     | Class | ID | Length | Payload | Checksum |
| B5 62    | 1     | 1  | 2      | N       | 2        |
```

**Key difference from CASIC**: Allystar has class/id before length (like UBX), while CASIC has length before class/id.

## Implementation Steps

### Step 1: Create internal/as/aspacket.go (package as)

Implement `gpsprot.PacketFormat`. Based on ubxpacket.go since header layout matches.

**Exports needed:**
```go
const Tag gpsprot.Tag = "ASBIN"
var PacketFormat gpsprot.PacketFormat = packetFormat{}
```

**State machine constants:**
```go
// Packet: sync(2) + class(1) + id(1) + len(2) + payload + checksum(2)
const (
    stateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
    stateSync2
    stateClass
    stateID
    stateLenLo
    stateLenHi
    stateBody
)
```

**Methods to implement:**
- `Tag() gpsprot.Tag` - return Tag
- `Next(state, buf, nextScanIndex, packetLen) gpsprot.ScanState` - state machine
- `IsFinal(state) bool` - return state == stateBody
- `MsgID(pkt []byte) string` - return asbin.PacketMsgId(pkt).String()
- `ExtractChecksum(pkt []byte) []byte` - return pkt[len(pkt)-2:]
- `ComputeChecksum(pkt []byte) []byte` - call asbin.checksum(pkt[2:len(pkt)-2])
- `RescanOnBadChecksum(bool, []byte) bool` - return false

**State machine logic:**
1. stateSync: if byte == 0xF1 (asbin.Sync1) → stateSync2, else → stateSync
2. stateSync2: if byte == 0xD9 (asbin.Sync2) → stateClass, else → stateSync
3. stateClass: → stateID
4. stateID: → stateLenLo
5. stateLenLo: → stateLenHi
6. stateLenHi: compute payloadLen from buf[4:6], return stateBody + payloadLen + 2
7. state > stateBody: return state - 1 (counting down)

**Checksum range:** bytes [2:len-2] (class through payload, excluding sync and checksum)

**Note:** `asbin.checksum()` is currently unexported (line 492). Options:
1. Export it as `asbin.Checksum()` (preferred - avoids duplication)
2. Duplicate the Fletcher-8 algorithm in aspacket.go

Fletcher-8 algorithm (from asbin.go):
```go
func checksum[B Bytes](bytes B) (ckA, ckB byte) {
    for i := 0; i < len(bytes); i++ {
        ckA += bytes[i]
        ckB += ckA
    }
    return
}
```

### Step 2: Create internal/as/asproc.go (package as)

Implement `gpsprot.PacketProcessor`.

**Exports needed:**
```go
func NewPacketProcessor() *PacketProcessor
```

**Structure:**
```go
type PacketProcessor struct {
    gpsprot.DefaultPacketProcessor
    mh gpsprot.MsgHandler
}
```

**Methods to implement:**
- `SetMsgHandler(handler gpsprot.MsgHandler)` - store handler
- `ProcessPacket(data string, tRead time.Time) (string, error)` - parse and dispatch

**ProcessPacket logic:**
1. Call `asbin.ParseMsg(data)` to get message struct
2. Get msgID string via `asbin.PacketMsgId([]byte(data)).String()`
3. Switch on message type:
   - `*asbin.NavTime` → call `timeNavTime()` then `mh.Time(tm, tRead)`
4. For unhandled messages, optionally call native handler

### Step 3: Create internal/as/astime.go (package as)

Convert `asbin.NavTime` to `gpsprot.TimeMsg`.

**Function:**
```go
func timeNavTime(m *asbin.NavTime) *gpsprot.TimeMsg
```

**Conversion logic:**
1. Check validity flags (m.Flags & asbin.NavTimeFlagWeekValid, etc.)
2. Convert time of week: `tow := ptime.Milliseconds(m.RefTow) + ptime.Nanoseconds(m.Fractow)`
3. Convert based on NavSys:
   - 0 (GPS): `ptime.GPS(int(m.Week), tow)`, GNSS = gpsprot.GPS
   - 1 (BeiDou): `ptime.BeiDou(int(m.Week), tow)`, GNSS = gpsprot.BDS
   - 2 (GLONASS): needs different handling (GLONASS uses different epoch)
   - 3 (Galileo): `ptime.Galileo(int(m.Week), tow)`, GNSS = gpsprot.GAL
4. Set Accuracy from TimeErr (nanoseconds → time.Duration)
5. Set NativeMsgID = "NAV-TIME"
6. Set Tag = Tag

**NavTimeSys values (from asbin):**
```go
NavTimeSysGPS      = 0
NavTimeSysBeiDou   = 1
NavTimeSysGLONASS  = 2
NavTimeSysGalileo  = 3
```

**NavTimeFlags (from asbin):**
```go
NavTimeFlagWeekValid    = 1 << 0
NavTimeFlagSecondValid  = 1 << 1
NavTimeFlagLeapSecValid = 1 << 2
```

### Step 4: Update internal/gpsreg/reg.go

1. Import already exists for `"github.com/jclark/satpulse/internal/as"`

2. Add to `PacketFormats` slice (line ~19):
```go
var PacketFormats = []gpsprot.PacketFormat{
    ubx.PacketFormat,
    casic.PacketFormat,
    as.PacketFormat,  // ADD THIS
    nmea.PacketFormat,
    ...
}
```

3. Add to `CreatePacketProcessors` map (line ~101):
```go
return map[gpsprot.Tag]gpsprot.PacketProcessor{
    ubx.Tag:       ubx.NewPacketProcessor(),
    casic.Tag:     casic.NewPacketProcessor(),
    as.Tag:        as.NewPacketProcessor(),  // ADD THIS
    ...
}
```

## NAV-TIME Message Structure (from spec)

```
Byte Offset  Type  Name      Unit  Description
0            U1    navSys          0=GPS, 1=BD, 2=GLONASS, 3=Galileo
1            U1    flag            Bit0=week valid, Bit1=second valid, Bit2=leapsec valid
2            S2    Fractow   ns    Fraction part of GNSS Time of week
4            U4    refTow    ms    Reference GNSS Time of week
8            U2    week            Week in GNSS time
10           S2    leapSec   s     Leap seconds (GPS-UTC offset)
12           U4    timeErr   ns    Time accuracy estimate
```

**Example from spec - Get GPS time:**
```
F1 D9 01 05 10 00 00 07 2C 79 FF 55 3E 16 10 00 12 00 06 00 00 00 92 5A
      |  |  |     |  |  |     |           |     |     |
      |  |  len=16|  |  |     refTow      week  leap  timeErr
      cls id      nav flg fractow         =16   =18   =6
                  =0  =7  =31020  =373183999
                  GPS all
                      valid
```

This example is already in `asbin_test.go:TestNavTime()` - the existing test verifies parsing.

**CFG-PPS:**
```
F1 D9 06 07 0F 00 40 42 0F 00 00 00 00 00 10 27 00 00 01 0D 01 F3 86
```

**ACK-NAK:**
```
F1 D9 05 00 02 00 06 01 0E 33
```

## File Structure After Implementation

Following the layered architecture in docs/internals.md:

**Library layer** - `package asbin` (no goroutines, no logging, no gpsprot dependency):
```
internal/asbin/
  asbin.go         <- existing: Sync1/2, MsgID, checksum(), ParseMsg(), messages
  asbin_test.go    <- existing: message parsing tests
```

**Domain layer** - `package as` (implements gpsprot abstractions, imports asbin):
```
internal/as/
  asnmea.go        <- existing: NMEA satellite numbering
  aspacket.go      <- NEW: Tag, PacketFormat (imports asbin for Sync1/2, checksum)
  aspacket_test.go <- NEW: packet scanning tests
  asproc.go        <- NEW: PacketProcessor, NewPacketProcessor() (imports asbin for ParseMsg)
  astime.go        <- NEW: timeNavTime() → gpsprot.TimeMsg (imports asbin for NavTime)
```

This mirrors the ubx structure:
- `internal/ubx/bin/` (library) ↔ `internal/asbin/` (library)
- `internal/ubx/` (domain) ↔ `internal/as/` (domain)

## Reference Files

Use these as models for implementation:

**For aspacket.go:**
- `internal/ubx/ubxpacket.go` - UBX PacketFormat (same header layout as Allystar)
- `internal/casic/caspacket.go` - CASIC PacketFormat (different layout but similar structure)

**For asproc.go:**
- `internal/ubx/ubxproc.go` - UBX PacketProcessor
- `internal/casic/casproc.go` - CASIC PacketProcessor

**For astime.go:**
- `internal/ubx/ubxtime.go` - UBX time conversion
- `internal/casic/castime.go` - CASIC time conversion
- `internal/ptime/` - Time functions (ptime.GPS(), ptime.BeiDou(), etc.)

**For gpsreg updates:**
- `internal/gpsreg/reg.go` - See existing registrations for ubx, casic

**Existing asbin code:**
- `internal/asbin/asbin.go` - Sync1, Sync2, ParseMsg(), checksum(), NavTime, MsgID

## Test Data for aspacket_test.go

Use these packets from the spec for packet scanning tests:

```go
// NAV-TIME - 24 bytes total
"\xF1\xD9\x01\x05\x10\x00\x00\x07\x2C\x79\xFF\x55\x3E\x16\x10\x00\x12\x00\x06\x00\x00\x00\x92\x5A"

// CFG-PPS - 23 bytes total
"\xF1\xD9\x06\x07\x0F\x00\x40\x42\x0F\x00\x00\x00\x00\x00\x10\x27\x00\x00\x01\x0D\x01\xF3\x86"

// ACK-NAK - 10 bytes total
"\xF1\xD9\x05\x00\x02\x00\x06\x01\x0E\x33"

// CFG-CFG - 16 bytes total
"\xF1\xD9\x06\x09\x08\x00\x00\x00\x00\x00\x03\x00\x00\x00\x1A\x07"
```

## Verification

1. `go test ./internal/asbin/...` - existing tests still pass
2. `go test ./internal/as/...` - new tests pass
3. `make test` - full test suite
4. Test with Allystar hardware via `satpulsetool gps`
