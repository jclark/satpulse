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

The output has `Vendor:`, `Hardware:`, `Firmware:`, `Supported GNSS:`, and
`Packet formats detected:` lines, and a `Supports:` line that is printed only
when the receiver has high-level configuration support. `--json` gives the
same as one JSON object (`receiver`, `supports`, `packetFormats`).

## High-level or low-level?

Decide in this order:

1. **Is the setting in the data model?** The high-level options are the ones
   in the `gps` synopsis: constellations, bands and signals, elevation mask,
   PPS, antenna cable delay, timing GNSS, survey and fixed position, NMEA and
   binary output selection, RTCM output, serial speed, save/reload/reset. If
   the setting is not one of these, it is low-level.
2. **Does this receiver have a `Supports:` line?** If not, everything is
   low-level. The line lists optional features by name; options that need a
   named feature are:

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

   Other high-level options need only the presence of the line. Requesting an
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
