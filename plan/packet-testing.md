# Packet log testing

Collect packet logs from real GPS receivers and use them to test scanner and decoding behavior deterministically.

This covers incoming packet traffic only. It does not cover configuration/probing traffic, `gpscmd` configuration replay, PHC synchronization, or `phcsync` timing behavior.

## Goals

- Build a collection of packet logs from different GPS receivers. Use the `/packet-testdata` skill to plan and execute captures for each receiver model.
- Test that `gps/scan.Scanner` correctly reconstructs packets regardless of how the byte stream is chunked.
- Test packet decoding and `gpsprot.Msg` generation.
- Generate expected decoded GPS message output using the `gpsprot.Msg` JSON
  shape from [gpsprot-json.md](./gpsprot-json.md).
- Provide test data that can later be used by a port in another language.

## Layout

```text
gps/testdata/packets/<vendor>/<model>/
  HW.toml
  factory.jsonl
  factory-115200.jsonl
  nav-pvt_nav-sat.jsonl
```

Vendor directory names are lowercased `gpsreg` vendor names (e.g. `u-blox`, `unicore`, `novatel`). Model directory names match the receiver model. If multiple receivers of the same model are needed (e.g. different firmware), add an ad-hoc suffix to the directory name.

### HW.toml

Each model directory has a `HW.toml` describing the hardware:

```toml
vendor = "u-blox"
model = "ZED-F9P"
firmware = "1.32"
default-baud = 38400
```

### Packet log files

Each `.jsonl` file is a packet log in `gpsio.PacketLogEntry` format containing only incoming packets. The filename indicates what messages are enabled:

- `factory.jsonl` -- messages from a factory-reset receiver at the default baud rate.
- `nav-pvt_nav-sat.jsonl` -- specific messages enabled, underscore-separated.
- `factory-115200.jsonl` -- `-NNNN` suffix indicates a non-default baud rate.

Captures should be long enough for `phcsync` reset mode to lock (which needs several seconds of aligned pulses and time messages), so around 30 seconds is a reasonable minimum. Captures should be taken on a machine with stable NTP/chrony synchronization and no clock step during the capture, since packet read timestamps are used to generate event timestamps.

## Scanner testing

Test `gps/scan.Scanner` independently of decoding by concatenating packet payloads from a log file into a raw byte stream, feeding it through the scanner under various chunk sizes, and checking that the recovered packets match the logged packets (ignoring read timestamps).

Chunk sizes should include: whole stream, one byte at a time, small fixed sizes, and random sizes with a fixed seed.

## Decode and event testing

Test packet decoding separately from scanner chunking: feed logged packets (with their recorded timestamps) through packet processors, emit `event` output, and compare against expected output.

Once the `gpsprot.Msg` JSON shape from [gpsprot-json.md](./gpsprot-json.md)
exists, generate an `event.jsonl` alongside each packet log containing the
expected decoded GPS message output. The packet log is the primary artifact;
the event file is derived from it and should only change when decoding or
event semantics intentionally change.

## `satpulsetool replay`

Add a `replay` subcommand to `satpulsetool` that reads a packet log, runs the full packet processing pipeline (`gpsreg.CreatePacketProcessors`, `NavEpochManager`), and emits JSONL on stdout.

This is the offline equivalent of `satpulsetool gps` (which works with a live receiver). Where `satpulsetool decode` inspects individual packet structure, `replay` runs the full pipeline and produces semantic output.

Input is a packet log file (or `-` for stdin). The vendor is specified with a `--vendor` flag.

The output uses the existing `gpsevent.LogEvent` format: each line has `t` (from the recorded packet read time), `nanos` (derived from wallclock deltas relative to the first packet), and one message field (`time`, `posGeo`, `navEpoch`, etc.). `pulseEdge` is not emitted since pulse edges come from the PHC, not from packets. Output is deterministic -- no dependency on current wallclock, goroutine scheduling, or iteration order.

When the `gpsprot.Msg` JSON cleanup from
[gpsprot-json.md](./gpsprot-json.md) lands, generated expected output should
use the cleaned `gpsprot.Msg` payload JSON.

Uses:

- Generate and update expected `event.jsonl` files for the packet log collection.
- Inspect decoded output: `satpulsetool replay factory.jsonl | jq ...`
- Feed a replay dev server for frontend development without GPS hardware.
- Agent-driven validation of decoding changes.

## Verify

- Packet logs load successfully and contain only incoming packets.
- Scanner reconstructs packets correctly under all chunk sizes.
- Event output from each packet log is deterministic.
- The same test data can be used as an acceptance suite for a future port.
