# Unicore Receiver Support Design and Implementation Plan

## Overview

This document outlines the complete design and implementation plan for supporting Unicore receivers (specifically targeting Nebula IV series including UM980) in SatPulse. The implementation requires both Unicore-specific functionality and infrastructure changes to support multiple configurable protocols cleanly (current code only handles UBX).

## Infrastructure Prerequisites

The Unicore implementation depends on several infrastructure improvements that are tracked as separate GitHub issues:

- #134: Make NMEA PacketFormat recognise NMEA-like packets (implemented in nmea-lax branch)
- #131: Decouple packet formats and configuration protocols (REQUIRED)
- #136: Redesign Configurator interface for better usability and testability (REQUIRED)
  - **Part 1**: Implement new Configurator2/ConfigRequest2 interfaces alongside existing ones
    - Create Configurator2 and ConfigRequest2 interfaces
    - Unicore will use Configurator2/ConfigRequest2 from the start
    - UBX continues using original Configurator/ConfigRequest
    - Both interface sets coexist temporarily
    - gpscfg.go will use both Configurator2/ConfigDirector and old Configurator
  - **Part 2**: Migrate UBX to Configurator2/ConfigRequest2 and cleanup
    - Port UBX implementation to Configurator2/ConfigRequest2
    - Remove original Configurator/ConfigRequest interfaces
    - Rename Configurator2/ConfigRequest2 to Configurator/ConfigRequest
- #132: Add Tag parameter to PacketProcessor interface (nice-to-have)
- #133: Allow dynamic changes to set of recognised packet formats (nice-to-have)

See the linked issues for detailed implementation specifications. The critical dependencies for Unicore support are #134, #131, and #136 Part 1.

## PacketFormat Implementation (`internal/unc/`)

**UNCB Binary Packets** (`binpacket.go`)
- 24-byte header with sync bytes (0xAA, 0x44, 0xB5)
- State machine for packet detection and boundary identification
- CRC verification using `uncmsg.CRC32()`
- Message ID extraction from header

**Testing approach:**
- Test sync byte detection with partial packets
- Test state machine transitions with fragmented input
- Use captured binary packets from real hardware

**UNCA ASCII Packets** (`asciipacket.go`)
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

**NovAtel Abbreviated ASCII** (`nov/abbrevasciipacket.go`)
- Lines beginning with `<` and terminated by CR/LF
- Printable ASCII + tab characters only
- Used for LOGLIST command response parsing to determine current serial port
- Required for baud rate configuration functionality (ConfigOptions.BaudRate)

**Testing approach:**
- Test with LOGLIST command output
- Verify printable ASCII validation
- Test tab character handling
- Test port detection from LOGLIST output parsing

## PacketProcessor Implementation

Converts Unicore packets into abstract `gpsprot` messages.

### Library Layer (`internal/uncmsg/`)

Low-level Unicore packet parsing library used by PacketProcessor to decode packet contents into Go structs. Analogous to `internal/ubx/bin`.

**Components**:
- **ASCII message parsing** (`ascii.go`) - Decodes ASCII log messages
- **Binary message parsing** (`bin.go`) - Decodes binary messages
- **CRC validation** (`crc.go`) - Implements Unicore 32-bit CRC algorithm
- **Common structures** (`common.go`) - Shared data structures and constants
- **Satellite handling** (`sats.go`) - Satellite-related message structures
- **Time handling** (`time.go`) - Time-related message structures
- **Version handling** (`version.go`) - Version-related message structures

**Implemented Message Types**:
- **VERSIONA/VERSIONB** (ID: 37) - Product model, firmware version, serial number
- **RECTIMEA/RECTIMEB** (ID: 102) - Receiver clock and UTC time information
- **PPSSTATUS** (ID: 9000) - PPS status and phase error information
- **GPSUTC** (ID: 19) - GPS UTC leap second parameters
- **GALUTC** (ID: 20) - Galileo UTC leap second parameters
- **BD3UTC** (ID: 22) - BDS-3 UTC leap second parameters
- **BDSUTC** (ID: 2012) - BDS UTC leap second parameters
- **SATSINFOA/SATSINFOB** (ID: 2124) - Satellite tracking and signal information

**Testing with `data_test.go`:**
- Capture ASCII/binary message pairs from real hardware (e.g., SATSINFOA and SATSINFOB)
- Test round-trip parsing and serialization for both formats
- Verify ASCII and binary formats decode to identical data structures
- Use `satpulsetool gps --test-log` to generate test data
- Use `uncanno` to validate message decoding during development

**Message Types Still Needed for gpsprot Mapping**:
- **BESTSAT** - Satellites used in navigation solution (needed for SatellitesMsg with Used field)

**Future Message Types (when gpsprot is extended)**:
- **BESTNAV/BESTNAVXYZ** - Position/velocity data (will be needed when PositionMsg is added to gpsprot)

### Message Mapping

Using the decoded structs from `uncmsg`, map to gpsprot abstract messages:
- `TimeMsg` - from RECTIMEB
- `LeapSecondMsg` - from GPSUTC, GALUTC, BD3UTC, BDSUTC
- `SatellitesMsg` - from SATSINFOB (needs BESTSAT for Used field)
- `SurveyMsg` - Not currently supported by Unicore messages

**Testing message mapping:**
- Use `uncanno` to decode captured packets and verify correct mappings
- Test critical conversions:
  - GPS week/milliseconds → TAI nanoseconds
  - Unicore satellite system IDs → gpsprot GNSS constants
  - Position/velocity coordinate transformations
- Validate that ASCII and binary messages produce identical abstract messages
- Test edge cases: week rollovers, leap seconds, coordinate system differences

## Configuration Support (PacketExchanger and Configurator)

Implements both `PacketExchanger` and `Configurator` interfaces for configuration-time operations.

### Core Functionality

- Probe packet generation and response detection
- Command generation for all ConfigProperties and ConfigOptions
- Acknowledgment parsing (`$command,<original_command>,response[: <status>]*<checksum>`)
- Multi-tag packet handling (UNCB, UNCA, NMEA, NOVAA)
- Response parsing for configuration queries
- Validation and error handling

**Testing approach:**

- Use replay testing (`replay_test.go`) to verify configuration sequences
- Generate test data with `satpulsetool gps --test-log`

### Configuration Command Mapping

This section maps `gpsprot.ConfigProperties` and `gpsprot.ConfigOptions` to Unicore protocol commands.

**ConfigProperties Mapping**

**SignalsEnabled Property**
- **Type**: `SignalSet` - specifies which GNSS signals should be enabled
- **Query**: `CONFIG` (returns current SIGNALGROUP, mask settings, and SBAS configuration)
- **Set**: `CONFIG SIGNALGROUP` + `MASK`/`UNMASK` + `CONFIG SBAS`
- **Implementation**: Map SignalSet to optimal SIGNALGROUP, then apply fine-grained masking. Handle SBAS as a constellation using `CONFIG SBAS ENABLE [system]` or `CONFIG SBAS DISABLE`
- **Testing**: Verify CONFIG response parsing, test SIGNALGROUP selection algorithm, confirm receiver reset handling, test SBAS enable/disable commands

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

**MinElevation Property**
- **Type**: `Angle` - minimum elevation angle for satellite tracking
- **Query**: `MASK` (extract elevation mask from mask settings)
- **Set**: `MASK [elevation_angle]` (set elevation mask in degrees)
- **Implementation**: Handle via existing maskProp using `MASK [number]` command format
- **Testing**: Test elevation mask parsing from MASK query response, verify elevation angle conversion

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
- **Dependency**: Requires NOVAA packet format parsing (LOGLIST outputs NOVAA format)
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

### State-Based Configuration Architecture

**Separation of Concerns:**
- `nativeConfigProps` (`cfgprops.go`): Pure transformation layer handling abstract↔native conversion, testable independently with table-driven tests
- `Configurator` and `ConfigRequest` (`config.go`): Protocol state machine implementing `gpsprot.Configurator` interface, handles packet sequencing, ACKs, timeouts

**Native Configuration Properties (`nativeConfigProps`):**
- Property-specific objects (ppsProp, signalGroupProp, maskProp, modeProp) that know Unicore command syntax  
- Simple interface: `updateFromCommand(cmd string) error`
- Each property implements methods for abstract↔native conversion and command generation
- Two-state command generation: clone current → update from abstract target → diff for minimal commands
- Designed for table-based testing of input/output transformations

Accepted command/response forms (normalized)
- $CONFIG,XYZ,CONFIG XYZ <params>
- #MODE,<header>;MODE <params>*xx
- $CONFIG,MASK,MASK <entry>
- $command,<original>,response[: <status>]*XX (ACK for any sent command)

**End-to-end lifecycle:**
1) Current native → Abstract current: convert native properties to abstract ConfigProps
2) Apply ConfigTarget → Abstract target: overlay target changes onto current abstract state  
3) Clone current → Target native: copy current native properties to preserve native-only fields
4) Abstract target → Native target: update cloned properties from complete abstract target
5) Generate command diff: compare target native vs current native, emit minimal commands

**Command execution:**
- Send commands sequentially
- On ACK success: update current native state
- On failure: leave current unchanged, re-plan as needed

**Preservation semantics:**
- Cloning preserves native-only fields not represented in gpsprot.ConfigProps (e.g., PPS userDelay parameter)
- Abstract→native conversion only modifies fields driven by abstract properties
- Native fields without abstract representation are preserved across updates

**Method responsibilities:**
- `updateFromCommand`: Parse command responses and emitted commands into native representation
- `convertToProps`: Convert native representation to abstract gpsprot.ConfigProps
- `updateFromProps`: Update native representation from abstract properties, preserving native-only fields  
- `generateCommands`: Generate minimal command diff between two native states

**Command ordering:**
- SIGNALGROUP before MASK (affects available signals)
- UNMASK before MASK (for same signal domain)
- Port/baud changes last (may require scanner resync)
- Save/reset commands at end (if requested)

**Property Implementation Notes:**
- **PPS**: Handles ENABLE/DISABLE variants, polarity, timing parameters, preserves userDelay
- **SIGNALGROUP**: Basic structure in place
- **MASK**: Signal masking implemented
- **MODE**: Basic structure with LLH/ECEF coordinate handling

## Remaining Implementation Tasks in Unicore-specific packages

This is what is needed to implement everything that has been implemented for u-blox and is supported by Unicore.

### Message Parsing (uncmsg)
- **BESTSAT message** - Parse satellites used in navigation solution (needed for SatellitesMsg.Used field)

### Message Mapping (unc)
- **LeapSecondMsg mapping** - Map GPSUTC, GALUTC, BD3UTC, BDSUTC to gpsprot.LeapSecondMsg
- **BESTSAT to SatellitesMsg.Used** - Map which satellites are actually used in solution
  * would benefit from additional SatsMsgFlag

### Configuration Options
- **PVT leap second messages** - GPSUTCB, BD3UTCB, GALUTCB for PVTMsgLeapSecond flag
- **BaudRate option** - Requires:
  - Requires determining COM port. Two possibilities:
    * Use LOGLIST, which requires NovAtel Abbreviated ASCII packet format
    * Use OEM7 ASCII message: where header has field identifying port e.g. BESTPOSA (but how to route this to configurator)
  - CONFIG COMx command generation
  - Requires implementing Configurator support related to speed changes

### Configuration Properties
- **SignalGroup** - Support changing signal group. Signal group change causes reset. So perhaps if user requests save and reset, we could change the signal group for them. But very unclear how to choose signal group.

### Testing

Adapt scripts in `internal/gpscmd/testdata` to work Unicore.

Test configuration with UM980 and save logs for replay testing.

Test with UM960 and see what needs fixing
