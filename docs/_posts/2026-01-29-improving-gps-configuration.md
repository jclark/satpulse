---
title: "Improving SatPulse's support for GPS configuration"
date: 2026-01-29
---

In this post, I want to describe some recent improvements in how SatPulse supports GPS configuration.

## How GPS receivers communicate

To understand why this feature is useful, it helps to understand how GPS receivers work at the protocol level. There are three layers: packet format, periodic messages, and configuration.

GPS receivers communicate over a serial connection by sending and receiving a byte stream consisting of a sequence of packets. Each packet has a specific format, and the stream can mix packets in different formats. A packet format defines how to represent a message type and a type-specific structured payload as a sequence of bytes with a checksum.  NMEA 0183 is the most important standard packet format. A NMEA packet is called a sentence: it starts with `$` and ends with a two-character hex checksum and CR/LF. The payload is represented as comma-delimited ASCII fields. RTCM 3 is another standard packet format, used for GPS correction data such as RTK base station observations. It uses a binary encoding with a 24-bit checksum. Many vendors define their own proprietary binary packet formats. For example, u-blox defines the UBX format.

GPS receivers compute position-velocity-time (PVT) solutions periodically, typically once per second. For each solution, they output messages with the results of the solution, and other information about how the solution was computed (such as the satellites used). These messages will be represented as message types in a packet format. NMEA defines standard sentence types, such as RMC, GGA, GSV for this and all GPS receivers support them. Almost all GPS receivers can also emit other periodic messages that provide information beyond what can be expressed by standard NMEA sentences. Some GPS receivers make use of the extensibility mechanism provided by NMEA ("proprietary sentences" beginning with `$P`), but many others use messages in proprietary binary packet formats.

All GPS receivers provide a configuration mechanism. At a minimum they provide a way to control the speed of the serial connection and which messages are emitted. But most GPS receivers allow for many aspects of their operation to be configured, for example, which constellations they should use or the width of the PPS pulse that they should generate. For GPS configuration, it is completely vendor-dependent: there is no standard. Configuration requires communication in both directions: from host to GPS receiver and well as from GPS receiver to host. As with periodic messages, configuration uses packets. RTCM 3 messages go both from host to receiver as well as from receiver to host, so GPS receivers need to be able to deal with a mix of packet formats in the input they receive.

I have seen three different approaches to configuration.

- Configuration uses the same binary packet format used for periodic messages. UBX is the most common example of this. A few other vendors use UBX-like protocols: CASIC (Zhongke Microsystems) and Allystar
- Configuration requests (from host to receiver) use NMEA proprietary sentences; responses may be either standard NMEA TXT sentences or proprietary sentences. Quectel is one vendor that uses this approach.
- Configuration requests use plain ASCII lines (usually CRLF terminated); responses are typically some sort of ASCII packet. Septentrio and NovAtel both use this approach. Their products are both very high-end, but several Chinese vendors have adopted protocols based on NovAtel, notably Unicore, SinoGNSS (ComNav) and ByNAV, in more affordable products.

## SatPulse 0.1

SatPulse 0.1 internally uses protocol-independent interfaces for all three layers, but they are implemented only for UBX.

It might seem unnecessary to have such abstraction for only one protocol, but in fact UBX has many versions which have major differences.
Only the packet layer of UBX remains constant between versions. The periodic message layer has significant differences: different versions of receivers support different messages. With the configuration layer, u-blox receivers from generation 9 onwards use a completely different approach from receivers of generation 8 and earlier. The earlier generations have separate message types for each aspect of configuration. The later generation uses a single message that wraps a completely separate key-value configuration system: this allows multiple configuration changes to be performed atomically by a single message.

The fundamental idea in 0.1 is to create protocol independent representation of all the configuration that needs to be done. This representation has two parts:
- it specifies the desired values for various properties; for example, it might say that the time pulse should have a pulse width of 0.1s,
- it specifies various operations that should be performed; for example, it might say that the configuration of the receiver should be saved to non-volatile memory
This representation is handed over to a protocol-specific subsystem, which does as much of what was requested as the receiver supports.
It then returns the actual properties that the receiver was able to implement. For example, there is a property for the set of GNSS signals that should be enabled. If you request that L1 and L5 signals should be enabled, but the receiver only supports L1, then it will enable L1 and return a property with only L1 being enabled.

You might ask: why bother with all this? SatPulse works best if the receiver is configured in very specific ways. My goal is to provide a "just works" experience: so I want SatPulse to be able to detect what kind of receiver it is and automatically configure it so that it works optimally. SatPulse 0.1 does achieve this for a broad range of UBX receivers.

## Multi-protocol support

One of my main goals for SatPulse after version 0.1 is to make it truly multi-protocol. It is one thing to design an interface to be protocol-independent. It is another thing for it to actually work well for a range of protocols. In choosing the second protocol to support, I wanted a protocol that was rich and sophisticated, completely different from UBX and has decent documentation.

I decided on the protocol implemented by the UM98x series of GPS receivers from Unicore, which is based on the NovAtel OEM 6 and 7 protocols. This is primarily designed for the RTK market, but it works well for timing also. Periodic messages (which are called logs in the NovAtel world) have dual binary/ASCII representations. Configuration requests use plain ASCII lines; responses are NMEA-like.

In implementing the configuration support for the UM98x, I substantially reworked the configuration interface.
The objective is to make the protocol-specific code as simple and testable as possible.
The protocol-specific code is completely deterministic and does no IO.
An intermediate protocol-independent orchestration layer does IO and manages retries and timeouts.

The documented UM98x packet formats and logs are similar but distinct from the NovAtel OEM6/7 ones. But it turns out that UM98x can also be configured to generate packet formats and logs that are exactly as defined in the NovAtel documentation. These packet formats and logs are also implemented by a couple of other recent Chinese GNSS receivers (Bynav M20 and SinoGNSS K901), so I also implemented support for these.

## GPS message files

The approach to configuration in 0.1 has some fundamental limitations:

- it's a lot of work to implement configuration support
- if this work has not been done for a receiver, then SatPulse provides no help at all in configuration
- if you want to configure a receiver feature that SatPulse does not support, then again SatPulse provides no help

Most vendors provide a proprietary, but free-to-use Windows GUI app that can be used to configure their receivers.
For example, u-blox provides u-center.
In practice, for many configuration tasks with SatPulse 0.1, users would have to rely on these Windows apps.
For users living on Linux or macOS, this is not a good situation.

The multi-protocol support is a nice addition but does not address these limitations.

In the last week, I have implemented an alternative, more pragmatic approach to configuration that tries to address these limitations.
The approach is a simple one: have a file that defines messages that can be sent to the GPS receiver.
There is a new `-m`/`--msg-file` option for `satpulsetool gps` that specifies the message definition file.
This does not use the configuration abstraction at all and allows arbitrary messages to be sent to the receiver.
Since the main SatPulse configuration file is TOML, I also chose TOML for the format of this definition file.

Here's a simple example. The Unicore UM980 supports Galileo HAS, but SatPulse configuration abstraction does not yet have anything for this.
To enable Galileo HAS, create a file `um980-has.toml`.

```toml
[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
[[line]]
text = "CONFIG PPP CONVERGE 10 20"
```

Each `[[line]]` block defines a text command. Run it with:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m um980-has.toml
```

This will send each line, terminated with CR/LF by default.
When used in this simple way, the main problem that this solves is enabling the user to see how the GPS receiver responded to the message.
Since SatPulse knows about packet formats, it can intelligently identify which packets might be responses to the message and display only those.
If you use `cat` and `stty`, you have no idea how the receiver responded.
If you try to use a terminal emulator, the periodic messages being continually output by the receiver (which may include binary) makes it difficult to see the responses.
There is also a `--packet-log` option that allows you to capture all packets sent and received.

### NMEA message type

The `nmea` message type is similar to `line`, but it knows about NMEA checksums.
For example, this is how to configure PPS on a Quectel LG290P.

```toml
[[nmea]]
text = "PQTMCFGPPS,W,1,1,100,2,1,0"
```

The tool prepends `$` if missing and appends the checksum automatically.

### Message libraries

The message file can also be used to create a library of messages.
Each message can be given a tag and a description.


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

Run specific tags with `-t`:

```
satpulsetool gps -d /dev/ttyUSB0 -s 460800 -m quectel.toml -t nmea-daemon,save
```

The `--show-tags` flag lists available tags with descriptions.

```
satpulsetool gps -m quectel.toml --show-tags
```

### Binary messages

Sending binary messages is a less straightforward.

You can specify the exact bytes to send. For example, with u-blox L5 receivers,
there is a special command you need to send get GPS L5 signal to work.
The u-blox docs give it as a hex string.

```toml
[[binary]]
hex = "B562068A0900000100000100321001DEED"
tag = "gps-l5-health"
description = "Use GPS L5 signal regardless of health status"
```

But usually it is not convenient to specify the full byte sequence directly.

When SatPulse has support for a binary packet format, you can use this to specify things in a more readable way.
For example, let's take the CASIC binary protocol, which is similar to UBX.
SatPulse has support for the packet format layer, but does not have configuration support.
There's a CFG-TP message for controlling the time pulse.
The CASIC specification describes it like this.

**CFG-TP (0x06 0x03)**

**Payload Content**

| Offset | Type | Name | Unit | Description |
| :---- | :---- | :---- | :---- | :---- |
| 0 | U4 | interval | us | Pulse Interval |
| 4 | U4 | width | us | Pulse Width |
| 8 | U1 | enable |  | Enable Flag (0=Off, 1=On, 2=Auto Maintain, 3=Fix Only) |
| 9 | I1 | polar |  | Polarity (0=Rising, 1=Falling) |
| 10 | U1 | timeRef |  | 0=UTC, 1=Satellite Time |
| 11 | U1 | timeSource |  | 0=GPS, 1=BDS, 2=GLN, 4=BDS(Main), 5=GPS(Main), 6=GLN(Main) |
| 12 | R4 | userDelay | s | User Delay |


We can use this to define a message as follows:

```toml
[[casbin]]
tag = "pps-gps"
description = "Enable PPS aligned to GPS time"
class = 0x06
id = 0x03
payload.types = "U4U4U1I1U1U1R4"
payload.values = [1000000, 100000, 3, 0, 1, 0, 0.0]
```

Each type descriptor in the `payload.types` string specifies how to encode corresponding entry in `payload.values`.
SatPulse doesn't know about the CFG-TP message but it does know how CASIC binary packets work, and it can use this
to produce the right packet from this higher-level description (the packet has two sync bytes, the payload length, the class, the message id, the payload with values in little-endian byte-order and then a checksum).

### Using AI to create message libraries

I have had good success using AI to create message libraries. The workflow is:

1. convert the protocol spec from PDF into a more AI-friendly format such as Markdown
2. prompt a coding agent (e.g., Claude Code) to generate a message library, giving it:
	- the protocol spec
	- description of the message file format
	- a few examples of a message library
3. allow the agent to use satpulsetool to access the GPS receiver:
	- try a message in the message library and see whether the receiver ACKs it
	- capture output from the receiver before and after to see whether the configuration message has had the expected effect on the output (use `--capture N` with `--packet-log` to capture packets for N seconds)

### Examples

The [configs/gpsmsg](https://github.com/jclark/satpulse/tree/master/configs/gpsmsg) directory in the repository has some example message libraries.