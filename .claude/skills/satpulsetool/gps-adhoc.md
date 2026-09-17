# satpulsetool gps: sending an ad-hoc command

When no message file tag does the job, write the command as a here document
with `-m -`. The message file format handles checksums, framing, and binary
packing. Full format reference: `format.md` in the message directory
(`/usr/share/satpulse/gpsmsg`).

Before doing this, check `--show-tags` on the vendor's message file
(`gps-msgfile.md`): the command may already exist under a tag.

## Line command (Unicore, NovAtel, ByNav style)

For Unicore receivers set `responsePattern = "unicore"`, so satpulsetool can
match the receiver's reply and report whether the command was accepted;
without it the tool cannot tell. A `[default.line]` block applies it to
every `[[line]]` in the document:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m - <<'TOML'
[default.line]
responsePattern = "unicore"
[[line]]
text = "CONFIG PPP ENABLE E6-HAS"
TOML
```

## NMEA command

The checksum is computed for you:

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m - <<'TOML'
[[nmea]]
text = "PQTMCFGPPS,W,1,1,100,2,1,0"
TOML
```

## Binary command

Give the message class and id and the payload as type specifiers plus
values; sync bytes, length, and checksum are added. Supported binary tables:
`[[ubx]]` (u-blox), `[[asbin]]` (Allystar), `[[casbin]]` (CASIC), `[[sdbp]]`
(Techtotop/Taidou). The type specifiers (`U1`, `U2`, `U4` unsigned; `I1`,
`I2`, `I4` signed; `R4`, `R8` float) can usually be read straight from the
protocol specification for the message.

```
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m - <<'TOML'
[[ubx]]
class = 0x06
id = 0x03
payload.types = "U4U4U1I1U1U1R4"
payload.values = [1000000, 100000, 3, 0, 1, 0, 0.0]
TOML
```

Add `--packet-log FILE` to see the response. `--capture N` keeps listening N
seconds after the reply wait ends.
