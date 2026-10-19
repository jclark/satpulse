# PHCs with channels but no programmable pins (#504)

## Overview

Some PHC drivers, such as TI am65-cpts and NXP enetc_ptp, have external
timestamp (EXTTS) and periodic output (PEROUT) channels but no
programmable pins: the kernel reports `n_pins == 0` while `n_ext_ts` or
`n_per_out` is non-zero. On these SoCs the routing of signals to channels
is fixed by the board's pinmux, typically in the device tree, so there is
no `PTP_PIN_SETFUNC` step; the channel is used directly. The kernel treats
the three counts independently: an EXTTS or PEROUT request checks only the
channel index, and a pin ioctl fails with `EINVAL` when there are no pins.

SatPulse currently requires pins: both satpulsed and `satpulsetool sdp`
refuse a PHC whose pin count is zero, and both always assign a pin before
enabling a channel. This plan makes them work on pinless PHCs with the
smallest change to the user model.

## User model

The rule is the same everywhere: on a PHC without programmable pins, the
"assign a pin to a channel" step does not exist. Anything that names or
assigns a pin is an error there; anything that acts on a channel is
unchanged; the forms with defaults adapt to what the hardware has.

On a PHC with pins, nothing changes.

## satpulsed

In the `[phc]` table:

- `channel` is unchanged: selects the EXTTS channel, default 0.
- `pin` is unchanged on a PHC with pins: default 0.
- On a PHC with no pins, `pin` must not be given. Giving it is a
  configuration error, "interface X has no programmable pins". With `pin`
  omitted, the daemon enables EXTTS on `channel` directly.

So a configuration naming only the interface works on either kind of PHC.

The log line "enabled external timestamping on the PTP hardware clock"
omits the `pin` attribute when there is no pin.

## satpulsetool sdp

### Pin and channel selection

- `--chan` is unchanged: default 0.
- `--pin` is unchanged on a PHC with pins: default 0. On a PHC with no
  pins, giving `--pin` is an error, "interface X has no programmable pins".

### Modes

- `-i`/`--extts`: on a PHC with pins, assigns the pin to the EXTTS channel,
  enables it and reads timestamps, as now. On a pinless PHC, enables the
  channel and reads timestamps. Output is unchanged; the `ExttsEvent` JSON
  has no pin field.
- `-o`/`--perout`: likewise; on a pinless PHC it enables the channel.
  `--period 0` still disables the output.
- `--disable`: unassigns a pin, as now. On a pinless PHC it is an error,
  "interface X has no programmable pins": there is no assignment to undo.
- `--show` is removed and replaced by two explicit show modes,
  `--show-ifaces` and `--show-pins`. The three show forms count as modes
  in the one-mode-only check, like `-i`, `-o` and `--disable`.

### Show modes

| Command | Equivalent |
|---|---|
| `sdp` | `sdp --show-ifaces` |
| `sdp iface` | `sdp --show-pins iface` if the PHC has pins, else `sdp --show-ifaces iface` |
| `sdp iface --pin p` | `sdp --show-pins iface --pin p` |
| `sdp --show-ifaces` | interface records for every PHC with pins or channels |
| `sdp --show-ifaces iface` | the interface record for `iface` (any interface with a PHC) |
| `sdp --show-ifaces --pin p` | error: `--pin` not allowed with `--show-ifaces` |
| `sdp --show-pins` | error: interface name required |
| `sdp --show-pins iface` | the pin entries for `iface`; error if the PHC has no pins |
| `sdp --show-pins iface --pin p` | the entry for pin `p` (index or name); error if not found or the PHC has no pins |

The explicit flags each have a fixed output type, so `-j` consumers can
rely on them; the bare forms are conveniences that pick one.

The interface record is as today (interface, status, driver, PCI details,
PTP clock device, pins, channel counts) except that the `Pins:` line is
omitted when the PHC has no pins. In JSON, `pins` is then an empty array,
not null. The listing with no interface includes PHCs that have channels
but no pins; it still skips PHCs with neither. Its no-data error becomes
"no interfaces with PTP hardware clock pins or channels found".

The pin entry (name, index, function, channel) is unchanged.

## Documentation

- `docs/man/satpulse.toml.5.md`, `[phc]` table: `pin` is omitted on PHCs
  whose pin routing is fixed in hardware; `channel` selects the channel.
- `docs/man/satpulsetool-sdp.1.md`: a paragraph on PHCs without
  programmable pins; `--show` replaced by `--show-ifaces` and
  `--show-pins`; the show-mode table above in prose; examples updated.
- `docs/_includes/NEWS.md`: one entry.

## Implementation notes

What becomes optional is the pin, nothing else.

- `time/app/daemon` `PHCConfig.Pin` becomes `*uint8` so an unset pin can
  be told from pin 0.
- `time/internal/ts` `PinDesc.PinIndex` becomes optional (pointer).
  `validatePinDesc` requires `n_ext_ts > 0` and a valid channel; rejects a
  pin when `PinCount() == 0`; otherwise defaults the pin to 0 and
  range-checks it as now. `StartWorker` calls `PinSetFunc` only when there
  is a pin.
- `internal/sdpcmd`: `sdpflags.go` no longer defaults `--pin` to "0", so
  an empty value means not given; adds `--show-ifaces` and `--show-pins`
  and drops `--show`; `--pin` is rejected with `--show-ifaces`.
  `phcOpen` returns an optional pin: error if `--pin` is given on a
  pinless PHC, default 0 when there are pins. `extts.go` and `perout.go`
  skip `PinSetFunc` when there is no pin. `disable.go` errors on a pinless
  PHC. `showifaces.go` keeps PHCs with channels but no pins and makes
  `Pins` an empty slice; `Print` omits the `Pins:` line when empty.
  `showpins.go` falls through to the single-interface record in the bare
  form; the explicit `--show-pins` errors. `sdpflags_test.go` and
  `showifaces_test.go` updated accordingly.
- `time/phc` is unchanged: `PinCount`, `ExttsChanCount` and
  `PeroutChanCount` already expose the three counts independently.
