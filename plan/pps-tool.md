# pps subcommand for satpulsetool (issue pending)

Issue not yet filed; the heading gets its number when it is.
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
                 [--cpu N] [--priority N] [--max-bracket seconds]
                 [-t|--timeout seconds] [-j|--jsonl]
```

No option is required. The selectors choose the mode:

- No selector: list the kernel PPS devices from `/sys/class/pps` (name,
  source path, mode), as `serial` and `sdp` show their objects when given
  nothing. It changes nothing.
- `-d`: print the device's assert timestamps as they arrive.
- `-g`: poll the GPIO and print the edges it catches, with the daemon's
  poller and the same options as `[sample.pps.gpio]`.
- `-d` with `-g`, under an explicit `--bias` option: estimate the kernel
  timestamp bias. See the section below.

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
  `[sample.serial.pps]` keys: `--cpu`, `--priority`, `--max-bracket`.
  Short letters can be added if typing them proves tiresome.
- Bias estimation is named by `--bias`, the way `sdp` names its modes,
  rather than implied by giving both selectors: it runs for minutes and
  alternates polling in a way plain edge printing does not, so the user
  asks for it by name.
- `--echo` enables the device's echo-on-assert output while the command
  runs. Applies to `-d`.
- `-t|--timeout` bounds how long the command runs, in seconds. The
  default is 10, as for `serial -p` and ppsbias; 0 runs until
  interrupted.
- Exit status 2 when no timestamp or edge arrived, as `serial -p` and
  `sdp -i`.
- `-j|--jsonl` selects JSON lines output. The edge object follows
  `serial -p`: a `device` string (for `-d`) or `gpio` number (for `-g`),
  an RFC 3339 UTC timestamp `t` with nanoseconds, and for polled edges
  `uncertainty` in seconds and `settling`. The text form is the time of
  day with nanoseconds.
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
5. Bias mode on `gpiomem` and `pps-tool`; it does not need the poller.

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

The estimator therefore polls only alternate seconds. Two quantities are
formed:

- **A**: mean of kernel timestamp minus polled edge over polled seconds.
  This is the lateness while polling.
- **B**: for each unpolled second, the kernel timestamp's phase against a
  one-second grid minus the mean phase of its two polled neighbours,
  averaged. Taking the neighbours' mean cancels linear clock drift. B is
  how much later the kernel stamps when nothing is polling.

The reported lateness is A + B, the value for normal operation with no
poller present. Each of A and B carries a standard error and the result
reports their combination. The method assumes only that the pulse source
is periodic to well under a microsecond over two seconds and that the
system clock's frequency is stable over the same interval.

### Polling schedule

Bias mode uses its own loop, as ppsbias does, not the daemon's adaptive
poller. What the poller adds -- finding the pulse with no reference,
shrinking the window to the minimum, recovering from lost pulses -- is
for a daemon running indefinitely; the bias estimate has the kernel
timestamps to predict from, wants every other second skipped, which the
poller's tracking would count as misses, and runs for minutes, so a
fixed window's CPU cost is irrelevant. Both loops place the edge at the
bracket midpoint, so the estimate is the same; the validation below was
done with the simple loop. What the two modes share is the platform
layer: the register read, the pinned real-time thread, the absolute
sleep, and the bracket width rejection, whose rule and default should
be the same under `-g` and `--bias`.

- Predict the next edge as the last kernel timestamp plus one period.
- Sleep with an absolute clock_nanosleep to a fixed time before the
  prediction, with timer slack reduced to a microsecond, then poll back to
  back until the edge is seen or the window closes. A window of one
  millisecond each side costs about 0.2 percent of one core.
- Run the loop on a locked OS thread pinned away from the CPU that takes
  the PPS interrupt, with no allocation inside the window.
- A pulse whose bracket is several times the median was interrupted by
  preemption and is discarded. A second in which the pin is already high
  when the window opens, or no edge is seen, counts as a miss. Missing or
  duplicated kernel events are counted from the PPS sequence numbers.

### Validation

Prototyped in C on 2026-09-05 and run on a Raspberry Pi 5 (kernel
6.18.39, ondemand governor):

- L1 enabled: 11.9, 12.0, 12.2 and 12.6 us over runs of 60 to 1200 s.
- L1 disabled: 7.1 and 7.3 us.
- A + B was unchanged when the polling path changed from the GPIO
  character device ioctl to a bare register read, and when reads were
  spaced 4 us apart instead of back to back, while A and B individually
  moved in opposite directions. The correction does what it claims.
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
prototyped. The tool offers the gpiomem register read, which the `-g`
mode uses anyway, with the GPIO given by `-g`; the device-tree lookup
below is a possible later convenience, not a requirement.

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

- Bias options beyond `--bias` and `--echo`, such as ppsbias's `-e` for
  polling every pulse.
- Whether the character-device path is ever offered, and if so whether
  the tool unbinds the pps-gpio driver for the user. Unbinding destroys
  the device node, so any process holding it open, such as an NTP
  daemon, keeps a dead handle until restarted.
- What the bias output reports and how its uncertainty is expressed.
- Whether other operations on the device belong here.
- Man page text.

## Reference

- `plan/archive/sdp-tool.md` and `plan/archive/serial-cli.md` for the
  family's conventions.
- `gps/lib/kpps` for reading the PPS device.
- `plan/gpio-poll.md` for the daemon's GPIO source and `gps/lib/gpiomem`.
