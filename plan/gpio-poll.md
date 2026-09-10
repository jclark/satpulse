# GPIO PPS polling as a pulse source (issue pending)

Issue not yet filed; the heading gets its number when it is.
Related: #412 (kernel PPS device as a pulse source).

## Overview

satpulsed gains a pulse source that reads a Raspberry Pi GPIO pin
directly. Around the predicted arrival of each pulse it reads the pin's
level in a tight loop with a clock reading before and after each read; the
edge is the midpoint between the last low read and the first high read,
and the interval between them, the bracket, bounds the error. This is the
serial PPS poll method applied to a GPIO register instead of a
modem-control line. Like serial PPS, it is timestamped by the system
clock, gets its UTC second from the receiver's time messages, and produces
refclock samples for systems without a PTP hardware clock.

Measurements on a Pi 5 under compile load showed that polling needs its
own CPU away from the one handling the PPS interrupt, and SCHED_FIFO
priority; with those, nearly every pulse was caught, with sub-microsecond
error against a PTP-disciplined PHC. Kernel PPS timestamps on the same
pulses were 12 to 20 us late with several microseconds of spread.

Linux and arm64 only initially. No cgo.

## Configuration

```
[pps]
gpio.pin = 18

[sample.pps]
delayUncertainty = 0.005
maxDelay = 0.8

[sample.pps.gpio]
cpu = 3
priority = 40
```

`[pps]` describes a pulse input that is not a serial modem-control line;
`[serial]` is unchanged. `gpio.pin` selects the GPIO source. `device` is
reserved for the kernel PPS device (#412). `[pps]` names one kind, and is
exclusive with `[serial] pps.pin` and `[phc] interface`.

`[sample.pps]` holds the keys that pair edges with receiver messages, which
apply to either kind. `[sample.pps.gpio]` holds the polling keys: the CPU
to pin the poller to and its SCHED_FIFO priority. A `preWarm` key is added
only if the GPIO source proves to need it. There is no `maxWakeupLatency`:
cpu_dma_latency is for the Intel serial-polling case.

## Packages

`gps/app/pps` is the plain pulse package. `serialpps` imports it, since
serial PPS is PPS piggybacked on the serial port while GPIO PPS piggybacks
on nothing. `pps` holds the source-neutral parts now in `serialpps`:
`Edge`, `CandidateEdge`, `Sample`, `Generator`, `PollStats`, and the
poller, exported as `Poll` taking an in-pulse reader and a parameter
struct for the constants that depend on state-read cost: initial polls,
minimum spacing and sleep quantum, which are 64 / 50 us / 1 ms for serial
and 10000 / 1 us / 10 us for GPIO. The GPIO source lives in `pps` behind
build tags; the kernel device will too. `serialpps` keeps `Config`,
`Detect`, `Wait`, `Wiring`, `Polarity` and its reader interfaces under
their current names.

`NewGenerator` takes a `pps` config type holding `delayUncertainty` and
`maxDelay`, which `serialpps.Config` embeds. go-toml v2 inlines an
untagged embedded struct on decode, so the `[sample.serial.pps]` keys are
unchanged. The sample config and `configs/config-schema.json` are
maintained by hand.

`gps/lib/gpiomem` (library layer, Linux-only like `gps/lib/kpps`) holds
the system-dependent and unsafe code: model detection from the device
tree, the read-only mapping of the gpiomem device, and the level read. It
is also the pin-reading path for the bias estimator planned for the
`satpulsetool pps` subcommand, which is where the GPIO source's
command-line counterpart goes.

## GPIO source

The pin level is read from the gpiomem device only, mapped read-only. The
model is detected, not configured.

The register read is `atomic.LoadUint32` (LDAR on arm64) rather than
assembly. Its adequacy is judged by the per-edge bracket statistics, which
are already measured and logged; a `dmb`-bracketed read in assembly is the
fallback if they show a loss.

The poller runs on a locked OS thread. CPU affinity and SCHED_FIFO
priority are set on that thread with `unix.SchedSetaffinity` and
`unix.SchedSetAttr` (present in the x/sys version go.mod requires). The Go
runtime creates new threads from a template thread when asked from a
locked M (`newm` in runtime/proc.go), so these settings do not leak.

Sleeps are a raw `unix.ClockNanosleep` with `TIMER_ABSTIME` and per-thread
`PR_SET_TIMERSLACK`, so the kernel wakes the FIFO thread directly rather
than through the runtime's timer path, which rounds to 1 ms and passes
through normal-priority threads. Context cancellation cannot interrupt a
raw sleep; sleeps are at most one period, which the service's stop
timeout tolerates.

Three runtime effects remain. Sysmon retakes a P from a thread in a
syscall after 10 ms, so returning from the inter-window sleep reacquires
a P, before the window opens. Sysmon force-preempts a goroutine running
over 10 ms, and GC stop-the-world preempts it too; a preempted locked
goroutine returns through the global run queue. Inside a window these
widen the bracket, which the dispatcher's uncertainty gate rejects and the
tracking loop counts as a miss: lost samples, never wrong ones.

The systemd unit's capability bounding set omits CAP_SYS_NICE, which
SCHED_FIFO needs; the unit gains it (or an RTPRIO limit).

## Phasing

Two stacked branches:

1. Move the source-neutral parts of `serialpps` into `gps/app/pps` and
   make `serialpps`, the daemon, the dispatcher and serialcmd use them.
   No behaviour change.
2. Add the GPIO source: `gps/lib/gpiomem`, the GPIO backend in `pps`, the
   configuration above, the daemon wiring, and the unit change.

## Later: armhf

`atomic.LoadUint32` on GOARCH=arm is a plain load followed by a full
barrier: `DMB ISH` when runtime goarm >= 7, else the kernel user helper at
`0xffff0fa0`, so the GOARM=6 package works on ARMv7 boards too. Assembly
would have to reproduce that goarm check, a further reason to avoid it.
The Pi 3 and 4 register layouts are verified, on 64-bit systems.
Single-core ARMv6 boards have no CPU to move the PPS interrupt away
from; without pps-gpio loaded there is no edge-coincident interrupt, but
the pin must then be put into input mode another way. Whether they work
is something to measure.
