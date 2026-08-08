# Serial PPS (#402)

## Introduction

Support a PPS signal delivered over a serial port's modem control lines,
detected and timestamped in user space, as the sample source for the NTP
refclock outputs (chrony SOCK, NTP SHM).

The first target is macOS: the receiver's data lines and its PPS output
are wired to a single USB to TTL serial adapter, with PPS on the CTS pin.
macOS has no kernel PPS support, so this is the main useful case of #259.

The reason for satpulsed to provide this, rather than leaving PPS to the
NTP daemon, is that it is functionality that cannot be in chrony or
ntpd-rs: they require a kernel PPS device (`/dev/ppsN`), and have no way
to watch a serial port's modem control lines. A `/dev/ppsN` reader is
explicitly out of scope: it would add nothing over what chrony already
does.

## Measured basis

Experiments on 2026-08-06 with a u-blox F9P and an FTDI FT232R on macOS
(`cmd/pollpps` on the `serial-pps-tmp` branch, on a MacBook Air with the
adapter attached directly) established:

- The `TIOCMGET` ioctl blocks for ~2 ms on this adapter (a USB round
  trip), capping polling at ~500 polls/s regardless of strategy.
- Polling continuously at that rate costs ~4-5% of a core, which is too
  much for an always-running daemon. Polling only around the expected
  pulse time reduces this to a negligible level.
- Edge timing repeatability is roughly 0.15-1 ms; a C implementation is
  no better than Go, so detection stays in-process in Go.
- Serial message arrival jitters with sd ~8 ms and this comes from the
  receiver's own emission timing (it is the same over native USB CDC),
  so message arrival times cannot serve as a precise fiducial; the PPS
  edge is the only usable one. Message timing is still easily good
  enough to identify which second a pulse belongs to (mean delay
  ~124 ms at 38400 baud, far inside the +/-0.5 s bound).

Experiments on 2026-08-08 on a desktop Mac, with the same FT232R and an
FT232H (high-speed USB), showed the ioctl cost is a property of the
machine, not the adapter: there `TIOCMGET` takes ~150-190 us on the
FT232R and ~125 us (one high-speed microframe) on the FT232H, through a
USB3 hub or attached directly, and edge jitter is sd ~50-95 us on both.
The 2 ms figure did not reproduce at all on that machine. Nothing in
the design depends on which regime holds -- the loop calibrates itself
to either -- but the same-day findings that do generalize are:

- Edge delivery has a millisecond-scale late tail (a small fraction of
  edges arrive up to ~1 ms late), which on a fast-ioctl machine makes
  the settled window far wider than the bracket gap; the safety factor
  is sized for that tail (a sweep on the FT232R: c = 4 caught 68% of
  pulses, c = 10 99%, c = 16 100%, with CPU flat at ~1% across
  c = 7..16).
- Phase 1 was validated end to end on that machine: chrony selects the
  SOCK refclock and tracks it with ~100 us error bounds; per-sample
  offsets have sd ~70-100 us, and successive median-filtered estimates
  wander by ~40 us regardless of filter length.

## Configuration

One new key in the `[serial]` table:

```toml
[serial]
device = "/dev/cu.usbserial-XXXXXXXX"
speed = 38400
pps.pin = "cts"
```

`pps.pin` names the modem control line carrying the PPS signal; its
presence enables PPS detection. Any of the four input lines is
accepted: `"cts"`, `"dcd"`, `"dsr"`, `"ri"`. CTS is the recommended and
tested wiring; DCD is the traditional serial-PPS line but many USB
serial drivers do not surface it.
The pulse's leading edge is assumed to be electrically rising, which on
a TTL-level adapter is observed as the CTS flag becoming deasserted; a
`pps.edge` key can be added later if inverted pulses turn out to matter.

The serial device must be a real TTY; configuring `pps.pin` on anything
else (FIFO, socket) is a startup error.
`pps.pin` and `phc.interface` are mutually exclusive: the PHC is the
higher-precision source, and configuring both is an error rather than
silently replacing it with serial PPS.

In the daemon package the `pps` table maps to a pointer field, nil when
the table is absent, following the existing pattern of `ntp.sock` and
`ntp.shm`:

```go
type SerialConfig struct {
	Device string
	Speed  *int
	PPS    *SerialPPSConfig `toml:"pps"`
}

type SerialPPSConfig struct {
	Pin string `toml:"pin"`
}
```

When `pps.pin` is configured, refclock samples come from PPS edges, and
the existing samples based on message arrival times are disabled. Time
messages are still consumed: they identify which second each pulse marks.

## Sample generation

A detected edge at system time T becomes one refclock sample: the
reference time is the integer second the pulse marks, identified from
recent time messages (whose `utc - tRead` bounds the system clock error
to well under the +/-0.5 s needed for correct rounding); the offset is
reference minus T; the leap indication comes from the time messages, as
in the existing message-based sampling. If the newest time message is
more than 3 s old, the edge produces no sample. Emission uses the
existing refclock path (SOCK and/or SHM); the fixed SHM precision
reported in this mode is 2^-11 s (~500 us), not the 2^-1 s used for
message-arrival sampling.

With the polling backend, edges caught before the settling latch (see
Edge detection) do not produce samples; they are known too coarsely
and would inject uncharacteristically bad data points. So the first
pulses at startup, and after a signal loss, yield no samples; after
settling every caught pulse yields one. The wait backend has no such
restriction: every detected leading edge yields a sample. Sawtooth
correction is irrelevant at this precision and is not used.

## Edge detection

Two backends behind one interface, selected by platform. For both, the
implemented rule is: the pulse's leading edge is the configured pin's
flag becoming deasserted (per the TTL-level assumption stated under
Configuration), and that is the only transition that is timestamped.

### Polling (phase 1: macOS; also any platform without a wait primitive)

One adaptive loop, driven by the fact that consecutive pulses are 1 s
apart. The state is the predicted next edge time P (previous edge
plus exactly 1 s, in the monotonic clock) and a window half-width M.
Each second the loop polls the window [P - M, P + M] at a target
spacing of M/N, floored at 50 us. It sleeps until each poll's
scheduled time; when the state query takes longer than the target
spacing the sleep never fires and the calls pace the loop by
themselves. The query's duration is never measured or assumed.

- Catch: two consecutive polls bracket the transition (the earlier
  out of pulse, the later in pulse). The edge timestamp is the
  midpoint of the pair; P becomes edge + 1 s and M becomes c times
  the pair's measured gap, capped at half the pulse period. Using
  the measured gap rather than the target spacing makes the window
  immune to sleep overshoot: a stretched bracket widens the next
  window automatically. A bracket whose gap is a full period or
  more may span several leading edges, so its midpoint identifies
  none of them; it counts as a miss.
- Miss: the window closes without a transition. P advances 1 s and
  M doubles, capped at half the pulse period; at the cap the
  window is the whole period and polling is uniform, which is the
  cold-start state. Acquisition is not a separate mode. A pulse
  already in progress when the window opens is not a miss: the
  windows advance in lockstep with the pulses, so aborting would
  reopen at the same phase every period and never acquire. The
  loop polls through the pulse and resumes the search for the next
  leading edge on its far side.

From cold, each catch shrinks M by the factor c/N until the bracket
gaps stop shrinking, at the floor set by the state-query time or
the spacing floor, whichever binds first. The loop never needs to
know which; the floor emerges from the measured gaps. PPS jitter
and clock drift do not enter the gaps: prediction error shows up as
misses that widen the window instead. Lock takes on the order of
ten pulses at about N polls per second, on any hardware, with no
retuning.

Constants: safety factor c = 16 (the window half-width in bracket
gaps, chosen from the measured FT232R delivery tail), shrink rate
r = 1/2 per catch, spacing floor 50 us. N is not chosen: it is
derived as c/r (32), since the shrink per catch is c/N and choosing
c and N independently allows a non-convergent pair (c >= N leaves
the window stuck at the cap). All are dimensionless shape
parameters with wide safe ranges; nothing encodes hardware timing,
so there is nothing to revise per adapter.

Publishing is gated by a latched settling state, not a threshold.
While each catch still improves on the previous catch's bracket
gap, caught edges are suppressed; a settled flag latches at the
first catch whose gap does not improve on the previous one, which
is the moment the measured floor is reached. Misses in between do
not affect the comparison: a miss says the prediction was wrong,
not that the resolution changed. The flag clears, and the gap
memory with it, only when M walks back up to the cap (signal loss,
hence a genuinely new settling period). The point of the
suppression is only to withhold the
uncharacteristically bad cold-start samples: after the latch every
catch is published, however coarse, because a sample stretched by an
oversleep is characteristic of what that system delivers, and the
NTP daemon's own filtering is the right place to handle such
outliers. Chrony has no notion of an initial settling period, so the
loop provides one.

The bracket needs a poll inside the pulse, so the poll spacing must
stay below the pulse width; the cold-start spacing of about 16 ms
suits the u-blox default width of 100 ms, and microsecond-width
pulses remain unsupported by this backend.

The polling goroutine locks its OS thread.

### Wait (phase 2: Linux, Windows)

Linux `TIOCMIWAIT` blocks until a chosen modem control line changes;
Windows has the equivalent `SetCommMask`/`WaitCommEvent`. Neither
reports the transition's direction nor a timestamp, so on each wakeup
the backend takes the timestamp immediately -- before any further
calls, since a state read can itself be a millisecond-scale USB round
trip -- and then reads the line state to classify the
transition. Every wakeup where the line has entered its pulse-asserted
sense produces a sample, subject only to identifying the second from
the time messages. Unlike the polling backend, nothing here relies on
the pulses being 1 s apart.

`term` gains a primitive that only waits; reading the state afterwards
is the existing method. Since only some terminals can wait, it is a
capability interface that the PPS source asserts for, not a method of
`Term`:

```go
type ModemControlPinWaiter interface {
	WaitModemControlPinChange(pin ModemControlPin) error
}
```

The primitive may return without the pin having changed (a spurious
wakeup or an interrupted syscall); callers detect actual transitions
by reading the state, which the backend does after every wakeup
anyway.

`gpsio.SerialConn` forwards it as usual, and whether the underlying
terminal satisfies the capability is how the PPS source chooses
between the wait and polling backends at startup.
Whether FreeBSD has an equivalent ioctl is unresolved (to be settled
before that platform is claimed; if it has none, FreeBSD uses the
polling backend).

Shutdown for this backend differs from every other goroutine in the
daemon and must be called out explicitly at the wiring point: the wait
cannot be interrupted portably (`TIOCMIWAIT` has no timeout, and
closing the descriptor is not guaranteed to wake it). While pulses are
arriving, the goroutine wakes within a second and observes the stop
flag; if the pulse has stopped, the goroutine may remain blocked in
the ioctl until process exit. That is accepted and harmless -- it must
not later be "fixed" as if it were a leak. `EINTR` from the ioctl is
retried. (On Windows, `SetCommMask` aborts a pending `WaitCommEvent`,
so an explicit unblock is available there.)

### D2XX on macOS (dropped)

FTDI's proprietary D2XX library delivers modem status change
notifications, and a wait-style macOS backend on it was built and
measured (the parked `serial-d2xx` branch). It was dropped on the
2026-08-08 measurements: with the loop constants sized for the
delivery tail, polling matches or beats it on the same hardware --
D2XX gave interval sd ~60 us on the FT232R against polling's ~72 us,
and sd ~80 us on the FT232H against polling's ~50 us, at equal ~1%
CPU. The implementation also had to absorb two library defects:
`FT_Read` implements its read timeout as a busy loop (a reader waiting
in it spins a full core), and the event condition variable wakes only
one waiter per event, forcing a single event-pump goroutine to fan
wakeups out to the data reader and the pin waiter. A proprietary
dlopen'd dependency plus that machinery, for no measured gain over
polling, is not worth shipping. If a machine class reappears where
polling is genuinely capped (the 2 ms-ioctl regime), the branch
records a working implementation and the measurements to revisit.

## Interface changes

`term` gets a device-independent replacement for the current raw
`ModemStatus`/`MODEM_*` API (which is removed; its only consumer,
`cmd/pollpps`, is updated):

```go
// ModemControlPin identifies a modem control pin that is an input
// to the host.
type ModemControlPin int

const (
	ModemCTS ModemControlPin = iota
	ModemDCD
	ModemDSR
	ModemRI
)

// ModemControlPinState is the set of modem control input pins that
// are asserted.
type ModemControlPinState int

func (s ModemControlPinState) Asserted(p ModemControlPin) bool

// ModemControlPinState is a method of the Term interface.
ModemControlPinState() (ModemControlPinState, error)
```

Each platform fills the state from its native call (`TIOCMGET`;
`GetCommModemStatus` on Windows, which reports exactly these four input
pins). `gpsio.SerialConn` exposes the same interface, re-exporting the
`term` types, with an error when the connection is not TTY-backed; the
daemon layer imports only `gpsio`. `WaitModemControlPinChange` (see
the Wait section) is a further capability interface rather than a
`Term` method; it arrives with the first wait-capable device backend.

## Daemon wiring

One new goroutine: the edge detector. It runs the chosen backend
(polling or wait) against the same `gpsio.SerialConn` the scan worker
is reading from, and delivers each detected edge's timestamp on a
channel. The polling variant locks its OS thread and observes
cancellation each time it wakes, which is at worst one slow polling
spacing; the wait variant's shutdown is as described above. The daemon
stops the edge detector before closing the serial connection.

Everything downstream reuses existing machinery, with no further
goroutines:

- The dispatcher's select loop gains a case for the edge channel,
  alongside the packet channel -- the same shape as its consumption of
  PHC timestamp events today.
- A generator, a plain struct called only from the dispatcher goroutine
  (so unsynchronized), receives both streams. It is the `MsgUTCTimer`
  sink, retaining recent (utc, tRead) pairs and the leap indication;
  on each edge it identifies the second, computes the sample, and hands
  it to the existing refclock proxy channel, where the existing
  refclock worker goroutine performs the SOCK/SHM writes.
- Message-arrival sampling is disabled simply by the `MsgUTCTimer` sink
  being this generator rather than the current direct sampler.
- The generator fires the same observer hook the current sampler fires
  (`obs.Observer.NTPSample`), so the offset remains observable; #274
  reworks that hook's shape and scope, and applies here as it does to
  message-arrival sampling.

The edge backends and the generator form a new application-layer
package, `time/internal/serialpps`; the wiring lives in
`time/app/daemon` as usual.

## Testing

The generator gets table-driven unit tests covering second
identification, the staleness rule, and leap passthrough, using
synthetic messages and edges. The edge backends are validated on real
hardware per the phasing below.

## Phasing

1. macOS end to end: the `term`/`gpsio` interface above, the polling
   backend, sample generation wired into the daemon as described under
   Configuration, validated against chrony on real hardware
   (done 2026-08-08, with an FT232R and an FT232H).
2. The wait-consuming edge backend in `serialpps` with the
   `TIOCMIWAIT` and `WaitCommEvent` implementations of the wait
   primitive for Linux and Windows. (A D2XX-backed macOS provider was
   built as the originally planned phase 2 and dropped; see "D2XX on
   macOS" above.)
