---
title: Message files
---

SatPulse's low-level receiver configuration uses message files.
A message file defines a collection of messages specific to a vendor protocol.
The format of a message file is TOML,
the same format used for the main SatPulse configuration file.
A message file is purely declarative:
it describes the messages themselves
and contains nothing specific to the program that sends them,
so the format is not tied to SatPulse's implementation of it.
There is no attempt to adapt a message to the receiver:
SatPulse sends what you tell it to send,
and it is up to you to decide what to send.
For vendors where SatPulse has no high-level configuration,
message files are how SatPulse configures the receiver;
for vendors where it does,
they provide access to receiver features that the high-level model does not cover.
SatPulse provides two ways of using message files:
from the command line with `satpulsetool gps`,
and from a GUI with SatPulse Workbench.

TODO: link the section overview for how the two configuration layers relate,
when it is written.

## A simple message file

The Quectel LG290P is configured with proprietary NMEA sentences.
SatPulse currently has no high-level configuration support for it,
so message files are how you configure it.
The simplest message file defines a single message.
This file enables the LG290P's output of the NMEA RMC sentence:

```toml
[[nmea]]
text = "PQTMCFGMSGRATE,W,RMC,1"
```

The structure of a message file is arrays of TOML tables,
where the name of the array is a message type:
the word inside the double brackets.
Each entry in an array defines one message of that type,
with the keys of the table describing the message.
An `nmea` message is an NMEA sentence given without its framing:
when it is sent, the `$` is prepended if missing,
and the checksum is computed and appended.

A file can define any number of messages,
which are sent in the order they appear.
This file enables the four NMEA sentences that satpulsed uses:

```toml
[[nmea]]
text = "PQTMCFGMSGRATE,W,RMC,1"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GGA,1"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GSV,1"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GSA,1"
```

A file like this is an ad-hoc sequence of messages, sent as a whole.

## Tags

Each message can be given a tag,
and a sender selects messages by tag:
given a list of tags, it sends the messages with those tags,
in the order the tags are given.
A file with tags is a collection of messages to select from,
rather than a sequence sent as a whole.
This file gives each message its own tag,
with an `-off` variant that undoes it:

```toml
[[nmea]]
text = "PQTMCFGMSGRATE,W,RMC,1"
tag = "nmea-rmc"
[[nmea]]
text = "PQTMCFGMSGRATE,W,RMC,0"
tag = "nmea-rmc-off"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GGA,1"
tag = "nmea-gga"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GGA,0"
tag = "nmea-gga-off"
```

Selecting `nmea-rmc` enables the RMC output,
and selecting `nmea-rmc-off` disables it again.
When a sender is given no tags, it sends the messages that have no tag;
that is why the untagged files above are sent as a whole.

A tag can also identify a group of messages that belong together.
Here one tag enables the four sentences that satpulsed uses:

```toml
[[nmea]]
text = "PQTMCFGMSGRATE,W,RMC,1"
tag = "nmea-daemon"
description = "Enable the NMEA messages used by satpulsed"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GGA,1"
tag = "nmea-daemon"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GSV,1"
tag = "nmea-daemon"
[[nmea]]
text = "PQTMCFGMSGRATE,W,GSA,1"
tag = "nmea-daemon"
```

Selecting `nmea-daemon` sends the four enabling messages as a unit.
A message can also be given a description,
which says what its tag does
and is shown when the tags in a file are listed;
a group is described once, on its first message.
[Using satpulsetool gps]({% link gps-config/satpulsetool.md %})
shows files like this being sent from the command line.

## Message types

Three message types are vendor-independent:
`nmea`, which the example above uses, `line`, and `binary`.

A `line` message is a plain text line, terminated with CR/LF by default.
This is how NovAtel-style receivers, such as the Unicore UM980, are configured;
for example, this UM980 command enables PPP using Galileo HAS corrections:

```toml
[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
```

A `binary` message is the exact bytes to send, written as a hex string.
For example, u-blox's
[GPS L5 application note](https://content.u-blox.com/sites/default/files/documents/GPS-L5-configuration_AppNote_UBX-21038688.pdf)
gives the command that makes a receiver use GPS L5 signals
regardless of their health status as a hex string,
which can be pasted directly into a message:

```toml
[[binary]]
hex = "B562068A0900000100000100321001DEED"
```

It is usually not convenient to specify a full byte sequence directly.
For receivers whose binary packet format SatPulse supports,
there are vendor-specific message types that describe a packet in text,
for example as a message class, id, and payload values given as numbers.
SatPulse does not need to know about the specific message,
just the protocol packet format:
sync bytes, length, byte order, checksum.
The vendor-specific message types are described
in the per-vendor configuration pages.

TODO: link the vendor pages when they are written.

## The rest of the format

A `[default.line]` table sets a key for every `line` message that
does not set it itself,
and the other message types have `default` tables of their own.
For example:

```toml
[default.line]
delay = 0.1
```

adds a pause of 0.1 seconds after sending each line.

The responses of a line protocol follow a vendor convention,
so for `line` messages the protocol alone does not determine
what a response looks like;
the `responsePattern` key names the convention,
so that responses can be matched to commands
and reported as a per-command OK or error.
The UM980 file in SatPulse's message file library sets `responsePattern = "unicore"`.
`waitLimit` changes how long the implementation waits for a message's responses
from the default 1.2 seconds.
`eol` changes a `line` message's terminator from the default CR/LF.

An `[[include]]` entry pulls in another message file,
with the path resolved relative to the including file:

```toml
[[include]]
src = "common.toml"
```

There is a JSON schema for message files, `gpsmsg-schema.json`,
which the Even Better TOML extension for Visual Studio Code
uses for validation and completion;
a `#:schema` comment on the first line of the file points to it.

The full format, including the protocol-specific message types,
is documented in `format.md` at the root of the message file library,
and the tag conventions in `tags.md`.

## The message file library

SatPulse includes a library of message files, organized by vendor.
Packages install the library under `/usr/share/satpulse/gpsmsg`.
The library includes files for vendors
that have no high-level configuration support:
for such a receiver, its library file is how you set it up for use with satpulsed.
[Using satpulsetool gps]({% link gps-config/satpulsetool.md %})
shows this done for the LG290P.

Tag names follow conventions shared across the vendor files,
documented in `tags.md` at the root of the library.

SatPulse Workbench has the library compiled into the binary
and lets you pick files and tags from it in its message file tab;
the **SATPULSE_GPSMSG_PATH** environment variable puts your own directories
ahead of the built-in library
(see [satpulsewb(1)]({% link man/satpulsewb.1.md %})).

## Creating message files with a coding agent

I have had good success using coding agents to create message libraries.
Implementing high-level configuration for a receiver is a lot of work;
a message file needs only the packet framing,
and once SatPulse has that,
going from the protocol manual to a working message file is straightforward.
The workflow:

1. convert the protocol spec from PDF into an agent-friendly format
   such as Markdown;
2. prompt the agent to generate a message library,
   giving it the spec, the message file format, and the tag conventions,
   along with a few existing files as examples;
3. let the agent use satpulsetool with the receiver attached,
   so that it can send each message and see whether the receiver acknowledges it,
   and capture packets before and after
   to see whether it has the documented effect.

The last step is what makes the result reliable:
each message is verified against the actual receiver,
not only against the manual.
Much of the included library was created and tested this way.

Message files also have a role in how vendor support itself is built:
before high-level support for a receiver exists,
they are the tool for experimenting with the receiver
and understanding how it behaves.
[Supporting a new vendor]({% link internals/vendor-support.md %})
describes that bring-up process.
