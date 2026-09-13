# satpulsetool decode and annotate

Neither touches hardware.

## decode: one packet to JSON

Takes a positional DATA argument. If every character is a hex digit, DATA is
hex-encoded binary (odd length is an error); otherwise it is ASCII text with
`\r\n` appended. `--bin` or `--line` forces the interpretation.

```
satpulsetool decode b562010614000000...
satpulsetool decode '$GNGGA,034418.00,1343.91295,N,...*64'
satpulsetool decode --bin b562010614000000...
satpulsetool decode --line '$GNGGA,034418.00,1343.91295,N,...*64'
```

Options: `-c`/`--compact` for single-line JSON; `--out` to decode the packet
as one sent to the receiver rather than received from it (affects u-blox
CFG-VAL* messages, whose meaning depends on direction).

Binary replies that `gps -m` prints as hex are decoded this way.

## annotate: a packet log with decoded fields

Adds `header`, `payload`, and `cfgData` fields to each record of a JSON Lines
packet log, as written by `--packet-log`:

```
satpulsetool annotate capture.jsonl > decoded.jsonl
satpulsetool annotate < capture.jsonl
```

Typical workflow, capture then annotate:

```
satpulsetool serial -d /dev/ttyACM0 -s 38400 --packet-log cap.jsonl -t 10
satpulsetool annotate cap.jsonl > decoded.jsonl
```
