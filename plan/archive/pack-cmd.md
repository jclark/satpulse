# satpulsetool pack command (#247)

Add a `satpulsetool pack` subcommand that converts JSONL packet logs back into
a packet byte stream corresponding to the original packet contents.

The command has two primary uses:

- Generate packet byte streams from packet logs for tools such as RTKLIB
  `convbin`.
- Replay packet logs through stdout with original packet pacing so `satpulsed`
  can be tested end to end through a FIFO-backed serial device, without GPS
  hardware.

Related: #246 (`satpulsetool scan`), which converts packet byte streams into
packet logs.

## CLI

Initial usage:

```text
satpulsetool pack [-h|--help] [--tag TAG] [--msg MSG] [--timing] file|-
```

Examples:

```sh
satpulsetool pack packets.jsonl > packets.bin
satpulsetool pack --tag UBX packets.jsonl > ubx.bin
satpulsetool pack --tag UBX --msg NAV-PVT packets.jsonl > nav-pvt.ubx
satpulsetool pack --tag RTCM --msg 1077 packets.jsonl > 1077.rtcm3
satpulsetool pack --timing packets.jsonl > /tmp/satpulse-gps.fifo
```

The option names intentionally mirror the packet log JSON field names:

- `--tag` matches `PacketLogEntry.tag`.
- `--msg` matches `PacketLogEntry.msg`.

Support one `--tag` value and one optional `--msg` value. `--msg` requires
`--tag`, because message IDs are meaningful only within a packet protocol/tag.

The file argument is required. A file argument of `-` means stdin.

## Packet Selection

Read JSONL line by line and unmarshal each line as `gpsio.PacketLogEntry`.

Skip records with neither `bin` nor `ascii`; this allows environment or other
metadata records in mixed logs.

Skip `out:true` entries by default. The current uses both need the receiver
output stream:

- RTKLIB should receive receiver-emitted GNSS protocol data.
- FIFO replay into `satpulsed` should look like a read-only preconfigured GPS
  receiver stream.

An `--include-out` option can be added later if a future use case needs to
preserve bidirectional traffic. It is not part of the initial command.

Filter matching is case-insensitive:

```sh
satpulsetool pack --tag ubx --msg nav-pvt packets.jsonl
```

matches:

```json
{"tag":"UBX","msg":"NAV-PVT","bin":"..."}
```

Use exact string equality modulo case, such as `strings.EqualFold`; do not add
protocol-specific aliases or normalization in the initial implementation.

No match is not an error and should not emit a warning. Empty output is the
normal result of a filter that matches nothing.

Invalid JSON or invalid `bin` hex should be a command error with line context.

## Byte Output

For each selected packet:

- If `bin` is present, write the decoded bytes.
- Otherwise write `ascii` bytes exactly as stored.

Do not re-encode, re-checksum, rescan, or reinterpret packet contents. `pack`
is an extraction command, not a serializer.

Write the packet byte stream to stdout. Diagnostics and usage errors go to stderr through
the existing `satpulsetool` command framework.

## Timing

`--timing` preserves inter-packet timing for emitted packets after filtering.

The packet log timestamp `t` records when the first byte of the packet was read
by `gps/scan.Scanner`. For replay, schedule packet write starts so the deltas
between write starts match the deltas between logged packet timestamps.

For each emitted packet:

```go
if havePrev {
    targetWriteStart := prevWriteStart.Add(entryT.Sub(prevEntryT))
    sleepUntil(targetWriteStart)
}

writeStart := now() // immediately before writing packet bytes
write(packet)
flush()

prevEntryT = entryT
prevWriteStart = writeStart
```

Rules:

- The first emitted packet writes immediately.
- Sleep deltas are based on the previous emitted packet, not the previous input
  line. This preserves gaps introduced by filtering out other packets.
- Zero or negative timestamp deltas write immediately.
- If the process is late because of scheduler delay or a blocking write, do not
  try to catch up by compressing later intervals beyond sleeping zero when the
  target time is already in the past.
- Flush stdout after every emitted packet when `--timing` is enabled so FIFO
  consumers see packet cadence rather than buffered bursts.
- If `--timing` is enabled and a selected packet has a zero/missing timestamp,
  return an error with line context.

## FIFO Replay For satpulsed

The motivating end-to-end test replays a recorded packet log into `satpulsed`
through a FIFO, with no GPS hardware attached. The daemon config names the FIFO
as its serial device:

```toml
[serial]
device = "/tmp/fifo0"

[log]
dir = "/tmp/satpulse-log"
event = true

[[http]]
listen = ":2001"
```

```sh
mkfifo /tmp/fifo0
satpulsed -f satpulse.toml &
satpulsetool pack --timing /var/log/satpulse/packet.ttyACM0.jsonl > /tmp/fifo0
```

The config must have no `[phc]` table. Without a PHC clock, a `gpscfg` detection
failure at startup ("GPS detection failed: no output detected") is only a
warning (`daemon.go` takes the `clk == nil && errors.Is(err,
gpscfg.ErrNotDetected)` branch). With a `[phc]` table the same failure is fatal
and the daemon exits before the replay warms up, because detection has a short
timeout (`listenTimeout` 2s, up to `listenMaxTimeout` 10s).

`[gps] config = false` is not required: a FIFO is not writable, so
`gpscfg.Configure` skips probing/configuration regardless. Setting it (with
`vendor`) just makes the intent explicit and restricts which packet formats the
scanner recognizes.

The FIFO basename becomes the log basename, so `/tmp/fifo0` produces
`/tmp/satpulse-log/event.fifo0.jsonl`; choose a sensible FIFO name.

On Linux, `gpsio.OpenSerial` already falls back to `term.OpenPolling`, which
supports FIFOs as `DevFIFO`. `SerialConn.ReadOnly` rejects writes on FIFO-backed
ports.

This is not "replay to a receiver." The useful behavior is paced stdout for
pipe/FIFO consumers, especially `satpulsed` exercising its normal serial input
path.

`satpulsed` opens the FIFO `O_RDWR`, so it does not see EOF when `pack`
finishes; the daemon keeps running and must be stopped by the harness. When its
scan worker is blocked reading an idle FIFO, the daemon can be slow to respond
to `SIGINT`, so a harness may need to escalate to `SIGTERM`/`SIGKILL`.

## Implementation Sketch

Add `internal/packcmd` with the same subcommand shape as `annotatecmd` and
`replaycmd`:

- parse flags with `pflag`
- open file or stdin
- scan with `bufio.Scanner` and a large enough buffer for packet log lines
- write to a `bufio.Writer` wrapping stdout
- inject clock/sleep/writer abstractions inside package-level helpers so timing
  tests do not sleep in real time

Wire it into `cmd/satpulsetool/commands.go` and add a command description in
`cmd/satpulsetool/satpulsetool.go`.

Keep the command narrowly scoped. Do not implement scan/pack round-trip timing
correction, pty management, daemon lifecycle control, or serial-device output
inside this command.

## Tests

Add focused tests for `internal/packcmd`:

- binary packets emit decoded bytes exactly
- ASCII packets emit bytes exactly, including line endings
- metadata lines with no packet data are skipped
- `out:true` entries are skipped by default
- `--tag` filters by `tag`
- `--msg` filters by `msg` and requires `--tag`
- `--tag` and `--msg` matching is case-insensitive
- no matches produce empty output and no warning/error
- invalid JSON fails with line context
- invalid `bin` hex fails with line context
- `--timing` schedules sleeps from emitted packet timestamp deltas
- `--timing` measures `prevWriteStart` immediately before writes
- filtered timing uses previous emitted packet, not previous input packet
- `--timing` flushes after each emitted packet
- `--timing` errors on selected packets with missing/zero `t`

Manual smoke tests:

```sh
satpulsetool pack --tag RTCM gps/testdata/packets/unicore/UM982/rtcm-legacy.jsonl \
  > /tmp/rtcm.bin

satpulsetool pack --tag UBX --msg NAV-PVT internal/gpscmd/testdata/f10t-noop.jsonl \
  > /tmp/nav-pvt.ubx
```

For the FIFO path, use a short fixture and assert that `satpulsed` produces the
expected packet log or event/log output under a temporary config with
`gps.config = false`.
