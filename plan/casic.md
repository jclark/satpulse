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

- When TimeSrc is GLONASS, does NavSol use GPS-style week numbers?

Traditional GLONASS uses 4-year intervals + day numbers, not week/TOW. Since NavSol has a unified Week/TOW format, we assume CASIC outputs GPS-style week numbers even for GLONASS. Needs verification with a GLONASS-only time source configuration.

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

Create `internal/casic/casic.go` implementing `gpsprot.PacketProcessor`, and `internal/casic/castime.go` for TimeMsg conversions.

**Files to Modify:**
- `internal/gpsreg/reg.go` - Add CASIC to `CreatePacketProcessors()`

#### PacketProcessor

- Embed `gpsprot.DefaultPacketProcessor`
- `ProcessPacket()` parses via `bin.ParseMsg()` and dispatches to time conversion functions
- Convert NAV-SOL, NAV-TIMEUTC, TIM-TP to `gpsprot.TimeMsg`
- Set `NavEpoch` from `RunTime` field on all messages
- Pass unhandled messages to `NativeMsgHandler`

#### TimeMsg Conversions

All conversions set `NavEpoch` from the message's `RunTime` field.

**NAV-SOL → TimeMsg (NavSolution):**
- Validity: `PosValid < NavPos3D` means invalid - return TimeMsg with zero TAITime
- Convert `Week`/`TOW` to TAI using `ptime.GPS()`, `ptime.BeiDou()`, or `ptime.GLONASSWeek()` based on `TimeSrc`
- GLONASS uses week epoch of December 31, 1995 (Sunday before N₄ epoch)

**NAV-TIMEUTC → TimeMsg (NavSolution):**
- Validity: `DateValid < NavDateFromSatellite` or `(Valid & NavTimeUTCTOWValid) == 0` means invalid - return TimeMsg with nil UTCTime
- Sub-second time: `nanos = round(MsErr * 1e6)` (MsErr is residual in ms)
- TAcc is variance scaled by 1/c²; convert to duration: `sqrt(TAcc) * c`
- Set GNSS from `TimeSrc`

**TIM-TP → TimeMsg (PrePulse):**
- No validity check - message only emitted when pulse is generated
- Always convert to TAI using `ptime.GPS()`, `ptime.BeiDou()`, or `ptime.GLONASSWeek()` based on `RefTimeGNSS()`
- Ignore `RefTimeBase` (UTC vs GNSS) distinction - always produce TAITime

**Functionality:** satpulsed can receive time from CASIC receivers (basic time sync works).

---

### Stage 4: SatellitesMsg - done

Extend PacketProcessor to produce `gpsprot.SatellitesMsg`.

**Files to Create:**
- `internal/casic/cassats.go` - Satellite message conversion and accumulator

**Files to Modify:**
- `internal/casic/casproc.go` - Add epoch tracking and satAccum field, dispatch satellite messages

---

#### Epoch Tracking in PacketProcessor

Following the UBX pattern, epoch tracking lives in `PacketProcessor` and is used to coordinate flushing of accumulated data (satellites now, potentially other things later).

```go
type PacketProcessor struct {
    gpsprot.DefaultPacketProcessor
    mh          gpsprot.MsgHandler
    curNavEpoch uint32  // current RunTime (0 means no epoch seen yet)
    satAccum    satAccum
}
```

The `handleNavEpoch()` method is called for every NavMsg:

```go
func (p *PacketProcessor) handleNavEpoch(nm bin.NavMsg, tRead time.Time) {
    e := nm.NavEpoch()
    if e != p.curNavEpoch {
        if p.curNavEpoch != 0 {
            p.flushNavEpoch(tRead)
        }
        p.curNavEpoch = e
    }
}

func (p *PacketProcessor) flushNavEpoch(tRead time.Time) {
    p.satAccum.epochChange(p.mh, tRead)
}
```

---

#### Satellite Accumulator

The `satAccum` struct accumulates satellite info within an epoch. It does not track the epoch itself—that's `PacketProcessor`'s job.

```go
type satAccum struct {
    nEpochs   int              // number of complete epochs seen (for early-flush gating)
    received  uint8            // bit vector of bin.GNSSID received this epoch (1<<GPS | 1<<BDS | 1<<GLN)
    predicted uint8            // bit vector of GNSS expected (from previous epoch)
    svs       []gpsprot.SVInfo
}
```

---

#### Epoch Combining Logic

Track which GNSSs we've seen in current epoch. Flush (emit SatellitesMsg) when:
1. Epoch changes (handled by `PacketProcessor.flushNavEpoch()`), OR
2. All predicted GNSSs received (`received == predicted && predicted != 0`), OR
3. All 3 GNSSs received (`received == 0x07`)

**Early-flush gating**: Conditions 2 and 3 are only checked after `nEpochs >= 2`. If we started listening mid-epoch, the first epoch's `received` bitmap is incomplete, and using it as `predicted` could cause premature flushes. After two complete epoch changes, we know `predicted` reflects a full epoch.

On flush:
- Emit accumulated `SatellitesMsg` if `len(svs) > 0`
- Update `predicted = received` (for next epoch prediction)
- Reset `received = 0` and `svs = nil`

---

#### Shared Conversion Function

```go
// convertNavSatInfo converts CASIC satellite info to gpsprot format.
// Uses fixed.System to determine GNSS type.
func convertNavSatInfo(fixed *bin.NavSatInfoFixed, svs []bin.NavSVInfo) []gpsprot.SVInfo
```

Dispatch in `casproc.go`:
```go
case *bin.NavGPSInfo:
    p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
case *bin.NavBDSInfo:
    p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
case *bin.NavGLNInfo:
    p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
```

---

#### SVID Conversion

CASIC binary uses raw PRN numbers in the `svid` field. For GPS, BDS, and GLONASS this maps directly to `gpsprot.SVID.Num`. However, SBAS and QZSS satellites are reported in NAV-GPSINFO (system=GPS) using their raw PRN numbers, which must be converted to RINEX numbering.

| System | Raw SVID | gpsprot.GNSS | gpsprot.SVID.Num |
|--------|----------|--------------|------------------|
| GPS | 1-32 | GPS | svid |
| BDS | 1-63 | BDS | svid |
| GLN | 1-32 | GLO | svid (slot number) |
| GPS | 120-158 | SBAS | svid - 100 |
| GPS | 193-199 | QZSS | svid - 192 |

```go
func casicSVID(gnss gpsprot.GNSS, svid uint8) gpsprot.SVID {
    if gnss == gpsprot.GPS {
        if svid >= 120 && svid <= 158 {
            return gpsprot.SVID{GNSS: gpsprot.SBAS, Num: svid - 100}
        }
        if svid >= 193 && svid <= 199 {
            return gpsprot.SVID{GNSS: gpsprot.QZSS, Num: svid - 192}
        }
    }
    return gpsprot.SVID{GNSS: gnss, Num: svid}
}
```

GNSS mapping (`bin.GNSSID` → `gpsprot.GNSS`):
- `bin.GPS` (0) → `gpsprot.GPS`
- `bin.BDS` (1) → `gpsprot.BDS`
- `bin.GLN` (2) → `gpsprot.GLO`

Use existing `gnssIDToGNSS()` from `castime.go`.

---

#### Signal ID Mapping

CASIC reports one CN0 per satellite (L1 legacy signal). Map to:
- GPS → `gpsprot.SigIDGPSL1CA` ("L1 C/A")
- BDS → `gpsprot.SigIDBDSB1I` ("B1I")
- GLONASS → `gpsprot.SigIDGLOL1` ("L1")

```go
var casicSignalID = map[bin.GNSSID]gpsprot.SignalID{
    bin.GPS: gpsprot.SigIDGPSL1CA,
    bin.BDS: gpsprot.SigIDBDSB1I,
    bin.GLN: gpsprot.SigIDGLOL1,
}
```

---

#### Quality Filtering

Filter satellites with `CNO == 0` (no signal lock). Include all others.

```go
if sv.CNO == 0 {
    continue
}
```

---

#### SVInfo Construction

For each `bin.NavSVInfo` with CNO > 0:

```go
used := sv.Flags&bin.NavSVUsed != 0
gpsprot.SVInfo{
    ID: casicSVID(gnss, sv.SVID),
    LookAngles: &gpsprot.LookAngles{
        Azimuth:   sv.Azim,   // int16, 0-360
        Elevation: sv.Elev,   // int8, -90 to 90
    },
    Signals: []gpsprot.SignalInfo{{
        ID:   casicSignalID[fixed.System],
        CN0:  sv.CNO,
        Used: used,
    }},
    Used: used,
}
```

---

#### SatellitesMsg Construction

On flush, emit:

```go
&gpsprot.SatellitesMsg{
    SVs:          accumulated,
    Tag:          Tag,
    NativeMsgID:  "NAV-SATINFO", // combined from GPS/BDS/GLN
    UsedValidity: gpsprot.SatelliteUsedSignal,
}
```

Like UBX-NAV-SAT: the message reports satellite-level "used", but since there's one signal per satellite, that signal is used iff the satellite is used—so we can use `SatelliteUsedSignal` and set `SignalInfo.Used`.

This differs from NMEA, where GSA reports satellite-level "used" but GSV reports multiple signals per satellite (e.g., L1 and L5). In that case we must use `SatelliteUsedSV` because we know the satellite is used but not which specific signal.

---

#### satAccum.accum Method

```go
func (a *satAccum) accum(fixed *bin.NavSatInfoFixed, svs []bin.NavSVInfo, mh gpsprot.MsgHandler, tRead time.Time) {
    // Convert and append
    converted := convertNavSatInfo(fixed, svs)
    a.svs = append(a.svs, converted...)

    // Mark this GNSS as received
    a.received |= uint8(1) << fixed.System

    // Check early-flush conditions (only after seeing 2 complete epochs)
    if a.nEpochs >= 2 &&
       (a.received == 0x07 || (a.predicted != 0 && a.received == a.predicted)) {
        a.flush(mh, tRead)
    }
}
```

---

#### satAccum.epochChange Method

Called by `PacketProcessor.flushNavEpoch()` on epoch change. Increments `nEpochs` then flushes.

```go
func (a *satAccum) epochChange(mh gpsprot.MsgHandler, tRead time.Time) {
    a.nEpochs++
    a.flush(mh, tRead)
}
```

---

#### satAccum.flush Method

Called by `epochChange()` on epoch change, or by `accum()` for early flush.

```go
func (a *satAccum) flush(mh gpsprot.MsgHandler, tRead time.Time) {
    if len(a.svs) == 0 {
        a.predicted = a.received
        a.received = 0
        return
    }

    msg := &gpsprot.SatellitesMsg{
        SVs:          a.svs,
        Tag:          Tag,
        NativeMsgID:  "NAV-SATINFO",
        UsedValidity: gpsprot.SatelliteUsedSignal,
    }

    if mh != nil {
        mh.Satellites(msg, tRead)
    }

    a.predicted = a.received
    a.received = 0
    a.svs = nil
}
```

---

#### Updated ProcessPacket and dispatch() Flow

`ProcessPacket` calls `handleNavEpoch` for every NavMsg before dispatching:

```go
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
    m, err := bin.ParseMsg(data)
    if err != nil {
        return PacketFormat.MsgID([]byte(data)), err
    }
    msgID := m.ID().String()
    if nm, ok := m.(bin.NavMsg); ok {
        p.handleNavEpoch(nm, tRead)
    }
    if p.dispatch(m, tRead) {
        return msgID, nil
    }
    // ... NativeMsgHandler handling ...
}
```

The `dispatch` method handles message-specific logic:

```go
func (p *PacketProcessor) dispatch(m bin.Msg, tRead time.Time) bool {
    switch mt := m.(type) {
    case *bin.NavSol:
        // ... existing time handling ...
    case *bin.NavTimeUTC:
        // ... existing time handling ...
    case *bin.TimTP:
        // ... existing time handling ...
    case *bin.NavGPSInfo:
        p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
        return true
    case *bin.NavBDSInfo:
        p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
        return true
    case *bin.NavGLNInfo:
        p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
        return true
    default:
        return false
    }
    // ... rest of time message handling ...
}
```

---

#### Edge Cases

1. **First epoch**: `curNavEpoch == 0` initially; `handleNavEpoch` doesn't flush on first message
2. **Missing GNSS**: If receiver doesn't emit a GNSS (e.g., no GLONASS satellites visible), that message won't arrive. Rely on epoch change to flush.
3. **Empty messages**: A GNSS info message with 0 satellites still counts as received (marks bit in `received`)
4. **Order independence**: Messages within an epoch can arrive in any order

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
