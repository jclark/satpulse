# RINEX observation conversion

The goal is to generate RINEX observation files from RTCM streams with MSM7
data or from UBX streams with UBX-RXM-RAWX data.

As part of this, define a modern JSONL format for RINEX observation conversion.
The format should be semantically close to RINEX observation data, but without
the fixed-column syntax and header-dependent record layout. It should be usable
as an intermediate format in conversion workflows.

The JSONL format uses `.obsj` files. Records are either `SignalObservation` or
`Metadata`. There is no envelope or discriminator: if a JSON object has a `t`
field, it is a `SignalObservation`; otherwise it is `Metadata`.

`SignalObservation` represents one satellite signal at one epoch. It carries
the RINEX-style satellite identifier, RINEX signal identifier, observation time,
and the observation values needed to produce RINEX C/L/D/S records. GLONASS
FDMA frequency channel is carried on the observation when known.

`Metadata` carries scraps of information that correspond to RINEX observation
header fields. Metadata records may appear anywhere in the file. A RINEX writer
will read the whole `.obsj` file, merge metadata with facts derived from the
observations, and then write the RINEX observation header and data records.

Generating RINEX output is not streamable because the header depends on the
whole set of observations and metadata. Generating `.obsj` should be streamable
from RTCM MSM7 and UBX-RXM-RAWX with only limited buffering.

The intermediate format definition lives in `gps/lib/rinex` so it can be reused
outside the SatPulse domain layer. It must not depend on `gpsprot`, but should
use compatible concepts for satellite identifiers, time handling, and
observation values.

The conversion code from RTCM streams and UBX streams may need domain-layer
packages. That code can live outside `gps/lib/rinex`; the reusable library
package should define the `.obsj` records and low-level formatting helpers, not
own the whole conversion pipeline.

## CLI tool

The command-line interface should be a generic observation converter, not a
UBX-specific tool. The working name is `convobs`, short for "convert
observations". This is intentionally similar to RTKLIB's `convbin`, but names
the thing being converted rather than the packet container. It is also short
enough to work as a future `satpulsetool convobs` subcommand.

The command shape should be:

```text
convobs [-r|--from raw|ubx|rtcm|uncb|unca|nova|novb|rinex|obsj]
        [--packet-log] [--to rinex|obsj] [-o path] [--metadata path]
        input...
```

At least one input positional argument is required. Each input is processed in
the order supplied. A literal `-` means stdin, and stdin is used only when that
literal input is present. Output defaults to stdout. An output path can be
specified with `-o`/`--output`. The output path is never taken from a
positional argument.

When multiple input files are supplied, they are treated as consecutive chunks
of the same observation stream. This supports the usual Unix pattern of
passing all input files positionally while keeping output redirection separate
from input selection.

The `--from` option selects the input observation format or packet protocol.
The short option `-r` is accepted as an alias for `--from`, following RTKLIB
`convbin` prior art. Most users should not need to specify `--from`: the
default is `raw`, which auto-detects the packet observation protocol from the
stream. RINEX and `.obsj` inputs must be selected explicitly with
`--from rinex` or `--from obsj`.

For packet input, `--from raw` means scan the packet stream until the first
supported raw observation message is found, then select the matching converter:

- `UBX-RXM-RAWX` selects the UBX converter.
- RTCM MSM7 messages select the RTCM converter.

Explicit packet-protocol formats, such as `ubx`, `rtcm`, `uncb`, `unca`,
`nova`, and `novb`, are case-insensitive and are for cases where the packet
protocol is known or the stream is ambiguous. When one of these formats is set,
only packets for the selected protocol should be considered for observation
conversion.

The `--packet-log` flag says that packet input is wrapped in a SatPulse JSONL
packet log instead of being a raw binary packet stream. It is valid only with
packet input formats: `raw`, `ubx`, `rtcm`, `uncb`, `unca`, `nova`, and
`novb`. It is not valid with `--from rinex` or `--from obsj`.

The `--to` option selects the output format. The supported output formats are
`rinex` and `obsj`. Output defaults to RINEX. Filenames do not imply formats:
use `--to obsj` to write JSONL observation records.

Examples:

```sh
satpulsetool convobs in.ubx > out.obs
satpulsetool convobs --to obsj -o out.obsj in.ubx
satpulsetool convobs --from ubx -o out.obs stream.bin
satpulsetool convobs --packet-log -o out.obs packets.jsonl
satpulsetool convobs --from obsj -o out.obs in.obsj
satpulsetool convobs --from rinex --to obsj -o out.obsj in.obs
```

Once a converter is selected, subsequent packets are fed to that converter.
The command should reject a later observation message from a different raw
observation family, because a single output file should have one coherent
time/receiver metadata model. Packets seen before converter selection may need
to be buffered or replayed so metadata packets that precede the first raw
observation message are not lost.

The current `ubx2rinex` command is temporary. It accepts exactly one positional
input, uses stdin only when that input is the literal `-`, and writes to stdout
unless `-o`/`--output` names an output file. It does not accept a positional
output path. When this command is moved into `satpulsetool`, it should be
renamed to the generic `convobs` subcommand even if only UBX input is
implemented at that point. Multiple positional inputs are a separate CLI
enhancement.

The command structure should stay close to the internal command packages:
parse flags into one command-local struct, open files in the command layer
with `defer`, and keep the conversion core expressed in terms of `io.Reader`,
`io.Writer`, metadata, writer options, and converter options.

RTKLIB's `convbin` uses `-r format` to select receiver/raw log input format,
for example `-r ubx`, `-r rtcm3`, `-r nov`, `-r unicore`, or `-r rinex`.
It also infers input format from file extension when `-r` is omitted.
`convobs` borrows `-r` as a short alias for `--from`, but intentionally does
not use filename inference for either input or output formats.

## Implementation plan

1. Done: define the core observation model in `gps/lib/rinex`.
   `SignalObservation` represents one satellite signal at one epoch, and
   `Metadata` represents RINEX-style header facts that can be merged with facts
   derived from observations.

2. Done: implement RINEX observation output in `gps/lib/rinex`. The RINEX
   writer/sink buffers observations, merges metadata, derives header facts such
   as observation code lists and time range, and writes valid RINEX observation
   files.

3. Done: implement UBX-RXM-RAWX conversion in `gps/lib/rinex/ubx`. The
   converter maps UBX satellites and signals to RINEX identifiers, derives LLI
   in the RTKLIB-compatible way, leaves SSI unset, and emits
   `SignalObservation` records through the RINEX sink interface.

4. Done: add the temporary `cmd/ubx2rinex` command. It uses the public packet
   scanner, accepts exactly one positional UBX input, uses stdin only for an
   explicit `-`, writes to stdout by default, and uses `-o`/`--output` for a
   named output file. It also accepts JSON metadata and command-line metadata
   flags.

5. Done: lock down current UBX behavior with golden tests. The test fixtures
   include long M8T and F9T UBX captures and compare generated RINEX output to
   checked-in golden files that are byte-identical to RTKLIB Explorer after
   normalizing generated header fields and ignoring RTKLIB's
   `SYS / PHASE SHIFT` header records.

6. Move the command into `satpulsetool` as `convobs`. The move should preserve
   the current UBX path but rename the user-facing command to the generic
   observation converter.

7. Implement `.obsj` output. Add a JSONL sink in
   `gps/lib/rinex` that writes `Metadata` and `SignalObservation` records as
   they arrive, so raw packet input can be converted streamably into the
   intermediate observation format. Wire this into `convobs` with
   `--to obsj`.

8. Implement `.obsj` input. Add a reader in `gps/lib/rinex`
    that reads JSONL records containing a mixture of `SignalObservation` and
    `Metadata`, merges metadata, and feeds the RINEX observation writer. Wire
    this into `convobs` with `--from obsj`.

9. Add `--packet-log` input mode. This mode reads SatPulse JSONL packet logs
   instead of raw binary packet streams and feeds the packet bytes through the
   same converter-selection path as raw input.

10. Implement RINEX observation input. Add a RINEX observation reader in
    `gps/lib/rinex` that reads RINEX observation files into the internal
    observation model and metadata records. Wire this into `convobs` with
    `--from rinex`, enabling RINEX to `.obsj` conversion.

11. Add explicit input selection to `convobs`. `--from raw` remains the packet
    auto-detection mode, while `--from ubx`, `--from rtcm`, and future
    packet-protocol formats such as `uncb`, `unca`, `nova`, and `novb` force a
    known packet protocol.

12. Add support for multiple positional inputs to `convobs`. Process the
    inputs in command-line order as consecutive chunks of one observation
    stream, with stdin used only for an explicit `-` input.

13. Support RTCM MSM7 input. Add an MSM7 converter that emits
    `SignalObservation` records from RTCM MSM7 messages and metadata records
    from relevant station, receiver, and antenna messages. It must assemble MSM
    fragments for the same epoch/reference station and map RTCM satellite and
    signal IDs to RINEX identifiers.

## Fixed-column format helper

Add a small reusable package, tentatively `gps/lib/fixcol`, for Fortran-style
fixed-column format strings. The goal is not a RINEX-specific parser helper; it
is a clean fixed-column text library that can also be used for RINEX-adjacent
GNSS formats such as CLK and ANTEX. RINEX observation files are the first client
because they give us concrete requirements and golden tests.

The public API should use format strings directly. Parsed operations may exist
internally and may be cached by format string, but callers should not have to
build or pass exported descriptor objects. The useful shape is:

```go
err := fixcol.Scan(line, "(A60,A20)", &content, &label)
line, err := fixcol.Format("(A20,A20,A20)", program, runBy, date)
```

The same format strings should be used for input and output. RINEX code should
define named constants for record layouts and use those constants for both
`Scan` and `Format`, rather than keeping separate read slices and write
`fmt.Sprintf` templates. For example:

```go
const pgmRunByDateFormat = "(A20,A20,A20,T61,'PGM / RUN BY / DATE')"

err := fixcol.Scan(line, pgmRunByDateFormat, &program, &runBy, &date)
line, err := fixcol.Format(pgmRunByDateFormat, program, runBy, date)
```

The package should be line-oriented. RINEX already describes each header or data
record separately, and the important records are easiest to read one line at a
time. A multi-record `/` operator can be deferred unless it becomes useful for
tests or navigation-message records.

The library should stay format-oriented rather than schema-oriented. It should
not know about header labels, observation codes, satellites, epochs, antenna
models, clock records, or station metadata. Higher-level packages should keep
small tables of record labels and format strings, then use `fixcol` for the
mechanical work of slicing, padding, parsing, and rendering fields. If a future
CLK or ANTEX reader needs the same fixed-column operations, it should be able to
use the same API without importing `gps/lib/rinex`.

The RINEX 4.02 specification shows that the minimum useful descriptor set is
small:

- `Aw` for fixed-width character fields. On input, leading and trailing blanks
  should be discarded, matching RINEX section 6.2. On output, strings should be
  left-justified in the field.
- `Iw` and `Iw.m` for integers. The `.m` form is needed for fields such as
  `I2.2` satellite numbers and `I5.5` extra epoch second digits.
- `Fw.d` for fixed-point floats. This covers header coordinates such as
  `3F14.4`, intervals such as `F10.3`, epoch seconds such as `F11.7`, and
  observation values such as `F14.3`.
- `Ew.d` for exponent floats, with `D` accepted as the same kind of numeric
  field on input. RINEX observation files do not need this much, but RINEX
  navigation files use records such as `4E19.12`.
- `nX` for skipped columns and blank output.
- `Tn` for absolute one-based column positioning. The spec usually expresses
  this as widths, but `T61,A20` is the natural way to say "header label in
  columns 61-80" when that is clearer than `A60,A20`.
- Quoted fixed text, such as `'PGM / RUN BY / DATE'`. On input this should
  verify that the field matches the literal, ignoring only omitted trailing
  blanks where the record ends early. On output it should write the literal.
  Fortran FORMAT supports character literals; `fixcol` only needs this simple
  quoted-string form, not old Hollerith syntax.
- Repeat counts before descriptors and groups, including `3F14.4`,
  `13(1X,A3)`, and `4(1X,I2.2)`. Dynamic record shapes should be handled by
  generating the format string from the record metadata, for example
  `fmt.Sprintf("A1,I2.2,%d(F14.3,I1,I1)", len(codes))`.

When scanning input, missing trailing columns should be treated as blanks. For
example, scanning a 70-character header line with `(A60,A20)` should read the
last 10 label characters as spaces, not fail because the physical line is
shorter than 80 characters. This matters because RINEX permits trailing blanks
to be omitted and variable-length observation records to stop after the last
non-empty field. Scanning into a plain numeric destination should reject a blank
field; scanning into `opt.Val[T]` should leave the value unset. Formatting an
unset `opt.Val[T]` should write blanks for the whole field. This matches the
RINEX "blank if not known" convention without making blank numeric fields
silently look like real zero values.

The first version should not try to be full Fortran formatted I/O. It does not
need scale factors, sign-control state, blank-control state, logical or complex
types, list-directed input, colon termination, unlimited-format items, or
general quoted literals. RINEX-specific rules such as epoch-second flooring,
observation code validation, GLONASS frequency semantics, continuation record
meaning, and overflow policy for phase observations should stay in
`gps/lib/rinex`. The test for adding a feature to `fixcol` should be whether it
is a general fixed-column formatting feature, not whether it makes one RINEX
case slightly shorter.

This should lead to a nicer RINEX implementation if it replaces the low-level
column mechanics without absorbing RINEX semantics. Specific cleanup targets:

- Header parsing can scan `(A60,A20)` once, dispatch on the label, and then scan
  the full line with the record format constant from Table A2, including the
  fixed label literal. This avoids scattered `line[:60]`, `line[60:80]`, and
  `strings.Fields` parsing while still checking that the label matches the
  selected record parser.
- Header writing can use the same fixed-column helper for the content and label
  fields, making records such as `PGM / RUN BY / DATE`,
  `APPROX POSITION XYZ`, `TIME OF FIRST OBS`, `SYS / # / OBS TYPES`, and
  `GLONASS SLOT / FRQ #` read like executable copies of the spec.
- Epoch parsing can use the Table A3 layout instead of `strings.Fields`, which
  is important for optional receiver clock offset fields and future
  pico-second epoch digits.
- Observation line parsing and writing can generate a format such as
  `A1,I2.2,%d(F14.3,I1,I1)` rather than hand-coded `3 + i*16` slicing and
  `%14.3f%c%c` formatting.
- Golden tests should remain byte-for-byte tests for RINEX output. Add focused
  `fixcol` tests for padding short input, blank optional numeric fields,
  repeated groups, zero-padded integers, `Tn`, and numeric overflow errors.

The package should live outside `gps/lib/rinex`, and should not import RINEX
types. It can support `gps/lib/opt` for optional fields, but the core API should
also work with ordinary Go strings and numbers so it remains useful for adjacent
formats. Good error messages are part of the design: parse and formatting
errors should identify the format item, field number, and input/output columns
where practical.

## UBX conversion

### UBX gnssId/sigId to RINEX signal mapping

Sources: u-blox X20-HPG-2.00 section 1.5.4 Table 4 (signal identifiers); RINEX
4.02 Tables 10-16 (observation codes).

Each row maps a `(gnssId, sigId)` pair from `UBX-RXM-RAWX` to a RINEX satellite
system letter and a two-character RINEX signal suffix. The system letter
combined with the RINEX-numbered SV gives the `SatelliteID`; the signal suffix
prefixed with the observation type (`C`, `L`, `D`, `S`) gives the
`ObservationCode`.

| gnssId | sigId | UBX signal name        | RINEX sys | RINEX sig | RINEX band   |
| -----: | ----: | ---------------------- | :-------: | :-------: | ------------ |
|      0 |     0 | GPS L1C/A              |     G     |    1C     | L1 1575.42   |
|      0 |     3 | GPS L2 CL              |     G     |    2L     | L2 1227.60   |
|      0 |     4 | GPS L2 CM              |     G     |    2S     | L2 1227.60   |
|      0 |     6 | GPS L5 I               |     G     |    5I     | L5 1176.45   |
|      0 |     7 | GPS L5 Q               |     G     |    5Q     | L5 1176.45   |
|      1 |     0 | SBAS L1C/A             |     S     |    1C     | L1 1575.42   |
|      2 |     0 | Galileo E1 C (pilot)   |     E     |    1C     | E1 1575.42   |
|      2 |     1 | Galileo E1 B (data)    |     E     |    1B     | E1 1575.42   |
|      2 |     3 | Galileo E5 aI          |     E     |    5I     | E5a 1176.45  |
|      2 |     4 | Galileo E5 aQ          |     E     |    5Q     | E5a 1176.45  |
|      2 |     5 | Galileo E5 bI          |     E     |    7I     | E5b 1207.140 |
|      2 |     6 | Galileo E5 bQ          |     E     |    7Q     | E5b 1207.140 |
|      2 |     8 | Galileo E6 B           |     E     |    6B     | E6 1278.75   |
|      2 |     9 | Galileo E6 C           |     E     |    6C     | E6 1278.75   |
|      2 |    10 | Galileo E6 A (PRS)     |     E     |    6A     | E6 1278.75   |
|      3 |     0 | BeiDou B1I D1          |     C     |    2I     | B1I 1561.098 |
|      3 |     1 | BeiDou B1I D2          |     C     |    2I     | B1I 1561.098 |
|      3 |     2 | BeiDou B2I D1          |     C     |    7I     | B2 1207.140  |
|      3 |     3 | BeiDou B2I D2          |     C     |    7I     | B2 1207.140  |
|      3 |     4 | BeiDou B3I D1          |     C     |    6I     | B3 1268.52   |
|      3 |    10 | BeiDou B3I D2          |     C     |    6I     | B3 1268.52   |
|      3 |     5 | BeiDou B1 Cp (pilot)   |     C     |    1P     | B1C 1575.42  |
|      3 |     6 | BeiDou B1 Cd (data)    |     C     |    1D     | B1C 1575.42  |
|      3 |     7 | BeiDou B2 ap (pilot)   |     C     |    5P     | B2a 1176.45  |
|      3 |     8 | BeiDou B2 ad (data)    |     C     |    5D     | B2a 1176.45  |
|      5 |     0 | QZSS L1C/A             |     J     |    1C     | L1 1575.42   |
|      5 |     1 | QZSS L1S               |     J     |    1Z     | L1 1575.42   |
|      5 |     4 | QZSS L2 CM             |     J     |    2S     | L2 1227.60   |
|      5 |     5 | QZSS L2 CL             |     J     |    2L     | L2 1227.60   |
|      5 |     8 | QZSS L5 I              |     J     |    5I     | L5 1176.45   |
|      5 |     9 | QZSS L5 Q              |     J     |    5Q     | L5 1176.45   |
|      5 |    12 | QZSS L1C/B             |     J     |    1E     | L1 1575.42   |
|      6 |     0 | GLONASS L1 OF (C/A)    |     R     |    1C     | G1 1602+k*9/16 |
|      6 |     2 | GLONASS L2 OF (C/A)    |     R     |    2C     | G2 1246+k*7/16 |
|      7 |     0 | NavIC L5 A (SPS)       |     I     |    5A     | L5 1176.45   |

Notes:

- BeiDou D1 and D2 are different navigation message variants on the same
  physical signal (B1I or B3I). RINEX has no separate code for the data
  variant, so `sigId` 0 and 1 both map to `2I` and `sigId` 4 and 10 both map
  to `6I`. The converter must treat the two variants as the same RINEX signal
  and merge observations accordingly.
- BeiDou B1I sits at 1561.098 MHz but is assigned RINEX band 2, not band 1.
  RINEX band 1 on BeiDou is reserved for the B1C signals (1575.42 MHz).
- For SBAS, the RINEX satellite letter is always `S`; the `svId` to RINEX
  PRN conversion follows the standard rules and is independent of the signal
  mapping.
- GLONASS observations need the FDMA frequency channel `k` (range -7..+6) on
  the `SignalObservation`. UBX `RXM-RAWX` carries it as `freqId`, with
  `k = freqId - 7`.
- The RINEX `1Z` code is used for both the legacy QZSS L1-SAIF signal and the
  updated L1S signal (per the RINEX 4.02 QZSS table note).
- UBX-RXM-RAWX only supplies `sigId` from protocol version 27 onward. Earlier
  protocol versions emit the subset marked "no explicit sigId" in u-blox
  Table 4; for the X20 (protocol >= 50) the field is always present and the
  full mapping above applies.
- gnssId values 4 (IMES) and any others not listed have no UBX-RXM-RAWX
  signals defined in the X20 spec and need not be mapped.

### UBX-RXM-RAWX LLI derivation

The RINEX loss-of-lock indicator (LLI) byte that follows each phase
observation is not provided directly by UBX-RXM-RAWX; it is derived from
per-measurement flags and from per-(sat, sig) state carried across
epochs. The rules below follow rtklib-ex's UBX decoder
(`src/rcv/ublox.c`, function `decode_rxmrawx`).

#### LLI bit 1: half-cycle ambiguity unresolved

Set when a phase value is present at this epoch and the receiver has not
resolved half-cycle ambiguity:

- Default: set when `trkStat.halfCycleValid == 0`.
- SBAS exception: the receiver does not expose `halfCycleValid` for SBAS,
  so the bit is set while `locktime <= 8000 ms` and cleared once
  `locktime > 8000 ms`.

#### LLI bit 0: cycle slip

A slip is detected for this (sat, sig) whenever any of the following is
true:

1. `locktime == 0` (no continuous tracking).
2. `locktime` decreased since the previous epoch on this signal.
3. `trkStat.halfCycleSubtracted` differs from the converter's stored value.
   The stored value starts as false, so the first observation of a signal with
   `halfCycleSubtracted` set is treated as a change.
4. `cpStdev` (the carrier-phase σ index, bits 3..0) is at or above the
   slip threshold. The default threshold is 15, which is u-blox's
   "max quantization noise / invalid" sentinel; the threshold should be
   configurable.

When a slip is detected, the converter remembers it. The slip bit is
stamped on the next epoch that actually reports a phase value for this
(sat, sig); the pending-slip flag is then cleared. This guarantees that
slips are never lost across gaps in phase reporting.

If the slip is caused by a `halfCycleSubtracted` change, the converter also
returns an immediate LLI bit 0 for the current observation. If the current
observation has no phase value but still has another value, such as
pseudorange, this can produce a RINEX phase field with no phase value and only
the LLI indicator. The pending-slip flag still remains set until a later phase
value is emitted.

#### Converter state

To compute these flags the converter keeps, per (sat, sig):

- The previous epoch's `locktime`.
- The stored `halfCycleSubtracted` bit, initialized to false.
- A pending-slip flag carried forward across phase-absent epochs.
- A `seen` flag used only for the locktime-decrease check.

### SSI

RINEX C/L/D/S records reserve a one-character SSI (signal-strength
indicator) digit immediately after each LLI byte. RTKLIB leaves this
column blank in its RINEX output, relying on the separate `S<band><attr>`
observation type to carry signal strength as dBHz. We follow RTKLIB's
convention: the converter does not populate `SSI` on `SignalObservation`.
Signal strength is carried in `CN0`, which the RINEX writer emits as the
`S<band><attr>` observation.

## RTCM conversion

### RTCM MSM signal ID to RINEX signal mapping

Each row maps a constellation and MSM signal ID from the MSM signal mask
(DF395) to a two-character RINEX signal suffix. The suffix prefixed with the
observation type (`C`, `L`, `D`, `S`) gives the RINEX observation code.

| Constellation | RTCM sig id | RINEX sig id |
| ------------- | ----------: | :----------: |
| GPS           |           2 |      1C      |
| GPS           |           3 |      1P      |
| GPS           |           4 |      1W      |
| GPS           |           8 |      2C      |
| GPS           |           9 |      2P      |
| GPS           |          10 |      2W      |
| GPS           |          15 |      2S      |
| GPS           |          16 |      2L      |
| GPS           |          17 |      2X      |
| GPS           |          22 |      5I      |
| GPS           |          23 |      5Q      |
| GPS           |          24 |      5X      |
| GPS           |          30 |      1S      |
| GPS           |          31 |      1L      |
| GPS           |          32 |      1X      |
| GLONASS       |           2 |      1C      |
| GLONASS       |           3 |      1P      |
| GLONASS       |           8 |      2C      |
| GLONASS       |           9 |      2P      |
| Galileo       |           2 |      1C      |
| Galileo       |           3 |      1A      |
| Galileo       |           4 |      1B      |
| Galileo       |           5 |      1X      |
| Galileo       |           6 |      1Z      |
| Galileo       |           8 |      6C      |
| Galileo       |           9 |      6A      |
| Galileo       |          10 |      6B      |
| Galileo       |          11 |      6X      |
| Galileo       |          12 |      6Z      |
| Galileo       |          14 |      7I      |
| Galileo       |          15 |      7Q      |
| Galileo       |          16 |      7X      |
| Galileo       |          18 |      8I      |
| Galileo       |          19 |      8Q      |
| Galileo       |          20 |      8X      |
| Galileo       |          22 |      5I      |
| Galileo       |          23 |      5Q      |
| Galileo       |          24 |      5X      |
| SBAS          |           2 |      1C      |
| SBAS          |          22 |      5I      |
| SBAS          |          23 |      5Q      |
| SBAS          |          24 |      5X      |
| QZSS          |           2 |      1C      |
| QZSS          |           9 |      6S      |
| QZSS          |          10 |      6L      |
| QZSS          |          11 |      6X      |
| QZSS          |          15 |      2S      |
| QZSS          |          16 |      2L      |
| QZSS          |          17 |      2X      |
| QZSS          |          22 |      5I      |
| QZSS          |          23 |      5Q      |
| QZSS          |          24 |      5X      |
| QZSS          |          30 |      1S      |
| QZSS          |          31 |      1L      |
| QZSS          |          32 |      1X      |
| BeiDou        |           2 |      2I      |
| BeiDou        |           3 |      2Q      |
| BeiDou        |           4 |      2X      |
| BeiDou        |           8 |      6I      |
| BeiDou        |           9 |      6Q      |
| BeiDou        |          10 |      6X      |
| BeiDou        |          14 |      7I      |
| BeiDou        |          15 |      7Q      |
| BeiDou        |          16 |      7X      |
| BeiDou        |          22 |      5D      |
| BeiDou        |          23 |      5P      |
| BeiDou        |          24 |      5X      |
| BeiDou        |          25 |      7D      |
| BeiDou        |          30 |      1D      |
| BeiDou        |          31 |      1P      |
| BeiDou        |          32 |      1X      |
| NavIC         |           8 |      9A      |
| NavIC         |          22 |      5A      |

The MSM converter should use this table only for the signal suffix. Satellite
system letters still come from the RTCM message type (`1077` GPS, `1087`
GLONASS, `1097` Galileo, `1107` SBAS, `1117` QZSS, `1127` BeiDou, `1137`
NavIC).
