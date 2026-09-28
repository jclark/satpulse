# find-serial

`find-serial` is a small macOS-only tool that discovers USB serial devices via
IOKit. It is independent of satpulse: it does not read `satpulse.toml` or know
about `satpulsed` flags. The Homebrew service uses it to locate the GPS receiver
at launch time.

It matches USB serial callout devices (`/dev/cu.*`) only; non-USB serial
services such as Bluetooth are ignored. Four options narrow the matches, and
combining them requires all of them to hold:

| Option | Argument | Matches |
| --- | --- | --- |
| `-v`, `--vid` | hexadecimal | USB vendor ID |
| `-p`, `--pid` | hexadecimal | USB product ID |
| `-s`, `--serial` | string | USB serial number, exactly |
| `-l`, `--location` | hexadecimal | macOS location ID |

`--serial` never matches a device that publishes no serial number. `--location`
accepts the value with or without leading zeros, so `3114000` and `03114000` are
the same port.

Use `--vid`/`--pid` to pick out a model of receiver, and `--serial` or
`--location` to choose between two of the same model: `--serial` follows the
device between ports, `--location` follows the port between devices.

Build it with the `Makefile` in this directory (`make`); it is not part of the
main Go build.

## Listing mode

With no `--exec`, it prints one `KEY=value` line per matching device and exits 0,
even when nothing matches (the output is just empty):

```sh
$ find-serial
device=/dev/cu.usbserial-BG02DBNX vid=0403 pid=6001 location=3112000 serial="BG02DBNX" model="FT232R USB UART" vendor="FTDI"
device=/dev/cu.usbserial-31140 vid=0403 pid=6014 location=3114000

$ find-serial --vid 0403 --pid 6014
device=/dev/cu.usbserial-31140 vid=0403 pid=6014 location=3114000

$ find-serial --serial BG02DBNX
device=/dev/cu.usbserial-BG02DBNX vid=0403 pid=6001 location=3112000 serial="BG02DBNX" model="FT232R USB UART" vendor="FTDI"
```

`location` is the macOS USB location ID in hex, which encodes the controller and
the chain of hub ports the device is plugged into, so it changes only if the
device is moved to a different port.

`serial`, `model` and `vendor` are the device's own USB serial number, product
and vendor strings; each is omitted when the device does not publish it. The
quoted value can contain spaces; any double quote or non-printable byte in the
string is replaced with `_`. `model` and `vendor` are there to identify a device
by eye; they cannot be matched on.

Between them, `serial` and `location` account for the `/dev/cu.*` name: the
driver names the device after its serial number when it has one, and after a
shortened form of its location ID otherwise, as in the two devices above.

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
