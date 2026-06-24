# NMEA GGA support: parsing, synthesis, and proxy (#329)

## Motivation

GGA is the NMEA sentence that carries a receiver position fix. We need better
GGA handling in several places:

- `satpulsetool ntrip --gga` needs to upload a literal GGA sentence to test VRS
  casters.
- `nmeamsg` needs typed GGA parsing and serialisation as the first step toward
  more general fieldenc-based NMEA handling.
- The daemon should be able to synthesize GGA from decoded `gpsprot` messages
  when the receiver is not emitting NMEA GGA.
- A TCP proxy serving NMEA should be able to include synthesized GGA when the
  receiver stream does not already include a real GGA for that UTC.

This plan is for the general NMEA/GGA work. VRS pull support remains in
`plan/vrs.md` and depends on stages 2, 3, and 4 here.

## Stage 1: `ntripcmd --gga` option (done)

Standalone diagnostic: forward a caller-supplied GGA so the ntrip pull path can
be exercised against a VRS caster (such as PointPerfect) without a live receiver
or position synthesis. The caller pastes a real GGA captured from a receiver;
`ntripcmd` validates it and sends it verbatim.

- Add `--gga sentence`. The argument is a full NMEA GGA sentence without a line
  terminator (e.g. `$GPGGA,...*47`); a trailing CRLF from copy-paste is
  tolerated.
- Validation uses the existing `nmeamsg` syntax helpers:
  `nmeamsg.CheckSyntax(...).IsValidApprovedNMEA()` for a well-formed approved
  sentence, that the address is a `GGA` sentence, and that the `*hh` checksum
  matches `nmeamsg.Checksum`. On success it returns the wire bytes with a single
  CRLF appended.
- Mechanism: once the v1 handshake succeeds (`ICY 200 OK`), `NtripSource` sends
  the GGA to the caster as a separate write on the accepted stream (the spec
  allows a GGA after the request). `ntripcmd` constructs the source with the
  validated bytes.
- Per the spec one GGA suffices to start the stream; `ntripcmd` does not resend.

This stage depends only on existing NMEA syntax support. It proves the upload
mechanism independently of GGA synthesis. The later VRS plan supersedes the
static `NtripSource.GGA` field by routing `--gga` through the shared
post-handshake GGA sender; this stage describes the completed first diagnostic,
not the final writer architecture.

## Stage 2: GGA parsing and serialisation with `fieldenc`

Add typed GGA parsing and serialisation to `gps/lib/nmeamsg`. This is a
low-level syntax/message package, so it must stay free of `gpsprot`
dependencies. The GGA work should be shaped as the first reusable NMEA
fieldenc-backed sentence implementation, not as a VRS helper.

### Sentence wrapper

Use the split used by `novmsg`: the wrapper carries sentence addressing, while
the payload struct is what `fieldenc` sees. Do not put a `Sentence()` method on
GGA data; packet construction (address, checksum, CRLF) belongs in the wrapper
serialiser.

```go
type TalkerID string

const TalkerGP TalkerID = "GP"

type SentenceFields interface {
	SentenceFormat() string
}

type Sentence[F SentenceFields] struct {
	TalkerID TalkerID
	Fields   F
}
```

`Sentence[F]` represents an addressed approved NMEA sentence. `F` is the typed
field set and provides the NMEA sentence format (`"GGA"` here). `Serialize`
applies `fieldenc.Encode` to `s.Fields`, builds the checksum payload as
`<talker><format>[,<fields>]`, computes `nmeamsg.Checksum` over that payload
only (the bytes between `$` and `*`), and returns `$` + payload + `*` +
checksum + CRLF:

```go
func Serialize[F SentenceFields](s Sentence[F]) ([]byte, error)
func Parse[F SentenceFields](data string) (Sentence[F], error)
```

`Parse` checks the NMEA syntax and checksum, verifies that the address format
matches `F.SentenceFormat()`, decodes the fields with `fieldenc`, and preserves
the talker ID from the input. If the generic `Parse[F]` API turns out awkward in
implementation, a GGA-specific parser is acceptable for the first step, but the
field layout should still be reusable for later NMEA sentences.

### GGA fields

`fieldenc` is strictly one struct field per output slot, but GGA latitude and
longitude are each two wire fields (magnitude + hemisphere). So they are
separate struct fields, and the constructor splits the signed angle into a
degrees/minutes magnitude and a hemisphere.

```go
type GGAFields struct {
	Time     opt.Val[todUTC]   // "hhmmss.ss"
	LatMin   opt.Val[latCoord] // "ddmm.mmmmm"  (deg+min struct, 2-digit degrees)
	LatNS    string            // "N"/"S", empty when absent
	LonMin   opt.Val[lonCoord] // "dddmm.mmmmm" (deg+min struct, 3-digit degrees)
	LonEW    string            // "E"/"W", empty when absent
	Quality  uint8Dec          // base-10 uint8, 0 = no fix
	NumSats  opt.Val[uint8Dec2] // two-digit base-10 uint8
	HDOP     opt.Val[float32]  // decimal number
	Alt      opt.Val[float64]  // antenna altitude
	AltUnit  string            // "M", empty when absent
	GeoidSep opt.Val[float64]  // geoidal separation
	GeoidU   string            // "M", empty when absent
	DGPSAge  opt.Val[float32]  // seconds since DGPS update
	DGPSID   opt.Val[uint16Dec4]
}

func (GGAFields) SentenceFormat() string { return "GGA" }
```

`gps/lib/nmeamsg` may import `gps/lib/opt`. Empty non-string fields are
represented by wrapping the real wire type in `opt.Val[T]`, not by weakening
numeric fields to strings. Fields whose wire type is already text, such as
hemisphere and unit fields, remain plain `string`; the empty string is the empty
wire field. Field-level semantic validation belongs in a later GGA validation
layer, not in `fieldenc` decoding. Most GGA fields must be optional because
valid no-fix receiver output can leave them empty. The NMEA 4.00 GGA definition
explicitly says only field 6, the GPS quality indicator, should not be a null
field. Cold-start logs confirm real receivers emit valid GGA with an empty time
(`$GPGGA,,,,,,0,00,99.99,...`) and with time present but position, altitude,
geoidal separation, and DGPS fields empty.

Custom types (each gets `MarshalText`, and `UnmarshalText` for parsing
symmetry):

- `todUTC` - UTC time-of-day `time.Time` <-> `hhmmss.ss`. The date part of the
  `time.Time` is not part of GGA and must not be significant after parsing.
- `latCoord` / `lonCoord` - one lat/lon magnitude field (`ddmm.mmmmm` /
  `dddmm.mmmmm`), represented as a small struct that mirrors the wire form:
  whole `deg uint16` plus `min int64`, the minutes held as fixed-point scaled by
  1e5 (so `0 <= min < 60e5`). Both types share one underlying `coord` struct
  and differ only in degree-field width (2 vs 3 digits). `MarshalText` prints
  `deg` zero-padded to that width, then the minutes as `%02d.%05d` from
  `min/1e5` and `min%1e5`. No float and no `decconv`, so output is exact and
  `UnmarshalText` round-trips it byte-for-byte.
- `uint8Dec`, `uint8Dec2`, `uint16Dec4` - unsigned integer wire fields parsed
  as base-10 with the named Go integer width; the suffix gives the serialized
  zero-padded field width when the NMEA spec fixes one (`xx`, `xxxx`). These are
  generic formatting types, not semantic field types: use them for NMEA fields
  where zero-padded decimal input such as `08` or `0001` must parse as decimal.
  Do not use bare unsigned integers for those fields, because `fieldenc` parses
  unsigned integers with `strconv.ParseUint(..., 0, ...)`.

`Parse` should validate the GGA field count explicitly before calling
`fieldenc.Decode`, so non-standard extra fields and truncated sentences have a
deliberate policy rather than falling out of `fieldenc`'s generic count rules.
Exact preservation of the original decimal spelling is not a goal of
`fieldenc` decoding; conversion into the appropriate wire type is enough. For
decimal-number fields, use `float32` for HDOP and DGPS age, and `float64` for
altitude and geoidal separation. This keeps the typed fields simple while
matching the existing `gps/internal/nmea` parser's acceptance of float-valued
fields.

API:

```go
func MakeGGA(talker TalkerID, t time.Time, lat, lon float64, quality, numSats uint8, hdop float64) Sentence[GGAFields]
func (g GGAFields) LatLon() (lat, lon float64)
```

`MakeGGA` derives the hemisphere from the original sign and splits the absolute
float64 degrees into the exact wire coord representation: `deg = int(absA)`,
`min = round((absA-deg)*60e5)`, carrying `min == 60e5` up into `deg`. It is a
convenience for complete synthesized fixes and sets the optional fields it can
populate. `LatLon` recombines the coord fields with the hemisphere fields,
recovering exactly the position the sentence encodes.

Tests:

- Golden sentences for known positions in each hemisphere, exact bytes incl.
  checksum, produced through `Serialize`.
- Parse golden sentences and verify the typed fields.
- Parse and serialize zero-padded decimal unsigned fields such as `08` and
  `0001`.
- Parse decimal fields with varied fractional digit counts and verify the typed
  `float32`/`float64` values.
- Parse captured no-fix GGA with empty time
  (`$GPGGA,,,,,,0,00,99.99,,,,,,*48`) and with time but no position
  (`$GNGGA,060713.00,,,,,0,06,4.88,,,,,,*49`).
- Unset optional fields and empty string fields serialize as empty fields; parse
  maps them back to unset optionals or empty strings respectively.
- GGA field-count validation rejects the wrong number of fields explicitly.
- Minutes < 10 (zero-padding) and the degree/minute split boundary.
- `todUTC` parse/serialize preserves the time-of-day fields; parsed dates are
  not significant.
- `LatLon` round-trips `MakeGGA(...).Fields` inputs within tolerance.
- Serialize -> parse checks typed fields; parsing captured receiver output does
  not require byte-exact reserialization of decimal spelling.

Follow-up: once `GGAFields` parses via `fieldenc.Decode`, it can replace the
hand-rolled `parseGGA` internals in `gps/internal/nmea/nmea.go`. That replacement
intentionally tightens empty-quality handling: `GGAFields.Quality` is
non-optional, while the legacy parser treats empty quality like no fix, so make
that compatibility decision explicitly when doing the replacement.

## Stage 3: NMEA GGA synthesis from `gpsprot`

Add a reusable `gpsprot.MsgHandler` that synthesizes GGA from decoded GPS
messages and sends typed GGA sentences to a sink. This produces candidate GGA
sentences; it does not decide whether a real receiver GGA should win.

Placement is fixed by the layering in `docs/internals.md`: `nmeasyn` is a
domain-layer package (`gps/nmeasyn`), alongside `gpsprot`. It uses the `gpsprot`
domain abstraction but has no goroutines and does no logging. It imports
`gpsprot`, so it is too high for `gps/lib/`, and it is wired from `time/`, so it
cannot be `gps/internal/`. Implementation must add an entry under the `### gps/`
section of `docs/internals.md`.

```go
type Sink interface {
	SynthGGA(gga nmeamsg.Sentence[nmeamsg.GGAFields])
}

func New(sink Sink) *Synth // *Synth implements gpsprot.MsgHandler
```

Behavior:

- Embed `gpsprot.DefaultHandler` and implement only the callbacks it needs.
- Accumulate per-epoch state from `TimeMsg` and `PosGeoMsg`/`PosECEFMsg`.
- On `NavEpochMsg`, build a `nmeamsg.Sentence[nmeamsg.GGAFields]` and call
  `sink.SynthGGA(gga)`.

This relies on the `gpsprot` epoch ordering contract: `NavEpochMsg` is emitted
once at the end of a navigation epoch, after the time, position, and velocity
messages for that epoch have been dispatched. The synthesizer treats
`NavEpochMsg` as the end-of-epoch trigger and uses the `TimeMsg` and position
state accumulated before that callback. If a future packet processor cannot
provide that ordering, it must adapt its output before using this synthesizer.

Field sources:

- time from `TimeMsg`;
- lat/lon from `PosGeoMsg.LatLon`;
- ecef from `PosECEFMsg.Pos`, bridged to `geopos.ECEF` and converted via
  `geopos.WGS84.ECEFtoLLH`, used only if there is no lat/lon;
- fix quality (GGA field 6) from `NavEpochMsg.FixLevel`, refined by
  `Correction` and `AuxSrc`:
  - below `FixLevelCode` with `AuxSrc&AuxSrcDR != 0` -> 6 (dead reckoning);
  - below `FixLevelCode` otherwise -> 0 (no fix);
  - `FixLevelCode` -> 1 (GPS), or 2 (DGPS) when `Correction&CorrUsed != 0`;
  - `FixLevelCarrierFloat` -> 5 (RTK float);
  - `FixLevelCarrierFixed` -> 4 (RTK fixed);
- HDOP from `NavEpochMsg.DOP.Hor`;
- satellites-used from `NavEpochMsg.NumSVUsed`;
- height and differential-data fields are intentionally empty initially.

The synthesizer emits a GGA every epoch. Quality is `0` when there is no fix, so
consumers that care about validity read `gga.Fields.Quality`. Optional source
values that are not available remain unset in `GGAFields` and serialize as empty
NMEA fields.

The initial synthesizer intentionally emits only GGA quality values that
`gpsprot` can represent without inventing information. It can synthesize quality
6 from `AuxSrcDR`, but it does not synthesize manual-input/simulator qualities 7
and 8 because `FixLevelNotMeasured` does not preserve that distinction.

Tests:

- Synthetic callbacks in `TimeMsg`, `PosGeoMsg`, `NavEpochMsg` order -> expected
  GGA fields (time, position, quality, HDOP, satellites-used), pinning the
  epoch ordering contract.
- No-fix epoch -> quality 0.
- ECEF-only epoch exercises the conversion.
- Later real NMEA selection is not tested here; this package only synthesizes
  candidates.

## Stage 4: selected GGA feed

Add the shared selection path that chooses the best GGA stream for consumers
that need position upload. This stage is not a proxy feature.

Inputs:

- original receiver GGA packets accepted by the normal NMEA packet processor;
- synthesized GGA candidates from stage 3.

Output:

- a selected GGA packet channel, carrying `scan.Packet` values that are valid
  NMEA GGA wire packets. This is a capacity-1 latest-value channel, not a FIFO:
  the selector publishes without blocking, and when the channel already contains
  a pending GGA it drops that old packet before trying to store the newer one.

The selected-GGA feed is the part VRS needs. It is also the core used by the
later proxy stage, but it does not create any proxy service and does not require
`[[proxy.tcp]]`.

`Dispatcher.handlePacket` is the natural join point for original receiver GGA.
It should identify a valid original GGA candidate in `handlePacket`, run the
normal `ProcessPacket`, and feed the original packet to the selector only if
processing succeeds. That way the selector sees packets that the normal parser
accepted.

For selector input, "valid original GGA candidate" means checksum-valid approved
NMEA GGA that also passes the stage 2 typed GGA parser, including the explicit
GGA field-count check. A packet that the legacy parser accepts only because it is
permissive must not enter the selected-GGA output.

The selector owns the selected-output channel and has two inputs:

- `OriginalGGA(pkt scan.Packet)` forwards a valid original GGA packet
  immediately and records its UTC field when it is comparable.
- `SynthGGA(gga nmeamsg.Sentence[nmeamsg.GGAFields])` serializes the sentence,
  wraps it as a valid NMEA `scan.Packet`, and forwards it only if it is not for
  the same UTC as the last directly emitted original GGA.

The selector is single-threaded: `gpsevent.Dispatcher` is the only caller, and
it calls `OriginalGGA` and `SynthGGA` from the event-dispatch goroutine. The
selector does not need internal locking unless a later caller breaks that
ownership model.

Publishing selected GGA must never block the dispatcher. The selector's output
operation is latest-wins: try to send, and if the capacity-1 channel is full,
do a nonblocking receive to discard the pending old GGA, then try to send the
newer GGA. If the consumer races with this replacement, dropping a selected GGA
is acceptable; GGA is current-position state, not a history stream.

The GGA comparison is intentionally simple. GGA has the fixed prefix `$xxGGA,`,
so the UTC field's integer-second digits are at `pkt.Data[7:13]` for a normal
approved GGA sentence with a non-empty `hhmmss` time. Use a helper that returns
`(utc string, ok bool)` and checks both the fixed prefix and the six time
digits. `OriginalGGA` forwards every valid original GGA immediately, but records
the UTC only when that helper succeeds. `SynthGGA` suppresses a synthesized
packet only when the last directly emitted original GGA and the synthesized
candidate both have comparable UTC seconds and those strings are equal; if
either time is empty or malformed, the synthesized packet is forwarded. There is
no need to decide whether one GGA is "after" another, so midnight wraparound
does not need special ordering logic. If a future use needs ordering, a
`ggaAfter` helper can compare the same fixed-position substrings with explicit
midnight handling.

Same-UTC suppression does not require a separate ordering mechanism. Synthesized
GGA is emitted only from the `NavEpochMsg` end-of-epoch callback, after the
messages that make up that epoch have been processed. Therefore any original
receiver GGA in the epoch has already reached `OriginalGGA` before `SynthGGA`
can offer the synthesized candidate for that epoch.

The daemon constructs this selected-GGA channel when any configured consumer
needs it, currently VRS pull or the proxy synth stage. `time/app/daemon` can see
both the receiver-side dispatcher and `stream.PullSetup`; `gps/app/stream` must
only receive the selected-GGA channel and must not import `time/internal`.

Daemon wiring:

- `time/app/daemon` determines whether a selected-GGA feed is needed after
  preparing `stream.PullSetup` and before creating `gpsevent.Dispatcher`.
- If needed, the daemon creates the selected-GGA channel and selector, gives the
  receive side to `stream.PullSetup` when VRS is enabled, and passes the selector
  into `gpsevent.NewDispatcher` as an optional receiver-side sink.
- When the selector is present, `gpsevent.Dispatcher` also owns a
  `nmeasyn.Synth` whose sink is that selector. The dispatcher fans out the same
  decoded `gpsprot` message callbacks it already receives to the synthesizer, in
  the same order, so `nmeasyn` sees the complete `TimeMsg`/position/`NavEpochMsg`
  epoch sequence.
- `gpsevent.Dispatcher` feeds original receiver GGA from `handlePacket` and
  synthesized GGA candidates from the embedded `nmeasyn` into that selector.
- `gps/app/stream` sees only `<-chan scan.Packet`; it does not construct the
  selector, depend on `gpsevent`, or import anything under `time/internal`.

Wrapping synthesized output as a `scan.Packet` likely needs `gpsreg` to
re-export the NMEA packet format, just as it already re-exports
`RTCMPacketFormat`.

Tests:

- Original GGA is forwarded immediately.
- Original GGA followed by synthesized GGA with the same UTC suppresses the
  synthesized packet.
- Dispatcher fan-out drives `nmeasyn` with `TimeMsg`, position, and `NavEpochMsg`
  in epoch order, and synthesized GGA is emitted from the end-of-epoch callback.
- Original GGA with an empty UTC field is forwarded but does not suppress a
  later synthesized GGA.
- Synthesized GGA with a different UTC is forwarded.
- Synthesized output is a checksum-valid NMEA `scan.Packet`.
- Publishing selected GGA with a blocked consumer does not block and keeps the
  newest pending GGA.
- The daemon wires a selected-GGA feed when VRS pull or proxy synth needs it.

## Stage 5: `proxy.tcp` NMEA synthesis option

Add a `synth` option to `[[proxy.tcp]]`. It applies only when
`protocol = "NMEA"`. It uses the selected-GGA core from stage 4 but is unrelated
to VRS.

Example:

```toml
[[proxy.tcp]]
listen = "127.0.0.1:2006"
protocol = "NMEA"
synth = true
```

Validation:

- `synth = true` is valid only for TCP proxy services with `protocol = "NMEA"`.
- It affects only the packets written to proxy clients.
- Other protocol-specific proxy services keep using the raw receiver packet
  broadcast.

Architecture:

- The daemon keeps the existing raw receiver packet broadcast (`pb`) fed by
  `startScan`.
- When a TCP NMEA proxy has `synth = true`, the daemon also creates a selected
  NMEA packet channel plus a second `bcast.Bcast[scan.Packet]`.
- `proxy.Start` grows access to both broadcasts. A TCP service with
  `protocol = "NMEA"` and `synth = true` subscribes to the selected NMEA
  broadcast. Other services subscribe to the raw receiver broadcast.
- `gpsevent.Dispatcher` gets an optional NMEA selector path. It feeds valid
  original NMEA packets from `handlePacket` to the selected NMEA output and
  feeds original GGA plus synthesized GGA candidates through the stage 4
  selected-GGA core.

For proxy clients, the NMEA selector output is broader than the stage 4
selected-GGA feed:

- non-GGA original NMEA packets pass through immediately;
- original GGA packets pass through immediately and update the shared
  selected-GGA state;
- synthesized GGA packets are inserted only when stage 4 does not suppress them
  for the same UTC as an original receiver GGA.

Tests:

- Config validation accepts `synth = true` only for `protocol = "NMEA"` on
  `[[proxy.tcp]]`.
- Selected NMEA bcast passes original NMEA packets through.
- Proxy NMEA output uses the same-UTC suppression as the selected-GGA core.
- TCP NMEA proxy with `synth = true` subscribes to selected NMEA; other proxy
  services stay on the raw packet stream.
- Add a `smoketest/scenarios/proxy/tcp-synth` scenario (`.py`, `.toml.in`,
  `SCENARIOS` entry, and README list entry) for `protocol = "NMEA"` with
  `synth = true`, checking that a live TCP proxy client receives original NMEA
  and synthesized GGA fill-ins.

## Open decisions

- Whether to add `synth` to Unix socket proxy services later. Stage 5 only covers
  `[[proxy.tcp]]`.
