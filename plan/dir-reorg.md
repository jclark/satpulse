# Directory Reorganization Plan

Goals:
1. **Separate GPS-specific code**: Group GPS functionality (`gps/` + `lib/gps/`) so it can be split into a separate module.
2. **Enforce internal boundaries**: Use Go's `internal/` visibility to enforce layer separation, preventing external access to implementation details (`time/internal/`, `gps/internal/`).

## Pre-reorganization Refinements

These changes should be made before the directory reorganization to ensure clean separation between GPS and time-sync code, and to simplify the reorganization itself.

### Flatten nested bin packages

Rename nested `bin` packages so that the reorganization doesn't change package names:

- `internal/ubx/bin` → `internal/ubxbin`
- `internal/casic/bin` → `internal/casbin`

(`internal/asbin` is already flat.)

### Tag enum in gpsreg

The `gpsreg` package needs to export protocol Tag constants for use by packages outside the GPS group. Currently, packages like `timemsg` and `syncsim` import protocol-specific packages (`nmea`, `ubx`) solely to access their `Tag` constants.

Add to `gpsreg`:

```go
// Protocol tags for external use
var (
    UBXTag   = ubx.Tag
    NMEATag  = nmea.Tag
    RTCMTag  = rtcm.Tag
    CASICTag = casic.Tag
    UNCTag   = unc.Tag
    NOVTag   = nov.Tag
    ASTag    = as.Tag
)
```

This allows packages outside `gps/` to reference protocol tags via `gpsreg.UBXTag` etc., without importing `gps/internal/` packages directly.

Affected packages:
- `timemsg` - uses `nmea.Tag`, `ubx.Tag`
- `syncsim` - uses `ubx.Tag`
- `daemon` (tests) - uses `nmea.Tag`, `rtcm.Tag`

### Split ptime: create phctime package

The `ptime` package contains both GPS time concepts and PHC clock-tracking concepts. These should be separated so GPS code doesn't depend on time-sync code.

**Create new `phctime` package** with PHC clock-tracking types:
- `Era` - represents a period without PHC clock steps
- `Time` - `ptime.Time` combined with `Era` (was `ptime.ClockTime`)
- `Sample` - `phctime.Time` paired with `time.Time` (system clock)

**Keep in `ptime`** (GPS time concepts):
- `Time` - TAI nanoseconds timestamp
- `UTCTime` - date + time of day
- `LeapSecond`, `LeapSecondState`, `LeapSecondKind`
- `GPS()`, `Galileo()`, `BeiDou()`, `GLONASS()` converters
- `GNSSLeapSecond` and related functions
- `CorrectionParams`
- `Picoseconds()`, `Seconds()` utilities

The `phctime` package imports `ptime` (for `Time`), but `ptime` has no dependency on `phctime`. This maintains the fundamental principle: GPS produces time, time-sync consumes it.

## New Directory Structure

| New Location | Current Location |
|--------------|------------------|
| **cmd/** | |
| `cmd/satpulsed/` | `cmd/satpulsed` |
| `cmd/satpulsetool/` | `cmd/satpulsetool` |
| `cmd/ifwait/` | `cmd/ifwait` |
| **gps/** | |
| `gps/gpscfg/` | `internal/gpscfg` |
| `gps/gpscmd/` | `internal/gpscmd` |
| `gps/gpsio/` | `internal/gpsio` |
| `gps/gpsprot/` | `internal/gpsprot` |
| `gps/gpsreg/` | `internal/gpsreg` |
| `gps/ptime/` | `internal/ptime` |
| `gps/scan/` | `internal/scan` |
| **gps/internal/** | |
| `gps/internal/as/` | `internal/as` |
| `gps/internal/casic/` | `internal/casic` |
| `gps/internal/nmea/` | `internal/nmea` |
| `gps/internal/nov/` | `internal/nov` |
| `gps/internal/rtcm/` | `internal/rtcm` |
| `gps/internal/scantest/` | `internal/scantest` |
| `gps/internal/sino/` | `internal/sino` |
| `gps/internal/ubx/` | `internal/ubx` |
| `gps/internal/unc/` | `internal/unc` |
| **time/** | |
| `time/phctime/` | new (split from `internal/ptime`) |
| `time/phc/` | `internal/phc` |
| `time/sockrefclock/` | `internal/sockrefclock` |
| `time/clocksim/` | `internal/clocksim` |
| `time/daemon/` | `internal/daemon` |
| `time/syncsimcmd/` | `internal/syncsimcmd` |
| **time/internal/** | |
| `time/internal/ts/` | `internal/ts` |
| `time/internal/gpsevent/` | `internal/gpsevent` |
| `time/internal/phcsync/` | `internal/phcsync` |
| `time/internal/timemsg/` | `internal/timemsg` |
| `time/internal/ptpgm/` | `internal/ptpgm` |
| `time/internal/refclock/` | `internal/refclock` |
| `time/internal/obs/` | `internal/obs` |
| `time/internal/obs/promobs/` | `internal/obs/promobs` |
| `time/internal/obs/sseobs/` | `internal/obs/sseobs` |
| `time/internal/logobs/` | `internal/logobs` |
| `time/internal/statsobs/` | `internal/statsobs` |
| `time/internal/syncsim/` | `internal/syncsim` |
| `time/internal/proxy/` | `internal/proxy` |
| `time/internal/bcast/` | `internal/bcast` |
| `time/internal/logfile/` | `internal/logfile` |
| **lib/** | |
| `lib/allan/` | `internal/allan` |
| `lib/check/` | `internal/check` |
| `lib/circbuf/` | `internal/circbuf` |
| `lib/devnotify/` | `internal/devnotify` |
| `lib/fuser/` | `internal/fuser` |
| `lib/ifwait/` | `internal/ifwait` |
| `lib/median/` | `internal/median` |
| `lib/ntptime/` | `internal/ntptime` |
| `lib/pmc/` | `internal/pmc` |
| `lib/sse/` | `internal/sse` |
| `lib/term/` | `term` |
| `lib/fieldenc/` | `internal/fieldenc` |
| **lib/gps/** | |
| `lib/gps/asbin/` | `internal/asbin` |
| `lib/gps/casbin/` | `internal/casic/bin` |
| `lib/gps/geopos/` | `internal/geopos` |
| `lib/gps/novmsg/` | `internal/novmsg` |
| `lib/gps/ubxbin/` | `internal/ubx/bin` |
| `lib/gps/ubxcfgval/` | `internal/ubxcfgval` |
| `lib/gps/ubxcfgval/cfgschema/` | `internal/ubxcfgval/cfgschema` |
| `lib/gps/uncmsg/` | `internal/uncmsg` |
| **internal/** (command layer - no time/internal/ deps) | |
| `internal/cmd/` | `internal/cmd` |
| `internal/sdpcmd/` | `internal/sdpcmd` |
| `internal/pmccmd/` | `internal/pmccmd` |
| **web/** | |
| `web/` | `web` |

## Layer Visibility

The structure uses Go's `internal/` visibility to enforce layer boundaries:

- **time/** - Public API of the time sync module.
- **time/internal/** - Application layer implementation. Can only be imported by packages under `time/`.
- **time/daemon/**, **time/syncsimcmd/** - Public interfaces to `time/internal/`. These expose the internal functionality to command-line programs.
- **time/phctime/**, **time/phc/**, **time/sockrefclock/**, **time/clocksim/** - Domain layer. Low-level abstractions usable independently of the application layer.
- **internal/** - Command layer packages that only use public APIs (`time/`, `gps/`, `lib/`).

Similarly for `gps/`:
- **gps/** - Public API (domain + application packages).
- **gps/internal/** - Protocol-specific implementations.
