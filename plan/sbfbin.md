# SBF decode layer and packet scanner (#340)

Core sub-plan of #340 (`plan/septentrio-core.md`); no separate issue.

This is a sub-plan of **`plan/septentrio-core.md`**; it is a checkbox in
that issue, not a separate issue. It covers `gps/lib/sbfbin` (the pure
wire-format codec) in full, plus the `gpsprot.PacketFormat` scanner and
a do-nothing `PacketProcessor` that routes every decoded block to
`NativeMsgHandler`. Block-to-`gpsprot` mapping, epoch handling, and the
ASCII config channel are sibling sub-plans, out of scope here. This is
the foundation and depends on nothing else.

## Problem and scope

Septentrio receivers (mosaic-X5, mosaic-G5) output GNSS data as SBF
(Septentrio Binary Format), a length-prefixed, CRC-checked binary
protocol with ~50 distinct block types. Before any of that data can
reach `gpsprot` (position, velocity, satellites, time, corrections),
satpulse needs:

1. A wire-format codec (`gps/lib/sbfbin`) that decodes/encodes SBF
   blocks to/from typed Go structs -- framing, CRC, revision handling,
   and every block struct needed for the target receiver's output.
2. A `gpsprot.PacketFormat` (`gps/internal/septentrio`) that frames
   `$@`-prefixed SBF packets out of a raw byte stream, so the generic
   scanner (`gps/scan`) and tools (`satpulsetool scan/decode/annotate`,
   `gpsdecode`) can find and validate SBF packets.
3. Registration in `gps/gpsreg` and `gps/gpsdecode` so vendor-agnostic
   tooling recognizes SBF at all, plus a `gpsprot.PacketProcessor` that
   at minimum forwards every decoded block to `NativeMsgHandler` (so
   `satpulsetool annotate` and any native-message consumer works even
   before real `gpsprot` mapping exists).

Milestone: `satpulsetool scan`, `decode`, and `annotate` work end to
end on a real or synthetic mosaic-G5 SBF capture, with every in-scope
block decoding to a typed struct and appearing in JSON output -- and
with nothing yet mapped to `gpsprot.Msg`. Conversion, epoch handling,
and SVID/SignalID mapping are `plan/septentrio-msg.md`; RINEX is
`plan/sbf-rinex.md`.

**Target receiver is the mosaic-G5.** The full block catalog below is
scoped to what a G5 can actually emit; see "Blocks excluded for G5" for
what is deliberately left out and why. G5-vs-X5 differences that affect
this layer are called out inline.

## The design

### Package layout

`gps/lib/sbfbin`, modeled on `gps/lib/casbin`/`gps/lib/ubxbin` but
diverging where SBF's own framing rules require it (see "How SBF
differs from casbin/ubxbin" below):

- `common.go`: `Sync1`/`Sync2` byte constants (`0x24`/`0x40`, i.e. `$@`);
  `HeaderLen`/`PacketMinLen` constants; the `MsgID` type and its
  `Unpack()` (13-bit block number + 3-bit revision); the `TimeStamp`
  struct (`TOW`/`WNc`) and its `Epoch()`; the `Params` interface
  (`BlockNumber() uint16`); the `Block` struct (`Rev`/`TimeStamp`/
  `Params`) and its `ID()`; the `Chunked` interface plus its
  `ReadBinChunked`/`WriteBinChunked` drivers -- the binary half of
  `novmsg`'s `Chunk` abstraction, copied here and tweaked (see "Message
  model" below); the `UnknownParams` fallback; the `regBlock[T, PT]`/
  `blockMap`/`idNameMap` registry: each family file's `init()` populates
  both (mirroring `casbin`/`ubxbin`'s `regMsg`), while `other.go`
  populates `idNameMap` alone for the undecoded blocks (see below).
- `crc.go`: table-driven CRC-16/CCITT (poly `0x1021`, seed `0`, no
  input/output reflection, MSB-first), table-driven in the style of
  `gps/lib/novmsg/crc.go` (structure only -- novmsg's is a CRC-32, a
  different algorithm), exposing `CRC16(data []byte) uint16` for both
  `sbfbin` itself and the packet-format scanner in
  `gps/internal/septentrio` to share. (The exact CRC-16/CCITT parameters
  already exist as `spartnbin`'s unexported, bit-wise `crcMSB`; a fresh
  table-driven implementation here is warranted rather than
  exporting/relocating that.)
- Entry points: `ParseMsg(packet string) (*Block, error)`,
  `Serialize(b *Block) ([]byte, error)`, `PackMsg(mid MsgID, payload
  []byte) ([]byte, error)`, `PacketMsgID[B ~string | ~[]byte](packet B)
  MsgID` (extracts and masks the ID field without a full parse, mirroring
  `casbin.PacketMsgID` -- same capitalisation and `~` constraint; `ubxbin`
  spells it `PacketMsgId`, without the tilde). No `Poll`: unlike UBX/CASIC
  (where `Poll(mid)` emits an empty-payload block to request it), SBF has
  no binary poll -- output is driven by the ASCII
  `setSBFOutput`/`exeSBFOnce` commands (a sibling plan's config channel),
  so an empty SBF block would request nothing.
- Family files split by SBF group, matching the guide's own section
  numbering: `pvt.go` (PVTCartesian/PVTGeodetic/EndOfPVT/the four Cov
  blocks/DOP), `meas.go` (MeasEpoch/MeasExtra/EndOfMeas), `sat.go`
  (ChannelStatus/SatVisibility), `status.go` (ReceiverStatus/RFStatus/
  QualityInd/GALAuthStatus), `time.go` (ReceiverTime/xPPSOffset),
  `corr.go` (DiffCorrIn/BaseStation), `setup.go` (ReceiverSetup),
  and `utc.go` for the three leap-second blocks (GPSUtc/GALUtc/BDSUtc,
  the only nav-message blocks in scope -- see "Blocks excluded" for why
  the rest of the nav-message family is not decoded). Each struct field
  gets a JSON tag naming it **exactly as the Septentrio guide names it**
  (`TOW`, `WNc`, `RxClkBias`, `Mode`, `SVID`, `SBLength`, ...), so the
  JSON that `gps/gpsdecode` and `internal/decodecmd` emit (they marshal
  the decoded struct straight to JSON as the `DecodeResult.Payload`) is
  directly cross-referenceable against the guide's block tables. Enum
  and bitfield fields get named types plus exported constants, but --
  following `ubxbin`/`casbin` -- **no `String()`/`MarshalText`
  methods**: they serialize as their raw numeric code, which is exactly
  what the guide's tables list (the only `String()` in those packages
  is on `MsgID`).
- `other.go`: no decode structs -- the name-only registry for blocks
  `sbfbin` recognizes but does not decode (the "Blocks excluded for the
  G5 target" set below). Its `init()` defines each such block's number
  as a named constant and registers just its name in `idNameMap`,
  exactly as `casbin`/`sdbpbin`'s `other.go` and `ubxbin`'s `msg.go` do
  for their unimplemented messages. This lets `MsgID.String()` -- and
  thus `satpulsetool scan`/`annotate` -- name a block a G5 emits but this
  phase does not decode (e.g. `GPSNav`) instead of printing a bare block
  number.
- **Data-centric, light on methods.** Like `ubxbin`/`casbin`, the
  per-block `Params` structs expose their fields directly and `sbfbin`
  exports almost no methods. The method surface is limited to interface
  conformance (`Params.BlockNumber()`, and `Chunks()` on the
  variable-length blocks), `Block`'s `ID()`/`Epoch()`, `MsgID` helpers
  (`Unpack`/`String`), and a *few* small accessors where a consumer needs
  a derived value (pulling a sub-field out of a packed bitfield). Do not
  add getters/setters that merely wrap ordinary field access.
- Layering (`docs/internals.md`): `gps/lib/sbfbin` imports only stdlib
  and sibling `gps/lib/*` packages -- it must not import `gpsprot`,
  `gps/scan`, or `gps/internal/*`. Consequence for this phase: struct
  fields are raw wire values (`uint8` SVID, `uint8` signal number, etc.),
  never `gpsprot.SVID`/`gpsprot.SignalID`/`gpsprot.GNSS` -- `sbfbin`
  cannot import `gpsprot`, so mapping a raw SVID/signal number to those
  is the phase-3 conversion layer's job (the 4.1.9/4.1.10 tables below
  are its reference). Mirroring `ubxbin.RINEXSatNum`/`RINEXSig` (which
  return a plain `uint8`/`string` and do **not** import `rinex`),
  `sbfbin` owns the raw-SVID-to-RINEX-number and signal-number-to-RINEX-
  code mappings, returning plain integers/strings as the single
  wire-semantics source `rnxsbf` (`plan/sbf-rinex.md`) uses; wrapping
  those into `rinex.SatelliteID`/typed codes stays in `rnxsbf`, exactly
  as `rnxubx` does over `ubxbin`. (`sbfbin` *may* import `gps/lib/rinex`
  per the layering, but following the `ubxbin` precedent it need not.)
- **`sbfbin` is the sole owner of SBF wire-format knowledge.** Every
  wire constant the later conversion package (`gps/internal/septentrio`,
  phase 3) needs -- block numbers, enum codes (`Mode`, `Error`,
  `TimeSystem`, `Datum`, `WACorrInfo`, `Timescale`, signal numbers,
  SVID ranges, ...), DNU sentinels, and bitfield masks/shifts -- is
  defined as an **exported named constant (or method) here**. The
  conversion package must contain **no magic wire numbers**: it refers
  to `sbfbin` constants only. If phase 3 finds it needs a raw value that
  `sbfbin` doesn't export, the fix is to export it from `sbfbin`, not to
  inline a literal downstream.
- Add an entry to `docs/internals.md` (`### gps/lib/` and
  `### gps/internal/` sections) for `gps/lib/sbfbin` and
  `gps/internal/septentrio` when this phase lands, per `CLAUDE.md`.

### How SBF differs from casbin/ubxbin

Three real differences from the existing `lib/*bin` packages, each
driving a concrete design choice:

**1. Trailing bytes are never an error, for every message, not opt-in.**
`casbin` has an `AllowTrailing` marker interface that a few message
types implement to skip the trailing-bytes check; `ubxbin` has no such
escape at all (trailing bytes are always an error there). SBF's own
revision rule (section 4.1.6 of the reference guide: a block's revision
number increments only when a *new field is appended* in space that was
previously padding; existing fields never move or shrink) means *every*
SBF block must tolerate trailing bytes it doesn't recognize -- there is
no block for which "reject unknown trailing bytes" is the right
behavior. So `sbfbin.ParseMsg` has **no trailing-bytes check at all**
(no `AllowTrailing` interface is needed): after decoding as many known
fields as the payload allows, whatever remains up to the block's
`Length` is treated as reserved/padding/newer-revision data and
silently discarded.

**2. `Length` is authoritative, not `sizeof(struct)`.** Validate
`Length >= knownMinSize` for the message's minimum (revision-0) shape,
then trust `Length` for how far the block extends; never require an
exact match against a compile-time struct size.

**3. Sub-blocks carry a runtime length (`SBLength`), and blocks grow by
revision -- both handled by one copied abstraction.** Unlike `casbin`'s
`VaryingPart`/`sliceLen` (a fixed compile-time element size), an SBF
sub-block can be wider on the wire than its known fields when a later
revision appends fields inside it, and a top-level block gains a
revision trailer the same way. Rather than invent per-shape machinery,
`sbfbin` copies the binary half of `novmsg`'s `Chunked` abstraction and
tweaks it for these two cases (see "Message model" below): a decoder
advances by the wire `SBLength` via an explicit padding piece (never by
`binary.Size` of the known fields), and a short older-revision stream
is tolerated, leaving unread fields at their DNU.

### Message model: block, chunks, decode/encode

**A decoded block is a `Block`: the revision and time stamp common to
every SBF block, plus the block-specific parameter set.** This is the
`novmsg`/`uncmsg` `Msg{Hdr, Body}` split (a common header vs. the
per-message wire struct), flattened into one struct using SBF's own
terms -- the "SBF Block Time Stamp" (guide 4.1.3) and the block
"Parameters":

```go
// TimeStamp is the SBF Block Time Stamp (guide 4.1.3): the TOW/WNc that
// follow the 8-byte block header on every SBF block.
type TimeStamp struct {
	TOW uint32 `json:"TOW"` // ms of GPS week, DNU 0xFFFFFFFF
	WNc uint16 `json:"WNc"` // continuous GPS week, DNU 0xFFFF
}

// Params is a block's parameter set (the fields after the time stamp);
// it names its block number. One struct per block type implements it.
type Params interface {
	BlockNumber() uint16
}

// Block is a decoded SBF block: its revision (from the block header's
// ID), its time stamp, and its parameters.
type Block struct {
	Rev uint8 `json:"-"` // revision (ID>>13); from the frame ID, not a wire field of the body
	TimeStamp            // TOW, WNc -- on the wire immediately after the block header
	Params Params        // block-specific parameters; the only per-block wire struct
}

func (b *Block) ID() MsgID               { return MsgID(b.Params.BlockNumber()) | MsgID(b.Rev)<<13 }
func (b *Block) Epoch() (uint32, uint16) { return b.TOW, b.WNc }
```

A concrete block is then just its parameters plus a one-line
`BlockNumber()` (the analogue of `nmeamsg.Sentence`'s per-field-set
`SentenceFormat()`). There is no per-block wrapper type: a block is
identified by asserting `b.Params` to the concrete `Params` type (as
`novmsg` callers assert `msg.Body`):

```go
type PVTGeodetic struct { // a Params; b.Params.(*PVTGeodetic) recovers it
	Mode  Mode
	Error ErrCode
	// ... parameters after the time stamp
}
func (*PVTGeodetic) BlockNumber() uint16 { return 4007 }
```

Only `TimeStamp` and the concrete `Params` value are ever on the wire.
`ParseMsg` masks the frame `ID` into block number + `Rev`, then over a
reader on the payload does one `binary.Read(r, &b.TimeStamp)` followed
by `ReadBinChunked(r, b.Params, name)` (into the interface's
`*PVTGeodetic` etc., exactly as `novmsg` reads its `MsgBody`); it
*assigns* `b.Rev` from the ID, so `binary` never sees `Rev` or the
`Params` interface. That is precisely why the frame-ID revision can live
in the struct without corrupting the `binary` layout -- it is assigned,
not decoded, mirroring how `novmsg`/`uncmsg` keep header metadata out of
the body struct. `Serialize` reverses it -- `binary.Write(&b.TimeStamp)`,
`WriteBinChunked(b.Params)`, then `PackMsg(b.ID(), payload)` frames it
(padding to a multiple of 4, writing the ID with `Rev<<13`, computing
the CRC). `ID()`/`Epoch()` are defined once on `Block`. The unknown-block
fallback is a `*UnknownParams` (a `Number`+`Payload` `Params`, mirroring
`novmsg.UnknownBinMsgBody`), special-cased in `ParseMsg`/`Serialize`
rather than run through `binary`.

`Rev` is what makes decode revision-aware and lets encode set the
revision bits in the emitted `ID`; decode itself is `Length`-driven (the
guide's authoritative rule, see "How SBF differs"), reading the
parameters the payload has room for and tolerating both older-revision
senders (trailing parameters stay at their pre-set DNU) and
newer-revision senders (extra bytes ignored). Round-trip preserves the
parsed `Rev`; whether `Serialize` re-emits an older-than-current
revision at exactly that revision's length or the current full length is
an encode-fidelity detail (see "Open decisions") -- value-level
round-trip holds either way, since decode is `Length`-authoritative.

The per-block field tables in "The block catalog" below describe the
full wire layout (`TOW`/`WNc` at offsets 0/4, then the parameters); the
`TimeStamp` covers `TOW`/`WNc` and each block's `Params` struct holds
the parameters from offset 6 onward. `Epoch()` hands the phase-3
conversion layer the `(TOW, WNc)` epoch key with no per-block
boilerplate (the analogue of `NavITOW.NavEpoch()`).

**`Chunked`: one mechanism for every variable-length block.** Copied
(binary half only) from `novmsg`'s `chunk.go` into `common.go`; the
ASCII/`fieldenc` half is dropped (SBF is binary-only):

```go
// Chunked is implemented by the Params structs whose wire layout is not
// a single fixed struct. Chunks yields each piece -- a struct pointer or
// a slice-element pointer -- in wire order; a piece yielded later may
// depend on a field read into an earlier piece (e.g. a count or an
// SBLength).
type Chunked interface {
	Chunks() func(yield func(chunk any) bool)
}
```

`ReadBinChunked(r, p)` / `WriteBinChunked(w, p)` drive it over a `Params`
value `p`: if `p` is `Chunked`, iterate its pieces and
`binary.Read`/`binary.Write` each against the shared stream; otherwise a
single `binary.Read`/`Write` of the whole `Params` struct. **The same
iterator drives decode and encode, so there is no separate encode path.**
The `TimeStamp` is read/written by `Block` itself, before/after the
`Params`, so the `Params` structs -- and their `Chunks()` -- cover only
the fields after the time stamp.

Two SBF-specific tweaks vs. the `novmsg` original:
- **Tolerant end-of-stream (revision trailers).** `ReadBinChunked`
  stops cleanly when the stream is exhausted at a piece boundary,
  leaving the not-yet-read pieces at whatever the struct was
  pre-initialised to. So an older-revision packet lacking its trailer
  is not an error -- those fields keep the Do-Not-Use sentinels they
  were pre-set to before decode (DNU is not always the Go zero: e.g.
  `HAccuracy`/`Latency` DNU is `65535`). Since `blockMap` constructs a
  zero-valued `Params`, a `Params` with a non-zero-DNU trailer field
  implements an optional `defaulter` interface (`setDNUDefaults()`) that
  the `regBlock` constructor calls on the freshly-`new`'d value, so the
  seeds are in place before `ReadBinChunked` runs and the read overwrites
  only the fields actually present. Exactly three blocks need it: the two
  Shape-2 PVT blocks (`Latency`/`HAccuracy`/`VAccuracy` -> `65535`) and
  `ReceiverSetup` (`Latitude`/`Longitude`/`Height` -> `-2e10`); every
  other absent-field case (Shape-1 has none; older Shape-3/4 sub-block
  fields are all zero-DNU or unused-when-zero) is correct at Go zero.
  Newer-
  revision extra bytes are ignored (the universal trailing-bytes rule).
- **Runtime `SBLength` padding.** A sub-block may be wider on
  the wire than its known fields (a later revision appended fields
  inside it). `Chunks()` yields a `[]byte` padding piece of `SBLength -
  binary.Size(sub)` after each sub-block, so `binary.Read`/`Write`
  consume/emit the full sub-block length. `SBLength` is in the head piece,
  already read, so the padding size is known when the padding piece is
  yielded.

One `Chunked` interface covers everything variable-length, so `sbfbin`
needs no per-shape marker interfaces (unlike `ubxbin`/`casbin`'s
`VaryingMsg`); a flat block's `Params` struct implements nothing extra.

**The shapes, in `Chunked` terms** (the catalog tags each block with
its shape):

- **Shape 1 -- flat fixed.** `EndOfPVT`, `EndOfMeas`, `DOP`, the four
  Cov blocks, `GALAuthStatus`, `xPPSOffset`, `ReceiverTime`,
  `BaseStation`, `GPSUtc`, `GALUtc`, `BDSUtc`. The `Params` struct is not
  `Chunked`; `ReadBinChunked` does one `binary.Read` into the whole
  `Params` struct (the `TimeStamp` having already been read by `Block`).
  No revision-gated fields.
- **Shape 2 -- fixed + revision trailer.** `PVTCartesian`,
  `PVTGeodetic` (Rev1 `PPPInfo`/`Latency`, Rev2 `HAccuracy`/
  `VAccuracy`/`Misc`), `ReceiverSetup` (Rev1-4). `Chunks()` yields the
  fixed part, then one piece per revision group of trailer fields;
  tolerant EOF handles older-revision senders (trailer fields stay at
  their pre-set DNU).
- **Shape 3 -- count-prefixed array / opaque tail.** `QualityInd`
  (`Indicators u2[N]`): yield the head (carrying `N`), then
  `make([]uint16, N)` and each element. `DiffCorrIn`: yield the head,
  then a single `[]byte` of `Length-16` for the opaque correction
  bytes.
- **Shape 4 -- variable-length (`SBLength`) sub-blocks, one or two
  levels.**
  `SatVisibility`/`RFStatus`/`ReceiverStatus`/`MeasExtra` (one level);
  `MeasEpoch`/`ChannelStatus` (two levels). `Chunks()` yields the head
  (carrying `N`/`SBLength`), then for each sub-block its struct piece
  followed by its `SBLength` padding piece. Two-level blocks nest: for
  each outer element yield it (reading its `N2`), then its `N2` inner
  sub-blocks (each with its own `SB2Length` padding). Because later
  pieces see earlier-read fields, the data-dependent `N2` and
  per-element sub-block lengths fall out naturally, with no cursor
  bookkeeping.
  `MeasExtra`'s `N` is modulo-256: recover the true count `NrSB =
  ((Length/SBLength - N)/256)*256 + N` before yielding the sub-blocks.

### SBF wire format essentials

8-byte header, little-endian: `Sync1`(`0x24`='$') `Sync2`(`0x40`='@')
`CRC`(u2) `ID`(u2) `Length`(u2). Body follows immediately; almost every
block starts the body with `TOW`(u4, ms) + `WNc`(u2, continuous
GPS-convention week), covered generically below rather than per block.

- **ID split**: `blockNumber = ID & 0x1FFF` (13 bits), `revision = ID >>
  13` (3 bits). Key every dispatch off block number; revision only ever
  gates optional trailing fields (Shape 2) or a larger sub-block
  `SBLength` (Shape 4), never a change to an existing field's meaning or
  position.
- **Length**: total bytes including the 8-byte header, always a
  multiple of 4. Authoritative; never require an exact match to a
  compile-time size (see "How SBF differs" above).
- **CRC-16/CCITT** (poly `0x1021`, seed 0, no reflection, no final XOR,
  MSB-first): covers `pkt[4:Length]` (ID through end of block),
  excluding the 2-byte Sync and the 2-byte CRC field itself.
- **TOW/WNc convention**: `TOW` is always milliseconds since the start
  of the current GPS week (DNU `0xFFFFFFFF`); `WNc` is always the
  continuous **GPS-convention** week number (DNU `0xFFFF`), even for
  blocks carrying non-GPS constellation data. Never reinterpret a
  block's own `WNc` header through a different constellation's epoch;
  only a block's *own* native week fields (e.g. the `*Utc` blocks'
  `WN_LSF`) are constellation-native.
- **Time-stamp categories** (informational, drives whether a block
  belongs in epoch/time-tracking logic in a later phase, not decoded as
  a field): **Receiver** (synchronous, non-decreasing once
  GNSS-aligned: PVT/status/measurement blocks), and **SIS**
  (signal-in-space; timestamp is when the satellite transmitted the
  data, no ordering guarantee relative to stream position: the `*Utc`
  leap-second blocks). No in-scope block carries an External/event
  timestamp.
- **Do-Not-Use (DNU) convention**: per-field, not per-wire-type. Common
  sentinels recur throughout (`u4` TOW `0xFFFFFFFF`; `u2` WNc `0xFFFF`;
  `f4`/`f8` position/clock-type fields `-2e10` exactly; `i1` UTC
  components and `DeltaLS`-style fields `-128`; various `u1`/`u2`
  `255`/`65535`, sometimes `0`) but each block's field table below
  states its own; do not infer DNU from wire type alone, and do not
  assume `0` is always a sentinel -- for bitfields (`WACorrInfo`,
  `AlertFlag`, `PPPInfo`, `Misc`, `SignalInfo`) `0` is a legitimate
  "no bits set" state, and for count fields (`NrSV`, `NrBases`) `0` is a
  legitimate "none" count, not a sentinel, except where a field's own
  table below says otherwise (e.g. `DOP`'s `PDOP`/`TDOP`/`HDOP`/`VDOP`,
  where `0` genuinely means "not available", unlike the bitfields in the
  very same block). Clipping (e.g. `LockTime` clipping at 65534,
  `HAccuracy`/`VAccuracy` clipping at 65534) is distinct from DNU
  (65535) -- do not conflate a clipped-but-valid reading with "absent".

### Shared PVT-family enumerations

`Mode`, `Error`, `TimeSystem`, `Datum`, `WACorrInfo`, `AlertFlag`,
`PPPInfo`, and `Misc` are byte-for-byte identical across
`PVTCartesian` and `PVTGeodetic`, and (`Mode`/`Error` only) the four
Cov blocks. Decode each with one shared Go type/constant set, not one
copy per block.

**`Mode`** (u1 bitfield): bits 0-3 = PVT-solution type (`0`=No GNSS PVT,
see `Error`; `1`=Stand-alone; `2`=Differential; `3`=Fixed location;
`4`=RTK fixed; `5`=RTK float; `6`=SBAS-aided; `7`=moving-base RTK fixed;
`8`=moving-base RTK float; `9`=Reserved; `10`=PPP; `11`=undefined gap,
treat as reserved/forward-compatible unknown; `12`=Reserved); bits 4-5
reserved; bit 6 = static/auto-survey converging; bit 7 = 2D/3D flag (set
= 2D, height held constant).

**`Error`** (u1 enum, meaningful only when `Mode` bits 0-3 == 0): `0`
no error, `1` not enough measurements, `2` not enough ephemerides, `3`
DOP too large (>15), `4` sum of squared residuals too large, `5` no
convergence, `6` not enough measurements after outlier rejection, `7`
export-law prohibition, `8` not enough differential corrections, `9`
base-station coordinates unavailable, `10` ambiguities not fixed
(RTK-fixed-only requested).

**`TimeSystem`** (u1 enum, DNU `255`): `0` GPS, `1` Galileo (GST), `3`
GLONASS, `4` BeiDou, `5` QZSS, `100` FugroAtomiChron (proprietary
corrections-clock reference, not a GNSS). Value `2` unassigned. Do not
confuse with `xPPSOffset.TimeScale`, a differently-numbered enum on a
different block that happens to reuse the value `3` for "Receiver time"
instead of GLONASS.

**`Datum`** (u1 enum, DNU `255`): `0` WGS84/ITRS, `19` same as DGNSS/RTK
base, `30` ETRS89, `31`-`33` NAD83 variants, `34`/`35` GDA94/GDA2020,
`36` JGD2011, `250`/`251` user-defined.

**`WACorrInfo`** (u1 bitfield, `0` = legitimate "no bits set", not a
sentinel): bit 0 orbit/clock correction used, bit 1 range correction
used, bit 2 ionospheric info used, bit 3 orbit-accuracy info used, bit 4
DO229 Precision Approach active, bits 5-6 differential-correction type
(`0` unknown/none, `1` physical base, `2` virtual base/VRS, `3` SSR),
bit 7 reserved.

**`AlertFlag`** (u1 bitfield, `0` = legitimate default): bits 0-1 RAIM
integrity (`0` not monitored, `1` success, `2` failed, `3` reserved),
bit 2 Galileo HPCA failure, bit 3 Galileo ionospheric storm flag, bit 4
reserved. Bits 5-6 are reserved on the mosaic-X5 firmware line; on
mosaic-G5 (Rev3) they are `SIG_AUTH_ALERT` (bit 5) and
`NAV_MSG_AUTH_ALERT` (bit 6) -- OSNMA/spoofing-detection flags, mirroring
the identical bits already present in `RFStatus.Flags` (bit 0/1). Decode
the raw bits unconditionally on the mosaic-G5 target; only the *named*
G5 semantics require checking the revision (`ID >> 13 >= 3`) or model.
Bit 7 reserved.

**`PPPInfo`** (u2 bitfield, Rev1, `0` = legitimate default): bits 0-11
age of last PPP seed in seconds, clipped at 4091, ignore when seed type
is 0; bit 12 reserved; bits 13-15 seed type (`0` not seeded/not PPP,
`1` manual, `2` from DGNSS, `3` from RTK fixed).

**`Misc`** (u1 bitfield, Rev2): bit 0 baseline-to-ARP vs phase-center
(DGNSS/RTK), bit 1 phase-center offset compensated at rover, bits 2-5
proprietary, bits 6-7 whether the marker position equals the ARP
position (`0` unknown, `1` offset is zero, `2` offset is nonzero).

### Satellite ID table (section 4.1.9, reference for a later phase)

Every SBF `SVID`/`PRN` field (whether a global cross-block `SVID` or a
per-family in-constellation index -- see per-block notes) is drawn from
this table. Recorded here in full since it governs how wide an integer
type and what range-check each per-block `SVID`/`PRN` field needs, even
though `sbfbin` does not itself classify a `SVID` into a `gpsprot.GNSS`.

| SVID range | Meaning | RINEX |
|---|---|---|
| 0 | Do-Not-Use | -- |
| 1-37 | GPS PRN | G01-G37 |
| 38-61 | GLONASS slot, offset 37 | R01-R24 |
| 62 | GLONASS, slot unknown | -- |
| 63-68 | GLONASS slot, offset 38 | R25-R30 |
| 71-106 | Galileo PRN, offset 70 | E01-E36 |
| 107-119 | L-band (MSS) satellite (name in `LBandBeams`, X5-only) | -- |
| 120-140 | SBAS PRN, offset 100 | S20-S40 |
| 141-180 | BeiDou PRN, offset 140 | C01-C40 |
| 181-190 | QZSS PRN, offset 180 | J01-J10 |
| 191-197 | NavIC/IRNSS PRN, offset 190 | I01-I07 |
| 198-215 | SBAS PRN, offset **157** | S41-S58 |
| 216-222 | NavIC/IRNSS PRN, offset 208 | I08-I14 |
| 223-245 | BeiDou PRN, offset 182 | C41-C63 |
| 250-251 | GPS PRN, offset 212 | G38-G39 |

(The 198-215 SBAS row's offset is **157**, giving RINEX S41-S58 via
`nn = SVID - 157`.)

The satellite blocks in scope (`ChannelStatus`, `MeasEpoch`,
`SatVisibility`) use the global offset-`SVID` convention above; a later
phase's conversion layer maps it to `gpsprot.SVID`. The `*Utc` blocks
carry a source-satellite `PRN`/`SVID` that is purely informational
(which SV's page supplied the leap-second parameters) and needs no
classification in this phase.

### Signal-number table (section 4.1.10, reference for a later phase)

Used by `MeasEpoch`/`MeasExtra`'s `Type.SigIdxLo` (+`ObsInfo`/`Misc`
extension when `SigIdxLo==31`), and by `SignalInfo` on the PVT-family
blocks (bits 0-31 only; signal numbers 32-39 cannot be represented in
that `u4` bitmask -- there is no `SignalInfo2` extension).

| # | Signal | GNSS | # | Signal | GNSS |
|---|---|---|---|---|---|
| 0 | L1CA | GPS | 20 | E5a | Galileo |
| 1 | L1P | GPS | 21 | E5b | Galileo |
| 2 | L2P | GPS | 22 | E5AltBOC | Galileo |
| 3 | L2C | GPS | 23 | L-band | MSS (not a GNSS signal) |
| 4 | L5 | GPS | 24 | L1CA | SBAS |
| 5 | L1C | GPS | 25 | L5 | SBAS |
| 6 | L1CA | QZSS | 26 | L5 | QZSS |
| 7 | L2C | QZSS | 27 | L6 | QZSS |
| 8 | L1CA | GLONASS | 28 | B1I | BeiDou |
| 9 | L1P | GLONASS | 29 | B2I | BeiDou |
| 10 | L2P | GLONASS | 30 | B3I | BeiDou |
| 11 | L2CA | GLONASS | 31 | (extension escape, see below) | -- |
| 12 | L3 | GLONASS | 32 | L1C | QZSS |
| 13 | B1C | BeiDou | 33 | L1S | QZSS |
| 14 | B2a | BeiDou | 34 | B2b | BeiDou |
| 15 | L5 | NavIC/IRNSS | 35-36 | reserved | -- |
| 16 | reserved | -- | 37 | L1 | NavIC/IRNSS |
| 17 | E1 | Galileo | 38 | L1CB | QZSS |
| 18 | reserved | -- | 39 | L5S | QZSS |
| 19 | E6 | Galileo (E6B when `MeasEpoch.CommonFlags` bit 6 set, else E6C) | | | |

`SigIdxLo == 31` is not a signal, it means "actual signal number is in
`ObsInfo`/`Misc` bits 3-7 with an offset of 32" (i.e. numbers 32-39,
which are only representable this way, never in a 5-bit `SigIdxLo`).
GLONASS `FreqNr` (frequency channel, offset 8: raw 1..14 = actual
freq -7..+6) is carried in `ObsInfo`/`Misc` bits 3-7 for `MeasEpoch`
when `SigIdxLo` is one of the four GLONASS signal numbers (8-11), not
in a dedicated field -- `MeasEpoch`'s sub-blocks have no `FreqNr` field
of their own, unlike `ChannelStatus`/`SatVisibility`.
No dedicated `gps/gpsprot/signal.go` constant exists yet for GLONASS L3,
SBAS L1/L5, or QZSS L1CB/L5S -- flagged here for whoever writes the
phase-3 mapping, not resolved in this phase.

## The block catalog

Organized by family. Every block states: number, revision(s), decode
shape (per the four shapes above), default rate/timestamp type (context
for a later phase, not a decoded field), and field table. Blocks whose
field list is byte-identical to an already-tabulated sibling are given
only their differences.

### Position/velocity/time (PVT)

**`PVTCartesian` (4006)** and **`PVTGeodetic` (4007)** -- Shape 2.
Rev0 through `AlertFlag`/`NrBases`; Rev1 appends `PPPInfo`+`Latency`;
Rev2 appends `HAccuracy`+`VAccuracy`+`Misc` (96 bytes total at Rev2, the
revision every real capture and the current guide edition shows).
`OnChange` at the default PVT rate; Receiver timestamp.

| Offset | Field | Wire | Units/scale | DNU | PVTCartesian | PVTGeodetic |
|---|---|---|---|---|---|---|
| 0 | TOW | u4 | 0.001 s | `0xFFFFFFFF` | | |
| 4 | WNc | u2 | 1 week | `0xFFFF` | | |
| 6 | Mode | u1 | bitfield | -- | shared enum above | |
| 7 | Error | u1 | enum | -- | shared enum above | |
| 8 | X / Latitude | f8 | 1 m / 1 rad | `-2e10` | ECEF X | lat, -pi/2..+pi/2 N+ |
| 16 | Y / Longitude | f8 | 1 m / 1 rad | `-2e10` | ECEF Y | lon, -pi..+pi E+ |
| 24 | Z / Height | f8 | 1 m | `-2e10` | ECEF Z | ellipsoidal height |
| 32 | Undulation | f4 | 1 m | `-2e10` | | |
| 36 | Vx / Vn | f4 | 1 m/s | `-2e10` | | local-level North |
| 40 | Vy / Ve | f4 | 1 m/s | `-2e10` | | local-level East |
| 44 | Vz / Vu | f4 | 1 m/s | `-2e10` | | local-level Up |
| 48 | COG | f4 | 1 deg | `-2e10` | course over ground, 0-360 E of North; also DNU when speed < 0.1 m/s (two causes, one sentinel) | |
| 52 | RxClkBias | f8 | 1 ms | `-2e10` | `tsys = trx - RxClkBias`, relative to `TimeSystem` | |
| 60 | RxClkDrift | f4 | 1 ppm | `-2e10` | positive = clock runs fast | |
| 64 | TimeSystem | u1 | enum | `255` | shared enum above | |
| 65 | Datum | u1 | enum | `255` | shared enum above | |
| 66 | NrSV | u1 | count | `255` | total SVs used | |
| 67 | WACorrInfo | u1 | bitfield | `0` (not sentinel) | shared enum above | |
| 68 | ReferenceID | u2 | id | `65535` | base-station ID or SBAS PRN 120-158; `65534`="multiple" | |
| 70 | MeanCorrAge | u2 | 0.01 s | `65535` | | |
| 72 | SignalInfo | u4 | bitmask | `0` (not sentinel) | bit i = signal i (4.1.10) used, bits 0-31 only | |
| 76 | AlertFlag | u1 | bitfield | `0` (not sentinel) | shared enum above | |
| 77 | NrBases | u1 | count | `0` (not sentinel) | | |
| 78 | PPPInfo | u2 | bitfield, Rev1 | `0` (not sentinel) | shared enum above | |
| 80 | Latency | u2 | 0.0001 s, Rev1 | `65535` | receiver processing time only | |
| 82 | HAccuracy | u2 | 0.01 m, Rev2 | `65535`, clip `65534` | 2DRMS horizontal | |
| 84 | VAccuracy | u2 | 0.01 m, Rev2 | `65535`, clip `65534` | 2-sigma vertical | |
| 86 | Misc | u1 | bitfield, Rev2 | -- | shared enum above | |

`Mode` bits 0-3 == 0 ("No GNSS PVT") implies every field after `Error`
reads DNU. Both blocks may be co-emitted for the same epoch (Cartesian
+ geodetic frame simultaneously); a later phase's dedup logic is out of
scope here.

**`EndOfPVT` (5921)** -- Shape 1. `TOW`+`WNc` only (16 bytes, 2 bytes
padding), pure epoch-flush marker for the whole PVT-family block set
(the guide: "marks the end of transmission of all PVT related blocks
belonging to the same epoch"). Independent of `EndOfMeas` -- do not
assume the two are emitted together or in a fixed relative order.

### Position/velocity covariance

**`PosCovCartesian` (5905)**, **`PosCovGeodetic` (5906)**,
**`VelCovCartesian` (5907)**, **`VelCovGeodetic` (5908)** -- Shape 1,
all four: `TOW`+`WNc`+`Mode`+`Error` (identical enums to the PVT blocks
above) + ten `f4` variance/covariance terms, 56 bytes total, no
padding. `OnChange` at the PVT rate; Receiver timestamp; each requires
its Cartesian/geodetic-frame PVT sibling to be enabled.

| Block | Diagonal terms | Off-diagonal terms |
|---|---|---|
| PosCovCartesian | Cov_xx, Cov_yy, Cov_zz, Cov_bb | Cov_xy, Cov_xz, Cov_xb, Cov_yz, Cov_yb, Cov_zb |
| PosCovGeodetic | Cov_latlat, Cov_lonlon, Cov_hgthgt, Cov_bb | Cov_latlon, Cov_lathgt, Cov_latb, Cov_lonhgt, Cov_lonb, Cov_hb |
| VelCovCartesian | Cov_VxVx, Cov_VyVy, Cov_VzVz, Cov_DtDt | Cov_VxVy, Cov_VxVz, Cov_VxDt, Cov_VyVz, Cov_VyDt, Cov_VzDt |
| VelCovGeodetic | Cov_VnVn, Cov_VeVe, Cov_VuVu, Cov_DtDt | Cov_VnVe, Cov_VnVu, Cov_VnDt, Cov_VeVu, Cov_VeDt, Cov_VuDt |

All ten fields per block are `f4`, DNU `-2e10` exactly (an exact
IEEE-754 value, safe to compare with `==`, no epsilon needed), units
`m^2`/`m^2/s^2` (position/velocity-squared, not standard deviations --
never take a square root before the DNU check). `Error != 0` implies
all ten fields are DNU. **The four blocks' 2D/3D-mode partial-DNU rules
are not symmetric -- do not assume a shared rule:**

- `PosCovCartesian`: 2D mode -> **all ten** fields DNU.
- `PosCovGeodetic`: 2D mode -> only the four height-coupled terms DNU
  (`Cov_hgthgt`, `Cov_lathgt`, `Cov_lonhgt`, `Cov_hb`); the other six
  can remain valid.
- `VelCovCartesian`: 2D mode -> **all ten** fields DNU.
- `VelCovGeodetic`: 2D mode -> only the four up-velocity-coupled terms
  DNU (`Cov_VuVu`, `Cov_VnVu`, `Cov_VeVu`, `Cov_VuDt`); the other six
  can remain valid.

On mosaic-G5 specifically, the six off-diagonal terms of any of these
four blocks may also read DNU unconditionally (independent of 2D/3D
mode) when the receiver's "Integrity" capability is not licensed --
this is a receiver-configuration fact from `getReceiverCapabilities`,
not detectable from the block bytes; a decoder simply observes DNU and
treats it as "unavailable" the same as any other DNU occurrence.

**`DOP` (4001)** -- Shape 1. `TOW`+`WNc`+`NrSV`(u1)+`Reserved`(u1)+
`PDOP`/`TDOP`/`HDOP`/`VDOP`(each u2, x0.01, DNU **`0`** -- a genuine
per-field sentinel here, unlike the bitfields elsewhere in this same
block's siblings)+`HPL`/`VPL`(each f4, 1 m, DNU `-2e10`). 32 bytes, no
padding. Unlike `PosCovCartesian` et al., `DOP` carries no `Mode`/
`Error` at all -- `NrSV == 0` implies all four DOP fields are `0` too.
HPL/VPL's meaning (SBAS-based vs internal-error-model-based) depends on
the sibling PVT block's `Mode` for the same epoch, but `DOP` itself has
no `Mode` field to consult.

### Quality/status

**`QualityInd` (4082)** -- Shape 3. `TOW`+`WNc`+`N`(u1, count)+
`Reserved`(u1)+`Indicators`(`u2[N]`, no `SBLength` -- fixed 2-byte
element). Each `Indicators` word: bits 0-7 indicator type, bits 8-11
value 0 (poor) - 10 (very high), 15 = unknown (per-subfield sentinel,
not per-word), bits 12-15 reserved. Documented types: 0 overall, 1/2
GNSS signals (main/aux1 antenna), 11/12 RF power (main/aux1), 21 CPU
headroom, 25 OCXO stability, 29 scintillation score (higher = less
scintillation), 30 base-station measurements (RTK only), 31 RTK
post-processing likelihood. `OnChange`, fixed 1 s interval (its own
rate, unlike the PVT-riding blocks above).

**`GALAuthStatus` (4245)** -- Shape 1. `TOW`+`WNc`+`OSNMAStatus`(u2
bitfield)+`TrustedTimeDelta`(f4, 1 s, DNU `-2e10`)+`GalActiveMask`(u8,
bit i = PRN i+1)+`GalAuthenticMask`(u8)+`GpsActiveMask`(u8, bit i = GPS
PRN i+1, via OSNMA cross-authentication)+`GpsAuthenticMask`(u8). 52
bytes, no padding. `OSNMAStatus`: bits 0-2 `Status` (0 disabled, 1
initializing, 2 waiting-for-trusted-time, 3-5 init-failed variants, 6
authenticating), bits 3-10 `InitProgress` (percent, meaningful only
when `Status==1`; `255` doubles as an alert/unavailable sentinel), bits
11-13 `TrustedTimeSource` (0 NTP, 1 L-Band, 2 Command, 7 Unknown), bit
14 `MerkleRenewal`, bit 15 reserved. Per satellite mask pair: bit clear
in `*ActiveMask` = no result; bit set in `*ActiveMask` and clear in the
matching `*AuthenticMask` = confirmed non-authentic; both set =
confirmed authentic. `OnChange`, fixed 1 s interval.

**`RFStatus` (4092)** -- Shape 4 (1 level). `TOW`+`WNc`+`N`(u1)+
`SBLength`(u1)+`Flags`(u1 bitfield)+`Reserved`(u1[3])+`RFBand[N]`
(`SBLength` bytes each, 8 at this revision: `Frequency` u4 Hz, `Bandwidth` u2
kHz [0 = short pulsed interference, bandwidth indeterminate], `Info` u1
bitfield, `Power` i1 dBm DNU `0`). `Flags` (block-level): bit 0
`SIG_AUTH_ALERT` (signal authenticity failed, may be set with zero
`RFBand` entries), bit 1 `NAV_MSG_AUTH_ALERT` (NMA failure), bits 2-7
reserved. `RFBand.Info`: bits 0-3 Mode (`1` manual notch, `2`
interference detected+canceled, `8` interference detected, no
mitigation; other values reserved), bits 4-5 reserved, bits 6-7 antenna
ID (0 main, 1 Aux1, 2 Aux2). `OnChange`, fixed 1 s interval; `N==0` is
normal ("nothing to report"), not withheld.

**`ReceiverStatus` (4014)** -- Shape 4 (1 level). `TOW`+`WNc`+
`CPULoad`(u1, %, DNU `255`)+`ExtError`(u1 bitfield, self-clears 1s
after last detection)+`UpTime`(u4, s)+`RxState`(u4 bitfield)+
`RxError`(u4 bitfield, sticky, cleared via `lif` commands)+`N`(u1)+
`SBLength`(u1)+`CmdCount`(u1, wraps 255->1, DNU `0`)+`Temperature`(u1,
degC = raw-100, DNU `0`)+`AGCState[N]` (`SBLength` bytes each, 4
at this revision: `FrontEndID` u1 bitfield [bits 0-4 front-end code 0-14,
bits 5-7 antenna ID], `Gain` i1 dB [DNU `-128` also means "PLL not
locked"], `SampleVar` u1 [nominal 100, DNU `0`], `BlankingStat` u1 %).
Fixed part is 32 bytes. `ExtError` bits: 0 SISERROR, 1 DIFFCORRERROR, 2
EXTSENSORERROR, 3 SETUPERROR, 4-7 reserved. `RxState` bits: 1
ACTIVEANTENNA, 2 EXT_FREQ, 3 EXT_TIME, 4-6 WNSET/TOWSET/FINETIME
(mirrors `ReceiverTime.SyncLevel` bits 0-2), 7-9 internal-disk activity/
full/mounted, 10 INT_ANT, 11 REFOUT_LOCKED, 13-15 external-disk
activity/full/mounted, 16 PPS_IN_CAL, 17 DIFFCORR_IN, 18 INTERNET, rest
reserved. `RxError` bits: 3 SOFTWARE, 4 WATCHDOG, 5 ANTENNA, 6
CONGESTION, 8 MISSEDEVENT (external-event congestion), 9 CPUOVERLOAD,
10 INVALIDCONFIG, 11 (Rev1)
`OUTOFGEOFENCE`; on mosaic-G5 this same bit 11 is `OUTOFFENCE` and is
widened to also cover motion-fencing, distinguished via `lif,RxMessage`.
`OnChange`, fixed 1 s interval on both models.

### Measurements

**`MeasEpoch` (4027)** -- Shape 4 (2 levels). Fixed head (12 bytes):
`TOW`+`WNc`+`N1`(u1)+`SB1Length`(u1)+`SB2Length`(u1)+`CommonFlags`(u1
bitfield)+`CumClkJumps`(u1, Rev1, ms mod 256)+`Reserved`(u1). `Chunks()`
yields the head, then for each of `N1` outer elements: the
`MeasEpochChannelType1` piece, its `SB1Length - sizeof(Type1)` padding
piece, then (reading that element's `N2`) each of its `N2`
`MeasEpochChannelType2` inner pieces, each followed by its `SB2Length -
sizeof(Type2)` padding piece:

```
yield &head                                  // reads N1, SB1Length, SB2Length
for i := range N1 {
    yield &t1[i]                             // reads t1[i].N2
    yield make([]byte, sb1Length-sizeof(Type1))
    for j := range t1[i].N2 {
        yield &t2[i][j]
        yield make([]byte, sb2Length-sizeof(Type2))
    }
}
```

The padding pieces absorb any forward-compat `SBLength` growth; the same
iterator re-emits them on encode. `binary.Read` into a later piece can
read a count set by an earlier piece, so no cursor bookkeeping is
needed.

`CommonFlags` bits: 0 multipath mitigation on, 1 code smoothing active,
2 reserved, 3 clock steering active, 4 n/a, 5 high-dynamics mode, 6
"E6B used" (Galileo E6 measurements use E6B not default E6C), 7
scrambling active (Measurement Availability permission not granted --
degrades every measurement silently, no per-field marker).

`MeasEpochChannelType1` (master, 20 named bytes): `RxChannel`(u1),
`Type`(u1 bitfield: bits 0-4 `SigIdxLo` [31 = escape to `ObsInfo`
bits3-7+32], bits 5-7 `AntennaID` [0 main/1 Aux1/2 Aux2]), `SVID`(u1,
DNU `0`), `Misc`(u1 bitfield: bits 0-3 `CodeMSB`, bits 4-7 reserved),
`CodeLSB`(u4, 0.001 m; `PR = (CodeMSB*4294967296 + CodeLSB)*0.001`,
invalid iff `CodeMSB==0 && CodeLSB==0`), `Doppler`(i4, 0.0001 Hz, DNU
`0x80000000`), `CarrierLSB`(u2, 0.001 cyc), `CarrierMSB`(i1, 65.536
cyc, DNU `-128`; `L = PR/lambda + (CarrierMSB*65536+CarrierLSB)*0.001`,
invalid iff `CarrierMSB==-128 && CarrierLSB==0`), `CN0`(u1, DNU `255`,
formula below), `LockTime`(u2, s, DNU `65535`, clipped `65534`),
`ObsInfo`(u1 bitfield: bit 0 smoothed, bit 1 reserved, bit 2 half-cycle
ambiguity, bits 3-7 signal-number/GLONASS-FreqNr extension per
"Signal-number table" above), `N2`(u1).

`MeasEpochChannelType2` (slave, 12 named bytes): `Type`(u1, same
bitfield shape as Type1's own `Type`, describing the slave signal),
`LockTime`(u1, s, DNU `255`, clipped `254` -- narrower width and
different clip point than Type1's, do not share a constant),
`CN0`(u1), `OffsetsMSB`(u1 bitfield: bits 0-2 signed `CodeOffsetMSB`
[65.536 m/LSB], bits 3-7 signed `DopplerOffsetMSB` [6.5536 Hz/LSB],
sign-extend each before use), `CarrierMSB`(i1, DNU `-128`), `ObsInfo`(u1,
same shape as Type1's), `CodeOffsetLSB`(u2, 0.001 m; `PR2 = PR1 +
(CodeOffsetMSB*65536+CodeOffsetLSB)*0.001`, invalid iff
`CodeOffsetMSB==-4 && CodeOffsetLSB==0`), `CarrierLSB`(u2, 0.001 cyc;
uses this sub-block's own signal's lambda, invalid iff
`CarrierMSB==-128 && CarrierLSB==0`), `DopplerOffsetLSB`(u2, 0.0001 Hz;
`D2 = D1*alpha + (DopplerOffsetMSB*65536+DopplerOffsetLSB)*1e-4` where
`alpha` is the carrier-frequency ratio of this signal to Type1's
master, invalid iff `DopplerOffsetMSB==-16 && DopplerOffsetLSB==0`).

CN0 formula (both Type1 and Type2, using the sub-block's own resolved
signal number): `CN0[dB-Hz] = raw*0.25` if signal number is 1 or 2 (GPS
L1P/L2P), else `raw*0.25 + 10`.

`OnChange` at the receiver's internal measurement rate; Receiver
timestamp.

**`MeasExtra` (4000)** -- Shape 4 (1 level, with count reconstruction).
Fixed head (12 bytes): `TOW`+`WNc`+`N`(u1, valid **modulo 256** only)+
`SBLength`(u1)+`DopplerVarFactor`(f4, 1 Hz^2/cycle^2). True count:
`NrSB = ((Length/SBLength - N) / 256) * 256 + N` (integer division).
`MeasExtraChannelSub` (up to 17 bytes depending on revision, must gate
field reads on `SBLength`, not assume all are present): `RxChannel`(u1),
`Type`(u1, same `SigIdxLo`/`AntennaID` shape as `MeasEpoch`'s), `MPCorrection`(i2,
0.001 m), `SmoothingCorr`(i2, 0.001 m), `CodeVar`(u2, 0.0001 m^2, DNU
`65535`, clip `65534`), `CarrierVar`(u2, 1 mcycle^2, DNU `65535`, clip
`65534`; `sigma^2_Doppler[mHz^2] = CarrierVar * DopplerVarFactor`),
`LockTime`(u2, s, DNU `65535`, clip `65534`), `CumLossCont`(u1, wraps
mod 256), then Rev1 `CarMPCorr`(i1, 1/512 cyc), Rev2 `Info`(u1, all
bits reserved), Rev3 `Misc`(u1 bitfield: bits 0-2 `CN0HighRes`
[0.03125 dB-Hz/LSB, add to `MeasEpoch`'s 0.25-dB-Hz CN0], bits 3-7
extended signal number when this sub-block's own `Type` bits 0-4 == 31,
offset 32). `OnChange` alongside `MeasEpoch`; optional companion, a
receiver can enable `MeasEpoch` without it.

**`EndOfMeas` (5922)** -- Shape 1. `TOW`+`WNc` only, epoch-flush marker
for the measurement-block family (`MeasEpoch`/`MeasExtra`), independent
of `EndOfPVT`.

### Satellites and channel status

**`ChannelStatus` (4013)** -- Shape 4 (2 levels). Fixed part (12
bytes): `TOW`+`WNc`+`N`(u1, `0` legal)+`SB1Length`(u1)+`SB2Length`(u1)+
`Reserved`(u1[3]). Decode identically to `MeasEpoch`'s two-level loop
(outer `ChannelSatInfo[N]` of `SB1Length` bytes each, each with its own
`N2`/`ChannelStateInfo[N2]` of `SB2Length` bytes each, interleaved
immediately after its parent).

`ChannelSatInfo` (12 named bytes): `SVID`(u1, DNU `0`; if 0, real ID is
in `SVIDFull`), `FreqNr`(u1, GLONASS offset+8, else reserved),
`SVIDFull`(u2, meaningful only when `SVID==0`), `Azimuth/RiseSet`(u2
bitfield: bits 0-8 azimuth `[0,359]` deg [DNU `511`], bits 9-13
reserved, bits 14-15 rise/set [0 setting, 1 rising, 2 undocumented, 3
elevation-rate-unknown]), `HealthStatus`(u2, eight 2-bit
constellation-dependent slots, see table below), `Elevation`(i1, deg,
DNU `-128`), `N2`(u1), `RxChannel`(u1), `Reserved2`(u1).

`ChannelStateInfo` (8 bytes): `Antenna`(u1, 0 main/1 Aux1/2 Aux2),
`Reserved`(u1), `TrackingStatus`(u2, same 8-slot layout as
`HealthStatus`, values 0 idle, 1 search, 2 sync, 3 tracking),
`PVTStatus`(u2, same layout, values 0 not used, 1 waiting for
ephemeris, 2 used, 3 rejected), `PVTInfo`(u2, opaque, no mapping).

`HealthStatus`/`TrackingStatus`/`PVTStatus` share one 8-slot,
2-bit-per-slot layout (slot 0 = bits 1-0, ... slot 7 = bits 15-14); the
signal each slot names is constellation-dependent (keyed off the
containing `SVID`/`SVIDFull`'s GNSS, not off bit position alone):

| GNSS | slot0 | slot1 | slot2 | slot3 | slot4 | slot5 | slot6 | slot7 |
|---|---|---|---|---|---|---|---|---|
| GPS | L1CA | P1(Y) | P2(Y) | L2C | L5 | L1C | -- | -- |
| GLONASS | L1CA | L1P | L2P | L2CA | L3 | -- | -- | -- |
| Galileo | -- | E1BC | -- | E6BC | E5a | E5b | E5ab | -- |
| SBAS | L1 | L5 | -- | -- | -- | -- | -- | -- |
| BeiDou | B1I | B2I | B3I | B1C | B2a | B2b | -- | -- |
| QZSS | L1CA | L2C | L5 | L6 | L1C | L1S | L1CB | L5S |
| NavIC | L5 | L1 | -- | -- | -- | -- | -- | -- |

`HealthStatus` value: 0 unknown/n-a, 1 healthy, 2 undocumented, 3
unhealthy. `OnChange`; default rate is the PVT rate on mosaic-X5 but a
fixed 1 s on mosaic-G5 -- a config-side detail, no wire-format effect.

**`SatVisibility` (4012)** -- Shape 4 (1 level). Fixed part (8 bytes):
`TOW`+`WNc`+`N`(u1, `0` legal)+`SBLength`(u1). `SatInfo[N]`, `SBLength`
bytes each (8 at Rev0, 10 at Rev1 -- **mosaic-G5 documents Rev1,
so this is the shape a G5 decoder should expect and be tolerant of
Rev0 as the fallback, not the other way around**): `SVID`(u1; at Rev1,
`0` means "use `SVIDFull`"), `FreqNr`(u1, GLONASS offset+8, else
reserved), `Azimuth`(u2, 0.01 deg, DNU `65535` -- **scale the raw
`uint16` by 0.01 first, then cast to the signed output type; casting
the raw `uint16` to `int16` before scaling overflows for any azimuth
above 327.68 deg**, e.g. raw 35000 (350.00 deg) cast straight to
`int16` gives -30536), `Elevation`(i2, 0.01 deg, DNU `-32768`),
`RiseSet`(u1 enum: 0 setting, 1 rising, 255 unknown -- note this is a
**different** encoding/sentinel from `ChannelStatus`'s packed
`RiseSet` bits, which use 3 not 255 for "unknown"), `SatelliteInfo`(u1
enum: 1 almanac, 2 ephemeris, 255 unknown), and at Rev1 only,
`SVIDFull`(u2, meaningful only when `SVID==0`). `OnChange`, fixed 1 s
interval on both models.

### Timing and PPS

**`ReceiverTime` (5914)** -- Shape 1. `TOW`+`WNc`+`UTCYear`(i1, 2-digit,
assume `2000+raw`, DNU `-128`)+`UTCMonth`(i1, DNU `-128`)+`UTCDay`(i1,
DNU `-128`)+`UTCHour`(i1, DNU `-128`)+`UTCMin`(i1, DNU `-128`)+
`UTCSec`(i1, DNU `-128`)+`DeltaLS`(i1, GPS-UTC leap offset, DNU
`-128`)+`SyncLevel`(u1 bitfield). 24 bytes, 2 bytes padding. `SyncLevel`:
bit 0 `WNSET` (week number set), bit 1 `TOWSET` (TOW set to within 20
ms), bit 2 `FINETIME` (TOW within the `setClockSyncThreshold` limit),
bits 3-7 reserved -- **each bit set means the named criterion is met;
"full synchronization" is `SyncLevel & 0x07 == 0x07`** (all three set),
mirrored by `ReceiverStatus.RxState` bits 4-6. `OnChange`, fixed 1 s
interval (its own rate, not tied to PVT/measurement).

**`xPPSOffset` (5911)** -- Shape 1. `TOW`+`WNc`+`SyncAge`(u1, s, clipped
at 255, not a DNU; always 0 when `Timescale==3`)+`Timescale`(u1 enum:
1 GPS, 2 UTC, 3 Receiver [unsynced, internal], 4 GLONASS, 5 Galileo, 6
BeiDou, 100 FugroAtomiChron -- **a different numbering from
`PVTCartesian.TimeSystem`; do not share one Go type between the two
blocks**)+`Offset`(f4, already in nanoseconds directly -- no further
scaling needed despite the guide's "1e-9 s" annotation, since that
annotation composes to "raw value in ns" the same way `TOW`'s "0.001 s"
composes to "raw value in ms"; negative = pulse fired early, positive
= late; magnitude typically a few ns). 20 bytes, no padding. `OnChange`
at the configured PPS rate, not `setSBFOutput`-decimatable; fires once
per physical pulse edge, a post-pulse (not predictive) report.

### Corrections and base station

**`DiffCorrIn` (5919)** -- Shape 3 (opaque trailing bytes: no element
count, since there is no `N`/`SBLength` -- `Length` alone bounds the
tail, yielded as a single `[]byte` chunk). `TOW`+`WNc`+`Mode`(u1 enum: 0 RTCM v2, 1 CMR [v2 on X5],
2 RTCM v3, 3 RTCMV, 4 SPARTN, 5 reserved)+`Source`(u1 enum, physical
port: 0-3 COM1-4, 4/5 USB1/2, 6 IP, 7 SBF file, 8 L-Band, 9 NTRIP, 10/11
OTG1/2, 12 Bluetooth, 15 UHF modem, 16 IPR, 17 direct call, 18 IPS, 19
SSR2OBS; DNU `255`)+ a single trailing `[]byte` spanning `raw[16:Length]`
(the correction message, still framed, verbatim; a decoder must not
trim padding here -- push that to whichever inner-protocol package,
e.g. `gps/lib/rtcmbin`/`gps/lib/spartnbin`, understands the specific
framing and finds its own true end, exactly as those packages already
do when parsing a native stream). No RTCM2/CMR/RTCMV unpacking belongs
in `sbfbin` -- store the raw trailing bytes and stop. `OnChange` per correction
message received; **Receiver** timestamp (time the receiver decoded
the message, not a signal-in-space time); no Flex-Rate/esoc.

**`BaseStation` (5949)** -- Shape 1. `TOW`+`WNc`+`BaseStationID`(u2,
as carried in the source correction message)+`BaseType`(u1 enum: 0
fixed, 1 moving [reserved], 255 unknown)+`Source`(u1 enum, correction
**format**, not a port -- do not conflate with `DiffCorrIn.Source`: 0
RTCM2 Msg3, 2 RTCM2 Msg24, 4 CMR Msg1, 8 RTCM3 Msg1005/1006, 9 RTCMV
Msg3, 10 CMR+ Type2; X/Y/Z interpretation is phase-center for
`{0,4,10}`, ARP for `{2,8}`, proprietary for `{9}`)+`Datum`(u1 enum,
same table as the PVT-family `Datum` above)+`Reserved`(u1)+`X`/`Y`/`Z`
(each f8, 1 m; no DNU documented for this block specifically, but
defensively treat exact `-2e10` as DNU by analogy with every other
position field). 44 bytes, no padding. `OnChange` per base-coordinate
correction message received; Receiver timestamp; no Flex-Rate/esoc.

### Other

**`ReceiverSetup` (5902)** -- Shape 2 (Rev1-4 trailing fields, all
fixed-width Latin-1 strings or scalars, gate each on available
`Length`). Rev0: `TOW`+`WNc`+`Reserved`(u1[2])+`MarkerName`(c1[60])+
`MarkerNumber`(c1[20])+`Observer`(c1[20])+`Agency`(c1[40])+
`RxSerialNumber`(c1[20])+`RxName`(c1[20])+`RxVersion`(c1[20])+
`AntSerialNbr`(c1[20])+`AntType`(c1[20])+`deltaH`/`deltaE`/`deltaN`
(each f4, 1 m). Rev1 adds `MarkerType`(c1[20]). Rev2 adds
`GNSSFWVersion`(c1[40]). Rev3 adds `ProductName`(c1[40])+
`Latitude`/`Longitude`(f8, 1 rad, DNU `-2e10`)+`Height`(f4, 1 m, DNU
`-2e10`) -- reference/marker position, not the live PVT solution. Rev4
adds `StationCode`(c1[10])+`MonumentIdx`(u1)+`ReceiverIdx`(u1)+
`CountryCode`(c1[3])+`Reserved1`(c1[21]). 424 bytes total at Rev4 (the
shape every real capture and the current guide show), no padding.
String fields are nul-padded fixed-width Latin-1 -- use
`gps/lib/latin1z.StringZ<N>`; add sizes 3, 20, 21, 40, 60 (none of which
exist yet -- the current set is 5, 10, 16, 30, 32, 33, 43, 66, 129) by
extending the `sizes` slice in `latin1z/mksizes.go` and re-running
`go generate` (do not hand-edit the generated `sizes.go`). `OnChange` every 60 s
and on any command changing a reflected parameter; Receiver timestamp;
esoc yes, Flex-Rate no.

### Leap-second (UTC) blocks

The three `*Utc` blocks are the only nav-message-family blocks in
scope -- each feeds `LeapSecondMsg` (see `plan/septentrio-msg.md`).
All three are Shape 1, **SIS** timestamp (`WNc` is GPS-convention per
the shared rule above), `OnChange` per decode, no Flex-Rate; the
receiver emits one instance per contributing satellite, and the
`PRN`/`SVID` field identifies the source satellite only. The rest of
the nav-message family (ephemeris, ionosphere) is deliberately not
decoded -- see "Blocks excluded for the G5 target".

`GPSUtc` (5894, 40 bytes): `PRN`+`Reserved`+`A_1`(f4, 1 s/s)+`A_0`(f8,
1 s)+`t_ot`(u4, 1 s)+`WN_t`(u1, mod-256)+`DEL_t_LS`(i1)+`WN_LSF`(u1,
mod-256)+`DN`(u1, **1-based, Sunday=1..Saturday=7**)+`DEL_t_LSF`(i1).

`GALUtc` (4031, 40 bytes): `SVID`+`Source`(u1 enum: 2 I/NAV, 16
F/NAV)+`A_1`(f4, DNU `-2e10` -- **unlike `GPSUtc`/`BDSUtc`, this block
documents a DNU for `A_1`/`A_0`**)+`A_0`(f8, DNU `-2e10`)+`t_ot`(u4)+
`WN_ot`(u1, mod-256)+`DEL_t_LS`(i1)+`WN_LSF`(u1, mod-256)+`DN`(u1,
1-based)+`DEL_t_LSF`(i1).

`BDSUtc` (4121, 32 bytes): `PRN`(global-SVID)+`Reserved`+`A_1`(f4)+
`A_0`(f8)+`DEL_t_LS`(i1)+`WN_LSF`(u1, mod-256)+`DN`(u1, **0-based,
Sunday=0..Saturday=6** -- differs from GPS/Galileo's 1-based
convention)+`DEL_t_LSF`(i1). No `t_ot`/`WN_t` reference-time fields at
all (BeiDou's D1/D2 UTC parameter set has none) -- 5 bytes shorter than
`GPSUtc`'s payload, not a truncation bug.

## Blocks excluded for the G5 target

Not implemented in this phase. Some are genuinely absent from (or
unconfirmed on) mosaic-G5 firmware (per the guide-vs-guide comparison);
others are present on the G5 but decode to no `gpsprot.Msg`, config
tag, or RINEX observation, so are out of scope by the "decode only
what's needed" principle:

Each block below still gets a name-only `idNameMap` entry in `other.go`
(see "Package layout"): registering a name is cheap, independent of
decoding, and is what keeps `satpulsetool scan`/`annotate` readable for
the "present on the G5" blocks this phase does not decode. Naming the
firmware-absent blocks too is harmless and keeps the registry a complete
SBF block-number catalog.

- **`ExtEvent`/`ExtEventPVTCartesian`/`ExtEventPVTGeodetic`**
  (5924/4037/4038) -- external-event timestamping is not a satpulse
  feature, so these are deliberately not decoded. Unlike the other
  entries here they are present on the G5 (a scope decision, not a
  model absence), and they appear in none of the example captures.
- **The ephemeris/ionosphere nav-message family** -- `GPSNav`,
  `GPSCNav`, `GPSIon`, `GALNav`, `GALIon`, `GLONav`, `GLOTime`,
  `BDSNav`, `BDSCNav1/2/3`, `BDSIon`, `QZSNav`, `NavICLNav`, `GEONav`,
  and the SBAS decoded blocks `GEOAlm`/`GEOIGPMask`/`GEOIonoDelay`
  (5897/5931/5933). Only the three `*Utc` leap-second blocks feed a
  `gpsprot.Msg` (`LeapSecondMsg`); the ephemeris/iono/almanac blocks
  would only be needed for RINEX *navigation* files or RTCM-ephemeris
  output -- separate future tracks (see `plan/rtcm-eph.md`), not the
  observation-only RINEX of `plan/sbf-rinex.md`. Present on the G5 (a
  scope decision, not a model absence); this also removes the entire
  guide-text-only, no-capture-cross-check risk area (`BDSCNav1/2/3`,
  `NavICLNav`).
- **`LBandRaw`** (4212) -- raw L-band (MSS) frames with no consumer in
  the current gpsprot / config / RINEX scope.
- **`Meas3Ranges`/`Meas3CN0HiRes`/`Meas3Doppler`/`Meas3PP`/`Meas3MP`**
  (4109/4110/4111/4112/4113) -- the compact delta-coded Meas3 format is
  an X5-only product feature (predates G5's existence and is absent
  from the G5 guide entirely); `MeasEpoch` is the observation source
  for both models.
- **`PosCart`** (4044) -- documented only in the X5 guide; G5's
  combined-block equivalents are `NavCart`/`NavGeod` (see below), not
  this block.
- **`EncapsulatedOutput`** (4097) -- X5-only; not in the G5 guide's
  Miscellaneous Blocks list at all.
- **`GEONetworkTime`** (5918) and the other 9 X5-only SBAS raw-message
  blocks (`GEOMT00`, `GEOPRNMask`, `GEOFastCorr`, `GEOIntegrity`,
  `GEOFastCorrDegr`, `GEODegrFactors`, `GEOLongTermCorr`,
  `GEOServiceLevel`, `GEOClockEphCovMatrix`) -- G5's SBAS L1 Decoded
  Message Blocks section covers only `GEONav`/`GEOAlm`/`GEOIGPMask`/
  `GEOIonoDelay`.
- **`RTCMDatum`** (4049), **`LBandBeams`** (4204),
  **`NTRIPClientStatus`/`NTRIPServerStatus`/`IPStatus`/`DynDNSStatus`/
  `P2PPStatus`** (4053/4122/4058/4105/4238) -- all require a network
  stack or NTRIP client G5 does not have.

Two G5-only blocks with no decode spec yet (not required for this
phase's milestone, worth a follow-up): **`NavCart`** (4272) and
**`NavGeod`** (4275) -- combined PVT+attitude+DOP+time blocks unique to
G5, and **`AuxAntPositions`** (5942) -- multi-antenna attitude, G5-only.

## Registration

`gps/gpsreg/reg.go`: add `TagSBF = septentrio.Tag` alongside the other
`Tag*` re-exports (`septentrio.Tag` is `gpsprot.Tag "SBF"`, following
`ubx.Tag`/`spartn.Tag`); add `septentrio.PacketFormat` to
`allVendorPacketFormats` and to `allVendorPacketFormatsMap[VendorSeptentrio]`;
add `septentrio.Tag: septentrio.NewPacketProcessor(mgr)` to the
`map[gpsprot.Tag]gpsprot.PacketProcessor` that `CreatePacketProcessors`
builds (the map is keyed by `Tag`, not by vendor -- vendor specialization
is applied afterward via `SetVendor`). `VendorSeptentrio` already exists
(`gps/gpsreg/reg.go:32`); no enum change needed.

`gps/gpsdecode/gpsdecode.go`: add a `case gpsreg.TagSBF:` arm to the
`Tag()` switch in `Decode`, calling a small `sbfDecode(data []byte)
(*DecodeResult, error)` helper that wraps `sbfbin.ParseMsg(string(data))`
(mirroring `ubxbinDecode`/`casicDecode`), returns `DecodeResult{Payload:
b.Params, Header: b.TimeStamp}`, and maps a `*sbfbin.UnknownParams` body
(`b.Params`) to `ErrUnknownMsg`. Without this, `satpulsetool
decode`/`annotate` will recognize and frame SBF packets (via the scanner)
but fail to decode their payload.

### `gpsprot.PacketFormat` (`gps/internal/septentrio`)

Byte-by-byte `Next`/`IsFinal` state machine, modeled closest on
`gps/internal/ubx` (fixed 8-byte header with the length field near the
end of the header) rather than `gps/internal/nov` (variable header
length via a header-length byte) -- SBF's CRC lives inside the fixed
header and needs no separate trailer bookkeeping during scanning; it
is only used after `IsFinal`, via `ExtractChecksum`/`ComputeChecksum`.

```go
const (
	stateSync    gpsprot.ScanState = iota + gpsprot.ScanStateSync // looking for '$'
	stateStarted                                                  // '$' seen, header still accumulating
	stateExpectN                                                  // countdown anchor; IsFinal when reached exactly
)
```

Transition table (`packetLen` = bytes already accepted, so the current
byte is at 0-indexed offset `packetLen` within the candidate packet):

| state | packetLen | condition | next |
|---|---|---|---|
| `stateSync` | -- | `b == 0x24` | `stateStarted` |
| `stateSync` | -- | else | `stateSync` |
| `stateStarted` | 1 (offset 1, Sync2) | `b == 0x40` | `stateStarted` |
| `stateStarted` | 1 | else | `stateSync` (single-byte rejection, see below) |
| `stateStarted` | 2-6 (CRC, ID, Length lo) | any | `stateStarted` |
| `stateStarted` | 7 (offset 7, Length hi byte) | `length := u16le(buf[nextScanIndex-1], b)`; `bodyLen := length-8`; `bodyLen > 0 && bodyLen%4==0` | `ScanState(int(stateExpectN) + bodyLen)` |
| `stateStarted` | 7 | else (bad length) | `stateSync` |
| `> stateExpectN` (countdown) | -- | any | `state - 1` |

(offset N is processed at `packetLen == N`, matching `ubxbin`'s
`packetFormat.Next` -- sync2 at `packetLen == 1`, length hi at
`packetLen == 7` -- since `$`/Sync1 is accepted at `packetLen == 0` in
sync state, leaving `packetLen == 1` for Sync2.)

`bodyLen <= 0` is rejected even though the spec only guarantees
"multiple of 4": every real block carries at least a TOW/WNc timestamp,
so a header-only frame (`Length == 8`) is far more likely a coincidental
`$@` byte pair inside other data than a genuine frame.

**`$@` needs no ASCII-format disambiguation logic of its own.** Every
other `$`-led framing in the Septentrio ecosystem (command replies
`$R`, async ASCII `$T`, formatted info `$-`, SNMP `$&`, NMEA sentences)
has a different second byte, so `stateStarted` at `packetLen==1`
requiring `b==0x40` rejects all of them in one byte, with no lookahead.
Ordering `septentrio.PacketFormat` relative to NMEA in
`gpsreg.allVendorPacketFormats` therefore does not matter for
correctness.

**Checksum**: `ExtractChecksum` returns `pkt[2:4]` (the CRC field,
little-endian, directly comparable to `ComputeChecksum`'s output);
`ComputeChecksum` returns `sbfbin.CRC16(pkt[4:])` as two little-endian
bytes (covers ID through end of block, i.e. everything after Sync and
CRC). `MsgID(pkt)` returns `sbfbin.PacketMsgID(pkt).String()`.
`IsBinary()` returns `true`. `RescanOnBadChecksum` always returns
`true`: SBF's 2-byte sync is the shortest of any binary format in this
codebase, and blocks routinely run past 1000 bytes (`ChannelStatus`,
`MeasEpoch`), giving ample room for a coincidental `0x24 0x40` pair
inside a payload -- a checksum failure here is more likely a false sync
than a corrupted real frame, so always resync one byte past the failed
candidate's start rather than skipping the whole (possibly
wrongly-sized) candidate span. This mirrors `spartn.PacketFormat`'s
same call for the same reason (a short, high-entropy-looking preamble).

### Do-nothing `PacketProcessor`

Modeled on `nov.AbbrevAsciiPacketProcessor` (the existing precedent for
"parse, then forward everything to `NativeMsgHandler`, produce no
protocol-agnostic messages"):

```go
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
}

func NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor {
	return &PacketProcessor{}
}

func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	msg, err := sbfbin.ParseMsg(data)
	if err != nil {
		return "", err
	}
	msgID := msg.ID().String()
	if nmh := p.GetNativeMsgHandler(); nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, msg, tRead)
	}
	return msgID, nil
}

func (p *PacketProcessor) NativeOnly() bool { return true }
```

`NewPacketProcessor` takes `*gpsprot.NavEpochManager` in anticipation of
phase 3's real conversion layer, even though this phase's processor does
not use it. (Not every vendor constructor takes the manager -- the
native-only `spartn` and `nov`-abbrev processors take none -- so taking
it now is a forward-looking choice, not a required signature. `spartn`,
also native-only and returning `RescanOnBadChecksum` true, is the closest
precedent; dropping the argument until phase 3 would be equally valid.) `NativeOnly() == true`
for this phase specifically -- it genuinely produces zero
protocol-agnostic messages right now, so `gpscfg`'s device-detection
logic (`suitableMessageCount`/`nativeOnlyTags`,
`gps/app/gpscfg/gpscfg.go`) correctly treats SBF traffic as
"native-only" until the real conversion layer (phase 3) flips this to
`false`. Do not forget to flip it then.

## Phasing within this plan

The ordering is driven by testability. `satpulsetool scan` is
block-agnostic -- `scancmd` only frames packets (`scan.New` + `Scan`),
never decoding a body -- so the scanner lands first and runs on every
example capture before any block struct exists. Decode/pack then grows
one block *shape* at a time, each validated against real (scan-filtered)
traffic, so a machinery bug surfaces on four blocks rather than two
dozen. Blocks split into Tier 1 (present in `~/SbfParser/sbf_files`,
testable now) and Tier 2 (absent from every example capture, deferred
behind G5 hardware); only `BaseStation` and `DiffCorrIn` are Tier 2.

The four block shapes, referenced by the milestones below:

- **Shape-1** (single `binary.Read`, not `Chunked`): `EndOfPVT`,
  `EndOfMeas`, `DOP`, the four Cov blocks, `GALAuthStatus`, `xPPSOffset`,
  `ReceiverTime`, the three `*Utc` leap-second blocks (`GPSUtc`,
  `GALUtc`, `BDSUtc`); Tier 2: `BaseStation`.
- **`Chunked` single-level**: `SatVisibility`, `RFStatus`,
  `ReceiverStatus`, `MeasExtra` (including the `NrSB` mod-256
  reconstruction), `QualityInd` (count-prefixed array); Tier 2:
  `DiffCorrIn` (opaque `[]byte` tail).
- **`Chunked` revision-trailer**: `PVTCartesian`, `PVTGeodetic`,
  `ReceiverSetup` (plus the `latin1z` size additions `ReceiverSetup`
  needs) -- exercises the tolerant end-of-stream behaviour.
- **`Chunked` two-level**: `MeasEpoch`, `ChannelStatus`.

**M0 -- scanner (block-agnostic; `scan` works on every capture).**
`crc.go` (`CRC16`); `gps/internal/septentrio`'s `PacketFormat` (sync
`$@`, `Length` multiple-of-4 and `> 8` validation, CRC, `MsgID`
block-number masking, `RescanOnBadChecksum`) and do-nothing
`PacketProcessor`; the minimal `common.go` the scanner needs
(`Sync`/`HeaderLen`/`MsgID`/`PacketMsgID`); `other.go`'s name-only
`idNameMap` covering all known block IDs (so `scan`/`annotate` name
every frame, decoded or not); registration in `gps/gpsreg`;
`docs/internals.md` entry. No block structs and no `Chunked` driver
yet. At this point `satpulsetool scan` frames the `$@` packets in all
five example captures, and the scanner unit tests run (including the
`$R`/`$T`/`$-`/`$&`/NMEA byte-2 rejection and the unknown block 5942).

**M1 -- machinery + one block per shape (decode/pack on real traffic).**
The rest of `common.go` (registry, `TimeStamp`, `Params`, `Block`,
`Chunked` + `ReadBinChunked`/`WriteBinChunked`/`UnknownParams`), plus
`ParseMsg`/`Serialize`/`PackMsg` and the `Decode` switch in
`gps/gpsdecode`. Implement exactly one Tier-1 block of each shape:
`EndOfMeas` (shape-1), `MeasExtra` (`Chunked` single-level),
`PVTGeodetic` (revision-trailer), `MeasEpoch` (two-level). Each
machinery path is then exercised end-to-end by filtering `scan` output
to those blocks (a `grep` on the JSONL) and running `decode`->`pack`.
Undecoded blocks fall through to `UnknownParams` and round-trip opaque,
so the filter only narrows what a failure implicates -- it is not needed
for correctness.

**M2 -- breadth-fill Tier 1.** The remaining Tier-1 blocks across all
four shapes, in any order (nothing depends on another block's structs,
only on M1's `Chunked` machinery). Milestone check: `decode`->`pack`
the whole `log_0000.sbf` cleanly.

**Tier 2 (deferred behind G5 hardware): `BaseStation`, `DiffCorrIn`.**
They stay name-only in `idNameMap` -- `UnknownParams` round-trips them
opaque -- until real captures exist, then graduate into the `Decode`
switch and fold into the `packet-testdata` corpus.

Round-trip unit tests are written alongside each block, not deferred
(see Testing).

## Testing

Same-package (`package sbfbin`) tests throughout, following the
`go-unit-test` skill. No `testdata/` directory needed for the unit
round-trips in this phase (hand-built `[]byte` literals suffice; a
later phase's `packet-testdata` work handles real-capture corpora), and
the real-file end-to-end below uses the existing captures directly, so
nothing is committed as a fixture.

- **End-to-end against the existing `.sbf` files, now (the backbone).**
  The example captures in `~/SbfParser/sbf_files` (X5-line, per
  `CLAUDE.local.md`) drive `satpulsetool` before any G5 hardware. At M0,
  `scan` frames the `$@` packets out of all five captures immediately.
  From M1 on, `decode`->`pack` runs on blocks filtered out of the `scan`
  JSONL with an ordinary `grep` (the block number sits at the `ID`
  offset in each entry's `bin` hex) -- no fixture files and no bespoke
  filter tool, since `scan`'s per-packet log *is* the filterable form.
  Blocks not yet decoded round-trip opaque through `UnknownParams`, so
  the real captures exercise that path for free. Widen the filter as each
  shape/block lands; the M2 check is a clean `decode`->`pack` of
  `log_0000.sbf`. This complements the per-block unit round-trips below
  and exercises the scanner and CRC on real traffic. Once G5 hardware and
  captures exist, fold representative blocks into the `packet-testdata`
  corpus per `plan/packet-testing.md` and repeat as a milestone smoke
  check.
- **Round-trip** (per block, Tier 1 first): a generic
  `testBlock[P Params]` helper (analogous to `casbin`/`ubxbin`'s
  per-message helper) that constructs a `Block` with a filled `*P` in
  `Params` and a chosen `Rev`, calls `Serialize`, then `ParseMsg`, and
  checks the result matches -- one call per block type, Tier-1 blocks
  (those present in the captures) first, covering every field including
  at least one non-DNU and one DNU sample value where practical. Because
  the same `Chunked` iterator
  drives both directions, `Serialize` is the inverse of `ParseMsg` by
  construction; the check is *value*-level (the re-decoded struct
  equals the original, including `Block.Rev`), not byte-exact --
  padding and dropped newer-revision trailing bytes make byte-exact
  round-trip neither achievable nor meaningful.
- **Revision tolerance** (`Chunked` trailer blocks): a test that
  `ParseMsg`s a hand-built packet truncated to the Rev0 length and
  checks the trailer parameters read as their DNU defaults (validating
  the tolerant end-of-stream), not an error; and a test that a packet
  with extra unknown trailing bytes (a future revision) parses
  successfully, ignoring them. Confirm that a `Block` carrying a given
  `Rev` serializes with matching `ID` revision bits, and that
  parse->serialize->parse preserves `Rev` value-level (the exact emitted
  `Length` for an older-than-current revision follows the encode-fidelity
  choice in "Open decisions").
- **`SBLength` tolerance** (`Chunked` sub-block blocks): a test where a
  sub-block's declared `SBLength` exceeds the known struct size (a
  newer-revision sender with an extra field this decoder doesn't
  understand), confirming the padding piece absorbs the slack and the
  next sub-block is found at the correct offset.
- **CRC**: a known-good `CRC16` test vector (from a block in the example
  `.sbf` files, or hand-computed), plus a bad-checksum case exercised
  through the scanner (see below).
- **`PacketFormat` scanner** (`gps/internal/septentrio`, same-package
  tests, reusing `gps/internal/scantest`'s `FindPacket`/
  `InsertRandomPrefix` the way other packet-format tests do):
  a minimal valid frame round-trips through `Next` to `IsFinal`; a
  truncated header does not panic or falsely report `IsFinal`; a
  `Length` that is not a multiple of 4, or `<= 8`, rejects at the
  length-field offset; a corrupted body byte with
  `RescanOnBadChecksum` recovers a following genuine frame one byte
  past the failed candidate's start; a literal `$@` byte pair embedded
  inside a large payload (e.g. a synthetic 1000+-byte `ChannelStatus`
  body) does not cause the outer frame to mis-split; `$R`/`$T`/`$-`/
  `$&`/a plain NMEA sentence are all rejected at the second byte; a
  nonzero-revision `ID` still yields the correct masked block number
  from `MsgID`, as does an unknown block number (e.g. 5942, present in
  the captures). The example logs are pure SBF -- no interleaved NMEA
  or `$R`/`$T` command traffic -- so these byte-2 rejections cannot be
  reached by the real-file end-to-end above and must stay unit tests;
  the interleaving they guard against only happens live on a port.

## Open decisions

- **`Chunked` driver details copied from `novmsg`.** The plan fixes
  the two SBF-specific behaviours (tolerant end-of-stream; `[]byte`
  padding pieces sized from `SBLength`) but leaves the exact copied
  signatures -- whether `ReadBinChunked`/`WriteBinChunked` keep
  `novmsg`'s `messageName` error-context argument, how EOF tolerance is
  signalled (a sentinel piece vs. the driver checking remaining bytes)
  -- to settle when writing `common.go`. These are implementation
  details that do not change the block catalog above.
- **Encode revision fidelity.** `Block.Rev` is preserved through
  parse->serialize (the emitted `ID` carries `Rev<<13`), and decode is
  `Length`-authoritative. What remains open is whether `Serialize`, for a
  block whose `Rev` is below the current (most-recent) revision, emits
  exactly that revision's shorter `Length` (writing only the trailer
  groups up to `Rev`) or the current full `Length` (writing all trailer
  parameters, with the not-received ones at their DNU defaults).
  Value-level round-trip holds either way, and a real G5 emits the
  current revision, so this only affects re-framing of hypothetical
  older-revision captures; settle it when writing `Serialize`.
- **`NavCart`/`NavGeod`/`AuxAntPositions`** are G5-only convenience/
  attitude blocks with no decode spec yet, deliberately deferred (see
  "Blocks excluded for the G5 target"). Worth a follow-up plan once
  attitude/combined-PVT output is in scope.
- **Whether to also decode X5-only blocks for cross-model reuse.**
  This phase is scoped to the G5 block set only. If satpulse ever
  supports the mosaic-X5 too, `Meas3*`, `PosCart`, `EncapsulatedOutput`,
  the extra SBAS GEO* raw blocks, and `RTCMDatum`/`LBandBeams`/the
  NTRIP-status blocks would need their own decode specs at that point;
  not fabricated here since they are out of scope for the current
  target hardware.
