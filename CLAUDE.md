# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

For detailed documentation of the package structure, dependencies, and layering, see @docs/internals.md.

Whenever you create a new package, add an entry describing it to the appropriate section of @docs/internals.md.

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

## Development commands

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

Black-box smoke tests of the real `satpulsed` binary live in `smoketest/`
(daemon-level config wiring, endpoints, logging, Ntrip, shutdown; no root or GPS
hardware). Build first with `make`, then run `make smoketest`. See
@smoketest/CLAUDE.md.

System testing on real hardware is doing using ansible in `systest/` directory.

## Git usage

- Never use `git add -A` or `git add .` - these add untracked files which may include test data or local files
- Use `git add -u` to stage modified/deleted tracked files, then add new files explicitly by name

## Development environment

System testing uses Ansible playbooks in `systest/`.

## Documentation style

- Headings use sentence case (capitalise only the first word and proper nouns)

## Release notes

- Implementing a user-facing feature MUST include an entry in `docs/_includes/NEWS.md`, in the same change as the implementation.
- This applies to new features, behaviour changes, and upgrade notes. Bug fixes are excluded.
- Add the entry under the current unreleased version heading, in the appropriate section, and reference the issue number(s) in parentheses to match the existing entries.

## Connected GPS

You can look at `/etc/satpulse.toml` if it exists to find device and speed of a connected GPS receiver.
But before using it, check that `satpulsed` is not running `ps ax | grep satpulsed`.