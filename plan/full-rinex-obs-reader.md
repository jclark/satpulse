# Full RINEX observation reader


This plan follows `plan/source-rinex-obs-reader.md`. The source-reader slice
keeps the current batch API sound by rejecting or skipping RINEX constructs
that it cannot represent. This plan is the larger work needed to read general
RINEX observation files while preserving the file's state changes and useful
metadata.

The key design point is that RINEX header records are state. Header values
remain valid until changed by another header record. Epoch flag `4` allows
header records to appear in the data section, which means observations before
and after the event may be decoded under different active state. This maps
well to the `.obsj` idea of a stream containing both metadata records and
observation records, but it does not map cleanly to the current
`ReadObservationFile` return shape of one `Metadata` value plus a flat
observation slice.

## Streaming reader

Add a lower-level RINEX observation reader that emits records in file order:

- metadata records from the initial header,
- observation records from epoch flags `0` and `1`,
- metadata updates from epoch flag `4`,
- event records or diagnostics for skipped event flags.

The reader should maintain active decoding state internally. Observation rows
must be decoded using the state in effect at that point in the file. The `.obsj`
writer can then preserve metadata changes by emitting metadata records before
the observations they affect.

Keep `ReadObservationFile` as a batch convenience API, but define it as a lossy
adapter over the streaming reader. It can return observations when the active
state can be applied internally without needing to expose state history. It
should reject files whose metadata changes cannot be represented faithfully by
one final or initial `Metadata` value.

## Mid-file header updates

Support epoch flag `4` by parsing the following header records as state
updates. Header records that affect later observation extraction include:

- `SYS / # / OBS TYPES`: changes the observation-column layout for a satellite
  system.
- `SYS / SCALE FACTOR`: changes the numeric scale of affected observation
  fields.
- `GLONASS SLOT / FRQ #`: changes GLONASS FDMA frequency-channel metadata.
- `LEAP SECONDS`, `TIME OF FIRST OBS`, and `TIME OF LAST OBS`: can affect
  time-system conversion for later epochs.
- `SIGNAL STRENGTH UNIT`: affects interpretation of `S` observations.

The tricky case is time conversion. A mid-file `LEAP SECONDS` update can mean
that different observations in one file are converted to GPST using different
offsets. The streaming reader can apply that state while emitting observations,
but the batch API cannot express that history in returned metadata.

## Observation modifiers

Implement observation modifiers that the source-reader slice rejects:

- `SYS / SCALE FACTOR`: divide stored observations by the active factor before
  populating `SignalObservation`.
- `SIGNAL STRENGTH UNIT`: only populate `CN0` when values are in dB-Hz or can be
  converted. Otherwise preserve enough metadata or reject rather than presenting
  values as dB-Hz.
- Optional RINEX 4.02 extra epoch second digits: preserve the precision encoded
  in the file.

Continue to ignore `SYS / PHASE SHIFT` and `GLONASS COD/PHS/BIS` for
observation extraction. RINEX 4.02 says phase-shift records are strongly
deprecated and should be ignored by decoders and encoders.

## Metadata coverage

Extend `Metadata` only where the data preserves real semantics, improves
diagnostics, or helps round-trip useful source information. Do not mirror every
fixed-column header line merely because it exists.

Useful additions beyond the core set defined in "Header/metadata design":

- File-level system and time-system declarations not yet captured.
- Receiver clock and signal handling: `RCV CLOCK OFFS APPL` and
  `SIGNAL STRENGTH UNIT`.
- Observation-modifying records that the reader applies, especially
  `SYS / SCALE FACTOR`.
- Correction provenance: `SYS / DCBS APPLIED` and `SYS / PCVS APPLIED`.
- Antenna and station details beyond the fields currently represented:
  antenna phase center, zero direction, bore-sight, center of mass, and other
  optional station records from the observation header.
- Deprecated compatibility records such as `SYS / PHASE SHIFT`, if preserving
  them helps compare against third-party RINEX producers. They should not be
  used to alter extracted observations for RINEX 4.02 decoding.

## Diagnostics

The full reader should have a way to report skipped or preserved events,
unknown header records, and unsupported future event flags without forcing
every caller to fail. This can be a warning sink, a structured event stream, or
metadata records in `.obsj`.

The important distinction is between incomplete metadata preservation and
incorrect observation extraction. Unknown records that cannot affect
observations may be reported and skipped. Unknown records that can change how
later observations should be decoded must be represented, applied, or rejected.
