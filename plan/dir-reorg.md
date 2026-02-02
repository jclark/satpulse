# Directory Reorganization Plan

Goals:
1. **Separate GPS-specific code**: Group GPS functionality under `gps/` with no external dependencies, so it can be split into a separate module.
2. **Enforce internal boundaries**: Use Go's `internal/` visibility to enforce layer separation, preventing external access to implementation details (`time/internal/`, `gps/internal/`).

## Pre-reorganization Refinements

These changes should be made before the directory reorganization to ensure clean separation between GPS and time-sync code, and to simplify the reorganization itself.

### Flatten nested bin packages

Rename nested `bin` packages so that the reorganization doesn't change package names:

- `internal/ubx/bin` → `internal/ubxbin`
- `internal/casic/bin` → `internal/casbin`

(`internal/asbin` is already flat.)

### Tag enum in gpsreg (done)

Done. Added to `gpsreg`:

```go
const (
	TagUBX          = ubx.Tag
	TagNMEA         = nmea.Tag
	TagRTCM         = rtcm.Tag
	TagCASICBin     = casic.Tag
	TagAllystarBin  = as.Tag
	TagUnicoreBin   = unc.TagBinary
	TagUnicoreAscii = unc.TagAscii
	TagNovAtelBin   = nov.TagBinary
	TagNovAtelAscii = nov.TagAscii
)
```

Updated packages to use `gpsreg.TagXxx` instead of importing protocol packages:
- `timemsg` - now uses `gpsreg.TagNMEA`, `gpsreg.TagUBX`
- `syncsim` - now uses `gpsreg.TagUBX`
- `daemon` (tests) - now uses `gpsreg.TagNMEA`, `gpsreg.TagRTCM`

### Factor gpsmsg from gpscmd

Factor out protocol-aware code from `gpscmd` into a new `gps/gpsmsg/` package:
- `msgfile.go` - message file handling
- `response.go` - response formatting

After this refactoring:
- `gps/gpsmsg/` - protocol-aware code that needs access to `gps/internal/`
- `internal/gpscmd/` - CLI orchestration, imports `gps/gpsmsg/`

This allows `gpscmd` to stay in `internal/` (command layer) while protocol-specific code moves to `gps/`.

### Factor daemon-specific code from cmd

Move daemon-specific error handling from `cmd` to `daemon`:
- `ConfigError` type
- `ConfigErrorf()` function
- `ExitConfig` constant
- `ExitCode()` function (depends on `ConfigError`)

This leaves `cmd` with general CLI utilities (`ErrPrintln`, `CancelOnSignal`, `NewLogger`, `VersionInfo`, `UsageFunc`) that are appropriate for `gps/lib/cmd`.

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
| `gps/gpsdecode/` | `internal/gpsdecode` |
| `gps/gpsio/` | `internal/gpsio` |
| `gps/gpsmsg/` | new (factored from `internal/gpscmd`) |
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
| **gps/lib/** | |
| `gps/lib/asbin/` | `internal/asbin` |
| `gps/lib/casbin/` | `internal/casic/bin` |
| `gps/lib/cmd/` | `internal/cmd` |
| `gps/lib/fieldenc/` | `internal/fieldenc` |
| `gps/lib/geopos/` | `internal/geopos` |
| `gps/lib/logfile/` | `internal/logfile` |
| `gps/lib/novmsg/` | `internal/novmsg` |
| `gps/lib/ntptime/` | `internal/ntptime` |
| `gps/lib/term/` | `term` |
| `gps/lib/ubxbin/` | `internal/ubx/bin` |
| `gps/lib/ubxcfgval/` | `internal/ubxcfgval` |
| `gps/lib/ubxcfgval/cfgschema/` | `internal/ubxcfgval/cfgschema` |
| `gps/lib/uncmsg/` | `internal/uncmsg` |
| **time/** | |
| `time/phctime/` | new (split from `internal/ptime`) |
| `time/phc/` | `internal/phc` |
| `time/sockrefclock/` | `internal/sockrefclock` |
| `time/clocksim/` | `internal/clocksim` |
| `time/daemon/` | `internal/daemon` |
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
| **time/lib/** | |
| `time/lib/allan/` | `internal/allan` |
| `time/lib/check/` | `internal/check` |
| `time/lib/circbuf/` | `internal/circbuf` |
| `time/lib/devnotify/` | `internal/devnotify` |
| `time/lib/fuser/` | `internal/fuser` |
| `time/lib/ifwait/` | `internal/ifwait` |
| `time/lib/median/` | `internal/median` |
| `time/lib/pmc/` | `internal/pmc` |
| `time/lib/sse/` | `internal/sse` |
| **internal/** (satpulsetool subcommands) | |
| `internal/gpscmd/` | `internal/gpscmd` |
| `internal/decodecmd/` | `internal/decodecmd` |
| `internal/sdpcmd/` | `internal/sdpcmd` |
| `internal/pmccmd/` | `internal/pmccmd` |
| `internal/syncsimcmd/` | `internal/syncsimcmd` |
| **web/** | |
| `web/` | `web` |

## Layer Visibility

The structure uses Go's `internal/` visibility to enforce layer boundaries:

- **gps/** - GPS library. Public API packages with no external dependencies. Can be split into a separate module.
- **gps/internal/** - Protocol-specific implementations. Only importable by packages under `gps/`.
- **gps/lib/** - Reusable libraries that can be imported by packages outside `gps/`.
- **time/** - Time sync library. Depends on `gps/`.
- **time/internal/** - Application layer implementation. Only importable by packages under `time/`.
- **time/lib/** - Reusable libraries that can be imported by packages outside `time/`.
- **internal/** - Command layer for satpulsetool subcommands. Can import from `gps/`, `time/`, and their libs. Not exposed for external use.

## Rejected Alternative: *cmd packages in gps/ and time/

An alternative structure would place `gpscmd` and `decodecmd` in `gps/`, and `sdpcmd`, `pmccmd`, `syncsimcmd` in `time/`.

**Why this was rejected:**

`satpulsetool gps --pvt-out daemon` configures the GPS to output the PVT messages that the daemon needs. This is a logical dependency of the gps tool on the daemon's requirements, expressed via the `TimePulsePVTMsgFlags` constant in `gpsevent`.

With `gpscmd` in `gps/` and `gpsevent` in `time/internal/`, this creates a dependency from `gps/` to `time/`, breaking the goal that `gps/` has no external dependencies.

The chosen structure (all `*cmd` packages in `internal/`) solves this: `internal/gpscmd/` can import `time/internal/gpsevent/` since the command layer can depend on both `gps/` and `time/`.
