# Connect to GPS receivers over TCP (#382)

satpulsetool, satpulsewb, and satpulsed should be able to work with
a GPS receiver reached over a TCP connection the same way they work with one on a directly attached
serial port. The typical far end is a satpulsed instance proxying the
receiver's serial port via `proxy.tcp`, but not always: some
receivers (e.g. Septentrio) have native TCP ports, and dumb
serial-to-TCP bridges like ser2net exist. This plan absorbs phase 10
of [satpulseweb.md](archive/satpulseweb.md) and the `tcp-connect`
entry in webui/packages/workbench/plan/issues.md.

The design decisions, all now resolved, are collected in the
Decisions section at the end.

## The transparency principle

The model is that a TCP connection is a virtual serial port: TCP
proxying looks exactly as if the receiver were talking its native
protocol over TCP. The client never knows, and must never need to
know, what is at the far end: satpulsed's proxy, the
receiver's own TCP port, or a dumb bridge that knows nothing about
GNSS protocols. Consequences:

- No proxy-vs-native distinction anywhere: no flags naming the
  far-end kind, no behavior keyed on it.
- TCP does not take the socket detection shortcut. `--socket` sets
  `ConfigOptions.Socket`, which tells gpscfg the daemon already
  validated the receiver: probe immediately, skip the
  silence/deadline timers and the serial-noise diagnostics. That
  assumption is sound for a unix socket (same box as satpulsed by
  construction) and unsound for TCP, where the far end may be
  ser2net at the wrong baud rate. TCP takes the ordinary
  serial-shaped path: full detection with probe and timers, scanner
  resync from an arbitrary mid-stream position, noise tolerance.
- A connection drop is device loss, the same path as an unplugged
  USB receiver: emit the disconnected state (or exit, for
  satpulsetool), let the user reconnect. No automatic redial. A
  reset-class operation needs no transport-specific handling: it
  may or may not drop the connection (many resets do not), and when
  it does, that is just device loss.

## Non-goals

- Working well over arbitrarily laggy or flaky links. TCP here is a
  fake serial port over a sane network (LAN, VPN, ssh tunnel). The
  serial-tuned client path is not deoptimized for TCP's benefit.
- UDP. `[[stream.push]]` UDP output is unrelated one-way push and
  stays as is.
- A framed or timestamped proxy protocol. Raw byte transparency is
  the contract; if end-to-end timing behavior needs improvement,
  the lever is how `proxy.tcp` writes (which we control), not a new
  wire format.

## Design: gpsio TCP dialing

A new opener in gpsio alongside `OpenSocket`:

```go
func OpenTCP(addr string) (*NetConn, error)
```

A plain `net.Dial` (stdlib default keepalive), returning the
existing `NetConn` unchanged (100ms read deadline, `Direct()` false,
`Buffered()` 0, `Drain()` no-op). `addr` is `host:port` with DNS
names and IPv4/IPv6 literals (bracketed IPv6), matching
`CorrectionSource.Host` semantics.

Keepalive is the drop detector: a dead peer with no FIN (far box
loses power, network partition) otherwise looks exactly like a
silent receiver -- an endless stream of read timeouts. When
keepalive probes exhaust, the next read errors, the scan loop exits,
and the ordinary device-loss path runs. Start with the stdlib
default keepalive (15s probes, OS-default count; detects a dead
peer in roughly two minutes); an explicit `KeepAliveConfig` can
tighten that later if two minutes proves too slow. Related: #59
covers the server (proxy) side of keepalive and connection-reset
handling; this is the client side.

## Inter-packet timing over TCP

How the mechanism works today: reads time out after 100ms
(serial and `NetConn` identically); an empty-buffer timeout becomes
an `IsInterPacketTimeout` packet, which drives `Idle()` in the
consumers -- the NMEA satellite buffer's primary flush trigger,
gpscfg, and satpulsed's dispatcher. `satpulsetool gps --socket`
already runs this entire pipeline through `NetConn` and works.

Expected behavior on a sane network: `proxy.tcp` writes one packet
per `Write` call and Go disables Nagle by default, so inter-burst
gaps of >= 100ms survive a LAN hop essentially intact. The failure
modes when timing does get distorted, all rare on the networks in
scope and all already survivable:

- An absorbed gap: the satellite buffer falls back to its
  key-detection flush and the satellite display lags one cycle (the
  known caveat from issues.md).
- Jitter inserting a >= 100ms pause mid-burst: an early flush with a
  partial constellation for one cycle.
- A >= 100ms stall in the middle of a packet: the scanner treats it
  as a mid-packet error and resyncs, dropping that packet.

No client-side changes are made for any of these. If real-world use
shows a problem, the fix is server-side in `proxy.tcp`'s write
behavior, where both ends are ours; a dumb bridge's timing is
whatever it is.

Read timestamps (`TRead`) become client-side arrival times, skewed
by network latency. For configuration and monitoring this is
irrelevant at LAN latencies. It is one reason satpulsed-as-client
(below) is a separate scoping decision.

## Surface: satpulsetool command line

A `--tcp host:port` flag in gpscmd's shared flags, a peer of
`--socket path`, with exactly one of
`--serial-device`/`--socket`/`--tcp` required. (A URL form on the
existing device flag was considered and rejected: it overloads a
flag documented as a device path.)

Because the flag lands in gpscmd's shared flag set, every
satpulsetool command that accepts `--socket` accepts `--tcp` at
once. Details that follow from the socket precedent, applied to
TCP:

- Speed flags do not apply (no baud rate on a TCP connection);
  same handling as socket.
- Output-packet logging is unsupported (the existing socket warning
  applies to TCP too).
- `ConfigOptions.Socket` is NOT set for TCP (see the transparency
  principle): detection runs the full serial-shaped path.

satpulsewb's flags reuse the satpulsetool names and help strings
exactly, per the satpulseweb.md convention: `--socket PATH` and
`--tcp HOST:PORT` join `-d`/`-s`, mutually exclusive with them, and
auto-connect at startup the way `-d` does.

## Surface: satpulse.toml (satpulsed as TCP client)

satpulsed is in scope: it can use a TCP-reached receiver in place
of `serial.device`. The motivating use case is timing with one GNSS
receiver and a PPS distributor: several machines share the
receiver's PPS through hardware distribution, one box owns the
serial port (and can proxy it), and the others take their PPS
locally and get the GNSS messages over TCP. The PPS carries the
precision; the TCP-delivered messages only label seconds, which
tolerates LAN-scale latency and jitter. Monitoring-only and
GPS-only instances fall out of the same support.

Daemon mechanics: the connection open in daemon.go branches to
`OpenTCP`; detection takes the full serial-shaped path (no socket
shortcut); speed configuration does not apply. Device loss keeps
its existing shape for free: a dropped connection ends the scan,
satpulsed exits, and systemd restarts it, which redials.

Config shape: `tcp.address` under the `[serial]` table, mutually
exclusive with `device` (and `speed` does not apply). The model is
that TCP provides a virtual serial port, so it belongs in the
`[serial]` table. The key naming matches the established
satpulse.toml convention: `address` is the key for outbound
connections (`[[stream.pull]]` has `tcp.address` with exactly this
form and meaning; `ntrip.address` likewise) while `listen` is the
key for binds.

## Surface: workbench GUI (satpulsewb)

Initially there is no way to configure a TCP connection from the
UI: satpulsewb gets the `--tcp` CLI option, which auto-connects at
startup the way `-d` does. In the connect bar, the device dropdown
shows the TCP address and the speed field is empty. Connecting to a
different address means restarting with a different flag. A
UI-entered address can be added later without disturbing this.

Reset-class controls are not gated on TCP connections (or any
others). Gating -- an idea inherited from satpulseweb.md -- was
rejected as both useless and wrong: useless because the Messages
tab can send a reset command from a message file regardless of
what the config UI hides, and wrong because its premise is false --
many resets do not re-enumerate or drop the connection at all. A
reset that does kill the connection is ordinary device loss. The
`Opener` seam is unaffected: `Opener.Socket()` answers only the
detection-shortcut question, and `TCPOpener{Addr}` returns false.
(The session briefly enforced this gating -- ApplyConfig refused
resets when Socket() was true -- and that has been removed.)

The wire contract additions (connect request carrying a transport
kind and address, snapshot/state changes if any) follow mechanically
from these decisions.

## Testing

- Smoke tests: a satpulsed instance replaying a packet log with a
  `proxy.tcp` service, with satpulsetool `--tcp` (and satpulsewb
  `--tcp`, via the program dimension) as the client -- exercises
  monitor-path behavior including idle-driven satellite flushing
  over a real local TCP hop. A socat/ser2net-style pty-to-TCP
  bridge scenario covers the dumb-far-end case (mid-stream connect,
  no packet-aligned writes).
- Detection over TCP: the u-blox simulator (`satpulsetool ubxsim`)
  behind a satpulsed `proxy.tcp` gives the full probe/identify path
  end to end with no hardware, once the phase-9 simulator provider
  from satpulseweb.md exists.
- Hardware check on this machine: the F9P behind a socat TCP bridge,
  and behind a local satpulsed `proxy.tcp`.

## Docs and delivery

- Man pages: satpulsetool(1) gps flags, satpulsewb(1), and
  satpulse.toml(5). NEWS.md entry with the issue number, in the
  same change as the implementation.
- No new packages; docs/internals/packages.md is unaffected (gpsio gains a
  function).
- Delivery: ordinary PRs off master. Natural split: PR 1 gpsio
  dialing + satpulsetool `--tcp` + smoke tests; PR 2 satpulsed
  (config shape, daemon wiring, man page); PR 3 satpulsewb flags +
  connect-bar display.
- File the issue and fix the heading; remove the issues.md
  tcp-connect entry when implemented. (satpulseweb.md's phase 10
  has been trimmed to a pointer here and that plan archived.)

## Decisions

All resolved: the satpulsetool flag is `--tcp host:port` (not a URL
form on `-d`); keepalive starts with the stdlib default (~2 min
detection), tightened later only if that proves too slow; satpulsed
is in scope (the PPS-distributor timing use case); the satpulse.toml
shape is `tcp.address` under `[serial]`; satpulsewb initially gets
`--tcp` only, no UI configuration (the device dropdown shows the
address, speed shows empty); reset-class controls are not gated on
any transport.
