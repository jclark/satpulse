# NMEA Lax Parsing Design

## Overview

This document describes the design for solving the problem of parsing both standards-compliant NMEA sentences and NMEA-like packets such as Unicore command acknowledgments. The solution uses a two-stage approach: loose packet detection followed by strict validation with explicit validity flags. This design has been implemented.

## Problem Statement

### Current Issues

1. **GitHub Issue #74**: NMEA parsing of `P*` sentences is broken
   - Assumes 3-letter manufacturer ID followed directly by fields (like PUBX)
   - Reality: Can have additional characters after manufacturer ID before comma
   - `^` escaping incorrectly applied to P sentences (should not be)

2. **Unicore Integration Need**: Unicore command acknowledgments use format `$command,<params>*XX` which:
   - Superficially resembles NMEA but violates NMEA standards
   - Must be detectable as "NMEA-like" packets
   - Need to be passed to NativeMsgHandler for processing

3. **Current Architecture Limitation**: NMEA-like (but not strictly NMEA compliant) packets are not recognized at all, and so cannot be handled by higher-level layers.

## Solution Architecture

### Packet Detection Constraints

The solution implements 6 structural constraints for acceptable NMEA-like packets, ensuring basic format validity while allowing non-standard content:

**Packet Constraints**:
1. **Single dollar sign**: First character is `$`, no other `$` characters in packet
2. **Line terminator**: Ends with CR/LF or LF  
3. **Printable ASCII**: All characters before terminator are 0x20-0x7E
4. **Length limit**: Total packet ≤ 128 characters (including terminator)
5. **Checksum format**: Exactly one `*` followed by two uppercase hex digits before terminator
6. **Non-empty address**: Address field (between `$` and first comma or `*`) has ≥1 character

These constraints allow both standard NMEA sentences and non-standard formats like Unicore command acknowledgments (`$FRESET,response: OK*2E`) to be detected as valid packets.

### Two-Stage Processing Model

**Stage 1: Packet Detection (Lax)**
- Apply the 6 packet constraints listed above
- Goal: Capture both valid NMEA and NMEA-like packets that meet basic structure
- No semantic NMEA validation at this stage

**Stage 2: Parsing with Validation Flags (Strict)**
- Parse all detected packets into structured representation
- Apply comprehensive NMEA compliance checks
- Set explicit validity flags indicating which checks pass
- Enable downstream code to make informed decisions

### Design Solution: Two-Level Sentence Representation

The design uses two distinct sentence types to handle the spectrum from basic packet validity to full NMEA compliance:

**`Sentence`** - Represents any valid NMEA-like packet
- Contains syntax flags indicating which validation checks pass
- Includes payload data for further processing
- Accepts Unicore command acknowledgments and other non-standard formats

**`ApprovedSentence`** - Represents fully NMEA-compliant sentences  
- Contains structured talker ID, format, and data fields
- Created only from `Sentence` objects that pass GNSS talker validation
- Used for standard NMEA message processing

## Syntax Flag System

### Design Concept

The `SentenceSyntaxFlags` system replaces binary pass/fail validation with granular property flags. Each flag represents a specific NMEA compliance aspect:

**Address Format Validation**
- Standard NMEA: exactly 5 uppercase alphanumeric characters  
- Proprietary: 'P' + 3+ characters for manufacturer ID

**GNSS Talker Recognition**
- Individual flags for each GNSS system (GPS, GLONASS, Galileo, etc.)
- Composite flag for any recognized GNSS talker

**Character Content Validation**
- Caret (^) character detection and escape sequence validation
- Invalid data character detection (backslash, exclamation mark, tilde)
- Reserved character enforcement ($ and * only in proper positions)

**Packet Structure Validation**
- Line termination format (CRLF vs LF)  
- Length compliance (82 character NMEA limit vs 128 character packet limit)
- Packet boundary detection per 6 constraints

### Individual Property Semantics

**ApprovedAddressFormat** - Standard NMEA address structure (NMEA 7.2.2.2)
- Exactly 5 uppercase alphanumeric characters
- Examples: `GPGGA`, `GLRMC`, `GARMC`
- Does not validate talker ID recognition - just format

**ProprietaryAddressFormat** - Proprietary sentence structure (NMEA 7.2.2.4)
- Starts with `P` followed by 3+ uppercase alphanumeric characters
- Examples: `PUBX`, `PMTK`, `PMTK123` (fixes issue #74)
- Allows extended manufacturer IDs beyond 3 characters

**TalkerIsXX** - Individual GNSS system identification
- `TalkerIsGP`: Address starts with "GP" (GPS)
- `TalkerIsGL`: Address starts with "GL" (GLONASS)
- `TalkerIsGA`: Address starts with "GA" (Galileo)
- `TalkerIsGB`: Address starts with "GB" (BeiDou current standard)
- `TalkerIsBD`: Address starts with "BD" (BeiDou legacy)
- `TalkerIsGI`: Address starts with "GI" (NavIC)
- `TalkerIsGQ`: Address starts with "GQ" (QZSS)
- `TalkerIsGN`: Address starts with "GN" (Multi-GNSS)
- Mutually exclusive - exactly one or none set
- `SentenceTalkerIsGNSS` is a composite flag (union of all individual GNSS talker flags)

**NoCarets** - No caret characters present
- No `^` characters found anywhere in the packet
- Optimization flag to avoid unnecessary unescape processing

**ValidCaretEscaping** - Proper caret escape sequences (NMEA 7.1.4)
- All `^` characters followed by exactly 2 uppercase hex digits
- Set to true even if no `^` characters present
- Validates well-formed escape sequences when carets exist

**ValidDataChars** - Valid data character content
- No backslash, exclamation mark, or tilde characters in packet
- Ensures compliance with NMEA character restrictions

**EndsWithCRLF** - Standard line termination
- Packet ends with `<CR><LF>` sequence (not just LF)
- Required by NMEA specification

**Length82OrLess** - NMEA length compliance
- Total packet length ≤ 82 characters
- Official NMEA maximum length limit (vs 128 char packet detection limit)

## Syntax Validation Criteria

Each detected packet is analyzed by CheckSyntax() and validated against specific criteria:

**Address Field Validation**:
- Standard NMEA: Exactly 5 alphanumeric uppercase chars
- Proprietary: `P` + 3+ alphanumeric chars for manufacturer/format ID

**Field Content Validation**:
- Comma-separated fields
- Each field contains only valid NMEA data characters
- Proper escape sequence handling where applicable

**Checksum Validation**:
- XOR of all bytes between `$` and `*` (exclusive)
- Represented as two uppercase hex digits

## Processing Architecture

**Packet Detection (Stage 1)**
- Implements the 6 packet constraints defined in Solution Architecture
- Accepts non-standard formats like Unicore command acknowledgments that meet these constraints
- PacketFormat.Next() enforces structural requirements only

**Syntax Analysis (Stage 2)**  
- `CheckSyntax()` function analyzes packet and sets granular flags
- Each validation rule gets its own flag bit
- Enables fine-grained compliance assessment

**Message Processing Flow**
1. Create `Sentence` from any valid packet
2. Attempt to create `ApprovedSentence` if GNSS-compliant
3. Process via standard NMEA handlers if approved
4. Fall back to native message handlers for non-standard packets

This architecture enables:
- Standard NMEA processing for compliant sentences
- Unicore command acknowledgment handling via native handlers  
- Graceful degradation for partially compliant packets

## Testing Strategy

The design uses a table-driven testing approach with over 600 test cases, each consisting of a packet string and expected syntax flags.

**Table-Driven Test Structure**
- Each test case: `{name, packet, expectedFlags}`
- Expected flags of `0` indicate packets that should fail detection entirely
- Single collection of test cases validates both packet detection and syntax analysis
- Same test data used for `PacketFormat.Next()` and `CheckSyntax()` testing

**Comprehensive Coverage**
- **Boundary conditions**: Packet length limits (82 vs 128 characters), address format variations
- **Constraint validation**: Each of the 6 packet constraints tested with positive/negative cases  
- **Flag validation**: Each syntax flag tested independently with expected combinations
- **Real-world compatibility**: Actual NMEA sentences, Unicore acknowledgments, extended manufacturer IDs

**Validation Approach**
- Optimized implementation cross-validated against reference implementation
- Fuzzing ensures edge case coverage
- Positive tests verify valid packets are accepted with correct flags
- Negative tests ensure invalid packets are properly rejected

This unified testing strategy ensures both packet detection and syntax analysis work correctly on the same data set, maintaining consistency between the two processing stages.

## Benefits for Native Message Handlers

Native message handlers can use the pre-computed syntax flags to efficiently route and process different packet types:

**Efficient Packet Classification**
- Quick identification of standard NMEA vs. non-standard packets
- Single bit test to check GNSS talker compliance
- Separate handling paths for command acknowledgments vs. data sentences

**Reduced Validation Overhead**
- Syntax checking performed once during packet detection
- Flags enable fast filtering without re-parsing
- Composable validation logic for complex scenarios

**Enhanced Protocol Support**
- Unicore command acknowledgments handled alongside standard NMEA
- Extended proprietary manufacturer IDs supported
- Graceful degradation for partially compliant packets

The composite flags like `SentenceTalkerIsGNSS` enable native handlers to distinguish between standard GNSS sentences and non-standard formats with minimal overhead, supporting both strict NMEA compliance and protocol-specific extensions.