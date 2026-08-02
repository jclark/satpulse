---
title: Using satpulsetool gps
---

The `satpulsetool gps` command provides a command-line interface for GPS configuration.
It supports both [high-level configuration]({% link gps-config/high-level.md %})
and low-level configuration with [message files]({% link gps-config/msg-files.md %}).

The complete command-line syntax is documented in the man page
[satpulsetool-gps(1)]({% link man/satpulsetool-gps.1.md %}).

## Connecting to the receiver

All configuration commands require options that say how to connect to the receiver.

The normal way to do this is to use the `-d` option to specify the serial device
(e.g. `-d /dev/ttyUSB0`)
and the `-s` option to specify the serial speed (e.g. `-s 115200`).

If you have a `satpulse.toml` specifying the device and speed for satpulsed,
then you can use `-f /etc/satpulse.toml`.

A serial device cannot be directly accessed at the same time by satpulsed and satpulsetool.
You can use satpulsetool at the same time as satpulsed by adding
a `proxy.socket` entry to
([satpulse.toml(5)]({% link man/satpulse.toml.5.md %})),
and then using satpulsetool with the `--socket` to access the serial device via satpulsed
rather than directly.

With connection options alone, `satpulsetool gps` implies the `--show-receiver` option,
which probes the receiver and shows information about it.

```sh
satpulsetool gps -d /dev/ttyACM0 -s 115200
```

```
Vendor: u-blox
Hardware: ZED-F9P
Firmware: HPG 1.51 PROTVER 27.50
Supported GNSS: GPS,GAL,BDS,GLO,QZSS,SBAS
Supports: signal, speed, survey, surveyAcc, surveyMsg, fixedPos, fixedPosAcc, raw, rtcmMSM4, rtcmMSM7, rtcmBaseID, port
Packet formats detected: UBX
```

If this identifies the hardware and firmware,
then high-level configuration is supported on the receiver.
If not, it will just show what kinds of packets where received;
in this case you need to use low-level configuration.

## High-level configuration

TODO: write this section.


## Message files

A [message file]({% link gps-config/msg-files.md %}) is a TOML file
that defines a collection of named messages
that can be sent to the GPS receiver.

### Why message files?

If you don't have SatPulse, the way you would send messages to a receiver on Linux
would be either to use a terminal emulator
or `stty` and `cat` writing directly to the serial device.

`satpulsetool gps` provides several advantages compared to this.

- It does the framing.
  For an NMEA sentence, the checksum is computed and appended.
  A binary packet is described textually in the message file,
  similar to how the protocol manual describes it,
  and it produces the actual packet bytes:
  the sync bytes, the length,
  the payload values encoded in the correct widths and byte order,
  and the checksum.
- It manages the output from the receiver.
  The receiver may well be generating voluminous periodic output,
  which often includes binary messages;
  this makes it difficult to type anything into a terminal emulator
  and see the responses,
  and with `cat` you do not see them at all.
  It parses the output from the receiver
  to identify the messages that are responses,
  and reports how the receiver responded to each message sent.
- A message file is a collection of named messages:
  a configuration that works is kept as a file
  and sent again by selecting its tags,
  instead of being retyped.
  The message file library provides such files
  for a wide variety of receivers.
- All packets sent and received can be captured to a log
  (see [Capturing packets](#capturing-packets)).

### Using a message library

SatPulse includes a library of message files, organized by vendor;
packages install it under `/usr/share/satpulse/gpsmsg`.
For a receiver with no high-level configuration support,
its library file is how you set it up for use with satpulsed;
for a receiver with high-level support,
the library gives access to receiver features
that the high-level model does not cover.

`-m` gives the path of the message file.
To see what a file offers,
`--show-tags` lists its tags with their descriptions;
it does not need a receiver connected:

```sh
satpulsetool gps -m /usr/share/satpulse/gpsmsg/u-blox/gen9.toml --show-tags
```

```
Tags:
  get-pubx-04 - Poll time and clock status (includes tpGran)
  get-version - Poll MON-VER for software and hardware version
  get-gnss - Poll MON-GNSS for supported GNSS and signal plans
  min-cno-25 - Set minimum C/N0 to 25 dBHz
  ubx-nav-timegps - Enable NAV-TIMEGPS
  ...
```

Tag names follow conventions shared across the library,
and for things that high-level configuration covers,
they are similar to the names of the corresponding
`satpulsetool gps` options:
`pps`, `min-elev-15`, `speed-115200`.
A `get-` prefix makes a query (`get-pps`),
an `-off` suffix disables what the plain tag enables (`pps-off`),
and `save` saves the configuration to non-volatile memory.

`-t` selects the tags to send.
A good first tag to send to any receiver is `get-version`:

```sh
satpulsetool gps -d /dev/ttyACM0 -s 38400 \
  -m /usr/share/satpulse/gpsmsg/u-blox/gen9.toml -t get-version
```

```
UBX-MON-VER b5620a04dc0045585420434f524520312e3030202839653137313629000000000000000030303139303030300000524f4d20424153452030783131384232303630000000000000000000000046575645523d48504720312e35310000000000000000000000000000000050524f545645523d32372e353000000000000000000000000000000000004d4f443d5a45442d463950000000000000000000000000000000000000004750533b474c4f3b47414c3b424453000000000000000000000000000000534241533b515a5353000000000000000000000000000000000000000000d18a
```

A binary response is shown as the message name
and the packet bytes in hex;
paste the hex into `satpulsetool decode` to see the fields:

```sh
satpulsetool decode b5620a04dc0045585420434f524520312e3030202839653137313629000000000000000030303139303030300000524f4d20424153452030783131384232303630000000000000000000000046575645523d48504720312e35310000000000000000000000000000000050524f545645523d32372e353000000000000000000000000000000000004d4f443d5a45442d463950000000000000000000000000000000000000004750533b474c4f3b47414c3b424453000000000000000000000000000000534241533b515a5353000000000000000000000000000000000000000000d18a
```

```
{
  "tag": "UBX",
  "msg": "MON-VER",
  "payload": {
    "swVersion": "EXT CORE 1.00 (9e1716)",
    "hwVersion": "00190000",
    "extension": [
      "ROM BASE 0x118B2060",
      "FWVER=HPG 1.51",
      "PROTVER=27.50",
      "MOD=ZED-F9P",
      "GPS;GLO;GAL;BDS",
      "SBAS;QZSS"
    ]
  }
}
```

This sends the single message with the `ubx-rxm-cor` tag,
which enables the UBX-RXM-COR message reporting
the status of differential corrections received by the receiver,
something the high-level model does not cover:

```sh
satpulsetool gps -d /dev/ttyACM0 -s 115200 \
  -m /usr/share/satpulse/gpsmsg/u-blox/gen9.toml -t ubx-rxm-cor --port usb
```

```
ubx-rxm-cor: OK
```

The `OK` reports the receiver's acknowledgment.
`--port` selects the receiver port the setting applies to,
since message output on a u-blox receiver is configured per port;
here the receiver is connected by USB.
If you are not sure which port that is,
`--show-port` shows the port the host is communicating on.
Adding `--save` makes the setting persist across a receiver power cycle.

`-t` takes a list of tags, sent in the order given.
One command sets up a Quectel LG290P,
a receiver with no high-level support, for use with satpulsed:

```sh
satpulsetool gps -d /dev/ttyUSB0 -s 460800 \
  -m /usr/share/satpulse/gpsmsg/quectel/lg290p.toml -t pps,nmea-daemon,save
```

This sends the messages with the three tags:
`pps` enables the PPS output,
`nmea-daemon` enables the NMEA sentences that satpulsed uses,
and `save` makes the configuration survive a power cycle.

### One-off messages

For a one-off command, you do not need a tag or even a file on disk.
Messages that have no tag are sent when there is no `-t`,
and `-m -` reads the message file from standard input,
so a message can be sent with a shell here document:

```sh
satpulsetool gps -d /dev/ttyACM0 -s 115200 -m - <<'TOML'
[[ubxval]]
keys = [0x10350005]
types = "U1"
values = [1]
TOML
```

This enables Galileo OSNMA authentication
on a generation 9 or later u-blox receiver.
A `ubxval` message writes u-blox configuration values,
with the key id (here CFG-GAL-USE_OSNMA)
taken from the interface manual;
the receiver's acknowledgment is shown.

## Capturing packets

`--packet-log` writes every packet sent and received to a file,
as one JSON object per line,
and `--capture` continues capturing
for a given number of seconds after the responses.
This works with both configuration layers:
add `--packet-log` to a high-level command
to see exactly what was sent to the receiver and what came back,
or use it to check that a message had its documented effect.

To check what a configuration command actually did,
query the receiver and capture the packets;
the library files have `get-` tags for queries.
This polls a u-blox receiver for its supported GNSS and signal plans:

```sh
satpulsetool gps -d /dev/ttyACM0 -s 115200 \
  -m /usr/share/satpulse/gpsmsg/u-blox/ubx.toml \
  -t get-gnss --packet-log gnss.jsonl --capture 2
satpulsetool annotate gnss.jsonl
```

`satpulsetool annotate` prints the log
with the decoded form of each packet added;
pipe it through `jq` to pretty-print.
