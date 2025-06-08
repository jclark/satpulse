# Plan for revising time mode configuration

Goals
- working with timing receivers with a time mode
- work with RTK receivers with base mode
- work standard precision receivers with dynamic model 
- work with stapulsetool gps
- work with satpulsed: do a survey if supported and no fixed position configured
- avoid UBX dependencies
- handle survey progress messages

## TOML interface

* surveyTime number in seconds
* surveyAcc number in meters
* stationary bool (default true)
* resurvey bool (default false)
* fixedPosECEF [X,Y,Z]
* fixedPosAcc

Does a survey if stationary is true and no fixedPos;
if already in survey mode, don't resurvey unless resurvey is true.

## Design of abstract interface in gpsprot

### Mode property

A single composite property in ConfigProps (replacing the existing TimeMode, Stationary, FixedPosECEF, and FixedPosAcc properties).

```
type Mode struct {
   Static bool
   FixedPosECEF  ECEF
   FixedPosAcc Length
}
```

Constraints:
* if Static is false, then other fields must be zero
* if FixedPosECEF is zero, then FixedPosAcc must be zero

Semantics:
* Static true requests receiver to operate in a mode where it assumes position (of antenna) does not move
* If Static is true, then FixedPosECEF, if non-zero, gives coordinates in ECEF of antenna position
* If Static is true and FixedPosECEF non-zero, then FixedPosAcc is accuracy of FixedPos
   * acc zero on read means receiver doesn't use the concept
   * acc zero on write means use sensible device-dependent default
* If static is true and FixedPosECEF is zero, it means that the receiver determines the fixed position itself (typically through survey)

Later extend to LLH
* have FixedPosLLH field
* have PosType enumeration to select PosTypeNone,PosTypeECEF,PosTypeLLH


UBX interpretation:
* on read
   * in TIM/FTS/HPG, static is true iff time mode is fixed or survey
   * on SPG, static is true iff dynmodel is stationary
* on write
  * in TIM/FTS/HPG
     * sets time mode to fixed if fixedposecef is provided
     * otherwise sets time mode to survey
  * on SPG, sets dynmodel to stationary or portable

### SetStatic config option

A configuration option in ConfigOptions that ensures the receiver is in static mode without changing any existing fixed position configuration.

   ```
   SetStatic bool
   ```

Semantics:
* SetStatic ensures that Mode.Static will be true after configuration
* Does NOT modify any existing FixedPosECEF or FixedPosAcc values
* Allows the daemon to request static mode while preserving user configuration

Typical daemon usage:
* If user has already configured the receiver with a fixed position (Mode.Static=true with FixedPosECEF set), SetStatic preserves this
* If receiver is unconfigured (Mode.Static=false or TimeMode=disabled), SetStatic will enable static mode, triggering a survey if no fixed position exists

### Survey config option

Provide parameters to use if survey is triggered. Also allow control over whether to do another survey, when receiver is already in survey mode.

```
type SurveyFlags int

const (
  SurveyAgain SurveyFlags = 1 << iota  // do a survey even if we have done one already
)

type Survey struct {
  Flags    SurveyFlags
  MinDur   time.Duration // survey should run at least this long
  AccLimit Length        // survey should run until this accuracy is achieved
}
```

Semantics:

* MinDur of zero means use sensible default (possibly from TOML configuration); non-zero must be greater than one second
* AccLimit of zero means don't constrain based on accuracy; just use time
* If saved to flash, survey will be performed on receiver startup
* SurveyAgain forces the receiver to do a new survey even if it has already completed one
   * Implementation details for forcing new survey are receiver-specific (hidden from user)
   * on gen8, set to disabled, and then set to survey
   * on gen9
      * if not survey again, then do not set survey params to same value
      * if survey again, then ensure survey params are set to a different value

We have chosen not to make MinDur/AccLimit a property
  - would be inconsistent with FixedPosECEF in Mode: setting that makes it use fixedpos
  - doesn't really make sense when it is a command to do something
  - (although does makes sense as a property saved to Flash)
  - but not really useful; when doing a survey, don't want to use the mindur/acclimit that happened to be configured; want to use sensible defaults

### Survey messages

Add PVTMsgSurvey flag to PVTMsgFlags for controlling survey progress messages.

Semantics:
* Turn on survey messages if we initiated a survey
* If we didn't initiate but receiver is in survey mode, poll for status
* If PVTMsgOff is set, turn off survey messages on receivers that support them

Implementation notes:
- UBX-TIM-SVIN continues generating after survey completion

## Common scenarios

### Daemon behaviour
* `stationary=true` in TOML → sets SetStatic option (does NOT set Mode property)
* `surveyTime` in TOML → sets Survey.MinDur option  
* `surveyAcc` in TOML → sets Survey.AccLimit option
* `fixedPosECEF` in TOML → sets Mode property with Static=true and the position
* `resurvey=true` in TOML → sets SurveyAgain flag

### CLI behaviour
* `--mobile` → sets Mode.Static to false
* `--survey` → requests a survey; sets Mode.Static to true and zeros Mode.FixedPosECEF
* `--fixedPos` → sets Mode with Static=true and a fixed position

## CLI change

Replace `--disable-time-mode` with `--mobile`
* Semantics: sets Mode.Static to false
* More intuitive naming that aligns with the Static field

## Migration from current design

Current design has four separate properties:
- TimeMode: three-way enum (disabled/survey/fixed) tied to u-blox concepts
- Stationary: corresponds to dynamic model (stationary vs portable)
- FixedPosECEF: only used when TimeMode is TimeModeFixed
- FixedPosAcc: only used when TimeMode is TimeModeFixed

New design consolidates these into one.

In terms of options. Complicated/confusing Survey.When option is replaced by
- SetStatic option, with clear semantics relative to Mode, and
- SurveyAgain flag

Problems with current design:
- Too tied to u-blox specific concepts
- Unclear interaction between TimeMode and Stationary
- Survey.When is complicated/confusing

## Implementation Plan

### UBX tmodeConfig intermediate representation

To reduce code duplication between legacy (TMODE/TMODE2/TMODE3) and modern (cfgval) implementations, introduce an intermediate `tmodeConfig` struct that represents time mode configuration in a format close to the gen 9 configuration keys:

```go
type tmodeConfig struct {
    // Core time mode fields
    mode          uint8   // 0=disabled, 1=survey, 2=fixed

    // Fixed position fields (split high-precision format)
    ecefHP        [3]int8   // High-precision fractional parts in 0.1mm
    ecef          [3]int32  // Main ECEF values in cm
     
    // Future LLH support
    useLLH        bool
    latLonHP      [2]int8   // Lat/Lon high-precision fractional parts in degrees * 1e-9
    heightHP      int8      // Height high-precision fractional part in 0.1mm
    latLon        [2]int32  // Lat/Lon main values in degrees * 1e-7
    height        int32     // Height main value in cm (same units as ECEF)

    fixedPosAcc   uint32     // Position accuracy in 0.1mm units

    // Survey parameters  
    svinMinDur    uint32    // seconds
    svinAccLimit  uint32     // Survey accuracy limit in 0.1mm units
}
```

### Translation layers

1. **ConfigTarget ↔ tmodeConfig**: Apply Mode property and SetStatic logic, translate between abstract interface and UBX concepts
2. **tmodeConfig ↔ wire formats**: Handle format-specific conversions:
   - **TMODE**: Position in cm, accuracy as mm² variance
   - **TMODE2**: Position in cm, accuracy in mm  
   - **TMODE3**: Split format with direct mapping (cm + 0.1mm)
   - **cfgval**: Convert split format to 0.1mm units
3. **Wire formats → tmodeConfig**: Parse responses and build tmodeConfig
4. **tmodeConfig → ConfigProps**: Build Mode property from tmodeConfig

### Benefits

- **Code reuse**: Common translation logic shared between legacy and modern implementations
- **Type safety**: Intermediate representation prevents unit confusion
- **Future extensibility**: Easy to add new receiver generations or formats
- **Cleaner testing**: Translation layers can be tested independently
- **Reduced complexity**: Hide format-specific details in translation layer

## Background

### U-blox

- On F9T, TIM 2.01
   * disabling time mode while survey is in progress aborts it
   * when survey is complete or in progress, sending a changed accuracy limit or min duration starts a new survey 

### Quectel LG290P

PQTMCFGRCVRMODE sets receiver mode base station, rover
* but base station mode enables RTCM but also disables NMEA!
* option to enable protocol on port
* needs reset to work

PQTMCFGSVIN sets/gets Survey-in feature

Three receiver modes:
* disable
* survey-in
* fixed mode

In survey-in mode
* minimum positioning time (as a count of number of observations)
* 3D accuracy limit in meters (as decimal); zero means no limit
In fixed mode, ECEF is specified (but no accuracy)

PQTMSVINSTATUS survey-in status
* mean ECEF position
* mean Acc
* number of observations

Survey-in status requires base station mode

### Allystar binary

CFG-FIXEDECEF sets fixed ECEF position
CFG-SURVEY
- minimum survey time in seconds
- accuracy limit in mm

Can be both polled and set


### Unicore

MODE command
* without parameters, queries
* specify base with RTCM id, and either ECEF or LLH
* self-optimizing base with TIME parameter
   * maximum time up to 3600s
   * distance parameter

Semantics of distance are strange

> Distance, in meters. The
receiver starts in self-
optimizing base station mode
and saves the optimized
position in Flash. When the
receiver restarts, it optimizes
the position again. If the
distance between the
optimized coordinates and
that saved in Flash is less
than the value of "Distance",
the receiver will set the
coordinates saved in Flash as
the base station coordinates.
The range of "Distance" is 0 ≤
Distance ≤ 10. If Distance = 0,
the receiver will start in self-
optimizing base station mode
and set the optimized result
as the coordinates of the base
station.

### CASIC

CFG-TMODE like UBX

* mode - disabled, survey, fixed
* in fixed
  * ECEF position
  * 3D-variance of fixedPos
* in survey
  * min duration
  * position err limit in m^2

## COMNAV

Fix position (in LLH)
Fix AUTO

### Skytraq

CONFIGURE 1PPS TIMING – Configure 1PPS timing of the GNSS receiver (0x54)

3 modes
- timing PVT mode
- timing survey mode
  * survey length
  * survey stddev
- timing static mode
  * LLA (not ECEF!)

Flag to say whether to update to Flash as well as RAM.


### novatel OEM7

FIX AUTO (says fix height at last position)
FIX POSITION L L H
FIX NONE

### bynav

RTKMODE sets base/rover

FIX BASE is like survey mode
similar semantics to unicore

> Distance, measured in meters. The receiver autonomously optimizes and
sets the base station mode startup, and the optimized coordinates will be
saved to flash memory. When the receiver restarts, it will again calculate
the coordinates using the autonomous optimization method (discarding
the first 60 seconds of single-point data and using the data from the
following 60 seconds; before the optimization is complete, the base
station coordinates will not be output). If the newly calculated
coordinates are within the 'Distance' from the coordinates stored in flash,
the receiver will use the coordinates stored in flash as the base station
coordinates. The 'Distance' value range: 0 ≤ Distance ≤ 100. When
Distance = 0, the receiver will use the autonomous optimization method
to start in base station mode and use the current optimization result as
the base station coordinates.

Also has FIX AUTO/POSITION/NONE

### Septentrio

setPVTMode can be static/rover
with static you can specific the reference position as auto or specific ECEF or LLH position,
specified indirectly with setStaticPosCartesion/Geodetic