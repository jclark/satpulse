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

Handle a single composite property in ConfigProps, working name is Mode

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
* If Stationary is true and FixedPosECEF non-zero, then FixedPosAcc is accuracy of FixedPos
   * acc zero on read means receiver doesn't use the concept
   * what does acc zero on write mean? use application default
* If static is true and FixedPosECEF is zero, it means that the receiver determines the fixed position itself

Later extend to LLH
* have FixedPosLLH field
* have PosType enumeration to select PosTypeNone,PosTypeECEF,PosTypeLLH


UBX interpretation:
* on read
   * in TIM/FTS/HPG, static is true iff time mode is fixed or survey
   * on SPG, static is true iff dynmodel is stationary
* on write
  * in TIM/FTS/HPS
     * sets time mode to fixed if fixedposecef is fixed
     * otherwise sets time mode to survey
  * on SPG, sets dynmodel to stationary or portable

### SetStatic config option

In addition, have two fields in ConfigOptions:

   ```
   SetStatic bool
   ```

If true, sets the mode to static, but without affecting the fixedpos.

### Survey config option
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

* MinDur of zero means use sensible default (possibly configured value); non-zero must be greater than one second
* AccLimit of zero means don't constrain based on accuracy; just use time
* If we save to flash, then effect should be to perform the survey on startup
* SurveyAgain says to try to force the receiver to do a survey even if it has already done one
   * on gen8, set to disabled, and then set to survey
   * on gen9
      * if not survey again, then do not set survey params to same value
      * if survey again, then ensure survey params are set to a different value


Issues

- should we use number of observations rather than duration?
- should MinDur/AccLimit be a property?
  - inconsistent with FixedPos in Mode: setting that makes it use fixedpos
  - doesn't really make sense when it is a command to do something
  - makes sense as a property saved to Flash
  - but not really useful; when doing a survey, don't want to use the mindur/acclimit that happened to be configured; want to use sensible defaults

### Survey messages

Add PVTMsgSurvey flag added to PVTMsgs.

Semantics:
* Turn on survey messages if we initiated a survey
* If we didn't but we are in survey mode, then poll
* If PvtMsgOff is set, then turn off survey messages on a receiver than enables them
* Add this to set of messages enabled by events

Issues:

- when do we enable survey-in; only when we initiated a survey otherwise poll once
- what is the event that UBX-TIM-SVIN is tied to? will it keep generating after survey is complete, yes

## Common scenarios

* daemon behaviour
  * stationary TOML sets SetStatic option, but does not set Mode
  * surveyTime TOML sets Survey.MinDur option
  * fixedPos TOML sets mode to static, with the fixedPos
* cli
  * --survey sets Mode to static with no fixedPos
  * --fixedPos sets Mode to static with a fixedPos

## CLI change

Currently we have --disable-time-mode
Change to --mobile: semantics is to set Mode.Static to false

## Compared to current design

We have four properties currently
- TimeMode: three way enum mapping direct to ublox
- Stationary: corresponds to dyn model of stationary vs portable
- FixedPosECEF: corresponds to u-blox ECEF property, only does anything if TimeMode is TimeModeFixed
- FixedPosAcc: corresponds to u-blox fixedposacc property, only does anything it

Too much ties to u-blox

Survey has When property that is set of timemode flags for when to do survey.


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
than the value of “Distance”,
the receiver will set the
coordinates saved in Flash as
the base station coordinates.
The range of “Distance” is 0 ≤
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
