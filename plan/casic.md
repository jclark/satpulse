# CASIC Protocol Implementation Plan

## Overview

Add support for the CASIC binary protocol used by Zhongke Microelectronics GPS receivers (AT6558D, ATGM3368, etc.). The implementation will follow the existing UBX protocol architecture in `internal/ubx/`.

## Protocol Summary

### Packet Structure

| Field | Size | Description |
|-------|------|-------------|
| Sync | 2 bytes | `0xBA 0xCE` |
| Length | 2 bytes | Payload length (little-endian) |
| Class | 1 byte | Message class |
| ID | 1 byte | Message ID |
| Payload | variable | Must be multiple of 4 bytes |
| Checksum | 4 bytes | 32-bit cumulative sum |

### Checksum Algorithm

```go
ckSum := (id << 24) + (class << 16) + len
for each 4-byte word in payload (little-endian):
    ckSum += word
```

**Note:** The spec documents class before id in the checksum, but per errata.md, receivers actually use `(id << 24) + (class << 16)`.

### Message Classes

| Name | Class | Description |
|------|-------|-------------|
| NAV | 0x01 | Navigation results |
| TIM | 0x02 | Timing messages |
| RXM | 0x03 | Receiver measurements |
| ACK | 0x05 | ACK/NAK responses |
| CFG | 0x06 | Configuration |
| MSG | 0x08 | Satellite navigation data |
| MON | 0x0A | Monitoring |
| AID | 0x0B | Assistance data |
| NMEA | 0x4E | NMEA message control |

### Key Differences from UBX

| Aspect | UBX | CASIC |
|--------|-----|-------|
| Sync bytes | `0xB5 0x62` | `0xBA 0xCE` |
| Header order | sync, class, id, len | sync, len, class, id |
| Checksum | 2-byte Fletcher | 4-byte word sum |
| Payload alignment | none | must be 4-byte aligned |


### Supported Periodic Messages

Messages supported by ATGM332D-5N31: AID-INI, MSG-BDSEPH, MSG-BDSION, MSG-BDSUTC, MSG-GLNEPH, MSG-GPSEPH, MSG-GPSION, MSG-GPSUTC, NAV-BDSINFO, NAV-CLOCK, NAV-DOP, NAV-GLNINFO, NAV-GPSINFO, NAV-PV, NAV-SOL, NAV-STATUS, NAV-TIMEUTC, RXM-MEASX, RXM-SVPOS, TIM-TP

Ones we should support for implementing various messages:
- NAV-SOL - gives GPS time; tow is a float; says what time comes from GPS or BDS
- NAV-TIMEUTC - gives UTC time
- TIM-TP - time of next pulse
- NAV-GPSINFO - info about GPS satellites
- NAV-BDSINFO - info about BDS satellites
- NAV-GLNINFO - info about GLONASS satellites
- MSG-GPSUTC - gives leap seconds for GPS
- MSG-BDSUTC - gives leap seconds for BDS
- NAV-CLOCK - gives UTC delta for each GNSS

Note: ATGM332D-5N31 does not emit MSG-GPSUTC or MSG-BDSUTC, although it ACKs the request to enable it.

### Information needed

- What does runTime field do? Do messages in same epoch have same runTime?

Messages in the same navigation epoch do have the same runTime value.
A TIM-TP message for second N is emitted before second N together with NAV messages for second N - 1. It has the same runTime value as the NAV messages for second N - 1.

- Is qErr in TIM-TP implemented?

In ATGM332D-5N31 it is not implemented.

## Implementation Stages

### Stage 1: Packet Framing and Registration - done

**Files Created:**
- `internal/casic/casicpacket.go` - PacketFormat with state machine, checksum
- `internal/casic/casicpacket_test.go` - Tests

**Files Modified:**
- `internal/gpsreg/reg.go` - Registered in `PacketFormats`, `VendorZhongke` added

**Functionality:** CASIC packets recognized and logged by `internal/scan/`.

---

### Stage 2: Binary Message Parsing (casic/bin) - done

Create `internal/casic/bin/` with message type system and all periodic messages.

**Files to Create:**

1. **`internal/casic/bin/common.go`** - Core infrastructure
   - `MsgID` type (uint16: class | id<<8)
   - Message class constants
   - `Msg` interface with `ID() MsgID`
   - `VaryingMsg` interface (same pattern as ubx/bin)
   - `ParseMsg()` function
   - `regMsg[T]()` generic registration

2. **`internal/casic/bin/nav.go`** - Navigation messages
   - `NavSol` (0x01 0x02) - GPS time, fix status
   - `NavTimeUTC` (0x01 0x10) - UTC time
   - `NavClock` (0x01 0x11) - UTC delta per GNSS
   - `NavGPSInfo` (0x01 0x20) - GPS satellites (VaryingMsg)
   - `NavBDSInfo` (0x01 0x21) - BDS satellites (VaryingMsg)
   - `NavGLNInfo` (0x01 0x22) - GLONASS satellites (VaryingMsg)

3. **`internal/casic/bin/tim.go`** - Timing messages
   - `TimTP` (0x02 0x00) - Time of next pulse

4. **`internal/casic/bin/msg.go`** - Satellite navigation data
   - `MsgGPSUTC` (0x08 0x02) - GPS leap seconds
   - `MsgBDSUTC` (0x08 0x12) - BDS leap seconds

**Functionality:** Binary message parsing/serialization for all periodic messages.

---

### Stage 3: PacketProcessor with TimeMsg

Create `internal/casic/casic.go` implementing `gpsprot.PacketProcessor`.

**Implement:**
- `ProcessPacket()` that parses CASIC packets via `bin.ParseMsg()`
- Convert `NavSol`, `TimTP`, `NavTimeUTC` in `gpsprot.TimeMsg`
- Basic epoch tracking via `runTime` field

**Files to Modify:**
- `internal/gpsreg/reg.go` - Add to `CreatePacketProcessors()`

**Functionality:** satpulsed can receive time from CASIC receivers (basic time sync works).

---

### Stage 4: SatellitesMsg

Extend PacketProcessor to produce `gpsprot.SatellitesMsg`.

**Implement:**
- Convert `NavGPSInfo`, `NavBDSInfo`, `NavGLNInfo` → `gpsprot.SatellitesMsg`
- Combine satellite messages from same epoch

**Functionality:** Satellite display in satpulsed web dashboard.

---

### Stage 5: LeapSecondMsg

**Note**: deferred until we find a receiver that implements it

Extend PacketProcessor to produce `gpsprot.LeapSecondMsg`.

**Implement:**
- Convert `MsgGPSUTC` or `MsgBDSUTC` → `gpsprot.LeapSecondMsg`

**Functionality:** Proper TAI/UTC conversion in satpulsed.

---

### Stage 6: Configuration Message Definitions (casic/bin)

Add configuration and acknowledgment message types to `casic/bin/`.

**Files to Create:**

1. **`internal/casic/bin/ack.go`** - ACK/NAK messages
   - `AckAck` (0x05 0x01)
   - `AckNak` (0x05 0x00)

2. **`internal/casic/bin/cfg.go`** - Configuration messages
   - `CfgPrt` (0x06 0x00) - Port configuration
   - `CfgMsg` (0x06 0x01) - Message rate configuration
   - `CfgTP` (0x06 0x03) - Time pulse configuration
   - `CfgTMode` (0x06 0x06) - Time mode configuration
   - `CfgNMEA` (0x4E 0x*) - NMEA message control

**Functionality:** Binary definitions for configuration commands.

---

### Stage 7: Minimal Configurator (Probing + PVTMsg + NMEAMsg)

Implement `gpsprot.ConfigProtocol` and `gpsprot.Configurator` with receiver detection and message output configuration.

**Files to Create:**
- `internal/casic/casiccfg.go` - ConfigProtocol implementation

**Implement:**
- `ProbePacket()` / `ProbeOK()` for receiver detection (e.g., poll MON-VER or similar)
- `ConfigOptions.PVTMsg` support (enable time-related messages)
- `ConfigOptions.NMEAMsg` support (enable/disable NMEA messages)
- ACK/NAK handling for configuration responses

**Files to Modify:**
- `internal/gpsreg/reg.go` - Add to `CreateConfigProtocols()`

**Functionality:**
- `satpulsetool gps --binary --nmea --nmea-out --pvt-out` works with CASIC receivers
- `satpulsed` with `config=true` works minimally: can take a factory-default CASIC receiver, detect it, and configure it to emit the binary messages needed for time sync

---

### Stage 8: TimePulse Configuration

Extend Configurator for time pulse settings.

**Implement:**
- `ConfigProps.TimePulse` and `ConfigProps.TimeGNSS` support via `CfgTP` message
- Width, period, polarity, alignment settings

**Functionality:** `satpulsetool gps --pps --time-gnss` works.

---

### Stage 9: TimeMode Configuration

Extend Configurator for time mode settings.

**Implement:**
- `ConfigProps.Mode` support via `CfgTMode` message
- Static/mobile mode
- Survey-in mode
- Fixed position mode

**Functionality:**
- `satpulsetool gps --survey --fixed-pos-ecef --mobile` works
- `satpulse.toml` time mode options work: `mobile`, `surveyTime`, `fixedPosECEF`, `fixedPosAcc`

---

### Stage 10: SignalsEnabled Configuration

Extend Configurator for GNSS signal selection.

**Implement:**
- `ConfigProps.SignalsEnabled` support
- Enable/disable GNSS constellations

**Functionality:** `satpulsetool gps --gnss --band` works.

---

### Stage 11: Speed Change

Extend Configurator to handle change in speec.

---

### Stage 12: Raw Observation Configuration

Extend Configurator for raw observation output.

**Implement:**
- `ConfigOptions.RawMsg` support via RXM message configuration
- Enable/disable RXM-MEASX and related raw observation messages

**Functionality:** `satpulsetool gps --raw-out` works.

## Struct Patterns

### Fixed-Size Message

```go
type AckAck struct {
    ClsID byte
    MsgID byte
    _     [2]byte // padding to 4 bytes
}

func (m *AckAck) ID() MsgID { return AckAckID }
```

### Message with Embedded Epoch

```go
type NavRunTime struct {
    RunTime uint32 // ms since boot
}

type NavDOP struct {
    NavRunTime
    PDOP float32
    HDOP float32
    // ...
}
```

### Variable-Length Message

```go
type NavGPSInfo struct {
    NavGPSInfoFixed
    SVs []NavSVInfo
}

type NavGPSInfoFixed struct {
    NavRunTime
    NumViewSV byte
    NumFixSV  byte
    System    byte
    _         byte
}

type NavSVInfo struct {
    Chn     byte
    SVID    byte
    Flags   byte
    Quality byte
    CNO     byte
    Elev    int8
    Azim    int16
    PRRes   float32
}

func (m *NavGPSInfo) InitVaryingPart(payloadLen int) error {
    fixedLen := 8
    elemLen := 12
    n := (payloadLen - fixedLen) / elemLen
    m.SVs = make([]NavSVInfo, n)
    return nil
}
```

## Known Errata

From `../casictool/spec/errata.md`:

1. **Checksum byte order**: Spec says `(class << 24) + (id << 16)`, but receivers use `(id << 24) + (class << 16)`

2. **MON-VER support**: Some receivers respond with ACK-NAK instead of version info

3. **CFG-MSG query**: Empty payload returns all message rates, not just one

4. **CFG-TMODE mode field**: Upper 2 bytes of U4 mode field contain unknown values; parse as U2 mode + U2 reserved

## Reference Materials

- Spec: `../casictool/spec/casic.md`, `casic1.md`, `casic2.md`
- Errata: `../casictool/spec/errata.md`
- Python prototype: `../casictool/casic.py`
- UBX reference: `internal/ubx/`, `internal/ubx/bin/`

## Testing Strategy

1. Unit tests for checksum calculation
2. Unit tests for packet framing state machine
3. Integration tests with captured CASIC packets
4. Test data from `../casictool/` if available
