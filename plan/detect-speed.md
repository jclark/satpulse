# Detect serial speed from received data (#326)

Determine the serial speed of a connected GNSS receiver by examining
what is received at a trial speed, instead of blindly trying speeds
one by one. The signals are:

- speed too high: we are oversampling, so the received bytes show long
  runs of identical bits, i.e. a low bit-transition ratio: the
  fraction of adjacent bit pairs that differ, treating the received
  bytes as a continuous LSB-first bitstream as in the issue's
  experiment code (`bits.OnesCount8((b ^ (b >> 1)) & 0x7f)` within a
  byte, plus one pair per inter-byte boundary);
- speed too low: received bytes generally have a higher transition
  ratio. Framing errors provide supporting evidence when the ratio is
  ambiguous, and also prove activity when the driver reports errors
  without delivering bytes.

Real UART measurements showed that neither transition ratio nor
framing errors identifies direction reliably over arbitrary speed
multiples: framing errors occur on both sides, and sparse bytes at a
very high trial speed can have a high transition ratio. The classifier
therefore uses two thresholds around an ambiguity band, and direction
hints are allowed to make only a bounded change to the caller's order.

## Detection code

Two functions in a new file in `gps/app/gpsio`.

### Contract

Detection follows the `gpscfg.Configure` pattern: it consumes scanned
packets from the standard gpsio reader goroutine via
`<-chan scan.Packet`, and touches the connection only to change speed.
The reader goroutine remains the port's only reader, and because all
data flows through the normal pipeline, packet logging captures all
bytes received during detection, including the wrong-speed garbage
(useful for tuning and for diagnosing misdetections); read errors
are not logged. The scanner passes
unrecognized bytes through promptly as packets with nil `Format`, so
the classifier sees all received data.

`TrySpeed` classifies at whatever speed the port currently has and
never touches the port at all: on its own it is a side-effect-free
check of the current speed. Speed changes between attempts are
`DetectSpeed`'s job, through the existing
`conn.WriteThenChangeSpeed(nil, speed)`: with an empty payload this
reduces to the settle delay plus `term.Change`, takes the write lock,
and records the speed change in the packet log.

### Lower layer: TrySpeed

```go
// TrySpeedResult classifies what was received while listening at one
// speed.
type TrySpeedResult int

const (
    TrySilent   TrySpeedResult = iota // zero value: no data, no serial errors
    TryDetected                       // checksum-valid packet of a known protocol
    TryOther                          // data received but no verdict: try another speed
    TryLower                          // strong low transition ratio: try a lower speed
    TryHigher                         // strong high ratio, or ambiguous with framing: try higher
)

// TrySpeed consumes packets for at most d and classifies what is
// arriving at the port's current speed. Returns early on TryDetected.
func TrySpeed(ctx context.Context, packetCh <-chan scan.Packet,
    procs map[gpsprot.Tag]gpsprot.PacketProcessor,
    d time.Duration) (TrySpeedResult, error)
```

TrySpeed does not take the connection: it only consumes packets,
which keeps it structurally side-effect-free. Its error return
covers context cancellation and the packet channel closing because
the port died; classification always arrives as the result.

Classification of the window:

- `TryDetected`: a packet with `ChecksumValid` whose format counts
  by the same rule as gpscfg's detection
  (`suitableMessageCount`): the format's tag has a packet
  processor, and that processor is not `NativeOnly`. This excludes
  the reply formats - Septentrio replies have no processor,
  NovAtel abbreviated ASCII's is NativeOnly - which matters
  because those checksum-free formats always scan as
  `ChecksumValid` while validating on framing alone. It also
  excludes correction-only formats (RTCM, SPARTN): a port
  emitting only corrections is not usable by satpulsed or
  satpulsewb, and the detected port is destined for them. The
  caller passes in the processor map it already creates with
  `gpsreg.CreatePacketProcessors` (the map's types are all
  gpsprot, so gpsio's layering is undisturbed), and the classifier
  applies the same test as gpscfg.
- `TrySilent`: no data and no serial errors for the whole window,
  i.e. nothing but inter-packet timeout markers
  (`Packet.IsInterPacketTimeout`). A window with read errors but
  no bytes is not silent: the device is transmitting.
- `TryLower`: the bit-transition ratio is below M, currently 0.30.
- `TryHigher`: the ratio is above N, currently 0.35. A ratio from M
  through N is ambiguous; a framing error resolves that ambiguity as
  `TryHigher`. A framing error without any delivered bytes also yields
  `TryHigher`, since the window is active rather than silent.
  Framing errors arrive as `Packet.ReadError` values implementing
  `SerialFraming()` (from term's serial-error support).
- `TryOther`: data or non-framing read errors were received, but the
  transition ratio is ambiguous and no framing error was reported.

The ratio is computed exactly as in the issue's experiment code
(continuous LSB-first bitstream, inter-byte boundaries included).
A strong ratio takes precedence over framing because hardware testing
showed framing errors at trial speeds both above and below the actual
speed.

### Upper layer: DetectSpeed

```go
// DetectSpeed finds the speed at which the device produces valid
// packets, trying the given speeds; 0 as an entry means the port's
// current speed. It returns the actual detected speed (never 0)
// and leaves the port set to it; on failure it restores the speed
// the port had on entry.
func DetectSpeed(ctx context.Context, lg *slog.Logger,
    packetCh <-chan scan.Packet, conn *SerialConn,
    procs map[gpsprot.Tag]gpsprot.PacketProcessor,
    speeds []int, d time.Duration,
    stopSilent func(tried []int) bool) (int, error)

// DefaultSpeedList returns the default speed list, common speeds
// first; 0 means the port's current speed.
func DefaultSpeedList() []int

var ErrSilent = ...           // nothing received at any tried speed
var ErrSpeedNotDetected = ... // data received but no speed validated
var ErrNotSerial = ...        // connection has no terminal behind it
var ErrCurrentSpeedUnknown = ... // entry speed is not a standard speed
var ErrSpeedRestore = ...     // failed detection could not restore it
```

DetectSpeed first checks that the connection is backed by a
terminal, with the same unexported `term()` accessor that
`WriteThenChangeSpeed` uses (it returns nil for a connection opened
through the non-termios fallback: a FIFO, or a non-termios
character device such as /dev/gnss0). Such a connection gets
`ErrNotSerial` immediately, before any packets are consumed: speed
changes on it are silent no-ops, so the candidate walk would
attribute whatever valid packets arrive to an arbitrary candidate
and return a fabricated speed. A pty is a terminal and passes the
check deliberately: termios speed changes are no-ops there too,
but a pty is how tests can drive detection end to end without
serial hardware, detecting at the first candidate whose window
shows valid packets.

The caller controls the speeds and the order they are tried in.
`DefaultSpeedList()` returns the default order, common speeds
first: 38400, 9600, 115200, 0, 460800, 230400, 57600, 19200,
4800, 921600. Callers prune, reorder, or extend it as they see
fit. On entry, `DetectSpeed` captures `conn.Speed()`, copies the
candidate list, and replaces every 0 with that captured speed
before trying or comparing any candidates. This lets the result
always be a real speed and makes direction hints compare against
the port's original speed rather than the literal zero or a speed
selected by an earlier attempt.

The captured entry speed has to be a standard speed: a port left
at a custom divisor rate reports 0, which could not be restored
afterwards, so `DetectSpeed` returns `ErrCurrentSpeedUnknown`
before it changes anything. On any failure, including
cancellation, a deferred restore puts the port back to the entry
speed and flushes. If that restore itself fails, its error is
joined onto the returned error and wraps `ErrSpeedRestore`, so
the caller can tell that the port has been left at an arbitrary
candidate speed. On success the port keeps the detected speed.

Duplicate list entries are removed while preserving the first
occurrence. The remaining list is tried in caller-specified order,
subject to one bounded use of each direction hint: a hint can swap
only the next two untried speeds, and only when the second matches the
hint while the first does not. A speed pushed later by such a swap is
recorded and cannot be pushed later a second time. No verdict removes
a speed.

This rule limits a wrong hint to one additional attempt. In the
default common prefix 38400, 9600, 115200, a strong `TryHigher` result
at 38400 swaps 115200 ahead of 9600; `TryLower` or an ambiguous result
preserves 9600 as the next attempt.

A `TrySilent` verdict does not by itself end the search: silence
at one speed is not proof of a silent device. But while nothing
at all has been received (no bytes, no serial errors), the
`stopSilent` function is consulted after each window with the
speeds tried so far, and true concludes the search early; nil
means never stop early. `ErrSilent` is returned when every tried
window was silent; when the list is exhausted without
`TryDetected` but something was received, return
`ErrSpeedNotDetected`.

When the connection is `DevUSB`, DetectSpeed prepends 115200 to
the list; duplicate removal makes the later entry a no-op. A
native-USB receiver delivers valid packets at whatever speed is
tried, so the first entry is the one that gets detected and
recorded; starting high makes that a sensible value, and on macOS
the speed used with a DevUSB device does make a difference. This
is only a starting point, not a shortcut: it is a common
misconception that a cdc-acm (ttyACM) port's speed is meaningless
- USB-to-serial converters also present as ttyACM with a
wire-real line speed - so DevUSB ports get the same full walk as
everything else.

The per-try window `d` is a parameter of `DetectSpeed`, passed
through to each `TrySpeed` call.

### Speed-change coordination

Three layers of stale data can leak across a speed change. The first
is handled where the change happens (in DetectSpeed, between
attempts); the others are TrySpeed's own:

1. Kernel input buffer: bytes received at the old speed are still
   queued. Immediately after the change, call `term.Flush` (TCIOFLUSH;
   gpsio already uses it at open). The output side is empty since
   nothing was written.
2. Pipeline: packets scanned before the change may still sit in the
   channel, and a read in flight during the flush can deliver
   pre-flush bytes with a post-change timestamp. TrySpeed discards
   packets whose `TRead` precedes its own start time plus a margin of
   about one read timeout (100 ms).
3. Scanner state: a stale partial packet can fuse with the first
   new-speed bytes into one boundary garbage packet. The same margin
   covers it; the classifier is statistical and tolerates one bad
   packet.

### Testing

The transition-ratio computation and the window classifier are pure
functions over byte strings and packet sequences; factor them so they
are testable directly, with `TrySpeed` a thin shell around them.
Drive classifier tests with synthetic packet streams (constructed
garbage with known transition ratios, `ReadError` packets, valid NMEA
and UBX packets) and with recorded wrong-speed captures from real
hardware once available. Test `DetectSpeed`'s search logic against a
scripted fake of TrySpeed results. Timing behavior (windows, cutoff
margins) uses `testing/synctest` per the repo pattern.

A pty cannot emulate wrong-speed effects (it ignores termios speeds),
so end-to-end validation is against real receivers. Tests on a u-blox
LEA-M8T listening initially at 38400 measured ratios of 0.138 (UBX)
and 0.196 (NMEA) when the receiver was at 9600, versus 0.396 (UBX)
and 0.475 (NMEA) when it was at 115200. The detector tried 9600 next
in the first two cases and swapped 115200 ahead of 9600 in the latter
two, then validated packets at the actual speed.

### Open questions

- Window length: long enough for receivers that emit only one burst
  per second; whether `TrySilent` needs a longer confirmation window
  than the directional verdicts.

## satpulsetool serial

A new subcommand combining port enumeration (#394) with speed
detection. It has three modes: enumeration (no arguments),
single-device detection (a device argument), and scan (`--scan`).

### Enumeration

`satpulsetool serial` with no arguments lists the enumerated
serial ports. As in `satpulsetool sdp`, printing goes through a
JSON-tagged struct (the sdpcmd Printer pattern), so `-j`/`--jsonl`
comes for free. The fields are `Device`, `Display`, and a composite
`USB` value containing numeric `VID`/`PID`, matching `serialenum.Port`
from the #394 plan. `Device` is the canonical `/dev/<kernel name>`
path; top-level aliases are shown only within `Display` and are not
separate output fields. Enumeration reads only sysfs and `/dev`
directory entries and never opens a device, so this mode needs no
dialout membership.

### Single-device detection

```
satpulsetool serial [--packet-log path] <device>
```

The device is opened directly by path (it need not appear in the
enumeration) at its current speed; the command starts the standard
gpsio reader goroutine and runs `DetectSpeed` with
`DefaultSpeedList()`, a per-try window of 1.25 s, and a
`stopSilent` that stops after five tried speeds (both detection
modes use these). On success the detected speed is
printed to stdout as a single number and the exit code is 0, so
scripts can compose it, e.g.
`satpulsetool gps -s $(satpulsetool serial /dev/ttyS0) ...`.

Failures print a description to stderr. The cases, with exit
codes:

- 1: permission denied. EACCES on open, i.e. the user is not in
  dialout; separated from other system errors so the message can
  name the fix.
- 1: locked. `term.Open` takes `flock(LOCK_EX|LOCK_NB)`
  unconditionally, and a port held by a TIOCEXCL user (gpsd)
  fails non-root opens with EBUSY. Root bypasses TIOCEXCL, so
  until #117 is implemented, detection run as root is vulnerable
  to conflict with programs that rely on TIOCEXCL.
- 1: no known protocol validated at any tried speed
  (`ErrSpeedNotDetected`). The wording must not overclaim: this
  case covers both a non-GNSS device and a GNSS receiver at a
  speed outside the candidate list.
- 1: not a serial device (`ErrNotSerial`, e.g. a FIFO): speed
  detection is meaningless there.
- 1: the port's current speed is not a standard speed
  (`ErrCurrentSpeedUnknown`), so nothing was tried.
- 1: the original speed could not be restored after a failed
  detection (`ErrSpeedRestore`), whatever the failure was: this
  outranks silence, because the port has been left on a candidate
  speed and the reason has to reach the user.
- 1: other system errors (device disappeared, ...).
- 2: silent (`ErrSilent`): nothing received.

### Scan

`satpulsetool serial --scan` (short `-s`) runs single-device
detection on every enumerated port in parallel. A port whose
holder locks it reports as locked (flock always;
TIOCEXCL for non-root scans); a holder that takes no lock is not
detectable, and scan will open and probe its port. Combining
`--scan` with a device argument is a usage error.

Each port where detection succeeds contributes one stdout line:
the device name, a space, the detected speed. The name is the
port's canonical `Device` from enumeration. Each failure
contributes one stderr line, `<device>: <description>`, with the
same case distinctions as above. Each port gets exactly one
output or error line, printed as soon as its detection finishes,
so the output order is completion order.

### Exit codes

One rule covers all modes. The possible outcomes for a port are
ordered from best to worst: detected, silent, error. The exit
code is the best actual outcome over a non-empty set of ports:
0 for detected, 2 for silent, and 1 for error. If no ports are
found, enumeration or scan exits 2 explicitly. Single-device
detection is the one-port case: 0 detected, 2 silent, 1 anything
else. For enumeration, which does not probe, a listed port counts
as detected. Usage errors and command-level failures are exit 1.

### Common

The detection modes run under `cmd.CancelOnSignal`, as the gps
command does: Ctrl-C cancels the context, which `TrySpeed` and
`DetectSpeed` honor mid-window. An interrupted probe restores the
port's original speed like any other failure, and a scan waits for
all its probes to finish that cleanup before exiting. An
interrupted run exits 1.

Detection is not exposed through `satpulsetool gps` (no `-s auto`):
the gps command's output vocabulary already uses "Serial speed" for
the receiver-side port configuration reported by `--show-port`, so a
host-side detection result has no unambiguous place to be reported
there. The composed form above covers that use.

Per-attempt diagnostics are logged by `DetectSpeed`, so they appear
under satpulsetool's global `-v` flag. They include the speed, verdict,
byte count, transition ratio, framing-error count, read-error count,
and stale-packet count. The received bytes are also recoverable
offline from the packet log, but read errors are not logged, so
framing-error evidence comes from these `-v` diagnostics, not from
captures.

`--packet-log` has the same semantics as the gps command's flag and
applies only to the single-device form. The captured wrong-speed
garbage is the raw material for tuning the classifier constants,
and speed changes appear in the log via the existing `logWrite` on
the change path.

Implementation is a new package `internal/serialcmd`, registered in
`cmd/satpulsetool/commands.go` alongside the existing subcommands.
It makes satpulsetool the second consumer of `gps/lib/serialenum`
and so depends on the #394 enumerator. The change adds a
`satpulsetool-serial.1.md` man page and carries a NEWS entry
(#326).

## Workbench (not implemented yet)

Workbench integration was explicitly excluded from this implementation.
The following remains possible future work.

The speed dropdown gets an explicit Auto entry, and Auto is the
default selection. Connecting with Auto runs detection between
opening the port and configuration; when detection succeeds, the
dropdown changes to the actual speed. On failure the session goes
back to disconnected and the connection bar shows the error, with
distinct messages for `ErrSilent` (no output from the device) and
`ErrSpeedNotDetected` (output seen but no speed validated); the
device stays filled in and Auto stays selected. While detection is
running the connection bar shows a "Detecting speed..." state.

Once detection succeeds, the detected number simply becomes the
current speed selection, and it persists like any user choice:
it survives disconnecting (so reconnecting is fixed-speed), and
changing the device never alters the speed selection. To re-detect,
the user selects Auto again.

### session.Speed

A new type represents a speed selection that is either a positive
speed or auto:

```go
// Speed is a serial speed selection: a positive speed or auto.
// The zero value is auto.
type Speed int

const SpeedAuto Speed = 0
```

It implements four marshaling methods:

- `MarshalJSON`/`UnmarshalJSON`: `"auto"` or a bare number on the
  wire.
- `MarshalText`/`UnmarshalText`: `auto` or digits; the satpulsewb
  flag adapter parses with `UnmarshalText` (see the satpulsewb
  command line section).

`SerialOpener.Speed` stays a plain int with its existing 0 =
keep-current meaning: `session.Speed` lives at the connect-request
surface, and the session maps auto to "open keeping the current
speed, then detect". The two zero values never meet.

### Wire and frontend

Because `Speed`'s zero value is auto, an optional speed is always
a `*session.Speed`, never a zero value: `omitempty` on a value
field would swallow auto before `MarshalJSON` could render it, and
an absent field would silently unmarshal to auto.

`POST /api/connect` takes `{device, speed}` where speed is
`session.Speed` (`"auto"` or a positive number); the current
`speed > 0` validation becomes validation of the unmarshaled type,
which unmarshals through a pointer so that an absent field is
rejected rather than read as auto.
In the connection reply, `connectionInfo.Speed` becomes a
`*session.Speed`: nil (field omitted) when no speed has been
specified or chosen, otherwise `"auto"` or the concrete number.
On the TypeScript side the transport's `connect` takes
`number | 'auto'`, `ConnectionInfo.speed` becomes
`number | 'auto'` but stays optional (absent when no speed has been
specified or chosen, as today), and `ConnState` gains a
`'detecting'` value between `'connecting'` and `'configuring'`.
While detection runs, the connection reply reports `speed: "auto"`
and `state: "detecting"`, so a browser loading mid-detection shows
the truth; when detection succeeds the server records the concrete
speed, keeping the property that a reloaded window sees the same
values as the one that connected. The
dropdown flips to the detected speed through the existing
`gps:speed` sticky event; no new event type is needed.

The shared speed list in `webui/packages/workbench/src/speeds.ts`
remains a list of concrete numeric baud rates because it is also used
by the receiver's UART-speed control. The connection panel prepends an
explicit Auto option to that list and maps its value to the `'auto'`
literal rather than parsing it as a number. The built webui assets are
regenerated and committed as usual.

### Session flow

An Auto connect runs, in the session's connect path:
open (keeping current speed) -> `DetectSpeed` with
`DefaultSpeedList()` (pruned or extended as the workbench sees
fit) -> emit `gps:speed` ->
`gpscfg.Configure`. How the auto request travels through
`sess.Connect` (an option, or a field beside the opener) is an
implementation choice. Detection failure follows the same path as
configuration failure today: state transition back to disconnected
with the error surfaced through the session's event stream.

After a successful detection the session holds a concrete speed,
exactly as if the user had connected with it manually; reconnection
after a device disappears and returns reuses it and does not
re-detect. The mechanism: when detection succeeds, the session
updates the stored opener's speed from 0 to the detected value
before configuration proceeds, so `reopen`, which re-runs the
stored opener unchanged, opens fixed-speed.

### satpulsewb command line

`-s`/`--device-speed` becomes a `*session.Speed` flag (nil when
not given, per the rule that an optional speed is a pointer),
populated through a small pflag `Value` adapter that parses with
`UnmarshalText`, so `-s auto` is accepted and behaves like
choosing Auto in the UI. The semantics of `-d` and `-s` are unchanged: each
prefills its control in the connection bar, and when both are given,
satpulsewb connects at startup with those values. The option has no
default (the presence of `-s` matters, so a default value would be a
lie): when `-s` is not given, no speed reaches the frontend and the
speed control starts on its own initial selection, which becomes
Auto. The groundwork is already in place: the connection reply omits
the speed when none has been specified or chosen, and the flag
carries no fake prefill value.
