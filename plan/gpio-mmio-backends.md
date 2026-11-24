# GPIO polling on SoCs without gpiomem (#571)

GPIO polling (#460) reads a GPIO's level from its controller's registers.
On the Raspberry Pi these are mapped through the downstream gpiomem
devices (`/dev/gpiomem`, `/dev/gpiomem0`), which need only file
permission. Other SoCs have no such device, so their registers must be
mapped through `/dev/mem`. This plan covers what that needs: the
privilege handling, finding the register range, the backend interface
change, and what each SoC family contributes.

## Families

Every controller below reads a line's level as one bit of one 32-bit
register, at an offset from the controller's base computed from the
gpiochip line number alone, as the kernel's own `get` does. Package
names follow the kernel's naming.

| Package | Controller (compatible) | Level register for line n | Boards |
|---|---|---|---|
| `bcm2835` | `brcm,bcm2835-gpio`, `brcm,bcm2711-gpio` | 0x34 + 4*(n/32) | Pi 3, Pi 4 (gpiomem, done) |
| `rp1` | `raspberrypi,rp1-gpio` | second reg range + 0x4000*bank + 8 | Pi 5 (gpiomem, done) |
| `rockchip` | `rockchip,gpio-bank` | `ext_port`: 0x70 on v2, 0x50 on v1, chosen by the version register at 0x78; one controller per 32-line bank | ODROID-M1/M2, Radxa ZERO 3E |
| `k3` | `ti,keystone-gpio` | 0x20 + 0x28*(n/32) | BeagleY-AI |
| `sunxi` | `allwinner,sun55i-a523-pinctrl` and the A733's; Radxa's vendor kernel uses `sun55iw3`/`sun60iw2` strings with the same layouts | port base + 0x10. A523/A527: ports 0x30 apart, port K at 0x500. A733: 0x80 + 0x80*bank | Radxa Cubie A5E (A527), Cubie A7A (A733) |
| `mxc` | `fsl,imx35-gpio` (the i.MX8M Plus's `fsl,imx8mp-gpio` lists it as fallback) | PSR at 0x08; five controllers of 32 lines | NXP FRDM i.MX8M Plus |
| `starfive` | `starfive,jh7110-sys-pinctrl`, `-aon-pinctrl` | sys: 0x118 + 4*(n/32), 64 lines; aon: 0x2c, 4 lines | VisionFive 2 (riscv64) |

Each family also parses the GPIO names its boards' header documentation
uses, case-insensitively:

| Family | Name | Resolves to |
|---|---|---|
| Rockchip | `GPIO3_D0` | controller at device-tree alias `gpio3`, line 8*X + n (D0 = 24) |
| Allwinner | `PD4` | main controller, line 32*(X-A) + n; ports L and up on the R controller, line 32*(X-L) + n |
| TI K3 | `GPIO1_11`, `MCU_GPIO0_5` | controller labelled `main_gpio1` or `mcu_gpio0` in `__symbols__`, line n |
| JH7110 | `GPIO44` | sys controller, line 44 |
| i.MX | `GPIO1_IO05` | controller labelled `gpio1` in `__symbols__`, line 5; the aliases are numbered from 0 (`gpio0` is GPIO1), so they do not match the names. TODO: check that the FRDM header documentation uses this form |

TI and i.MX depend on `__symbols__`, which exists only when the device
tree is built with overlay support; the BeagleBoard image and the
FRDM's kernel have it.

Only controllers whose registers are memory-mapped can be polled.
Expanders behind a slow bus, such as the two PCAL6416 I2C expanders on
the FRDM board, cannot, and appear in the listing without a backend.

Each family is added only after a measurement run on a board, as the
Pi 3, 4 and 5 were.

## Mapping and privilege

Opening `/dev/mem` needs `CAP_SYS_RAWIO`, which no long-running process
should keep. The kernel checks it only at open (`open_port` in
`drivers/char/mem.c`); `mmap` checks only that the range is allowed.
So:

1. At startup, open `/dev/mem` read-only and map read-only only the
   page-aligned range holding the controller's level register, not the
   whole controller or more.
2. Close the descriptor.
3. Once the caller has opened everything it needs, it calls
   `DropPrivilege(PrivRawMemoryRead)`, which removes `CAP_SYS_RAWIO`
   from the effective, permitted and inheritable sets of every thread.
   Capabilities are per thread, so this needs
   `syscall.AllThreadsSyscall`, which works only in a binary without
   cgo; the release builds use netgo and osusergo and have none. The
   removal cannot be undone, since `capset` cannot raise the permitted
   set. The bounding set is left alone: dropping from it needs
   `CAP_SETPCAP`, which `satpulse@.service` does not grant, and it
   would only limit what an `exec` can gain, which satpulsed never
   does and `NoNewPrivileges=yes` already prevents.

After that the process has read access to one register page and nothing
else. A read-only open cannot be upgraded: `mmap` refuses `PROT_WRITE`
on a shared mapping of a file opened without write access and clears
`VM_MAYWRITE`, so `mprotect` cannot add it later.

Passing an open `/dev/mem` descriptor from a helper does not help: the
descriptor can map any allowed range, which is as powerful as the
capability.

This needs `Open` to stay separate from polling, as it is now: satpulsed
opens the GPIO at startup and polls later, so the drop happens before
any receiver or network input is processed. Each later poll reuses the
same mapping.

`satpulse@.service` does not grant `CAP_SYS_RAWIO`, and must not. A
drop-in shipped inactive (e.g. `/usr/share/satpulse/`) containing

```ini
[Service]
CapabilityBoundingSet=CAP_SYS_RAWIO
```

can be copied by those who want it into
`/etc/systemd/system/satpulse@.service.d/`; systemd merges
`CapabilityBoundingSet=` lines by OR. Without it, opening `/dev/mem`
fails with `EPERM`. `Open` wraps that error so that
`errors.Is(err, os.ErrPermission)` holds, and satpulsed, which knows
from the GPIO's `Privilege` why permission was needed, names the
drop-in in its error. Neither gpiomem nor a backend mentions the
drop-in. The Pi needs none of this.

Mapping a range a driver has claimed is refused on kernels built with
`IO_STRICT_DEVMEM` unless booted with `iomem=relaxed`. The kernels of
Raspberry Pi OS and the BeagleBoard 7.4 image have `STRICT_DEVMEM` off;
Armbian's Rockchip kernel and the 7.3 kernel on the FRDM i.MX8M Plus
have `STRICT_DEVMEM` on and `IO_STRICT_DEVMEM` off. Each board's
kernel needs checking, and the error for a refused mapping should say
what to change.

## Finding the register range

The base address is the controller node's `reg`, translated to a CPU
physical address through each parent's `ranges`, as the Devicetree
Specification defines. Two shortcuts do not qualify:

- `/proc/iomem` has the translated ranges but shows zero addresses
  without `CAP_SYS_ADMIN`.
- The platform device name (e.g. `3f200000.gpio`) carries the
  translated address of the first range, but its format is the
  kernel's implementation, not ABI.

The translation matters: on a Pi 3 the untranslated `reg` is
0x7e200000, the CPU address 0x3f200000.

## gpiomem interface

`Open`, `Pin.High`, `Pin.Close` and `Lookup` keep their signatures.
What changes is what gpiomem says about how a GPIO's registers are
reached, and what that needs:

```go
// Privilege is what opening a GPIO needs.
type Privilege int

const (
	// PrivNone means the GPIO cannot be opened, as for a controller with no
	// backend.
	PrivNone Privilege = iota
	// PrivGPIOMemRead is permission to read a device that maps only the
	// GPIO controller's registers, such as /dev/gpiomem0. On Raspberry Pi
	// OS the gpio group has it.
	PrivGPIOMemRead
	// PrivRawMemoryRead is permission to read physical memory. On Linux
	// this is CAP_SYS_RAWIO, which allows more than that.
	PrivRawMemoryRead
)

// DropPrivilege gives up p where the system allows it.
func DropPrivilege(p Privilege) error
```

- `GPIO` (from `Lookup`) and `Controller` (from `List`) gain a
  `Privilege` field. The values name what is needed, not what the
  system grants, so that further levels can be added.
- `DropPrivilege(PrivRawMemoryRead)` removes `CAP_SYS_RAWIO` as above.
  `DropPrivilege(PrivGPIOMemRead)` does nothing and returns nil: read
  permission on a GPIO device cannot be given up in-process, and grants
  nothing beyond the GPIO registers.
- The caller decides when to drop: after it has opened everything it
  needs, not when gpiomem has opened the GPIO.
- A device is only a device that specifically maps the controller's
  registers, as the gpiomem devices do. A controller read through
  `/dev/mem` has no device: `Controller` gains the physical address of
  the mapped range, shown with its length, and `Device` is empty.
  `Pin.Device`, which satpulsed logs, needs the same treatment.

## Backend interface

`backend.Backend` now has `GPIOMem() (device, length)`. It becomes a
choice between a gpiomem device and a `/dev/mem` mapping of a register
range of the controller node, with:

- which `reg` range holds the level register (RP1 uses the second);
- an optional check of the mapped registers before use, such as
  Rockchip's version register, which also selects the layout;
- whether the level reads true while pps-gpio holds the line for its
  interrupt (below).

Backends stay pure: gpiomem reads the device tree, translates `reg`,
maps, and drops the privilege when asked; a backend only computes from
what it is given. A family is one new package plus one line in gpiomem's list.

## Reading while pps-gpio holds the line

The GPIO polled is one a `pps-gpio` node uses, so the kernel holds the
line for its interrupt. Whether the level register then reads the
pin's state is per controller:

- Pi: yes, measured.
- A523/A527: yes (kernel commit eaf84ff67340).
- A733: the mainline driver still sets `irq_read_needs_mux`, so not
  without re-muxing; unsupported until measured otherwise.
- Others: to measure on each board.

The held line also keeps the controller powered where the driver uses
runtime PM: `gpio-mxc`, for example, takes a runtime-PM reference when a
line is requested. Each family's driver needs the same check, since a
suspended controller would read wrong or fault.

## Architectures

`word` uses `atomic.LoadUint32`, an LDAR on arm64. On riscv64 Go
compiles it to LR.W, which the U74 manual says raises a precise access
exception on uncacheable regions, so the VisionFive 2 needs its own
read: a plain 32-bit load in a small assembly function, in its own file
as `word` is. The command and the daemon's `[gpio]` support are
currently registered on linux/arm64 only, and would gain linux/riscv64.

## Order

Rockchip first, since it is the simplest `/dev/mem` family and has
several boards, and it brings in all the shared work: `reg`
translation, the `/dev/mem` mapping, the capability drop and the
drop-in. Then one PR per family, each with its measurement run.
