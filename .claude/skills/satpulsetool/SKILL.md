---
name: satpulsetool
description: Work with GPS/GNSS receivers (modules), serial ports, and PPS signals using satpulsetool - find what is plugged into which port, detect a receiver's speed, capture and decode what it sends, check PPS on a serial modem-control pin or a PHC SDP, and query or change receiver configuration. Load when a task involves a GNSS receiver, or a serial port or PPS signal that may be connected to one.
allowed-tools: Read, Bash, Glob, Grep
---

# satpulsetool

`satpulsetool` is a command-line tool with subcommands. This file has the rules
that apply to every subcommand and a table saying which file to read for each
kind of task. Read only the file for the task at hand.

## Which file to read

| Task | Read |
|------|------|
| Which serial ports exist, what is plugged into each, what speed a receiver runs at, what it is sending, whether PPS arrives on a modem-control pin (CTS, DCD, DSR, RI) | `serial.md` |
| Whether PPS arrives on a PHC software-defined pin (SDP), or generating a pulse on one | `sdp.md` |
| Show receiver info, query or change receiver configuration, send a command to a receiver | `gps.md` (which routes to `gps-highlevel.md`, `gps-msgfile.md`, or `gps-adhoc.md`) |
| Decode one packet, or annotate a packet log with decoded fields | `decode.md` |

Other subcommands (`pack`, `scan`, `replay`, `convobs`, `syncsim`, `ubxsim`,
`ntrip`, `pmc`) do not touch hardware and are not covered here; see
satpulsetool(1) and their `--help`.

## Rules for every subcommand

Get help for any subcommand with `--help`:

```
satpulsetool gps --help
```

Global options go BEFORE the subcommand. The useful one is `-v` for verbose
output (repeat for more):

```
satpulsetool -v gps ...
```

WRONG: `satpulsetool gps -v ...` (fails: `-v` is not a `gps` option).

The daemon `satpulsed` cannot share a serial port with `satpulsetool`. Before
using a port, check that no `satpulsed` is using it (`ps ax | grep satpulsed`
shows each daemon's `--serial-device`); if one is, stop it first, or ask. On a
packaged install the daemon is a systemd instance per device, named after the
device without `/dev/`, and restarts if merely killed:

```
sudo systemctl stop satpulse@ttyUSB0
sudo systemctl start satpulse@ttyUSB0
```

The `gps` and `serial` subcommands take `-d` (serial device) and `-s` (speed in
bits per second). If the device or speed is not known, `satpulsetool serial`
finds them (see `serial.md`); `gps` can instead read both from a satpulse
configuration file with `-f` (on a packaged install, `/etc/satpulse.toml` or
the per-device `/etc/satpulse.d/<device>.toml`).

If opening a serial device fails with permission denied, the fix is to make
the user a member of the group that owns the device (`ls -l DEV` shows it;
`dialout` on Debian-derived systems), not to run satpulsetool as root.

Do NOT wrap satpulsetool in the `timeout` command. Every subcommand that runs
for a while has `-t N` (seconds) instead; always give it when capturing.

`--packet-log FILE` overwrites an existing file, so use a new filename each
time.

Exit status 2 from `serial` and `sdp` means no data found (no ports, no
packets, no edges, no timestamps), not an error.

## Confirm with the user first

These change persistent or physical state. Do not run them without the user
agreeing to that specific action:

- `gps --save`, `--save-all`: write receiver non-volatile memory
- `gps --reset`, `--factory-reset`: reset the receiver; `--reload` discards unsaved configuration
- `sdp --extts`, `--perout`, `--disable`: change the function of a PHC pin

## Reference

Man pages: satpulsetool(1), satpulsetool-serial(1), satpulsetool-gps(1),
satpulsetool-sdp(1). Message files live in `/usr/share/satpulse/gpsmsg`.
