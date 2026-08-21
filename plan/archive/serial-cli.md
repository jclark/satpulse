# Revised satpulsetool serial CLI (#408)

## Motivation

The serial subcommand's option surface grew mode by mode and is
inconsistent with the rest of the tool family:

- The commands that open a GPS serial device (gps, satpulsewb,
  satpulsed) name it with `-d/--serial-device` and its speed with
  `-s/--device-speed`; serial uses a positional port and gives `-s`
  to `--detect-speed`.
- serial's `--speed` is a false friend: it has the semantics of gps
  `-s/--device-speed` (the host port's speed) but the name of gps
  `--speed` (the receiver's configured baud rate).
- `--packet-log` works only with `--detect-speed`, although PPS
  monitoring also drains input, and a packet log correlated with
  edge timestamps is exactly what diagnosing a pin needs (real PPS
  vs TX crosstalk).

The revision re-letters the command to match the family and makes
the capabilities compose.

## Design rules

Three rules produce the whole grammar, and each is explainable to a
user in one sentence.

1. gps talks, serial listens. serial never transmits a byte to the
   device: it describes ports, reads what a receiver emits, and
   polls modem-control lines. Passive packet capture is therefore
   serial's job, guaranteed passive by the nature of the command
   rather than (as in gps today) by which options are absent. This
   is also what makes reading safe enough to be the default mode.
   Capture is not merely safe in serial but at home there: it is
   reading with the result recorded, alongside reading to detect
   the speed and reading to time edges.

2. serial is patterned on gps, not sdp. `-d/--serial-device` and
   `-s/--device-speed` take gps's exact names and letters, and
   naming a device performs a useful default action, as gps
   defaults to `--show-receiver` -- except that serial listens
   where gps talks. (`-i` colliding with sdp's `--extts` letter is
   accepted for the same reason.)

3. A target licenses opening. `-d` names one port and `-a` names
   all discovered ports; a target is opened and read, and reading's
   default purpose is detecting the speed of a connected receiver
   -- the command's primary job, so it is the unmarked case. The
   marked exception is `-i/--info`: describe the target without
   opening it. Reading requires an explicit target; `-i`'s default
   target is `-a`, and `-i` itself is the default when no target is
   named, so bare `serial` lists every port without opening
   anything.

## Command surface

    serial                              info on all ports (never opens)
    serial -i -d DEV                    info on one port (never opens)
    serial -a                           detect speed of every port
    serial -d DEV                       detect one port's speed
    serial -d DEV --packet-log F        detect speed, logging packets
    serial -p cts -a                    watch CTS on every port
    serial -p cts -d DEV -t 30          monitor PPS edges on one port
    serial -p cts -s 38400 -d DEV --packet-log F
                                        edges + packets at a set speed
    serial -s 38400 -d DEV --packet-log F -t 30
                                        passive capture, known speed
    serial -s 0 -d DEV --packet-log F   passive capture, current speed

Constraints:

- Reading requires an explicit target: `-d` or `-a` (mutually
  exclusive). `-i`'s default target is `-a`.
- `-i` cannot be combined with `-s`, `-p`, `-t`, or `--packet-log`.
- `-s` requires `-d`: setting the speed of every discovered port is
  never wanted.
- `-s` is numeric only; 0 keeps the port's current speed (the
  existing `--speed` convention), which is the spelling for capture
  at the current speed.
- `-s` given with neither `-p` nor `--packet-log` is an error:
  nothing to detect or record.
- `-p` without `-s` leaves the speed alone: modem-line polling does
  not need a correct baud rate, and the all-ports pin scan must not
  autobaud every port. Detect-then-monitor is shell composition:
  `serial -p cts -d DEV -s $(serial -d DEV)`.
- `--packet-log` requires `-d`.
- `-t` bounds edge monitoring (default 10 s) and capture (default:
  until interrupted, matching `gps --capture 0`); speed detection
  keeps its internal time budget.
- Exit status keeps 0/1/2; a capture that logged no packets exits 2
  by symmetry with the other "no data found" cases.

## User-facing explanation

The man page DESCRIPTION becomes a short story: the serial command
reads from the serial ports of the host, and never transmits to a
connected device. Naming a port with `-d`, or all ports with `-a`,
opens the target and reads from it: by default this detects the
speed of a connected GPS receiver from the data being emitted by
the receiver; with `-p` it detects PPS edges on a modem-control
input; with `--packet-log` it records the packets received. With
`-i` it instead describes the target ports without opening them;
when no target is named, it behaves as if `-i` were specified.

The two new entries in house style:

    -i, --info
    : Describe the target ports without opening them.
    The default target is -a.
    This option is the default when neither -d nor -a is specified.
    Cannot be combined with -s, -p, -t, or --packet-log.

    -a, --all
    : Select all discovered serial ports as the target.
    Cannot be combined with -d.

## Port listing output

The human listing moves from the composed display string to one
key=value line per port, udev/logfmt style, so VID/PID and aliases
are visible without `-j`:

    device=/dev/ttyACM0 vid=1546 pid=01a9 serial="DBENIA5X" alias=/dev/gps0 display="..."

- Keys mirror the JSONL field names, except that aliases print as a
  singular `alias=` key repeated once per alias.
- vid/pid print as four-digit lower-case hex (lsusb convention);
  the JSONL numbers stay numeric.
- Values that can contain spaces are quoted; paths are bare.
- `display=` prints the Display string verbatim. On Linux this
  repeats the device and aliases, which is accepted: Display's
  composition is platform-dependent and satpulsewb consumes it, so
  there is no cross-platform decomposition to print instead.
- The single-port speed result stays a bare number so
  `gps -s $(serial -d DEV)` keeps working.
- All-ports edge output gains a device prefix (edges from several
  ports interleave), and JSONL edge objects gain a device field.

## Alternatives rejected

- An explicit `-r/--read` flag licensing opening, implied by `-s`
  and `-p`: it marks the command's normal operation instead of the
  exception -- every speed detection, the primary use, pays a flag
  -- and it needs implication rules ("implied by -s and -p;
  required but not caused by -t and --packet-log") that the target
  rule makes unnecessary. It also made `-d` alone a passive
  describe, the only `-d` in the family that does not connect.
- `-s auto` for requesting speed detection: bare `-s` cannot
  default to auto (a pflag optional-value flag accepts explicit
  values only as `-s=38400`), and with detection the default
  purpose of reading it is unnecessary; `-s` stays purely numeric.
- `-f/--config-file`: the `[serial]` table now includes `pps.pin`,
  so importing the file would make the command's mode depend on file
  content, and importing only device and speed saves little typing
  on a command whose common invocations need no device at all.
  Checking the production wiring stays explicit:
  `serial -p cts -d DEV -s N`.

## Phasing

Phase 1 (serial-cli branch): everything except `-p`. Replace the positional
port with `-d`; add `-a` and `-i`, with reading as the normal mode
and speed detection as its default purpose; give `-s` to
`--device-speed`; the `--packet-log` rules, `-t`, passive capture,
and the key=value listing. The renames of `serial -s` and the
positional port are breaking and accepted (near-zero users). This
lands before phase 2 so serial offers passive capture before gps
loses it.

Phase 2 (serial-cli branch): remove passive capture from gps. `--capture`
keeps its post-configuration role; `--packet-log` plus `--capture`
with no other action falls through to the default show-receiver
probe, a deliberate behavior change. The "capture without probing"
example moves from satpulsetool-gps(1) to satpulsetool-serial(1);
skills and docs using the old idiom are updated.

Phase 3 (serial-pps branch, after phases 1 and 2 have merged to
master and master has been merged in): the PPS surface.
`--detect-pps` becomes `-p` (long name `--pin` vs `--pps-pin` still
undecided), requiring a target; the branch's `--speed` and
`--timeout` are subsumed by phase 1's `-s` and `-t`; `-p` with `-a`
scans all ports in parallel with per-port failures on stderr, exit
2 when no port pulses; `--packet-log` composes with `-p`; edge
output gains the device tagging described above.
