# GPS Message Files

`satpulsetool gps` provides protocol-agnostic configuration, but:

- there will always be protocol-specific details that cannot be abstracted
- technical users are comfortable reading their receiver's manual and constructing exact commands

GPS message files let you send protocol-specific commands to a GPS receiver without modifying satpulsetool.
The message file format uses [TOML](https://toml.io/en/), the same format as the main SatPulse configuration file.

## Simplest case

Example file called `um980-ppp.toml`:

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

This specifies two messages each of which are lines.
The `[[line]]` is a TOML array of tables; each `[[line]]` entry defines one message.
The word inside the double brackets is the message type; `line` means a text line.

Send these messages with:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m um980-ppp.toml
```

The `-m` flag specifies the message file.
Each line will be terminated with CR/LF and sent to the receiver.

The `-m` flag cannot be combined with config flags like `--gnss`, `--pps`, `--save`, etc.
This avoids ambiguity about ordering of manual messages versus higher-level configuration.

## Delay key

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
delay = 0.1

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

This will add delay 0.1 seconds after sending the first line.

## Defaults

```toml
[default.line]
delay = 0.1

[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

This will add a delay of 0.1 seconds after every line.

## Line terminator

The `eol` key is a string specifying the line terminator.

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
eol = "\n"
```

This is usually specified in the `default.line` table:

```toml
[default.line]
eol = "\n"

[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

You can use `eol = ""` to send plain text with no line terminator.

## Binary messages

The `binary` message type sends raw bytes specified as a hex string:

```toml
[[binary]]
hex = "B562068A0900000100000100321001DEED"
```

Hex string must have even length. Whitespace within the hex string is ignored.

## NMEA messages

The `nmea` message type handles NMEA sentence framing automatically:

```toml
[[nmea]]
text = "PCAS04,3"
```

This is like `line`, except:
- leading `$` is prepended if missing
- trailing `*XX` checksum is computed and appended if missing
- CRLF is always appended

## Tags

Tags let you group messages and select which groups to send.

Call the file `um980.toml`:

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
tag = "ppp"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
tag = "ppp"

[[line]]
text = "SIGNALGROUP 1"
tag = "signalgroup1"

[[line]]
text = "SIGNALGROUP 2"
tag = "signalgroup2"
```

Use `-t` to select which tags to send:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m um980.toml -t ppp
```

This sends only the messages with tag `ppp`.

Use `-t signalgroup1,ppp` to send messages with tag signalgroup1 and then messages with tag ppp.

If there is no `-t` flag, messages with the empty tag `""` are sent.
Since none of the messages in this file have an empty tag, nothing would be sent without `-t`.

`-t foo,,bar` will send messages with foo tag, then empty tag, then bar tag.

Default tag can be set:

```toml
[default.line]
tag = "setup"

[[line]]
text = "CONFIG PPP CONVERGE 10 20"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"

[[line]]
text = "SIGNALGROUP 1"

[[line]]
text = "SIGNALGROUP 2"
tag = "signalgroup2"
```

The first three messages have tag `setup` (from the default); the last has tag `signalgroup2`.

## Description key

`description` is an optional string that documents what a tag does. It is displayed by `--show-tags`.

```toml
[[nmea]]
text = "PQTMVERNO"
tag = "version"
description = "Query firmware version"

[[nmea]]
text = "PQTMCFGPPS,R,1"
tag = "query-pps"
description = "Query PPS configuration"
```

Use `--show-tags` to list all tags in a message file:

```
satpulsetool gps -m lg290p.toml --show-tags
```

When multiple messages share the same tag, each can have a `description`. The rule is: all non-empty descriptions for a tag must be identical. This allows flexibility:

```toml
# Option 1: description on first message only
[[nmea]]
text = "PQTMCFGMSGRATE,W,RMC,1"
tag = "nmea-satpulse"
description = "Enable NMEA messages for satpulse"

[[nmea]]
text = "PQTMCFGMSGRATE,W,GGA,1"
tag = "nmea-satpulse"

[[nmea]]
text = "PQTMCFGMSGRATE,W,GSV,1"
tag = "nmea-satpulse"

# Option 2: repeat for clarity (must match)
[[nmea]]
text = "PQTMRESTOREPAR"
tag = "factory-reset"
description = "Restore factory defaults and reboot"

[[nmea]]
text = "PQTMSRR"
tag = "factory-reset"
description = "Restore factory defaults and reboot"
```

Default is empty string `""`. Not allowed in `[default.line]` etc.

## Protocol-specific message types

For u-blox UBX and CASIC binary protocols, use structured payload encoding:

```toml
[[ubx]]
tag = "gps-l5-health"
class = 0x06
id = 0x8A
payload.types = "U1U1U2U4U1"
payload.values = [0, 1, 0, 0x10320001, 1]

[[casbin]]
tag = "cfg-tp"
class = 0x06
id = 0x03
payload.types = "U1U2U4"
payload.values = [1, 100, 0x12345678]
```

| Key | Type | Description |
|-----|------|-------------|
| `class` | integer | Message class (0-255) |
| `id` | integer | Message ID (0-255) |
| `payload.types` | string | Type specifiers for payload encoding |
| `payload.values` | array | Values to encode into payload |

Type specifiers are two characters each:
- `U1`, `U2`, `U4` - unsigned integers (1, 2, or 4 bytes)
- `I1`, `I2`, `I4` - signed integers (1, 2, or 4 bytes)
- `R4`, `R8` - floating point (4 or 8 bytes)

## Message types summary

| Type | Keys | Framing |
|------|------|---------|
| `[[line]]` | `text`, `eol`, `delay`, `tag`, `description` | appends eol (default `\r\n`) |
| `[[binary]]` | `hex`, `delay`, `tag`, `description` | none |
| `[[nmea]]` | `text`, `delay`, `tag`, `description` | prepends `$`, appends `*XX\r\n` checksum |
| `[[ubx]]` | `class`, `id`, `payload`, `delay`, `tag`, `description` | UBX framing with header and checksum |
| `[[casbin]]` | `class`, `id`, `payload`, `delay`, `tag`, `description` | CASIC binary framing with header and checksum |

## Schema

There is a JSON schema for message files at `gpsmsg-schema.json` in this directory.
With Visual Studio Code, the [Even Better TOML](https://marketplace.visualstudio.com/items?itemName=tamasfe.even-better-toml)
extension supports schema-sensitive editing. The first line of the TOML file can have a line like this:

```
#:schema ./gpsmsg-schema.json
```

to tell the extension which schema to use.
