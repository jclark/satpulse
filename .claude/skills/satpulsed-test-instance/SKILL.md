---
name: satpulsed-test-instance
description: Stand up a throwaway satpulsed instance as an unprivileged user for testing, fed from a serial device or a FIFO. Use directly to run a test instance, or as the setup step for the drive-satpulsed-from-log and hardware-test-gps-msgs skills.
user-invocable: true
allowed-tools: Read, Bash, Glob, Grep, Write, Edit, AskUserQuestion
---

# Run a test satpulsed instance (unprivileged)

Run `satpulsed` as a normal user to exercise its pipeline (scan, decode, events, web interface) without root and without touching a production instance. The input source is set by the caller: a real serial device, or a FIFO fed by a replayed log.

This skill is the common setup. The `drive-satpulsed-from-log` and `hardware-test-gps-msgs` skills build on it.

## Build

```bash
make
```

Determine the architecture for the binary path:

```bash
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
```

Binaries are then at `out/$ARCH/satpulsed` and `out/$ARCH/satpulsetool`.

## Working directory

```bash
mkdir -p tmp/satpulsed/log
```

## Config

Create `tmp/satpulsed/<name>.toml`. The minimal shape is:

```toml
[serial]
device = "..."   # set by the caller: a serial device path or a FIFO

[gps]
config = false   # do not reconfigure the receiver
# vendor = "u-blox"   # optional; restricts which packet formats are recognized

[log]
dir = "tmp/satpulsed/log"
event = true     # optional; enable to inspect decoded events

[[http]]
listen = ":2001"   # optional; web interface on a non-2000 port
```

Rules, and why:

- **No `[phc]` table.** Without a PHC, `satpulsed` runs without root, and a startup "GPS detection failed: no output detected" is only a `WARN` rather than fatal. With a `[phc]` table the same failure is fatal: the daemon exits if it does not see GPS data within the detection window (about 2s, up to 10s), which it usually will not before the input is flowing.
- **`[log] dir` must be under your tmp directory.** The default is `/var/log/satpulse`, which is root-owned; an unprivileged daemon cannot write there.
- **`[[http]] listen` must not be `:2000`** if a system `satpulsed` may be using that port. Use `:2001` (or another free port).
- The `[log]` and `[[http]]` blocks are both optional. Include only what the test needs.

## Run

For interactive or web-interface testing, run in the background, redirecting output:

```bash
out/$ARCH/satpulsed -v -f tmp/satpulsed/<name>.toml > tmp/satpulsed/satpulsed.log 2>&1 &
```

For a fixed observation window, run under `timeout` (exit code 124 on normal expiry):

```bash
timeout 10 out/$ARCH/satpulsed -v -f tmp/satpulsed/<name>.toml > tmp/satpulsed/satpulsed.log 2>&1
```

In the startup log, expect `running without a PTP hardware clock` (no `[phc]`), and a `GPS detection failed: no output detected` `WARN` until input starts flowing. Neither is an error.

## Inspect

Log files are written to `tmp/satpulsed/log/` as `<kind>.<device-base>.jsonl`, where `<device-base>` is the basename of the `[serial] device`. For example a FIFO `/tmp/fifo0` produces `event.fifo0.jsonl`.

Decode a packet log to see each packet as Go structs:

```bash
out/$ARCH/satpulsetool annotate tmp/satpulsed/log/packet.<base>.jsonl \
  > tmp/satpulsed/log/packet.<base>.decoded.jsonl
```

## Teardown

Stop the daemon with `SIGTERM` (or `SIGINT`). If its scan worker is blocked reading an idle input (a FIFO with no data flowing), the daemon can be slow to respond; escalate to `SIGKILL`:

```bash
kill -TERM <pid>; sleep 1; kill -KILL <pid> 2>/dev/null
```

Confirm the HTTP port is released afterwards (`ss -ltn | grep :2001`).
