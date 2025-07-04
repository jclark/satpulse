# Plan for Implementing Time Mode for New Style Config

## Overview
The main task is to refactor `tmodeConfigsTargetOld` to work for both legacy and new style config, and implement a new `timeModeBuild` function. The key differences are:
- **Legacy (Gen8)**: Force resurvey by disabling then re-enabling survey mode
- **Gen9**: Force resurvey by changing survey parameters slightly (±1 in svinAccLimit = ±0.1mm)

## Implementation Plan

### 1. Add `resurveyMethod` enum type
Create an enum to specify how to handle forced resurveys:
- `resurveyDisable`: Disable then re-enable survey mode (legacy behavior)
- `resurveyChange`: Change survey parameters slightly (gen9 behavior)

### 2. Refactor `tmodeConfigsTargetOld` to `createTmodeConfigs`
Rename and refactor the function to work for both legacy and new style config:
- Add `resurveyMethod` parameter to control resurvey behavior
- Keep the same core logic for determining what tmodeConfigs are needed
- Handle the `resurveyChange` method by slightly modifying survey parameters instead of returning disable/enable sequence
- Function signature: `func createTmodeConfigs(target *gpsprot.ConfigTarget, cur *tmodeConfig, method resurveyMethod) (*tmodeConfig, *tmodeConfig, error)`

### 3. Update `changeTmode` in `ubxcfgold.go`
Update the legacy implementation to call the new function with `resurveyDisable` method.

### 4. Analyze info level requirements
Based on careful analysis of `createTmodeConfigs` implementation, determine exactly when `cur` is accessed:

**Analysis of `cur` access in `createTmodeConfigs`:**
1. **NEVER accessed**: When `!haveMode && !setStatic` (early return, lines 70-71)
2. **Mode needed**: In all other cases:
   - Line 77: `if cur.mode == tmodeFixed` 
   - Line 82: `if cur.mode == tmodeSurveyIn && ...`
   - Line 103: `if cur.mode == tmodeSurveyIn` 
3. **Survey parameters needed**: Only when `resurveyChange` method AND we reach line 107: `if tmc.svinAccLimit == cur.svinAccLimit`

**Info requirements:**
- **No info needed**: When `!haveMode && !setStatic` 
- **Mode only**: Most cases (sufficient for legacy `resurveyDisable`)
- **Mode + survey**: For `resurveyChange` scenarios when comparing `svinAccLimit`
- **Fixed parameters**: Never needed by `createTmodeConfigs`

**Proposed info flags:**
- `tmodeInfoMode`: Just the current time mode
- `tmodeInfoSurvey`: Survey parameters regardless of current mode
- `tmodeInfoFixed`: Fixed position parameters regardless of current mode
- `tmodeInfoRelevant`: Mode-dependent behavior - only fetch parameters relevant to the actual mode (survey params for survey mode, fixed params for fixed mode). This is needed because `ubxcfgvals` indirectly uses this in Cook functions where we don't know in advance what mode we're in.
- `tmodeInfoAll`: Everything (equivalent to `Mode|Survey|Fixed`)

### 5. Design and implement info flags system
Add flags to `tmodeConfig.fromCfgVals()` to control what information to retrieve:

**New function signature:**
```go
func (tc *tmodeConfig) fromCfgVals(vals *CfgVals, info tmodeInfo) bool
```

**Info flags design:**
```go
type tmodeInfo uint8

const (
    tmodeInfoMode   tmodeInfo = 1 << iota // Just mode
    tmodeInfoSurvey                       // Survey parameters regardless of mode
    tmodeInfoFixed                        // Fixed position parameters regardless of mode
    tmodeInfoAll    = tmodeInfoMode | tmodeInfoSurvey | tmodeInfoFixed // Everything
)
```

**Implementation approach:**
- Update `fromCfgVals` to only require specific keys to be available based on flags
- With `tmodeInfoMode`, only `KTmodeMode` key is required to be present
- With `tmodeInfoSurvey`, survey-related keys must be present
- With `tmodeInfoFixed`, fixed position keys must be present
- Add helper functions to determine required info level based on operation
- Ensure backward compatibility with existing `fromCfgVals(vals, true/false)` calls

### 6. Add helper function to determine required keys for info levels
Add a function that maps info levels to the specific CFG-VAL keys that need to be fetched.
Note: `tmodeRequiredKeys` cannot handle `tmodeInfoRelevant` since it requires knowing the mode first:

**Function signature:**
```go
func tmodeRequiredKeys(info tmodeInfo) []ucv.Key
```

**Implementation:**
```go
func tmodeRequiredKeys(info tmodeInfo) []ucv.Key {
    var keys []ucv.Key
    
    if info&tmodeInfoMode != 0 {
        keys = append(keys, ucv.KTmodeMode)
    }
    
    if info&tmodeInfoSurvey != 0 {
        keys = append(keys, 
            ucv.KTmodeSvinMinDur,
            ucv.KTmodeSvinAccLimit,
        )
    }
    
    if info&tmodeInfoFixed != 0 {
        keys = append(keys,
            ucv.KTmodePosType,
            ucv.KTmodeEcefX, ucv.KTmodeEcefY, ucv.KTmodeEcefZ,
            ucv.KTmodeEcefXHp, ucv.KTmodeEcefYHp, ucv.KTmodeEcefZHp,
            ucv.KTmodeLat, ucv.KTmodeLon, ucv.KTmodeHeight,
            ucv.KTmodeLatHp, ucv.KTmodeLonHp, ucv.KTmodeHeightHp,
            ucv.KTmodeFixedPosAcc,
        )
    }
    
    return keys
}
```

### 7. Add helper function to determine required info for resurveyChange case
Add a function that analyzes a `gpsprot.ConfigTarget` to determine what `tmodeInfo` flags are needed for the `resurveyChange` method:

**Function signature:**
```go
func tmodeRequiredInfoResurveyChange(target *gpsprot.ConfigTarget) tmodeInfo
```

**Implementation:**
```go
func tmodeRequiredInfoResurveyChange(target *gpsprot.ConfigTarget) tmodeInfo {
    mode, haveMode := target.Props.GetMode()
    survey := target.Opts.Survey
    
    // If we might need to do resurveyChange, we need survey info
    if haveMode && mode.Static && mode.PosType == gpsprot.PosTypeNone && 
       survey.Flags&gpsprot.SurveyAgain != 0 {
        return tmodeInfoSurvey
    }
    
    // For most cases, just mode is sufficient
    return tmodeInfoMode
}
```

This helper will be called by `timeModeBuild` to efficiently determine what information to request before calling `createTmodeConfigs`.

### 8. Implement new `timeModeBuild` function for gen9
Create the new function for gen9 that uses CFG-VAL configuration:

**Function signature:**
```go
func timeModeBuild(target *gpsprot.ConfigTarget, vals *CfgVals) ([]ucv.Item, error)
```

**Implementation steps:**
1. **Determine required info level:**
   ```go
   info := tmodeRequiredInfoResurveyChange(target)
   ```

2. **Check if required keys are available, request them if not:**
   ```go
   requiredKeys := tmodeRequiredKeys(info)
   if !vals.HasKeys(requiredKeys) {
       // Add missing keys to request list and return
       vals.RequestKeys(requiredKeys)
       return nil, nil // Will be called again after keys are fetched
   }
   ```

3. **Get current config efficiently:**
   ```go
   var cur tmodeConfig
   if !cur.fromCfgVals(vals, info) {
       return nil, fmt.Errorf("insufficient tmode info available")
   }
   ```

4. **Generate target configs:**
   ```go
   first, second, err := createTmodeConfigs(target, &cur, resurveyChange)
   ```

5. **Convert to config items:**
   ```go
   var items []ucv.Item
   if first != nil {
       first.toItems(&items, false)
   }
   if second != nil {
       // Handle the two-phase case for forced resurvey
       items = append(items, ucv.Item{...}) // separator or special handling
       second.toItems(&items, false)
   }
   ```

**Helper function for info determination:**
```go
func determineRequiredInfo(target *gpsprot.ConfigTarget) tmodeInfo {
    mode, haveMode := target.Props.GetMode()
    survey := target.Opts.Survey
    
    // If we might need to do resurveyChange, we need survey info
    if haveMode && mode.Static && mode.PosType == gpsprot.PosTypeNone && 
       survey.Flags&gpsprot.SurveyAgain != 0 {
        return tmodeInfoSurvey
    }
    
    // For most cases, just mode is sufficient
    return tmodeInfoMode
}
```

## Progress

### ✅ COMPLETED: Steps 1-3

#### Step 1: ✅ Added `resurveyMethod` enum type
Successfully implemented in `ubxcfgtmode.go:26-34` with `resurveyDisable` and `resurveyChange` constants.

#### Step 2: ✅ Refactored to `createTmodeConfigs`
Successfully renamed and refactored `tmodeConfigsTargetOld` to `createTmodeConfigs` in `ubxcfgtmode.go:67-113`. The implementation is simpler and more robust than originally planned:
- Converts SetStatic scenarios to equivalent `Mode{Static:true}` for cleaner logic
- Handles `resurveyChange` method by incrementing `svinAccLimit` when it equals current value
- Uses consistent error handling via `newTmodeConfig`

#### Step 3: ✅ Updated legacy code
Successfully updated `ubxcfgold.go` to call `createTmodeConfigs(target, cur, resurveyDisable)`.

#### Additional: ✅ Added test coverage
Updated `TestCreateTmodeConfigs` to include `resurveyMethod` field in all test cases with `resurveyDisable` value.

### ⏳ REMAINING: Step 8
- Implement new `timeModeBuild` function for gen9

### ✅ COMPLETED: Steps 4-7

#### Step 4: ✅ Analyzed info requirements 
Completed requirements analysis showing exactly when `cur` is accessed in `createTmodeConfigs`.

#### Step 5: ✅ Designed and implemented info flags system
Successfully implemented the info flags system for `tmodeConfig.fromCfgVals()` including `tmodeInfoRelevant` for Cook functions in `ubxcfgvals`. All tests passing.

#### Step 6: ✅ Added helper function to determine required keys for info levels
Successfully implemented `tmodeRequiredKeys(info tmodeInfo) []ucv.AnyTypedKey` function that maps info levels to specific CFG-VAL keys. Returns `[]ucv.AnyTypedKey` for compatibility with the typed key system.

#### Step 7: ✅ Added helper function to determine required info for resurveyChange case
Successfully implemented `tmodeRequiredInfoResurveyChange(target *gpsprot.ConfigTarget) tmodeInfo` function that analyzes a target configuration to determine what info flags are needed for the resurveyChange method.

## Key Benefits of This Approach
- Implementation-driven design: Understand actual requirements before over-engineering
- Early validation: Test core logic changes before adding complexity
- Clear separation: Resurvey method handling is separated from info management
- Incremental progress: Each step provides working functionality