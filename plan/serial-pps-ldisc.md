# Kernel PPS for serial PPS on DCD (#411)

## Introduction

When `pps.pin = "dcd"` is configured on Linux, upgrade edge detection
from the TIOCMIWAIT wait backend to kernel PPS: attach the N_PPS line
discipline to the tty satpulsed already owns, and read edges from the
`/dev/ppsN` device it creates. No new configuration; any setup failure
falls back to the existing TIOCMIWAIT backend (which itself falls back
to polling), so the upgrade is invisible except in the log line naming
the selected backend.

The gain over TIOCMIWAIT is where the timestamp is taken. The wait
backend timestamps its own wakeup in user space, strictly after the
edge, so scheduling latency lands one-sidedly in every sample. The
line discipline timestamps the edge in the kernel when the driver
processes the modem-status change, before any process is scheduled,
and user space merely fetches the stored timestamp later. On a native
UART that is the hardware interrupt path; on a USB serial adapter the
timestamp is taken at URB completion, so the adapter's USB delivery
latency remains and only the user-space wakeup component is removed.
Kernel PPS also gives exact missed-edge accounting (sequence numbers
per edge) and prompt cancellation (the device is pollable), neither of
which TIOCMIWAIT can offer.

This stays functionality no NTP daemon can provide for itself: only
the process holding the tty can attach the line discipline, and
satpulsed holds it for the GPS messages. gpsd attaches N_PPS to its
own data fd in exactly this way.

DCD only: the line discipline's sole hook is `dcd_change`, so
`pps.pin` values `cts`, `dsr`, and `ri` keep the existing backend.

## Kernel facts

Verified against Linux master (2026-08-14), `drivers/pps/*`,
`drivers/tty/tty_io.c`, `drivers/tty/tty_ldisc.c`,
`drivers/usb/serial/ftdi_sio.c`:

- `pps-ldisc.c` builds its ops by copying the N_TTY ops
  (`n_tty_inherit_ops`) and overrides only `open`, `close`, and
  `dcd_change`; `open`/`close` chain to the saved N_TTY methods. Data
  IO on the tty is therefore unaffected while N_PPS is attached.
- `pps_tty_dcd_change` takes the timestamp (`pps_get_ts`) before
  reporting the event. Assert events (flag bit set) and clear events
  each carry their own timestamp and an independent sequence counter.
- USB serial drivers report DCD to the line discipline:
  `ftdi_sio` calls `usb_serial_handle_dcd_change`, which invokes
  `ld->ops->dcd_change`. So kernel PPS works on FTDI adapters, with
  the latency caveat above.
- `ioctl(TIOCSETD)` has no capability check; it needs only the open
  fd. If the `pps_ldisc` module is not loaded, autoload during the
  ioctl is gated by the `dev.tty.ldisc_autoload` sysctl (default 1);
  with autoload off and no `CAP_SYS_MODULE` the ioctl fails.
- `pps_tty_open` registers the source with `path` set to the tty's
  own device path (`/dev/<driver><index>`). The sysfs attributes are
  class `dev_groups`, created before the udev add event fires, and
  the devtmpfs node exists when the ioctl returns.
- Identification is by content, not timing: each
  `/sys/class/pps/ppsN/path` holds the tty path for ldisc-backed
  sources and is empty for others (e.g. PHC-backed `ptp0`), so
  concurrent attachers on different ttys cannot confuse each other.
- `PPS_FETCH` with a zero timeout returns the stored latest events
  immediately; a nonzero timeout waits (ETIMEDOUT), the
  `PPS_TIME_INVALID` flag waits indefinitely, and the wait is
  interruptible (EINTR). The fd is pollable: POLLIN when an event
  newer than the last fetch exists.
- The ioctl constants and structs (`PPS_FETCH`, `PPS_GETCAP`,
  `PPSFData`, ...) are already in `golang.org/x/sys/unix`, per-arch
  (the uapi macros embed a pointer size, so the values differ between
  32- and 64-bit; the generated constants are correct).
- `/dev/ppsN` is devtmpfs default root:root 0600; no distro or gpsd
  udev rule relaxes it. Fetch and getcap need no capability once the
  fd is open; only `PPS_SETPARAMS`/`PPS_KC_BIND` need `CAP_SYS_TIME`,
  and neither is needed (the source's defaults capture both edges).
- The line discipline is a property of the tty, not the fd: the
  source and `/dev/ppsN` last until the ldisc is changed or the tty
  is finally closed, and N is not stable across reopens.

## Backend design

A second `ModemControlPinWatch` implementation in `term_linux.go`.
`NewModemControlPinWatch` tries it when the pin is `ModemDCD` and
falls back to the existing `pinWatch` on any setup failure. Nothing
above `term` changes: `serialpps.Wait`, `gpsio.SerialConn`, and the
daemon wiring are untouched. `term` stays log-free, so the concrete
watch types identify themselves (a `String` method: "kernel PPS",
"TIOCMIWAIT") and `serialpps.Detect` logs which backend it got.

Setup, on the watch's private dup (the ldisc is per-tty, so which fd
issues the ioctls does not matter):

1. `TIOCGETD` to save the current ldisc; `TIOCSETD` to N_PPS (18).
2. Resolve the tty's canonical device path via
   `os.Readlink("/proc/self/fd/N")`, which is immune to the
   configured path being a symlink (`/dev/serial/by-id/...`).
3. Scan `/sys/class/pps/*/path` for that path to learn N. Entries
   that vanish mid-scan (another process's source) are skipped.
4. Open `/dev/ppsN`, with a short retry loop on EACCES (about 2 s
   total) to cover udev applying the packaged rule asynchronously.
5. `PPS_GETCAP` sanity check.

Any failure restores the saved ldisc and falls back. Close also
restores the saved ldisc.

Wait blocks in `poll` on the pps fd and an eventfd, with no timeout.
`Cancel` writes the eventfd, so cancellation is prompt and the
abandoned-goroutine caveat that `gpsio.SerialConn` documents for
TIOCMIWAIT does not apply to this watch. On POLLIN it fetches with a
zero timeout and diffs the assert and clear sequence numbers against
the last seen:

- Each advanced counter yields a `ModemControlPinChange` with the
  kernel timestamp; when both advanced, the two changes are emitted
  in timestamp order, the second buffered for the next Wait call
  without re-polling.
- `missed` is the sequence delta minus one, per edge type -- exact,
  where TIOCMIWAIT's `TIOCGICOUNT` deltas are an inferred lower
  bound. Nothing is withheld: each event carries its own timestamp,
  so the multi-transition ambiguity that makes the TIOCMIWAIT
  backend withhold a wakeup cannot arise.
- Asserted is true for an assert event, false for a clear event, per
  the uapi's definition (assert means the flag bit is set).

Timestamps: `Wall` is the kernel's CLOCK_REALTIME edge timestamp,
carrying no monotonic reading; `Mono` is an ordinary `time.Now` taken
at the wakeup. Unlike the other backends the two are readings of
slightly different instants -- the edge and its delivery -- but
`Mono` only labels the second against message read times, where the
association bounds are milliseconds and the delivery skew is far
below them. The `ModemControlPinChange` contract comment gains a
sentence saying a backend may do this.

Edge sense is unchanged at the `serialpps` layer: the leading edge is
the flag becoming deasserted (the TTL-adapter convention from
`plan/archive/serial-pps.md`), which this watch reports as the clear
event. Both senses are delivered, so nothing at this layer blocks a
future `pps.edge` key (see Open questions).

## udev rule and packaging

`/dev/ppsN` being root-only is fine for satpulsed under the packaged
unit, which runs as root; unprivileged runs (test instances, a future
Workbench use) need a udev rule. Ship one in the deb/rpm and
`make install`, scoped to tty-backed sources so PHC-backed pps
devices are not opened up:

```
SUBSYSTEM=="pps", ATTR{path}=="/dev/tty*", GROUP="dialout", MODE="0660"
```

Installed to `/usr/lib/udev/rules.d/60-satpulse-pps.rules`. The
EACCES retry in setup step 4 covers the gap between the devtmpfs node
appearing and udev applying the rule. Without the rule an
unprivileged run falls back to TIOCMIWAIT after the retry window; the
fallback log line says why.

## Data-loss blip at attach

Changing the line discipline replaces the ldisc instance, so buffered
input not yet read can be dropped at attach (and again at the restore
in Close or fallback). The packet scanner resynchronizes on framing,
so the cost is at most a lost packet or two at startup, the same
class of blip as opening the port mid-stream. Not worth engineering
around.

## Testing

The sequence bookkeeping (event diffing, ordering, the one-change
buffer, missed arithmetic) is pure logic once the fetch is behind a
small interface; table-driven tests feed it synthetic `PPSKInfo`
sequences, including both-edges-advanced and counter-jump cases. The
sysfs scan gets a test against a fake directory tree, including a
non-matching PHC-style entry with an empty path. The poll/eventfd
loop and the ioctls stay a thin shell exercised by the hardware runs;
`serialpps` and above need no new tests.

Hardware validation uses the EVK-X20P, whose DB9 wires time pulse 1
to pins 1 and 6 -- DCD and DSR -- at RS-232 levels, so a USB-RS232
adapter (or a real UART) sees the pulse on DCD directly. Validate on
a host synced to the LAN stratum-1 as for the wait backend: long-run
sequence continuity (missed = 0), per-sample offset bias and jitter
against the TIOCMIWAIT backend on the same wiring (the wakeup
component of the bias should disappear), and end to end against
chrony. The fallback path is validated by pointing the backend at a
tty whose driver lacks DCD and by an unprivileged run without the
udev rule.

## Open questions

- `pps.edge`: on true RS-232 levels the flag sense is inverted
  relative to a TTL adapter -- the electrical rising edge asserts the
  flag -- so on the EVK-X20P the leading edge is the assert event,
  and the current deassert-only rule would timestamp the trailing
  edge, off by the pulse width. `plan/archive/serial-pps.md` deferred
  a `pps.edge` key until inverted pulses mattered; the EVK is that
  case, and validation will hit it. Decide during implementation
  whether the key lands here (a `Wiring` field and a config key; the
  watch already delivers both senses) or the EVK is validated with
  its pulse polarity configured to match the TTL convention.
- Whether the restore-on-Close of the saved ldisc is worth keeping if
  it complicates shutdown ordering; leaving N_PPS attached is
  harmless (it behaves as N_TTY) and the tty teardown removes it.
