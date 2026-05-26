# Sound RINEX observation reader

This plan covers the first, deliberately narrow slice of making the RINEX
observation reader trustworthy for source files not produced by `convobs`.

The objective is soundness, not completeness: if `ReadObservationFile`
succeeds, every returned `SignalObservation` must accurately correspond to an
observation measurement in the file, decoded using the RINEX semantics that the
reader understands. If a valid RINEX construct could change the meaning of
later observations and the reader cannot apply it, the reader must reject the
file rather than return plausible but wrong observations.

This is separate from the full-reader work in
`plan/full-rinex-obs-reader.md`, which adds a richer streaming model and more
metadata coverage.

## Current problem

The current reader is intentionally narrow. It reads the initial header, then
treats every data-section epoch record as an ordinary observation epoch. It
parses the epoch time and count, converts the time to GPST, and then consumes
that many following lines as satellite observation records.

That is correct only for epoch flags `0` and `1`. RINEX epoch flags `2`
through `5` are event records, and flag `6` contains cycle-slip records rather
than measurements. For event flags, the count field is the number of special
records following the epoch line, not the number of satellites. The RINEX spec
also allows event records without significant time to leave the epoch fields
blank, so the reader must inspect the flag and count without first requiring a
complete timestamp.

## Reader contract

The reader may remain incomplete, but it must be sound:

- Successful parses must not include event records, cycle-slip records, or
  unknown payload lines as `SignalObservation` values.
- Successful parses must not silently ignore observation-affecting header
  semantics that would make returned values wrong.
- Unsupported observation-affecting constructs should produce clear errors.
- Harmless records that cannot affect the returned observations may be skipped.

This contract is especially important because the batch API returns one
`Metadata` value and one flat observation slice. It has no warning channel and
no way to represent mid-file metadata changes.

## Epoch flags

Parse the epoch flag and record count explicitly. Use enough fixed-column
parsing to handle event records with blank epoch fields.

Handle flags as follows:

- `0`: ordinary observation epoch. Require a timestamp and parse the following
  records as satellite observations.
- `1`: observation epoch after power failure. Require a timestamp and parse the
  following records as satellite observations. Ignore the power-failure event
  semantics for now because they do not change the measurement values.
- `2`: start moving antenna. Skip the stated number of special records.
- `3`: new site occupation. Skip the stated number of special records.
- `4`: header information follows. Reject for this slice, because mid-file
  header updates can change the active decoding state for later observations.
- `5`: external event. Skip the stated number of special records.
- `6`: cycle-slip records follow. Skip the stated number of records; they use
  the observation row layout, but contain slips rather than measurements.
- Any other flag: reject.

For skipped records, report unexpected EOF distinctly from invalid observation
data. Skipping should not call `parseSatelliteObservationLine`.

## Observation-affecting header records

The initial header already contains state needed to decode observations:

- `SYS / # / OBS TYPES` defines the column layout for each satellite system.
- `GLONASS SLOT / FRQ #` supplies GLONASS FDMA frequency channels.
- `TIME OF FIRST OBS`, `TIME OF LAST OBS`, and `LEAP SECONDS` affect time-system
  conversion to GPST.

For this slice, continue to apply the initial header records already supported.
Add explicit rejection for initial header records that affect observation
values but are not yet applied:

- `SYS / SCALE FACTOR`: reject until scale-factor support is implemented. The
  spec says stored observations must be divided by this factor before use.
- `SIGNAL STRENGTH UNIT`: accept only the dB-Hz representation the reader maps
  to `CN0`; reject any other unit.

Continue to ignore records that the RINEX 4.02 spec says decoders should ignore
for observation extraction, such as `SYS / PHASE SHIFT`.

## Tests

Add focused reader tests:

- Flag `1` still produces observations.
- Flags `2`, `3`, and `5` skip their special records and continue with later
  observations.
- Skipped event records may have blank epoch fields.
- Flag `6` cycle-slip rows are skipped and do not produce observations.
- Flag `4` rejects with an unsupported mid-file header update error.
- Unknown flags reject.
- `SYS / SCALE FACTOR` in the initial header rejects until implemented.
