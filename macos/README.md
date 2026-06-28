# find-serial

`find-serial` is a small macOS-only tool that discovers USB serial devices via
IOKit. It is independent of satpulse: it does not read `satpulse.toml` or know
about `satpulsed` flags. The Homebrew service uses it to locate the GPS receiver
at launch time.

It matches USB serial callout devices (`/dev/cu.*`) only; non-USB serial
services such as Bluetooth are ignored. `--vid` and `--pid` (hexadecimal) narrow
the matches by USB vendor and product ID.

Build it with the `Makefile` in this directory (`make`); it is not part of the
main Go build.

## Listing mode

With no `--exec`, it prints one `KEY=value` line per matching device and exits 0,
even when nothing matches (the output is just empty):

```sh
$ find-serial
DEVICE=/dev/cu.usbmodem11301 VID=1546 PID=01A9 MODEL="u-blox GNSS receiver" VENDOR="u-blox AG - www.u-blox.com"

$ find-serial --vid 1546 --pid 01A9
DEVICE=/dev/cu.usbmodem11301 VID=1546 PID=01A9 MODEL="u-blox GNSS receiver" VENDOR="u-blox AG - www.u-blox.com"
```

`MODEL` and `VENDOR` are the device's own USB product and vendor strings; each is
omitted when the device does not publish it. The quoted value can contain spaces;
any double quote or non-printable byte in the string is replaced with `_`. VID/PID
remain the reliable keys for matching.

## Exec mode

With `--exec`, the remaining arguments are a command to run, with exactly one
`{}` placeholder replaced by the matched device path. Use `--` to separate
find-serial's own options from the child command:

```sh
find-serial --vid 1546 --pid 01A9 --exec -- satpulsed -f satpulse.toml -d {}
```

Exec mode requires exactly one match: zero matches exits 69 (`EX_UNAVAILABLE`),
multiple matches exits 2.

## Waiting for a device

`--wait` (`-w`) makes find-serial block until a matching device appears instead
of failing when none is present yet. It first scans as usual; if there is no
match it waits on IOKit hot-plug notifications (no polling) and rescans as
devices arrive. This lets a launchd job start before the GPS receiver is plugged
in and simply wait for it:

```sh
find-serial --wait --vid 1546 --pid 01A9 --exec -- satpulsed -f satpulse.toml -d {}
```

Once a device matches, the normal exec semantics apply (including the ambiguous
exit if several appear at once).
