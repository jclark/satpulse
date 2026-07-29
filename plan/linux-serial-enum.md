# Replace go.bug.st serial port enumeration on Linux (#394)

The problems motivating this are listed in the issue. In short: on Linux,
go.bug.st's port listing reconfigures in-use legacy UARTs (raw mode, 9600)
as a side effect of its phantom-port check, omits USB product strings,
recognizes ports by a name whitelist, and knows nothing about udev
aliases. This plan replaces the Linux implementation of
`gps/lib/serialenum` with a sysfs-based enumerator that never opens a
device. Other platforms keep go.bug.st.

Related: #326 (baud detection) and the workbench port-status work will
consume the richer output. #117 (locking) is separate; this enumerator
does no busy detection.

## Current state

`serialenum.List()` wraps `enumerator.GetDetailedPortsList()` and returns
`[]Port{Device, Display}`. The only consumer is `satpulsewb`'s
`GET /api/ports` handler. `display()` uses the USB product string (never
populated on Linux) and a u-blox VID/PID generation tag.

## Algorithm

All steps are file reads and readlinks; no device node is ever opened.

1. Read the entries of `/sys/class/tty`. Each entry is a symlink; resolve
   it to its real directory under `/sys/devices` and use that path for
   everything that follows. The kernel name (`ttyACM0`) is the last path
   component; the device node is `/dev/<name>`.

2. Skip entries whose resolved path is under `/sys/devices/virtual/`:
   these are virtual terminals and other non-hardware ttys. This replaces
   go.bug.st's name whitelist; any driver's serial port is included
   without knowing its name.

3. If the port's directory contains a `type` file and it reads 0
   (`PORT_UNKNOWN`), skip the port: it is a legacy UART node with no
   hardware behind it. Only serial-core drivers create this file; its
   absence means no check is needed. This replaces go.bug.st's
   open-and-configure probe, which is the destructive part.

4. Walk up the parent directories of the resolved path. At each level
   read the `subsystem` symlink and the `uevent` file. At the first level
   where the subsystem is `usb` and `DEVTYPE=usb_device`, read
   `idVendor`, `idProduct`, `serial`, `manufacturer`, and `product`, and
   stop. Ports with no such ancestor (platform UARTs) simply have no USB
   fields. This subsystem-matching walk is the form required by the
   kernel's sysfs rules (admin-guide/sysfs-rules.html): never depend on a
   fixed number of parent levels, and never use the `device` symlink as a
   path element. It also handles cdc-acm and usb-serial topologies with
   one code path where go.bug.st hardcodes two.

   Also record the driver name: the `driver` symlink target of the
   nearest ancestor that has one (e.g. `cdc_acm`, `port`).

5. Read the top-level entries of `/dev` (one readdir, no recursion,
   `d_type` to spot symlinks without stat'ing). A symlink whose target
   is directly in `/dev` (a relative target is interpreted relative to
   `/dev`) and matches an enumerated node is a user-defined alias,
   e.g. `/dev/gps0 -> ttyACM0`. Nothing else qualifies: udev's
   generated link directories (`/dev/serial/by-id`, `/dev/serial/by-path`,
   `/dev/char`) are subdirectories, and links like `/dev/stdin` target
   `/proc`. If several aliases point at one port, the first in sort
   order is primary and the rest are kept.

## Port struct

```go
type Port struct {
    Device  string   `json:"device"`  // path to open: user alias if present, else /dev/<kernel name>
    Display string   `json:"display"` // human-readable label
    Name    string   `json:"name"`    // kernel name, e.g. "ttyACM0"
    Aliases []string `json:"aliases,omitempty"` // user-defined udev symlinks
    Driver  string   `json:"driver,omitempty"`
    VID     string   `json:"vid,omitempty"`     // USB only, lowercase hex
    PID     string   `json:"pid,omitempty"`
    Serial  string   `json:"serial,omitempty"`  // USB serial number
    Product string   `json:"product,omitempty"` // USB product string
}
```

Decisions already settled:

- When a user-defined alias exists, it is the port's primary path:
  `Device` is the alias, and the workbench shows it first with the
  kernel name in parentheses. The user added the alias for a reason,
  and it is what belongs in a config file. `Display` for such a port
  looks like `gps0 - u-blox GNSS receiver (ttyACM0)`.
- Otherwise `Device` is `/dev/<kernel name>` and `Display` is built
  from the USB product string plus the existing u-blox generation tag,
  falling back to the bare path as today.
- The replug-stable identity that later work (remembered baud rate,
  #326) keys on is VID+PID+Serial; the udev by-id links encode the
  same information, so they are not collected. This plan only
  supplies the fields.

## Code layout

`gps/lib/serialenum` grows build-tagged files: the new Linux
implementation, and the existing go.bug.st-based code moved to a
non-Linux file. The exported API (`List() ([]Port, error)`) is
unchanged; the new `Port` fields are simply empty on other platforms.
The `display()`/`ubloxTag` logic is shared. go.bug.st remains a
dependency for macOS and Windows enumeration only.

## Testing

The enumerator takes its filesystem roots (`/sys/class/tty`, `/dev`) as
package-level variables so tests can point it at a constructed tree.
Tests build the fake tree in `t.TempDir()` at runtime (symlinks
included), rather than committing symlink trees to git: cover a
platform UART, a cdc-acm device, a usb-serial device, a phantom ttyS
(type 0), a virtual tty, a user alias, non-qualifying symlinks (target
outside `/dev`, links in subdirectories), and an alias-less port. A Linux-only test can additionally run `List()`
against the real system and assert it returns without error.

Manual validation on abondance: expect ttyS0, ttyS1 (real 16550A),
ttyACM0-2 with Septentrio/u-blox strings, `/dev/gps0` as the primary
path for ttyACM0, and no ttyS2/ttyS3. Verify with strace that the run
performs no `openat` on any `/dev/tty*` node.

## Non-goals

- No busy/lock detection and no port probing of any kind (#117, #326).
- No hotplug watching (udev netlink/inotify); the workbench re-lists on
  demand today and live updates are later workbench work.
- No change to enumeration on macOS or Windows.
