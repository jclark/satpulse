# GPS message files

This directory contains message files for configuring GPS receivers using the `-m` and `-t` options of `satpulsetool gps`.

`satpulsetool gps` provides options that allow convenient, high-level configuration of GPS receivers in a protocol-independent way.
But these options only work when `satpulsetool` includes the necessary code for the protocol used by the specific receiver.
Message files are less convenient but can work with any GPS receiver.
Message files can also be used to configure receiver-specific features that satpulsetool does not support in a high-level way.

## Usage

Send messages from a file:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m allystar.toml -t pps
```

The `-m` flag specifies the message file. The `-t` flag selects which tags to send.
Tags listed with `-t` are sent in the order they are listed.

Tag rules:
- A tag can only be used with one message type in a file (`line`, `binary`, `nmea`, `casbin`, `asbin`, or `ubx`).
- Within a message type section, all messages with the same tag must be consecutive.
- These rules apply to effective tags, including inherited default tags and the empty tag.

List available tags:

```
satpulsetool gps -m allystar.toml --show-tags
```

The `-m` flag cannot be combined with config flags like `--gnss`, `--pps`, `--save`.

You can use `--packet-log file.json --capture 3` options to capture packets for 3 seconds and save them to `file.jsonl`.
You can then use `satpulsetool decode --packet-log file.json` to decode the binary packets: pipe through `jq` to pretty-print the JSONL.
This can help with seeing whether your receiver is handling the commands correctly.

## Available message files

| File | Receiver | Protocol |
|------|----------|----------|
| [allystar.toml](allystar.toml) | Allystar (e.g. TAU1201, TAU951M) | Allystar binary |
| [lg290p.toml](lg290p.toml) | Quectel LG290P | NMEA (PQTM) |
| [atgm332d-v5.toml](atgm332d-v5.toml) | Zhongke Micro ATGM332D/ATGM336H firmware V5.x | CASIC binary |
| [atgm332d-v6.toml](atgm332d-v6.toml) | Zhongke Micro ATGM332D/ATGM336H firmware V6.x | CASIC binary |
| [ubx.toml](ubx.toml) | u-blox (e.g. ZED-F9P) | UBX binary, NMEA (PUBX) |
| [sinognss.toml](sinognss.toml) | SinoGNSS K901/K902 | NovAtel-style commands |
| [bynav.toml](bynav.toml) | Bynav M2 | NovAtel-style commands |
| [um980.toml](um980.toml) | Unicore UM980 | Unicore commands |
| [lc29h.toml](lc29h.toml) | Quectel LC29H | NMEA (PQTM, PAIR) |

## Configuring for satpulsed

### Allystar

Configure an Allystar receiver, such as the TAU1201, for use with satpulsed:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m allystar.toml -t pps,asbin-nav-time,asbin-nav-svinfo,nmea-off,gnss-all
```

This configures:
- `pps` - 1PPS output with 0.1s pulse width, only when the receiver has a lock
- `asbin-nav-time` - NAV-TIME binary messages at 1Hz (required by satpulsed)
- `asbin-nav-svinfo` - Satellite visibility info at 1Hz
- `nmea-off` - Disable all NMEA messages
- `gnss-all` - Enable all GNSS constellations

If you prefer to use a single constellation, you can use e.g. `gnss-gps` instead of `gnss-all`.

You can verify the configuration by querying current settings:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m allystar.toml \
  -t get-pps,get-gnss --packet-log verify.jsonl --capture 2
satpulsetool decode --packet-log verify.jsonl | jq
```

If the configuration seems to be working, you can save it to non-volatile memory:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m allystar.toml -t save
```

## Documentation

- [format.md](format.md) - Message file format
- [tags.md](tags.md) - Tag naming conventions
- [gpsmsg-schema.json](gpsmsg-schema.json) - JSON schema for editor support
