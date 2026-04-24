# PPS sample mode

## Introduction

This document describes a mode of satpulsed operation in which edges come from the Linux kernel PPS API rather than from a PHC PPS timestamper. Edges arrive as `CLOCK_REALTIME` values; satpulsed correlates them with GPS time messages and feeds samples to chrony via the SOCK refclock.

**Assumes `plan/phc-sample.md`.** Most of the reasoning, vocabulary, and data shapes carry over. This plan is scoped as a delta: what is reused, what is refactored, what is new. Refer back to `phc-sample.md` for the motivation around chrony hardware timestamping, leap-second strategy, and the shape of the label/calibration pipeline.

## Prerequisites and relationship to other modes

Two satpulse changes need to land before this plan is picked up:

- **#258 (`ntime.Time`).** The refclock path's timestamp type becomes domain-neutral. This plan is written in terms of `ntime.Time` throughout.
- **`gensample` extraction** (phase 1 below). The shared pieces are lifted out of `phcsample` into a peer package that this mode and `phcsample` both consume.

This mode (tracked as #259) is one of three in the same family that leave the PHC free-running (or have no PHC at all):

- **#256** — non-syncing PHC mode, satpulse ships system-referenced samples.
- **#257** — PHC-base-clock mode, satpulse ships PHC-referenced samples; chrony does PHC-to-system mapping. Needs upstream chrony `multi-clock` support.
- **#259 (this plan)** — kernel-PPS edge source, satpulse ships system-referenced samples, no PHC involved.

This mode's distinguishing niche is deployments where no NIC PHC is usable (or wanted), typically Raspberry Pi with `pps-gpio`.

## Motivation

Three situations where kernel PPS is the right input source rather than a NIC PHC:

1. **No usable PHC.** The host has no NIC with a PHC, or the PHC on the NIC in question is not clocking off the same oscillator that carries the PPS line.
2. **GPIO PPS.** On Raspberry Pi–class hardware the PPS line is wired to a GPIO pin driven through `pps-gpio`, not through a NIC. The kernel exposes it at `/dev/ppsN`; there is no PHC in the picture.
3. **Deployment simplicity.** Where hardware timestamping on a specific NIC is not a requirement, kernel PPS is one fewer moving part than `phcsample`.

This is a product-scope addition, not a correctness improvement. Where a NIC PHC is driven off the same oscillator as the PPS line and hardware timestamping matters, `phcsample` remains the right choice.

**MVP target: Raspberry Pi with `pps-gpio`.** The dominant modern deployment: a GPS receiver's PPS line wired to a GPIO pin, `pps-gpio` exposing it as `/dev/ppsN`. This is the configuration the MVP aims to support. Several other input paths (serial DCD via `pps_ldisc`, direct modem-control watching, Darwin-native) are valuable but post-MVP, each broken out in the "Not in MVP" section.

## Shape of the problem

The kernel delivers each edge as a `CLOCK_REALTIME` timestamp at the moment of the edge (rising, falling, or both, depending on capture mode). Compared with `phcsample`, this collapses two subsystems:

- No PHC → no PHC-to-UTC calibration regression. Nothing analogous to `mapEdgesToUTC` or `fitAndEvaluate`.
- No cross-sample query. Each edge *is* a sample. The refclock offset is computed once per edge and emitted immediately.

What remains is the message-side work: mapping the edge to its integer UTC second, and filtering wrong-stride / wrong-polarity edges before they reach the emitter.

The core per-edge calculation (under #258, with `ntime.Time` as the neutral timestamp type):

```
edge_ntime  := ntime.FromSysTime(edge_realtime)
utc_at_edge := wallClock.SecondAt(edge_realtime)   // returns ntime.Time
offset      := utc_at_edge.Sub(edge_ntime).Seconds() + sawtooth
sampler.NTPSample(sys = edge_ntime, offset, leap)
```

That is the whole Generator, modulo warm-up, edge buffering, and filtering.

## Wall-clock-only edge times are fine

Kernel PPS produces `CLOCK_REALTIME` without a monotonic companion. `gensample.WallClock.SecondAt` takes a `time.Time` on its X axis; internally it computes `mono.Sub(anchorX)` where `anchorX` is a monotonic-bearing `time.Time` from `timemsg.Buffer`. Go's `time.Time.Sub` falls back to wall-clock subtraction when either operand lacks a monotonic reading, so passing the kernel's `time.Unix(sec, nsec)` value through unchanged works.

The fallback answer is "CLOCK_REALTIME elapsed since anchor capture" rather than "monotonic elapsed," which diverge only across `CLOCK_REALTIME` steps. Chrony slews rather than steps once settled, and the wallClock window turns over in tens of seconds, so the divergence is irrelevant for the sub-second precision `SecondAt` needs.

The Y-axis return (`ntime.Time`) is integer-nanosecond arithmetic with no monotonic-vs-wall-clock subtlety, so the above concerns only the X-axis input.

No edge-monotonic derivation is required on the PPS side. Contrast with `phcsample`, which must scale a PHC delta to real time and subtract from `TRead.Mono` because there is no direct PHC-side wall-clock anchor.

## Configuration

A single new TOML section:

```toml
[pps]
device = "/dev/pps0"
edge   = "assert"         # assert | clear (default assert)

# wallClock tuning (same fields as [phcsample]'s wallClock section)
msgWindow = 30
maxMsgGap = 4
minMsgSpan = 3
clockRateLimit = 0.1
msgTimingVariation = 0.25
expectedDelay = 0.1

# edge filter tuning
pulseWindow = 16
pulseVariation = 500
```

In MVP, `device` is expected to be a `/dev/ppsN` character device (typically from `pps-gpio` on a Raspberry Pi). Post-MVP variants that accept a serial tty and handle the underlying details are broken out in "Not in MVP" below.

`edge` selects which edges become samples: `assert` (rising) or `clear` (falling). There is no "both" option — only one polarity is ever the top-of-second edge, and capturing both would just emit half the samples with a pulse-width offset. The default is `assert`; configure `clear` only if the pulse is inverted (active-low).

The remaining fields are the same wallClock and edge-filter tuning carried by `[phcsample]`. `ppssample.Config` embeds `gensample.Config`, and Go's TOML encoder surfaces the combined fields at the top of `[pps]`. `pulseWidthDetectLimit` has no analogue here because dual-edge-polarity detection does not apply.

Mode selection has two orthogonal axes:

1. **Edge source** — determined by the presence of `[pps]`. Absent: PHC via `/dev/ptpN`. Present: kernel PPS via `/dev/ppsN`.
2. **SOCK sample domain** — determined by `[ntp].clock` (introduced with #257). `"system"` means satpulse ships system-referenced samples; `"phc-utc"` / `"phc-tai"` mean PHC-referenced samples.

With `[pps]` present, only `clock = "system"` is valid — the PHC-referenced modes require a PHC, which this path does not have. Rejecting the combination at config load is straightforward.

## Module structure

`gensample` is a new peer package to `phcsample` and `ppssample`. It owns the pieces that are not specific to either PHC or kernel-PPS input paths: the message-stream regression, the stride/polarity filters (generic over edge type), the Sampler interface, and the shared Config surface. Neither mode imports the other.

```
time/internal/gensample/         (new; peer of phcsample and ppssample)
    wallclock.go                 WallClock — renamed from phcsample.wallClock
    consistent_edges.go          ConsistentEdges[E any]  (generic)
    select_stream.go              SelectTimingStream[E any] (generic)
    config.go                    Config — wallClock + filter tuning
    sampler.go                   Sampler interface (NTPSample method)
    errors.go                    ErrNotReady
    pulse_corrector.go           PulseCorrector interface (phase-2 sawtooth)

time/internal/phcsample/         (refactored; same public interface)
    config.go                    Config embeds gensample.Config (flat)
    phc_window.go                Calls gensample.ConsistentEdges / SelectTimingStream
    generator.go                 Holds a *gensample.WallClock, unchanged public API
    sim/                         Unchanged

time/internal/ppssample/         (new)
    config.go                    Config embeds gensample.Config (flat)
    edge.go                      PulseEdge { Timestamp time.Time }
    generator.go                 Per-edge emitter with small edge buffer
    sim/                         Parallel to phcsample/sim, minus PHC oscillator

time/internal/ppsreader/         (new)
    reader.go                    /dev/ppsN via syscall/ioctl
    reader_test.go               mock or hardware-gated
```

Each Config renders flat in its TOML section; `phcsample.Config` embeds `gensample.Config`, `ppssample.Config` embeds `gensample.Config`. Defaults compose: each mode's `DefaultConfig` calls `gensample.DefaultConfig`, overrides fields whose defaults differ by context, then sets mode-specific fields.

## Dispatcher wiring

Runtime mode is the product of edge source and sample domain:

| Edge source | `clock` | Controller | Generator | Mode |
|-------------|---------|------------|-----------|------|
| (none) | `system` | nil | nil | Serial timing |
| PHC events | `system` | phcsync | nil | PHC disciplined |
| PHC events | `system` | nil | phcsample | PHC free-running (#256) |
| PHC events | `phc-utc` / `phc-tai` | nil | phcdirect | PHC-base-clock (#257) |
| kernel PPS | `system` | nil | ppssample | PPS sample (this plan) |
| kernel PPS | `phc-*` | — | — | rejected at config load |

Implications for PPS sample mode:

- `ppsreader` runs as a goroutine analogous to the PHC event source, delivering edges into `ppssample.Generator.Pulse`.
- `SetMsgUTCTimer` wires to the ppssample generator; serial SOCK sampling is disabled, same as in the other generator-driven modes.
- The PHC path is not instantiated at all in ppssample mode.
- `obs.Observer.NTPSample` is already in place; ppssample hooks into it through the same `Sampler` structural match as phcsample. Under #258 the Sampler interface is typed in `ntime.Time`.

## Simulation challenge

No existing code or data to base a PPS simulator on.

What is and isn't needed:

- **Sample stimuli are straightforward.** The clocksim / syncsim GPS simulator (pulse jitter, sawtooth, outages, outliers) is reusable verbatim — it already produces pulse events at ground-truth times plus GPS-side error. The PHC oscillator layer is simply dropped; there is no PHC in the picture. `sys` is synthesised from ground-truth time, same as in `phcsample/sim`.
- **No PHC-side noise model.** `phcsample/sim` runs PHC edges through a simulated oscillator (white noise + drift + sinusoids) because the PHC is free-running. For ppssample the kernel's `CLOCK_REALTIME` is what carries the noise, and in the sim we treat it as ground-truth plus receiver error — same assumption the phcsample sim makes for its `sys` argument.
- **No hardware data to calibrate against.** Unlike phcsample, where the sim can be sanity-checked against `phcsync` behaviour traces, ppssample has no prior-art internal reference. Calibration happens against kernel-PPS traces from real Pi / PPS-GPIO hardware during phase 4 bring-up. The sim's job in phase 3 is to verify the Generator's logic, not to predict absolute hardware residuals.

Practical consequence: the sim rig is cheap to build but has less cross-check value than phcsample/sim. Expect to revise thresholds in the `ppssample/sim` tests after the first real-hardware run.

## Phasing

### Phase 1 — refactor phcsample into gensample + phcsample

Pure refactor. No behaviour change, no new features, no change in public API of `phcsample`. The only observable difference is the physical location of types: what was unexported inside `phcsample` is now exported from `gensample` and referenced by `phcsample`. Assumes #258 has already landed, so `WallClock` is already `ntime.Time`-typed on its Y axis.

1. **Create `time/internal/gensample`.** Move `phcsample.wallClock` to `gensample.WallClock`, exporting. Move `ErrNotReady`, `Sampler` interface, `pulseCorrector` → `PulseCorrector` interface. Move `consistentEdges` → `ConsistentEdges[E any]` and `selectTimingStream` → `SelectTimingStream[E any]`, generifying over edge-timestamp type via an `interval func(prev, next E) time.Duration` callback. Extract the shared Config fields (`MsgWindow`, `MaxMsgGap`, `MinMsgSpan`, `ClockRateLimit`, `MsgTimingVariation`, `ExpectedDelay`, `PulseWindow`, `PulseVariation`) into `gensample.Config`. `PulseWidthDetectLimit` stays in `phcsample.Config` — it drives `SelectTimingStream`, which only phcsample calls.

2. **Rework `phcsample` around `gensample`.** `phcsample.Config` becomes `struct { gensample.Config; …PHC-only… }`. `phcsample.phcWindow` calls the generic filters, passing a phctime-aware interval function. `phcsample.Generator` holds `*gensample.WallClock`. Existing tests either move to `gensample` (filter and wallClock unit tests) or stay in `phcsample` (Generator and sim integration). `phcsample/sim` is unchanged. All tests remain green, end to end.

End of phase 1: the tree compiles and behaves identically; no caller of `phcsample`'s public API sees any change. The daemon runs exactly as before.

### Phase 2 — PPS reader

Standalone package, buildable and testable without any ppssample code in place.

3. **`time/internal/ppsreader`.** Open `/dev/ppsN`, fetch edges via `PPS_FETCH` ioctl, emit `time.Time` events on a channel. Capture mode is single-polarity — assert-only or clear-only — per `[pps].edge`. Mock/fake backend for unit tests so Generator-level tests do not require kernel PPS.
4. **Hardware bring-up.** Real-hardware smoke test on Pi / x86 test beds with GPIO-PPS or NIC-PPS-derived `/dev/ppsN` devices. Confirm edge rate, event latency, timestamp plausibility.

The PPS API itself is cross-platform (RFC 2783, originating from NTP). What differs is how userspace reaches it. On Linux the API is exposed as `/dev/ppsN` character devices driven by ioctls defined in `<linux/pps.h>`, which is reachable from pure Go via `golang.org/x/sys/unix`. On BSD and Darwin the API is `<sys/timepps.h>`, a macro-heavy C header that effectively requires cgo.

satpulse's policy is no cgo on Linux. The MVP reader is therefore Linux-only and pure Go. If `x/sys/unix` does not already expose the PPS ioctls and structs, they are reproduced from `<linux/pps.h>` using its primitives (`IoctlSetPointerInt` and friends, with manually-packed structs). A separate research task evaluates existing Go PPS packages before committing to a hand-rolled reader.

### Source variety (MVP and beyond)

From the reader's steady-state point of view there is only one path: `/dev/ppsN`. The kernel PPS subsystem aggregates multiple underlying sources behind that interface — `pps-gpio` for a GPIO pin on a Raspberry Pi, `pps_parport` for a parallel-port pin, `pps_ldisc` line-discipline-attached serial DCD, and so on — and the read loop is oblivious to which. (Unrelated path for contrast: a NIC PHC's own PPS input is reached via `/dev/ptpN` with `PTP_EXTTS_REQUEST`, and that is phcsync's code path, not ppssample's.)

MVP consumes whatever `/dev/ppsN` the kernel has already set up. If the underlying source is a serial DCD line, an external tool (e.g. `ldattach 18 /dev/ttyUSB0`) has to have attached `pps_ldisc` before satpulsed starts. Automatic attach, the Linux serial-direct paths, and the Darwin paths are all post-MVP — see "Not in MVP" below.

### Phase 3 — ppssample

5. **`ppssample.Config`.** Config surface lands and validates. Embeds `gensample.Config`. `[pps].device` is a separate small struct (ignored if section absent).
6. **`ppssample.Generator`.** Core type. Holds a bounded edge buffer; on `Pulse`, appends the edge and runs `gensample.ConsistentEdges` to filter wrong-stride edges. If the new edge is admitted, calls `WallClock.SecondAt` (returning `ntime.Time`), computes the offset, and emits via `Sampler`. Implements `MsgUTCTimer`. Includes leap-second reset behaviour. Sawtooth / `PulseOffset` correction is additive — applied only when `PulseCorrector` is non-nil, matching phcsample's phase-2 convention. No polarity detection (`gensample.SelectTimingStream` is not called; only the configured single polarity is ever captured by `ppsreader`).
7. **`ppssample/sim`.** Parallels `phcsample/sim` minus the PHC oscillator: reuses syncsim's GPS simulator (pulse jitter, outages, outliers, sawtooth) for edge timing; treats `sys` as ground-truth plus receiver error; scores each emitted sample against ground truth. Unit tests cover warm-up, steady-state, missing messages, missing edges, gross outliers, multi-rate messages.
8. **Generator-level tests.** Table-driven, same style as phcsample.

End of phase 3: `ppssample` passes unit and sim tests; no daemon wiring yet.

### Phase 4 — wire into the daemon

9. **Config parse.** Add the `[pps]` section to the daemon config loader. Reject combinations that conflict with existing mode selectors, including `[pps]` with `[ntp].clock != "system"`.
10. **Dispatcher mode.** Fourth runtime mode per the table above. Route edges from `ppsreader` into `ppssample.Generator.Pulse`. Wire `SetMsgUTCTimer` to the generator. Disable serial SOCK sampling. PHC path not instantiated.
11. **End-to-end.** satpulsed + chrony + kernel PPS, on real hardware. Collect residual traces and use them to retune `ppssample/sim` thresholds where model and reality diverge.

End of phase 4: ppssample is a usable, documented, tested production mode.

## Open decisions

- **Edge capture shape.** `ppssample.PulseEdge` carries just `time.Time`. Whether it needs a tag for the captured polarity is unlikely — the reader captures only one polarity at a time and that is known from config — but confirm during phase 3.
- **PPS reader implementation (Linux MVP).** Whether an existing pure-Go library is usable, or the reader is hand-rolled on `x/sys/unix`. Pure Go either way.
- **Shared-config defaults.** Whether the wallClock defaults in `gensample.DefaultConfig` are the same for both modes, or whether phcsample and ppssample override specific fields. Likely the same, but confirm at phase 1.

## Not in MVP

Four distinct input paths beyond `/dev/ppsN`, each worth doing but separate from MVP scope. Order below is rough priority.

### 1. Attach `pps_ldisc` ourselves (including shared tty)

Today MVP requires `/dev/ppsN` to already exist; if the PPS source is serial DCD, the operator has to run `ldattach 18 /dev/ttyXXX` before satpulsed starts. This feature lets satpulsed attach `pps_ldisc` itself, so `[pps].device` can name a tty directly.

The useful and tricky case is the **shared-tty** variant: one serial interface carrying GPS messages on the data lines *and* PPS on DCD. Many low-cost receivers (USB GPS dongles, UART GPS modules on a Pi) are wired this way, so second-class treatment costs real users.

At the kernel API level, the only new operation is one `ioctl(tty_fd, TIOCSETD, &N_PPS)` after termios is configured. `pps_ldisc` is a passthrough for data, so `read(tty_fd)` continues to deliver GPS messages; the DCD transitions surface on a freshly-created `/dev/ppsN` that `PPS_FETCH` consumes independently. No shared state at runtime.

In satpulse code, this is three small pieces:

1. **Bridging method on the tty-owning type.** `term.Term` gains an `AttachPPSLdisc()` method that does the ioctl on its own fd. The PPS reader calls it at startup; the fd stays owned by `term`.
2. **`/dev/ppsN` discovery.** `TIOCSETD` doesn't return the new pps number. Scan `/sys/class/pps/pps*/path` and match against the tty's canonical path:
   ```go
   func findPPSForTTY(ttyPath string) (string, error) {
       entries, _ := os.ReadDir("/sys/class/pps")
       want, _ := filepath.EvalSymlinks(ttyPath)
       for _, e := range entries {
           data, err := os.ReadFile(filepath.Join("/sys/class/pps", e.Name(), "path"))
           if err != nil { continue }
           got, _ := filepath.EvalSymlinks(strings.TrimSpace(string(data)))
           if got == want { return filepath.Join("/dev", e.Name()), nil }
       }
       return "", fmt.Errorf("no /dev/pps entry for %s", ttyPath)
   }
   ```
   sysfs population is asynchronous, so expect to retry for a short while after the attach.
3. **Same-device detection for the shared-tty case.** `Fstat(gpsTTY).Rdev == Fstat(ppsDevice).Rdev` → same kernel device, use the GPS reader's existing fd rather than opening a second one. Bulletproof against symlinks, `/dev/serial/by-id/` aliases, etc.

Startup ordering: open + termios → `AttachPPSLdisc()` → `findPPSForTTY()` → open `/dev/ppsN` → PPS reader goroutine → GPS reader goroutine. Shutdown is the reverse; closing the last tty open reverts the ldisc with no explicit cleanup.

Scope: ~80 lines of net new code, mostly the sysfs walk. Not architecturally hard.

### 2. Linux `TIOCMIWAIT` direct modem-control watching

Classic gpsd approach: `ioctl(tty_fd, TIOCMIWAIT, TIOCM_CAR)` blocks until DCD changes, then `TIOCGICOUNT` tells you the transition count. No kernel PPS involvement at all; no `pps_ldisc` required.

Useful when:
- The kernel is built without `pps_ldisc` support (uncommon but possible on stripped-down embedded builds).
- Attaching `pps_ldisc` conflicts with something else on the tty.

The cost is precision: the timestamp comes from userspace `time.Now()` after the block returns, so it picks up scheduling latency and ioctl overhead that `pps_ldisc` avoids by timestamping inside the kernel's `dcd_change` hook. Acceptable for some deployments, not for others.

### 3. Darwin CTS polling (pollpps.go approach)

Darwin has no `/dev/ppsN`, no `pps_ldisc`, no `TIOCMIWAIT`. The experiment at `cmd/pollpps/pollpps.go` busy-polls the CTS line at ~100 µs intervals, detects the edge as a CTS flag transition, and records the timestamp with `time.Now()`. Works on USB-TTL adapters like the Waveshare FT232RNL; unable to do DCD on many Darwin USB-serial drivers because they simply don't surface it, so CTS is the practical option.

Naïvely this is CPU-heavy: 10 000 modem-status ioctls per second, most of them useless. The refinement — which would make this production-grade — is **adaptive polling around the expected edge time**. Once the wallClock has locked on, the next edge's approximate monotonic time is predictable to within ~1 ms; start polling only ~5 ms before the expected edge and stop as soon as the transition is seen. Duty-cycle drops from ~1.0 to ~0.005 at 1 Hz PPS, making the CPU cost negligible.

This is the principal Darwin path and would be very handy for mac-based development and field use. It doesn't need cgo.

### 4. FreeBSD `<sys/timepps.h>` via cgo

The standard BSD PPS API as specified by RFC 2783, reached through the system's C header. FreeBSD's kernel implements this properly (unlike Darwin, which does not) — given a PPS-capable serial driver or a `/dev/pps*` equivalent, userspace can call `time_pps_fetch` and get kernel-timestamped edges. Would require cgo, which is permitted by project policy outside Linux. satpulse nominally builds on FreeBSD today (though untested), so this is the natural PPS path if a FreeBSD deployment ever comes up; (3) remains the Darwin story.

## Appendix: Linux PPS in pure Go

The reader is pure Go at runtime. cgo appears only as a build-time code generator (`go tool cgo -godefs`) to resolve constants from `<linux/pps.h>`; its output is a normal Go source file that ships in the repository. No cgo is linked into the satpulse binary.

### What `x/sys/unix` provides and what is missing

`golang.org/x/sys/unix` already exports, per-arch:

- Ioctl request numbers: `PPS_GETCAP`, `PPS_GETPARAMS`, `PPS_SETPARAMS`, `PPS_FETCH`.
- Structs: `PPSKParams`, `PPSFData`, `PPSKInfo`, `PPSKTime`, and their field names.

Missing (as of `x/sys v0.43.0`): the mode-flag and time-flag constants defined alongside in the kernel header:

- `PPS_CAPTUREASSERT`, `PPS_CAPTURECLEAR`, `PPS_CAPTUREBOTH` — which edges to capture.
- `PPS_OFFSETASSERT`, `PPS_OFFSETCLEAR` — per-side constant-offset compensation.
- `PPS_ECHOASSERT`, `PPS_ECHOCLEAR` — echo edges back to output (rare hardware).
- `PPS_CANWAIT`, `PPS_CANPOLL` — capability bits returned by `PPS_GETCAP`.
- `PPS_TSFMT_TSPEC`, `PPS_TSFMT_NTPFP` — timestamp format selection; TSPEC is the kernel default when no format bit is set.
- `PPS_TIME_INVALID` — `pps_ktime.flags` value for "no timeout, block until edge".

Of these, only `PPS_CAPTUREASSERT` and/or `PPS_CAPTURECLEAR` are strictly required — without at least one, `PPS_FETCH` blocks forever because nothing is captured. The rest are nice to have for readable code. Worth filing a patch to upstream `x/sys`; in the meantime the reader carries its own generated copy.

### Defs file (`consts_linux_pps.go`)

```go
//go:build ignore

package ppsreader

/*
#include <linux/pps.h>
*/
import "C"

const (
    PPS_CAPTUREASSERT = C.PPS_CAPTUREASSERT
    PPS_CAPTURECLEAR  = C.PPS_CAPTURECLEAR
    PPS_CAPTUREBOTH   = C.PPS_CAPTUREBOTH
    PPS_OFFSETASSERT  = C.PPS_OFFSETASSERT
    PPS_OFFSETCLEAR   = C.PPS_OFFSETCLEAR
    PPS_ECHOASSERT    = C.PPS_ECHOASSERT
    PPS_ECHOCLEAR     = C.PPS_ECHOCLEAR
    PPS_CANWAIT       = C.PPS_CANWAIT
    PPS_CANPOLL       = C.PPS_CANPOLL
    PPS_TSFMT_TSPEC   = C.PPS_TSFMT_TSPEC
    PPS_TSFMT_NTPFP   = C.PPS_TSFMT_NTPFP
    PPS_TIME_INVALID  = C.PPS_TIME_INVALID
)
```

The `//go:build ignore` tag keeps this file out of the normal build; it is consumed only by `cgo -godefs`.

### go:generate directive

In a regular `.go` file in the same package (e.g., `doc.go`):

```go
//go:generate go tool cgo -godefs consts_linux_pps.go > zconsts_linux_pps.go
```

Running `go generate ./time/internal/ppsreader/...` produces `zconsts_linux_pps.go` and commits it to the repo. Regeneration is only needed if the kernel header adds new constants worth picking up.

### Generated output (`zconsts_linux_pps.go`)

```go
// Code generated by cmd/cgo -godefs; DO NOT EDIT.
// cgo -godefs consts_linux_pps.go

//go:build linux

package ppsreader

const (
    PPS_CAPTUREASSERT = 0x1
    PPS_CAPTURECLEAR  = 0x2
    PPS_CAPTUREBOTH   = 0x3
    PPS_OFFSETASSERT  = 0x10
    PPS_OFFSETCLEAR   = 0x20
    PPS_ECHOASSERT    = 0x40
    PPS_ECHOCLEAR     = 0x80
    PPS_CANWAIT       = 0x100
    PPS_CANPOLL       = 0x200
    PPS_TSFMT_TSPEC   = 0x1000
    PPS_TSFMT_NTPFP   = 0x2000
    PPS_TIME_INVALID  = 0x1
)
```

Committed to the repository. `//go:build linux` keeps it out of darwin/BSD builds, where a separate (cgo-based) backend would live.

### Reader sketch

```go
package ppsreader

import (
    "os"
    "unsafe"

    "golang.org/x/sys/unix"
)

func ioctlPtr(fd int, req uint, p unsafe.Pointer) error {
    _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(p))
    if errno != 0 {
        return errno
    }
    return nil
}

func Run(devicePath string) error {
    f, err := os.OpenFile(devicePath, os.O_RDWR, 0)
    if err != nil {
        return err
    }
    defer f.Close()

    if _, err := unix.IoctlGetInt(int(f.Fd()), unix.PPS_GETCAP); err != nil {
        return err
    }

    // mode bits: PPS_CAPTUREASSERT for rising, PPS_CAPTURECLEAR for falling.
    // Exactly one, per [pps].edge in the real reader.
    params := unix.PPSKParams{
        Api_version: 1, // PPS_API_VERS_1
        Mode:        PPS_CAPTUREASSERT,
    }
    if err := ioctlPtr(int(f.Fd()), unix.PPS_SETPARAMS, unsafe.Pointer(&params)); err != nil {
        return err
    }

    for {
        fd := unix.PPSFData{}
        fd.Timeout.Flags = PPS_TIME_INVALID

        if err := ioctlPtr(int(f.Fd()), unix.PPS_FETCH, unsafe.Pointer(&fd)); err != nil {
            if err == unix.EINTR {
                continue
            }
            return err
        }

        // fd.Info.Assert_tu.{Sec,Nsec}, Clear_tu.{Sec,Nsec},
        // Assert_sequence, Clear_sequence — emit events here.
    }
}
```

Notes:

- `PPS_TSFMT_TSPEC` is omitted from the mode word: the kernel defaults to timespec format when no `TSFMT` bit is set (`drivers/pps/pps.c`, `PPS_SETPARAMS` branch).
- `PPS_FETCH` blocks in the kernel. Run the loop on its own goroutine dedicated to the fd.
- Interruption model: a second goroutine closing the fd unblocks `PPS_FETCH` with `EBADF`, which the reader treats as a clean shutdown signal.
