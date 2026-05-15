# Local clock bias/drift (#279)

Receiver clock bias (offset from GPS time) and clock drift (rate of change)
are produced by several protocols. The decoding layer (`gps/lib/*`) already
parses these fields into vendor-specific structs, but there is no
vendor-neutral representation in `gpsprot`, so the protocol handlers in
`gps/internal/*` discard the values.

This plan covers adding a vendor-neutral representation in `gpsprot` and
wiring the existing decoded values through the protocol handlers. The
proposed home is `NavEpochMsg`, which is already emitted once per
navigation epoch alongside the position/velocity solution and accuracy
fields.

The vendor-neutral units are bias in nanoseconds and drift in nanoseconds
per second. Protocols that report seconds or range-equivalent metres are
converted at the handler boundary.

Where this information is documented in implemented protocols:

- u-blox `UBX-NAV-CLOCK` (0x01 0x22): `clkB` (I4, ns), `clkD` (I4, ns/s),
  plus `tAcc` (U4, ns) and `fAcc` (U4, ps/s)
- Allystar `NAV-CLOCK` (0x01 0x22): identical layout to u-blox --
  `clkB` (S4, ns), `clkD` (S4, ns/s), `tAcc`, `fAcc`
- CASIC ZKW3 `NAV2-CLK` (0x11 0x07): `clkBias` (I4, ns),
  `dfxTcxo` (R4, s/s relative frequency bias), `tAcc`, `fAcc`
- NovAtel `CLOCKMODEL` (ID 16): `bias` (Double, m), `rate` (Double, m/s),
  plus variance fields -- range-equivalent units (divide by c)
- NovAtel `TIME` (ID 101): `offset` (Double, s), `offset_std` (Double, s) --
  bias only, no drift
- Unicore `RECTIME` (ID 102): `offset` (Double, s), `offset_std` (Double, s)
  -- bias only, no drift
- Bynav `TIME` (ID 101): `offset` (Double, s) -- same as NovAtel, no drift
- Sinognss `TIME` (ID 101): `offset` (Double, s) -- same, no drift

Not available:

- Quectel PQTM: no clock solution message
- NMEA: no standard sentence for receiver clock

Comparison to current code:

- `gps/lib/ubxbin/nav.go` already parses `NavClock` with `ClkB`, `ClkD`,
  `TAcc`, `FAcc` -- but the handler in `gps/internal/ubx/` does not consume it.
- `gps/lib/asbin/nav.go` already parses `NavClock` with identical fields --
  handler in `gps/internal/as/` does not consume it.
- `gps/lib/casbin/nav.go` parses `NavClock` (0x01 0x11) with `FreqBias`,
  `TAcc`, `FAcc`. The ZKW3 `NAV2-CLK` (0x11 0x07) is registered but not parsed.
- `gps/lib/novmsg/time.go` parses `Time` with `Offset` and `OffsetStd` --
  `gps/internal/nov/time.go` uses it for time but does not extract clock fields.
- `gps/lib/uncmsg/time.go` wraps `novmsg.Time` as `RecTime` --
  `gps/internal/unc/` uses it for time but does not extract clock fields.

So decoding-layer parser structs already exist for u-blox, Allystar, CASIC,
NovAtel, and Unicore. The missing pieces are (1) a vendor-neutral
representation in `gpsprot` (fields on `NavEpochMsg`) and (2) wiring the
existing decoded values through the `gps/internal/*` handlers.
