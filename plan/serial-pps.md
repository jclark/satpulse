# Serial PPS (#259)

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
(`cmd/pollpps` on the `serial-pps-tmp` branch) established:

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

## Configuration

One new key in the `[serial]` table:

```toml
[serial]
device = "/dev/cu.usbserial-XXXXXXXX"
speed = 38400
pps.line = "cts"
```

`pps.line` names the modem control line carrying the PPS signal; its
presence enables PPS detection. Any of the four input lines is
accepted: `"cts"`, `"dcd"`, `"dsr"`, `"ri"`. CTS is the recommended and
tested wiring; DCD is the traditional serial-PPS line but many USB
serial drivers do not surface it.
The pulse's leading edge is assumed to be electrically rising, which on
a TTL-level adapter is observed as the CTS flag becoming deasserted; a
`pps.edge` key can be added later if inverted pulses turn out to matter.

The serial device must be a real TTY; configuring `pps.line` on anything
else (FIFO, socket) is a startup error.
`pps.line` and `phc.interface` are mutually exclusive: the PHC is the
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
	Line string `toml:"line"`
}
```

When `pps.line` is configured, refclock samples come from PPS edges, and
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
reported in this mode is 2^-9 s (~2 ms), not the 2^-1 s used for
message-arrival sampling.

With the polling backend, edges found while the pulse phase is unknown
(see below) do not produce samples; they are known too coarsely and
would inject bad data points. So the first pulse at startup, and the
first after an outage, yields no sample; every pulse in steady
operation yields one. The wait backend has no such restriction: every
detected leading edge yields a sample. Sawtooth correction is
irrelevant at this precision and is not used.

## Edge detection

Two backends behind one interface, selected by platform. For both, the
implemented rule is: the pulse's leading edge is the configured line's
flag becoming deasserted (per the TTL-level assumption stated under
Configuration), and that is the only transition that is timestamped.

### Polling (phase 1: macOS; also any platform without a wait primitive)

Adaptive polling, driven by the fact that consecutive pulses are 1 s
apart:

- While the pulse phase is unknown (startup, or the pulse stopped),
  poll slowly -- tens of ms apart -- purely to find the approximate
  phase.
- Once the next pulse's time can be predicted, sleep until shortly
  before it, then poll repeatedly until the line changes or the
  expected time has passed by a sufficient margin. The edge timestamp
  is the midpoint of the two polls that straddle the transition.
- A minimum spacing between polls is enforced by sleeping, so the loop
  is well behaved on drivers that answer from a cached value instead of
  blocking; on hardware like the FT232R the ioctl's own blocking
  exceeds the minimum and the sleep never fires.
- Missed pulses widen the margin and eventually drop back to slow
  polling.

The tuning values are internal constants, at least initially; there is
no configuration for them. Initial values, to be revised against real
hardware:

- Slow polling spacing: 100 ms. An edge found this way is known to
  within the spacing, which is enough to place the next window.
- Prediction: previous edge time plus exactly 1 s, in the monotonic
  clock. No period estimation: the clock's rate error (ppm-level) is
  orders of magnitude below the window margin, so estimating the
  inter-pulse period buys nothing.
- Window margin: open 5 ms before the predicted edge; a miss is
  declared 5 ms after it. The first window after a slow-poll detection
  opens earlier and closes later by half the slow polling spacing,
  reflecting the coarser phase knowledge.
- If the line is already in its pulse-asserted state when the window
  opens, that also counts as a miss.
- On each miss the margin doubles and the prediction advances 1 s;
  when the margin would exceed 100 ms, drop back to slow polling.
- Minimum spacing between polls: 200 us.

The selected flag's deasserted interval must overlap at least one poll.
In tracking this means a practical minimum pulse width of roughly one
modem-state query interval (about 2 ms on the tested FT232R). Slow phase
acquisition samples at 100 ms spacing, so narrow pulses can take much
longer to acquire. The u-blox default width of 100 ms works well;
microsecond-width pulses are not supported by this backend.

The polling goroutine locks its OS thread.

### Wait (phase 2 via D2XX on macOS; phase 3: Linux, Windows)

Linux `TIOCMIWAIT` blocks until a chosen modem control line changes;
Windows has the equivalent `SetCommMask`/`WaitCommEvent`. Neither
reports the transition's direction nor a timestamp, so on each wakeup
the backend takes the timestamp immediately -- before any further
calls, since on hardware like the FT232R a state read is itself a
~2 ms round trip -- and then reads the line state to classify the
transition. Every wakeup where the line has entered its pulse-asserted
sense produces a sample, subject only to identifying the second from
the time messages. Unlike the polling backend, nothing here relies on
the pulses being 1 s apart.

`term` gains a primitive that only waits; reading the state afterwards
is the existing method. Since only some terminals can wait, it is a
capability interface that the PPS source asserts for, not a method of
`Term`:

```go
type ModemControlLineWaiter interface {
	WaitModemControlLineChange(line ModemControlLine) error
	CancelModemControlLineWait()
}
```

The cancel exists so that shutdown is prompt on backends that can abort
a pending wait; where nothing can interrupt it, it does nothing and the
shutdown behaviour below applies.

The primitive may return without the line having changed (a spurious
wakeup, an interrupted syscall, or -- on D2XX -- an event for another
line or for received data); callers detect actual transitions by
reading the state, which the backend does after every wakeup anyway.

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

### D2XX on macOS

FTDI's proprietary D2XX library delivers modem status change
notifications, giving macOS a wait-style backend after all -- but only
for FTDI adapters, and only when the user has installed the library.
Measured on the FT232R (2026-08-06, 2.5 minute run): edge intervals
with sd 85 us, 90% within +/-140 us, worst deviation 262 us, ~1% CPU,
no missed pulses -- roughly 5-10x better than the polling backend can
achieve, with none of the adaptive machinery. Longer-run stability, an
FT232H (high-speed USB, so possibly tighter still), and the absolute
latency offset (needs a calibrated reference) remain to be measured.

The library is loaded dynamically (dlopen), so binaries have no
mandatory dependency on it; it coexists with the Apple VCP driver (no
driver removal needed, unlike Linux). The
definitions derived from FTDI's headers (constants, struct layouts)
are generated with `cgo -godefs` and the generated file committed, so
the FTDI headers are needed only when regenerating, not at build time;
the runtime implementation is entirely in Go. The `go:generate`
directive invokes `generate_ftd2xx.sh`; `FTD2XX_DIR` supplies the
directory containing the headers at regeneration time, so no local
include path is embedded in the source or generated output. The
build-ignored `ftd2xx_defs.go` is input to `cgo -godefs`, while the
committed `zftd2xx_defs_darwin.go` is the normal Go build input. There
are no `.c` files and normal builds do not invoke cgo.

FTDI and pthread symbols are called through `purego`. Because `purego`
represents C `void *` parameters as `unsafe.Pointer`, D2XX handles and
C-allocated addresses use that type; direct unsafe operations are
confined to small helpers for applying generated field offsets and
reading a bounded C character array. The generated `EVENT_HANDLE`
layout is allocated with `calloc`. A C toolchain and the FTDI headers
are needed only to regenerate the definitions; cgo-disabled builds
include the D2XX backend as well. The `cmd/pollpps --d2xx` mode is the
measurement diagnostic.

Structurally this is a new kind of device in `term`, not in `gpsio`,
and not a separate package: build-tagged (`darwin`) files inside
`gps/lib/term`, parallel to the existing platform files. It is a peer
implementation of the `Term` interface, plus the
`ModemControlLineWaiter` capability that the termios terminals do not
provide, selected at open time on Darwin when the device is an FTDI
adapter and the library is present, with the termios path as fallback.
Everything from `gpsio` up remains D2XX-agnostic: `gpsio` forwards the
generic wait primitive, and the PPS source selects it through the same
capability assertion intended for Linux/Windows. The D2XX
backend must own the data stream as well as the PPS line: the receive
queue has to be drained promptly (a full queue degrades event
delivery -- measured as event timing collapsing after ~40 s), and the
drained bytes are the receiver's output, so data reads go through
D2XX (`FT_Read`) rather than the tty. In satpulsed the scan worker's
ordinary reads are the draining; the wait path never touches data. An
explicit drain exists only in the standalone diagnostic, where nothing
else reads.

D2XX is deliberately not used on other platforms: on Linux it would
require detaching `ftdi_sio` and duplicates what `TIOCMIWAIT` provides
for any serial driver; on Windows `WaitCommEvent` already goes through
FTDI's own driver stack.

## Interface changes

`term` gets a device-independent replacement for the current raw
`ModemStatus`/`MODEM_*` API (which is removed; its only consumer,
`cmd/pollpps`, is updated):

```go
// ModemControlLine identifies a modem control line that is an input
// to the host.
type ModemControlLine int

const (
	ModemCTS ModemControlLine = iota
	ModemDCD
	ModemDSR
	ModemRI
)

// ModemControlLineState is the set of modem control input lines that
// are asserted.
type ModemControlLineState int

func (s ModemControlLineState) Asserted(l ModemControlLine) bool

// ModemControlLineState is a method of the Term interface.
ModemControlLineState() (ModemControlLineState, error)
```

Each platform fills the state from its native call (`TIOCMGET`;
`GetCommModemStatus` on Windows, which reports exactly these four input
lines). `gpsio.SerialConn` exposes the same interface, re-exporting the
`term` types, with an error when the connection is not TTY-backed; the
daemon layer imports only `gpsio`. `WaitModemControlLineChange` (see
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
   Configuration, validated against chrony on real hardware (F9P +
   FT232R).
2. The D2XX-backed `term` device for macOS, together with the
   wait-consuming edge backend in `serialpps` (D2XX is its first
   provider); the polling backend remains as the fallback for non-FTDI
   adapters or when the library is absent.
3. The `TIOCMIWAIT` and `WaitCommEvent` implementations of the wait
   primitive for Linux and Windows.
