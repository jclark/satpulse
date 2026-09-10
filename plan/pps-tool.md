# pps subcommand for satpulsetool (#461)

Related: #412 (satpulsed reading a kernel PPS device).

## Purpose

A `pps` subcommand of `satpulsetool` for working with PPS signals
outside the serial port: kernel PPS devices (`/dev/ppsN`) and, on a
Raspberry Pi, a GPIO pin polled directly. It is in the same family as
`serial` and `sdp`, replaces `ppstest` for checking a PPS device, and is
the command-line counterpart of the daemon's GPIO source
(`plan/gpio-poll.md`). Linux only. No cgo.

## Syntax

```
satpulsetool pps [-h|--help] [-d|--pps-device path] [-g|--gpio-pin N]
                 [--bias] [-e|--every-pulse]
                 [--cpu N] [--priority N] [--outlier-ratio ratio]
                 [-t|--timeout seconds] [-j|--jsonl]
```

No option is required. The selectors choose the mode:

- No selector: list the kernel PPS devices from `/sys/class/pps` (name,
  source path, mode), as `serial` and `sdp` show their objects when given
  nothing. It changes nothing.
- `-d`: print the device's assert timestamps as they arrive.
- `-g`: poll the GPIO and print the edges it catches, with the daemon's
  poller and the same options as `[sample.pps.gpio]`.
- `--bias`, which requires both `-d` and `-g`: estimate the kernel
  timestamp bias by comparing the device's timestamps with polled edges
  of the same pulses. See the section below.

## Decided

- Selectors follow `serial`: `-d|--pps-device` like `-d|--serial-device`,
  not a positional argument. Neither selector has a default; `-g 18` and
  `/dev/pps0` are typed, since an absent selector means a different mode.
- `-g|--gpio-pin` names the GPIO as the `pps-gpio` overlay's `gpiopin`
  parameter and the `gpio.pin` config key do, so a user copies the number
  from the `dtoverlay` line. It is the GPIO number, not the header pin.
  `-g` is the letter ppsbias uses; `-p` means a pin name in `serial` and
  a pin index in `sdp`, and `--pin` alone would invite the header number.
- Options that mirror a `[sample.pps.gpio]` key take the key's name in
  kebab case and have no short letter, as `serial` does for its
  `[sample.serial.pps]` keys: `--cpu`, `--priority`, `--outlier-ratio`.
  Short letters can be added if typing them proves tiresome. `--cpu` and
  `--priority` apply to `-g` and `--bias`; `--outlier-ratio` to `-g` only.
- Bias estimation is named by `--bias`, the way `sdp` names its modes,
  rather than implied by giving both selectors: it runs for minutes and
  alternates polling in a way plain edge printing does not, so the user
  asks for it by name. `--bias` rather than `--offset`: it names what is
  measured, as ppsbias and the blog post do, and an option that measures
  should not be named after the chrony option the result is typed into;
  the man page says the value is for chrony's `offset`.
- `-e|--every-pulse` is ppsbias's `-e`: poll every pulse instead of
  alternate ones. It keeps ppsbias's letter, since unlike the config-key
  options it has no config counterpart to mirror. ppsbias's window and
  spacing (`-w`, `-s`) get no option: 1 ms each side, reads back to back.
- `--echo` enables the device's echo-on-assert output while the command
  runs. Applies to `-d`.
- `-t|--timeout` bounds how long the command runs, in seconds, counted
  from the start of the command in every mode (ppsbias counts from its
  first synchronised pulse). The default is 10, as for `serial -p` and
  ppsbias; 0 runs until interrupted.
- Exit status 2 when no timestamp or edge arrived, or `--bias` produced
  no estimate, as `serial -p` and `sdp -i`. An interrupted `--bias` run
  prints its summary and exits 0 (ppsbias exits 128 plus the signal).
- `-j|--jsonl` selects JSON lines output. The edge object follows
  `serial -p`: a `device` string (for `-d`) or `gpio` number (for `-g`),
  an RFC 3339 UTC timestamp `t` with nanoseconds, and for polled edges
  `uncertainty` in seconds and `settling`. The text form is the time of
  day with nanoseconds.
- `--bias` output works like the other modes: one message per second
  to stdout, then a summary as the last line. The per-second message
  carries the raw observations of that second and nothing derived: the
  kernel timestamp with its sequence number as `-d` prints them, and,
  for a polled second, the polled edge with its bracket as `-g` prints
  them; whichever of the two the second produced. A second with no
  kernel timestamp, or a polled second with no edge, is a message
  without that part. In JSONL an object with the same fields. The
  estimate is computed from the collected observations and appears only
  in the summary, so nothing printed is ever retracted and no status
  vocabulary is needed; whatever ppsbias's verbose statuses say is
  recoverable from the messages.
  The text summary is the median bias alone in seconds, positive for a
  late timestamp, in ppsbias's `12.7e-6` form, so a script can take the
  last line; the JSONL summary is an object with the median, mean,
  standard deviation, sample count, median and maximum bracket, and how
  the run ended, all durations in seconds. The standard deviation and
  count are the uncertainty; nothing more is derived. Keys are camelCase
  as in the rest of SatPulse's JSON. Half-nanosecond fractions, which
  ppsbias carries through its midpoint arithmetic, are dropped: Go's
  nanosecond integers are far below the bracket.
- What ppsbias prints as its first line, the configuration (device,
  GPIO, mode, window, affinity, timer slack, PPS capabilities), goes to
  the log at info level; its warning that the median bracket exceeds
  1.5 us, which means the poller is sharing a CPU with the interrupt,
  goes to the log at warn level. Neither is output.
- Time values are float seconds.
- Data goes to stdout, diagnostics to stderr, following `serial` and `sdp`.
- ppsbias's `-v` is the global `satpulsetool -v`. Its `-w` window and
  `-s` spacing have no counterpart: the poller adapts them.

## Branches

The command lands in pieces so that each is reviewable alone and the
daemon's GPIO source does not carry the tool:

1. `gpiomem` off master: `gps/lib/gpiomem`, the register read.
2. `pps-tool` off master: `internal/ppscmd` with the listing, `-d`, `-t`,
   `-j`, the man page and NEWS entry. Needs only `gps/lib/kpps`.
3. `pps-package` off master: the source-neutral `gps/app/pps` package.
4. `gpio-poll` on all three: the daemon source, and the `-g` mode with
   its options added to `ppscmd`.
5. Bias mode on `gpiomem` and `pps-tool`. It has its own polling loop and
   its own thread setup and sleep (about 40 lines that the daemon's GPIO
   source also has, each tuned to its loop), so it does not depend on
   `pps-package` or `gpio-poll`. Built for `linux && arm64` like the
   source, with a stub elsewhere.

## Bias estimation

### What is estimated

A kernel PPS timestamp is taken in the interrupt handler, so it is later
than the physical edge by the whole interrupt path: pin, GPIO controller,
interrupt delivery, handler dispatch, clock read. On a Raspberry Pi 5 the
GPIO controller is the RP1 chip behind a PCIe link, and the path is about
12 us with default settings and about 7 us with the link's L1 power state
disabled. An NTP daemon can compensate a stable lateness with a fixed
refclock offset, positive for a late capture in chrony's convention. The
estimator measures that lateness so the user does not need a counter,
an echo output or any extra wiring.

### Method

Two observations are made of the same pulses.

1. The kernel timestamp of each pulse, read from the PPS device.
2. A polled estimate of the physical edge. Around the predicted arrival of
   a pulse the program reads the pin's level in a tight loop, recording
   the clock before and after each read. A read's sample instant is taken
   as the midpoint of its two clock readings, so the read's own latency
   cancels. The edge is the midpoint between the last low sample instant
   and the first high one; the interval between them, the bracket, bounds
   the error of that one estimate. On a Pi 5 a read is one PCIe round
   trip of about 1 us, so brackets are about 1 to 1.5 us.

The polled edge is an unbiased estimate of the physical edge; averaged over
hundreds of pulses its error falls well below the bracket. The difference
kernel timestamp minus polled edge is therefore the kernel's lateness on
that pulse.

### Correcting for the poller's own effect

Polling changes what it measures. The reads keep the PCIe link out of L1
and a CPU busy, and they contend with the interrupt path on the bus, so
the kernel stamps pulses differently while polling than in normal
operation. On the Pi 5 the difference is about 5 us with L1 enabled.

The estimator therefore polls only alternate seconds. With kernel
timestamps K0, K1, K2 and polled edges P0 and P2, the RP1 is busy for K0
and K2 but idle for K1. P1 is inferred as the midpoint of P0 and P2,
which also cancels linear clock drift, and the estimate for that pulse
is K1 - P1: the lateness of a timestamp taken while nothing was polling.
The result is the median of those per-pulse estimates, with their mean
and standard deviation alongside. With `-e` every pulse is polled and
the estimate for each is K - P, the lateness while polling. The method
assumes only that the pulse source is periodic to well under a
microsecond over two seconds and that the system clock's frequency is
stable over the same interval. This is how ppsbias computes it.

### Polling schedule

Bias mode uses its own loop, as ppsbias does, not the daemon's adaptive
poller. What the poller adds -- finding the pulse with no reference,
shrinking the window to the minimum, recovering from lost pulses -- is
for a daemon running indefinitely; the bias estimate has the kernel
timestamps to predict from, wants every other second skipped, which the
poller's tracking would count as misses, and runs for minutes, so a
fixed window's CPU cost is irrelevant. Both loops place the edge at the
bracket midpoint, so the estimate is the same; the validation below was
done with the simple loop. The two modes share `gpiomem` for the read
and nothing else: the thread setup (affinity, SCHED_FIFO, timer slack)
and the absolute clock_nanosleep are a few lines each and are written
separately for each loop rather than shared. Bias mode rejects no edge
for its bracket width, as ppsbias does not: the median absorbs a stalled
read, and a per-pulse line shows its bracket. `--outlier-ratio` belongs
to `-g` alone.

- Predict the next edge as the last kernel timestamp plus one period.
- Sleep with an absolute clock_nanosleep to a fixed time before the
  prediction, with timer slack reduced to a microsecond, then poll back to
  back until the edge is seen or the window closes. A window of one
  millisecond each side costs about 0.2 percent of one core.
- Run the loop on a locked OS thread with the affinity and SCHED_FIFO
  priority of `--cpu` and `--priority`, and timer slack reduced, with no
  allocation inside the window. ppsbias sets only the slack and leaves
  pinning to `taskset`.
- A second in which the pin is already high when the window opens, or no
  edge is seen, counts as a polling failure. Missing or duplicated kernel
  events are counted from the PPS sequence numbers.

### Clocks and run control

The estimate compares kernel timestamps on the realtime clock with polled
reads on the realtime clock, while the schedule runs on the monotonic
clock. ppsbias tracks the realtime-minus-monotonic offset as an interval
across every pair of reads, places each window through it, and ends the
run with a `clockStep` status if the interval ever moves, since samples
across a step are not comparable; the Go version does the same.

The run waits up to 3 s for a fresh assert timestamp, then predicts each
slot as the last good timestamp plus one second. After an event that
does not fit its slot it resynchronises the same way. Three consecutive
slots without an event, or a failed resynchronisation, end the run early
with the summary of what was collected. `-t` counts from the start of
the command, not from synchronisation.

### PPS device parameters

Open. The kernel PPS parameters (capture mode, assert and clear offsets)
belong to the device, and any process with it open can change them for
every other. chrony sets the capture mode once when it opens the device,
to the edge it uses (assert by default, clear with its `:clear` option),
and never sets an offset; its `offset` option is applied by chrony
itself, as ntpd's `time1` is by ntpd. ppstest ORs assert capture into
the mode and zeroes the assert offset on every run, so it alters the
device under a running daemon. ppsbias changes nothing: it requires
assert capture to be on, subtracts the assert offset from every
timestamp, and aborts if the mode or offset changes during the run. The
`-d` mode reads the device without looking at the parameters, and ends
with exit 2 and a message when clear timestamps arrive but no asserts.
Whether `--bias` needs more than that is not decided.

### Validation

Prototyped in C on 2026-09-05 and run on a Raspberry Pi 5 (kernel
6.18.39, ondemand governor):

- L1 enabled: 11.9, 12.0, 12.2 and 12.6 us over runs of 60 to 1200 s.
- L1 disabled: 7.1 and 7.3 us.
- The estimate was unchanged when the polling path changed from the GPIO
  character device ioctl to a bare register read, and when reads were
  spaced 4 us apart instead of back to back, while the lateness measured
  on the polled pulses themselves moved with each change. The
  interpolation does what it claims.
- Kernel timestamps from GPIO edge events and from the pps-gpio device
  gave the same result within noise.
- A tinyGTC counter measuring pulse-to-echo the same day gave 14.85 us
  median. The 2.85 us difference is the post-timestamp path, consistent
  with the roughly 2 us of software measured by kprobes plus the write
  reaching the pad.
- The machine's own lateness wanders by about 0.5 us between sessions,
  which is the useful precision of any fixed offset.

### Reading the pin

The polled reads need access to the pin's level. Two paths were
prototyped. The tool offers only the gpiomem register read, which the
`-g` mode uses anyway, with the GPIO given by `-g`; `gpiomem` finds the
SoC from `/proc/device-tree/compatible`, so there is no model option.
The character-device path is not offered: it needs the line free, so
pps-gpio would have to be unbound, which destroys the device node under
any process holding it. The device-tree lookup below is a possible
later convenience, not a requirement.

- **GPIO character device.** Portable to any Linux board, no cgo, but a
  line request fails while pps-gpio owns the line, so the driver would
  have to be unbound for the run. The same request can also supply the
  kernel timestamps as edge events, which the prototype showed agree with
  pps-gpio's.
- **gpiomem register read.** Raspberry Pi only. Needs no ownership of the
  line, so pps-gpio stays bound and the PPS device stays available. The
  register can be found from `/dev/ppsN` alone: the pps source name gives
  the pps-gpio platform device, its device-tree `gpios` property gives the
  controller phandle and line, the controller's compatible string selects
  the register layout (RP1 or BCM283x), and the gpiomem device at the same
  address gives the mapping and its `/dev` name. Verified on a Pi 5; the
  BCM283x half is from documentation.

## Not yet decided

- The PPS device parameters, above.
- Whether other operations on the device belong here.
- Man page text.

## Reference

- `plan/archive/sdp-tool.md` and `plan/archive/serial-cli.md` for the
  family's conventions.
- `gps/lib/kpps` for reading the PPS device.
- `plan/gpio-poll.md` for the daemon's GPIO source and `gps/lib/gpiomem`.
