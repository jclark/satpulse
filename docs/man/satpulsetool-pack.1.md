# NAME

satpulsetool-pack - convert a JSONL packet log to a packet byte stream

# SYNOPSIS

**satpulsetool** [*global options*] **pack** [**\-h**\|**\-\-help**]\
&nbsp;&nbsp;&nbsp;&nbsp;[**\-t**\|**\-\-tag** *tag*] [**\-m**\|**\-\-msg** *msg*] [**\-r**\|**\-\-realtime**]\
&nbsp;&nbsp;&nbsp;&nbsp;*file*\|**\-**

# DESCRIPTION

The **satpulsetool** **pack** command reads a JSONL packet log and writes a packet byte stream corresponding to the original packet contents.
Packet logs are produced by **satpulsetool** **gps** with **\-\-packet\-log**,
and by **satpulsed(8)** when packet logging is enabled.

By default, **pack** writes packets received from the GPS receiver.
Log entries for packets sent to the receiver are skipped.
Entries that do not contain packet data are also skipped.

If the log entry contains a `bin` field, it is decoded from hex and written as binary data.
Otherwise, if the log entry contains an `ascii` field, the string is written exactly as stored.
The command does not rescan, re-encode, re-checksum or otherwise interpret the packet contents.

The output is written to standard output.
Use **\-** as *file* to read from standard input.

# OPTIONS

**\-h**, **\-\-help**
: Show usage help for the **pack** command.

**\-t**, **\-\-tag** *tag*
: Write only packets whose `tag` field matches *tag*.
The match is case-insensitive.
Typical values are **UBX**, **RTCM**, and **NMEA**.

**\-m**, **\-\-msg** *msg*
: Write only packets whose `msg` field matches *msg*.
The match is case-insensitive.
This option requires **\-\-tag**, since message IDs are meaningful only within a packet protocol.

**\-r**, **\-\-realtime**
: Preserve the timing between packets that are written.
The first selected packet is written immediately.
Subsequent selected packets are delayed so that the time between write starts matches the time between packet timestamps in the log.
When this option is used, standard output is flushed after every packet.
Each selected packet must have a non-zero `t` timestamp.

# EXAMPLES

Convert a packet log to a packet byte stream:

    satpulsetool pack packets.jsonl > packets.bin

Extract only UBX packets:

    satpulsetool pack --tag UBX packets.jsonl > ubx.bin

Extract UBX NAV-PVT packets:

    satpulsetool pack --tag UBX --msg NAV-PVT packets.jsonl > nav-pvt.ubx

Extract RTCM message 1077 packets:

    satpulsetool pack --tag RTCM --msg 1077 packets.jsonl > 1077.rtcm3

Replay a recorded packet log with original packet pacing through a FIFO:

    mkfifo /tmp/fifo0
    satpulsetool pack --realtime \
        /var/log/satpulse/packet.ttyACM0.jsonl > /tmp/fifo0

This can drive **satpulsed(8)** without GPS hardware, using a `satpulse.toml` like this.

    [serial]
    device = "/tmp/fifo0"

    # Enable event log in /tmp/satpulse-log/event.fifo0.jsonl
    [log]
    dir = "/tmp/satpulse-log"
    event = true

    # Enable Web UI
    [[http]]
    listen = ":2001"

# SEE ALSO

**satpulsetool(1)**, **satpulsetool-scan(1)**, **satpulsetool-gps(1)**, **satpulsed(8)**
