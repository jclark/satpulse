---
title: Dissecting serial PPS latency
---

The upcoming SatPulse 0.3 release adds support for reading a GPS PPS signal from a modem-control pin on a serial port.
This is the classic way of building a stratum-1 NTP server: wire the time pulse to the DCD pin, and the kernel timestamps the interrupt that the UART generates when the pin changes.
Everybody knows that the accuracy you get this way is in the microsecond range, a few orders of magnitude worse than timestamping the pulse with a PTP hardware clock (PHC).
But I wanted to understand exactly where those microseconds come from.
This post describes a series of experiments that account for them rather precisely, with a couple of surprises along the way.
The main tool was bpftrace, which turned out to be capable of measuring things I had assumed were invisible to software, including delays that happen before the first point at which software can observe the interrupt.

## The setup

The measurements were made on one of my test machines: an ASUS S500SD desktop PC (12th gen Intel i5-12400, B660 chipset) running Debian 13 with a 6.12 kernel.
A u-blox EVK-X20P evaluation kit is connected to the machine's serial port over RS-232, with the receiver's time pulse wired to the DCD pin.
The time pulse marks the top of each UTC second with an error measured in nanoseconds.
The serial port is the traditional kind: a 16550A-compatible UART inside the motherboard's Super I/O chip, accessed via port I/O at the legacy address 0x3F8, interrupting on the legacy IRQ 4.

What makes the experiments possible is that the system clock on this machine is itself synchronized to GPS at the nanosecond level, using entirely separate hardware.
A second GNSS receiver's time pulse is timestamped in hardware by the PHC of an Intel i226 ethernet controller, and the PHC time is transferred to the system clock using PCIe PTM cross-timestamping.
The two receivers share one antenna through a splitter, with similar cable lengths, so their pulses should agree to within tens of nanoseconds; I have not measured the difference directly, but any plausible error is a thousand times smaller than the delays being measured here.
During these experiments chrony reported an RMS offset of 6ns to its reference.
This means that the system clock's fractional second is, for our purposes, the true time since the pulse.
So if software anywhere in the kernel reads the clock and sees .000019, we know that 19µs have genuinely elapsed since the pulse; the measurement error is three orders of magnitude smaller than the quantities being measured.

The thing being measured is the timestamp produced by the kernel PPS subsystem (the `pps-ldisc` line discipline), which SatPulse uses when available.
A perfect timestamp would be .000000.
What I actually saw, on an idle machine, was a mean offset of +107µs with a standard deviation of 56µs.

## First layer: CPU idle states

The first culprit was not a surprise once found, but the size of its contribution was.
Modern CPUs idle in deep C-states; this machine idles in states with documented exit latencies of 220µs to 680µs.
When the UART interrupt arrives, the first thing that happens is that a CPU core has to wake up, and the timestamp pays for that wake-up.

Linux provides `/dev/cpu_dma_latency`, which lets a program hold a PM QoS constraint on acceptable idle-state exit latency for as long as it keeps the file open; the idle governor then avoids states whose advertised exit latency exceeds the constraint.
Holding it at 0 (which on this machine effectively leaves only a polling idle state) transformed the measurement: the mean offset dropped to +29µs and the standard deviation collapsed from 56µs to 1.7µs.
SatPulse 0.3 will have an [opt-in configuration key](https://github.com/jclark/satpulse/pull/417) that makes the daemon hold this file while serial PPS is in use.

So about 78µs of the mean, and essentially all of the variance, was idle-state wake-up cost.
(I attribute this to the CPU cores and package together: keeping every core polling also changes package-level power states, and this experiment does not separate the two.)
That left a residual of +29µs that was strikingly stable: 1.7µs standard deviation, with occasional samples about 6µs faster than the rest.
A stable bias is in principle harmless, because chrony's refclock `offset` parameter can absorb it; the catch, which I will come back to at the end, is knowing what value to give it.
And 29µs is a lot of time for a machine that executes several instructions per nanosecond, so I wanted to know where it was going.

Back-of-envelope arithmetic said it should not be there.
The datasheet for this class of UART specifies at most 35ns from a modem-input change to asserting its interrupt output.
An LPC bus I/O read is specified at around 0.4µs on the wire, and the SERIRQ protocol that carries legacy interrupts over LPC needs at most about 2.2µs.
Adding generous allowances for everything else still left more than 20µs unexplained.

## Tracing the path with bpftrace

When the DCD pin changes, this is the path from the wire to the timestamp:

1. The UART raises its interrupt output.
2. The interrupt travels from the Super I/O chip to the chipset (as a SERIRQ frame or an eSPI virtual-wire message), then to the I/O APIC, then as a message to the CPU's local APIC.
3. The CPU vectors to the kernel's interrupt entry code.
4. The generic IRQ layer works out that this is IRQ 4 and calls the 8250 serial driver's handler.
5. The driver reads the IIR register to ask the chip why it interrupted, reads LSR to check for received data, drains any waiting bytes from the RX FIFO (two more register reads per byte), and finally reads MSR, which reveals that DCD changed.
6. The MSR result is passed to the PPS line discipline, whose first action is to read the clock. This is the timestamp.

Steps 3 onwards are all kernel code, so every boundary can be timestamped with a bpftrace probe.
I attached probes to the `irq_handler_entry` tracepoint (filtered to IRQ 4) and to kprobes on `serial8250_rx_chars`, `serial8250_read_char`, `serial8250_modem_status` and `pps_tty_dcd_change`, recording at each point both a monotonic timestamp and the fractional second of the wall clock.
(One wrinkle: BPF programs cannot read CLOCK_REALTIME, but they can read CLOCK_TAI, and since the TAI offset is a whole number of seconds, the fractional second is the same.)

The results for the top-of-second edge, over 30 edges with `/dev/cpu_dma_latency` held at 0:

| segment | mean |
|---|---|
| pulse to IRQ handler entry | 19.7µs (stdev 0.96µs) |
| IRQ handler entry to timestamp | 13.2µs |

The second number includes a few microseconds of probe overhead; without tracing it is about 10µs.
Two conclusions dropped out immediately.

First, the in-kernel half is dominated by the three register reads.
The probes bracket the MSR read almost exactly, and it took 3.4µs.
Each register read is an `inb` instruction that stalls the CPU for a full round trip: core, to chipset, to the LPC or eSPI bus, to the Super I/O chip, and back.
The on-wire bus transaction accounts for only a fraction of that (an LPC read is specified at around 0.4µs, though I have not established whether this board carries the Super I/O traffic over LPC or eSPI); how the remainder divides among the CPU, the chipset and the bus transport is not known.
Three reads at ~3µs each, plus about 1.5µs of kernel interrupt dispatch, is the ~10µs.
This is a fundamental cost of a Super I/O UART.
An SoC-integrated memory-mapped UART can complete these accesses far faster.
A PCIe UART should also improve substantially on this path, though its register reads are still non-posted round trips that can pay their own power-state latency.

Second, my leading hypothesis was refuted by a single column of the output.
I had suspected that the receiver's serial output was sitting in the RX FIFO at the moment of the pulse, and that draining it ahead of the MSR read (step 5 runs the receive path first) was a large part of the delay.
The trace counts the bytes drained per interrupt: it was zero on every single top-of-second edge.
The receiver is simply silent at the top of the second.
(The trailing edge of the pulse, 100ms later, lands in the middle of the receiver's output burst, and there the trace shows the cost clearly: about 4.8µs per pending byte. If your receiver does transmit across the top of second, this term is real.)

That left the first number as the mystery: nearly 20µs elapse before the kernel's interrupt handler runs at all.
Adding a kprobe on `__common_interrupt`, the earliest convenient probe point in the interrupt path (only the low-level entry code and `irqentry_enter` run before it), tightened the bound further: 18.7µs of the delay happens before this first software observation point, and the kernel's path from there to the driver's handler is only about 1.5µs.
Software had reached its observation floor: whatever was eating the time was upstream of the kernel.

## A red herring: the other GPS

At this point I fed the numbers to an AI model and asked for hypotheses.
Its favourite was clever: remember that this machine has a second GNSS receiver whose pulse is timestamped by the i226 ethernet controller.
That timestamp raises an interrupt too, at the same instant as the UART's, and its handler could delay the UART interrupt.

The trace made this look very plausible.
Extending the script to also probe the i226's interrupt handler showed it firing 13-15µs after each pulse and running for about 10µs, and the UART's arrival time tracked it beautifully: on the seconds when the i226 handler ran long, the UART interrupt was pushed out correspondingly late.
There was even a matching explanation for the occasional fast edges.

The definitive test was to remove the i226 interrupt entirely, by temporarily disabling the second receiver's time pulse.
The system clock coasts meanwhile; it moved by about 3µs over the couple of minutes of the test, which adds a slow trend across the samples that is far too small to affect the observed structure.
Result: apart from that offset, nothing changed.
The UART delay had exactly the same structure with the i226 interrupt gone.
The correlation was real but common-mode: something was slowing both interrupts in the same seconds, and I had been looking at two victims, not a perpetrator and a victim.

It was worth the detour, because the refutation contained the answer.
The i226 is a PCIe device delivering a message-signaled interrupt; the UART is a legacy device behind the Super I/O chip.
These paths share almost nothing, yet both showed the same ~13µs base delay.
The one thing they do share is the chipset fabric and the DMI link between the chipset and the CPU.

## Second layer: the platform sleeps too

The hypothesis this suggests is that the residual delay is another wake-up cost, one level below the CPU: the CPU-to-chipset interconnect enters a low-power state, and an interrupt arriving from either device has to wait for it to wake.
`/dev/cpu_dma_latency` does nothing about this; it only constrains CPU core idle states.

The test is simple: keep the path busy so it can never sleep, and see if the delay disappears.
I ran a tight loop reading legacy port 0x80 (the traditional, harmless POST-code port) via `/dev/port`, which continuously exercises the core-to-chipset-to-legacy-bus path, and repeated the trace.

| measurement | fabric idle | fabric kept busy |
|---|---|---|
| i226 interrupt arrival after pulse | 13µs | 4µs |
| UART interrupt arrival after pulse | 19-20µs | 5-8µs |
| PPS timestamp offset | +29µs | ~+19µs |

This is strong evidence that roughly 10µs of the residual delay is a wake-up cost paid somewhere in the platform between the devices and the CPU cores: the analogue, one level down, of the CPU idle-state exit eliminated earlier.
Whether the sleeping component is the DMI link, chipset clock gating, or the CPU uncore, this experiment cannot distinguish.
Nor are the two improvements identical: the i226 gained about 9µs, the UART 11-15µs, so on top of the shared component there may be a smaller warm/cold effect specific to the legacy path.
It also suggests the most likely explanation for the occasional fast samples in every previous run: seconds when something else happened to have woken the fabric just before the pulse.
I have not demonstrated this directly, though; the fast samples were a few microseconds slower than the kept-busy numbers, so there may be more than one idle level involved.

I should note that the busy-loop is a diagnostic, not a fix: with it running, the offsets were actually more jittery than before (the loop's own port reads contend with the interrupt handler's), so the calibrated stable +29µs is preferable in practice.
The candidate mechanisms could only be controlled from BIOS settings I have not yet experimented with.
Notably, Intel's own BIOS guidance for real-time systems recommends disabling DMI ASPM, which is circumstantial evidence pointing the same way.

## The final budget

Putting it all together, the timestamp offset for serial PPS on this machine decomposes as:

| component | idle machine | with cpu_dma_latency=0 |
|---|---|---|
| CPU/package idle-state contribution | ~78µs mean, nearly all the jitter | suppressed |
| platform wake-up (DMI/chipset/uncore) | ~10-13µs | ~10-13µs |
| interrupt delivery and kernel dispatch, warm | ~6µs | ~6µs |
| three UART register reads at ~3µs each | ~10µs | ~10µs |
| observed total | +107µs, stdev 56µs | +29µs, stdev 1.7µs |

Some practical conclusions.

The offset budget depends mostly on how your serial port is attached, and you can get good clues from userspace:

```
cat /sys/class/tty/ttyS0/io_type   # 0 = x86 port I/O access
cat /sys/class/tty/ttyS0/port      # 0x3F8 = traditional COM1 address
cat /sys/class/tty/ttyS0/irq       # 4 = traditional COM1 IRQ
readlink -f /sys/class/tty/ttyS0/device
```

None of these is individually conclusive (a PCIe UART can also expose its registers through port I/O), but the combination of port I/O at 0x3F8 on IRQ 4 is characteristic of a motherboard Super I/O UART.
A Super I/O port like this one carries a bias of a few tens of microseconds; an SoC-integrated memory-mapped UART should lose most of the register-read term, and a PCIe UART with a message-signaled interrupt should improve on both terms, although the i226 result shows that a PCIe device does not escape the shared platform wake cost.
I have a PCIe serial card on order and will report the comparison.

The bias itself is less of an enemy than the jitter, but I should be careful not to oversell calibration.
In these runs, once the idle states were constrained, the remaining +29µs was stable to under 2µs, and chrony's refclock `offset` option can compensate for a stable bias once you know it.
The problem is knowing it.
I could measure this bias only because the machine has an independent nanosecond-grade reference, and if you have that hardware, you have no reason to be taking time from a COM port.
The number is also specific to this machine; a different Super I/O, chipset or BIOS will give a different one.
What the measurements do justify elsewhere is an expectation: on this class of hardware, with idle states constrained, the timestamps will read a few tens of microseconds late, so an uncalibrated `offset` of around 30µs is probably closer to the truth than zero.
But it is an estimate, not a calibration.

And the theme of the whole investigation: it is wake-up latency all the way down.
The CPU sleeps, and the evidence says something below it sleeps too, and each layer quietly adds its exit latency to your interrupt timestamps.
Power management constraints like `cpu_dma_latency` handle the first layer but not the second.
If you have ever wondered why interrupt timing on an idle modern PC is worse than on a busy one, this is why.
