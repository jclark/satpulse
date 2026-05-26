# GPS message file format

GPS message files let you send protocol-specific commands to a GPS receiver without modifying satpulsetool.
The message file format uses [TOML](https://toml.io/en/), the same format as the main SatPulse configuration file.

## Simplest case

Example file called `um980-has.toml`:

```toml
[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
```

This specifies two messages each of which are lines.
The `[[line]]` is a TOML array of tables; each `[[line]]` entry defines one message.
The word inside the double brackets is the message type; `line` means a text line.

Send these messages with:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m um980-has.toml
```

The `-m` flag specifies the message file.
Each line will be terminated with CR/LF and sent to the receiver.

The `-m` flag cannot be combined with config flags like `--gnss`, `--pps`, etc.
This avoids ambiguity about ordering of manual messages versus higher-level configuration.
The `--save` flag is permitted with `-m`, but only for save-aware message types (`[[ubxval]]` and `[[ubxvalport]]`);
see [u-blox Gen9+ CFG-VALSET](#u-blox-gen9-cfg-valset) below.

## Delay key

```toml
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
delay = 0.1

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

This will add delay 0.1 seconds after sending the first line.

## Wait limit

The `waitLimit` key says how long to wait for the receiver.
This applies in two situations:
between messages, if the next message would produce an ACK indistinguishable from a still-pending one;
and after all messages have been sent, for any remaining expected ACKs and data responses.

The `waitLimit` key works by establishing a response deadline.
Each message extends the deadline by its `waitLimit` from the time it was sent;
a later message with a shorter `waitLimit` won't pull the deadline back.
After sending, waiting stops early if all expected responses arrive before the deadline.

The default `waitLimit` is 1.2 seconds.

```toml
[[nmea]]
text = "PQTMVERNO"
waitLimit = 3.0
```

If `--capture` is also specified, capture time is added after response waiting is complete.

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

## Response pattern

The `responsePattern` key tells satpulsetool how to match responses from the receiver to sent commands.
This enables display of per-command ACK/NAK results instead of raw protocol data.

```toml
[default.line]
responsePattern = "unicore"

[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

Supported values:

| Value | Description |
|-------|-------------|
| `"unicore"` | Unicore receivers (UM980, etc.). Matches `$command,CMD,response: OK` acks and `$CONFIG,...` data replies. |

If omitted, line messages use generic matching (displays any non-periodic text as potentially relevant).

## Binary messages

The `binary` message type sends raw bytes specified as a hex string.
For example, with u-blox L5 receivers, there is a special command you need to send to get GPS L5 signal to work:

```toml
[[binary]]
hex = "B562068A0900000100000100321001DEED"
tag = "gps-l5-health"
description = "Use GPS L5 signal regardless of health status"
```

Hex string must have even length. Whitespace within the hex string is ignored.

## NMEA messages

The `nmea` message type handles NMEA sentence framing automatically.
For example, this is how to configure PPS on a Quectel LG290P:

```toml
[[nmea]]
text = "PQTMCFGPPS,W,1,1,100,2,1,0"
```

This is like `line`, except:
- leading `$` is prepended if missing
- trailing `*XX` checksum is computed and appended if missing
- CRLF is always appended

## Tags

Tags let you group messages and select which groups to send.

```toml
[[nmea]]
text = "PQTMCFGMSGRATE,W,RMC,1"
tag = "nmea-daemon"
description = "Enable NMEA messages understood by satpulse daemon"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GGA,1"
tag = "nmea-daemon"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GSA,1"
tag = "nmea-daemon"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GSV,1"
tag = "nmea-daemon"
[[nmea]]
text = "PQTMSAVEPAR"
tag = "save"
description = "Save configuration to NVM"
```

Use `-t` to select which tags to send:

```
satpulsetool gps -d /dev/ttyUSB0 -s 460800 -m quectel.toml -t nmea-daemon,save
```

This sends messages with tag `nmea-daemon`, then messages with tag `save`.

The `--show-tags` flag lists available tags with descriptions:

```
satpulsetool gps -m quectel.toml --show-tags
```

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

When multiple messages share the same tag, each can have a `description`. The rule is: all non-empty descriptions for a tag must be identical.

```toml
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

For u-blox UBX, CASIC, Allystar, SDBP (Techtotop/Taidou) binary protocols, use structured binary payload encoding.
These binary formats are all UBX-like: they have sync bytes, message class, message ID, payload length and checksum.
The payload consists of binary integers and floats of various widths.

For example, the CASIC CFG-TP message (0x06 0x03) controls the time pulse:

```toml
[[casbin]]
tag = "pps-gps"
description = "Enable PPS aligned to GPS time"
class = 0x06
id = 0x03
payload.types = "U4U4U1I1U1U1R4"
payload.values = [1000000, 100000, 3, 0, 1, 0, 0.0]
```

For Allystar receivers (TAU1201, etc.), use `asbin`:

```toml
[[asbin]]
tag = "pps"
description = "Configure 1PPS with 100us pulse width, rising edge"
class = 0x06
id = 0x07
payload.types = "U4I4U4U1U1U1"
payload.values = [1000000, 0, 100000, 1, 13, 1]
```

Each type descriptor in the `payload.types` string specifies how to encode the corresponding entry in `payload.values`.
SatPulse doesn't need to know about the specific message, just the protocol packet format.
It uses this to produce the correct packet.

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

## u-blox Gen9+ CFG-VALSET

The `[[ubxval]]` message type writes one or more items to the u-blox Gen9+
configuration database using `UBX-CFG-VALSET`. Unlike raw `[[ubx]]` entries,
the layer byte is chosen by the `--save` command-line option, so the same
TOML file can be used for either a RAM-only or persistent write.

```toml
[[ubxval]]
tag = "osnma-on"
description = "Enable Galileo OSNMA authentication"
keys = [0x10350005]
types = "U1"
values = [1]
```

Without `--save`, this writes the item to RAM only. With `--save`, the
same item is written to `RAM|BBR|Flash`.

A single `[[ubxval]]` entry can carry several items: provide parallel
`keys`, `types`, and `values` arrays of equal length, with one type
specifier per value (two characters each, matching the `payload.types`
grammar described above). All items are sent in one CFG-VALSET packet.

```toml
[[ubxval]]
tag = "osnma-and-only-auth"
keys = [0x10350005, 0x101100DD]
types = "U1U1"
values = [1, 1]
```

The `types` grammar mirrors `payload.types`: `U1`, `U2`, `U4`, `I1`, `I2`,
`I4`, `R4`, `R8`. Supplied SatPulse UBX TOML files use the prefix that
matches the scalar type in the u-blox specification:

| u-blox scalar | `[[ubxval]]` type |
|---------------|----------------------|
| `Un`, `Xn`, `E1` | `U1`, `U2`, or `U4` |
| `In` | `I1`, `I2`, or `I4` |
| `Rn` | `R4` or `R8` |
| `L` (boolean) | `U1` with value `0` or `1` |

For each item the encoded byte width must match the value width encoded
in the Key ID; for example `CFG-MSGOUT_*` keys have width 1 and must use
`U1` with `0` or `1`. 8-byte integer types are not currently accepted
in this format.

Raw `[[ubx]]` packets are not interpreted, so `--save` does not rewrite a
hard-coded `CFG-VALSET` layer byte inside a raw `payload`. Persistent
configuration writes from a message file must use `[[ubxval]]` or
`[[ubxvalport]]`.

## u-blox Gen9+ port-dependent CFG-VALSET

The `[[ubxvalport]]` message type is a port-aware variant of `[[ubxval]]`
for one-byte port-dependent CFG keys such as `CFG-MSGOUT-*_<PORT>` and
`CFG-INFMSG-*_<PORT>`. The message file supplies one or more port-specific
key IDs and a single one-byte `value`. The receiver port is chosen by
the command-line `--port` argument when the file is sent, not by the
message file:

```toml
[[ubxvalport]]
tag = "ubx-nav-timeutc"
description = "Enable UBX-NAV-TIMEUTC"
key.usb = 0x2091005e
value = 1
```

Send it with `--port`:

```
satpulsetool gps -d /dev/ttyACM0 -m ubx9.toml -t ubx-nav-timeutc --port usb
```

`--port` accepts `i2c`, `uart1`, `uart2`, `usb`, and `spi` (case
insensitive).

When fewer than five `key.<port>` values are supplied, the helper
infers the keys for the other ports using the standard u-blox port
offset (`i2c=0`, `uart1=1`, `uart2=2`, `usb=3`, `spi=4`). Only known
inferable families are accepted in this mode:

| Family | Group ID |
|--------|----------|
| `CFG-MSGOUT` | `0x2091` |
| `CFG-INFMSG` | `0x2092` |

If multiple keys are supplied they must imply the same port-neutral
base key.

When all five `key.<port>` values are supplied explicitly, no family or
offset-pattern check is performed. The helper picks the key matching
`--port` directly. This supports port-dependent keys that do not follow
the offset pattern, such as `CFG-*OUTPROT-*`:

```toml
[[ubxvalport]]
tag = "ubx-outprot-ubx"
description = "Enable UBX output protocol"
key.i2c   = 0x10720001
key.uart1 = 0x10740001
key.uart2 = 0x10760001
key.usb   = 0x10780001
key.spi   = 0x107a0001
value = 1
```

Like `[[ubxval]]`, `--save` controls the CFG-VALSET layer mask. Without
`--save`, items are written to RAM only. With `--save`, items are
written to `RAM|BBR|Flash`.

## Message types summary

| Type | Keys | Framing |
|------|------|---------|
| `[[line]]` | `text`, `eol`, `responsePattern`, `delay`, `waitLimit`, `tag`, `description` | appends eol (default `\r\n`) |
| `[[binary]]` | `hex`, `delay`, `waitLimit`, `tag`, `description` | none |
| `[[nmea]]` | `text`, `delay`, `waitLimit`, `tag`, `description` | prepends `$`, appends `*XX\r\n` checksum |
| `[[ubx]]` | `class`, `id`, `payload`, `delay`, `waitLimit`, `tag`, `description` | UBX binary packets |
| `[[ubxval]]` | `keys`, `types`, `values`, `delay`, `waitLimit`, `tag`, `description` | UBX-CFG-VALSET (layer chosen by `--save`) |
| `[[ubxvalport]]` | `key.<port>`, `value`, `delay`, `waitLimit`, `tag`, `description` | UBX-CFG-VALSET with one-byte port-dependent key (port chosen by `--port`, layer by `--save`) |
| `[[casbin]]` | `class`, `id`, `payload`, `delay`, `waitLimit`, `tag`, `description` | CASIC binary packets |
| `[[asbin]]` | `class`, `id`, `payload`, `delay`, `waitLimit`, `tag`, `description` | Allystar binary packets |
| `[[sdbp]]` | `class`, `id`, `payload`, `delay`, `waitLimit`, `tag`, `description` | Techtotop/Taidou SDBP binary packets |

## Includes

Message files can include other message files:

```toml
[[include]]
src = "common.toml"
```

The `src` path is resolved relative to the directory of the including file.
It should use `/` as a directory separator, which will be converted to the OS path separator.

Tags in an including file override tags in an included file. Apart from this, it is an error for the same tag to appear in multiple included files. Defaults are applied to each file separately.
`--show-tags` will show tags from included files in order, after the tags from the including file.

## Schema

There is a JSON schema for message files at `gpsmsg-schema.json` in this directory.
With Visual Studio Code, the [Even Better TOML](https://marketplace.visualstudio.com/items?itemName=tamasfe.even-better-toml)
extension supports schema-sensitive editing. The first line of the TOML file can have a line like this:

```
#:schema ./gpsmsg-schema.json
```

to tell the extension which schema to use.
