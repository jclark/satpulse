# Local clock bias/drift

Receiver clock bias (offset from GPS time) and clock drift (rate of change).
These are natural additions to `NavEpochMsg` since they are produced once per
navigation epoch alongside the position/velocity solution.

The vendor-neutral representation is bias in nanoseconds and drift in
nanoseconds per second. Some protocols use seconds or range-equivalent metres
(multiply by c); conversion is straightforward.

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

So the parser structs already exist for u-blox, Allystar, CASIC, NovAtel, and
Unicore. The missing piece is adding clock bias/drift fields to `gpsprot` and
wiring the existing parsed values through the handlers.
