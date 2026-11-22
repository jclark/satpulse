---
title: "Serial PPS accuracy"
---

<!-- DRAFT: tables and setup only; prose to follow. Data: ~/serial-latency/data/matrix (B), RESULTS-abondance.lan (A). -->

## Setup

Two machines, each with its system clock disciplined by chrony from a local GNSS PPS reference, so that the fractional-second part of a timestamp is directly its offset from the true top of second.

- **Machine A**: ASUS D500MD desktop, Intel Core i5-12400 (Alder Lake), Debian 13, kernel 6.12. chrony RMS offset 11 ns. CPU idle-state exit latencies: C1E 2 µs, C6 220 µs, C8 280 µs, C10 680 µs.
- **Machine B**: Minisforum MS-01 mini PC, 12th Gen Intel Core i5-12600H (Alder Lake-P), Fedora 44, kernel 7.0. chrony RMS offset ~20 ns. CPU idle-state exit latencies: C1E 2 µs, C6 170 µs, C8 200 µs, C10 230 µs.

## Connections

| connection | machine | UART | bus | driver | PPS pin | receiver |
|---|---|---|---|---|---|---|
| Motherboard | A | 16550A in the Super I/O chip | LPC | `8250` | DCD | u-blox F10N, over RS-232 |
| Thunderbolt | B | ASIX AX99100 single-port PCIe serial card in a Thunderbolt enclosure | PCIe | `8250_pci` | DCD | u-blox F10N, over RS-232 |
| FT232H | B | FTDI FT232H | USB high-speed | `ftdi_sio` | DCD | Zhongke ATGM332D |
| FT232R | B | FTDI FT232R | USB full-speed | `ftdi_sio` | CTS | Allystar TAU951M-P200 |
| CP2102 | A | Silicon Labs CP2102 | USB full-speed, behind a USB 2.0 hub | `cp210x` | CTS | u-blox M10 (UBX-M10050-KB) |
| CH343 | A | WCH CH343 | USB full-speed, same hub | `cdc_acm` | none | u-blox M10 (UBX-M10050-KB) |

The PPS pin is fixed per connection. Where the adapter has no DCD the pulse is on CTS, which rules out the kernel method. The `cp210x` driver supports neither `TIOCMIWAIT` nor the PPS line discipline, so the CP2102 supports only poll. `cdc_acm` cannot see CTS at all, so the CH343 gives NMEA only.

Each port timestamps the time pulse of the receiver connected to it; the two RS-232 ports (Motherboard and Thunderbolt) are connected to the same F10N. For the PPS methods the receiver makes no difference; for NMEA it is the main factor.

Every configuration was measured for 120 s (about 120 edges; poll publishes ~113, the first few seconds being acquisition), one configuration at a time, with `satpulsetool serial`. The max-latency column is the `--max-wakeup-latency` setting, which holds `/dev/cpu_dma_latency`: **0** disallows every CPU idle state, **10 µs** allows C1E only (on both CPUs), **none** leaves power management alone. Accuracy is the median of the offsets, precision their standard deviation; the PPS tables are in microseconds, the NMEA table in milliseconds.

## Kernel

The N_PPS line discipline timestamps the DCD interrupt in the kernel.

| connection | max latency (µs) | accuracy (µs) | precision (µs) |
|---|---|---:|---:|
| Motherboard | 0 | 29.0 | 1.5 |
| Motherboard | 10 | 34.0 | 1.4 |
| Motherboard | none | 94.5 | 31.3 |
| Thunderbolt | 0 | 12.0 | 1.4 |
| Thunderbolt | 10 | 29.0 | 2.9 |
| Thunderbolt | none | 218.5 | 57.1 |
| FT232H | 0 | 20.0 | 6.7 |
| FT232H | 10 | 35.5 | 11.2 |
| FT232H | none | 215.0 | 53.1 |

## Wait

`TIOCMIWAIT` wakes the process on a modem-status change; the process reads the clock.

| connection | max latency (µs) | accuracy (µs) | precision (µs) |
|---|---|---:|---:|
| Motherboard | 0 | 35.0 | 2.2 |
| Motherboard | 10 | 58.0 | 7.2 |
| Motherboard | none | 134.0 | 45.3 |
| Thunderbolt | 0 | 22.0 | 2.3 |
| Thunderbolt | 10 | 94.0 | 10.5 |
| Thunderbolt | none | 343.0 | 92.9 |
| FT232H | 0 | 32.0 | 7.7 |
| FT232H | 10 | 102.0 | 16.7 |
| FT232H | none | 340.0 | 99.4 |
| FT232R | 0 | 61.0 | 17.3 |
| FT232R | 10 | 127.0 | 20.4 |
| FT232R | none | 386.5 | 87.5 |

## Poll

The process reads the modem status in a tight loop around the predicted edge; the edge is placed at the midpoint of the two reads that bracket it.

| connection | max latency (µs) | accuracy (µs) | precision (µs) |
|---|---|---:|---:|
| Motherboard | 0 | 0.0 | 1.1 |
| Motherboard | 10 | 0.0 | 2.8 |
| Motherboard | none | 0.0 | 1.9 |
| Thunderbolt | 0 | −1.0 | 1.5 |
| Thunderbolt | 10 | 1.0 | 1.9 |
| Thunderbolt | none | 1.0 | 5.3 |
| FT232H | 0 | 2.0 | 6.1 |
| FT232H | 10 | 14.0 | 13.0 |
| FT232H | none | 16.0 | 31.3 |
| FT232R | 0 | 27.0 | 31.5 |
| FT232R | 10 | 28.0 | 48.0 |
| FT232R | none | 63.0 | 91.0 |
| CP2102 | 0 | 45.0 | 46.0 |
| CP2102 | 10 | 84.0 | 50.8 |
| CP2102 | none | 72.0 | 69.2 |

## NMEA

The arrival time of the first NMEA sentence of each second, which is what you get without a PPS wire at all. Unlike the PPS methods, this depends on the receiver: the sentence is sent once the receiver has computed its navigation solution for that second, and newer receivers with faster processors emit it sooner after the top of the second. The baud rate then sets how long the sentence takes to arrive. The port and the host contribute little by comparison, and the max-latency setting does not apply.

| connection | baud | accuracy (ms) | precision (ms) |
|---|---:|---:|---:|
| Motherboard | 38400 | 64.9 | 2.5 |
| Motherboard | 115200 | 61.7 | 2.6 |
| Thunderbolt | 38400 | 64.9 | 9.5 |
| FT232H | 9600 | 180.2 | 18.8 |
| FT232R | 115200 | 91.8 | 3.7 |
| CP2102 | 38400 | 165.5 | 8.0 |
| CP2102 | 115200 | 127.9 | 13.3 |
| CH343 | 38400 | 151.6 | 15.1 |
