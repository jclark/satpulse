---
name: packet-testdata
description: Build systematic collection of packet logs for a GPS receiver, covering all implemented decode paths per plan/packet-testing.md
allowed-tools: Read, Bash, Glob, Grep, Write, Edit, AskUserQuestion, Agent, Skill
---

# Packet test data collection skill

Build a systematic collection of packet logs for a specific GPS receiver model, as described in `plan/packet-testing.md`. The goal is to produce a set of traces in `gps/testdata/packets/` that exercises every implemented decode path in the receiver's lib and domain layers, and includes time messages suitable for deterministic replay testing per `plan/timemsg-sync-testing.md`.

This is NOT for ad-hoc packet capture during development. It is for building the curated test data collection.

## Configuration approaches

There are two ways to configure GPS receivers for captures, corresponding to the two configuration modes of `satpulsetool gps` (see `docs/man/satpulsetool-gps.1.md`):

- **High-level configuration** uses device-independent flags like `--pvt-out`, `--sats-out`, `--binary`, `--nmea-out`, `--time-gnss`, `--gnss`, `--survey`, `--reload`. Currently supported on u-blox receivers (u-blox 6 through X20) and Unicore Nebulas IV (UM980, UM981, UM982, UM960). See `highlevel-config.md`.

- **Low-level configuration** uses a TOML message file (`-m`) containing protocol-specific commands. This works with all receivers. Message files are in `configs/gpsmsg/` (e.g., `ubx.toml`, `um980.toml`, `allystar.toml`). See `lowlevel-config.md`.

Both approaches can be combined: use high-level config first, then a second invocation with `-m` to add messages that high-level config cannot reach.

## Required information

1. **Serial device** - device path (e.g., `/dev/ttyACM0`)
2. **Baud rate** - saved baud rate in NVM
3. **Vendor** - GPS vendor (e.g., `u-blox`, `unicore`)

Check `CLAUDE.local.md` for device and baud rate. If not found, ask the user.

Use the `satpulsetool` skill for reference on satpulsetool command usage.

## Prerequisites

1. **NTP sync**: The capture host must have good time sync. Verify with `chronyc sources` -- there should be a selected source (marked with `*`) and the offset should be small (under a few ms). Packet timestamps depend on wall clock accuracy.

2. **Stop satpulsed**: The user must stop satpulsed if it is running on the device. This requires sudo. Ask the user to do this -- do not attempt it yourself.

```
sudo systemctl stop satpulsed
```

## Output directory

```
gps/testdata/packets/<vendor>/<model>/
```

The model name comes from `--show-receiver` output (see high-level config section).

### HW.toml

Create a metadata file in the output directory:

```toml
vendor = "<vendor>"
model = "<model>"
firmware = "<firmware string from --show-receiver>"
default-baud = <baud>
```

## General principles

Read `capture-principles.md` for device-independent capture design principles.

## Determining what to capture

Read `capture-analysis.md` for how to analyze the lib and domain layers to determine what messages need capturing.

## Receiver-specific configuration

For receivers with high-level configuration support (u-blox, Unicore), read `highlevel-config.md`.

For u-blox receivers specifically, also read `ubx-config.md`.

For all receivers, message files in `configs/gpsmsg/` provide low-level message tags. Read `lowlevel-config.md` for how to use these.

## Capture procedure

For each capture:

1. **Reload** (high-level config receivers only): `satpulsetool gps -d <device> -s <baud> --reload` to restore NVM state. This ensures no residual configuration from a prior capture leaks in. On USB, add `sleep 2` after reload as the device may briefly disconnect. For receivers without high-level config, reset by power cycling or using a reset command from the message file.

2. **Configure** (if needed): Run satpulsetool with configuration flags but without `--capture` or `--packet-log`.

3. **Add low-level messages** (if needed): Run satpulsetool with `-m <file> -t <tags>` to enable messages not reachable via high-level config. This can follow a high-level config step since `-m` does not probe or reset the receiver.

4. **Capture**: Run satpulsetool with `--packet-log <file> --capture 30` (60 for survey). Use 30 seconds for most captures.

5. **Verify**: Check message types in the capture with:
   ```
   jq -r 'select(.out != true) | .msg' <file> | sort | uniq -c | sort -rn
   ```
   Confirm steady-state messages match expectations. First-epoch residual messages from before configuration took effect are normal and expected.

## After all captures

1. Reload the receiver: `satpulsetool gps -d <device> -s <baud> --reload`
2. Ask the user to restart satpulsed.

## Replay verification

After collecting all captures, replay them with `out/amd64/satpulsetool replay <file>` (build first with `make`) and examine the event output for anomalies:

- Compare time messages across captures -- TAI times should be plausible and consistent.
- Check that gnss fields are correctly populated for all constellations.
- Verify that taiTime values from different time message variants within the same epoch agree to within a few nanoseconds.
- For per-constellation captures, check that TimTP gnss reflects the configured constellation and that TAI values are not offset by unexpected amounts.
