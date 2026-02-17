---
name: hardware-test-gps-msgs
description: End-to-end test of GPS message (gpsprot.*Msg) pipeline with GPS hardware
allowed-tools: Read, Bash, Glob, Grep, Write, Edit, AskUserQuestion
---

# Hardware test GPS messages

End-to-end test of the GPS message pipeline (`gpsprot.*Msg`) against real GPS hardware. Runs satpulsed with event and packet logging to verify that binary packets are parsed, converted to gpsprot messages, and contain plausible data.

## Required information

1. **Serial device** - device path (e.g., `/dev/ttyUSB0`)
2. **Baud rate** - speed (e.g., `115200`)
3. **Messages to enable** - what to test, specified either as:
   - `--pvt-out` flags (for receivers with full config support), OR
   - message file path + tag(s) (for message-file-based config)

Check `CLAUDE.local.md` for serial device and baud rate. If not found there, ask the user.

## Setup

### Build

```bash
make
```

The build output shows the binary path (e.g., `out/amd64/satpulsetool`).

### Determine architecture

```bash
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
```

### Create directory structure

```bash
mkdir -p tmp/satpulsed/log
```

### Create satpulsed config

Extract the device base name from the serial path (e.g., `/dev/ttyUSB0` -> `ttyUSB0`).

Create `tmp/satpulsed/$DEVBASE.toml`:

```toml
[serial]
device = "/dev/ttyUSB0"
speed = 115200

[gps]
config = false

[log]
packet = true
event = true
dir = "tmp/satpulsed/log"
```

Replace `device` and `speed` with the actual values.

## Workflow

### 1. Enable messages on the receiver

Use `satpulsetool gps` to enable the binary messages you want to test.

**Option A: Full configuration support** (e.g., u-blox):
```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --pvt-out pos,vel,off
```

**Option B: Message file** (e.g., Unicore UM980):
```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t $TAG
```

To see available tags in a message file:
```bash
out/$ARCH/satpulsetool gps -m $MSGFILE --show-tags
```

For help on all options:
```bash
out/$ARCH/satpulsetool gps --help
```

### 2. Run satpulsed

Run with a timeout (default 10 seconds), redirecting stdout/stderr:

```bash
timeout 10 out/$ARCH/satpulsed -v -f tmp/satpulsed/$DEVBASE.toml > tmp/satpulsed/satpulsed.log 2>&1
```

Use `-v` for verbose output. The exit code from `timeout` will be 124 on normal timeout expiry.

### 3. Rename log files

Logs are appended on each run, so rename them to a unique name after each run to keep results separate:

```bash
STAMP=$(date +%Y%m%d-%H%M%S)
for f in tmp/satpulsed/log/*.jsonl; do
  [ -f "$f" ] && mv "$f" "${f%.jsonl}-${STAMP}.jsonl"
done
```

### 4. Inspect results

Check the satpulsed log for errors:
```bash
cat tmp/satpulsed/satpulsed.log
```

Log files are named `<kind>.<device-base>.jsonl`, e.g.:
- `tmp/satpulsed/log/event.ttyUSB0.jsonl` - parsed GPS events
- `tmp/satpulsed/log/packet.ttyUSB0.jsonl` - raw packets

After renaming they will have a timestamp suffix.

#### Decode packet log

Use `satpulsetool decode` to see decoded representations of each packet. Save stdout to a `.decoded.jsonl` file derived from the packet log filename:

```bash
out/$ARCH/satpulsetool decode --packet-log tmp/satpulsed/log/packet.$DEVBASE-$STAMP.jsonl \
  > tmp/satpulsed/log/packet.$DEVBASE-$STAMP.decoded.jsonl
```

This annotates each JSONL line with additional fields showing the binary packet decoded into Go structs. The exact fields added depend on the protocol. Use this to verify that packets are being parsed correctly and contain valid data.

#### Event log

Event and packet logs are JSONL (one JSON object per line). Inspect the event log for generated events.

Look for events like:
- `PosGeo` - geodetic position (lat/lon/height)
- `VelGeo` - geodetic velocity (ground speed, course)
- `PosECEF` - ECEF position (X/Y/Z)
- `VelECEF` - ECEF velocity (VX/VY/VZ)
- `NavEpoch` - navigation epoch boundary
- `RecTime` - receiver time
- `SatsInfo` - satellite information
- `LeapSecond` - leap second data

### 5. Disable messages

After testing, disable the messages you enabled:

**Option A:**
```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED --pvt-out off
```

**Option B:**
```bash
out/$ARCH/satpulsetool gps -d $DEV -s $SPEED -m $MSGFILE -t $OFFTAG
```

## Plausibility checks

After confirming events are generated, verify the data is plausible.

### ECEF position

Compare `posECEF.pos` values (in micrometers) against a known reference position. Sources for the reference position, in priority order:

1. ECEF position in `CLAUDE.local.md` (e.g., "Accurate ECEF position: X,Y,Z")
2. `fixedPosECEF` value in `/etc/satpulse.toml` or `/etc/satpulse.d/*.toml`
3. Ask the user

Convert micrometers to meters by dividing by 1e6. Single-point accuracy is typically within a few meters of the reference.

### Geodetic position

Compare `posGeo.latLon` values (in nanodegrees) against expected location. Convert by dividing by 1e9 to get degrees. If an ECEF reference is available but no geodetic reference, convert ECEF to approximate lat/lon for comparison.

`posGeo.heightMSL` (in micrometers) should be a reasonable altitude for the location.

### Time

Each log entry has a `t` field (system timestamp when the event was received). The `time` event payload has either `utcTime` or `taiTime` (or both). Compare the GPS-derived time against the system timestamp `t`:

- **`utcTime`** (ISO 8601 string): should agree with `t` within a few seconds.
- **`taiTime`** (decimal string like `"1771291691.000000000"`): seconds.nanoseconds since 1970-01-01 TAI. Convert as if it were a Unix timestamp; the resulting time will be ahead of UTC by the current UTC-TAI offset (currently 37 seconds). So `taiTime` interpreted as Unix time minus 37 seconds should agree with `t`.

A large discrepancy indicates a parsing or time conversion error.

### Velocity

For a stationary receiver, velocity should be close to zero:
- `velGeo.groundSpeed` (in micrometers/second) should typically be under 50000 (0.05 m/s)
- `velECEF.vel` components should each be close to zero

Non-zero velocity on a stationary receiver is normal noise, but values above ~1 m/s suggest a problem.

### NavEpoch

There should be approximately one `navEpoch` per second for 1Hz messages. The count should be close to (but may be one less than) the number of position/velocity events, since the epoch is flushed when the next epoch starts.

## Report

Report what events were seen in the log, including:
- Which event types appeared and how many of each
- Any errors or warnings in the satpulsed log
- Whether the expected events were generated
- Plausibility check results (position vs reference, velocity near zero, time reasonable)
- Sample event data if relevant
