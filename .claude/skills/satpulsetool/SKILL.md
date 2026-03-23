---
name: satpulsetool
description: How to use satpulsetool - GPS receiver configuration and packet decoding tool
allowed-tools: Read, Bash, Glob, Grep
---

# satpulsetool cookbook

`satpulsetool` is a command-line tool with subcommands. Get help for any subcommand with `--help`:

```
out/amd64/satpulsetool gps --help
out/amd64/satpulsetool decode --help
```

Global options go BEFORE the subcommand. The useful one is `-v` for verbose output (repeat for more):

```
out/amd64/satpulsetool -v gps ...
out/amd64/satpulsetool -v -v decode ...
```

WRONG: `satpulsetool gps -v ...` (fails - `-v` is not a gps option)

## Finding the binary

Build with `make`. The binary location depends on platform:
- Linux x86_64: `out/amd64/satpulsetool`
- Linux aarch64: `out/arm64/satpulsetool`
- macOS arm64: `out/darwin_arm64/satpulsetool`

Run `uname -m` once to determine which.

## Serial device and speed

The `gps` subcommand always needs `-d` (serial device) and `-s` (baud rate), or `-f` (config file containing both).

Look in `CLAUDE.local.md` for the device and speed of any connected receiver. If not documented there, ask the user and update `CLAUDE.local.md` with what they tell you.

Before using a serial device, check that `satpulsed` is not running (`ps ax | grep satpulsed`) since they cannot share the device.

## gps subcommand

Configures GPS receivers and captures packets. Full reference: `docs/man/satpulsetool-gps.1.md`. Four main uses:

### 1. Packet capture

Capture packets from a receiver to a JSONL file. This is passive - it just records what the receiver sends.

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --packet-log capture.jsonl --capture 10
```

Do NOT use the `timeout` command - use `--capture N` (seconds) instead. Note that `--packet-log` appends to an existing file, so use a new filename each time.

### 2. Low-level configuration with message files

Message files are TOML files in `configs/gpsmsg/` that define named messages for specific receivers. Messages are selected by tag. See `configs/gpsmsg/tags.md` for tag naming conventions. When sending messages, satpulsetool correlates receiver responses with sent commands and reports whether each message was accepted (OK) or rejected (NAK).

List available tags in a message file:

```
out/amd64/satpulsetool gps -m configs/gpsmsg/um980.toml --show-tags
```

Send tagged messages to a receiver:

```
out/arm64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/um980.toml -t pps
```

Send multiple tags in order:

```
out/arm64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/um980.toml -t get-version,get-pps
```

Combine with `--packet-log` to see the full exchange:

```
out/arm64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/um980.toml -t pps --packet-log result.jsonl
```

When querying with a `get-*` tag, binary replies are printed as hex. Use `satpulsetool decode --bin` to decode them (see decode subcommand below).

### 3. Sending arbitrary commands

Use `-m -` to pipe the message file as a here document. No temporary file needed. The TOML format handles checksums and binary packing for you. See `configs/gpsmsg/format.md` for the full format reference.

Send a line command (e.g. Unicore):

```
out/amd64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m - <<'TOML'
[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
TOML
```

Send an NMEA command (checksum computed automatically):

```
out/amd64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m - <<'TOML'
[[nmea]]
text = "PQTMCFGPPS,W,1,1,100,2,1,0"
TOML
```

Send a binary command (payload packed automatically). Supported binary formats: `[[ubx]]` (u-blox), `[[asbin]]` (Allystar), `[[casbin]]` (CASIC), `[[sdbp]]` (Techtotop/Taidou). You specify the message class/id and the payload as type specifiers (`payload.types`) and values (`payload.values`). The type specifiers (`U1`, `U2`, `U4` unsigned; `I1`, `I2`, `I4` signed; `R4`, `R8` float) can typically be read directly from the protocol specification for the message. Satpulsetool handles sync bytes, length fields, and checksums.

```
out/amd64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m - <<'TOML'
[[ubx]]
class = 0x06
id = 0x03
payload.types = "U4U4U1I1U1U1R4"
payload.values = [1000000, 100000, 3, 0, 1, 0, 0.0]
TOML
```

Combine with `--packet-log` to see the response.

### 4. High-level configuration

Device-independent options that satpulsetool translates into the right commands for the receiver. Currently supported on u-blox (6 through X20) and Unicore Nebulas IV (UM980/981/982/960).

Show receiver info (also the default if no config options given):

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --show-receiver
```

Show current configuration:

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --show-config
```

Enable constellations and bands:

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 -g GPS,GAL -b L1,L2
```

Configure PPS (pulse width in seconds, 0 disables):

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --pps 0.5
```

Start a position survey (for timing mode):

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --survey --survey-time 3000 --survey-acc 1.5
```

Use a known fixed position:

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --fixed-pos-ecef -941709.7,5965766.5,1553280.3 --fixed-pos-acc 0.1
```

Enable binary output for satpulsed:

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --binary --pvt-out daemon
```

Configure RTCM output:

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --rtcm-out MSM4,ARP -g GPS,GAL,BDS
```

Save configuration to receiver's non-volatile memory:

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --save
```

Combine with `--packet-log` to see what commands are sent and how the receiver responds:

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --show-config --packet-log config.jsonl --capture 5
```

Reset, reload, factory reset:

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --reload
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --reset
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --factory-reset
```

**Important note**

`--save`, `--factory-reset`, and `--reset` modify persistent state - confirm with user before using.

## decode subcommand

Decodes GPS packets into JSON. Three input modes:

Decode a hex-encoded binary packet:

```
out/amd64/satpulsetool decode --bin b562010614000000...
```

Decode an ASCII packet (adds \r\n automatically):

```
out/amd64/satpulsetool decode --line '$GNGGA,034418.00,1343.91295,N,...*64'
```

Annotate a JSONL packet log (as produced by `gps --packet-log`):

```
out/amd64/satpulsetool decode --packet-log capture.jsonl
out/amd64/satpulsetool decode --packet-log - < capture.jsonl
```

This adds `payload`, `header`, and `cfgData` fields to each log entry that can be decoded.

Options:
- `-c` / `--compact` - single-line JSON output
- `--out` - treat packet as outgoing (affects CFG-VAL* decoding direction)

Without any flags, reads one packet from stdin.

### Typical workflow: capture then decode

```
out/amd64/satpulsetool gps -d /dev/ttyACM0 -s 38400 --packet-log cap.jsonl --capture 10
out/amd64/satpulsetool decode --packet-log cap.jsonl > decoded.jsonl
```

