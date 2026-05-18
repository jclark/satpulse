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

## Next steps

Add an `.obsj` reader in `gps/lib/rinex`. It should take an `io.Reader`, read
JSONL records containing a mixture of `SignalObservation` and `Metadata`, and
return separate slices of observations and metadata.

Add metadata merge support. The input should be a slice of `Metadata` records;
the output should be a single merged metadata value suitable for driving RINEX
header generation.

Add a RINEX observation writer. It should take an `io.Writer`, the merged metadata,
and a slice of `SignalObservation`, then write a
RINEX observation file or return an error. The writer will derive observation
code lists, epoch ordering, satellite ordering, and other header facts from the
complete observation slice.

Add a sink interface in `gps/lib/rinex` for conversion output. It should have
methods for `Metadata` and `SignalObservation`, and probably a `Flush` or close
method for final output and error reporting.

Provide one sink implementation that writes `.obsj` JSONL to an `io.Writer`.
This implementation can write records as they arrive.

Provide another sink implementation that writes RINEX observation output to an
`io.Writer`. This implementation should merge metadata as it arrives and buffer
observations until flush/finalization, then call the RINEX observation writer
described above. RINEX output needs whole-file facts for the header,
observation code lists, epoch ordering, and satellite ordering.

Add conversion from receiver stream formats outside `gps/lib/rinex`. The
converter code may depend on domain-layer packages and protocol-specific packet
handling. Converters should write their output to the `gps/lib/rinex` sink
interface, so conversion can be used for streaming `.obsj` generation, direct
RINEX generation, and tests.

Start with UBX-RXM-RAWX. It maps naturally to `SignalObservation`: one RAWX
measurement group becomes one signal observation, with the RAWX message epoch
providing the GPS-scale `Time`. The main missing piece is the mapping from UBX
`gnssId`/`sigId` to RINEX satellite and signal identifiers, plus state for
deriving LLI from carrier-phase validity and lock-time changes.

Add RTCM MSM7 conversion after that. MSM7 provides pseudorange, carrier phase,
Doppler, carrier-to-noise density, lock time, half-cycle ambiguity, and for
GLONASS the FDMA frequency channel. The converter takes a stream of RTCM
messages: MSM7 messages emit `SignalObservation` records, while ARP, receiver,
and antenna-related messages emit `Metadata` records. The converter will need
to assemble MSM fragments for the same epoch/reference station and map RTCM
satellite and signal IDs to RINEX identifiers.
