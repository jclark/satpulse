# Unicore Receiver Support Design and Implementation Plan

## Overview

This document outlines the complete design and implementation plan for supporting Unicore receivers (specifically targeting Nebula IV series including UM980) in SatPulse. The implementation requires both Unicore-specific functionality and infrastructure changes to support multiple configurable protocols cleanly (current code only handles UBX).

## 1. Refactoring Existing Code for Unicore Support

This section covers modifications to existing SatPulse components necessary to enable the Unicore-specific implementation.

### 1.1 NMEA Packet Handling Changes

**Problem**: Unicore command acknowledgment responses use format `$command,<params>*XX` which superficially resembles NMEA but violates NMEA standards.

**Solution**: Modify division of responsibility between NMEA packet detection and parsing:

1. **Looser packet detection**: Accept any packet starting with `$`, containing printable characters, ending with `*XX` checksum and CR/LF
2. **Stricter parsing validation**: Move NMEA standard compliance checks to parsing stage; this would include checking of `^` escapes.
3. **Invalid NMEA handling**: The representation of an NMEA packet (currently nmea.Sentence) that is passed to NativeMsgHandler would be changed so that it can represent anything detected as an NMEA packet. We should also take this opportunity to fix handling of compliant NMEA proprietary sentences, which is currently broken.

### 1.2 Multi-Protocol Configuration Support

**Current Limitation**: The configuration system only supports a single protocol (UBX) because `gpscfg.go` creates just the first PacketExchanger it finds. Supporting multiple protocols like Unicore requires architectural changes.

**Solution**: Extend PacketExchanger to include NativeMsgHandler, centralize PacketExchanger creation, and use parallel probing with message fan-out.

#### 1.3.1 Interface Architecture

**PacketExchanger Interface Extension**:
```go
// PacketExchanger manages packet processing and generation for GPS configuration
type PacketExchanger interface {
    NativeMsgHandler  // Embed the NativeMsgHandler interface
    ProbePacket() []byte
    ProbeOK() bool
    Configure(*ConfigTarget) (Configurator, error)
}
```

This makes the relationship explicit: PacketExchangers must handle native messages during configuration.

#### 1.3.2 Centralized PacketExchanger Creation

Add to `internal/gpsreg/reg.go`:
```go
// CreatePacketExchangers creates all available packet exchangers
func CreatePacketExchangers() []gpsprot.PacketExchanger {
    return []gpsprot.PacketExchanger{
        ubx.NewPacketExchanger(),
        unc.NewPacketExchanger(),
        // future: other protocols
    }
}
```

Remove `CreatePacketExchanger()` from the PacketProcessor interface. This separation ensures PacketProcessors focus solely on packet parsing.

#### 1.3.3 Message Fan-out During Probing

Add to `internal/gpsprot/msg.go`:
```go
// MultiNativeMsgHandler fans out NativeMsg calls to multiple handlers
type MultiNativeMsgHandler struct {
    handlers []NativeMsgHandler
}

func NewMultiNativeMsgHandler(handlers ...NativeMsgHandler) *MultiNativeMsgHandler {
    return &MultiNativeMsgHandler{handlers: handlers}
}

func (m *MultiNativeMsgHandler) NativeMsg(tag Tag, msgID string, msg any, tRead time.Time) error {
    var firstErr error
    for _, h := range m.handlers {
        if err := h.NativeMsg(tag, msgID, msg, tRead); err != nil && firstErr == nil {
            firstErr = err
        }
    }
    return firstErr
}
```

#### 1.3.4 Parallel Probing Implementation

Modify `gpscfg.go` to probe all protocols simultaneously:

1. **Probe Phase**: Create all PacketExchangers, install MultiNativeMsgHandler to fan out messages to all
2. **Send Probes**: Send probe packets for all protocols (e.g., UBX-MON-VER poll, Unicore VERSIONA)  
3. **Select Winner**: First PacketExchanger to return ProbeOK() wins
4. **Configure Phase**: Switch NativeMsgHandler to the selected PacketExchanger for direct routing

**Benefits**:
- Fast detection (first responder wins)
- Clean architecture (parsing vs configuration separated)
- No packet type interest declarations needed
- Reusable MultiNativeMsgHandler component

#### 1.3.5 Implementation Impact

**Minimal changes required**:
- UBX PacketExchanger already implements both interfaces
- Unicore PacketExchanger will follow the same pattern
- gpscfg modifications are localized to probe logic
- PacketProcessors simplified (no configuration responsibility)

**For Unicore specifically**:
- Probe: Send `VERSIONA` command
- ProbeOK: When `#VERSIONA` response received
- NativeMsg: Handle command ACKs (`$command,...`), UNCA/UNCB messages
- All packet types (UNCB, UNCA, NMEA, NOVA) naturally route through shared NativeMsgHandler

This design provides a clean, extensible architecture for multi-protocol support without complex packet routing declarations.

### 1.3 Multi-Format PacketProcessor Support (Nice-to-Have)

**Current Limitation**: PacketProcessor interface assumes a one-to-one mapping between PacketFormat and PacketProcessor. However, Unicore has both ASCII and binary formats that logically belong to the same protocol and should be handled by a single PacketProcessor.

**Problem**: The current `PacketProcessor.ProcessPacket()` method has no way to know which format the packet came from, making it difficult to handle multiple formats in a single processor.

**Proposed Interface Change**:
```go
type PacketProcessor interface {
    // Current interface (no Tag parameter)
    ProcessPacket(data string, tRead time.Time) (string, error)
    
    // Proposed change: add Tag parameter like NativeMessageHandler
    ProcessPacket(tag Tag, data string, tRead time.Time) (string, error)
}
```

**Benefits**:
1. **Logical grouping**: Single Unicore PacketProcessor can handle both UNCA and UNCB packets
2. **Consistency**: Matches the `NativeMessageHandler` interface pattern which already uses Tag
3. **Flexibility**: Allows PacketProcessors to dispatch internally based on format
4. **Simplified architecture**: Reduces the number of PacketProcessor implementations needed

**Implementation Impact**:
- Update `gpsprot.PacketProcessor` interface definition
- Modify existing PacketProcessor implementations (UBX, NMEA, RTCM) to accept Tag parameter
- Unicore PacketProcessor can switch on Tag to handle ASCII vs binary parsing

### 1.4 Packet Scanner Architecture Updates (Nice-to-Have)

**Current Limitation**: Scanner gets packet format list from `gpsreg`, creating inappropriate coupling between scanner and protocol registry.

**Required Changes**:

#### Configuration Phase
1. **Initial packet formats**: `gpsreg` provides comprehensive set of packet formats suitable for probing and configuration
2. **Daemon/gpscmd responsibility**: These packages create scanner with appropriate initial packet format list
3. **Scanner independence**: Scanner has no built-in knowledge of specific packet formats

#### Post-Configuration Phase  
1. **ConfigResult specification**: After successful configuration, `ConfigResult` includes list of packet format tags needed for operation
2. **PacketExchanger knowledge**: Selected PacketExchanger knows what receiver type it's communicating with and can specify appropriate operational packet formats
3. **Runtime reconfiguration**: Scanner packet format list updated based on ConfigResult

**Scanner Interface Changes**:
```go
type PacketScanner interface {
    UpdatePacketFormats(formats []PacketFormat) error
    // ... existing methods
}
```

**ConfigResult Enhancement**:
```go
type ConfigResult struct {
    // ... existing fields
    PacketTags []string  // Tags needed for operation (e.g., ["UNCB", "UNCA"])
}
```

## 2. Unicore-Specific Functionality (`internal/unc/`)

The `internal/unc/` package implements all the `gpsprot` interfaces for Unicore receivers, analogous to how `internal/ubx` implements them for u-blox receivers.

### 2.1 PacketFormat Implementation

**UNCB Binary Packets** (`binpacket.go`) - ✅ Implemented
- 24-byte header with sync bytes (0xAA, 0x44, 0xB5)
- State machine for packet detection and boundary identification
- CRC verification using `uncmsg.CRC32()`
- Message ID extraction from header

**Testing approach:**
- Test sync byte detection with partial packets
- Test state machine transitions with fragmented input
- Use captured binary packets from real hardware

**UNCA ASCII Packets** (`asciipacket.go`) - ✅ Implemented
- ASCII log messages with `#` prefix
- Format: `#MessageName,header_fields;data_fields*checksum\r\n`
- Handles two checksum variants:
  - **32-bit CRC (*xxxxxxxx)**: Standard ASCII log messages (e.g., SATSINFOA, RECTIMEA)
  - **8-bit XOR (*xx)**: MODE query response only (e.g., `#MODE,...;MODE ROVER SURVEY,*1B`)
- Message name extraction from packet header
- **Note**: Command acknowledgments (`$command,...`) are handled via NMEA → invalid → NativeMsgHandler flow

**Testing approach:**
- Test both checksum variants (8-bit XOR for MODE, 32-bit CRC for other logs)
- Verify message name extraction
- Test with real captured ASCII packets
- Test MODE query response parsing specifically

**NovAtel Abbreviated ASCII** (`novapacket.go`)
- Lines beginning with `<` and terminated by CR/LF
- Printable ASCII + tab characters only
- Used for LOGLIST command response parsing to determine current serial port
- Required for baud rate configuration functionality (ConfigOptions.BaudRate)

**Testing approach:**
- Test with LOGLIST command output
- Verify printable ASCII validation
- Test tab character handling
- Test port detection from LOGLIST output parsing

### 2.2 PacketProcessor Implementation

Converts Unicore packets into abstract `gpsprot` messages.

#### 2.2.1 Library Layer (`internal/uncmsg/`)

Low-level Unicore packet parsing library used by PacketProcessor to decode packet contents into Go structs. Analogous to `internal/ubx/bin`.

**Components**:
- **ASCII message parsing** (`ascii.go`) - Decodes ASCII log messages
- **Binary message parsing** (`bin.go`) - Decodes binary messages
- **CRC validation** (`crc.go`) - Implements Unicore 32-bit CRC algorithm
- **Common structures** (`common.go`) - Shared data structures and constants
- **Satellite handling** (`sats.go`) - Satellite-related message structures
- **Time handling** (`time.go`) - Time-related message structures

**Testing with `data_test.go`:**
- Capture ASCII/binary message pairs from real hardware (e.g., SATSINFOA and SATSINFOB)
- Test round-trip parsing and serialization for both formats
- Verify ASCII and binary formats decode to identical data structures
- Use `satpulsetool gps --test-log` to generate test data
- Use `uncanno` to validate message decoding during development

Several more message types need implementing.

#### 2.2.2 Message Mapping - 🚧 Needs Implementation

Using the decoded structs from `uncmsg`, map to gpsprot abstract messages:
- `TimeMsg` - from Unicore time messages (RECTIMEB, etc.)
- `SatellitesMsg` - from Unicore satellite status messages (SATSINFOB, etc.)
- `PVTMsg` - from Unicore position/velocity messages (BESTNAVB, etc.)
- Other abstract message types as needed

**Testing message mapping:**
- Use `uncanno` to decode captured packets and verify correct mappings
- Test critical conversions:
  - GPS week/milliseconds → TAI nanoseconds
  - Unicore satellite system IDs → gpsprot GNSS constants
  - Position/velocity coordinate transformations
- Validate that ASCII and binary messages produce identical abstract messages
- Test edge cases: week rollovers, leap seconds, coordinate system differences

### 2.3 Configuration Support (PacketExchanger and Configurator)

Implements both `PacketExchanger` and `Configurator` interfaces for configuration-time operations.

#### 2.3.1 Core Functionality - 🚧 Needs Implementation

- Probe packet generation and response detection
- Command generation for all ConfigProperties and ConfigOptions
- Acknowledgment parsing (`$command,<original_command>,response[: <status>]*<checksum>`)
- Multi-tag packet handling (UNCB, UNCA, NMEA, NOVA)
- Response parsing for configuration queries
- Validation and error handling

**Testing approach:**

- Use replay testing (`replay_test.go`) to verify configuration sequences
- Generate test data with `satpulsetool gps --test-log`

#### 2.3.2 Configuration Command Mapping

This section maps `gpsprot.ConfigProperties` and `gpsprot.ConfigOptions` to Unicore protocol commands.

**ConfigProperties Mapping**

**SignalsEnabled Property**
- **Type**: `SignalSet` - specifies which GNSS signals should be enabled
- **Query**: `CONFIG` (returns current SIGNALGROUP and mask settings)
- **Set**: `CONFIG SIGNALGROUP` + `MASK`/`UNMASK`
- **Implementation**: Map SignalSet to optimal SIGNALGROUP, then apply fine-grained masking
- **Testing**: Verify CONFIG response parsing, test SIGNALGROUP selection algorithm, confirm receiver reset handling

**TimeGNSS Property**
- **Type**: `GNSS` - specifies which GNSS time system to use for time pulse
- **Query**: `CONFIG` (extract time reference from PPS configuration)
- **Set**: `CONFIG PPS` (modify time reference parameter)
- **Testing**: Test parsing of `CONFIG PPS ENABLE GPS POSITIVE...`, verify time system mapping

**TimePulse Properties**
- **Types**: `TimePulseWidth`, `TimePulsePeriod`, `TimePulseAlignToGNSS`, `TimePulseOnlyWhenLocked`, `TimePulsePolarityRising`
- **Query**: `CONFIG` (returns PPS settings)
- **Set**: `CONFIG PPS` (modify specific parameters while preserving others)

**Mode Property**
- **Type**: `Mode` - receiver operating mode (static/kinematic, fixed position)
- **Query**: `MODE` (returns current mode)
- **Set**: `MODE BASE lat lon height` or `MODE ROVER [type]`
- **Testing**: Test MODE query response parsing, verify coordinate format handling (LLH/ECEF), test all rover types

**AntennaCableDelay Property**
- **Type**: `time.Duration` - antenna cable delay compensation
- **Query**: `CONFIG` (extract RfDelay from PPS configuration)
- **Set**: `CONFIG PPS` (modify RfDelay parameter)

**NavMsgAuth Property**
- **Type**: `NavMsgAuth` - navigation message authentication method
- **Status**: Not supported by Unicore receivers (return appropriate error)

**ConfigOptions Mapping**

**Message Output Options**

*PVTMsg Option (PVTMsgFlags)*
- `PVTMsgPos` → `BESTNAVB 1` or `BESTNAVXYZB 1` (based on PVTMsgECEF flag)
- `PVTMsgVel` → Already included in BESTNAV messages
- `PVTMsgTime` → `RECTIMEB 1`
- `PVTMsgTimePulse` → `PPSSTATUS 1` (if available)
- `PVTMsgLeapSecond` → `GPSUTCB 1`, `BD3UTCB 1`, `GALUTCB 1`
- `PVTMsgSurvey` → Not supported (Unicore has no survey status messages)

*NMEAMsg Option (NMEAMsgFlags)*
- `NMEAMsgRMC` → `GPRMC 1`
- `NMEAMsgGGA` → `GPGGA 1`
- `NMEAMsgGSA` → `GPGSA 1`
- `NMEAMsgGSV` → `GPGSV 1`
- `NMEAMsgZDA` → `GPZDA 1`
- `NMEAMsgVTG` → `GPVTG 1`

*RTCMMsg Option (RTCMMsgFlags)*
- `RTCMMsgMSM4` → `RTCM1074 1`, `RTCM1084 1`, `RTCM1124 1` (GPS, GLO, BDS MSM4)
- `RTCMMsgMSM7` → `RTCM1127 1` (BDS MSM7, others as available)
- `RTCMMsgARP` → `RTCM1005 1` or `RTCM1006 1`

*SatsMsg Option (SatsMsgFlags)*
- Flags we have at the moment don't map well to unicore.
- SATSINFO gives both satellite and signal info
- BESTSAT says what satellites and signals are used in solution.
- Maybe add SatsMsgUsed flag.


*RawMsg Option (RawMsgFlags)*
- `RawMsgObs` → `OBSVMB 1` (for RINEX .obs generation)
- `RawMsgNavData` → Ephemeris messages for RINEX .nav file generation (only for enabled constellations):
  - `GPSEPHB 1` (if GPS signals enabled)
  - `BDSEPHB 1` (if BeiDou signals enabled)
  - `GALEPHB 1` (if Galileo signals enabled)  
  - `GLOEPHB 1` (if GLONASS signals enabled)
  - `QZSSEPHB 1` (if QZSS signals enabled)
  - `IRNSSEPHB 1` (if IRNSS signals enabled)

Rename RawMsgNavData to RawMsgNav and clarify semantics it means sufficient info to generate RINEX .nav files.

**Configuration Management Options**

*BaudRate Option*
- **Implementation**: Use `LOGLIST` to identify current port, then `CONFIG COMx baudrate`
- **Dependency**: Requires NOVA packet format parsing (LOGLIST outputs NOVA format)
- **Supported rates**: 9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600

*Save/Reset Options*
- `SaveNone` - no commands needed
- `SaveMinimal`/`SaveAll` - `SAVECONFIG`
- `ResetNone` - no commands needed
- `ResetReload` - `RESET`
- `ResetCold` - `RESET ALL`
- `ResetFactory` - `FRESET`

*Survey Option*
- **Implementation**: `MODE BASE TIME duration [distance]`
- **Semantic differences**: Unicore survey always runs full duration (no early completion based on accuracy)

**Binary vs ASCII Message Format Choice**

Design decision for examples above (use binary `*B` commands):

*Binary Format Advantages*:
- Efficiency: More compact, faster parsing
- Precision: No ASCII conversion/rounding issues
- Consistency: Matches u-blox UBX approach

*ASCII Format Advantages*:
- Portability: May be processable by non-Unicore receivers
- Standardization: More standardized across receiver types
- Debugging: Human-readable

*Key Consideration*: Binary Unicore messages use Unicore-specific sync bytes, making them receiver-specific. ASCII messages are more portable.

#### 2.3.3 State-Based Configuration Architecture

Unicore configuration uses a sophisticated state-based approach that handles the receiver's cumulative/stateful configuration model through pure functions.

**Four-Stage Pipeline**:

1. **Query Parsing**: `CONFIG`/`MODE`/`MASK` responses → internal state representation
2. **State Reading** (1 input): Complete internal state → all gpsprot properties (GET functionality)
3. **State Merging** (2 inputs): Current properties + partial target properties → new complete internal state (SET functionality)  
4. **Delta Generation**: Old state + new state → minimal command sequence including removals

**Key Design Insight: Two Distinct Mappings**

The reading and writing mappings are fundamentally different:

- **Reading Mapping**: `stateToProperties(state) → properties`
  - Takes complete internal state representation
  - Produces complete set of gpsprot properties
  - Implements GET functionality for all properties

- **Writing Mapping**: `propertiesToState(current, target) → newState`
  - Takes current complete properties + target partial properties
  - Produces new complete internal state representation
  - Handles partial updates while preserving unchanged settings
  - Enables complex multi-property commands like `CONFIG PPS`

**Multi-Property Command Example**:
`CONFIG PPS ENABLE GAL POSITIVE 1000000 1000 100 0` synthesized from:
- TimePulse property (ENABLE, period, width, polarity)
- TimeGNSS property (GAL)  
- AntennaCableDelay property (100ns)

**Delta Generation with UNMASK**:

Critical: Must generate removal commands for stateful configurations.

Example mask transition:
```
Old state: [MASK GPS, MASK 10]  // GPS disabled, 10° elevation
New state: [MASK BDS, MASK 15]  // BDS disabled, 15° elevation

Generated commands:
UNMASK GPS    // Remove old GPS mask
UNMASK 10     // Remove old elevation mask  
MASK BDS      // Add new BDS mask
MASK 15       // Add new elevation mask
```

Without UNMASK commands, old masks remain active (cumulative behavior).

**Testability Through Pure Functions**:

Each stage is a pure function testable with simple input/output pairs:

- **Query parsing**: Test with captured CONFIG/MODE/MASK responses
- **State reading**: Test state → properties conversion
- **State merging**: Test current + partial target → new complete state
- **Delta generation**: Test old state + new state → command sequence

Pure functions enable comprehensive table-driven testing without hardware, covering complex state transitions, multi-property synthesis, and removal scenarios.

**State Consistency Through Command Acknowledgments**:

The internal state representation stays synchronized with the actual receiver state through command acknowledgments:

- **Command Success**: When `$command,CONFIG PPS ENABLE,response: OK*04` received → update internal state with that command
- **Command Failure**: When `$command,CONFIG XYZ,response: PARSING FAILED*14` received → internal state remains unchanged
- **Accurate Tracking**: Internal state always reflects actual receiver configuration, even when some commands are rejected

This ensures the internal state representation is always accurate, enabling:
- Correct delta generation for subsequent configuration changes
- Reliable property GET operations based on actual receiver state
- Proper error handling without losing state synchronization

**Architectural Precedent**: This pattern mirrors the u-blox implementation in `internal/ubx/ubxcfg.go`:
- `RawConfig` struct maintains internal state as collection of binary UBX commands
- `msgSetRequest.Done()` updates state only after successful ACK-ACK (lines 838-843)
- `RawConfig.AddMsg()` synchronizes state with received messages (lines 718-765)
- Same acknowledgment-based state consistency used by both receiver types

## Implementation Status

### ✅ Completed Components

**Library Layer (`internal/uncmsg/`)**
- `ascii.go` - ASCII message parsing
- `bin.go` - Binary message parsing  
- `crc.go` - CRC validation
- `common.go` - Shared structures and constants
- `sats.go` - Satellite-related processing  
- `time.go` - Time-related processing

**Domain Layer (`internal/unc/`) - PacketFormat Implementation**
- `asciipacket.go` - UNCA ASCII packet format
- `binpacket.go` - UNCB binary packet format

### 🚧 Remaining Components

**Multi-Protocol Infrastructure**
- NMEA packet handling changes (essential)
- Multi-tag PacketExchanger routing (essential)
- Multi-format PacketProcessor support (nice-to-have)
- Scanner architecture updates (nice-to-have)
- ConfigResult enhancements (nice-to-have)

**Domain Layer (`internal/unc/`) - Remaining Implementations**
- **NOVA abbreviated ASCII packet format (PacketFormat) - ESSENTIAL for port detection**
- PacketProcessor interface implementation
- PacketExchanger interface implementation
- Configurator interface implementation
- Protocol registration with gpsreg

### Testing Infrastructure

**Packet Replay Testing** (`internal/gpscmd/replay_test.go`):
- Replays captured configuration sequences without hardware
- Validates configuration logic produces same output packets
- Enables regression testing across code changes
- Works with Unicore's ASCII command acknowledgments

**Test Data Generation**:
- `satpulsetool gps --test-log filename.jsonl` - captures all packets during configuration
- Records both commands sent and responses received
- `uncanno` - annotates packets with decoded messages for analysis
- Particularly useful for understanding Unicore's ASCII responses

**Test Coverage Requirements**:
- Basic probe and detection sequences
- All ConfigProperties getters/setters with proper acknowledgment checking
- All ConfigOptions implementations
- Error cases (PARSING FAILED responses, timeouts)
- Multi-packet command sequences
- Command ordering constraints (e.g., SIGNALGROUP reset behavior)

## Future Enhancements

### Additional GNSS Features

**PPP (Precise Point Positioning) Property**
- Standard GNSS feature, not Unicore-specific
- Support for BeiDou B2b, Galileo HAS, QZSS MADOCA services
- Deferred until u-blox X20P protocol definition available

**Elevation Mask Property** 
- Handled by MASK with number; we will need to parse this anyway
- Supported by both u-blox and Unicore
- Has existing GitHub issue for implementation
- Should be implemented as ConfigProperty

**LLH Coordinate Support**
- Both u-blox and Unicore already support LLH coordinates in their protocols
- u-blox implementation in SatPulse already handles LLH
- Current limitation: Front-end packages (daemon TOML config and gpscmd CLI) only accept ECEF
- Enhancement would be to add LLH support to the front-ends

