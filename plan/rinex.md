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
Observation time is always represented in GPS time (GPST), even for GLONASS
and for observations read from single-constellation RINEX files that use a
different RINEX file time system. This keeps `.obsj` suitable for mixed-GNSS
and PPP workflows. RINEX file time systems are handled only at the RINEX
reader boundary. The RINEX writer deliberately writes `GPS` as the file time
system for every observation file, including single-constellation files. RINEX
defaults pure GLONASS, BDS, and other single-system files to their native
system time only when the time-system identifier is omitted; explicit `GPS`
keeps the file aligned with the `.obsj` time model.

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

6. Done: move ubx2rinex command into `satpulsetool` as `convobs`. It is now
   `internal/convobscmd`.

7. Done: implement `.obsj` output. Add a JSONL sink in
   `gps/lib/rinex` that writes `Metadata` and `SignalObservation` records as
   they arrive, so raw packet input can be converted streamably into the
   intermediate observation format. Wire this into `convobs` with
   `--to obsj`.

8. Done: implement `.obsj` input. Add a reader in `gps/lib/rinex`
    that reads JSONL records containing a mixture of `SignalObservation` and
    `Metadata`, merges metadata, and feeds the RINEX observation writer. Wire
    this into `convobs` with `--from obsj`.

9. Done: add `--packet-log` input mode. This mode reads SatPulse JSONL packet logs
   instead of raw binary packet streams and feeds the packet bytes through the
   same converter-selection path as raw input.

10. Done: implement RINEX observation input. Add a RINEX observation reader in
    `gps/lib/rinex` that reads RINEX observation files into the internal
    observation model and metadata records. Wire this into `convobs` with
    `--from rinex`, enabling RINEX to `.obsj` conversion.

11. Done: add support for multiple positional inputs to `convobs`. Process the
    inputs in command-line order as consecutive chunks of one observation
    stream, with stdin used only for an explicit `-` input.

12. Done: add observation decimation. Implement a `rinex.Sink` wrapper that
    emits only epochs on a GPS-time-aligned interval grid. Restrict intervals
    to at least one second and to values that divide one GPS day exactly. For
    grid matching, round the epoch label to the nearest 0.1 second, then test
    the rounded time against the interval grid; emitted observations keep
    their original epoch label. Skipped observations are dropped, but their LLI
    bits are ORed into the next emitted observation for the same satellite and
    signal. Wire this into `convobs` so raw, `.obsj`, and RINEX inputs can all
    be decimated before writing RINEX or `.obsj`.

13. Done: add explicit input selection to `convobs`. `--from raw` remains the
    packet auto-detection mode, while `--from ubx`, `--from rtcm`, and future
    packet-protocol formats such as `uncb`, `unca`, `nova`, and `novb` force a
    known packet protocol.

14. Done: support RTCM MSM7 input, split into these implementation substeps:
    a) Done: update `gps/lib/rtcmbin` with RTCM-to-RINEX satellite-system,
    satellite-number, and signal mapping helpers, with focused tests covering
    valid, reserved, and unknown RTCM IDs.
    b) Done: add a new `gps/lib/rinex/rtcm` package that converts parsed RTCM
    MSM7 messages into `SignalObservation` records and relevant station,
    receiver, and antenna messages into metadata records. It converts each MSM7
    message independently, without buffering MSM fragments, and uses the
    `rtcmbin` mapping helpers for RINEX identifiers.
    c) Done: wire RTCM input into `convobs`, including raw and packet-log RTCM
    selection, week-inference options and warnings, passing constraints to
    `rinex/rtcm`, and routing emitted observations and metadata to RINEX or
    `.obsj` output.

15. Write a `satpulsetool-convobs.1` man page following the existing
    `docs/man/satpulsetool-*.1.md` pattern. Document the command synopsis,
    input and output formats, multiple input behavior, stdin handling,
    packet-log mode, metadata options, and examples for raw, packet-log,
    `.obsj`, and RINEX conversions.

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

### Week inference

RTCM MSM messages do not carry a complete date. Their epoch field identifies
the time within a constellation-specific repeat cycle. GPS, Galileo, QZSS,
SBAS, NavIC, and BeiDou MSM epochs repeat weekly because they carry a
time-of-week in milliseconds. BeiDou carries BDT time-of-week, which must be
converted to GPST before emitting a `SignalObservation`. GLONASS MSM epochs
also repeat weekly because they carry a day-of-week plus a time-of-day in the
GLONASS time scale; these epochs must be converted to GPST using GPS-UTC leap
seconds before emitting a `SignalObservation`.
For normal GLONASS MSM, the day within the week is present and the missing
component is still the absolute week. If the GLONASS day-of-week value is 7,
the day is unknown; resolution must then come from existing stream context or
from an interval that yields exactly one possible absolute epoch. RINEX
observations need a complete epoch for each `SignalObservation`, so the
converter must infer the missing absolute week before it can write
observations.

The implementation should not silently use the system clock as the RTCM
epoch date. That is convenient for live streams, but it can create a RINEX
file with the wrong week when converting old RTCM files. Instead, `convobs`
provides an optional absolute time interval. The interval is not GNSS-specific.
It says that the epoch represented by the current RTCM message must fall within
this absolute range. Supplied intervals must be finite, half-open, and short
enough that the RTCM epoch field cannot resolve to more than one absolute
epoch. For ordinary MSM messages this means the interval length must be no
greater than one week. Resolution succeeds only if exactly one epoch matches.

Represent the interval as a start time plus a duration, with half-open
semantics:

```go
type TimeInterval struct {
	Start    time.Time
	Duration time.Duration
}
```

`convobs` should keep the interval together with the user-facing reporting
information in a value called `WeekConstraint`:

```go
type WeekConstraint struct {
	Interval TimeInterval
	Errf     string
	WarnMsg  string
	WarnArgs []any
}
```

`Errf` is a format string containing `%w`; it is used to wrap an error returned
by `rinex/rtcm`. `WarnMsg` and `WarnArgs` are passed to `slog.Warn` after
successful conversion if the warning has not already been emitted for that
file. An empty `WarnMsg` means no warning.

There are three stages:

1. `convobs` constructs a file-level or packet-level `WeekConstraint`.
   In packet-log mode, each RTCM message gets a tight interval derived from
   the packet-log timestamp, using a generous one-minute slack on either side
   of the timestamp. In direct file mode, `convobs` constructs one interval for
   the file and supplies it with the first RTCM message in that file. The RTCM
   converter stores that interval before dispatch, so metadata or other
   non-MSM7 messages can carry the file interval for the first later MSM
   message in each GNSS. Later messages normally omit the interval and rely on
   the RTCM converter's continuity state.
2. The per-message caller passes the RTCM message to `rinex/rtcm` together
   with only the optional `TimeInterval`. The `WeekConstraint` itself stays in
   `convobs`, at the layer that calls the RINEX converter. In the current
   `convobs` call graph, that layer is `convertPacket`.
3. `rinex/rtcm` interprets the RTCM epoch field using RTCM semantics. If an
   interval is supplied, it must find exactly one complete epoch consistent
   with that interval. If no interval is supplied, it uses previously
   established stream time context and handles week rollover as needed. If the
   converter has no usable time context and no interval is supplied, conversion
   fails.

The RTCM converter owns the state needed for rollover and for conversion to
GPST. It may maintain separate week or day context for each GNSS internally,
because constellation time scales and week starts are not all identical. That
detail is hidden from `convobs`: a supplied interval resolves the current
message and updates the per-GNSS state. When no interval is supplied, the
converter uses existing per-GNSS continuity state. This also means that
`convobs` does not need to know which constellations appear in the RTCM stream.
The rollover policy must distinguish plausible rollover from out-of-order
packets, duplicated messages, and file splices; not every backward movement in
time-of-week or GLONASS day-of-week/time-of-day should be treated as rollover.
`rinex/rtcm` should panic on interval contract violations such as a zero start,
an empty duration, or a duration greater than one week, even though `convobs`
is expected to validate intervals before passing them down. Data-dependent
resolution failures, such as no epoch matching the supplied interval or more
than one matching epoch, are returned as errors. A supplied interval
deliberately overrides existing continuity for the current message.

For direct file input, `convobs` builds the file-level `WeekConstraint` by
intersecting all applicable constraints. It should capture `now` once per file
or conversion operation and use that same value for all constraints and
diagnostics, so boundary behavior is deterministic.

- Packet-log mode uses packet-log timestamps for week inference. Explicit week
  inference options such as `--date`, `--recent`, and `--date-from-filename`
  should be rejected in packet-log mode.
- An explicit `--recent` option means observations are within the last week,
  measured from the captured `now`. It produces the interval
  `[now - 1 week, now)`, with no warning.
- An explicit `--date YYYYMMDD` option means the observations are on that
  civil date. Because the user's time zone may be unknown, this produces an
  interval from 14 hours before `YYYY-MM-DDT00:00:00Z` to 36 hours after it.
  This explicit interval is not intersected with a `now`-based constraint. The
  resulting interval must still be unambiguous for the first RTCM message that
  needs epoch resolution. `--date`, `--recent`, and `--date-from-filename` are
  mutually exclusive.
- An explicit `--date-from-filename` option, with short name `-f`, means
  `convobs` must parse a date from the input filename and construct the same
  civil-date interval as `--date`. A RINEX long filename date such as
  `YYYYDDDHHMM` contributes its calendar date; the time-of-day part is not
  needed for week inference. The option applies per input file. If a file has
  no recognized date, or has multiple conflicting recognized dates, conversion
  of that file must fail. The option is invalid for stdin, because there is no
  filename to parse.
- If there is no explicit `--date`, `--recent`, or `--date-from-filename`,
  `convobs` may use an automatic recent assumption for convenience. This
  produces the same interval as `--recent`, but the `WeekConstraint` includes
  a warning explaining that an undated file is being assumed to contain
  observations from the last week.
- File modification time is not enough to date an RTCM file by itself. When
  the automatic recent assumption is being considered, mtime is only a gate.
  If mtime is before `now - 1 week`, and there is no explicit `--date`,
  `--date-from-filename`, or `--recent`, `convobs` must fail and require an
  explicit option for week inference. Otherwise mtime does not affect the
  interval.
- The automatic recent interval is bounded by `now`, since observations should
  not be inferred to be in the future.
- In direct file mode, stdin has no mtime and no filename. If no explicit week
  inference option is provided, `convobs` should use the automatic recent
  interval and warn after the first accepted RTCM message.

If intersecting the applicable constraints produces an empty interval, `convobs`
must fail before conversion and require an explicit argument such as `--date`,
`--recent`, or `--date-from-filename`. Conflicting explicit options should
also fail during option processing, before any RTCM messages are read.

Warnings are controlled by `convobs`. If a direct file conversion succeeds
with a non-empty `WarnMsg` in its `WeekConstraint`, `convobs` should print one
warning for that file after the first RTCM message has been accepted. This
warns about the only dangerous convenience behavior: assuming that an undated
file contains observations from the last week. Packet-log timestamps, `--date`,
`--recent`, and `--date-from-filename` do not need this warning. Errors
returned by `rinex/rtcm`, such as no complete epoch matching the supplied
interval or an ambiguous interval, are fatal conversion errors. `convobs`
should combine them with the per-message error format string, for example with
a message that explains which file-level time assumption was being used and
which explicit option the user can provide instead.

### MSM7 observation construction

The RTCM observation converter handles MSM7 observation messages only.
MSM1-6 messages are not used to emit `SignalObservation` records. Metadata
messages such as 1005, 1006, 1007, 1008, 1013, 1033, and 1230 may still be
consumed when present.

Each MSM7 message is converted independently. Do not buffer MSM fragments. The
MSM multiple message bit (DF393) may be used for cheap diagnostics, but the
converter is not an RTCM stream validator and should not require a complete
fragment group before emitting observations. In MSM7, the satellite-level rough
range/range-rate fields and the signal-level fine observation fields needed for
one satellite/signal observation are all present in the same message.

For each MSM7 message, expand the satellite mask and signal mask to increasing
1-based IDs. Walk the cell mask in satellite-major order: for each satellite ID
from the satellite mask, visit each signal ID from the signal mask. Each set
cell consumes the next element of every signal-data slice. Each set cell may
produce at most one `SignalObservation` for that `(time, satellite, signal)`.
Unknown or reserved satellite and signal IDs are skipped. Invalid or unavailable
RTCM fields leave the corresponding optional observation value unset. Malformed
input must not crash conversion; skip the affected cell or field and continue
with the rest of the stream.

Emit a `SignalObservation` only when it has a valid time, satellite ID, signal
ID, and at least one of `PR`, `CP`, `Do`, or `CN0` set. Do not emit an
observation solely for GLONASS frequency channel, LLI, SSI, or other
non-observable state. This follows the RINEX observation-record model: records
may contain blank observation fields, and the header declares the observation
types that actually occur in the file.

The MSM7 numeric reconstruction rules are:

- Use `c = 299792458` meters/second.
- `PR` is the MSM high-resolution pseudorange in meters:
  `c/1000 * (DF397 + DF398/1024 + DF405*2^-29)`. Leave unset if DF397 is the
  invalid value 255 or DF405 is the invalid value `-524288`.
- `CP` is the MSM high-resolution phase range converted from meters to carrier
  cycles: `phaseRangeMeters / wavelength`. The phase range in meters is
  `c/1000 * (DF397 + DF398/1024 + DF406*2^-31)`. Leave unset if DF397 is 255,
  DF406 is the invalid value `-8388608`, or the wavelength is not known.
- `Do` is the MSM phase range rate converted to RINEX Doppler in Hz:
  `-(DF399 + DF404*0.0001) / wavelength`. Leave unset if DF399 is the invalid
  value `-8192`, DF404 is the invalid value `-16384`, or the wavelength is not
  known.
- `CN0` is DF408 in dB-Hz, using the high-resolution scale
  `DF408 * 0.0625`. Leave unset when DF408 is zero.
- `Frq` is set only for GLONASS MSM7 when DF419 is in the range 0..13, using
  `k = DF419 - 7`. Leave it unset when DF419 is 15, meaning unknown or not
  applicable. Treat DF419 value 14 as reserved and do not use it for wavelength
  conversion.

Carrier wavelength is `c / frequency`. Determine frequency from the RINEX
satellite system and two-character signal identifier:

| System | RINEX band | Frequency MHz |
| ------ | ---------- | ------------: |
| GPS    | 1          | 1575.420      |
| GPS    | 2          | 1227.600      |
| GPS    | 5          | 1176.450      |
| GLONASS | 1         | `1602.000 + k*0.5625` |
| GLONASS | 2         | `1246.000 + k*0.4375` |
| Galileo | 1         | 1575.420      |
| Galileo | 5         | 1176.450      |
| Galileo | 6         | 1278.750      |
| Galileo | 7         | 1207.140      |
| Galileo | 8         | 1191.795      |
| SBAS   | 1          | 1575.420      |
| SBAS   | 5          | 1176.450      |
| QZSS   | 1          | 1575.420      |
| QZSS   | 2          | 1227.600      |
| QZSS   | 5          | 1176.450      |
| QZSS   | 6          | 1278.750      |
| BeiDou | 1          | 1575.420      |
| BeiDou | 2          | 1561.098      |
| BeiDou | 5          | 1176.450      |
| BeiDou | 6          | 1268.520      |
| BeiDou | 7          | 1207.140      |
| NavIC  | 5          | 1176.450      |
| NavIC  | 9          | 2492.028      |

For GLONASS FDMA signals, code and signal-strength observations may still be
emitted when `Frq` is unknown, but carrier phase and Doppler must be left unset
because wavelength is unknown. Do not apply any additional phase-shift
correction in the RTCM converter; MSM phase ranges are assumed to have the
alignment required for RINEX.

The RINEX LLI field is derived independently for each mapped satellite signal.
The converter keeps lock state keyed by the RINEX satellite ID and mapped RINEX
signal ID, matching the key used to merge observations for one signal. If two
input signal IDs map to the same RINEX signal, they share the same LLI state.

For each mapped MSM7 cell, use the raw MSM7 lock-time indicator as the lock
state value. Initialize the previous lock value for a signal to zero. Set
`LLILostLock` on the current observation when the current lock value is less
than the previous lock value, or when both the current and previous lock values
are zero. Then store the current lock value as the previous value for the next
cell for that signal. This means a first observation with a nonzero lock value
does not get `LLILostLock`, while a first observation with a zero lock value
does.

The MSM7 half-cycle ambiguity indicator maps one-to-one to
`LLIHalfCycleAmbiguity`: set the bit when the RTCM half-cycle bit is set, and
leave it clear when the RTCM half-cycle bit is clear. Do not invert it, debounce
it, or carry it forward from earlier cells. MSM7 does not set
`LLIBOCTracking`.

The converter may update LLI lock state for any valid mapped MSM7 cell, even if
that cell does not emit a `SignalObservation` because all observable values are
unavailable. When an observation is emitted, set `LLI` only if the derived LLI
value is nonzero. Do not emit an observation solely for LLI.

### RTCM RINEX mapping helpers

The RTCM-to-RINEX satellite-system, satellite-number, and signal mappings
belong in `gps/lib/rtcmbin`, not in the higher-level `gps/lib/rinex/rtcm`
converter. Add a `gps/lib/rtcmbin/rinex.go` file similar to
`gps/lib/ubxbin/rinex.go`, with exported helper functions such as:

- `RINEXSys(gnss GNSS) string`, returning `G`, `R`, `E`, `S`, `J`, `C`, or
  `I`, and `""` for unknown constellations.
- `RINEXSatNum(gnss GNSS, satID uint8) uint8`, applying the DF394 satellite
  ID table below and returning 0 for reserved or unmapped IDs.
- `RINEXSig(gnss GNSS, sigID uint8) string`, applying the DF395 signal table
  below and returning `""` for reserved or unmapped signal IDs.

The RINEX converter should call these helpers rather than duplicating RTCM
mapping tables. Add focused `rtcmbin` tests for all valid rows plus representative
reserved and unknown values.

### RTCM MSM satellite ID to RINEX satellite mapping

DF394 is always decoded the same way: the first encoded bit is RTCM satellite
ID 1, the second encoded bit is RTCM satellite ID 2, and so on through RTCM
satellite ID 64. Convert that RTCM satellite ID to a RINEX satellite ID using
the following table. IDs in reserved ranges are skipped.

| RTCM message | Constellation | RTCM satellite ID | GNSS identifier | RINEX satellite ID |
| ------------ | ------------- | ----------------- | --------------- | ------------------ |
| 1077         | GPS           | 1..63             | PRN = ID        | `G%02d`, using ID |
| 1077         | GPS           | 64                | reserved        | skip |
| 1087         | GLONASS       | 1..24             | slot = ID       | `R%02d`, using ID |
| 1087         | GLONASS       | 25..64            | reserved        | skip |
| 1097         | Galileo       | 1..50             | PRN = ID        | `E%02d`, using ID |
| 1097         | Galileo       | 51                | GIOVE-A         | skip |
| 1097         | Galileo       | 52                | GIOVE-B         | skip |
| 1097         | Galileo       | 53..64            | reserved        | skip |
| 1107         | SBAS          | 1..39             | PRN = ID + 119  | `S%02d`, using ID + 19 |
| 1107         | SBAS          | 40..64            | reserved        | skip |
| 1117         | QZSS          | 1..10             | PRN = ID + 192  | `J%02d`, using ID |
| 1117         | QZSS          | 11..64            | reserved        | skip |
| 1127         | BeiDou        | 1..63             | PRN = ID        | `C%02d`, using ID |
| 1127         | BeiDou        | 64                | reserved        | skip |
| 1137         | NavIC         | 1..14             | PRN = ID        | `I%02d`, using ID |
| 1137         | NavIC         | 15..64            | reserved        | skip |

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

## Unicore conversion

Initial Unicore support should convert `OBSVMB` messages to `.obsj` and RINEX
observation output. This is the message enabled by SatPulse's existing
`--raw-out obs` Unicore configuration path, which sends `OBSVMB 1`.

Do not include the other Unicore observation messages in the first pass:

- `OBSVMA` is the ASCII form of the same uncompressed master-antenna
  observation data. It can be added after the binary path is working.
- `OBSVBASEB` and `OBSVHB` use the same uncompressed observation-record layout,
  but are base-station and slave-antenna logs rather than the normal receiver
  observation stream.
- `OBSVMCMPB` and `OBSVHCMPB` are compressed 24-byte-per-observation forms.
  They should be treated as a separate follow-up because they require bitfield
  unpacking.
- Raw navigation messages such as `GPSEPHB`, `BDSEPHB`, `GLOEPHB`, `GALEPHB`,
  and `QZSSEPHB` are navigation-file work, not observation conversion.

### Unicore message parsing

Add an `ObsVM` message type to `gps/lib/uncmsg` and register it for message ID
`12`, with ASCII name `OBSVMA` and binary name `OBSVM` through the existing
registration conventions. The binary packet parser already parses the 24-byte
Unicore binary header and dispatches by message ID; what is missing is the
typed payload.

The `OBSVMB` payload starts immediately after the Unicore binary header:

| Offset | Field | Type | Meaning |
| ------:| ----- | ---- | ------- |
| `H+0`  | obs number | `uint32` | Number of observation records |
| `H+4`  | observations | `[]ObsVMRecord` | `obs number` fixed 40-byte records |

Each `ObsVMRecord` has this layout:

| Record offset | Field | Type | Meaning |
| -------------:| ----- | ---- | ------- |
| `+0`  | system frequency | `uint16` | GLONASS frequency channel plus 7; zero for other systems |
| `+2`  | PRN/slot | `uint16` | Unicore satellite PRN/slot number |
| `+4`  | pseudorange | `float64` | Meters |
| `+12` | ADR | `float64` | Carrier phase / accumulated Doppler range, cycles |
| `+20` | pseudorange std | `uint16` | Standard deviation times 100 |
| `+22` | ADR std | `uint16` | Standard deviation times 10000 |
| `+24` | Doppler | `float32` | Hz |
| `+28` | C/N0 | `uint16` | dB-Hz times 100 |
| `+30` | reserved | `uint16` | Reserved |
| `+32` | lock time | `float32` | Continuous tracking time without cycle slip, seconds |
| `+36` | tracking status | `uint32` | Validity, system, and signal fields |

Add focused `uncmsg` tests using the existing UM980 `OBSVM` packet in
`internal/gpscmd/testdata/unicore/um980-raw-out.jsonl`. The tests should lock
down message ID dispatch, record count, selected record values, and
round-tripping through binary serialization.

### Unicore observation converter

Add a `gps/lib/rinex/unc` package, parallel to `gps/lib/rinex/ubx` and
`gps/lib/rinex/rtcm`. The converter should accept parsed Unicore messages plus
their header and emit `rinex.SignalObservation` records to a `rinex.Sink`.

Observation time comes from the Unicore message header. Convert the header's
GPS week and millisecond-of-week to `rinex.Time`:

```go
rinex.TimeFromGPSWeekSeconds(int64(h.Week), float64(h.MillisecondsOfWeek)/1000)
```

Only emit observations whose tracking status maps to a known RINEX satellite
and signal. Use the pseudorange and carrier-phase validity bits to decide
whether `PR` and `CP` are set. Leave a field unset when the corresponding
source value is invalid or unavailable. Do not emit an observation with no
RINEX observation values.

Tracking status fields documented by the Unicore protocol are:

| Bits | Meaning |
| ---: | ------- |
| 10 | Carrier phase valid |
| 12 | Pseudorange valid |
| 16..18 | Satellite system: GPS, GLONASS, SBAS, Galileo, BDS, QZSS, IRNSS |
| 21..25 | Signal type, interpreted per satellite system |
| 26 | L2C flag, disambiguating GPS/QZSS L2P(Y) from L2C |

Map C/N0 by dividing the source value by 100. Map Doppler directly in Hz. Map
carrier phase with the RTKLIB-compatible sign convention used for Unicore:
the RINEX carrier phase value is `-ADR`.

For GLONASS, convert the Unicore PRN/slot range to RINEX slot numbering by
subtracting 37. The RINEX `Frq` value should be the GLONASS frequency channel,
so derive it as `int8(systemFrequency) - 7` when the source value is known.

### Unicore RINEX mappings

Put Unicore-to-RINEX mapping helpers in `gps/lib/uncmsg`, not in the
higher-level RINEX converter. This mirrors `ubxbin` and `rtcmbin` and keeps the
wire-protocol mapping close to the protocol package.

Use `gpsprot.SignalID` as the midpoint. `gps/internal/unc` already maps
Unicore system/frequency IDs to `gpsprot.SignalID`, and UBX already has both
UBX-to-`gpsprot.SignalID` and UBX-to-RINEX mappings. The Unicore mapping table
should therefore be built from:

- Unicore V1.13 tracking-status system and signal-type IDs.
- Existing `gps/internal/unc` mappings to `gpsprot.SignalID`.
- Existing UBX RINEX mappings where the same `gpsprot.SignalID` is already
  represented.
- RINEX 4.02 observation-code tables for signals UBX does not expose.

The helpers should cover:

- `TrackingStatus.SysID() SysID`, extracting the tracking-status system value
  for use by the RINEX helpers.
- `TrackingStatus.SignalType() FreqID` and `TrackingStatus.L2C() bool`,
  extracting the tracking-status signal fields used by `RINEXObsSig`.
  The Unicore V1.13 Table 7-178 Channel Tracking Status calls bits 21..25
  `Signal type`; the values reuse the package's `FreqID` type.
- `RINEXSys(system SysID) string`, mapping tracking-status system values to
  `G`, `R`, `S`, `E`, `C`, `J`, and `I`.
- `RINEXSatNum(system SysID, prnSlot uint16) uint8`, applying Unicore PRN/slot
  numbering and returning 0 for reserved or unsupported IDs.
- `RINEXObsSig(system SysID, sigType FreqID, l2c bool) string`, applying
  the OBSVM Channel Tracking Status `Signal type` field. The name and
  documentation should make clear that this maps OBSVM tracking-status signal
  types, not Unicore frequency identifiers in general. The caller gets these
  values from `TrackingStatus.SysID`, `TrackingStatus.SignalType`, and
  `TrackingStatus.L2C`.

Initial `RINEXObsSig` table for OBSVM Channel Tracking Status:

| System | Signal type | `l2c` | Unicore signal | `gpsprot.SignalID` | RINEX signal |
| ------ | ----------- | ----- | -------------- | ------------------ | ------------ |
| GPS | 0 | - | L1 C/A | `SigIDGPSL1CA` | `1C` |
| GPS | 9 | false | L2P(Y) | `SigIDGPSL2P` | `2W` |
| GPS | 9 | true | L2C(M) | `SigIDGPSL2CM` | `2S` |
| GPS | 3 | - | L1C pilot | `SigIDGPSL1CP` | `1L` |
| GPS | 11 | - | L1C data | `SigIDGPSL1CD` | `1S` |
| GPS | 6 | - | L5 data | `SigIDGPSL5I` | `5I` |
| GPS | 14 | - | L5 pilot | `SigIDGPSL5Q` | `5Q` |
| GPS | 17 | - | L2C(L) | `SigIDGPSL2CL` | `2L` |
| GLONASS | 0 | - | L1 C/A | `SigIDGLOL1` | `1C` |
| GLONASS | 5 | - | L2 C/A | `SigIDGLOL2` | `2C` |
| GLONASS | 6 | - | G3I | `SigIDGLOL3I` | `3I` |
| GLONASS | 7 | - | G3Q | `SigIDGLOL3Q` | `3Q` |
| Galileo | 1 | - | E1B | `SigIDGALE1B` | `1B` |
| Galileo | 2 | - | E1C | `SigIDGALE1C` | `1C` |
| Galileo | 12 | - | E5a pilot | `SigIDGALE5aQ` | `5Q` |
| Galileo | 17 | - | E5b pilot | `SigIDGALE5bQ` | `7Q` |
| Galileo | 18 | - | E6B | `SigIDGALE6B` | `6B` |
| Galileo | 22 | - | E6C | `SigIDGALE6C` | `6C` |
| BDS | 0 | - | B1I | `SigIDBDSB1I` | `2I` |
| BDS | 4 | - | B1Q | `SigIDBDSB1Q` | `2Q` |
| BDS | 8 | - | B1C pilot | `SigIDBDSB1CP` | `1P` |
| BDS | 23 | - | B1C data | `SigIDBDSB1CD` | `1D` |
| BDS | 5 | - | B2Q | `SigIDBDSB2Q` | `7Q` |
| BDS | 17 | - | B2I | `SigIDBDSB2I` | `7I` |
| BDS | 12 | - | B2a pilot | `SigIDBDSB2aP` | `5P` |
| BDS | 28 | - | B2a data | `SigIDBDSB2aD` | `5D` |
| BDS | 6 | - | B3Q | `SigIDBDSB3Q` | `6Q` |
| BDS | 21 | - | B3I | `SigIDBDSB3I` | `6I` |
| BDS | 13 | - | B2b(I) | `SigIDBDSB2bI` | `7D` |
| QZSS | 0 | - | L1 C/A | `SigIDQZSSL1CA` | `1C` |
| QZSS | 1 | - | L1C/B | `SigIDQZSSL1CB` | `1E` |
| QZSS | 3 | - | L1C pilot | `SigIDQZSSL1CP` | `1L` |
| QZSS | 4 | - | L1S | `SigIDQZSSL1S` | `1Z` |
| QZSS | 6 | - | L5 data | `SigIDQZSSL5I` | `5I` |
| QZSS | 9 | false/true | Undocumented in OBSVM | - | Unmapped |
| QZSS | 11 | - | L1C data | `SigIDQZSSL1CD` | `1S` |
| QZSS | 14 | - | L5 pilot | `SigIDQZSSL5Q` | `5Q` |
| QZSS | 17 | - | L2C(L) | `SigIDQZSSL2CL` | `2L` |
| QZSS | 21 | - | L6D | `SigIDQZSSL6` | `6S` |
| QZSS | 27 | - | L6E | `SigIDQZSSL6E` | `6E` |
| SBAS | 0 | - | L1 C/A | `SigIDGPSL1CA` | `1C` |
| SBAS | 6 | - | L5(I) | `SigIDGPSL5I` | `5I` |
| NavIC | 6 | - | L5 data | `SigIDNAVICL5I` | `5A` |
| NavIC | 14 | - | L5 pilot | `SigIDNAVICL5Q` | Unmapped |

The entries above deliberately follow the local Unicore and `gpsprot`
midpoint tables. Add tests for every supported row, plus representative
reserved and unknown values.

Cross-check notes and inconsistencies:

The table below is only a cross-check. It separates four things that can
disagree:

- The Unicore OBSVM tracking-status and frequency-identifier documentation.
- The current SatPulse `gps/internal/unc` mapping to `gpsprot.SignalID`.
- The RTKLIB Explorer Unicore `sig2code` mapping.
- The RINEX 4.02 observation-code definitions.

The current SatPulse midpoint mapping is based on `SATSINFO`'s `Freq status`
field, which references Unicore's separate `Frequency Identifier` table. OBSVM
uses `ch-tr status`, which references `Table Channel Tracking Status`; bits
`21..25` are the tracking-status `Signal type`, with bit `26` as an additional
L2C discriminator. The existing midpoint table is useful supporting context,
but it is not definitive for OBSVM tracking-status-to-RINEX mapping.

GPS `sigType=9` is not an inconsistency in this OBSVM mapping. For OBSVM, the
Channel Tracking Status table defines bit `26` as part of the signal identity:
`sigType=9, l2c=0` maps to RINEX `2W`, and `sigType=9, l2c=1` maps to RINEX
`2S`.

| Case | Unicore docs | Current SatPulse mapping | RTKLIB Explorer mapping | RINEX 4.02 context | What is inconsistent |
| ---- | ------------ | ------------------------ | ----------------------- | ------------------ | -------------------- |
| QZSS `sigType=9` | OBSVM lists QZSS signal types `0`, `1`, `3`, `4`, `6`, `11`, `14`, `17`, `21`, and `27`. It does not list `9` for QZSS. | There is no QZSS `FreqID=9` row in `gps/internal/unc`; QZSS L2C(L) is `17`. This is only supporting context because SATSINFO uses the separate Frequency Identifier table. | `sig2code` has a QZSS `sigType=9` branch and applies the same `l2c` split as GPS: `2W` or `2S`. | QZSS has RINEX L2C(M), L2C(L), and L2C(M+L) codes, including `2S` and `2L`; RINEX 4.02 does not define QZSS `2W`. | RTKLIB Explorer maps a QZSS signal type that is absent from the Unicore OBSVM Channel Tracking Status table. This row is resolved as unmapped. |
| QZSS `sigType=21` L6D | OBSVM says QZSS `21 = L6D`. | `gps/internal/unc` maps `FreqQZSSL6D` to `SigIDQZSSL6`, with a comment that `gpsprot` uses `L6` to mean L6D. This agrees with the OBSVM signal name, but it is only supporting context. | `sig2code` maps `sigType=21` to `CODE_L6Z`, i.e. RINEX `6Z`. | RINEX 4.02 QZSS says L6D is `6S`, L6E is `6E`, and L6(D+E) is `6Z`. | RTKLIB Explorer maps the OBSVM-documented single-channel L6D signal to the RINEX combined L6(D+E) code. This conflicts with both the OBSVM signal name and the RINEX QZSS L6 code definitions. |
| BDS `sigType=13` B2b(I) | OBSVM says BDS `13 = B2b(I)`. Other Unicore fields also use B2b I/Q terminology. | `gps/internal/unc` maps `FreqBDSB2bI` to `SigIDBDSB2bI`. This agrees with the Unicore name, but it is only supporting context because it comes from the Frequency Identifier table path. | `sig2code` maps `sigType=13` to `CODE_L7P`, i.e. RINEX `7P`. | RINEX 4.02 BDS B2b codes are `7D` for data, `7P` for pilot, and `7Z` for data+pilot. | The OBSVM table names the signal B2b(I), while RTKLIB Explorer maps it to the RINEX pilot code. The conflict is between RTKLIB Explorer's `7P` choice and the usual GNSS convention that I denotes the data channel; RINEX itself uses data/pilot names rather than I/Q names for B2b. |
| NavIC `sigType=6` and `14` | OBSVM says IRNSS/NavIC `6 = L5 data` and `14 = L5 pilot`. A separate Unicore GNSS ID / Signal ID table names NavIC `L5-SPS` and `L5-RS`, but that is a different identifier scheme from the OBSVM tracking-status signal type. | `gps/internal/unc` maps `6` to `SigIDNAVICL5I` and `14` to `SigIDNAVICL5Q`. These `gpsprot` NavIC signal IDs use generic I/Q-style names and do not resolve the RINEX SPS-vs-RS distinction. | `sig2code` maps `6` to `CODE_L5A`, i.e. RINEX `5A`, and `14` to `CODE_L5C`, i.e. RINEX `5C`. | RINEX 4.02 NavIC L5 says `5A` is L5 A SPS, `5B` is L5 B RS data, `5C` is L5 C RS pilot, and `5X` is B+C. | Unicore OBSVM uses generic data/pilot labels, while RINEX NavIC data/pilot labels refer to restricted-service B/C and the ordinary SPS signal is A. RTKLIB Explorer maps the data row to SPS `5A` but the pilot row to restricted-service `5C`; the current SatPulse `gpsprot` midpoint names are not authoritative for this RINEX decision. |

Resolution notes:

- GPS `sigType=9` is resolved by using the OBSVM `l2c` bit in `RINEXObsSig`.
- BDS `sigType=13` should map to RINEX `7D`. The OBSVM signal name is
  `B2b(I)`, and I is the B2b data component. RTKLIB Explorer's `7P` mapping is
  treated as wrong for this row.
- QZSS `sigType=21` should map to RINEX `6S`. The OBSVM signal name is L6D,
  and RINEX `6S` is the QZSS L6D code. RTKLIB Explorer's `6Z` mapping is
  treated as wrong for this row.
- NavIC `sigType=6` should map to RINEX `5A`, the NavIC L5 SPS signal.
- NavIC `sigType=14` should remain unmapped. RTKLIB Explorer maps it to RINEX
  `5C`, but `5C` is NavIC L5 restricted-service pilot.
- QZSS `sigType=9` should remain unmapped because it is not documented in the
  OBSVM Channel Tracking Status table.

### Unicore LLI handling

Unicore `OBSVMB` has a continuous `locktime` field rather than an RTCM-style
lock-time indicator. Use the same state-management pattern as `rinex/rtcm`,
but base the slip check on whether `locktime` advanced by approximately the
epoch interval:

```text
locktime - previousLocktime + 0.05 <= epochDeltaSeconds
```

When that condition is true for the same satellite and signal, set
`LLILostLock`. If carrier phase is invalid for the current observation, carry
the lost-lock state forward and emit it on the next observation for that
satellite and signal that has carrier phase. The `0.05` second tolerance
matches the current RTKLIB Explorer Unicore decoder, but the implementation
should be validated with local tests rather than treating that decoder as
authoritative.

The Unicore V1.13 documentation does not identify a half-cycle ambiguity bit in
`OBSVMB` tracking status. The converter should therefore not set
`LLIHalfCycleAmbiguity` for Unicore observations. Do not infer half-cycle
ambiguity from unrelated reserved bits.

### Unicore convobs wiring

Wire the converter into `convobs` after the package-level decoder is tested:

- Add explicit `--from uncb` input selection.
- Add `--from unca` only when `OBSVMA` parsing is implemented.
- Include Unicore binary packets in packet-log conversion. The current
  packet-log scanner is UBX-specific and must be widened before packet-log
  `UNCB` input can work.
- In `--from raw` mode, select the Unicore converter when the first supported
  raw observation message is `UNCB OBSVM`.
- Reject later UBX RAWX or RTCM MSM7 observations after selecting Unicore, and
  reject later Unicore `OBSVM` observations after selecting UBX or RTCM.

## Full RINEX observation reader

The current RINEX observation reader is intentionally narrow. It can read the
ordinary observation files generated by `convobs`, but it is not yet a general
reader for every valid RINEX observation file. The first goal is to correctly
extract `SignalObservation` records from any valid RINEX observation file. The
second goal is to extend `Metadata` so that useful header information that is
currently ignored can be represented in `.obsj` and optionally written back to
RINEX.

### Observation extraction

The reader should parse the epoch flag explicitly instead of treating every
epoch record as an ordinary observation epoch. At present it reads the time and
count, converts the time to GPST, and then assumes the following records are
satellite observation records. This works for epoch flags `0` and `1`, but not
for the other valid event forms.

Needed changes:

- Handle ordinary observation epochs, flags `0` and `1`, as observations.
  Flag `1` should not change the observation values, but the power-failure
  event should be available for reporting or future metadata if needed.
- Skip or process event records for flags `2` through `5`. These are special
  event records, not satellite observation records. Some event records may have
  blank epoch fields, so the epoch parser must not require a full timestamp for
  these flags.
- For flag `4`, parse the following header records sufficiently to keep the
  reader state correct for later observations. Header records that affect
  observation decoding, such as `SYS / # / OBS TYPES`, `SYS / SCALE FACTOR`,
  `GLONASS SLOT / FRQ #`, and `LEAP SECONDS`, must update the active header
  state.
- Treat flag `6` cycle-slip records as non-observation records. They use the
  same row layout as observations, but the values are slips, not measurements,
  so ingesting them as `SignalObservation` records is incorrect.
- Deal with unknown or future event flags by skipping the stated number of
  following records and reporting the event, rather than trying to parse the
  records as observations.
- Apply `SYS / SCALE FACTOR` to the affected observation fields. The stored
  RINEX value must be divided by the scale factor before use.
- Honor the optional extra epoch second digits in RINEX 4.02 epoch records so
  epochs can be read at the precision encoded in the file.
- Handle the `SIGNAL STRENGTH UNIT` header when populating `CN0`. If the unit
  is not the expected dB-Hz representation, the reader should either convert
  the value or preserve enough metadata to avoid presenting it as dB-Hz.
- Continue to ignore `SYS / PHASE SHIFT` and `GLONASS COD/PHS/BIS` for
  observation extraction. In RINEX 4.02 these records are strongly deprecated
  and specified as ignored by decoders and encoders.

### Metadata coverage

After the reader can correctly extract observations from valid files, extend
`Metadata` for header information that is useful for diagnostics, round trips,
or downstream tools. Unknown header records should still not be fatal; they
should be ignored, preserved as comments/raw records, or reported depending on
how much structure we choose to expose.

Useful additions include:

- RINEX file provenance: `RINEX VERSION / TYPE`, `PGM / RUN BY / DATE`, and
  file-level system/time-system declarations.
- Observation timing headers: `INTERVAL`, `TIME OF FIRST OBS`, `TIME OF LAST
  OBS`, and `RCV CLOCK OFFS APPL`.
- Signal metadata: `SIGNAL STRENGTH UNIT`, `SYS / SCALE FACTOR`, and any
  active observation type lists needed to explain how records were decoded.
- Correction provenance: `SYS / DCBS APPLIED` and `SYS / PCVS APPLIED`.
- Antenna and station details beyond the fields currently represented:
  antenna phase center, zero direction, bore-sight, center of mass, and other
  optional station records from the observation header.
- Deprecated compatibility records, such as `SYS / PHASE SHIFT` and
  `GLONASS COD/PHS/BIS`, if preserving them helps compare against third-party
  RINEX producers. They should not be used to alter extracted observations for
  RINEX 4.02 decoding.

This work should keep the `.obsj` model focused on observations first. Metadata
fields should be added when they preserve real semantics or make diagnostics
and comparisons clearer, not merely to mirror every fixed-column header line.
