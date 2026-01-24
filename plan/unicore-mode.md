# MODE Property Implementation Plan

## Overview

This document describes the implementation plan for the `modeProp` in the Unicore GPS configuration system. The `modeProp` handles the MODE command which controls the receiver's operating mode (base station, rover, or heading).

## MODE Command Format

The MODE command supports three main operating modes:

### 1. ROVER Mode
- `MODE ROVER [param1] [param2]`
- Examples:
  - `MODE ROVER`
  - `MODE ROVER UAV`
  - `MODE ROVER SURVEY MOW`
  - `MODE ROVER AUTOMOTIVE DEFAULT`

### 2. BASE Mode
- `MODE BASE [ID] [parameters]`
- Parameters can be:
  - Nothing (default 60s survey)
  - `TIME duration [distance]` (survey mode)
  - `lat lon height` (fixed LLH coordinates)
  - `x y z` (fixed ECEF coordinates)
- Examples:
  - `MODE BASE`
  - `MODE BASE 123`
  - `MODE BASE TIME 60`
  - `MODE BASE 123 TIME 60 5`
  - `MODE BASE 40.45628476579 116.2859754968 58.0984`
  - `MODE BASE 123 -2160489.0276 4383620.1006 4084738.1110`

### 3. HEADING2 Mode
- `MODE HEADING2 [parameter]`
- Examples:
  - `MODE HEADING2`
  - `MODE HEADING2 FIXLENGTH`
  - `MODE HEADING2 STATIC`

## Regular Expression

The MODE regexp pattern is:
```regexp
^MODE (?:(ROVER(?: [A-Z]+)?(?: [A-Z]+)?)|(HEADING2(?: [A-Z]+)?)|(BASE)(?: (\d+))?(?: TIME (\d+)(?: (\d+))?| (-?\d+\.?\d*) (-?\d+\.?\d*) (-?\d+\.?\d*))?)$
```

### Capture Groups

| Group | Content | Description |
|-------|---------|-------------|
| 0 | Full match | The entire matched command |
| 1 | ROVER part | Complete ROVER command (e.g., "ROVER UAV HIGHDYN") |
| 2 | HEADING2 part | Complete HEADING2 command (e.g., "HEADING2 STATIC") |
| 3 | BASE | Literal "BASE" when in base mode |
| 4 | Base ID | Base station ID (0-4095, optional) |
| 5 | TIME duration | Survey duration in seconds |
| 6 | TIME distance | Survey distance parameter (optional) |
| 7 | Coordinate 1 | First coordinate (lat or X) |
| 8 | Coordinate 2 | Second coordinate (lon or Y) |
| 9 | Coordinate 3 | Third coordinate (height or Z) |

### Examples of Group Matching

| Command | Groups |
|---------|--------|
| `MODE ROVER` | 1="ROVER" |
| `MODE ROVER SURVEY MOW` | 1="ROVER SURVEY MOW" |
| `MODE HEADING2 STATIC` | 2="HEADING2 STATIC" |
| `MODE BASE` | 3="BASE" |
| `MODE BASE 123` | 3="BASE", 4="123" |
| `MODE BASE TIME 60` | 3="BASE", 5="60" |
| `MODE BASE 123 TIME 60 5` | 3="BASE", 4="123", 5="60", 6="5" |
| `MODE BASE 40.0 116.0 50.0` | 3="BASE", 7="40.0", 8="116.0", 9="50.0" |
| `MODE BASE 123 -2160489.0 4383620.1 4084738.1` | 3="BASE", 4="123", 7="-2160489.0", 8="4383620.1", 9="4084738.1" |

## Implementation Plan

### 1. Completed: Regexp and updateFromCommand
- ✅ Created regexp pattern to validate MODE commands
- ✅ Implemented `updateFromCommand` with validation
- ✅ Added comprehensive tests

### 2. Next: convertToProps
Convert parsed MODE command to gpsprot.ConfigProps:

**ROVER mode:**
- Set `Mode.Static = false`

**BASE mode:**
- Set `Mode.Static = true`
- Extract base ID → `SetRTCMBaseID()`
- If TIME present → `Mode.PosType = PosTypeNone` (survey mode)
- If coordinates present:
  - Detect LLH vs ECEF based on coordinate ranges
  - Set `Mode.PosType = PosTypeLLH` or `PosTypeECEF`
  - Set coordinates and default accuracy

**HEADING2 mode:**
- Not mapped to current gpsprot properties (vendor-specific)

### 3. Architecture Change Needed
The current architecture only passes `ConfigProps` to individual properties, but MODE needs access to `ConfigOptions` for `SetStatic` and `Survey` parameters.

**Required changes:**
1. Change `nativeConfigProps.updateFromProps(props *ConfigProps)` → `updateFromTarget(target *ConfigTarget)`
2. Change `generateConfigCommands(target *ConfigProps)` → `generateConfigCommands(target *ConfigTarget)`
3. Update individual property interfaces to handle ConfigTarget where needed
4. MODE property gets `updateFromTarget` method instead of `updateFromProps`

**Why this change:**
- Need access to `ConfigOptions.SetStatic` flag
- Need access to `ConfigOptions.Survey` parameters  
- Follows u-blox pattern from `createTmodeConfigs`
- Other properties can ignore ConfigOptions if not needed

### 4. Implementation Steps

**Step 1: Update nativeConfigProps interface**
- Change `updateFromProps` to `updateFromTarget` 
- Change `generateConfigCommands` parameter to `ConfigTarget`
- Update all existing properties to handle new interface

**Step 2: Implement MODE-specific logic**
- `modeProp.updateFromTarget` with priority logic:
  1. If `!Mode.Static && !SetStatic` → `MODE ROVER SURVEY`
  2. If `Mode.Static` or `SetStatic=true`:
     - With fixed position → `MODE BASE [id] [coordinates]`
     - Without position (PosType=None) → `MODE BASE [id] TIME [duration]`
     - Survey parameters from `ConfigOptions.Survey`
  3. If `SetStatic=true` overrides non-static Mode → treat as static
  4. Use `RTCMBaseID` for base station ID

**Step 3: Maintain backward compatibility**
- Other properties can extract just `target.Props` if they don't need options
- MODE property accesses both `target.Props` and `target.Opts`

**Command generation:**
- LLH: `MODE BASE [id] lat lon height`
- ECEF: `MODE BASE [id] x y z`
- Survey: `MODE BASE [id] TIME duration`
- Default: `MODE BASE [id]` or `MODE BASE`

### 4. Integration Notes

**Coordinate Detection:**
- LLH: lat ∈ [-90, 90], lon ∈ [-180, 180]
- ECEF: coordinates outside LLH ranges

**Survey Parameters:**
- Use `ConfigOptions.Survey.MinDur` for TIME duration
- Default to 60 seconds if not specified
- Max duration: 3600 seconds

**Base Station ID:**
- Optional parameter (0-4095)
- Used for RTCM base station identification
- Maps to `PropIDRTCMBaseID`

## Testing

Comprehensive tests cover:
- All MODE command variants
- Regexp capture group validation
- Invalid command rejection
- Edge cases (large IDs, negative coordinates, etc.)

The implementation follows the established pattern from `ppsProp` and other config properties in the codebase.