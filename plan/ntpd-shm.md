# NTP SHM refclock (#300)

## Introduction

This plan adds support for the classic ntpd/NTPsec SHM (driver 28)
refclock interface as a second way -- alongside the existing chrony
SOCK refclock -- for satpulsed to deliver GNSS UTC time samples to a
local NTP daemon.

Conceptual model:

- Chrony SOCK: satpulsed -> Unix datagram socket -> chronyd
- NTP SHM:    satpulsed -> SysV shared memory segment -> ntpd polls

Scope: time-of-day / seconds-numbering only. PPS is *not* provided by
satpulsed; users wanting sub-second discipline are expected to
configure a separate PPS/ATOM refclock in ntpd. The SHM segment
carries one (clockTimeStamp, receiveTimeStamp) pair per sample,
meaning "at local time R the GNSS receiver reported UTC time C".

This refclock is a *peer* of the existing chrony SOCK refclock, not a
replacement. Both may be configured simultaneously (in different
deployments; nothing prevents both being present in one file, though
it is unusual). The two paths share the dispatcher's sample
construction logic; only the publication transport differs.

## Architecture changes

There are two current call sites that produce a refclock sample,
both in `time/internal/gpsevent/dispatcher.go`:

- `Dispatcher.sysSample` -- PHC tracking mode, called from
  `timestamp()` on each PPS edge once `phcsync.Controller` is in
  `ModeTracking`. Carries `(ref ptime.Time, sys time.Time)` with both
  true-time and local-receive timestamps available.
- `Dispatcher.MsgUTCTime` -- serial timing mode (no PHC), called from
  `timemsg.Buffer` when a fresh UTC time message arrives. Carries
  `(utc time.Time, tRead time.Time)` directly.

The existing SOCK pipeline routes both sites through a
`refclock.ProxyRefClock` -> channel -> `refclock.RefClockWorker`
goroutine, because socket writes can block (timeouts, AppArmor,
unwritable peer). SHM publication does not need that: writes are a
handful of memory stores plus two atomic ops, never block, and have
no IO error mode that retries help with. We therefore call the SHM
writer **synchronously** from the dispatcher.

Concretely:

- Add two new fields on `Dispatcher`:
  - `shm shmSink` (may be nil) -- the SHM writer, accessed through
    an unexported interface declared in `gpsevent`:
    ```go
    type shmSink interface {
        Write(clockTime, receiveTime time.Time, leap ptime.LeapSecondKind, precision int8)
        Close() error
    }
    ```
    `*ntpshm.Writer` satisfies it; tests substitute a fake.
  - `shmPrecision int8` -- the current precision value passed
    into `Write`. Lifecycle: if the user set
    `[ntp.shm] precision = N`, `NewDispatcher` writes that and
    the value stays put. Otherwise it depends on mode -- serial
    mode initialises to `-1` once at startup; PHC mode
    initialises lazily on the first valid PPS event and is then
    updated once after the calibration window. Details in the
    Configuration section.
  `NewDispatcher` takes the concrete writer (`*ntpshm.Writer`) and
  the optional precision override (`*int8`) as separate
  parameters, plumbed from the daemon factory:
  - `cfg.NTP.NewSHMWriter(lg)` returns `(*ntpshm.Writer, error)`
    -- the **concrete** type, following the Go convention that
    factories return concrete types and callers hold interfaces.
  - Inside `NewDispatcher`, the concrete pointer is assigned into
    the `shmSink` field only when non-nil:
    ```go
    if shm != nil {
        d.shm = shm
    }
    ```
    This is the typed-nil guard: a direct assignment of a nil
    `*ntpshm.Writer` to the `shmSink` interface field would
    produce a typed-nil interface, defeating later `d.shm != nil`
    checks and panicking in `d.shm.Write`. The nil-test on the
    concrete pointer happens *before* the conversion.
  - The precision override (a `*int8`) is passed through to
    `NewDispatcher` from the daemon. The caller must nil-guard
    the parent `*NTPSHMConfig` because it is nil when `[ntp.shm]`
    is absent from the TOML:
    ```go
    var shmPrecision *int8
    if cfg.NTP.SHM != nil {
        shmPrecision = cfg.NTP.SHM.Precision
    }
    // ... pass shmPrecision into NewDispatcher.
    ```
    `NewDispatcher` checks the resulting `*int8` (nil = no
    override) to decide whether the calibration policy runs.
  Independent of the existing `rc *refclock.ProxyRefClock` field.
- Expand the two existing "any sink configured" gates to cover SHM
  as well as SOCK:
  - `NewDispatcher` (`dispatcher.go:84`): the serial-timing UTC
    timer registration changes from `controller == nil && rc != nil`
    to `controller == nil && (rc != nil || shm != nil)`.
  - `sysSample` (`dispatcher.go:286`): the early return changes
    from `d.rc == nil || ref.IsZero() || ...` to
    `(d.rc == nil && d.shm == nil) || ref.IsZero() || ...`.
- Two call sites; in each, gate **each sink independently** and
  call them independently. SOCK takes `(sys, offset, leap)`; SHM
  takes `(clockTime, receiveTime, leap)`. The two transports
  carry the same information but in different shapes -- spell
  out each site so an implementer does not copy-paste the wrong
  arguments:

  `sysSample` widens to take the per-event precision Duration so
  the calibration policy runs inside it. The caller in `timestamp`
  (today: `d.sysSample(e.TReadWall.PHC.T, e.TReadWall.Sys)`)
  becomes `d.sysSample(e.TReadWall.PHC.T, e.TReadWall.Sys, e.Precision)`.

  ```go
  func (d *Dispatcher) sysSample(ref ptime.Time, sys time.Time, samplePrecision time.Duration) {
      // ... existing early-return gate (per the gate-expansion bullet above) ...
      d.updateShmPrecision(samplePrecision) // feeds the three-stage policy
      clockTime := d.ls.TimeToSys(ref)
      offset    := clockTime.Sub(sys).Seconds()
      leap      := d.ls.StateAt(ref).LeapTonight
      if d.rc != nil {
          if err := d.rc.Sample(sys, offset, leap); err != nil {
              d.lg.Warn("refclock sample failed", "err", err)
              // fall through; do not return -- SHM and obs still need to fire
          }
      }
      if d.shm != nil {
          d.shm.Write(clockTime, sys, leap, d.shmPrecision)   // (clockTime, receiveTime, precision)
      }
      d.obs.NTPSample(sys, offset, leap, ref)
  }
  ```

  `updateShmPrecision(d time.Duration)` is an unexported method
  encapsulating the three-stage calibration (initial estimate /
  median over `precisionCalibrationSamples` / fixed). It is a no-op
  when `[ntp.shm] precision` was explicitly configured.

  In `MsgUTCTime`, both timestamps arrive directly:
  ```go
  offset := utc.Sub(tRead).Seconds()
  if d.rc != nil {
      if err := d.rc.Sample(tRead, offset, leap); err != nil {
          d.lg.Warn("refclock sample failed", "err", err)
      }
  }
  if d.shm != nil {
      d.shm.Write(utc, tRead, leap, d.shmPrecision)   // (clockTime, receiveTime, precision)
  }
  d.obs.NTPSample(tRead, offset, leap, ptime.Time(0))
  ```

  Notes on the new structure:
  - The existing `d.obs.NTPSample(...)` call fires exactly once per
    dispatch event, regardless of which sinks are configured or
    succeeded. Today it sits behind the early-return on
    `d.rc == nil`; the rewrite lifts it out so observers see every
    sample we constructed.
  - Today `sysSample` returns early on `d.rc == nil` and
    `MsgUTCTime` calls `d.rc.Sample` unconditionally inside its
    body. Both sites need the new per-sink nil guard. A SOCK error
    must not skip the SHM write or the obs hook.
- Plumb the optional SHM writer through `NewDispatcher` /
  `gpsevent.NewDispatcher` the same way `rc` is.
- Shutdown: in `Dispatcher.Run`, the existing `defer d.rc.Close()`
  is wrapped in `if d.rc != nil { ... }` (`dispatcher.go:99-101`).
  Add a parallel `if d.shm != nil { defer func() { if err :=
  d.shm.Close(); err != nil { d.lg.Warn("SHM detach failed",
  "err", err) } }() }`. The library `Close` returns an error and
  this is the app-layer site that logs it (consistent with the
  "no logging in `time/lib/`" rule).

Relationship to issue #274 ("Allow observability of difference
between sys time and time from GPS receiver"): #274 proposes to
decouple the sys-vs-true offset *computation* from refclock
delivery, so the offset can be observed even when no refclock is
configured. That refactor will eliminate the "sink gate" pattern
this plan extends -- after #274 the offset is always computed and
observed, with sinks delivering off the same computed value. We
deliberately keep the change here mechanical (expand the existing
gate to cover SHM) rather than pre-emptively doing #274's split:
the SHM-specific complexity introduced here goes away as part of
#274 with no rework specific to SHM.

Rationale for keeping SHM separate from `RefClock` rather than
multiplexing:

- The `refclock.RefClock` interface is `Sample(sys, offset, leap)`.
  That signature is sufficient for SHM (clockTime = sys + offset),
  but the proxy + worker pattern around it is an unnecessary thread
  hop for a non-blocking writer.
- The two transports have different lifecycle and error semantics:
  SOCK may transiently fail with ENOENT/ECONNREFUSED if the peer is
  not yet up (handled by `LoggingSockRefClock`); SHM either has the
  segment or it doesn't (a one-time attach at startup).
- Treating them as separate sinks keeps the logic readable and
  avoids forcing one transport into the other's lifecycle model.

If we later add a third NTP-flavour sink, the two-sinks pattern is
trivially extensible; we should resist abstracting prematurely.

## Configuration

Extend `NTPConfig` in `time/app/daemon/config.go`:

```go
type NTPConfig struct {
    Sock *NTPSockConfig `toml:"sock"`
    SHM  *NTPSHMConfig  `toml:"shm"`
}

type NTPSHMConfig struct {
    Unit      *uint8 `toml:"unit"`
    Precision *int8  `toml:"precision"`
}
```

`Unit` is a `*uint8` so that the user can write `shm.unit = 0`
explicitly and have it take effect. (`unit` without a value, i.e. no
`[ntp.shm]` table at all, leaves SHM disabled.) The element type is
`uint8`, matching the other byte-sized fields in the same file
(`PHCConfig.Pin`, `PHCConfig.Channel`, `LeapSecondConfig.Before`,
`LeapSecondConfig.After`); pointer is needed only to distinguish
absent from `0`.

`Precision` is an optional override for the SHM `Precision` field
(`log2(seconds)`, NTP convention). When set, the explicit value is
used unconditionally. When unset, the daemon picks a path-aware
default: a fixed value in serial mode, or a lazy first-event
estimate refined once after a calibration window in PHC mode (see
"Default precision" below). `*int8` so `0` (= 1 second) can be set
distinguishably from "unset".

Example `satpulse.toml`:

```toml
[ntp]
shm.unit = 2
# shm.precision = -23  # optional; default is auto-calibrated in PHC mode
```

Validation:

- `Unit` is `0..255` (the full `uint8` range). This matches the
  natural limit imposed by the `127.127.28.u` IP-octet refclock
  address form. ntpd documents units 0..3 as examples, but there
  is no smaller hard cap in the code.
- If `Unit < 2`, the segment is created with permissions 0600 and
  the daemon must run as the same UID as the ntpd that will read it.
  Document this rather than enforce it; root is the usual case.
- If `Unit >= 2`, use permissions 0666 (matching ntpd's own choice for
  units 2/3). This is the recommended unit range when satpulsed and
  ntpd run as different users.
- `Precision` accepts the full `int8` range. ntpd documentation
  suggests values typically between `-20` (~1µs) and `0` (1s).
  No further validation -- the field is opaque to satpulsed.

Default precision (used when `Precision` is unset).

Single ownership model: the dispatcher holds `shmPrecision int8`,
written exactly once or twice over the life of the run depending on
which mode the daemon is in. The library `ntpshm.Writer.Write`
receives the current value per call but treats it as opaque -- no
policy lives in the library.

- **Serial timing mode** (`controller == nil`, `MsgUTCTime` path):
  `shmPrecision` is set once at startup to fixed `-1` (0.5 s). The
  receive timestamp is taken at UTC-message arrival; precision is
  dominated by serial-line and scheduling jitter, and we report a
  conservative bound rather than a measured value.
- **PHC tracking mode** (`controller != nil`, `sysSample` path):
  `shmPrecision` is set on the first valid PPS event by passing
  the event's precision Duration through `ntpshm.Precision`,
  updated exactly once after `precisionCalibrationSamples` events
  are collected (median of the buffered Durations -> new value),
  then fixed for the rest of the run. Details below.

If the user sets `[ntp.shm] precision = N` explicitly, both modes
use the configured value; no calibration runs.

To support the PHC path:

- `phc.MultiSample.Extract` and `phc.MultiSample.Reduce` each gain
  a third return value `time.Duration` representing the precision
  of the extracted/reduced sample. `Extract(i)` returns
  `max(ms.SysInterval(i), 5*time.Nanosecond)`. `Reduce` calls
  `Extract(Select())` and propagates the third return -- i.e.,
  the precision is the bracket interval of the selected
  (best-bracket) sample, floored at 5 ns. The 5 ns floor reflects
  PCIe PTM granularity and makes the computation uniform across
  PRECISE and EXTENDED: for PRECISE the bracket is 0 (Sys length
  1, pre == post), so the floor produces 5 ns automatically; for
  EXTENDED the floor only intervenes if the measured interval is
  implausibly small. `time/phc` therefore does not need to
  distinguish PRECISE from EXTENDED in this path. Existing callers
  of `Reduce`/`Extract` (`time/internal/ts/clock.go`,
  `time/phc/phc_cmp.go`) update to the new signatures.
- `ts.Event` gains a `Precision time.Duration` field carrying that
  value through to the dispatcher. `phctime.Sample` does **not**
  change; instead `Clock.wallSample` gains a third return value
  `time.Duration` and the call site in the event loop becomes
  `event.TReadWall, event.Precision, err = clk.wallSample()`. The
  Reduce return propagates from `wallSample` -> `event.Precision`.
- `ntpshm` exposes a helper `Precision(d time.Duration) int8`
  mapping a duration to the corresponding `log2(seconds)` exponent.
  Semantics: returns the smallest `int8` `p` such that
  `2^p >= d.Seconds()` (conservative / ceiling rounding -- a slightly
  larger reported precision under-claims rather than over-claims
  capability). Clamps to the `int8` range `[-128, 127]`. A
  nonpositive duration is treated as the smallest representable
  positive value and returns `-128`; this matches the floor enforced
  in `MultiSample.Reduce` and means the helper has well-defined
  output for any input.
- The dispatcher resolves the stream of per-event precision
  Durations into the segment's `Precision` field in three stages,
  parameterised by an unexported package constant
  `precisionCalibrationSamples` (initial value `61`, abbreviated
  `N` below):
  1. **Initial estimate** -- on the first valid event, the
     dispatcher sets `d.shmPrecision = ntpshm.Precision(d)` for
     that event's Duration `d`. The first event's own `Write`
     call publishes that value. Sample publication begins
     immediately.
  2. **Calibrated estimate** -- the dispatcher buffers the first
     `N` valid events' precision Durations as they arrive (the
     buffer includes the very first event already accounted for
     in stage 1). On event `N`, *after* buffering its Duration
     but *before* its `Write` call, the dispatcher computes the
     median of the buffered Durations and overwrites
     `d.shmPrecision` once. Event `N`'s `Write` therefore
     publishes the calibrated value.
  3. **Fixed for run** -- after event `N`, the buffer is
     released, the median is never recomputed, and
     `d.shmPrecision` is never updated again for the lifetime of
     the run.
  The constant is odd so the median is well-defined; 61 gives a
  clean median over roughly one minute at 1 Hz. The buffer is
  bounded; no statistic is recomputed per sample.

Schema (`configs/config-schema.json`): add a `shm` object under `ntp`
with two integer properties: `unit` (`minimum: 0`, `maximum: 255`)
and an optional `precision` (`minimum: -128`, `maximum: 127`).

## Platform / build tags

**Initial implementation is Linux-only.** SysV SHM is also available
on FreeBSD and macOS (with kernel-tunable size limits), but those
ABIs are not in scope here; see "Follow-ons" below.

Layering note: the package lives under `time/lib/`. Per
`docs/internals.md`, library-layer packages do not use goroutines and
do not perform logging. The writer satisfies both: writes are
synchronous and the package returns enough information through its
API for callers to log appropriately.

Build tag strategy for the initial landing:

`Writer` is exported and platform-independent. Per-platform
implementation lives behind an unexported `shmWriter` whose
definition and methods are build-tagged. The supported version
holds `*shmTime` and the syscall plumbing; the stub is an empty
struct whose `newShmWriter` returns `ErrUnsupported`.

- `time/lib/ntpshm` package contains:
  - `ntpshm.go` (no build tag) -- exported `Writer` (a thin
    wrapper around `shmWriter`), its public methods (`Write`,
    `Close` delegating to lowercase `shmWriter` methods); the
    top-level constructor `New(unit uint8)`; `Attach` return
    struct; sentinel errors (incl. `ErrUnsupported`); leap mapping;
    `Precision(time.Duration) int8` helper.
  - `types_linux.go` (`//go:build ignore`) -- input to `cgo -godefs`.
  - `ztypes_linux.go` -- generated, checked in.
  - `shm_linux.go` (`//go:build linux`) -- unexported
    `type shmWriter struct { t *shmTime; addr uintptr; ... }`
    plus `newShmWriter`, `(shmWriter).write`, `(shmWriter).close`
    via raw syscalls in `golang.org/x/sys/unix`. Carries the
    `//go:generate` line. The build tag broadens to
    `linux || freebsd || darwin` when those platforms gain
    support.
  - `shm_stub.go` (`//go:build !linux`) -- unexported
    `type shmWriter struct{}` plus `newShmWriter` returning
    `shmWriter{}, Attach{}, ErrUnsupported`; stub `write`
    (unreachable: `New` returns an error first) and `close`
    (returns nil). The build tag narrows as platforms are added.
- The dispatcher and config code carry no build tag and reference
  the package unconditionally. They use `*Writer` and check for
  `errors.Is(err, ntpshm.ErrUnsupported)` if they want to
  distinguish "not supported on this build" from other failures.

Follow-ons (separate changes, not in this plan):

- **FreeBSD support**: add `types_freebsd.go`, generate
  `ztypes_freebsd.go` *on a FreeBSD host* using its native
  toolchain, broaden the `shm_linux.go` build tag to include
  `freebsd` (or rename to `shm_unix.go` with `//go:build linux || freebsd`),
  and narrow `shm_stub.go` accordingly. Do **not** check in a
  `ztypes_freebsd.go` produced anywhere else -- it would amount to
  a hand-maintained ABI guess.
- **macOS support**: same pattern, generated on macOS.

## SHM lifecycle management

Per the standard NTP SHM driver convention:

- Key: `0x4e545030 + unit` (the ASCII bytes `NTP0`, `NTP1`, ...).
- Size: `sizeof(struct shmTime)`. Use the generated constant
  (see ABI section).
- Flags: `IPC_CREAT | mode` where `mode` is `0600` for units 0/1 and
  `0666` for units >=2.
- The segment is **persistent across satpulsed restarts**: `New`
  attaches with `IPC_CREAT`, `Close` does only `shmdt`. We do not
  call `IPC_RMID`. Reasoning: on Linux, once a segment is marked
  for removal, `shmget(key, ...)` without `IPC_CREAT` cannot find
  it, and a peer calling `shmget(key, ..., IPC_CREAT)` creates a
  *new* segment with the same key but a different shmid. If
  satpulsed RMIDed at attach and ntpd started later, ntpd would
  land on a fresh segment while satpulsed kept writing to the
  ghost. This is exactly the split-brain we want to avoid. Other
  NTP SHM publishers (gpsd, chrony's SHM refclock driver) take the
  same approach: leave the segment in the IPC namespace.

Package API (in `time/lib/ntpshm`):

```go
type Writer struct { /* unexported: addr, t *shmTime, ... */ }

// Attach reports the result of attaching to the segment.
// Fields are read by the caller for logging; the library itself
// does not log.
type Attach struct {
    Unit int
    Key  uint32
}

func New(unit uint8) (*Writer, Attach, error)

func (w *Writer) Write(clockTime, receiveTime time.Time, leap ptime.LeapSecondKind, precision int8)

func (w *Writer) Close() error
```

Lifecycle steps in `ntpshm.New(unit uint8)`:

1. Compute key (`0x4e545030 + unit`), mode (`0600` if `unit < 2`,
   else `0666`), and `shmget` flags (`IPC_CREAT | mode`). No
   range check needed -- `uint8` already constrains `unit` to the
   `0..255` natural limit.
2. `SYS_SHMGET(key, size, IPC_CREAT|mode)` -> shmid. EEXIST cannot
   occur without `IPC_EXCL`; we accept and attach to any existing
   segment regardless of which process created it. Linux returns
   EINVAL if an existing segment is *smaller* than the requested
   size; that should never happen in practice (the struct has been
   stable for years and the compile-time `expectedSize` assertion
   prevents us from shipping a different layout), but if it does,
   `New` returns the EINVAL wrapped so the operator sees the
   underlying cause. Resolution requires `ipcrm` to clear the old
   segment.
3. `SYS_SHMAT(shmid, 0, 0)` -> address. On error, return the error.
4. Cast the address to a `*shmTime` (see ABI section).
5. Initialise metadata fields and invalidate the slot using the
   same prologue the normal `Write` uses (Valid=0 first, then
   Count++, then fields, then Count++):
   ```
   atomic.StoreInt32(&s.Valid, 0)  // block acceptance for any reader starting after this point
   atomic.AddInt32(&s.Count, 1)    // open the seqlock window
   s.Mode      = 1                 // declare count-protected seqlock
   s.Nsamples  = 1                 // sample count (constant; ntpd may use as a filter hint)
   atomic.AddInt32(&s.Count, 1)    // close the seqlock window
   ```
   Why this exact order:
   - `Valid = 0` **must come first**. If we did `Count++` first
     and `Valid = 0` second, a reader starting between those two
     stores would see a stable `Count` (no further changes during
     its loop), the old timestamps, and still `Valid = 1` -- and
     would accept the stale sample.
   - `Mode` and `Nsamples` are written inside the count bracket
     so that any reader poll straddling this attach sees a count
     mismatch and reports CLASH (discarding that one poll) rather
     than observing a partial state. The segment may have been
     created with `Mode = 0` (zero-value of fresh SysV memory);
     writing `Mode = 1` inside the bracket means no reader poll
     ever accepts a slot with both `Mode = 0` and our new
     timestamps.
   - `Nsamples = 1` is conservative: ntpd's classic
     `refclock_shm.c` does not appear to consume this field in
     practice, but gpsd writes a small positive value by
     convention. We pick 1 explicitly rather than leaving the
     zero value so the choice is documented in code.

   Do **not** clear `ClockTimeStamp*` / `ReceiveTimeStamp*`:
   zeroing would expose a window where a reader observes zero
   timestamps with a consistent count.

   Limitation: this sequence cannot retroactively stop a reader
   that was already deep in its do-while body before step 5 began
   and that completes (re-reading `Count`, finding it unchanged
   relative to its captured `c1`, and exiting with the captured
   `Valid = 1`) before our `Valid = 0` and first `Count++` land.
   That single stale read is unavoidable without process-level
   coordination; subsequent reads are correctly blocked.
6. Return `(writer, attach, nil)`.

Because `time/lib/ntpshm` cannot log, the daemon's factory
(`NTPConfig.NewSHMWriter` in the app layer) is responsible for
emitting the "attached" line based on the returned `Attach` struct.

Startup ordering between satpulsed and ntpd does not matter:
whichever process runs `shmget(IPC_CREAT|...)` first creates the
segment, and the other process attaches to it on a later call.
Because neither side calls `IPC_RMID`, the key remains discoverable
across restarts of either process.

Cleanup in `Writer.Close()`:

- `SYS_SHMDT(addr)`. Return the error if any; the caller logs.

The segment itself outlives satpulsed. It persists until the
operator removes it (`ipcrm -M 0x4e545032`) or the system reboots
(SysV IPC namespaces are kernel-global and do not survive boot).
This is the same behaviour as gpsd's SHM publisher and as chrony's
SHM refclock driver. A future `satpulsetool shm-rm <unit>`
subcommand could automate operator cleanup; out of scope here.

If satpulsed crashes without `Close`, the kernel detaches the
segment automatically on process exit. The segment stays in the
namespace; the next satpulsed start attaches to the same segment.

## Generated ABI / layout strategy

The canonical layout from ntpd's `include/ntp_shm.h` /
`ntpd/refclock_shm.c`, mirrored by NTPsec
(`ntpd/refclock_shm.c`):

```c
struct shmTime {
    int    mode;                       /* 0 = legacy, 1 = count-protected */
    volatile int count;
    time_t clockTimeStampSec;
    int    clockTimeStampUSec;
    time_t receiveTimeStampSec;
    int    receiveTimeStampUSec;
    int    leap;
    int    precision;
    int    nsamples;
    volatile int valid;
    unsigned clockTimeStampNSec;
    unsigned receiveTimeStampNSec;
    int    dummy[8];
};
```

Two ABI traps the layout has to survive:

- `time_t` width. On Linux x86_64 / arm64 / riscv64 it is 64-bit. On
  32-bit Linux it depends on whether ntpd was built with
  `_TIME_BITS=64`; both ntpd 4.2.8p17 and NTPsec default to 64-bit
  `time_t` on glibc, but a few older builds remain 32-bit.
- Padding. The `int`-after-`time_t` triples (`clockTimeStampSec`
  then `clockTimeStampUSec`, etc.) need correct trailing pad on
  64-bit ABIs.

Strategy: use the standard `cgo -godefs` mechanism, matching the
pattern already established in this repository (e.g.
`gps/lib/term/types_linux.go` -> `ztypes_linux.go`). A small
cgo-tagged input file declares the Go type in terms of the C
struct; `go tool cgo -godefs` reads it and emits a pure-Go file with
the field layout measured by the C compiler. The generated file is
checked in. The runtime package never imports "C".

Layout of the code in `time/lib/ntpshm/`:

- `types_linux.go` -- input to `cgo -godefs`, build-tagged out of
  the normal build:
  ```go
  //go:build ignore
  // +build ignore

  /*
  Input to cgo -godefs.
  */
  package ntpshm

  /*
  #include <sys/time.h>

  struct shmTime {
      int    mode;
      int    count;
      time_t clockTimeStampSec;
      int    clockTimeStampUSec;
      time_t receiveTimeStampSec;
      int    receiveTimeStampUSec;
      int    leap;
      int    precision;
      int    nsamples;
      int    valid;
      unsigned clockTimeStampNSec;
      unsigned receiveTimeStampNSec;
      int    dummy[8];
  };
  */
  import "C"

  type shmTime C.struct_shmTime
  ```
  We declare `struct shmTime` inline rather than `#include`ing
  upstream `ntp_shm.h` so the generator runs on any host with a C
  compiler, without needing ntpd/NTPsec headers installed.
- `ztypes_linux.go` -- generated, checked in:
  ```go
  // Code generated by cmd/cgo -godefs; DO NOT EDIT.
  // cgo -godefs types_linux.go

  package ntpshm

  type shmTime struct {
      Mode                 int32
      Count                int32
      ClockTimeStampSec    int64
      ClockTimeStampUSec   int32
      Pad_cgo_0            [4]byte  // possibly, depending on ABI
      ReceiveTimeStampSec  int64
      ReceiveTimeStampUSec int32
      Leap                 int32
      Precision            int32
      Nsamples             int32
      Valid                int32
      ClockTimeStampNSec   uint32
      ReceiveTimeStampNSec uint32
      Dummy                [8]int32
  }
  ```
  (Exact layout including any `Pad_cgo_*` fields is determined by
  cgo at generation time.)
- The `//go:generate` directive lives on `shm_linux.go`, matching
  the `gps/lib/term/ioctl_linux.go` convention:
  ```go
  //go:generate sh -c "go tool cgo -godefs types_linux.go | gofmt > ztypes_linux.go && rm -rf _obj"
  ```

The generator needs a C compiler at generation time but produces no
cgo dependency in the runtime build. `go generate
./time/lib/ntpshm` is a developer step run when `types_linux.go`
changes. **There is no automated drift detection.** `types_linux.go`
carries `//go:build ignore`, so a stale `ztypes_linux.go` will not
cause a normal build to fail. Regeneration is on convention plus
the compile-time layout check below; if a contributor edits
`types_linux.go` without regenerating, the change has no effect
until the next regeneration. For a struct of this stability that is
acceptable; if drift becomes a problem we can add a `make
check-generate` target later.

Compile-time layout check: alongside the generated file we keep a
small `const expectedSize = 96` constant and assert at compile
time with the standard Go trick:

```go
var _ [expectedSize - unsafe.Sizeof(shmTime{})]byte
var _ [unsafe.Sizeof(shmTime{}) - expectedSize]byte
```

This catches accidental ABI drift (e.g. someone changes a field
type by hand instead of regenerating) without any CI plumbing.

Bootstrapping: the first time the generator is wired up, the
generated `ztypes_*.go` files are committed alongside the
`types_*.go` inputs. Subsequent edits to either side require
regenerating and committing.

If the upstream C struct ever changes (extremely rare; the last
substantive change predates the project), the workflow is:

1. Update the inline struct in `types_linux.go` (and siblings).
2. `go generate ./time/lib/ntpshm/...`.
3. Inspect the diff in `ztypes_*.go`.
4. Commit both.

## Publication / write path

`ntpshm.Writer` is a concrete struct (mirroring
`sockrefclock.SockRefClock`); `New` returns `*Writer`. Method
signatures differ from `RefClock.Sample` to carry both timestamps
without lossy reconstruction:

```go
func (w *Writer) Write(clockTime, receiveTime time.Time, leap ptime.LeapSecondKind, precision int8)
func (w *Writer) Close() error
```

`Write` never returns an error: the write is unconditional, atomic at
the level the SHM protocol requires, and has no IO that can fail.
Errors at attach time are reported once at startup; if attach fails,
`New` returns `(nil, Attach{}, err)` and the dispatcher proceeds
without SHM.

The segment never publishes a `Valid = 1` slot with `Precision = 0`.
`New` leaves `Valid = 0` (lifecycle step 5), and the dispatcher's
first `Write` writes both the resolved `Precision` and `Valid = 1`
inside the same `Count++ ... Count++` bracket. A reader that polls
between attach and the first `Write` sees `Valid = 0` and discards.

See the Architecture section for the dispatcher's `shmSink`
interface that `*ntpshm.Writer` satisfies. The interface lives on
the consumer side, not in `ntpshm` itself.

Implementation of `Write`:

```go
func (w *Writer) Write(clock, recv time.Time, leap ptime.LeapSecondKind, precision int8) {
    s := w.t // *shmTime, points into the SHM segment

    // 1. Valid = 0
    atomic.StoreInt32(&s.Valid, 0)

    // 2. Count++
    atomic.AddInt32(&s.Count, 1)

    // 3. write timestamps and metadata.  Plain stores; the
    //    surrounding atomics give us ordering.
    s.ClockTimeStampSec    = clock.Unix()
    s.ClockTimeStampNSec   = uint32(clock.Nanosecond())
    s.ClockTimeStampUSec   = int32(clock.Nanosecond() / 1000)
    s.ReceiveTimeStampSec  = recv.Unix()
    s.ReceiveTimeStampNSec = uint32(recv.Nanosecond())
    s.ReceiveTimeStampUSec = int32(recv.Nanosecond() / 1000)
    s.Leap                 = shmLeap(leap)
    s.Precision            = int32(precision)
    // Mode and Nsamples set once at attach; Precision is rewritten
    // each call (cheap, stays inside the Count bracket); Dummy left
    // untouched.

    // 4. Count++
    atomic.AddInt32(&s.Count, 1)

    // 5. Valid = 1
    atomic.StoreInt32(&s.Valid, 1)
}
```

Field names are capitalised because `cgo -godefs` exports them.
Even though `shmTime` is itself unexported (lowercase type name),
the generator emits each member with an initial capital. All
references to the struct fields throughout the plan use that form.

Notes:

- We populate both the legacy `USec` fields and the modern `NSec`
  fields. ntpd 4.2.8 and NTPsec both read `NSec` when non-zero,
  falling back to `USec * 1000`; populating both is safe and matches
  reference writers (e.g. gpsd).
- `precision` follows the ntpd convention: `log2(seconds)` of the
  expected timing precision. It is passed in by the caller per
  `Write` invocation; the writer does no policy. The dispatcher
  owns the value's lifecycle -- fixed `-1` in serial mode; lazy
  init on the first PPS event plus a one-time update after the
  calibration window in PHC mode; or a fixed override when
  `[ntp.shm] precision` is configured. See the Configuration
  section for the full model.
- `leap` maps from `ptime.LeapSecondKind` (the same enum SOCK uses):
  - `LeapSecondNone`     -> 0 (LEAP_NOWARNING)
  - `LeapSecondPositive` -> 1 (LEAP_ADDSECOND)
  - `LeapSecondNegative` -> 2 (LEAP_DELSECOND)
  Matches the SOCK mapping in `sockrefclock/sockpacket.go`. NTP's
  `3` (LEAP_NOTINSYNC) is not used: the dispatcher always has a
  definite leap value at publication time (via the configured leap
  table refined by receiver `LeapSecond` messages), so there is no
  "unknown" state to encode.
- `Write` runs on the dispatcher's main goroutine. No locking; SHM
  reader sync is by the count/valid protocol described next.

## Atomic synchronization

Mode 1 reader behaviour (per ntpd's and NTPsec's `refclock_shm.c`):
on each poll the reader reads `count` once, reads the fields, reads
`count` again, and accepts the sample only if both counts match and
`valid == 1`. If the counts disagree the reader reports a CLASH
(NTPsec ups its bad-count statistic; ntpd similarly accounts a
clash) and *discards* this poll's sample. The reader does **not**
loop or retry.

```
c1 = count
read fields
c2 = count
if c1 == c2 && valid {
    accept sample
    valid = 0
} else {
    report clash; discard
}
```

For the writer-side protocol to deliver clean samples, this means
the writer's job is to make CLASH events *rare* -- each clash is a
one-poll sample loss. The writer's `count++` on each side of the
field writes, plus the `valid=0` before and `valid=1` after,
provides the necessary boundaries so that any reader poll that
straddles the writer's window sees `c1 != c2` and discards. A reader
that polls entirely between two writer events sees a stable `count`
and accepts the latest published slot.

Operationally: at 1 Hz writes and NTPsec polling at ~16 s default,
the overlap window is tiny and clash events should be a low
single-digit percentage at worst. "No CLASH spam in `ntpq` peer
stats over a 10-minute soak" should be part of interop validation.

Memory ordering, in detail:

- `atomic.StoreInt32(&s.Valid, 0)` and `atomic.AddInt32(&s.Count, 1)`
  are sequentially consistent in the Go memory model. They emit
  full barriers on x86 and acquire/release on arm64, which is
  stronger than the SHM protocol requires.
- The plain field stores between the two `Count++` operations can
  be reordered with respect to each other, but cannot escape past
  the second `Count++` because that is a sequentially consistent
  atomic.
- The trailing `Valid = 1` flags the slot ready.

We deliberately use `sync/atomic` rather than `unsafe.Pointer` raw
stores: the atomics document intent and pull in the compiler fence
behaviour we want. The cost is one extra `LOCK XADD` on x86 per
sample, which is negligible at one sample per second.

We also need `Count` and `Valid` to remain 32-bit int32 (the C
`int`). The C `volatile` qualifier is a no-op for our purposes; our
atomics provide the visibility C `volatile` was approximating.

Word-tearing risk on `time_t`: `ClockTimeStampSec` is 64-bit and
naturally aligned in the struct. A racing reader could observe a
torn 64-bit value on a 32-bit architecture. Two mitigations:

- We target 64-bit platforms only for the daemon. (32-bit ARM systest
  exists but is rare; revisit if anyone hits it.)
- The mode-1 count protocol catches torn values: a reader observing
  any byte-level inconsistency from a concurrent writer sees
  `c1 != c2` and discards the poll (CLASH). The next poll, after
  the writer has finished, accepts cleanly. Torn-then-accepted
  cannot happen.

## Error handling and cleanup

Errors break down into:

- **Attach time errors** (returned from `ntpshm.New`):
  - `shmget` EACCES / ENOMEM / ENOSPC -- returned as a wrapped error.
    Daemon logs at Warn and continues without SHM.
  - `shmat` EACCES / EINVAL / ENOMEM -- same.
- **Write time** -- no errors possible. The dispatcher loop is hot
  path; we keep it allocation- and branch-free.
- **Close time** -- `shmdt` errors are returned; caller logs.

Failure to attach is **not fatal** to satpulsed startup. If the user
asked for SHM and we cannot provide it, the daemon logs at Warn and
continues. Rationale: the existing SOCK path has the same property
(`LoggingSockRefClock` tolerates a missing peer), and we want
satpulsed to keep providing the rest of its functionality even when
one downstream consumer is misconfigured.

Logging is the responsibility of the **caller** (the app-layer
factory `NTPConfig.NewSHMWriter` and the dispatcher), not the
library. Suggested convention there:

- At successful attach: `lg.Info("attached to NTP SHM segment", "unit", a.Unit, "key", fmt.Sprintf("0x%x", a.Key))`.
- On attach failure: `lg.Warn("could not attach NTP SHM segment", "unit", n, "err", err)`.
- On detach: `lg.Debug("detached from NTP SHM segment", "unit", n)`.

No per-sample logging. The chrony SOCK path logs each sample at
Debug; we should match that here only if the dispatcher already has
the data on hand. Keep the library `Write` allocation- and
log-free.

## Testing strategy

Unit tests, `time/lib/ntpshm`:

- ABI: covered by the compile-time layout assertion next to the
  generated file (the package fails to compile if `sizeof(shmTime)`
  drifts from `expectedSize`). No CI plumbing.
- `TestWriteRoundTrip` -- single-goroutine write then read-back via
  the same `*shmTime` pointer. Verify each field stored matches
  what `Write` was given, including `Precision` reflecting the
  passed `precision int8` (covering at least one positive value,
  one negative value, and the int8 extremes), that `Mode == 1`,
  `Valid == 1`, and `Count` advanced by 2.
- `TestPrecision` -- table-driven test of `ntpshm.Precision`:
  - Inputs spanning the practical range (e.g. `1ns`, `5ns`,
    `100ns`, `1µs`, `1ms`, `0.5s`, `1s`, `60s`) yielding the
    expected `int8` outputs.
  - Verifies ceil-rounding: a duration whose `log2(seconds)` is
    not an exact integer rounds up to the next exponent. Probe
    a power-of-two duration (e.g. `2*time.Second` -> `1`) and a
    just-above-power-of-two (e.g. `2*time.Second + 1` -> `2`).
  - Verifies clamping: a duration so small that `log2(seconds)`
    underflows below `-128` returns `-128`; a duration so large
    that it overflows above `127` returns `127`.
  - Verifies edge cases: `0` and negative durations return `-128`
    (matches the 5 ns floor convention used in
    `MultiSample.Reduce` / `Extract`).
- `TestAttachExisting` -- one `New` call creates the segment; a
  second `New` call attaches to the same key and sees a `Write`
  through the first writer's mapping. Detach both; the segment
  persists (no `IPC_RMID`). Tests use a unit number outside the
  expected production range so they can clean up with `ipcrm` in
  a `t.Cleanup` and re-run idempotently.

No concurrent-reader microbenchmark. The seqlock correctness comes
from the Go memory model and the `sync/atomic` API (documented in
the Atomic synchronization section). The interop smoke test below
exercises real-world concurrency against a real reader.

Integration tests, `time/internal/gpsevent` (existing dispatcher
tests):

- Extend the dispatcher test scaffolding to accept a fake through
  the private `shmSink` interface declared in `gpsevent`. Drive a
  `Time` message; assert the fake's `Write` was called with the
  expected `(clockTime, receiveTime)`.
- Verify the SHM gate matches the SOCK gate in PHC tracking mode
  (only called when `phcsync.ModeTracking`).
- Verify both sinks fire on the same dispatch event when both are
  configured.
- Verify the PHC-mode precision lifecycle: the fake `Write`
  observes (a) on event 1, the initial-estimate precision derived
  from event 1's Duration; (b) on events 2 through `N - 1`, the
  same initial estimate (no recomputation); (c) on event `N`,
  the median of events 1..N inclusive (one-time switch); (d) on
  events after `N`, the same calibrated value held constant even
  when subsequent events carry different Durations. Also verify
  that an explicit `[ntp.shm] precision = N` override is used
  unchanged from the first call onward and no buffering occurs.

Systest, `systest/`:

- One playbook against either NTPsec or classic ntpd (pick
  whichever is already easiest to install on the systest target;
  NTPsec on Debian is convenient). Set the segment unit to 2 in
  `satpulse.toml`, add the corresponding refclock line, start
  both, verify:
  - `ntpq -p` shows the GNSS peer with stratum 0 and a ticking
    reach;
  - the peer's offset and jitter values are sensible within a
    minute or two;
  - reach count stays stable for at least five minutes;
  - `ntpq -c "clockvar &1"` shows zero or near-zero count of
    CLASH / bad-count events over a 10-minute soak (writer-reader
    racing is rare; chronic spam indicates a protocol bug in the
    writer prologue).
- Tear down satpulsed and confirm the peer goes unreachable; bring
  satpulsed back and confirm recovery within one polling interval.

## Interoperability testing

For the initial landing, the systest playbook above is the interop
smoke test. The other reader (whichever was not chosen) and a
`gpsd` cross-check are useful follow-ons but not required to land
the feature.

Follow-on interop validation:

- Second systest playbook against the other of {classic ntpd,
  NTPsec}.
- `gpsd` cross-check: run gpsd against the same GPS without
  satpulsed and compare SHM contents qualitatively. gpsd is the
  reference SHM producer; meaningful divergence is worth
  understanding.
- `ipcs -m` / `ipcrm` sanity:
  - With both processes up, `ipcs -m` shows the segment with
    nattch=2 and no `dest` flag (we do not call IPC_RMID).
  - Stopping both processes leaves the segment with nattch=0.
  - `ipcrm -M 0x4e545032` removes the segment cleanly when no
    processes are attached.
  - Document: operators should stop satpulsed before `ipcrm`.

## Suggested package / module organization

New:

- `time/lib/ntpshm/`
  - `ntpshm.go` -- exported `Writer` wrapping an unexported `shmWriter`; `Writer`'s methods (`Write`, `Close`) delegate to lowercase `shmWriter` methods; top-level constructor `New(unit uint8) (*Writer, Attach, error)` delegating to `newShmWriter`. Also `Attach`, sentinel errors (incl. `ErrUnsupported`), `Precision(time.Duration) int8` helper. Reuses `ptime.LeapSecondKind` (same as SOCK) for the leap argument. Pure Go, no build tag. No logging, no goroutines (library layer).
  - `types_linux.go` (`//go:build ignore`) -- input to `cgo -godefs`, declares `type shmTime C.struct_shmTime`.
  - `ztypes_linux.go` -- generated by `cgo -godefs` on Linux, checked in. `// Code generated ... DO NOT EDIT.` header.
  - `shm_linux.go` (`//go:build linux`) -- unexported `shmWriter` with `*shmTime` field and the syscall plumbing for `newShmWriter` / `write` / `close`. Carries the `//go:generate` line.
  - `shm_stub.go` (`//go:build !linux`) -- unexported `shmWriter` as an empty struct; `newShmWriter` returns `ErrUnsupported`. FreeBSD and macOS support are follow-on changes generated on those OSes.
  - `ntpshm_test.go`.

Modified:

- `time/internal/gpsevent/dispatcher.go` -- declares the unexported
  `shmSink` interface; new `shm shmSink` field on `Dispatcher`; new
  `shmPrecision int8` field plus the three-stage calibration
  policy in PHC mode (first-event init, median-of-N update,
  fixed); per-sink-nil-guarded SHM call sites in `sysSample` and
  `MsgUTCTime`; a deferred close wrapper in `Run` that calls
  `d.shm.Close()` when non-nil and logs the returned error. See
  the Architecture and Configuration sections for the exact
  snippets. `refclock` stays focused on the proxy/worker
  mechanics.
- `time/phc/types.go` -- `MultiSample.Extract` and
  `MultiSample.Reduce` each gain a third return value
  `time.Duration`: the selected sample's `SysInterval` floored at
  5 ns. Existing call sites: `time/internal/ts/clock.go` must
  update (it's in the normal build); `time/phc/phc_cmp.go` should
  also update for consistency but carries `//go:build ignore`, so
  forgetting it won't break the build.
- `time/internal/ts/clock.go` -- `Event` gains a
  `Precision time.Duration` field. `Clock.wallSample` returns an
  additional `time.Duration` (the Reduce result), and the event
  loop assigns it directly into `event.Precision`.
  `phctime.Sample` is unchanged.
- `time/app/daemon/config.go` -- `NTPSHMConfig`; factory function
  `NTPConfig.NewSHMWriter(lg) (*ntpshm.Writer, error)` that calls
  `ntpshm.New`, logs the attach result, and returns the concrete
  writer (or `nil` with a non-fatal error when SHM is not
  configured / attach failed).
- `time/app/daemon/daemon.go` -- call `cfg.NTP.NewSHMWriter(lg)`,
  then nil-guard `cfg.NTP.SHM` (which is `*NTPSHMConfig` and nil
  when `[ntp.shm]` is absent) before reading `.Precision`. Pass
  both the writer (`*ntpshm.Writer`, may be nil) and the
  resulting precision override (`*int8`, may be nil) into
  `NewDispatcher`.
- `configs/config-schema.json` -- `ntp.shm.unit` and optional
  `ntp.shm.precision`.
- `configs/satpulse.toml` -- commented-out example block (and link
  from `docs/setup/`).
- `docs/internals.md` -- add a `time/lib/ntpshm` entry under
  `time/lib/`.

Documentation (in scope):

- `docs/man/satpulse.toml.5.md` -- add `[ntp.shm]` section
  documenting the `unit` key (range, default = absent = disabled,
  permission implications for unit 0/1 vs unit 2+).

Follow-on (separate change, not part of this plan): a user-facing
setup page for connecting satpulsed to an NTP server. The current
intent is a combined `docs/setup/ntp.md` covering chrony, classic
ntpd, NTPsec, and ntpd-rs in one place rather than splitting per
implementation. The "Documentation and configuration examples"
section below sketches the classic-ntpd/NTPsec content to fold in.

## Compatibility pitfalls and race conditions

- **Permissions and ownership**: unit 0/1 means 0600; ntpd reading
  unit 0/1 expects to own the segment. The simplest deployment has
  both daemons running as root. If they do not, use unit 2+.
- **Leap-second semantics**: the `leap` field uses NTP packet
  encoding (`0=none, 1=insert, 2=delete`). See the Publication /
  write path section for the mapping from `ptime.LeapSecondKind`.
- **time_t width on 32-bit**: as noted, we assume 64-bit time_t. If
  we ever ship a 32-bit daemon build, generate a 32-bit variant
  with a build tag.
- **Segment persistence and split-brain**: the segment lives in
  the kernel IPC namespace and survives satpulsed exit. We
  deliberately do **not** call `IPC_RMID`. Calling RMID at attach
  is unsafe for NTP SHM: on Linux, after RMID a later
  `shmget(key)` by ntpd creates a *fresh* segment with the same
  key but a different shmid, and satpulsed and ntpd end up on
  different segments. Persisting the segment also requires that
  only one writer ever attaches per unit -- "do not run two
  publishers (satpulsed and gpsd, or two satpulseds) on the same
  unit" -- which matches the ntpd convention anyway.
- **Leftover segments**: stopping satpulsed leaves the segment
  present (`ipcs -m` will list it with nattch reflecting ntpd's
  attachment, if any). Reboot clears it. Manual cleanup:
  `ipcrm -M 0x4e545030` (or +unit). Document this and consider a
  future `satpulsetool shm-rm <unit>` helper.
- **Stale data on attach**: the segment is persistent and may
  contain `Valid=1` plus stale timestamps from a previous instance
  (own or other). `New` invalidates the slot using the
  `Valid=0; Count++; ...; Count++` write-prologue order described
  in lifecycle step 5. A bare `Valid = 0` alone is racy with a
  reader that has already captured `c1` and read the timestamps;
  the surrounding `Count++` operations ensure any such concurrent
  poll observes a count mismatch and reports CLASH (one discarded
  poll) rather than accepting the stale sample. Timestamps are
  deliberately not zeroed: leaving them in place is safe because
  the invalidation prevents acceptance until our first full
  `Write` cycle completes.
- **macOS shmmax**: tiny default. Our segment is ~96 bytes, so no
  problem. Document anyway.
- **SELinux / AppArmor**: SysV SHM has its own policy. On selinux
  systems, satpulsed needs `sysvipc` permission. Mention in
  `selinux/` policy update if needed.

## Documentation and configuration examples

In scope -- `docs/man/satpulse.toml.5.md`:

Add a subsection under the existing `[ntp]` description, sibling to
the SOCK documentation:

    [ntp.shm]
    unit
        SysV shared-memory unit number for an NTP SHM (driver 28)
        refclock. When set, satpulsed publishes GNSS UTC time
        samples into the segment with key 0x4e545030 + unit. Units
        0 and 1 require the segment to be created with mode 0600
        and are normally only usable when satpulsed and ntpd run
        as the same user (typically root); units 2 and above use
        mode 0666 and work across users. The ntpd / NTPsec
        refclock line should NOT include "mode 1" -- that is a
        driver-mode word with bit 0 forcing a private 0600 segment
        for units 2+, which would defeat cross-user access. The
        in-segment synchronization protocol (sometimes also called
        "mode") is selected by satpulsed unconditionally; the
        reader does not need to be told.

    precision
        Optional override of the SHM precision field
        (log2 seconds, NTP convention). When unset, satpulsed
        chooses a value automatically: in serial timing mode it
        uses a fixed conservative value; in PHC tracking mode it
        derives precision from the PHC/system cross-timestamping
        read uncertainty, set on the first PPS event and refined
        once after roughly one minute of calibration samples,
        then held constant for the rest of the run. Override
        only if the automatic value misrepresents your hardware.


Follow-on -- proposed `docs/setup/ntp.md` content for the ntpd /
NTPsec section (keep here so it is not lost; the actual page will
combine this with the existing chrony content and any ntpd-rs
material):

```markdown
---
title: ntpd / NTPsec setup
---

Make sure ntpd or ntpsec is installed.

Add this to `satpulse.toml`:

    [ntp]
    shm.unit = 2

This makes satpulsed publish GNSS UTC time samples into NTP SHM
segment number 2. Restart the satpulse service.

## Classic ntpd

Add this to `/etc/ntp.conf`:

    server 127.127.28.2 prefer
    fudge  127.127.28.2 refid GNSS stratum 0

Restart ntpd.

## NTPsec

Add this to `/etc/ntpsec/ntp.conf`:

    refclock shm unit 2 prefer

Restart ntpsec.

## PPS

satpulsed does not publish PPS through SHM. If you need sub-second
discipline, configure the PPS/ATOM refclock (driver 22) in ntpd
separately, against the kernel PPS device backed by your GPS pulse.
```

`configs/satpulse.toml`: append a commented example block analogous
to the existing `[ntp]` SOCK example.

`docs/man/satpulse.toml.5.md`: a new `[ntp.shm]` subsection
describing `unit` and pointing to the ntpd setup page.

## Phasing

Recommended landing order:

1. `ntpshm` package: `ntpshm.go` (exported `Writer` + delegates),
   `types_linux.go` input and its generated `ztypes_linux.go`
   output, `shm_linux.go` syscall implementation of `shmWriter`,
   `shm_stub.go` for non-Linux builds, and the compile-time
   layout assertion. No dispatcher wiring yet.
2. Config additions and validation.
3. Dispatcher wiring, with both sink call sites guarded.
4. Documentation.
5. Systest playbooks for ntpd and NTPsec.

Each step is independently mergeable. Step 1 alone has no user
effect; step 3 turns the feature on.

## Open questions

- Is there an interest in writing the segment from a separate
  process (so satpulsed itself remains pure Go and the SHM glue
  lives elsewhere)? Out of scope -- the user explicitly asks for
  pure-Go SHM inside satpulsed.
