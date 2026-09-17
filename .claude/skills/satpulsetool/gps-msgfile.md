# satpulsetool gps: sending messages from a message file

Low-level configuration uses a message file: a TOML file defining named
messages for a specific receiver family, selected by tag. This works on any
receiver, including ones without high-level support, and reaches settings
outside the data model.

## Finding the file

Message files are installed in `/usr/share/satpulse/gpsmsg`, one
subdirectory per vendor (`u-blox`, `unicore`, `quectel`, `allystar`,
`zhongke`, `techtotop`, `bynav`, `sinognss`, `septentrio`), each with a
`README.md` saying which file covers which modules. `tags.md` in the top
directory gives the tag naming conventions, so a tag for a common setting
(PPS, constellations, elevation mask, restart, save) has a predictable name.

List the tags in a file with their descriptions:

```
satpulsetool gps -m /usr/share/satpulse/gpsmsg/unicore/um980.toml --show-tags
```

This needs no receiver connection.

## Sending

Send one tag, or several in order:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m /usr/share/satpulse/gpsmsg/unicore/um980.toml -t pps
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m /usr/share/satpulse/gpsmsg/unicore/um980.toml -t get-version,get-pps
```

satpulsetool correlates receiver responses with the messages it sent and
reports whether each was accepted or rejected; a rejection gives exit status
1. Without `-t`, messages with no tag are sent.

Query tags (`get-*`) print the reply; binary replies are printed as hex, and
`satpulsetool decode --bin` decodes them (see `decode.md`). `--packet-log`
records the whole exchange.

## u-blox specifics

For files using `[[ubxval]]` or `[[ubxvalport]]` entries (u-blox generation 9
and later):

- `--save` with `-m` persists the CFG-VALSET write to RAM, BBR, and flash instead of RAM only. Confirm with the user first (see SKILL.md).
- `[[ubxvalport]]` entries depend on which receiver port the host is on; `--port uart1|uart2|usb|i2c|spi` says which. `--show-port` (a high-level option, so a separate invocation) reports it.
