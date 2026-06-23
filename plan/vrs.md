# Ntrip VRS support: client GGA upload (#325)

## Motivation

A Virtual Reference Station (VRS) caster synthesises corrections for the
client's own location, so the client must upload its position as an NMEA GGA
sentence before the caster will stream. This is defined in the Ntrip spec
(v1 sec 5.5.3, v2 sec 2.1.3): when a mountpoint's source-table `<nmea>` field
is `1`, the caster needs at least one GGA to "prepare the data and start
sending". u-blox PointPerfect's Ntrip service is a VRS, so we cannot pull from
it until we can send a GGA.

Scope notes:
- Our own caster (`gps/app/ntrip`) is a physical base, never a VRS: field 12
  `<nmea>` is hard-coded `"0"` in `strrec.go`. Nothing here changes that.
- The spec requires only *one* GGA to start; sending more is "allowed ... at
  any time" (used to update a moving client's position). There is **no**
  periodic-GGA / keepalive requirement for the HTTP/TCP streaming flow we use.
  (The v2 "Keep-Alive" section is RTSP/RTP-transport specific and unrelated.)
- We implement the Ntrip v1 request flow.

Synthesising NMEA is generally useful beyond VRS (e.g. exposing NMEA over a TCP
port from a receiver that only emits UBX), so the synthesis lives in its own
reusable package (`nmeasyn`, stage 2) rather than VRS-specific code.

## Stage 1: `ntripcmd --gga` option

Standalone diagnostic: forward a caller-supplied GGA so the ntrip pull path can
be exercised against a VRS caster (such as PointPerfect) without a live receiver
or position synthesis. The caller pastes a real GGA captured from a receiver;
`ntripcmd` validates it and sends it verbatim. It needs only the existing
`nmeamsg` syntax library, so it lands before the GGA synthesis (stage 2).

- Add `--gga sentence`. The argument is a full NMEA GGA sentence without a line
  terminator (e.g. `$GPGGA,...*47`); a trailing CRLF from copy-paste is
  tolerated. Presence of the flag marks VRS mode for the command.
- Validation (`validateGGA`) uses the existing `nmeamsg` syntax helpers, not the
  stage 2 GGA builder: `nmeamsg.CheckSyntax(...).IsValidApprovedNMEA()` for a
  well-formed approved sentence, that the address is a `GGA` sentence, and that
  the `*hh` checksum matches `nmeamsg.Checksum`. On success it returns the wire
  bytes with a single CRLF appended.
- Mechanism: once the v1 handshake succeeds (`ICY 200 OK`), `NtripSource` sends
  the GGA to the caster as a separate write on the accepted stream (the spec
  allows a GGA after the request); `ntripcmd` constructs the source with the
  validated bytes. The write happens inside the handshake, so `Source.Connect`
  stays read-only (`io.ReadCloser`) - the writer half is not exposed yet.
  Stage 3 keeps this same post-connect write but moves ownership to the
  `ggaSender` goroutine, which also re-sends on reconnect and tracks a moving
  client.
- Per the spec one GGA suffices to start the stream; `ntripcmd` does not resend.

Because it forwards a literal sentence, this stage depends only on the existing
`nmeamsg` syntax library, not on the GGA synthesis (stage 2). It is the first
end-to-end proof of the VRS request flow (GGA upload -> caster streams), and -
because it already uses the post-connect socket write - the first confirmation
that the caster accepts a GGA on the stream rather than in the request, the
delivery mechanism stage 3 relies on.

## Stage 2: GGA synthesis

Generate GGA sentences from the receiver's own position, so the daemon can drive
a VRS caster (and expose NMEA generally) without a hand-supplied sentence. Two
parts: a low-level GGA builder in `nmeamsg`, and a domain-layer synthesiser
(`nmeasyn`) that drives the builder from the gpsprot message stream.

### GGA builder (`gps/lib/nmeamsg/gga.go`)

A typed field-set struct holding the GGA fields in wire order, serialised with
`fieldenc` (the long-term direction for NMEA, as in `gps/lib/qtmmsg`).
`nmeamsg` is a low-level syntax library with no `gpsprot` dependency; the
builder keeps that property and takes primitive inputs. Follow the split used by
`novmsg`: the wrapper carries sentence addressing, while the payload struct is
what the field encoder sees. Do not put a `Sentence()` method on GGA data;
packet construction (address, checksum, CRLF) belongs in `nmeamsg.Serialize`.

`fieldenc` is strictly one struct field per output slot, but GGA latitude and
longitude are each two wire fields (magnitude + hemisphere). So they are
separate struct fields, and the constructor splits the signed angle into a
degrees/minutes magnitude and a hemisphere.

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

type GGAFields struct {
	Time     todUTC   // "hhmmss.ss"   (custom marshaler)
	LatMin   latCoord // "ddmm.mmmmm"  (deg+min struct, 2-digit degrees)
	LatNS    hemi     // "N"/"S"       (hemi is `type hemi string`, no marshaler)
	LonMin   lonCoord // "dddmm.mmmmm" (deg+min struct, 3-digit degrees)
	LonEW    hemi     // "E"/"W"
	Quality  uint8    // 0..8 fix quality (0 = no fix)
	NumSats  uint8
	HDOP     dec    // fixed-point, formatted via decconv
	Alt      string // empty (height not used)
	AltUnit  string // empty
	GeoidSep string // empty
	GeoidU   string // empty
	DGPSAge  string // empty
	DGPSID   string // empty
}

func (GGAFields) SentenceFormat() string { return "GGA" }
```

`Sentence[F]` represents an addressed approved NMEA sentence. `F` is the typed
field set and provides the NMEA sentence format (`"GGA"` here). `Serialize`
applies `fieldenc.Encode` to `s.Fields`, then builds the sentence payload as
`<talker><format>[,<fields>]`, appends `*` + `nmeamsg.Checksum` + CRLF, and
returns the packet bytes:

```go
func Serialize[F SentenceFields](s Sentence[F]) ([]byte, error)
```

For `Sentence[GGAFields]` with `TalkerGP`, this yields an address of `GPGGA`.
The talker ID is not baked into the serializer or the GGA field set, so adding
RMC/GSA/etc. later is another `SentenceFields` implementation plus the same
`Sentence[F]` wrapper.

Custom types (each gets `MarshalText`, and `UnmarshalText` for parsing
symmetry):
- `todUTC` - UTC `time.Time` <-> `hhmmss.ss`.
- `latCoord` / `lonCoord` - one lat/lon magnitude field (`ddmm.mmmmm` /
  `dddmm.mmmmm`), represented as a small struct that mirrors the wire form -
  whole `deg uint16` plus `min int64`, the minutes held as fixed-point scaled by
  1e5 (so `0 <= min < 60e5`) - rather than a single packed number. Both types
  share one underlying `coord` struct and differ only in degree-field width (2
  vs 3 digits). `MarshalText` prints `deg` zero-padded to that width, then the
  minutes as `%02d.%05d` from `min/1e5` (whole) and `min%1e5` (fraction) - no
  float and no `decconv`, so output is exact and `UnmarshalText` round-trips it
  byte-for-byte. `MakeGGA` splits its float64 degrees into `deg`/`min`
  (`deg = int(a)`, `min = round((a-deg)*60e5)`, carrying `min == 60e5` up into
  `deg`); `LatLon` recombines them, recovering exactly the position the sentence
  encodes.
- `dec` - fixed-point decimal (HDOP), via `decconv.FormatInt64`.
- `hemi` - `type hemi string`; values "N"/"S"/"E"/"W". No marshaler (reflects
  as its string value).

API (primitive, `gpsprot`-free):

```go
func MakeGGA(talker TalkerID, t time.Time, lat, lon float64, quality, numSats uint8, hdop float64) Sentence[GGAFields]
func Serialize[F SentenceFields](s Sentence[F]) ([]byte, error) // "$<talker><format>,<fields>*hh\r\n"
func (g GGAFields) LatLon() (lat, lon float64) // signed decimal degrees, from coord + hemisphere
```

`LatLon` lets a consumer recover the position numerically without re-parsing the
formatted sentence (used by the VRS move gate, stage 3).

Tests:
- Golden sentences for known positions in each hemisphere, exact bytes incl.
  checksum, produced through `Serialize`.
- Minutes < 10 (zero-padding) and the degree/minute split boundary.
- `LatLon` round-trips `MakeGGA(...).Fields` inputs within tolerance.
- Encode -> decode -> encode round-trip is byte-exact (exercises the
  `UnmarshalText` halves; exactness follows from the integer deg/min coord
  representation).

Follow-up (not required here): once `GGAFields` parses via `fieldenc.Decode`,
it can replace the hand-rolled `parseGGA` in `gps/internal/nmea/nmea.go`.

### NMEA synthesis package (`gps/nmeasyn`)

Placement is fixed by the layering in `docs/internals.md`: `nmeasyn` is a
**domain-layer** package (bare `gps/`), alongside `gpsprot`. It uses the
`gpsprot` domain abstraction but has no goroutines and does no logging (the
`ggaSender` goroutine lives in `gps/app/stream`, behind the `Sink` interface),
so it is not application layer (`gps/app/`); it imports `gpsprot`, so it is too
high for the library layer (`gps/lib/`); and it is wired from `time/`, so it
cannot be `gps/internal/`. Implementation must add an entry under the `### gps/`
section of `docs/internals.md`.

A reusable `gpsprot.MsgHandler` that synthesises NMEA from the gpsprot message
stream and feeds the resulting sentence structs to a sink. Initially produces
GGA only, but as correctly as possible, and structured so other sentence types
(RMC, GSA, ...) can be added.

```go
// Sink receives synthesised NMEA messages (one method).
type Sink interface { GGA(gga nmeamsg.Sentence[nmeamsg.GGAFields]) }

func New(sink Sink) *Synth   // *Synth implements gpsprot.MsgHandler
```

- Implements `gpsprot.MsgHandler` (embedding `gpsprot.DefaultHandler`, the no-op
  base, for the callbacks it ignores). It accumulates per-epoch state from
  `TimeMsg` and `PosGeoMsg`/`PosECEFMsg`, and on `NavEpochMsg` (the epoch
  boundary) builds a `nmeamsg.Sentence[nmeamsg.GGAFields]` from that state plus
  the `NavEpochMsg` fields and calls `sink.GGA(gga)`. (Later sentence types such
  as GSA/GSV would also accumulate `SatellitesMsg` and add their own sink
  methods or a serialization adapter.)
- Field sources, to keep the GGA non-bogus:
  - time from `TimeMsg`;
  - lat/lon from `PosGeoMsg.LatLon` (`Angle`, to decimal degrees);
  - ecef from `PosECEFMsg.Pos` (a `Point3D`, bridged to `geopos.ECEF` and
    converted via `geopos.WGS84.ECEFtoLLH`, used only if there is no lat/lon);
  - fix quality (GGA field 6) from `NavEpochMsg.FixLevel`, refined by
    `Correction` (a `CorrKind` bitmask):
    - below `FixLevelCode` (none / not-measured / Doppler) -> 0 (no fix);
    - `FixLevelCode` -> 1 (GPS), or 2 (DGPS) when `Correction&CorrUsed != 0`;
    - `FixLevelCarrierFloat` -> 5 (RTK float);
    - `FixLevelCarrierFixed` -> 4 (RTK fixed);
  - HDOP from `NavEpochMsg.DOP.Hor` (an `opt.Val`; left empty if unset);
  - satellites-used (GGA field 7) from `NavEpochMsg.NumSVUsed` (an `opt.Val`,
    already on the epoch-boundary message; left empty if unset);
  - height, and the differential-data fields (age / station ID), intentionally
    not filled; populating `DGPSAge` from `NavEpochMsg.DiffAge` is a follow-up
    that would extend `MakeGGA` (the unexported field types mean callers build
    GGAs only through the constructor).
- Emits a GGA every epoch; quality is `0` when there is no fix, so consumers
  that care about validity read `gga.Fields.Quality`. This keeps `nmeasyn`
  general (a TCP-NMEA-server sink wants every epoch).
- Wired into the dispatch fan-out (the `gpsprot.MultiHandler` built in
  `gpsevent.NewDispatcher`) as a `gpsprot.MsgHandler`. Because it joins via the
  interface, only the site that constructs `nmeasyn.New(sink)` imports the
  package.

**Open: how `nmeasyn` joins the dispatch fan-out** - see Open decisions.
(Placement is settled above: `gps/nmeasyn`.)

## Stage 3: `stream` pull VRS support (mobile)

The production path. Keeps working for a moving client: an initial GGA on
connect and live updates as the position changes. The `nmeasyn` sink for pull
is a forwarder that bridges synthesised GGAs into the pull connection; the GGA
is the *only* payload it needs (it carries position and validity), so there is
no separate ECEF / position feed.

1. **Config** - a per-mountpoint `ntrip.vrs = true` flag on the pull ntrip
   config. Only meaningful for ntrip pull; ignored for TCP.

2. **Read-write connection** - `Source.Connect` returns `io.ReadWriteCloser`
   instead of `io.ReadCloser`, so the writer half is available. Only
   `NtripSource` uses it; `TCPSource` ignores it.

3. **`ggaSender`** - a struct owning a goroutine and all of `(currentWriter,
   lastSentLatLon)` plus the latest `Sentence[GGAFields]`; no shared mutable
   state. It
   `select`s on:
   - the GGA feed (cap-1, from the `nmeasyn` sink): hold it as latest; if
     `currentWriter` is set, `gga.Fields.Quality > 0`, and the 2D distance from
     `lastSentLatLon` exceeds the threshold, serialize the latest GGA with
     `nmeamsg.Serialize`, write those bytes with a write deadline, and update
     `lastSentLatLon`; on write error drop the writer (the read side will detect
     the dead connection and reconnect).
   - `connCh` (from `reader`, one value per successful dial: the new writer):
     adopt the writer and immediately serialize and force-send the held latest
     GGA (the initial GGA the caster needs), setting `lastSentLatLon`.
   - `ctx.Done()`: exit.

   The move gate uses `gga.Fields.LatLon()` and a 2D flat-earth
   (equirectangular) distance - `dx = dLon*cos(lat)*k`, `dy = dLat*k`,
   `hypot` - ignoring height, which is ample for a tens-of-metres threshold over
   short distances.

4. **Reconnect (rides the existing loop)** - `reader()` is the only goroutine
   that dials, so it owns reconnection; the existing connect -> `readLoop` ->
   close + emit `errReconnect` -> adaptive-backoff -> reconnect cycle is
   unchanged. The one addition: after each successful `Connect`, `reader()`
   sends the new connection's writer to the `ggaSender` over `connCh`. A value
   on `connCh` *is* the reconnect event for the sender (item 3): it adopts the
   writer and force-sends the held GGA, so even a stationary client re-announces
   on every redial. A broken connection is detected once, by the read side,
   which drives the loop and delivers a fresh writer over `connCh`; the sender's
   dropped writer is replaced then. Backoff, disconnect detection, and the
   pruning-queue reset are the existing mechanism, untouched.

5. **Connect gating** - in VRS mode `reader()` does not dial until the first
   GGA with quality > 0 (a valid fix) exists, so there is something to
   force-send on connect; later reconnects use the held latest immediately. The
   initial GGA goes out as the `ggaSender`'s first socket write after connect
   (v1 allows sending the GGA on the stream after the request), moved out of the
   handshake so the GGA is owned in one place (the `ggaSender`).

6. **Significant-change gate** - 2D distance threshold (constant or config;
   order of tens of metres). No keepalive timer (spec does not require one).

Dependencies: the GGA synthesis (stage 2); the read-write `Source` change.

Behavioural note to document: in VRS mode pull does not connect until a valid
position fix exists, so corrections are gated on the receiver first achieving a
standalone fix (which does not itself need corrections).

Test: Add a scenario to smoketest/ for this.

## Open decisions

- **How `nmeasyn` joins the dispatch fan-out** (placement settled: `gps/nmeasyn`,
  domain layer - see stage 2). The daemon does the binding: `gpsevent` is blind
  to pull (`NewDispatcher` takes no stream argument and `time/internal/gpsevent`
  does not import `gps/app/stream`), so only `time/app/daemon` sees both the pull
  setup and the dispatcher. Construction order already supports this:
  `cfg.Stream.Pull.Prepare` (daemon.go:231) runs before `NewDispatcher`
  (daemon.go:318), so the daemon can build `nmeasyn.New(pullSetup.Sink())` and
  inject it. The sink bridges into the `ggaSender`; because Go satisfies
  interfaces implicitly, `gps/app/stream` only imports `gps/lib/nmeamsg` (to name
  `nmeamsg.Sentence[nmeamsg.GGAFields]`), never `gps/nmeasyn` - so no import
  cycle and `stream` stays unaware of `nmeasyn`. The daemon also decides
  *whether* to build it at all (no pull / no NMEA consumer -> no sink -> no
  handler). Remaining choice: have
  `NewDispatcher` grow an `extraHandlers ...gpsprot.MsgHandler` parameter
  (preferred) vs. assembling the `MultiHandler` in the daemon. `PullSetup` must
  gain a method exposing the sink.
- Significant-move threshold value, and whether it is configurable.
- Confirm PointPerfect accepts the Ntrip v1 request, or whether it requires v2
  (`Ntrip-Version: Ntrip/2.0`, HTTP/1.1 status line). We currently only accept
  `ICY 200 OK`. If v2 is required, that is separate work.

## Testing

- Stage 1: `validateGGA` accepts good sentences and rejects bad-checksum /
  non-GGA / non-NMEA input; the GGA is sent as a separate write after the
  handshake (`ICY 200 OK`), not in the request.
- Stage 2: golden sentences, padding, `LatLon` round-trip, encode/decode
  round-trip (builder); `nmeasyn` builds the expected GGA from a synthetic epoch
  (fix level -> quality, DOP -> HDOP, used SVs -> NumSats, no-fix -> quality 0),
  ECEF-only epoch exercises the conversion.
- Stage 3: `ggaSender` - force-send on connect, send on significant 2D move,
  silence when stationary or quality 0, write-deadline / write-error handling,
  reconnect re-send; config parsing.
