# satpulsetool ntrip subcommand

Add `satpulsetool ntrip` for fetching data from an NTRIP caster.
Primary use cases:

- Interop-test the `NTRIPSource` client code.
- Debug/analyse NTRIP mountpoints.  Default output is a JSONL
  packet log, which can be piped through `satpulsetool annotate`
  to see message types, rates, station IDs, and payload details
  for whatever a caster is actually sending.
- Forward RTCM corrections to a local receiver (replaces the
  `str2str` step in `docs/howtos/rtk.md`).

Prerequisite: `NTRIPSource` in [gps/app/stream/pull.go](../gps/app/stream/pull.go).

## Usage

```
satpulsetool ntrip [options] <address[:port]> <mountpoint>

  --user user[:password]   NTRIP credentials (password may be omitted)
  --bin                    emit raw bytes to stdout (default: packet log JSONL)
  -h, --help               show help
```

`address` defaults to port 2101 (NTRIP conventional port) if no
port is given.

Default output: one line of JSON per scanned packet to stdout,
using the existing `gpsio.PacketLogEntry` schema (`t`, `tag`,
`msg`, `bin`/`ascii`).  Timestamps are stamped by the scanner
when bytes come off the socket, so timing is accurate within the
process.  Terminal-safe by default.

`--bin` writes raw bytes to stdout with no framing.  Loses
per-packet timing, but that is fine for the forwarding use case
(an RTCM-consuming receiver scans its own bytes).  Example:

```
satpulsetool ntrip ptp.lan:2101 bkk --user jjc:xyzzy --bin \
  | socat - UNIX-CONNECT:/var/run/satpulse.sock
```

## Behaviour

- Single connection.  No auto-reconnect: the command exits when
  the caster closes the connection or on SIGINT.  If reconnect is
  wanted later, add `--reconnect`.
- Scanner uses `gpsreg.CreatePacketFormats(gpsreg.VendorUnknown)`
  (NMEA + RTCM + all vendor binary/ASCII formats).  Casters
  normally send RTCM only; a few emit ASCII status lines on
  error, which the NMEA-style line scanner will capture as
  `ascii`.
- Password in argv is a known leak via `ps`.  NTRIP passwords are
  low-value (widely shared, often public); accept the risk for
  now.  Add `--password-file` / env var later if needed.
- No `--timeout` flag for v1; can add later.

## Implementation

New package `internal/ntripcmd` exposing

```go
func Cmd(logWriter io.Writer, logLevel slog.Level, progName, cmdName string, args []string) (usage string, err error)
```

matching the convention of the other `internal/*cmd` packages
(see [internal/decodecmd/decodecmd.go](../internal/decodecmd/decodecmd.go)
for the simplest example).

Wire into [cmd/satpulsetool/satpulsetool.go](../cmd/satpulsetool/satpulsetool.go):

- Add `case "ntrip": exec = ntripcmd.Cmd` to the dispatch switch.
- Add `"  ntrip - fetch data from an NTRIP caster"` to the
  `usage` function.

Flow inside `Cmd`:

1. Parse flags with `pflag.NewFlagSet(cmdName,
   pflag.ContinueOnError)` and build `usageFunc` via
   `cmd.UsageFunc` ([gps/app/cmd/usage.go](../gps/app/cmd/usage.go)).
   Require exactly two positional args.  If
   `net.SplitHostPort(addr)` fails, default the port to 2101 via
   `net.JoinHostPort`.  Split `--user` on the first `:` (no colon
   = whole string is username, empty password).
2. `lg := cmd.NewDefaultLogger(logWriter, logLevel)`, then
   `ctx, _ := cmd.CancelOnSignal(context.Background(), lg)`
   ([gps/app/cmd/cmd.go](../gps/app/cmd/cmd.go)).
3. Build `stream.NTRIPSource{Addr, Mountpoint, Username,
   Password, UserAgent: stream.NTRIPUserAgent{Version: v}}` where
   `v, _ := cmd.Version()`.  Call `src.Connect(ctx)` and
   `defer rc.Close()`.
4. If `--bin`: `io.Copy(os.Stdout, rc)`.
5. Otherwise: `scan.New(rc, 16, formats)` loop (bufSize 16
   matches `stream.scanBufSize`).  For each scanned packet, write
   `json.NewEncoder(os.Stdout).Encode(gpsio.NewPacketLogEntry(pkt))`.
   Treat `io.EOF` (or cancelled ctx) as a clean exit.

### gpsio helper

The "scanned packet -> PacketLogEntry" conversion lives inside
`PacketLog.LogInput` ([gps/app/gpsio/log.go](../gps/app/gpsio/log.go))
and depends on the unexported `useBinary`.  Rather than duplicate
that logic, extract it into an exported helper:

```go
// NewPacketLogEntry builds a PacketLogEntry for an incoming scanned packet.
func NewPacketLogEntry(pkt scan.Packet) PacketLogEntry { ... }
```

and have `LogInput` call it.  Trivial refactor; keeps a single
source of truth.  (This is the extraction foreshadowed by issue
#246.)

## Testing

Unit tests for flag parsing only: positional-arg count, port
default, `--user` with / without password / with embedded
colons, `--bin` vs default, `-h` returns usage with nil error.

Skip an integration test with a fake caster: `NTRIPSource`
(connect + handshake) and `scan.New` (packet scanning) already
have coverage in their own packages, and everything ntripcmd
adds on top is thin glue best verified end-to-end against a real
caster.

## Verification

Manual end-to-end against a public caster:

```
satpulsetool ntrip caster.example:2101 MNT | satpulsetool annotate | head
```

Expect JSONL with `tag: "rtcm"` and decoded MT100x / MT107x
bodies.  SIGINT during the stream should exit cleanly.

## Future

- `--reconnect` with adaptive backoff (reuse `stream.backoff`).
- `--password-file` / `$NTRIP_PASSWORD` env.
- `--timeout` on connect.
- Source table request (omit mountpoint -> print source table):
  would need the deferred source-table parser from
  `plan/ntrip-client.md`.
