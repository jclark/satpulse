---
title: Supporting a new vendor
---

## Purpose

This document describes, in a vendor-independent way, what it takes to fully support a new GPS/GNSS receiver vendor (or a new protocol) in SatPulse. It is grounded in the existing code structure (see [internals.md](internals.md)) and in the patterns already established by the supported vendors (u-blox, NovAtel, Unicore, CASIC/Zhongke, Allystar, Quectel, SinoGNSS, ByNav, Septentrio, plus the vendor-agnostic NMEA). It has two halves: first the work as a dependency graph of chunks -- what to do in what order, what can run in parallel, what needs hardware, and what is optional -- then a map by code area, which the chunks reference for the details of what to build where.

Throughout, `<vendor>` stands for the new vendor's short name and `<proto>` for the wire protocol's short name (often the same). Real vendor names appear only as *models to copy*, never as the subject.

## Background: configuration layers and the message pipeline

SatPulse abstracts a receiver into device-independent messages (post-configuration) and, where implemented, device-independent configuration. Low-level configuration through message files (`configs/gpsmsg/<vendor>/`, using the common tag conventions in `configs/gpsmsg/tags.md`) is the common base: it lets users send documented vendor commands directly and is also useful when developing or debugging high-level support. High-level configuration adds a `gpsprot.ConfigProtocol`/Configurator so `satpulsetool gps --gnss`, `--pps`, `--signal`, survey, and similar options work through protocol-independent properties.

Both configuration layers depend on the same decode/message pipeline. The runtime path is:

```
receiver bytes -> gps/scan (PacketFormat) -> gpsprot.Packet -> PacketProcessor.ProcessPacket -> gpsprot Msg(s) -> MsgHandler
```

The device-independent target is the `gpsprot.Msg` set and its `MsgHandler` (`gps/gpsprot/msg.go`):

| gpsprot Msg | Represents |
|---|---|
| `TimeMsg` | Time and PPS: TAI/UTC time, accuracy, GNSS source, `Ref` (NavSolution / PrePulse / PostPulse), `PulseOffset` |
| `PosGeoMsg` / `PosECEFMsg` | Position (geodetic, or ECEF) with accuracy |
| `VelGeoMsg` / `VelECEFMsg` | Velocity (NED / ground-speed-course, or ECEF) |
| `LeapSecondMsg` | Leap-second info (`ptime.LeapSecond` + source GNSS) |
| `SurveyMsg` | Survey-in / self-survey status |
| `SatellitesMsg` | Per-SV visibility/usage (`[]SVInfo`, each with `[]SignalInfo`), look angles, used-validity |
| `NavEpochMsg` | Per-epoch solution-quality summary: FixLevel, SolutionDim, CorrKind, AuxSrc, DOP, accuracy, SV counts, GNSS/bands used, diff age, base ID |
| `CorReportMsg` | Correction-packet observation |

Anything a vendor message carries with no gpsprot home is delivered via `NativeMsgHandler.NativeMsg` rather than dropped.

Not every dimension applies to every vendor, but for a full GNSS receiver most do.

## The chunks and their dependencies

Vendor support divides into chunks of work. The chunks are deliberately not numbered: they do not form a line. Each has explicit dependency edges, and chunks with no edge between them can proceed in parallel. Which chunks exist at all for a given vendor is decided up front by the documentation analysis (see "Protocol shapes" below for how the graph shrinks). Each chunk description points into the code-area sections in the second half of this document for the details of what to build.

```
documentation analysis
 |
 +-- minimal exploration slice
      |
      +-- hardware exploration                  (needs the receiver)
      |
      +-- protocol codec                        (message set scoped by exploration)
      |    |
      |    +-- Msg conversion
      |    +-- RINEX observation conversion     [optional]
      |
      +-- low-level configuration               (reply handling fed by exploration)
           |
           +-- high-level configuration         [optional; also needs the codec]
```

### Documentation analysis

Everything starts with a systematic reading of the vendor's protocol documentation. The deliverable is decisions and reference material, not code:

- **The protocol's shape** (see "Protocol shapes"), which fixes which of the other chunks exist for this vendor.
- **Message triage.** Enumerate every message the receiver emits and triage each against the gpsprot Msg set and the RINEX-observation source. Be generous -- SV counts, DOP, and end-of-epoch markers all count. The triage records which messages get decode structs and which are deliberately excluded (registered name-only), with the reason.
- **Command-surface survey.** The vendor's configuration interface mapped against the standard tag categories in `configs/gpsmsg/tags.md`, including whether current configuration state can be queried -- the technical gate for high-level configuration.
- **Decisions on the optional chunks**, on both grounds: technical possibility and an importance judgment on the vendor (see the per-chunk *Applies* notes).
- **Divergence analysis.** How the protocol differs from the existing `gps/lib/*` models; each real difference drives a concrete design choice downstream.
- **Wire-fact reference tables.** Satellite numbering, signal identifiers, time conventions, unavailable-data sentinels -- recorded once here, consumed by every downstream chunk.
- **Model misfits** surfaced as explicit open decisions (see "Mechanical work and judgment").

For command replies, record what the documentation claims *and* the open questions hardware must answer -- reply framing and correlation is the area where protocol documentation is least reliable.

*Depends on:* nothing. *Hardware:* none; this chunk can start the day the vendor is chosen, and its output also determines which receiver model to order and what to check when it arrives. *Milestone:* the parent plan and the chunk-plan skeletons exist (see "Plan documents").

### Minimal exploration slice

A deliberately thin cut across the protocol codec and low-level configuration chunks, existing to unblock hardware exploration:

- **Scanning and packet logging, without decoding.** The `gpsprot.PacketFormat`(s) and their registration in `gps/gpsreg`, plus a do-nothing `PacketProcessor` that routes every packet to `NativeMsgHandler.NativeMsg`, so the scanner frames and tags the vendor's packets and packet logs capture them.
- **Command transmission, without correlation.** Enough to get documented commands onto the wire byte-correctly: nothing new for an ASCII command channel (the existing line machinery serves), but a binary command protocol needs its packet message type in `gps/msgfile` so valid command packets can be constructed. No response analyzer, no correlation.

*Depends on:* documentation analysis. *Hardware:* none to build it; it exists to be used with hardware. *Milestone:* `satpulsetool scan` frames and tags the vendor's packets in a capture, and a documented command reaches the receiver.

### Hardware exploration

Manuals routinely diverge from what the hardware implements: receivers often implement a subset of the documented protocol, and reply behaviour is typically under-documented. This chunk establishes ground truth before the expensive chunks commit to the documented picture:

- Log what the receiver actually emits, by default and as messages are enabled.
- Send documented commands and observe the actual replies: framing, ack/nak forms, readback, completion signalling.
- Run the vendor's own application against the receiver through the satpulsed TCP proxy and log the packets. Seeing what the official tool sends and what the receiver answers is often the fastest route to under-documented reply behaviour and to the message set the vendor treats as canonical.

Findings go back into the plans as dated observed facts (see "Plan documents") and scope the codec's message set and the low-level configuration reply handling.

*Depends on:* the minimal exploration slice. *Hardware:* the receiver -- this chunk is the reason to want it on the bench early, before the decode work is finished. *Applies:* usually; skippable when the documentation proves comprehensive and accurate enough to build against directly.

### Protocol codec

The full `gps/lib/<proto>bin` or `gps/lib/<proto>msg` package (see its code-area section below), implementing the triaged message set: a decode struct per in-scope message, name-only registration for the excluded ones, the entry points and protocol-level helpers, plus the per-format decode branch in `gps/gpsdecode`.

*Depends on:* the documentation analysis (the message triage); scoped by hardware exploration's findings where they diverge from the documentation. *Hardware:* none -- documentation and sample captures suffice. *Milestone:* `satpulsetool decode` and `annotate` work end to end on a capture: every in-scope message decodes to a typed struct and appears in JSON, with nothing yet mapped to gpsprot Msgs.

### Msg conversion

The `gps/internal/<vendor>` conversion of decoded messages into the gpsprot Msg set (see its code-area section below): per-message converters, navigation-epoch aggregation, time/leap-second/PPS handling, solution-quality mapping, satellite numbering, and the observation-side signal mapping.

*Depends on:* the protocol codec. *Hardware:* documentation and captures, except PPS/sawtooth sign and startup behaviour, which documentation is often too ambiguous to settle -- hardware decides those. *Milestone:* driving satpulsed from a recorded log (the `drive-satpulsed-from-log` skill) yields the Msg stream: time, position, satellites, epoch summaries.

### Low-level configuration

Two halves of different natures (see the `gps/msgfile` code-area section below). **Command coverage** -- the message files under `configs/gpsmsg/<vendor>/` -- is documentation-driven and can start from the minimal slice, in parallel with the codec. **Response handling** -- reply framing, the response analyzer, correlation, completion -- is typically *discovered* on hardware rather than read out of the documentation; hardware exploration feeds it.

*Depends on:* the minimal slice (the command channel); hardware exploration for the response half; the codec only when commands travel in the same wire format as the data. *Hardware:* needed for the response half. *Milestone:* tagged commands round-trip with correlated replies against the receiver (`gps-msg-test`, then `hardware-test-gps-msgs`).

### High-level configuration

The `gpsprot.ConfigProtocol`/Configurator (see its code-area section below; the `implement-configprotocol` skill covers the process).

*Applies when both hold:* the protocol can support the Configurator model -- a state-neutral probe and queryable current configuration state -- and the vendor matters enough to justify the effort. Skipping this chunk is a normal, settled outcome, not a gap: message files remain the supported configuration endpoint for such a vendor.

*Depends on:* low-level configuration (the command channel and its reply framing) and the codec (probe and version/capability decode). It does not depend on Msg conversion: signal configuration here uses the coarse configuration-side `gpsprot.Signal`/`SignalSet` mapping, not the observation-side `SignalID` table. *Hardware:* the characterisation genuinely needs the receiver (`gpshwtest`). *Milestone:* the property/operation surface (`--gnss`, `--pps`, survey, ...) works against the receiver and the characterisation is recorded.

### RINEX observation conversion

`gps/lib/rnx<vendor>` plus its wiring into `internal/convobscmd` (see its code-area section below), so `satpulsetool convobs` reads the new format.

*Applies when both hold:* the receiver emits raw observations, and converting them is worth the effort for this vendor. *Depends on:* the codec's raw-measurement decode and `gps/lib/rinex`; not on Msg conversion. *Hardware:* none -- captures suffice. *Milestone:* `satpulsetool convobs` converts a capture and the output is accepted by standard RINEX tooling.

## Protocol shapes

The graph above is drawn for the full shape: a vendor data format plus a configuration interface. The documentation analysis places a vendor on this spectrum, and the graph shrinks accordingly:

- **Full vendor protocol** (a binary or ASCII data format plus configuration): the whole graph.
- **Proprietary NMEA extension** (the vendor's data and configuration ride proprietary NMEA sentences): no scanner of its own. The codec collapses into the payload parser, Msg conversion into an `nmea.ExtSentenceHandler`; the minimal slice is nearly free because NMEA scanning and packet logging already exist; low-level configuration keeps its shape via the proprietary NMEA classifier.
- **Config-only** (the data output is entirely standard NMEA; the proprietary protocol exists only to configure): the codec, Msg conversion, and RINEX chunks vanish, and the graph collapses to analysis, exploration, low-level configuration, and optionally high-level configuration. What remains of the codec is the payload parsing for config commands and replies.

Mixed vendors sit between shapes -- plain NMEA data plus one or two proprietary sentences carrying real data, say -- so treat the shapes as points on a spectrum the analysis places you on, not a forced classification.

Correction-carrying protocols (e.g. SPARTN) are a different kind of bring-up and are not covered by this document.

## Plan documents

A bring-up is planned as a parent plan plus one plan per chunk, following the conventions in `plan/CLAUDE.md`. The parent plan is an index, not a design doc: the vendor's protocol shape and the decisions on the optional chunks, the chunk list with its dependency edges, the hardware situation (which model is on the bench, which model-specific paths wait for other hardware), and the supported model list. The always-applicable chunks are checkboxes in one issue; each optional chunk is its own issue with its own plan.

Chunk plans share a skeleton: problem and scope; the design; model differences, when the vendor has several models sharing the protocol; phasing within this plan; testing; open decisions. Note the two levels of phasing: the graph orders chunks against each other, while each chunk plan additionally phases its own work internally -- do not conflate them.

Hardware findings are recorded in the plan as dated observed facts, kept distinct from what the documentation claims; when hardware contradicts the documentation, the plan records both. Model misfits are surfaced in the plan as explicit open decisions, never resolved in passing (next section).

## Mechanical work and judgment

The chunks divide by whose vocabulary they live in. The protocol codec and the message files stay entirely in the vendor's vocabulary, so their correctness is checkable against the vendor documentation alone, and with the existing packages as models the work is mostly mechanical. Judgment concentrates at the two translation boundaries where vendor semantics meet SatPulse-defined semantics: mapping vendor messages onto the gpsprot Msg set (Msg conversion), and mapping vendor configuration onto the `configtarget.go` properties and operations (high-level configuration). There the correspondence is many-to-many and approximate -- what bounds an epoch, which time source to trust, what a quality enum actually claims, whether a vendor knob implements a property or only approximates it -- and a wrong call produces a plausible-looking mapping that is subtly wrong.

Inevitably -- though decreasingly, as the model accretes -- a vendor feature will not fit the device-independent model: a message with no Msg home, a quality state that straddles the `CorrKind` lattice, a wire structure the codec conventions do not cover, a configuration knob with no matching property. That is a design decision, not an implementation detail. The choices are to keep it native (`NativeMsg`, a vendor-specific tag, best-effort or absent in the Configurator) or to extend the model (a Msg field, an enum value, a codec hook, a configtarget property). Extending the model changes SatPulse's semantics for every vendor, so it is a deliberate design step in its own right: never stretch the vendor's meaning to force a fit, and never widen the model casually from one vendor's viewpoint. In a bring-up plan, surface these misfits as explicit open decisions rather than resolving them in passing.

## `gps/lib/<proto>bin` or `gps/lib/<proto>msg` -- protocol decode/encode

The pure protocol codec: decode/encode Go structs plus the protocol-level helpers needed by packet processors and message-file response analyzers, and nothing else. It imports only stdlib and sibling `gps/lib/*` -- never `gpsprot`/`scan`/`internal`. Use a `*bin` package for binary formats (`gps/lib/ubxbin`, `gps/lib/casbin`, `gps/lib/rtcmbin`, `gps/lib/sdbpbin`) and a `*msg` package for ASCII/text message protocols (`gps/lib/novmsg`, `gps/lib/uncmsg`, `gps/lib/qtmmsg`, `gps/lib/airmsg`). What to build here:

- **Framing and checksum** helpers when the protocol owns a packet format: sync byte(s), header/length or line layout, and the checksum algorithm with the exact byte range it covers. Proprietary NMEA extensions do not own this layer; `gps/lib/nmeamsg` and the NMEA `PacketFormat` already frame and validate the sentence, so the vendor package parses the NMEA payload between `$` and `*HH`.
- **The message-ID scheme**, a registry, and `ParseMsg`/`Serialize`/`PackMsg`.
- **A decode struct per message**, with per-field detail (wire type, Go type, units, scale, sentinel), plus any variable-length sub-block handling.
- **Wire-format ownership.** All knowledge of a binary wire format belongs in its `*bin` package. Every value or encoding rule a higher layer needs -- message IDs, enum codes, unavailable-data sentinels, bitfield masks/shifts, field offsets, units/scales, and command/response classifiers -- is defined there as an exported constant, type, method, or small helper. Higher layers (`gps/internal/<vendor>`, config, RINEX, `gps/msgfile`) reference those definitions rather than repeating protocol constants or arithmetic. The corresponding knowledge for a text protocol generally belongs in its `*msg` package.
- **Wire field types and constants.** Give a protocol field a named integer type when its protocol-defined values form an enum, flag set, mask, or selector. Use that type on the decode struct field, and make the first constant in each block explicitly typed so the rest inherit the type. Normally name the type from the message and field, following the compact `ubxbin` pattern (`CfgNav5DynModel`, `CfgPrtMode`, `CfgRstResetMode`), rather than inventing a longer semantic description. When several structs use the same protocol-defined value domain, reuse one shared type instead; GNSS and signal IDs are common examples. Share a type only when the fields have the same protocol semantics, not merely because their numeric encodings coincide. Keep ordinary scalar fields as the corresponding built-in Go type. A unit, scale, or lone DNU value does not justify a named type; define the DNU value as a constant in the codec package, following `sbfbin` constants such as `TOWDNU` and `CovDNU`. Apply the same rule to exported protocol-value fields in `*msg` packages, but not to private types whose purpose is implementing text parsing or formatting.
- **Encoding conventions** (these vary per protocol -- extract them from the doc, do not assume): how *unavailable data* is marked (per-field sentinel, per-type sentinel, validity flag, NaN) and whether that differs from a clipped/saturated value; whether the message identifier packs a revision/variant that must be masked before dispatch, or newer firmware appends trailing fields (treat the declared length as authoritative); and whether repeated/variable-length sub-structures are strided by a runtime length field rather than a fixed struct size, possibly nested. SBF and CASIC exhibit all three, but as examples, not universal rules.
- **Variant-specific values and wire arithmetic.** Keep constants separate when protocol or firmware variants give the same numeric value different meanings; a shared number is not a shared semantic value. The codec also owns documented wire units, scales, and encoding arithmetic. Expose these as named constants or small helpers when a caller would otherwise repeat protocol arithmetic, without wrapping the underlying scalar in a new type solely to express its unit.
- **JSON field names match the vendor doc.** Give each decode struct field a JSON tag naming it exactly as the vendor's reference guide names it, so the JSON emitted by `gps/gpsdecode`/`internal/decodecmd` is directly cross-referenceable against the guide's message tables.
- **Data-centric, light on methods.** These packages are deliberately data-oriented: decode structs expose their fields directly and export almost no methods. The method surface is essentially interface conformance (`ID()`, any varying/chunked hooks), message-ID helpers, and a *few* small accessors where a consumer needs a derived value (e.g. an epoch key, or a sub-field extracted from a packed bitfield). Enum/bitfield fields are named types with exported constants but no `String()`/`MarshalText` (they serialize as their raw numeric code, matching the doc); the only `String()` is typically on the message-ID type. Don't add getters/setters that merely wrap field access. See `gps/lib/ubxbin`, `gps/lib/casbin`.

## `gps/internal/<vendor>` -- scanning, message construction, and mappings

This package (named for the vendor or for the protocol family -- vendors that share a protocol share the package -- and also hosting config-side code that is not the binary protocol) does most of the domain work.

- **Packet-format identification.** How many distinct wire packet formats are needed, and their framing. A standalone binary or ASCII protocol gets one or more `gpsprot.PacketFormat`s: binary data, line-based command replies, state/readback lines, or prompt/completion tokens as needed. A proprietary NMEA extension (`P...` sentences) is different: it normally reuses the common NMEA packet format and adds an `nmea.ExtSentenceHandler` in `gps/internal/<vendor>` rather than adding a new scanner format (model: `gps/internal/quectel` + `gps/lib/qtmmsg`). The existing NMEA format only frames sentences ending in a `*HH` checksum, so checksum-free replies that carry ack/nak, readback, or completion state need real `PacketFormat`s and tags (model `gps/internal/nov/abbrevasciipacket.go`) unless an existing format genuinely frames them. Do not use the scanner's `Format == nil` / `EmptyTag` path as a protocol boundary: it is best-effort display of unrecognized printable bytes, not a reliable response-framing contract. If a protocol has headerless state lines or an unterminated prompt, settle how those are framed before relying on them for response data or command completion. Each standalone format is a `gpsprot.PacketFormat` (a `Next` state machine plus `IsFinal`/`MsgID`/`ExtractChecksum`/`ComputeChecksum`/`RescanOnBadChecksum`/`IsBinary`), driven by `gps/scan.Scanner`. Note the scanner commits to a format on the first byte and only differentiates later, so formats sharing a lead byte need careful ordering: put the more specific format early enough that it is selected directly rather than only after a looser format fails and the scanner retries later formats from the same start byte.
- **Constructing the gpsprot Msg set.** A `PacketProcessor` (embedding `gpsprot.DefaultPacketProcessor`) whose `ProcessPacket` parses via `<proto>bin.ParseMsg` or `<proto>msg.ParseMsg`, dispatches into gpsprot Msgs through per-message converter functions (field-by-field construction), and routes the rest to `NativeMsgHandler.NativeMsg`. For proprietary NMEA extensions, implement `nmea.ExtSentenceHandler.HandleSentence`: validate that the sentence is the vendor's proprietary family, parse the payload with `gps/lib/<proto>msg`, return gpsprot Msgs and epoch state to the NMEA processor, and return `gpsprot.ErrNotHandled` for unrelated or unrecognized sentences. Set `MsgPriority` on emitted position/velocity/epoch messages. Models: `gps/internal/ubx`, `gps/internal/nov`, `gps/internal/quectel`.
- **Navigation-epoch aggregation.** Accumulate a `NavEpochMsg` and implement `gpsprot.EpochFlusher.FlushNavEpoch`; the shared `gpsprot.NavEpochManager` flushes and merges by priority with the NMEA-derived epoch. Determine the epoch key (header week+tow, or a per-message epoch field) and what each boundary signal actually bounds. A TOW increase or an explicit whole-receiver end marker can call the manager's all-active flush path (`EpochStarted`/`EndOfEpoch`). A marker that only ends one protocol's PVT family must not flush other active protocols, because NMEA or satellite messages for the same epoch may still be in flight; keep that protocol's accumulator ready and defer the merge until a whole-receiver boundary, adding a narrower manager helper if needed. `SatellitesMsg` is a separate stream with its own boundaries, not part of `NavEpochMsg`; populate `NavEpochMsg` satellite-count/GNSS/band fields only from the protocol's PVT or quality messages unless the protocol really defines those satellite messages as epoch inputs. If satellite data spans several messages, combine into one `SatellitesMsg` (model: `ubxsats.go`).
- **Time, leap seconds, and PPS.** Convert time-of-week + week using the protocol's documented time convention; do not assume a constellation/time-system field changes the header epoch, because some protocols use one fixed convention for every block. Avoid applying constellation offsets twice. Populate `LeapSecondMsg` only from messages that identify a leap-second event or broadcast UTC model with enough information to date it; a running GNSS-UTC offset with no boundary date belongs in `TimeMsg.UTCOffset`, not in `LeapSecondMsg`. `TimeMsg` sources are often complementary rather than competing: navigation solution time, UTC-offset time, and pre-/post-pulse timing may all be useful. Set `Ref` and `PulseOffset` from the receiver's PPS quantization/sawtooth report, using `TimeMsg`'s contract (`trueTime = pulseTime + PulseOffset`) to decide the sign, and handle the startup window where individual time fields are unavailable. Models: `ubxtime.go`, `nov/time.go`.
- **Solution-quality mapping.** Map fix-mode/quality onto `NavEpochMsg`'s `FixLevel`, `SolutionDim`, `CorrKind`, `AuxSrc`, and DOP. `CorrKind` (`gps/gpsprot/msg.go`) has a documented lattice covering OSR (RTK, dual-frequency), SSR (SBAS, CLAS, SPARTN), and PPP with source distinction (`CorrPPPHAS`, `CorrPPPMDC`, `CorrPPPB2b`, converging/converged) -- so a PPP-with-source solution reports through the standard path if the protocol identifies the source. Models: `nov/quality.go`, `ubx/ubxpv.go`.
- **Satellite numbering.** Map the per-constellation satellite-ID scheme to `gpsprot.SVID` and to RINEX satellite numbers (model `ubxbin/rinex.go` `RINEXSatNum`). If the receiver's NMEA output uses a vendor-specific SV-numbering scheme, add it (models: `gps/internal/sino`, `gps/internal/as`) and wire via `gpsreg.FindNMEASVNumbering`.
- **Observation-side signal mapping** -- what you *observe*, as distinct from what you *enable* (the configuration-side `gpsprot.Signal`/`SignalSet` mapping in the `gps/msgfile` section; the two are different types at different granularities, and conflating them is a recurring point of confusion). The vendor's per-signal identifier as it appears in measurement/satellite messages maps to `gpsprot.SignalID` (used in `SatellitesMsg`) and to the two-character RINEX observation code (used by the converter below; model `ubxbin/rinex.go` `RINEXSig`). One vendor identifier, two representations. A receiver may report one fixed component per signal, or split data/pilot, or use a different granularity in different message types -- read it out of the doc per signal; do not assume.

## `gps/gpsprot` -- the device-independent targets

You do not usually add code here, but this is what everything maps to and from: the `Msg` set and `MsgHandler` (table above), and the configuration target `configtarget.go` (the device-independent properties/operations that high-level config drives). Read these to know the shape of what your converters and Configurator must produce.

## `gps/lib/rnx<vendor>` -- RINEX observation conversion

If the receiver emits raw observations, add a converter for RINEX **observations** (not navigation files), analogous to `gps/lib/rnxubx`/`rnxunc`/`rnxrtcm`. A `Converter` takes the decoded raw-measurement message and emits `gps/lib/rinex.SignalObservation` records, using the observation-side signal mapping and the RINEX satellite numbering. Depends only on the protocol codec package. The converter is not reachable until it is wired into `internal/convobscmd`: a new `inputFormat` value, its packet tag in `inputPacketTags`, converter construction in the packet-input paths (including auto-selection under the default `raw` input), and the `satpulsetool-convobs` man-page format list.

## `gps/gpsreg` -- registration

Wire everything into `gps/gpsreg/reg.go`: the `Vendor` enum value; the `Tag` re-exports for standalone packet formats; the `PacketFormat`(s) in `allVendorPacketFormats` and the per-vendor map when the protocol has its own scanner formats; `<vendor>.NewPacketProcessor(mgr)` in `CreatePacketProcessors` (sharing the one `NavEpochManager`); optional `SetVendor` setup (SV numbering, protocol variant); and, for high-level configuration, the `ConfigProtocol` in `CreateConfigProtocols`. For proprietary NMEA extensions, there is usually no new `PacketFormat` or tag; instead, register the vendor extension handler on the common NMEA packet processor in `CreatePacketProcessors` (model: `nmeaPP.AddExtHandler(quectel.NewHandler())`). Registration is not a chunk of its own: each chunk lands the registration lines it needs.

Separately, wire decoding into `gps/gpsdecode/gpsdecode.go`: each standalone format gets a decode branch dispatching on its `PacketFormat` (model: `casicDecode`), and proprietary NMEA payload decode hooks into `nmeaDecode` there.

## `gps/msgfile` + `configs/gpsmsg/<vendor>` -- low-level configuration

Two sub-dimensions.

**Command coverage.** Author the message file(s) under `configs/gpsmsg/<vendor>/`: a single `<vendor>.toml` when one file covers the product line, or one file per model or protocol family when models differ (models: `configs/gpsmsg/zhongke/`, `configs/gpsmsg/quectel/`). Map every standard tag in `configs/gpsmsg/tags.md` to the vendor's command(s). For each category, find the command or mark it unsupported/approximate rather than inventing one. The categories:

- version query
- NMEA message control (per-sentence enable/disable, all-off, a daemon set) and NMEA version selection
- elevation mask
- constellation selection
- PPS configuration
- fix rate
- port / baud
- restart (hot / warm / cold)
- configuration management (save / reload / reset / factory-reset)
- survey-in and fixed position (base station)
- RTCM output and reference-station ID
- PPP source selection -- receivers vary widely; the dominant flavour is usually a specific service (e.g. a Galileo/HAS or QZSS/MADOCA or BeiDou/B2b service), often configured by enabling the correction signal plus a PPP positioning mode rather than a single "PPP" command, and frequently model- or licence-gated
- nav-message authentication (OSNMA)
- RTK mode (base / rover)

Also add vendor-specific tags, including a tag that enables exactly the messages satpulsed needs (analogous to `nmea-daemon`). Receivers genuinely differ across these (no separate fix-rate knob, no configurable survey threshold, no ephemeris-only warm start, PPP present only on some models), so the mapping is real analysis, not translation. Where models share a protocol but differ in command surface or capability, split shared and model-specific files or includes rather than letting one model's command set leak into another.

**Response handling.** Add the code that understands command replies. The correlator (`gps/msgfile/correlate.go`) dispatches each received packet to a `responseAnalyzer` keyed by tag and matches acks/naks via `ackTag`/`ackCorrelate`/`expectAck` (models: `gps/msgfile/ubx.go` binary ack/nak, `unc.go` ASCII-sentence ack). Determine: which real packet tag(s) replies arrive on; how ack/nak/wait/data/completion packets are distinguished; the correlation key (often the echoed command -- confirm verbatim vs normalised); whether there are multiple reply-frame shapes needing separate branches; whether the protocol permits pipelining or is single-flight; and how end-of-response is signalled. For checksum-free ASCII, do not rely on `EmptyTag`; give the replies, state/readback lines, and any prompt/completion token real framing before depending on them. For a single-flight protocol whose ack text is not a stable echo, a shared correlation key can be correct because `Correlator.ReadyToSend` serializes requests with the same `(ackTag, ackCorrelate)`. Do not treat an ack as command completion when the protocol says to wait for a prompt, command-done line, fixed response count, or other explicit completion signal; otherwise the tool can send the next command too early or report partial readback.

How this wires in depends on the message-file form:

- Standalone binary or ASCII command formats use a request analyzer on the message type (`UBXMsg`, `LineMsg`, etc.) plus a `responseAnalyzer` in the correlator map. A line-based command family with selectable behavior gets a `ResponsePattern` value, a request analyzer (`gps/msgfile/<proto>.go`), a correlator-map entry, and any schema enum updates needed by TOML validation.
- Proprietary NMEA command families (`P...` sentences) usually use `[[nmea]]` entries and the common NMEA tag. Add a `proprietaryNMEA` classifier keyed by the 3-letter vendor code in `gps/msgfile/nmea.go`, with protocol-specific classification in `gps/lib/<proto>msg` (models: `gps/msgfile/pqtm.go`, `gps/lib/qtmmsg`). Do not add a `ResponsePattern` just to classify checksummed proprietary NMEA replies.

This is also where the **configuration-side signal mapping** lands -- what you *enable*, as distinct from what you *observe* (the observation-side `gpsprot.SignalID` mapping in the `gps/internal/<vendor>` section): the device-independent enablement set `gpsprot.Signal`/`SignalSet` (`satpulsetool gps --signal`) maps to the vendor's signal-enablement names and constellation aliases. This set can be coarser or finer than the observation-side identifiers; document the relationship (including one-to-many expansions) explicitly rather than assuming it matches.

## `gps/gpsprot` ConfigProtocol + `gps/internal/<vendor>` -- high-level configuration

Device-independent configuration via a `gpsprot.ConfigProtocol` and Configurator (interfaces in `gps/gpsprot/configprotocol.go`; the property/operation set in `gps/gpsprot/configtarget.go` -- GNSS/signal enablement, PPS, elevation mask, survey/fixed position, nav-message authentication such as OSNMA, etc.). This is an additional layer above message-file configuration, not a replacement for it.

Use the `implement-configprotocol` skill for the common process, semantics contract, director machinery, and verification ladder; do not duplicate that material in the vendor plan. The vendor-specific plan should record only the inputs that skill needs: the verified lower chunks (message files, reply framing, packet library, packet processor), the state-neutral probe and capability source, property-to-command/readback mappings, model/family capability gating, request-correlation and pipelining rules, speed-change/NVM/reset behavior, and which configurable features exist only as best-effort or absence. Prefer receiver-reported capabilities and readback over hardware/model-string branches whenever the protocol exposes them; use model/family branches only for genuine command or wire-format differences. Keep configuration work separate from message processing: enabling a message is configuration, but adding decode/conversion for that message is a separate lower-layer change unless it is already part of the protocol-support phase.

## Testing

Round-trip unit tests per protocol codec package (`go-unit-test` skill); scanner-robustness tests (`gps/internal/scantest`) for protocols that add packet formats; NMEA extension handler tests for proprietary `P...` sentences; a systematic packet-log corpus covering every decode path (`packet-testdata` skill) and driving the pipeline from a log without hardware (`drive-satpulsed-from-log`); decode/annotate spot checks (`satpulsetool decode`/`annotate`); message-file tests (`gps-msg-test`, then `hardware-test-gps-msgs` with hardware); high-level-config characterisation against real receivers (`gpshwtest`); the daemon black-box suite (`smoketest/`). Which of these apply at which point follows from the chunk milestones above.

## New files and registration checklist

- `gps/lib/<proto>bin/` or `gps/lib/<proto>msg/` -- protocol codec.
- `gps/internal/<vendor>/` -- `PacketFormat`(s) and `PacketProcessor`Lets, or an NMEA `ExtSentenceHandler`; converters, sat/signal/time/quality/epoch mappings; reply/readback/completion formats if needed.
- `gps/lib/rnx<vendor>/` -- RINEX observation converter (if raw observations exist), wired into `internal/convobscmd` as a `convobs` input format.
- `gps/gpsreg/reg.go` -- Vendor enum, tag re-exports for standalone formats, packet-format/processor or NMEA-extension registration, optional `SetVendor`/SV-numbering/ConfigProtocol.
- `gps/gpsdecode/gpsdecode.go` -- decode branch per standalone format; proprietary NMEA payload decode in `nmeaDecode`.
- `gps/msgfile/<proto>.go` (one per command family -- a vendor can have more than one) + either a `ResponsePattern` value/correlator-map entry for standalone command replies, or a proprietary NMEA classifier for `P...` commands.
- `configs/gpsmsg/<vendor>/` -- message file(s) mapping standard tags, split per model or protocol family as needed.
- `docs/internals.md` -- a package entry for each new package (required by the repo conventions).
- `docs/_includes/NEWS.md` -- a release-note entry for the user-facing feature.

## References

- Architecture: [internals.md](internals.md); the "tour of the GPS modules" post for the user-facing configuration model.
- Plan/analysis conventions: `plan/CLAUDE.md`.
- Skills: `implement-configprotocol` (high-level configuration), `gps-msg-file` / `gps-msg-add` / `gps-msg-test` (message files), `packet-testdata` (corpus), `drive-satpulsed-from-log` and `satpulsed-test-instance` (no-hardware pipeline runs), `hardware-test-gps-msgs` (with hardware), `satpulsetool` (decode/annotate/config), `go-unit-test` (test style).
- Harnesses: `gpshwtest/` (high-level config characterisation), `smoketest/` (daemon black-box).
