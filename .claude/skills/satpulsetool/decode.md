# satpulsetool decode

Does not touch hardware. To decode a whole packet log, see `packetlog.md`.

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
CFG-VAL* messages, whose meaning depends on direction); `--vendor` to decode
a NovAtel-format packet (tags `NOVA`, `NOVB`) as the named vendor's variant
of the protocol, which decides the port encoding and any vendor-specific
logs. Without `--vendor`, `SATPULSE_VENDORS` applies.

Binary replies that `gps -m` prints as hex are decoded this way.
