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

## PacketProcessor Implementation

Converts Unicore packets into abstract `gpsprot` messages.

### Library Layer (`internal/uncmsg/`)

Low-level Unicore packet parsing library used by PacketProcessor to decode packet contents into Go structs. Analogous to `internal/ubx/bin`.

**Components**:
- **ASCII message parsing** (`ascii.go`) - Decodes ASCII log messages ✅
- **Binary message parsing** (`bin.go`) - Decodes binary messages ✅
- **CRC validation** (`crc.go`) - Implements Unicore 32-bit CRC algorithm ✅
- **Common structures** (`common.go`) - Shared data structures and constants ✅
- **Satellite handling** (`sats.go`) - Satellite-related message structures ✅
- **Time handling** (`time.go`) - Time-related message structures ✅
- **Version handling** (`version.go`) - Version-related message structures ✅

**Implemented Message Types**:
- **VERSIONA/VERSIONB** (ID: 37) - Product model, firmware version, serial number ✅
- **RECTIMEA/RECTIMEB** (ID: 102) - Receiver clock and UTC time information ✅
- **PPSSTATUS** (ID: 9000) - PPS status and phase error information ✅
- **GPSUTC** (ID: 19) - GPS UTC leap second parameters ✅
- **GALUTC** (ID: 20) - Galileo UTC leap second parameters ✅
- **BD3UTC** (ID: 22) - BDS-3 UTC leap second parameters ✅
- **BDSUTC** (ID: 2012) - BDS UTC leap second parameters ✅
- **SATSINFOA/SATSINFOB** (ID: 2124) - Satellite tracking and signal information ✅

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

### Message Mapping - 🚧 Needs Implementation

Using the decoded structs from `uncmsg`, map to gpsprot abstract messages:
- `TimeMsg` - from RECTIMEB (ready to implement)
- `LeapSecondMsg` - from GPSUTC, GALUTC, BD3UTC, BDSUTC (ready to implement)
- `SatellitesMsg` - from SATSINFOB (partial - needs BESTSAT for Used field)
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

### Core Functionality - 🚧 Needs Implementation

- Probe packet generation and response detection
- Command generation for all ConfigProperties and ConfigOptions
- Acknowledgment parsing (`$command,<original_command>,response[: <status>]*<checksum>`)
- Multi-tag packet handling (UNCB, UNCA, NMEA, NOVA)
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

**Property Implementation Status:**
- **PPS**: Complete - handles ENABLE/DISABLE variants, polarity, timing parameters, preserves userDelay
- **SIGNALGROUP**: Basic structure, needs conversion logic
- **MASK**: Partial - signal masking implemented, elevation mask needs work  
- **MODE**: Stub only - needs parsing and conversion logic

## Phased Implementation Approach

The implementation will follow two parallel tracks to achieve working functionality as quickly as possible:

### Track 1: Getting satpulsed to use Unicore binary messages

**Initial goal**: Enable satpulsed to receive timing messages from Unicore receivers.

1. Implement PacketProcessor interface in `internal/unc/`
   - Map RECTIMEB to TimeMsg (minimum for timing sync)
2. Register with gpsreg

**Finish implementation**:
- Map SATSINFOB to SatellitesMsg
- Implement BESTSAT for Used field
- Map UTC parameter messages to LeapSecondMsg

### Track 2: Configuration

We want to implement this using the new Configurator design in `plan/configurator.md`. This will help validate the design. But initially we have the new and old Configurator designs co-exist, so we do not need to modify ubx to new the new Configurator design, until we have validated/refined it through use with Unicore.

**Initial goal**: Get satpulsetool gps to do something with Unicore receivers

1. Add unc.ConfigProtocol which will implement new ConfigProtocol (old PacketExchanger) from new Configurator design and issue #131, but without yet changing gpsprot.
2. Develop unc.Configurator further so that it will be easy to implement the new gpsprot.Configurator2 interface, but without needing gpsprot changes yet
   - Handling probing with VERSIONB (or VERSIONA)
   - Implement Configurator.GenerateRequests using nativeConfigProps.generateCommands
   - Implement ConfigRequest interface (but how to deal with ConfigRequestState type/constants?)
   - Try to test this in isolation to ensure it is working
3. Implement #136 Part 1 (would be a separate branch off master)
   - Add Configurator2 interface to gpsprot (matching what Unicore already has)
   - Update gpscfg to use Configurator2 for protocols that provide it
   - Test ConfigDirector on its own
4. Fixup Unicore so that it actually implements Configurator2 and register with gpsreg

**Next goal**: Configuration that is needed by satpulsed
* Support for `MODE` property
   * Implement parsing of #MODE response in uncmsg
   * Handle MODE property in unc
* PVT message enablement
   * RECTIME initially
   * GPSUTC for leap seconds
* Satellites message enablement
   * initally just SATSINFO

**Finish implementation**:
* Message enablement
   - RTCM
   - NMEA
   - Raw messages 
* Support for Save/Reset/FactoryReset
* Handle SBAS
* MinElevation property
* Support for baud-rate
   * Add NOVA packet format for BaudRate support
   * Handle LOGLIST output
   * Handle BaudRate config option
* #136 Part 2 - Migrate UBX to Configurator2 approach

## Implementated so far

**Infrastructure**

- Flexible NMEA parsing #134
- Protocol-agnostic GPS detection #137 (removes hardcoded protocol knowledge from gpscfg)

**Library Layer (`internal/uncmsg/`)**

Parsing of binary and ASCII Unicore messages. Analogous to ubx/bin package.

**Domain Layer (`internal/unc/`)**
- `asciipacket.go` - UNCA ASCII packet format (PacketFormat)
- `binpacket.go` - UNCB binary packet format (PacketFormat)
- `processor.go` - PacketProcessor implementation with:
  - BinPacketProcessor for UNCB packets
  - AsciiPacketProcessor for UNCA packets
  - Registration in gpsreg
- `time.go` - Time message mapping:
  - RECTIMEB → TimeMsg conversion (basic timing functionality)
  - UTC offset and accuracy conversion
  - TimeRef to GNSS mapping
- `sats.go` - Satellite message mapping:
  - SATSINFOB → SatellitesMsg conversion
  - Signal tracking status and CN0 mapping
  - Multi-constellation support
- `cfgprops.go` - Native configuration properties implementation
  - ppsProp with full PPS command handling
  - signalGroupProp with basic signal group support
  - maskProp with MASK/UNMASK command support
  - modeProp with MODE command handling (LLH/ECEF coordinates, survey mode, SetStatic support)
- `config.go` - Configurator that partially implements new Configurator2 interface

**Track 1 Status**: Initial goal achieved - satpulsed can now use Unicore binary messages for timing when manually configured to output RECTIMEB
- SATSINFO to SatellitesMsg conversion implemented

## Future Enhancements

### Additional GNSS Features

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

