# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Go code style

**CRITICAL: You MUST follow these rules for ALL Go code you write or modify in this repository. These rules override any default Go conventions you might know. Check each rule before generating code.**

### Consistency is paramount
- When modifying existing code, match the style of the surrounding code
- Consistency hierarchy: function > file > package > repo

### Variable/function naming
- Use short names for local variables with limited scope:
  - Loop indices: `i`, `j`, `k`
  - Common types: `s` (string), `b` (byte/buffer), `n` (number/count), `err` (error)
  - Short-lived intermediates: `v` (value), `ok` (bool from map/type assertion)
  - Abbreviate name of type
- Longer names are OK for:
  - Package-level variables, especially if exported
  - Struct fields
  - Variables with wider scope or complex meaning
- For exported function/variables, the name the user sees is the package name plus the function name

### Code density and readability
- Minimize blank lines - use them only to separate large logical sections
- Inline simple expressions instead of creating single-use variables
  - Good: `return strings.Join(processStrings(input), ",")`
  - Avoid: `processed := processStrings(input); joined := strings.Join(processed, ","); return joined`
- DO create variables to avoid repetition:
  - Bad: `process(config.Server.Host, config.Server.Port)`
  - Good: `srv := config.Server; process(srv.Host, srv.Port)`

### Comments
- Every exported function needs a comment starting with the function name
- NO comments inside functions unless explaining non-obvious behavior
- Comments should explain WHY, not WHAT

### Character encoding
- Use ASCII only - avoid non-ASCII characters (no fancy quotes, checkmarks, emojis, etc.)
- Exception: math symbols where truly needed (e.g., μs for microseconds)

### Function ordering
- Order code for top-to-bottom readability: readers should understand what's happening without jumping around
- Type definitions come before the functions that use them
- Main/exported functions come before their helper functions
- Example ordering:
  1. Type definitions (structs, interfaces, constants)
  2. Constructor/factory functions
  3. Main methods on those types
  4. Helper functions used by the methods
- The goal: reading from top to bottom tells a story - what the types are, what the main operations are, then how they're implemented

## Development Commands

**CRITICAL: Always use `make` to build. NEVER use `go build` directly - it creates binaries in the wrong location and clutters the repository.**

Build system uses GNU Make:
- `make` - Build for current architecture
- `make test` - Run all tests (`go test -v ./...`)
- `make install` - Install binaries and configs to `/usr/local/`
- `make pkg` - Build both deb and rpm packages
- `make clean` - Remove build artifacts

It builds on Linux only. On macOS, use `bsd-build.sh` instead.

Web interface is build using npm in `web/` directory.
- `npm run build` - Rebuilds .js and .css files that are embedded.

Testing:
- Individual package: `go test -v ./internal/packagename`
- All tests: `make test`
- Test files follow `*_test.go` convention

System testing on real hardware is doing using ansible in `systest/` directory.

## Local environment

Host abondance.lan is Debian Linux and can run without Docker.

It has a u-blox ZED-F9P connected to /dev/ttyACM0.
The PPS out is connected to pin 1 on the PHC clock of network interface enp4s0.

## Project Architecture

SatPulse has a layered Go architecture:

### Main Binaries
- `cmd/satpulsed/` - Main daemon that orchestrates GPS/PTP synchronization
- `cmd/satpulsetool/` - CLI tool with `gps` and `pmc` subcommands

### Core Data Flow
1. `internal/gpsio` - Reads GPS packets from serial/network
2. `internal/gpscfg` - Runs the GPS configuration phase
3. `internal/gpsevent` - Main event loop processing GPS packets and timestamps 
4. `internal/combine` - Combines GPS time messages with PPS timestamps
5. `internal/mon` - Removes outliers, monitors sync status
6. `internal/servo` - PI controller adjusting PHC frequency

### GPS Protocol Support
- `internal/ubx/` - U-blox UBX protocol (primary)
- `internal/nmea/` - NMEA protocol 
- `internal/rtcm/` - RTCM protocol
- `internal/gpsprot/` - Protocol-agnostic abstractions
- `internal/gpsreg/` - Protocol registry

### Time and Hardware
- `internal/ptime/` - PTP timescale representation (TAI nanoseconds since 1970)
- `internal/phc/` - Linux PTP hardware clock interface
- `internal/ts/` - External timestamp capture from PHC

### PTP Integration
- `internal/pmc/` - PTP management client
- `internal/sockrefclock/` - Chrony refclock protocol

### Configuration
- Main config: `configs/satpulse.toml`
- Schema: `configs/config-schema.json` 
- `internal/daemon/config.go` handles TOML parsing

### Web Interface
- `web/` - TypeScript/Preact dashboard (transpiled to JavaScript)
- Embedded in Go binary via `web/embed.go`

## Key Technical Notes

- Supports arm64 and amd64 architectures
- Separate build system for

## Git Usage

- Never use `git add -A` or `git add .` - these add untracked files which may include test data or local files
- Use `git add -u` to stage modified/deleted tracked files, then add new files explicitly by name

## Development Environment

System testing uses Ansible playbooks in `systest/`.