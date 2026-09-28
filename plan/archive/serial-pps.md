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
  enough to identify which second a pulse belongs to: a non-prepulse
  message is emitted after its pulse, and its measured delay is checked
  against configurable causal bounds.

Experiments on 2026-08-08 with the same FT232R and an FT232H
(high-speed USB) on both machines, measured directly with
`pollpps --ioctltime`, located the `TIOCMGET` cost precisely: it is the
host controller's handling of full-speed transactions, and only that.
The median read is ~135 us for the FT232R on the desktop Mac (any
topology) but ~2 ms on the MacBook Air when attached directly; behind a
high-speed hub the Air improves to ~283 us, because the hub's
transaction translator runs the full-speed leg itself and the host's
side stays high-speed. The FT232H, natively high-speed, measures
~95-130 us on both machines regardless of topology -- which is the real
argument for a high-speed adapter at 1 Hz. Drivers were ruled out:
Apple's `AppleUSBFTDI` and FTDI's VCP dext measure identically on both
chips. (The FTDI dext, if installed, binds every FTDI adapter alongside
Apple's driver and creates a duplicate tty whose name collides with the
serial-number name; deactivate it, or device paths become unstable.)
Nothing in the design depends on which regime holds -- the loop
calibrates itself to either -- but the same-day findings that do
generalize are:

- Edge delivery has a millisecond-scale late tail (a small fraction of
  edges arrive up to ~1 ms late), which on a fast-ioctl machine makes
  the needed window far wider than the bracket width; the window must
  absorb that tail (a sweep on the FT232R with a fixed window of c
  bracket widths: c = 4 caught 68% of pulses, c = 10 99%, c = 16 100%,
  with CPU flat at ~1% across c = 7..16).
- Phase 1 was validated end to end on that machine: chrony selects the
  SOCK refclock and tracks it with ~100 us error bounds; per-sample
  offsets have sd ~70-100 us, and successive median-filtered estimates
  wander by ~40 us regardless of filter length.

The adaptive-window rework (settled window tracked additively at a
target miss rate of 1/k) was validated on Linux on 2026-08-09, with
an FT232R and an FT232H on `ftdi_sio`, on a host synced to a LAN
stratum-1. `TIOCMGET` is far cheaper than on the macOS full-speed
path: median ~110 us on the FT232R, ~40 us on the FT232H. Long
runs of the loop against an otherwise idle port: the FT232R over
40 minutes published 2319 edges with 29 isolated single-pulse
misses (about 1 in 80), no signal-loss restarts, an equilibrium
cost of ~4 state queries per pulse, and offsets of mean +54 us,
sd ~62 us; the FT232H over 30 minutes published 1771 edges with 13
misses (about 1 in 137), ~7 queries per pulse, and offset sd
~230 us on that receiver's pulse. Two hardware behaviours the loop
absorbed without tuning: the FT232H delivers modem-status changes
in ~1.2 ms steps regardless of its ~40 us ioctl, and in the daemon,
with the scan worker reading the same port, brackets on both
adapters settle around 1.2-1.4 ms rather than the idle-port floor.

On 2026-08-10, per-catch diagnostics located the cause of those
wide in-daemon brackets and a daemon-only failure of the original
settling latch. Draining the port and traffic on a second adapter
were both ruled out by measurement (`TIOCMGET` stays at ~110 us
with a concurrent reader, and a lone adapter reproduced the
effect); the cause is that the daemon's sleeps overshoot by
0.3-1 ms, against microseconds in a bare test process, so bracket
widths plateau near 1.2 ms while the loop is still sleep-paced.
The original latch -- settle at the first catch whose bracket does
not improve on the previous one -- misfired on that noise inside
that plateau. With the latch reworked to observe pacing directly,
the same daemon and FT232R settle in ~9 s at the ~110 us bracket
floor and hover at a ~1.8 ms window: 8-10 state queries per pulse
and offsets within ~150 us, in the daemon, matching the idle-port
loop.

The Linux wait backend was measured on two full-speed FTDI adapters
(`ftdi_sio`), on a host synced to a LAN stratum-1 within tens of
nanoseconds so the absolute sample error is visible: one sample per
pulse with no gaps, a bias of about -200 us, and per-sample jitter of
sd ~90 us, the same band as the polling backend on macOS. The bias is
one-sided because the wait primitive timestamps the wakeup, which is
strictly after the edge, so the full-speed USB delivery of the
modem-status change appears wholesale; the polling backend's midpoint
estimator cancels most of its own latency instead. Around 200 us is
characteristic of full-speed FTDI adapters on Linux; compensate with
the chrony refclock `offset` option (`offset 200e-6`).

The Windows clocks were measured on 2026-08-10 (Windows 11, an FT232R
on the FTDI VCP driver) before wiring the backend to them. time.Now's
wall reading advances in ~523 us steps on this machine, 99% of
consecutive readings being equal, while `GetSystemTimePreciseAsFileTime`
resolves its 100 ns representation at ~57 ns per read with no backwards
step over 10^6 reads; the two read the same system clock, the coarse
one lagging by 8-368 us, always within one quantum. `GetCommModemStatus`
answers in ~1.5 us median (p99 2.9 us, max 15.5 us) -- far too fast for
a USB round trip, so the VCP driver answers from its cached modem
status: the polling loop brackets the cache flip and the status-delivery
cadence appears as edge scatter for the window to learn. The same 2000
calls timed with time.Now report min=0 median=0 p90=0: the earlier `-i`
distributions on Windows were clock quantization, not measurement.

What paces the settled loop here is neither the state read nor the 50 us
spacing floor, but the timer. Measured on 2026-08-14: the monotonic
component of `time.Now` has the same ~524 us granularity as its wall
component, and a timer set for 50 us fires at a median 523.8 us, one set
for 200 us at 525.9 us, one for 1 ms at 1.527 ms -- all landing on that
same quantum, because Windows cannot sleep for less than a tick. A state
read of ~11 us therefore always leaves time on the clock, so every poll
sleeps, every sleep costs a tick, and the floor is unreachable. The
achieved bracket is ~524 us and the published uncertainty half of it,
which is what an FT232H measured: 262, 281, 268 us. Windows polling
resolution is floored by the OS at half a tick, and only the wait
backend, which blocks on an event rather than sleeping, escapes it.
Because every poll sleeps, the settled latch's query-paced confirmation
cannot fire on Windows either; it settles through the spacing-floor
branch, which is what that branch is for. With the wait
backend wired to the two clocks, the same adapter delivered one edge
per second with no misses and the published timestamps resolved to
100 ns, scattering a few hundred us per edge (delivery jitter, no
longer clock quantization); the polling backend's own poll-duration
statistics on the wired port report a median of 1.1 us.

## Configuration

One new key in the `[serial]` table enables the physical PPS source:

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

The bounds used to associate messages with pulses are configured
separately from the physical serial source and have these defaults:

```toml
[sample.serial.pps]
delayUncertainty = 0.005
maxDelay = 0.8
```

`delayUncertainty` allows the inferred delay to be slightly negative
because the relative pulse and message timestamps have measurement
uncertainty; it is not a physically negative message delay. `maxDelay`
is the maximum credible delay from a pulse to its post-pulse message.
The sum must be less than one second, ensuring that at most one integer
second can satisfy the interval.

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

A detected edge becomes one refclock sample. Both backends deliver the
edge as two readings of the same instant, wall and mono (see the Wait
section for why they can be distinct clocks). Labelling uses the mono
reading and the adjusted first-byte message timestamp `tRead`:
advancing the message UTC by `mono - tRead` places the edge on the
message's UTC timescale without consulting the system wall clock. For
each nearby integer second the generator infers the corresponding
post-pulse message delay and accepts the unique label satisfying

```text
-delayUncertainty <= inferredDelay < maxDelay
```

The lower allowance represents measurement uncertainty; the physical
message is guaranteed to be emitted after its pulse. If no label
satisfies the interval, the edge produces no sample. The sample's
system time is the edge's wall reading, the offset is the accepted
reference time minus it, and the leap indication comes from the time
messages, as in the existing message-based sampling. If the newest
time message is more than 3 s old, the edge also produces no sample.
Emission uses the existing refclock path (SOCK and/or SHM); the fixed
SHM precision reported in this mode is 2^-11 s (~500 us), not the 2^-1 s
used for message-arrival sampling.

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
plus exactly 1 s, in the monotonic clock) and a poll window of
width W centered on P. Each second the loop polls the window at a
target spacing of W/N, floored at 50 us. It sleeps until each
poll's scheduled time; when the state query takes longer than the
target spacing the sleep never fires and the calls pace the loop by
themselves. The query's duration is never measured or assumed.

- Catch: two consecutive polls bracket the transition (the earlier
  out of pulse, the later in pulse). The edge timestamp is the
  bracket's midpoint, and the bracket's width -- the time between
  its two polls -- is the measurement resolution. P becomes
  edge + 1 s. A bracket a full period or more wide may span
  several leading edges, so its midpoint identifies none of them;
  it counts as a miss.
- Miss: the window closes without a transition. P advances 1 s. A
  pulse already in progress when the window opens is not a miss:
  the windows advance in lockstep with the pulses, so aborting
  would reopen at the same phase every period and never acquire.
  The loop polls through the pulse and resumes the search for the
  next leading edge on its far side.

The window moves through two regimes. While halving, each catch
halves the window (floored at two bracket widths); from cold it
is the whole period, polled uniformly, and a miss leaves it alone
(the prediction may simply still be coarse). Halving ends at its
floor, or at the first miss after the settled latch (below) is
set -- the halving overshot the edge scatter -- and the window is
then tracked additively: a miss widens it by a bracket width at
each end, and every k-th consecutive catch narrows it by the
same. At equilibrium the window hovers just above the observed
edge scatter, missing about one pulse in k; misses are cheap (the
NTP daemon's filtering absorbs a lost second), so they are the
probe that keeps the window, and with it the steady-state cost --
roughly the number of bracket widths in half the window, in state
queries per second -- at the minimum the scatter permits. A stall
or delivery-tail miss costs one widening step, not the lock.
Every caught edge (with its bracket, prediction offset, and poll
lateness) and every window size change is logged at debug level,
so settling, the equilibrium, and the miss rate are visible on a
long run.

In either regime, missLimit consecutive misses mean the pulse is
gone: the window returns to the whole period in one step and a
genuinely new settling period begins. At full size, consecutive
windows tile the period exactly, so the poll grid is advanced by
a fixed fraction of the spacing each period; a locked grid would
otherwise revisit the same phases every period and could
indefinitely straddle a pulse narrower than the spacing, since
clock drift alone sweeps phase far too slowly.

Constants: k = 60, the target miss rate, chosen against what
chrony tolerates rather than against any hardware; N = 64, the
initial polls per second, which sets the cold-start spacing of
~16 ms and is nearly inert once settled, when the query time or
the spacing floor paces the loop; missLimit = 10, how long a
stopped pulse is waited on before cold restart; spacing floor
50 us, a CPU bound
for very fast state queries. None encodes an adapter's timing:
the former safety factor c, sized from the measured FT232R
delivery tail, is gone -- the tail is learned as equilibrium
growth instead.

Publishing is gated by a latched settled state, meaning the
polling schedule no longer controls resolution. The loop observes
that condition directly from its pacing rather than inferring it
from bracket measurements. A catch settles immediately when the
spacing target sits at the 50 us floor, which halving no longer
changes. Otherwise two consecutive caught windows must be polled
without a single sleep firing; since each catch halves the window,
the second confirms at a smaller spacing that the state queries,
not the target spacing, pace the loop. A sleep-paced catch or a
miss clears the confirmation, so a transient run of slow queries
cannot open the gate. Bracket widths play no part in the latch:
sleep overshoot stretches them while the loop is sleep-paced
(measured at 0.3-1 ms inside the daemon, against microseconds in a
bare test process), and the earlier latch -- settle at the first
catch whose bracket does not improve on the previous one --
misfired on that noise, publishing millisecond-class samples from
a still-wide window. The flag clears only on the cold restart
(signal loss, hence a genuinely new settling period). The point of
the suppression is only to withhold the
uncharacteristically bad cold-start samples: after the latch every
catch is published, however coarse, because a sample stretched by an
oversleep is characteristic of what that system delivers, and the
NTP daemon's own filtering is the right place to handle such
outliers. Chrony has no notion of an initial settling period, so the
loop provides one.

The bracket needs a poll inside the pulse. Once settled, the
achieved poll interval is paced by the state query or the spacing
floor, so tracking only needs the pulse to be wider than one
achieved poll interval. Acquisition
polls at period/N, about 16 ms: pulses at least that wide are
caught deterministically, narrower ones (e.g. Septentrio's 5 ms
default) are found by the phase sweep at full window size,
stretching acquisition by roughly spacing/width. Microsecond-width
pulses remain unsupported by this backend.

The polling goroutine locks its OS thread.

### Wait (phase 2: Linux, Windows)

Linux `TIOCMIWAIT` blocks until a chosen modem control line changes;
Windows has the equivalent `SetCommMask`/`WaitCommEvent`. Neither
reports a timestamp, so the primitive timestamps the wakeup before
any other call and then reads the line state. A state read can itself
be a millisecond-scale USB round trip. `Wall`, the most precise
system-time reading the platform offers, becomes the published sample
time, while `Mono`, an ordinary `time.Now` reading, serves elapsed-time
arithmetic against message read times. On Unix one `time.Now` reading
serves both. On Windows `time.Now` is quantized to the shared clock
page (~0.5 ms measured), while `GetSystemTimePreciseAsFileTime` reads
to ~100 ns but carries no monotonic component, so the Windows backend
takes the two readings back-to-back.

`term` exposes a synchronous primitive. A watch owns its own claim on
the port, remains usable after its `Term` closes, and contains no
goroutine or logging:

```go
type ModemControlPinChange struct {
	Wall, Mono time.Time
	Asserted   bool
}

type ModemControlPinWatch interface {
	Wait() (change ModemControlPinChange, missed int, err error)
	Cancel()
	Close() error
}

type ModemControlPinWatcher interface {
	NewModemControlPinWatch(pin ModemControlPin) (ModemControlPinWatch, error)
}
```

`Cancel` is sticky: after it fires, every `Wait` returns
`ErrCancelled`. It ends a pending wait as promptly as the platform
allows. `Close` releases that claim and must not overlap `Wait`.

The Linux backend uses `TIOCGICOUNT` to attribute a wakeup. It reads
the watched pin's counter before arming `TIOCMIWAIT`, timestamps the
wakeup, reads the pin sense, and reads the counter again. A delta of
one is an unambiguous transition and produces a `ModemControlPinChange`; any other
delta is withheld and carried into `missed` before the wait is
re-armed. Counter changes that occurred while no wait was armed are
also included in `missed`. There is an inherent window between the
pre-arm counter read and the ioctl's entry snapshot: an edge there can
inflate the delta, occasionally withholding a clean wakeup. This fails
safe and recovers when the wait is re-armed. If `TIOCGICOUNT` is not
supported, the backend degrades to its best post-wakeup state reading,
never withholds a wakeup, and always reports zero missed transitions.
`missed` is a diagnostic lower bound, not a total, and classification
never depends on it.

`gpsio.SerialConn` owns the concurrency. For each call it runs the
synchronous `ModemControlPinWatch.Wait` on a goroutine and selects its result
against the caller's context. Cancellation calls the watch's sticky
`Cancel` and returns the context error. On Linux nothing can end
`TIOCMIWAIT` itself: it has no timeout, closing the descriptor does not
wake it, and runtime signals are installed with `SA_RESTART`. The
cancelled goroutine may therefore remain parked until the next pin
change or process exit. On a late wake it observes `ErrCancelled`,
closes the watch, and exits. Until then the private dup keeps the
port's flock held, but it cannot touch a descriptor number reused
after `SerialConn.Close`; process exit releases it in the no-wakeup
case. `SerialConn.Stop` also cancels the watch.

On Linux every tty satisfies the capability interface, but support
still depends on its driver. There is no nonblocking probe, so a
driver without `TIOCMIWAIT` fails the first wait with `ENOTTY`, which
`term` maps to `errors.ErrUnsupported`; the detector then closes the
watch and falls back to polling. Whether FreeBSD has an equivalent
ioctl remains unresolved.

The Windows backend waits in `WaitCommEvent` on the event selected by
`SetCommMask`. Windows has no transition counter, so it is the
degraded mode the Linux backend falls into without `TIOCGICOUNT`:
nothing is withheld, `missed` is always zero, and `Asserted` is the
best reading `GetCommModemStatus` gives after the wakeup. The mask is
installed once, by `NewPinWatch`, because `SetCommMask` resets the
driver's event history: setting it before every wait would create a
window in which a transition between successive waits could be lost,
whereas setting it once lets an event arriving while no wait is armed
complete the next `WaitCommEvent` immediately. A driver that rejects a
valid pin's event mask as unsupported triggers the polling fallback.

Cancellation is prompt, which is what Linux cannot manage: `Cancel`
sets the sticky flag and then calls `SetCommMask(0)`, which
`IOCTL_SERIAL_SET_WAIT_MASK` documents as completing a pending
wait-on-mask request immediately with a zero event mask. That this
works across the watch's duplicated handle follows from the object
model rather than from the serial documentation: a duplicate refers to
the same object as the original, and a driver receives IRPs bearing a
file object, never a handle, so the wait mask, the event history and
the pending wait are one piece of state that either handle reaches.
The same sharing means the watch's mask is also the main handle's
mask; that is harmless, since nothing else calls `WaitCommEvent` and
the mask affects nothing else. `Close` closes the duplicate; the file
object survives until both handles are closed, so an abandoned wait
is undisturbed by `SerialConn.Close` and cannot touch a handle value
reused after it.

The COM handle is opened with `FILE_FLAG_OVERLAPPED`, and `ReadFile`,
`WriteFile`, and `WaitCommEvent` are all performed as overlapped
operations, the duplicate inheriting the file object's asynchronous
mode. This is required for the event wait and the scan reader to
operate concurrently: with a synchronous handle an FT232R produced only
the first two CTS edges and the scan reader stopped receiving, whereas
with overlapped I/O the same daemon run produced 91 consecutive edges
and 90 serial-PPS samples over 90 seconds. It is also what lets a
cancelling `SetCommMask` reach the driver while the wait is pending,
since an asynchronous file object does not serialize requests the way a
synchronous one does. An earlier implementation cancelled with
`CancelIoEx` first, because a `SetCommMask` had been seen to queue
behind the pending wait and deadlock shutdown; that was the file
object's synchronous mode, not the FTDI driver. Measured on 2026-08-14
with an FT232H on COM4: a wait armed on `RI`, which never changes,
stayed pending for the whole 5-second run and the process exited
0.23 s past its timeout, so `SetCommMask` alone releases the wait and
`CancelIoEx` is not needed.

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
daemon layer imports only `gpsio`. `ModemControlPinWatcher` (see the
Wait section) is a further capability interface rather than a `Term`
method; `gpsio.SerialConn` builds its ctx-based
`WaitModemControlPinChange` on it.

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
synthetic messages and edges. The polling loop runs under
`testing/synctest` against a simulated pulse source with configurable
query duration and transient slowdowns, pulse width, delivery delay,
and outages, so
settling, steady-state poll cost, and miss handling are checked
deterministically. The source also models the daemon's sleep
overshoot (jittered and stalled wakeups), pinning the settled
latch to confirmed query pacing rather than the bracket noise or
a single slow-query burst. These do not run on Windows: `now` reads the
system clock through a syscall no bubble can fake, so the loop would
predict edges on the real clock while sleeping on the fake one. Rather
than shape the clock around the tests, the bubble tests are skipped
there and the loop is covered by the Linux and macOS runs. The edge
backends are also validated on real hardware per the phasing below.

## Phasing

1. macOS end to end: the `term`/`gpsio` interface above, the polling
   backend, sample generation wired into the daemon as described under
   Configuration, validated against chrony on real hardware
   (done 2026-08-08, with an FT232R and an FT232H).
2. The wait-consuming edge backend in `serialpps` with the
   `TIOCMIWAIT` and `WaitCommEvent` implementations of the wait
   primitive for Linux and Windows; the polling backend remains as
   the fallback for tty drivers without the ioctl. (A D2XX-backed
   macOS provider was built as the originally planned phase 2 and
   dropped; see "D2XX on macOS" above.) (done 2026-08-10; the Windows
   backend was validated with CTS on an FT232R, including cancellation
   while `WaitCommEvent` was pending and a 90-second continuous daemon
   run using overlapped serial I/O. That backend was then reworked onto
   `PinWatch` -- a duplicated handle, a mask installed once by
   `NewPinWatch`, and `SetCommMask` cancellation in place of
   `CancelIoEx` -- and revalidated on 2026-08-14 with an FT232H on
   COM4: 90 edges over 90 seconds with no missed transitions, prompt
   cancellation of a wait pending on an idle pin, and the polling
   fallback settling to a 0.26 ms bracket.)
