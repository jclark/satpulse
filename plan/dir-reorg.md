# Directory Reorganization Plan

Goals:
1. **Separate GPS-specific code**: Group GPS functionality under `gps/` with no external dependencies, so it can be split into a separate module.
2. **Enforce internal boundaries**: Use Go's `internal/` visibility to enforce layer separation, preventing external access to implementation details (`time/internal/`, `gps/internal/`).

## Pre-reorganization Refinements

These changes should be made before the directory reorganization to ensure clean separation between GPS and time-sync code, and to simplify the reorganization itself.

### Flatten nested bin packages (done)

Rename nested `bin` packages so that the reorganization doesn't change package names:

- `internal/ubx/bin` → `internal/ubxbin`
- `internal/casic/bin` → `internal/casbin`

(`internal/asbin` is already flat.)

### Tag enum in gpsreg (done)

Added to `gpsreg`:

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
- `gpscmd` (response.go) - now uses `gpsreg.TagUBX`, `gpsreg.TagCASICBin`, `gpsreg.TagAllystarBin`, `gpsreg.TagNMEA`
- `gpscfg` - now uses `gpsreg.TagNMEA`, tests use `gpsreg.TagUBX`, `gpsreg.TagNMEA`, `gpsreg.TagRTCM`

### Split nmea: create nmeamsg package (done)

The `nmea` package mixes library-level code (like `Checksum`, `CheckSyntax`) with domain-level protocol implementation. Other protocols have this split (e.g., `ubx`/`ubxbin`, `casic`/`casbin`).

**Create new `nmeamsg` package** (in `gps/lib/nmeamsg/`) with library-level functions (done):
- `Checksum()` - computes NMEA checksum
- `CheckSyntax()` - validates NMEA sentence syntax
- `SyntaxFlags` type and constants

**Keep in `nmea`** (in `gps/internal/nmea/`) the domain layer code:
- Parser implementation
- Format detection
- Sentence type handling

This allows `internal/gpscmd/` to use `gps/lib/nmeamsg/` without needing access to `gps/internal/nmea/`.

### Factor daemon-specific code from cmd (done)

Move daemon-specific error handling from `cmd` to `daemon`:
- `ConfigError` type
- `ConfigErrorf()` function
- `ExitConfig` constant
- `ExitCode()` function (depends on `ConfigError`)

This leaves `cmd` with general CLI utilities (`ErrPrintln`, `CancelOnSignal`, `NewLogger`, `VersionInfo`, `UsageFunc`) that are appropriate for `gps/app/cmd`.

### Move TimePulsePVTMsgFlags to daemon (done)

Move `TimePulsePVTMsgFlags` from `gpsevent` to `daemon`.

This constant defines which PVT messages the daemon requires from GPS. It's used by `gpscmd` for `--pvt-out daemon`. After the reorganization, `gpsevent` will be in `time/internal/gpsevent/` (not importable by `internal/gpscmd/`), but `daemon` will be in `time/app/daemon/` (public, importable).

### Split ptime: create phctime package (done)

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
| `gps/gpsdecode/` | `internal/gpsdecode` |
| `gps/gpsprot/` | `internal/gpsprot` |
| `gps/gpsreg/` | `internal/gpsreg` |
| `gps/ptime/` | `internal/ptime` |
| `gps/scan/` | `internal/scan` |
| **gps/app/** | |
| `gps/app/gpscfg/` | `internal/gpscfg` |
| `gps/app/gpsio/` | `internal/gpsio` |
| `gps/app/cmd/` | `internal/cmd` |
| `gps/app/logfile/` | `internal/logfile` |
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
| `gps/lib/fieldenc/` | `internal/fieldenc` |
| `gps/lib/geopos/` | `internal/geopos` |
| `gps/lib/nmeamsg/` | `internal/nmeamsg` |
| `gps/lib/nmeamsg/testdata/` | `internal/nmeamsg/testdata` |
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
| **time/app/** | |
| `time/app/daemon/` | `internal/daemon` |
| `time/app/syncsimcmd/` | `internal/syncsimcmd` |
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
| **web/** | |
| `web/` | `web` |

## Directory to Layer Mapping

This section maps the new directory structure to the layers defined in `docs/internals.md`. After reorganization, the directory structure itself will document these distinctions, making the layer abstraction less necessary.

| Directory | Layer(s) | Description |
|-----------|----------|-------------|
| `cmd/` | Command | Program entry points (`main` packages) |
| `gps/` | Domain | GPS types, abstractions, protocol-independent interfaces. No goroutines, no logging. |
| `gps/app/` | Application | GPS orchestration and CLI infrastructure. Uses goroutines and/or logging. |
| `gps/internal/` | Domain | Protocol-specific implementations (private). No goroutines, no logging. |
| `gps/lib/` | Library | General-purpose reusable packages |
| `time/` | Domain | Time-sync types and abstractions. No goroutines, no logging. |
| `time/app/` | Command | Daemon orchestration and CLI. Uses goroutines and logging. |
| `time/internal/` | Application | Time-sync workers and event processing (private). Uses goroutines and logging. |
| `time/lib/` | Library | General-purpose reusable packages |
| `internal/` | Command | satpulsetool subcommand implementations |
| `web/` | Application | Web interface (embedded HTML/JS) |

Note: The `cmd` package (in `gps/app/cmd`) is reclassified from Command to Application layer. It provides infrastructure services (logger creation, signal handling) to Command layer packages rather than being a command itself.

**Layer rules by directory type:**
- **Base directories** (`gps/`, `time/`): Domain layer - no goroutines, no logging
- **`app/` directories**: May use goroutines and logging
- **`internal/` directories**: Private to parent. Each subtree uses `internal/` consistently for what it needs to hide: `gps/internal/` hides Domain (protocol implementations), `time/internal/` hides Application (workers, event handling).
- **`lib/` directories**: General-purpose libraries - no goroutines, no logging

## Layer Visibility

The structure uses Go's `internal/` visibility to enforce layer boundaries:

- **gps/** - GPS Domain API. No external dependencies. Can be split into a separate module.
- **gps/app/** - GPS orchestration (uses goroutines/logging). Importable by packages outside `gps/`.
- **gps/internal/** - Protocol-specific Domain implementations. Only importable by packages under `gps/`.
- **gps/lib/** - Reusable libraries. Importable by packages outside `gps/`.
- **time/** - Time-sync Domain API. Depends on `gps/`.
- **time/app/** - Time-sync orchestration, daemon. Importable by packages outside `time/`.
- **time/internal/** - Time-sync Application implementation. Only importable by packages under `time/`.
- **time/lib/** - Reusable libraries. Importable by packages outside `time/`.
- **internal/** - satpulsetool subcommands. Can import from `gps/`, `time/`, and their subdirs. Not exposed for external use.

## Dependency Rules

The dependency graph is strictly layered (no cycles):

```
internal/ → time/ → gps/
```

**Rules:**
1. `gps/` has no dependencies on `time/` or `internal/`
2. `time/` may depend on `gps/` but not `internal/`
3. `internal/` may depend on both `gps/` and `time/`

These rules ensure `gps/` can be extracted as a separate Go module without modification.

## Notes

- `syncsimcmd` is in `time/app/` rather than `internal/` because it simulates the daemon's time-sync behavior and needs access to `time/internal/` packages.

## Rejected Alternative: *cmd packages in gps/ and time/

An alternative structure would place `gpscmd` and `decodecmd` in `gps/`, and `sdpcmd`, `pmccmd`, `syncsimcmd` in `time/`.

**Why this was rejected:**

`satpulsetool gps --pvt-out daemon` configures the GPS to output the PVT messages that the daemon needs. This is a logical dependency of the gps tool on the daemon's requirements, expressed via the `TimePulsePVTMsgFlags` constant (moved to `daemon` as a prerequisite refactoring).

With `gpscmd` in `gps/` and `daemon` in `time/app/`, this creates a dependency from `gps/` to `time/`, breaking the goal that `gps/` has no external dependencies.

The chosen structure (all `*cmd` packages in `internal/`) solves this: `internal/gpscmd/` can import `time/daemon/` since the command layer can depend on both `gps/` and `time/`.

Note: Go's `internal/` visibility means `internal/` packages cannot import from `gps/internal/` or `time/internal/`. The prerequisite refactorings ensure `*cmd` packages only need public APIs from `gps/` and `time/`.
