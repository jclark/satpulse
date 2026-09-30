# satpulsetool and packet logs

None of these touches hardware. Each takes a packet log as a file argument, or
`-` for standard input, and writes to standard output.

## Packet logs

A packet log is JSON Lines, one record per line, as written by
`gps --packet-log`, `serial --packet-log`, and by satpulsed when `packet` is
enabled in its `[log]` table (`packet.<device>.jsonl` in its log directory,
`/var/log/satpulse` by default). A record has a timestamp `t`, and for a
recognized packet a `tag` (packet protocol, such as `UBX`, `NMEA`, `RTCM`,
`NOVA`) and `msg` (message ID), with the packet data as hex in `bin` or as text
in `ascii`. Packets sent to the receiver have `"out":true`. Records without a
`tag` hold bytes that were not part of a recognized packet.

`--vendor` restricts or selects the packet formats as for `decode`; without it,
`SATPULSE_VENDORS` applies.

## annotate: a packet log with decoded fields

Adds `header`, `payload`, and `cfgData` fields to each record:

```
satpulsetool annotate capture.jsonl > decoded.jsonl
satpulsetool annotate < capture.jsonl
```

Typical workflow, capture then annotate:

```
satpulsetool serial -d /dev/ttyACM0 -s 38400 --packet-log cap.jsonl -t 10
satpulsetool annotate cap.jsonl > decoded.jsonl
```

## replay: the events a packet log produces

Runs the received packets through the processing pipeline and writes the
resulting events as JSON Lines, like a satpulsed event log (`time`, `posGeo`,
`velGeo`, `navEpoch`, `satellites`, ...). Processing warnings go to standard
error.

```
satpulsetool replay --vendor u-blox capture.jsonl > events.jsonl
```

`--idle-gap SECONDS` (default 0.15, 0 disables) is the gap between packets
treated as the receiver going idle.

No satpulsed instance is needed for this. Run satpulsed from a log (the
`drive-satpulsed-from-log` skill) only when the daemon itself is under test,
such as its web interface.

## Checking a decoding change against captures

Run `replay` (for events) or `annotate` (for decoded packets) on the same logs
with the build from before the change and the build with it, and `diff` the
outputs. Any difference, and any warning on standard error, is what the change
did to those captures.

## pack and scan: packet log to byte stream and back

`pack` writes the received packets' raw bytes; `scan` splits a raw byte stream
into packets and writes a packet log.

```
satpulsetool pack capture.jsonl > capture.bin
satpulsetool pack --tag UBX --msg NAV-PVT capture.jsonl > nav-pvt.ubx
satpulsetool scan --vendor u-blox capture.bin > capture.jsonl
satpulsetool scan capture.bin | satpulsetool annotate - > decoded.jsonl
```

`pack --tag` and `--msg` (which needs `--tag`) select packets,
case-insensitively. `pack --realtime FACTOR` reproduces the original spacing
between packets divided by FACTOR, for feeding something that expects live
input, such as satpulsed through a FIFO.

See satpulsetool-pack(1) and satpulsetool-scan(1).
