# macOS find-serial tool (#322)

This plan defines `find-serial`, a small macOS serial-device discovery tool. It
discovers serial devices, optionally filters them, and optionally uses the
selected device path in an `execvp` call.

The tool is deliberately independent of satpulse. It does not read or write
`satpulse.toml`, does not know about `satpulsed` flags, and does not contain
Homebrew prefix paths. `plan/brew.md` depends on this tool and describes how the
Homebrew service uses it.

## CLI behavior

Without exec mode, the tool lists matching devices:

```sh
find-serial [options]
```

It prints one machine-readable line per discovered device, using `KEY=value`
fields:

```text
DEVICE=/dev/cu.usbmodem312301 VID=1546 PID=01A9
```

Listing mode returns success even when there are no matches; the output is just
empty. This makes the command useful for install-time inspection and debugging.

Initial options:

- `--vid HEX` restricts USB matches to a vendor ID.
- `--pid HEX` restricts USB matches to a product ID.
- `--exec` enables exec mode.
- `--help` prints usage.

Matching is USB serial callout devices only. Non-USB serial services (such as
Bluetooth) are not selected. `--vid` and `--pid` filter the USB matches.

Because every matched device is USB, the output carries no device-type field. A
`TYPE=` field is reserved for the future `--bluetooth` extension (see Out of
scope), which would need to distinguish USB from Bluetooth candidates.

Options use normal POSIX-style parsing. `--exec` is just another option. When
exec mode is used, `--` is recommended to make the boundary between discovery
tool options and child arguments explicit:

```sh
find-serial --vid 1546 --pid 01A9 --exec -- satpulsed -f satpulse.toml -d {}
```

In exec mode, the remaining positional arguments are the `execvp` argv. Exactly
one argument must be `{}`. The tool resolves the matching serial device,
replaces `{}` with the device path, and calls `execvp`.

Exec mode requires exactly one matching device:

- zero matching devices is a failure with a distinct no-match exit code;
- multiple matching devices is a failure with a distinct ambiguous-match exit
  code;
- exactly one matching device replaces `{}` and execs the child command.

Proposed exit codes:

- `0`: listing succeeded, or the child process replaced the tool.
- `64`: usage error, following `EX_USAGE`.
- `69`: no matching device, following `EX_UNAVAILABLE`.
- `2`: multiple matching devices.
- `1`: IOKit or other internal failure.

On exec failure after a successful match, the tool prints the failed command and
returns `1`.

## Implementation strategy

Implement the tool as a single macOS-only C source file using
IOKit/CoreFoundation and libc.

The IOKit enumeration should mirror the tested behavior from the
`desktop-gui` branch, which uses `go.bug.st/serial/enumerator`:

- enumerate `IOSerialBSDClient` services;
- read `IOCalloutDevice` for the outgoing `/dev/cu.*` path;
- walk up the `IOService` parent chain;
- classify `IOUSBDevice` and `IOUSBHostDevice` parents as USB;
- read `idVendor` and `idProduct` for USB devices;
- skip serial services with no USB parent (such as Bluetooth).

Reference material:

- `git show origin/desktop-gui:desktop/serialenum/serialenum.go`
- `git show origin/desktop-gui:desktop/cmd/listports/listports.go`
- `git show origin/desktop-gui:desktop/plan/archive/port-enumeration.md`
- `~/go/pkg/mod/go.bug.st/serial@v1.6.4/enumerator/usb_darwin.go`

Use `getopt_long` from macOS libc for option parsing. Do not introduce any
external dependency. Parse VID/PID as hexadecimal values and print them
normalized as four uppercase hex digits.

Use `execvp` for exec mode. The implementation should build a child argv array
from the remaining arguments, replacing the single `{}` placeholder with the
selected device path. It must validate that exec mode has a command and exactly
one placeholder before doing device selection.

Build the C file with a `make`-based `macos/Makefile`, Darwin-only, linking
with:

```sh
-framework IOKit -framework CoreFoundation
```

The tool is independent of the main Go build; `unix-build.sh` does not build it.
`plan/brew.md` runs this Makefile from the formula to produce the binary.

No Linux build, Go module dependency, or satpulse behavior change is part of
this plan.

## Out of scope

- Reading, writing, or extending `satpulse.toml`.
- Adding device discovery to `satpulsed` or `satpulsetool`.
- Adding cgo or IOKit dependencies to the main Go modules.
- Homebrew service wiring; that belongs in `plan/brew.md`.
- Linux device discovery.
- Baud-rate discovery (#326).
- Homebrew tap release automation, bottles, or broader packaging polish.
- Bluetooth serial device discovery. A future `--bluetooth` option would include
  Bluetooth serial services as candidates and add a `TYPE=usb`/`TYPE=bluetooth`
  field to each output line to distinguish them. Until then the output is
  USB-only and carries no `TYPE=` field.
