# satpulsetool gps

Configures a GPS receiver. Full reference: satpulsetool-gps(1). This file
covers what every `gps` use needs and how to choose between the two kinds of
configuration; then read exactly one of:

- `gps-highlevel.md`: device-independent options (`--pps`, `-g`, `--survey`, ...) that satpulsetool translates for the receiver
- `gps-msgfile.md`: sending tagged messages from an existing message file
- `gps-adhoc.md`: sending a one-off command written as a here document

High-level and low-level configuration cannot be combined in one invocation.

## Connecting

`-d DEV -s SPEED` for a serial device; `-f FILE` to take both from a satpulse
configuration file (only `device` and `speed` in `[serial]` are read);
`--socket PATH` for a satpulsed proxy socket instead of a serial device.

## First step: show the receiver

With connection options only, or with `--show-receiver`, satpulsetool probes
the receiver and prints what it found:

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --show-receiver
```

The question it answers is whether satpulse can work with the receiver, and
how. There are three outcomes:

- **Identified.** A configuration protocol recognized the receiver. Output is
  `High-level configuration: VENDOR`, then `Hardware:`, `Firmware:`,
  `Supported GNSS:`, `Supports:` and `Packet formats detected:`.
  High-level configuration is available.
- **Not identified, but usable.** No configuration protocol recognized the
  receiver, but it sends time, position or satellite messages. Output is
  `High-level configuration: not supported` and `Packet formats detected:`.
  Only low-level configuration is possible; this is the normal result for a
  receiver satpulse has no high-level support for.
- **Failure.** Exit status is non-zero and stderr says why. The receiver is
  producing nothing satpulse can use as it is:
  - `configuration probes could not identify GPS; no usable output (only
    formats detected: NOVAA)`: it sends only command replies (or only
    RTCM/SPARTN). This is the expected verdict for an unsupported receiver
    with no periodic output, not a fault: configure it low-level to turn
    output on.
  - `no output from GPS`: wrong device, or the receiver is unpowered or
    miswired.
  - `framing errors reading GPS output (wrong speed?)`: find the speed with
    `satpulsetool serial` (`serial.md`).
  - `corrupted GPS output (multiple processes reading from serial port?)`:
    another process, typically a satpulsed instance, holds the port.
  - `configuration probes could not identify GPS; output not in any
    recognized format`: wrong speed on a UART, or a protocol satpulse does
    not know.

`--json` gives one JSON object (`receiver`, `supports`, `packetFormats`, and
`error` on failure); `receiver` is absent when the receiver was not
identified.

## High-level or low-level?

Decide in this order:

1. **Is the setting in the data model?** The high-level options are the ones
   in the `gps` synopsis: constellations, bands and signals, elevation mask,
   PPS, antenna cable delay, timing GNSS, survey and fixed position, NMEA and
   binary output selection, RTCM output, serial speed, save/reload/reset. If
   the setting is not one of these, it is low-level.
2. **Is `High-level configuration:` a vendor, not `not supported`?** If not,
   everything is low-level. The `Supports:` line lists optional features by
   name; options that need a named feature are:

   | Option | Needs |
   |--------|-------|
   | `--band`, `--signal`, `--except-signal` | `signal` |
   | `--speed` | `speed` |
   | `--survey` / `--survey-acc` | `survey` / `surveyAcc` |
   | `--fixed-pos-ecef`, `--fixed-pos-llh` / `--fixed-pos-acc` | `fixedPos` / `fixedPosAcc` |
   | `--raw-out` | `raw` |
   | `--pvt-out survey` | `surveyMsg` |
   | `--rtcm-out MSM4` / `MSM7` | `rtcmMSM4` / `rtcmMSM7` |
   | `--rtcm-base-id` | `rtcmBaseID` |
   | `--reload` | `reload` |
   | `--show-port` | `port` |

   Other high-level options need no named feature. Requesting an
   option the receiver lacks produces a warning naming the option.
3. **Otherwise low-level.** Look for a message file for the receiver's vendor
   in the message directory and check its tags with `--show-tags`
   (`gps-msgfile.md`). Only if no tag does the job, write an ad-hoc command
   (`gps-adhoc.md`).

## Seeing the exchange

`--packet-log FILE` records every packet sent and received as JSON Lines;
`--capture N` keeps capturing for N seconds after configuration finishes (0
means until interrupted). Use this to see what commands were sent and how the
receiver answered:

```
satpulsetool gps -d /dev/ttyACM0 -s 38400 --show-config --packet-log config.jsonl --capture 5
```

Decode the log with `satpulsetool annotate` (see `decode.md`).

## Exit status

1 means the connection failed, a high-level request could not be applied, or
the receiver rejected a message; 2 is a usage error.
