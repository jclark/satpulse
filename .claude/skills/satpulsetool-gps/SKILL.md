---
name: satpulsetool-gps
description: How to use `satpulsetool gps` command
allowed-tools: Read, Bash, Glob, Grep
---

# Running satpulsetool gps

Quick reference for running `satpulsetool gps`.

## Finding the Binary

Determine the architecture once per session, then use the resolved path directly:

```bash
# Run once to determine architecture
uname -m   # Returns x86_64 or aarch64
```

Then use the correct binary path directly in subsequent commands:
- For x86_64: `out/amd64/satpulsetool`
- For aarch64: `out/arm64/satpulsetool`

**DO NOT** use `$(uname -m | sed ...)` in every command. Determine the path once and reuse it.

Build first with `make` if needed.

## Required Options

Always specify:
- `-d /dev/ttyXXX` - Serial device (ask user, don't guess)
- `-s SPEED` - Baud rate (e.g., `115200`)

## Message File Options

Send messages from a TOML file:
- `-m FILE` - Message file path (e.g., `configs/gpsmsg/allystar.toml`)
- `-t TAG` - Tag(s) to send (e.g., `-t pps` or `-t get-version,get-pps`)
- `--show-tags` - List all tags in the message file (use with `-m`)

Example:
```bash
out/amd64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/allystar.toml -t get-version
```

List available tags:
```bash
out/amd64/satpulsetool gps -m configs/gpsmsg/allystar.toml --show-tags
```

## Packet Logging

- `--packet-log FILE` - Log packets to JSONL file (appends by default, use new filename each time)
- `--capture N` - Capture for N seconds after config (0 = forever until Ctrl+C)

**IMPORTANT:** Use `--capture N` to run for a fixed duration. Do NOT use the `timeout` command.

Example:
```bash
out/amd64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 --packet-log capture.jsonl --capture 10
```

## Verbose/Debug Mode

**CRITICAL: `-v` is a global flag on `satpulsetool` itself, NOT on the `gps` subcommand. It MUST go BEFORE `gps`.**

- `-v` - Verbose output
- `-v -v` - Debug output (more detail)

```bash
out/amd64/satpulsetool -v -v gps -d /dev/ttyUSB0 -s 115200
```

WRONG (will fail): `satpulsetool gps -v -d ...`
RIGHT: `satpulsetool -v gps -d ...`

## Other Options

Run with `--help` for all command-line options:
```bash
out/amd64/satpulsetool gps --help
```

See the man page for full documentation: `docs/man/satpulsetool-gps.1.md`
