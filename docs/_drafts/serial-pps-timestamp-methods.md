---
title: Timestamping a serial PPS: kernel, wait, poll and NMEA compared
---

<!-- DRAFT: tables and setup only; prose to follow. Data: ~/serial-latency/data/matrix (B), RESULTS-abondance.lan (A). -->

## Setup

Two machines, each with its system clock disciplined by chrony from a local GNSS PPS reference, so that the fractional-second part of a timestamp is directly its offset from the true top of second.

- **Machine A**: ASUS D500MD desktop, Intel Core i5-12400 (Alder Lake), Debian 13, kernel 6.12. chrony RMS offset 11 ns. CPU idle-state exit latencies: C1E 2 µs, C6 220 µs, C8 280 µs, C10 680 µs.
- **Machine B**: Minisforum MS-01 mini PC, 12th Gen Intel Core i5-12600H (Alder Lake-P), Fedora 44, kernel 7.0. chrony RMS offset ~20 ns. CPU idle-state exit latencies: C1E 2 µs, C6 170 µs, C8 200 µs, C10 230 µs.

Adapters (the PPS pin is fixed per adapter; where the adapter has no DCD the pulse is on CTS, which rules out the kernel method):

- **16550 (LPC)** — machine A's motherboard COM1: a 16550A UART in the Super I/O chip on the LPC bus, `8250` driver, PPS on DCD. Receiver: u-blox F10N, over RS-232.
- **CP2102** — Silicon Labs CP2102, USB full-speed, behind a USB 2.0 hub, on machine A; PPS on CTS; the `cp210x` driver supports neither `TIOCMIWAIT` nor the PPS line discipline, so only poll. Receiver: u-blox M10 (UBX-M10050-KB).
- **CH343** — WCH CH343 under the generic `cdc_acm` driver, USB full-speed, same hub, machine A. `cdc_acm` cannot see CTS, so NMEA only. Receiver: u-blox M10 (UBX-M10050-KB).
- **AX99100 (PCIe)** — ASIX AX99100 single-port PCIe serial card in a Thunderbolt enclosure, `8250_pci` driver, PPS on DCD, machine B. Receiver: the same u-blox F10N as machine A's COM port, over RS-232.
- **FT232H** — FTDI FT232H, USB high-speed, `ftdi_sio`, PPS on DCD, machine B. Receiver: Zhongke ATGM332D.
- **FT232R** — FTDI FT232R, USB full-speed, `ftdi_sio`, PPS on CTS (no DCD), machine B. Receiver: Allystar TAU951M-P200.

Each port timestamps the time pulse of the receiver connected to it; the two RS-232 ports (machine A's COM port and machine B's AX99100) are connected to the same F10N. For the PPS rows the receiver makes no difference; for the NMEA rows it is the main factor.

Methods:

- **kernel** — the N_PPS line discipline timestamps the DCD interrupt in the kernel.
- **wait** — `TIOCMIWAIT` wakes the process on a modem-status change; the process reads the clock.
- **poll** — the process reads the modem status in a tight loop around the predicted edge; the edge is placed at the midpoint of the two reads that bracket it.
- **NMEA** — the arrival time of the first NMEA sentence of each second, which is what you get without a PPS wire at all.

Every configuration was measured for 120 s (about 120 edges; poll publishes ~113, the first few seconds being acquisition), one configuration at a time, with `satpulsetool serial`. The wakeup-latency column is the `--max-wakeup-latency` setting, which holds `/dev/cpu_dma_latency`: **0** disallows every CPU idle state, **10 µs** allows C1E only (on both CPUs), **none** leaves power management alone. The median is the accuracy of the method, the standard deviation its precision; the PPS table is in microseconds, the NMEA table in milliseconds.

## PPS edge timestamps

| machine | adapter | wakeup latency | method | median (µs) | sd (µs) |
|---|---|---|---|---:|---:|
| A | 16550 (LPC) | 0 | kernel | 29.0 | 1.5 |
| A | 16550 (LPC) | 0 | wait | 35.0 | 2.2 |
| A | 16550 (LPC) | 0 | poll | 0.0 | 1.1 |
| A | 16550 (LPC) | 10 µs | kernel | 34.0 | 1.4 |
| A | 16550 (LPC) | 10 µs | wait | 58.0 | 7.2 |
| A | 16550 (LPC) | 10 µs | poll | 0.0 | 2.8 |
| A | 16550 (LPC) | none | kernel | 94.5 | 31.3 |
| A | 16550 (LPC) | none | wait | 134.0 | 45.3 |
| A | 16550 (LPC) | none | poll | 0.0 | 1.9 |
| A | CP2102 | 0 | poll | 45.0 | 46.0 |
| A | CP2102 | 10 µs | poll | 84.0 | 50.8 |
| A | CP2102 | none | poll | 72.0 | 69.2 |
| B | AX99100 (PCIe) | 0 | kernel | 12.0 | 1.4 |
| B | AX99100 (PCIe) | 0 | wait | 22.0 | 2.3 |
| B | AX99100 (PCIe) | 0 | poll | −1.0 | 1.5 |
| B | AX99100 (PCIe) | 10 µs | kernel | 29.0 | 2.9 |
| B | AX99100 (PCIe) | 10 µs | wait | 94.0 | 10.5 |
| B | AX99100 (PCIe) | 10 µs | poll | 1.0 | 1.9 |
| B | AX99100 (PCIe) | none | kernel | 218.5 | 57.1 |
| B | AX99100 (PCIe) | none | wait | 343.0 | 92.9 |
| B | AX99100 (PCIe) | none | poll | 1.0 | 5.3 |
| B | FT232H | 0 | kernel | 20.0 | 6.7 |
| B | FT232H | 0 | wait | 32.0 | 7.7 |
| B | FT232H | 0 | poll | 2.0 | 6.1 |
| B | FT232H | 10 µs | kernel | 35.5 | 11.2 |
| B | FT232H | 10 µs | wait | 102.0 | 16.7 |
| B | FT232H | 10 µs | poll | 14.0 | 13.0 |
| B | FT232H | none | kernel | 215.0 | 53.1 |
| B | FT232H | none | wait | 340.0 | 99.4 |
| B | FT232H | none | poll | 16.0 | 31.3 |
| B | FT232R | 0 | wait | 61.0 | 17.3 |
| B | FT232R | 0 | poll | 27.0 | 31.5 |
| B | FT232R | 10 µs | wait | 127.0 | 20.4 |
| B | FT232R | 10 µs | poll | 28.0 | 48.0 |
| B | FT232R | none | wait | 386.5 | 87.5 |
| B | FT232R | none | poll | 63.0 | 91.0 |

## NMEA arrival times

Time of the first NMEA sentence of each second. Unlike the PPS rows, these depend on the receiver: the sentence is sent once the receiver has computed its navigation solution for that second, and newer receivers with faster processors emit it sooner after the top of the second. The baud rate then sets how long the sentence takes to arrive. The port and the host contribute little by comparison, and the wakeup-latency setting does not apply.

| machine | adapter | receiver | baud | median (ms) | sd (ms) |
|---|---|---|---:|---:|---:|
| A | 16550 (LPC) | u-blox F10N | 38400 | 64.9 | 2.5 |
| A | 16550 (LPC) | u-blox F10N | 115200 | 61.7 | 2.6 |
| A | CP2102 | u-blox M10 | 38400 | 165.5 | 8.0 |
| A | CP2102 | u-blox M10 | 115200 | 127.9 | 13.3 |
| A | CH343 | u-blox M10 | 38400 | 151.6 | 15.1 |
| B | AX99100 (PCIe) | u-blox F10N | 38400 | 64.9 | 9.5 |
| B | FT232H | Zhongke ATGM332D | 9600 | 180.2 | 18.8 |
| B | FT232R | Allystar TAU951M-P200 | 115200 | 91.8 | 3.7 |
