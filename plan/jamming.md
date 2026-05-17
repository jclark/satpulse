# Receiver-level jamming detected (#231)

`JammingDetected opt.Val[bool]` is a reasonable additive field on
`NavEpochMsg`.

The only vendor-neutral meaning that survives across the protocols we
implement is a coarse receiver-level detector:

- `true`: the receiver reports significant RF interference/jamming on at least
  one monitored GNSS RF path, band, or frequency
- `false`: the receiver explicitly reports no such interference
- absent: the protocol does not expose a compatible detector, or reports
  unknown / disabled / unavailable

This should be treated as an explicit detector output, not synthesized from
raw AGC, noise, or CW magnitude. Some protocols expose richer per-band or
per-frequency data; `JammingDetected` is the lossy epoch-level projection.

Where this information is documented in implemented protocols:

- u-blox `UBX-SEC-SIG`: `jamDetEnabled`, `jamState`, and repeated
  per-center-frequency `jammed` flags; `jamState` is the best current source
- u-blox `UBX-MON-RF`: older/deprecated fallback `jammingState` per RF block,
  plus `noisePerMS`, `agcCnt`, `cwSuppression`
- CASIC `MON-SEC`: `jamDetEn`, `jamLevel`
- CASIC `MON-JSM`: `jamDetEn`, `jamLevel`, plus repeated `jamLevelChn`
- Unicore `JAMSTATUS`: `CWFlag` and `CWRatio`
- Unicore `FREQJAMSTATUS`: `L1CWFlag`, `L2CWFlag`, `L5CWFlag` and per-band
  ratios
- Bynav `ANTIJAMTYPE`: per-frequency interference type; anything except
  `non interference` can map to `true`
- NovAtel `ITDETECTSTATUS`: detected interference entries on `L1`, `L2`, `L5`;
  presence of any entry can map to `true`

Not available / not a clean bool source:

- NMEA: no standard sentence
- Allystar `MON-CWI`: frequency offset and peak value for CW interference, but
  no obvious vendor-neutral threshold for a bool
- raw `noisePerMS`, `agcCnt`, `CWRatio`, or `Peak value` should not on their
  own define the bool unless the protocol also provides an explicit status bit

Comparison to current code:

- `gps/gpsprot/msg.go` already has `NavEpochMsg` as the natural home; adding
  `JammingDetected opt.Val[bool]` is an additive change like `DiffAge`.
- `gps/lib/ubxbin` does not currently parse `UBX-SEC-SIG` or `UBX-MON-RF`, so
  `gps/internal/ubx` has no path to fill this today. `gps/lib/ubxcfgval`
  already defines message keys for `UBX_MON_RF` and `UBX_SEC_SIG`.
- `gps/lib/casbin` / `gps/internal/casic` do not currently parse `MON-SEC` or
  `MON-JSM`.
- `gps/lib/uncmsg` registers `JAMSTATUS` and `FREQJAMSTATUS` IDs, but there
  are no body structs or dispatch logic in `gps/internal/unc`.
- `gps/lib/novmsg` / `gps/internal/nov` do not currently parse
  `ITDETECTSTATUS` or `ANTIJAMTYPE`.
- `gps/lib/asbin` knows the Allystar `MON-CWI` message ID, but there is no
  parser or handler and the message does not naturally produce the proposed
  bool anyway.
- `gps/internal/nmea` has no standard message to populate this.

Unlike separate DOP or accuracy fields, this is not mostly model plumbing.
The `NavEpochMsg` field itself is easy, but most protocol families would need
new message parsers before the field can be populated.
