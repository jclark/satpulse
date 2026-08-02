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

## Pre-change state

`serialenum.List()` wraps `enumerator.GetDetailedPortsList()` and returns
`[]Port{Device, Display}`. The only consumer is `satpulsewb`'s
`GET /api/ports` handler. `display()` uses the USB product string (never
populated on Linux) and a u-blox VID/PID generation tag.

## Algorithm

All steps are file reads and readlinks; no device node is ever opened.

1. Read the entries of `/sys/class/tty`. Each entry is a symlink; resolve
   it to its real directory under `/sys/devices` and use that path for
   everything that follows. The kernel name (`ttyACM0`) is the last path
   component; the device node is `/dev/<name>`. If a per-port operation
   returns `ENOENT` or `ENODEV`, the tty disappeared during enumeration;
   skip it and continue. Other errors still fail the enumeration.

2. Skip entries whose resolved path is under `/sys/devices/virtual/`:
   these are normally virtual terminals and other non-hardware ttys.
   RFCOMM devices (`rfcomm` followed by a decimal number) are the
   exception: although represented as virtual TTYs, they are usable
   Bluetooth serial ports and were supported by the old enumerator.
   Apart from this explicit exception, the sysfs topology replaces
   go.bug.st's hardware-port name whitelist.

3. If the port's directory contains a `type` file and it reads 0
   (`PORT_UNKNOWN`), skip the port: it is a legacy UART node with no
   hardware behind it. Only serial-core drivers create this file; its
   absence means no check is needed. This replaces go.bug.st's
   open-and-configure probe, which is the destructive part.

4. Walk up the parent directories of the resolved path. At each level
   read the `subsystem` symlink; when it names the USB subsystem, read
   `uevent`. At the first level with `DEVTYPE=usb_device`, read
   `idVendor`, `idProduct`, and `product`, and stop. `product` is the
   USB device descriptor, not the platform-dependent friendly-port
   `Product` returned by go.bug.st. Ports with no such ancestor
   (platform UARTs) simply have no USB fields. This
   subsystem-matching walk is the form required by the
   kernel's sysfs rules (admin-guide/sysfs-rules.html): never depend on a
   fixed number of parent levels, and never use the `device` symlink as a
   path element. It also handles cdc-acm and usb-serial topologies with
   one code path where go.bug.st hardcodes two.

5. Read the top-level entries of `/dev` (one readdir, no recursion,
   `d_type` to spot symlinks without stat'ing). Discard any sysfs
   candidate that has no corresponding `/dev/<kernel name>` entry. This
   prevents a process with a restricted `/dev`, such as a container,
   from being offered host devices visible through `/sys` but not
   available in its device namespace. A symlink whose target is directly
   in `/dev` (a relative target is interpreted relative to `/dev`) and
   matches a surviving node is a user-defined alias, e.g.
   `/dev/gps0 -> ttyACM0`. Nothing else qualifies: udev's generated link
   directories (`/dev/serial/by-id`, `/dev/serial/by-path`, `/dev/char`)
   are subdirectories, and links like `/dev/stdin` target `/proc`. Keep
   every qualifying alias in sort order for construction of `Display`;
   aliases remain an implementation detail rather than separate result
   fields.

## Port struct

```go
type Port struct {
    Device  string `json:"device"`       // canonical device node to open
    Display string `json:"display"`      // human-readable label
    USB     USBID  `json:"usb,omitzero"` // USB vendor and product IDs
}

type USBID struct {
    VID uint16 `json:"vid"`
    PID uint16 `json:"pid"`
}
```

- `Device` is always the canonical `/dev/<kernel name>` path and is
  unaffected by aliases. This gives callers and the workbench device
  input a uniform value.
- `Display` always starts with `Device`. Every qualifying alias follows
  inside the parentheses, in sort order. Thus Raspberry Pi's standard
  alias is shown as `/dev/ttyAMA0 (/dev/serial0)`, without changing the
  path used to open the port.
- A specific u-blox generation tag takes precedence over the generic
  u-blox USB product string; otherwise the USB product string is
  appended after any aliases inside the same parentheses. Examples are
  `/dev/ttyACM0 (/dev/gps0, u-blox gen 9)` and
  `/dev/ttyACM1 (Septentrio USB Device)`. With no alias or useful
  device detail, `Display` is simply `Device`.
- VID and PID are numeric because they are USB IDs, not display
  strings; JSON therefore represents them as numbers too. They are
  grouped in `USB`, which is omitted when both are zero; once present,
  both members are serialized, so a real PID of zero is retained. They
  are useful for matching a receiver model, as in `macos/find-serial`.
- Alias lists, serial number, manufacturer, driver name, and USB
  product string are not exposed as separate fields. Aliases and the
  product are used internally only to construct `Display`.

## Code layout

`gps/lib/serialenum` grows build-tagged files: the new Linux
implementation, and the existing go.bug.st-based code moved to a
non-Linux file. `List() ([]Port, error)` keeps its signature, while
`Port` gains a composite `USB` field containing numeric VID and PID.
The non-Linux implementation populates it from go.bug.st, so the fields
have the same meaning on every platform. Incomplete or malformed
go.bug.st ID strings omit `USB` rather than failing the complete
enumeration. `ubloxTag` is shared, but display formatting is not:
go.bug.st's platform-dependent `Product` field is not semantically the
same as Linux's USB product descriptor. go.bug.st remains a dependency
for enumeration on supported non-Linux platforms.

An ignored standalone program, `listports.go`, prints `Device`, VID/PID
in hexadecimal, and `Display` for manual testing:

```
go run ./gps/lib/serialenum/listports.go
```

## Testing

The enumerator takes its filesystem roots (`/sys/class/tty`, `/dev`) as
package-level variables so tests can point it at a constructed tree.
Tests build the fake tree in `t.TempDir()` at runtime (symlinks
included), rather than committing symlink trees to git: cover a
platform UART, a cdc-acm device, a usb-serial device, a phantom ttyS
(type 0), a sysfs port with no matching `/dev` entry, an excluded
virtual tty, an included virtual RFCOMM port, multiple aliases (all
displayed in sort order), non-qualifying symlinks (target outside
`/dev`, links in subdirectories), and an alias-less port. Common tests
cover hexadecimal VID/PID parsing, numeric JSON output (including a
zero PID), and the retained go.bug.st display formatting. A Linux-only
test covers the Linux implementation entirely through the constructed
filesystem; automated tests do not depend on the host's `/sys` or
`/dev`.

Manual validation on abondance with the ignored program should produce:

```
device=/dev/ttyACM0 vid=1546 pid=01a9 display="/dev/ttyACM0 (/dev/gps0, u-blox gen 9)"
device=/dev/ttyACM1 vid=152a pid=8231 display="/dev/ttyACM1 (Septentrio USB Device)"
device=/dev/ttyACM2 vid=152a pid=8231 display="/dev/ttyACM2 (Septentrio USB Device)"
device=/dev/ttyS0 display="/dev/ttyS0"
device=/dev/ttyS1 display="/dev/ttyS1"
```

There should be no ttyS2/ttyS3. Verify with strace that enumeration
performs no `openat` on any `/dev/tty*` node.

## Non-goals

- No busy/lock detection and no port probing of any kind (#117, #326).
- No hotplug watching (udev netlink/inotify); the workbench re-lists on
  demand today and live updates are later workbench work.
- No change to the enumeration algorithm on non-Linux platforms.
