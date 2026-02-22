# Solution quality

Prerequisite: [nav-epoch.md](nav-epoch.md) (adds `NavEpochMsg` with `MsgHandler.NavEpoch`). Implemented: `NavEpochMsg` currently carries `Acc` (accuracy), `Tag`, and `StartTime`.

Related: [multi-prot-nav-epoch.md](multi-prot-nav-epoch.md) (cross-protocol epoch coordination via `NavEpochManager`, which merges per-protocol `NavEpochMsg` contributions when binary + NMEA are both active). Implemented: all `PacketProcessor`s accept `*NavEpochManager` in their constructors and implement `EpochFlusher`.

## Motivation

Modern GNSS receivers expose a wide variety of "fix type" and "quality" indicators across different proprietary protocols and NMEA sentences. Unfortunately, these indicators are inconsistent, overlapping, and often mix multiple conceptual dimensions into a single enum. For example, a single vendor code may conflate measurement type (code vs carrier), correction architecture (RTK vs SBAS vs PPP), ambiguity state (float vs fixed), convergence state (PPP converging vs converged), or even whether the solution is GNSS-based at all. This makes it difficult to build a clean, vendor-neutral abstraction layer that can represent solution quality consistently across devices.

This plan adds metadata fields to the existing `NavEpochMsg` to model the navigation solution along a small number of orthogonal axes. Since this system is fundamentally about GNSS receivers, GNSS is the implicit baseline -- it is always present when a GNSS-based fix is available. The primary ordered axis FixLevel describes the technique used to compute the GNSS solution, ordered by increasing quality (None -> Code -> CodeCorrected -> CarrierFloat -> CarrierFixed), with a special value FixLevelNotMeasured for positions not derived from any navigation solution (e.g. manual input or simulated). An AuxSrc bitmask captures additional sources that contributed to the solution (dead reckoning, INS). Additional independent enums describe correction metadata (CorrKind, a bitmask of correction assertions) and solution dimensionality (FixDim). This approach separates estimator state from raw measurement state (e.g. per-satellite tracking data) and from the low-latency time/position/velocity messages. The result is a normalized, future-proof solution metadata model that can be mapped conservatively from diverse vendor protocols while remaining semantically clean and extensible.

The core navigation outputs -- time, position, and velocity -- must be emitted with essentially zero added latency, because downstream consumers (timing, logging, UI, control loops) often depend on them immediately when they appear. However, many receivers do not provide all "fix quality" metadata in the same message as the solution itself. Instead, the metadata is scattered across multiple protocol messages (or arrives in a different sentence within the same epoch), which means a normalized view of solution quality is inherently an aggregation problem and may only be complete once the remaining messages for that epoch have arrived (and in the worst case, not until the first message of the next epoch establishes the boundary).

The `NavEpochMsg` is already emitted at the end of each navigation epoch (see nav-epoch.md). This plan populates it with synthesized metadata from one or more raw receiver messages. A receiver-specific adapter can combine information from GGA/GSA/GNS (or proprietary status blocks, PPP/RTK state messages, DOP reports, etc.) and populate the `NavEpochMsg` fields when the metadata for an epoch is sufficiently known. Because these properties typically change slowly (fix transitions, correction acquisition, ambiguity resolution, PPP convergence), the epoch-end emission is acceptable and avoids contaminating the timing-critical data path, while still giving applications a coherent, vendor-neutral description of the current navigation mode and quality.

Position and velocity accuracy estimates also belong in `NavEpochMsg` rather than in the individual PVT messages. While many binary protocols (UBX, Allystar) bundle accuracy with position/velocity in the same message, this is not universal. The Quectel LG290P provides accuracy in a separate PQTMEPE message (estimated position error: north, east, down, 2D, 3D) with no position or velocity data. NMEA provides no metric accuracy at all (only DOPs). Since accuracy is not always available at the time position/velocity messages are emitted, it fits naturally into the epoch-end synthesis alongside DOPs and fix quality. For protocols that do bundle accuracy, the values are simply cached and included when the epoch flushes.

Time accuracy, however, stays in `TimeMsg` rather than moving into `NavEpochMsg.Accuracy`. The reasoning is that time accuracy is a property of the time measurement itself, not of the navigation epoch. Time messages come from two distinct sources: navigation-epoch messages (e.g. UBX NAV-TIMEGPS, NAV-PVT) and non-epoch messages (e.g. UBX TIM-TOS, which is a post-pulse timing message unrelated to any navigation epoch). Putting time accuracy in `NavEpochMsg` would either exclude non-epoch time messages or require them to awkwardly write into an epoch they don't belong to. Furthermore, no protocol provides time accuracy separately from the time message itself -- unlike position accuracy (PQTMEPE), there is no "time accuracy only" message that would need epoch-level aggregation. Each time message carries its own accuracy estimate, so `TimeMsg.Accuracy` is the natural home.

## Type definitions

The existing `NavEpochMsg` (which currently contains `Acc`, `Tag`, and `StartTime`) is extended with metadata fields. New supporting types (`FixLevel`, `FixDim`, `CorrKind`, `AuxSrc`, `DOP`) are added to `gps/gpsprot/`. The `Accuracy` type and `Accuracy.Merge` method already exist. The `MergeNavEpoch` function also already exists and will need to be extended to merge the new fields.

```
// NavEpochMsg: new fields added to the existing struct.
// Existing fields: Acc (Accuracy), Tag (Tag), StartTime (time.Time).
//
// GNSS is the implicit baseline source when FixLevel indicates a GNSS-based
// fix. AuxSrc captures additional sources (e.g. DR/INS).
// The zero value of each field means "not provided" or "not applicable".
// CorrKind is a bitmask (not an enum) and its bits are related by a partial
// order (see CorrKind docs).
type NavEpochMsg struct {
	FixLevel       FixLevel             `json:"fixLevel,omitzero"`
	FixDim         FixDim               `json:"fixDim,omitzero"`
	Correction     CorrKind             `json:"correction,omitzero"` // meaningful when FixLevel >= FixLevelCodeCorrected
	AuxSrc         AuxSrc               `json:"auxSrc,omitzero"`
	Acc            Accuracy             `json:"acc,omitzero"`         // already present
	DOP            DOP                  `json:"dop,omitzero"`
	// DiffAge is the age of the differential corrections applied to the
	// current solution. Unset when no corrections are in use or the
	// protocol doesn't report it.
	DiffAge        opt.Val[time.Duration] `json:"diffAge,omitzero"`
	// RTCMRefBaseID is the RTCM reference station ID (DF003, 0-4095) of
	// the base station whose corrections are applied to this solution.
	// Distinct from the RTCMBaseID config property (this receiver's own
	// base ID for RTCM output). Values > 4095 and non-numeric vendor-
	// specific codes (Unicore 9xxx, NovAtel "TSTR") are not stored here;
	// those are used to enrich the Correction bitmask instead.
	RTCMRefBaseID  opt.Val[uint16]       `json:"rtcmRefBaseID,omitzero"`
	NumSVUsed      opt.Val[uint16]      `json:"numSVUsed,omitzero"`
	NumSVTracked   opt.Val[uint16]      `json:"numSVTracked,omitzero"`
	SignalsUsed    SignalSet            `json:"signalsUsed,omitzero"`
	Tag            Tag                  `json:"tag,omitzero"`         // already present
	StartTime      time.Time            `json:"startTime"`            // already present
}

// Accuracy already exists in gps/gpsprot/msg.go with Merge method.
// No changes needed.
type Accuracy struct {
	Pos         opt.Val[Length] `json:"pos,omitzero"`         // 3D position accuracy
	Hor         opt.Val[Length] `json:"hor,omitzero"`         // horizontal position accuracy
	Vert        opt.Val[Length] `json:"vert,omitzero"`        // vertical position accuracy
	Speed       opt.Val[Speed]  `json:"speed,omitzero"`       // 3D speed accuracy
	GroundSpeed opt.Val[Speed]  `json:"groundSpeed,omitzero"` // 2D ground speed accuracy
	Course      opt.Val[Angle]  `json:"course,omitzero"`      // course/heading accuracy
}

// DOP holds dilution of precision values for the navigation solution. Fields
// are opt.Val because different protocols provide different subsets. DOP may
// be synthesized from multiple messages within an epoch (e.g. UBX-NAV-DOP
// provides all five, while NMEA GSA provides only PDOP/HDOP/VDOP).
type DOP struct {
	Geom opt.Val[float64] `json:"geom,omitzero"` // geometric DOP
	Pos  opt.Val[float64] `json:"pos,omitzero"`  // position (3D) DOP
	Hor  opt.Val[float64] `json:"hor,omitzero"`  // horizontal DOP
	Vert opt.Val[float64] `json:"vert,omitzero"` // vertical DOP
	Time opt.Val[float64] `json:"time,omitzero"` // time DOP
}

// AuxSrc is a bitmask of additional data sources that contributed to the
// navigation solution. GNSS contribution is implicit when FixLevel indicates
// a GNSS-based fix.
type AuxSrc uint8

const (
	AuxSrcDR        AuxSrc = 1 << iota // dead reckoning (e.g. wheel ticks, motion model)
	AuxSrcINS                          // inertial navigation system
)

// FixLevel is the primary ordered axis describing the technique used to compute
// the GNSS navigation solution, ordered by increasing quality. Higher values
// represent higher intrinsic precision. The zero value means "not provided"
// or "not applicable".
type FixLevel uint8

const (
	// FixLevelNone indicates that no valid GNSS solution is available.
	FixLevelNone FixLevel = iota + 1

	// FixLevelNotMeasured indicates that the receiver reports a position that
	// is not based on any measurement (e.g. manual input or simulated). No
	// component of the PVT solution is being computed from observations.
	// CorrKind and FixDim do not apply.
	FixLevelNotMeasured

	// FixLevelCode indicates an uncorrected code-based GNSS solution
	// (e.g. standalone SPS or single point positioning).
	FixLevelCode

	// FixLevelCodeCorrected indicates a code-based GNSS solution with
	// corrections applied (e.g. DGPS or SBAS). This improves accuracy but
	// remains limited by code measurement precision.
	FixLevelCodeCorrected

	// FixLevelCarrierFloat indicates a carrier-phase-based solution with
	// ambiguities estimated as float (non-integer) values. This includes
	// RTK float and classical PPP solutions prior to ambiguity fixing.
	FixLevelCarrierFloat

	// FixLevelCarrierFixed indicates a carrier-phase-based solution with
	// integer ambiguities resolved and constrained. This includes RTK fixed
	// and PPP-AR/PPP-RTK solutions.
	FixLevelCarrierFixed
)

// CorrKind is a bitmask of assertions about corrections applied in the navigation
// solution. The zero value means "no correction assertions".
//
// The Corr* bits are not necessarily orthogonal; they are related by a partial
// order. When you assert a more specific fact, you must also assert the more
// general facts it implies.
//
// Partial order definition: A <= B means asserting A implies asserting B.
//
// Ordering (immediate implications):
//   CorrBaseStation <= CorrUsed
//   CorrWideArea <= CorrUsed
//   CorrRTCM <= CorrUsed
//   CorrPartialDualFreq <= CorrBaseStation
//   CorrFullDualFreq <= CorrPartialDualFreq
//   CorrSBAS <= CorrWideArea
//   CorrCLAS <= CorrWideArea
//   CorrSPARTN <= CorrWideArea
//   CorrPPP <= CorrWideArea
//   CorrPPPRTK <= CorrPPP
//   CorrPPPConverging <= CorrPPP
//   CorrPPPConverged <= CorrPPP
type CorrKind uint16

const (
	// CorrUsed asserts that external corrections are applied.
	CorrUsed CorrKind = 1 << iota

	// CorrBaseStation asserts that corrections are base/network referenced.
	// This corresponds to OSR (Observation-State Representation / observation-space)
	// corrections such as RTK/network RTK.
	// CorrBaseStation <= CorrUsed.
	CorrBaseStation

	// CorrWideArea asserts that corrections are wide-area/broadcast/service
	// corrections (not tied to a local base station).
	// This corresponds to SSR (State-Space Representation / state-space) corrections
	// such as SBAS/PPP/CLAS/SPARTN and RTCM-SSR streams.
	// CorrWideArea <= CorrUsed.
	CorrWideArea

	// CorrRTCM asserts that corrections being applied are delivered/packaged as RTCM.
	// CorrRTCM <= CorrUsed.
	CorrRTCM

	// CorrPartialDualFreq asserts that a base-station solution uses a dual-frequency
	// ambiguity strategy that partially exploits multiple frequencies (e.g. wide-lane
	// techniques) rather than fully resolving the full dual-frequency ambiguity set.
	// CorrPartialDualFreq <= CorrBaseStation.
	CorrPartialDualFreq

	// CorrFullDualFreq asserts that a base-station solution uses a dual-frequency
	// ambiguity strategy that fully exploits multiple frequencies (e.g. narrow-lane
	// or ionosphere-free dual-frequency models, as opposed to wide-lane-only strategies)
	// CorrFullDualFreq <= CorrPartialDualFreq.
	CorrFullDualFreq

	// CorrSBAS asserts that the wide-area corrections being applied are SBAS.
	// CorrSBAS <= CorrWideArea.
	CorrSBAS

	// CorrCLAS asserts that the wide-area corrections being applied are CLAS.
	// CorrCLAS <= CorrWideArea.
	CorrCLAS

	// CorrSPARTN asserts that the wide-area corrections being applied are SPARTN.
	// CorrSPARTN <= CorrWideArea.
	CorrSPARTN

	// CorrPPP asserts that the wide-area corrections being applied are PPP-class
	// (standalone absolute solution regime with convergence).
	// CorrPPP <= CorrWideArea.
	CorrPPP

	// CorrPPPRTK asserts that the PPP corrections being applied are PPP-RTK/SSR-RTK
	// class (PPP with additional information enabling rapid ambiguity resolution).
	// CorrPPPRTK <= CorrPPP.
	CorrPPPRTK

	// CorrPPPConverging asserts that the PPP solution is converging.
	// CorrPPPConverging <= CorrPPP.
	CorrPPPConverging

	// CorrPPPConverged asserts that the PPP solution is converged.
	// CorrPPPConverged <= CorrPPP.
	CorrPPPConverged
)

// FixDim describes the dimensionality of the GNSS navigation solution. This is
// primarily about position dimensionality (2D/3D), but includes "time-only" and
// "velocity-only" cases for receivers that report those explicitly.
// The zero value means "not provided" or "not applicable".
type FixDim uint8

const (
	// FixDim2D indicates a two-dimensional solution where height is fixed
	// or constrained and only horizontal position is solved.
	FixDim2D FixDim = iota + 1

	// FixDim3D indicates a full three-dimensional solution where
	// position and clock bias are fully estimated.
	FixDim3D

	// FixDimTimeOnly indicates a solution where only time (and possibly
	// clock bias) is solved, without a valid position.
	FixDimTimeOnly

	// FixDimVelocityOnly indicates a solution where only velocity is solved
	// (e.g. from Doppler), without a valid position or time.
	FixDimVelocityOnly
)
```

## Message enablement

For NMEA, users explicitly select which sentences to enable via `NMEAMsgFlags` (GGA, GSA, RMC, etc.), and metadata extraction piggybacks on whichever sentences are enabled. No additional flag is needed.

For proprietary protocols (UBX, Unicore, etc.), the receiver must be told to output the underlying messages that feed the quality synthesis. A new `PVTMsgQuality` flag in `PVTMsgFlags` (`gps/gpsprot/configtarget.go`) requests solution quality messages. It is a message flag (not an option), included in `PVTMsgAny`, and parsed as `"qual"` on the CLI.

### Design note: single quality flag vs separate DOP flag

DOP values require a dedicated protocol message (UBX NAV-DOP, Unicore STADOP, Allystar NavDop) distinct from the messages that carry fix level, accuracy, and corrections (UBX NAV-PVT, Unicore BESTNAV). This raised the question of whether DOP should be a separate `--pvt-out dop` flag.

We decided against a separate DOP flag for these reasons:

- **DOP is part of solution quality.** Users asking "what is the quality of this fix?" intuitively expect DOPs alongside fix level, accuracy, and corrections. Splitting them requires naming the non-DOP subset, and no word cleanly covers "fix level + accuracy + corrections + SV counts but not DOP."
- **They always go together.** No practical use case wants DOPs without fix quality information, or fix quality without DOPs.
- **Negligible protocol cost.** DOP messages are small (~26 bytes for UBX NAV-DOP, ~84 bytes for Unicore STADOP), adding <1% bandwidth overhead at typical baud rates.

A single `quality` flag enables all messages needed to populate every field in `NavEpochMsg`: fix level, dimensionality, corrections, accuracy, DOPs, and SV counts.

## NMEA

NMEA already emits `NavEpochMsg` via the `NavEpochManager` (implemented as part of multi-prot-nav-epoch.md). The NMEA `PacketProcessor` detects epoch boundaries via time-of-day changes in RMC/GGA/VTG/ZDA and calls `mgr.EpochStarted`; extension handlers can also signal `EndOfEpoch`. The `FlushNavEpoch` method returns the accumulated `NavEpochMsg` with `PriGenericHigh`. This plan adds solution quality metadata population to the existing epoch handling.

The NMEA sentences currently handled by the `nmea` package are RMC, VTG, ZDA, GSV, GSA, and GGA. Of these, GGA and GSA carry solution metadata. GGA currently extracts the quality indicator only for the "no fix" check (field 5 = "0") and position/height; it does not extract numSV, HDOP, DiffAge, or RefStationID. GSA currently extracts only used SVIDs and GNSS; it ignores the fix mode and DOP fields.

The relevant sentences and their fields are:

### GGA (Global Positioning System Fix Data)

| Field | Content | Maps to |
|-------|---------|---------|
| 5 | GPS quality indicator | AuxSrc, FixLevel, Correction |
| 6 | Number of satellites in use | NumSVUsed |
| 7 | HDOP | DOP.Hor |
| 13 | Age of differential GPS data (seconds) | DiffAge |
| 14 | Differential reference station ID (0000-4095) | RTCMRefBaseID |

GGA quality indicator mapping:

| GGA | AuxSrc | FixLevel | Correction |
|-----|--------|---------|------------|
| 0 | 0 | FixLevelNone | 0 |
| 1 | 0 | FixLevelCode | 0 |
| 2 | 0 | FixLevelCodeCorrected | CorrUsed |
| 3 | 0 | FixLevelCodeCorrected | CorrUsed |
| 4 | 0 | FixLevelCarrierFixed | CorrBaseStation |
| 5 | 0 | FixLevelCarrierFloat | CorrBaseStation |
| 6 | AuxSrcDR | FixLevelNone | 0 |
| 7 | 0 | FixLevelNotMeasured | 0 |
| 8 | 0 | FixLevelNotMeasured | 0 |

Notes:
- GGA quality 2 is "Differential GPS fix" but many receivers report SBAS under quality 2 as well, so we cannot determine base-station vs wide-area. Use `CorrUsed` only.
- GGA quality 3 has a muddled history ("PPS fix" originally, sometimes reinterpreted as wide-area). It is not reliably used across receivers, so we treat it the same as quality 2: `CorrUsed` only.
- GGA quality 4 is "Real Time Kinematic" (fixed integers) and quality 5 is "Float RTK". Both are unambiguously base-station/network-referenced solutions (`CorrBaseStation`).

### GSA (GNSS DOP and Active Satellites)

| Field | Content | Maps to |
|-------|---------|---------|
| 0 | Fix selection mode (M/A) | (not used) |
| 1 | Fix type (1/2/3) | FixDim |
| 2-13 | Satellite IDs used | (already used for SatellitesMsg) |
| 14 | PDOP | DOP.Pos |
| 15 | HDOP | DOP.Hor |
| 16 | VDOP | DOP.Vert |
| 17 | System ID (optional, NMEA 4.10+) | (already used) |

GSA fix type mapping:

| GSA fix | FixDim |
|---------|------|
| 1 | no position fix (consistent with FixLevelNone) |
| 2 | FixDim2D |
| 3 | FixDim3D |

### RMC (Recommended Minimum Specific GNSS Data)

| Field | Content | Maps to |
|-------|---------|---------|
| 11 | Mode indicator (NMEA 2.3+) | AuxSrc, FixLevel, Correction |

RMC mode indicator mapping:

| RMC mode | AuxSrc | FixLevel | Correction |
|----------|--------|---------|------------|
| N | 0 | FixLevelNone | 0 |
| A | 0 | FixLevelCode | 0 |
| D | 0 | FixLevelCodeCorrected | CorrUsed |
| P | 0 | FixLevelCodeCorrected | CorrWideArea |
| R | 0 | FixLevelCarrierFixed | CorrBaseStation |
| F | 0 | FixLevelCarrierFloat | CorrBaseStation |
| E | AuxSrcDR | FixLevelNone | 0 |
| M | 0 | FixLevelNotMeasured | 0 |
| S | 0 | FixLevelNotMeasured | 0 |

Notes:
- RMC mode D (differential) is ambiguous: it could be either base-station or wide-area. Use `CorrUsed` only.
- The extended mode indicators R, F, P are part of later NMEA 4.x revisions. When present, they are authoritative (see Synthesis).
- RMC mode P indicates wide-area corrections (e.g. SBAS/PPP). When both GGA and RMC are present, we merge the best information from each (see Synthesis).

### Synthesis

A single NavEpochMsg per epoch is assembled by merging the best available data from GGA, RMC and GSA within the existing `satellitesBuffer` flush cycle.

Neither GGA nor RMC is strictly superior. Each contributes distinct information:
- **GGA** provides NumSVUsed, HDOP, and distinguishes RTK fixed (4) vs float (5).
- **RMC** mode P indicates wide-area corrections, whereas GGA quality 2 vs 3 is unreliable for distinguishing base-station vs wide-area on some receivers.
- **GSA** provides FixDim and DOPs (PDOP, HDOP, VDOP) independently of the quality source.

Merge rules:
- **AuxSrc**: only set when there is a positive claim of an additional contributing source (DR, INS). Pure GNSS fixes leave AuxSrc as zero since GNSS is implicit.
- **AuxSrc, FixLevel, Correction**: if RMC carries an extended mode indicator (R, F, or P), it is authoritative for all three fields and takes precedence over GGA. R/F set `CorrBaseStation`; P sets `CorrWideArea`. Otherwise, prefer GGA when present (e.g. GGA quality 4/5 overrides RMC mode A/D for RTK fixed/float); fall back to RMC only if GGA is absent. For corrected-code solutions (GGA quality 2/3, RMC mode D), set `CorrUsed` only since the correction style is ambiguous.
- **FixDim**: from GSA fix type.
- **DOPs**: PDOP and VDOP from GSA; HDOP from GSA (preferred) or GGA.
- **NumSVUsed**: from GGA.
- **DiffAge**: from GGA field 13 (seconds). Null when DGPS is not used.
- **RTCMRefBaseID**: from GGA field 14. Parse as a number; only set if the value is <= 4095 (valid RTCM 3.x range). Values > 4095 (e.g. Unicore's 9xxx satellite-based service codes) are ignored. Standard NMEA 4.00 defines the range as 0000-1023; RTCM 3.x extends to 0-4095.

The changes needed are:

1. Extend `parseGGA` to extract quality indicator (field 5), HDOP (field 7), differential age (field 13), and reference station ID (field 14) in addition to numSV. `parseGGA` already receives `*NavEpoch`; populate quality fields directly on the epoch.
2. Extend `parseGSA` to extract fix type (field 1) and DOPs (fields 14-16). Currently `parseGSA` returns a `gsaSentence` (SVIDs only) and does not have access to `NavEpoch`. The `satellitesBuffer.gsaProcess` method needs a `*NavEpoch` parameter so that DOP and FixDim values can be written to the epoch as each GSA sentence arrives. Multiple GSA sentences may arrive per epoch (one per constellation); the DOP and FixDim values are per-solution (not per-constellation) and should be identical, so overwriting is safe.
3. Extend `parseRMC` to extract mode indicator (field 11). `parseRMC` already receives `*NavEpoch`; populate quality fields on the epoch.
4. Accumulate the extracted metadata fields on the existing `NavEpoch` struct (which is stored in `curNavEpoch` and already tracks epoch identity via time-of-day changes). The epoch detection and flush mechanism is already in place via `handleEpoch`/`NavEpochManager`; no new `navEpochBuffer` is needed. GSA has no time of day but arrives in the same epoch as the preceding GGA/RMC.
5. On epoch flush (via `FlushNavEpoch`), the accumulated metadata fields from `curNavEpoch.NavEpochMsg` are returned with `PriGenericHigh`. The merge rules above determine how GGA, RMC, and GSA metadata are combined within the `NavEpoch` as each sentence is parsed.

## Quectel PQTM

The Quectel LG290P uses proprietary PQTM periodic messages that are processed as NMEA extension sentences via the `ExtSentenceHandler` interface (`gps/internal/quectel/handler.go`). The handler already emits position, velocity, time, and accuracy from PQTMNAV, PQTMPVT, PQTMVEL, and PQTMEPE. This plan adds solution quality metadata population from PQTMNAV, PQTMPVT, and PQTMDOP.

Since the Quectel handler writes directly to the NMEA `NavEpoch` (which embeds `NavEpochMsg`), quality fields are populated inline as each message is processed, following the same pattern as the existing accuracy population.

### Inputs

Quality metadata comes from:
- **PQTMNAV** (`qtmmsg.NAV`): the primary quality source. Provides `SolType`, `SatUsed`, `SatView`, `DiffAge`, `DiffID`, plus position/velocity standard deviations (already used for accuracy). Priority `PriVendorLow`.
- **PQTMPVT** (`qtmmsg.PVT`): provides `FixType`, `NumSV`, `HDOP`, `PDOP`. Less detailed than NAV (no correction info, no DiffAge). Same priority `PriVendorLow`.
- **PQTMDOP** (`qtmmsg.DOP`): provides GDOP, PDOP, TDOP, VDOP, HDOP. Already parsed by `qtmmsg` but not dispatched to `NavEpochMsg`.

### Mapping: PQTMNAV `SolType` -> `NavEpochMsg`

| SolType | Description | FixLevel | FixDim | Correction |
|---------|-------------|----------|--------|------------|
| 0 | Not fixed | FixLevelNone | 0 | 0 |
| 1 | Single | FixLevelCode | FixDim3D | 0 |
| 2 | SBAS | FixLevelCodeCorrected | FixDim3D | CorrSBAS |
| 5 | Pseudorange differential | FixLevelCodeCorrected | FixDim3D | CorrBaseStation |
| 8 | RTK float | FixLevelCarrierFloat | FixDim3D | CorrBaseStation |
| 12 | RTK fixed | FixLevelCarrierFixed | FixDim3D | CorrBaseStation |

Notes:
- All GNSS-based fix types are 3D; the LG290P does not report 2D fixes via PQTMNAV.
- SolType 2 (SBAS) asserts `CorrSBAS` (which implies `CorrWideArea | CorrUsed` via the partial order).
- SolType 5 (pseudorange differential) asserts `CorrBaseStation | CorrUsed`.
- SolType 8 and 12 (RTK float/fixed) assert `CorrBaseStation | CorrUsed`.

Other fields from PQTMNAV:
- `SatUsed` -> `NumSVUsed`
- `SatView` -> `NumSVTracked`
- `DiffAge` -> `DiffAge` (convert float64 seconds to `time.Duration`)
- `DiffID` -> `RTCMRefBaseID` (only if <= 4095)

### Mapping: PQTMPVT `FixType` -> `NavEpochMsg`

| FixType | Description | FixLevel | FixDim |
|---------|-------------|----------|--------|
| 0 | No fix | FixLevelNone | 0 |
| 2 | 2D fix | FixLevelCode | FixDim2D |
| 3 | 3D fix | FixLevelCode | FixDim3D |

PQTMPVT cannot distinguish correction levels (no SolType field), so `Correction` is always left unset. When both PQTMNAV and PQTMPVT are enabled, PQTMNAV provides the authoritative quality information. Since both use `PriVendorLow`, whichever writes to the epoch fields last wins within the epoch; typically PQTMNAV arrives after PQTMPVT, but to be safe, PQTMNAV should only overwrite fields unconditionally (not conditionally on "is set").

Other fields from PQTMPVT:
- `NumSV` -> `NumSVUsed`
- `HDOP` -> `DOP.Hor`
- `PDOP` -> `DOP.Pos`

### Mapping: PQTMDOP -> `NavEpochMsg`

| DOP field | Maps to |
|-----------|---------|
| GDOP | DOP.Geom |
| PDOP | DOP.Pos |
| TDOP | DOP.Time |
| VDOP | DOP.Vert |
| HDOP | DOP.Hor |

### Where this lives in code

All changes are in `gps/internal/quectel/handler.go`:
1. Extend `msgBundleNAV` to populate quality fields (FixLevel, FixDim, Correction, AuxSrc, NumSVUsed, NumSVTracked, DiffAge, RTCMRefBaseID) on the epoch from PQTMNAV `SolType` and related fields. A helper function (e.g. `qualityNAV`) maps `SolType` to the quality fields.
2. Extend `msgBundlePVT` to populate quality fields (FixLevel, FixDim, NumSVUsed, DOP.Hor, DOP.Pos) on the epoch from PQTMPVT `FixType`, `NumSV`, `HDOP`, `PDOP`. This requires adding the epoch parameter to `msgBundlePVT`.
3. Add a `case *qtmmsg.DOP:` in `HandleSentence` that populates DOP fields on the epoch.

### Message enablement

PQTM message enablement is via message files (`configs/gpsmsg/lg290p.toml`). The tags `pqtm-dop` (already defined) enable PQTMDOP. PQTMNAV is already used for position/velocity/time and carries quality metadata at no additional cost. No programmatic configuration is needed.

## UBX

The UBX `PacketProcessor` already emits a `NavEpochMsg` at each epoch boundary via the `NavEpochManager` (from multi-prot-nav-epoch.md). The `NavEpochManager` is created in `CreatePacketProcessors` and passed to the `PacketProcessor` constructor. Epoch boundaries are detected by iTOW changes in `handleNavEpoch`, which calls `mgr.EpochStarted`. The `FlushNavEpoch` method (implementing `EpochFlusher`) returns the accumulated `curNavEpochMsg` with `Tag=UBX` and `PriVendorLow`. Accuracy fields (`Acc.Hor`, `Acc.Vert`, `Acc.Speed`, `Acc.Course`, `Acc.Pos`) are already populated inline by the position/velocity conversion functions in `ubxpv.go`. This plan adds the remaining solution quality metadata fields.

### Inputs (epoch-keyed by `iTOW`)

Minimum useful input:
- **UBX-NAV-PVT** (`ubxbin.NavPVT`): fix classification (`FixType`), status flags (`Flags`), carrier-solution status (`Flags` bits 6..7), differential flag (`Flags` bit 1), `NumSV`, `PDOP`, and `Flags3` bits 4..1 (`lastCorrectionAge`, bucketed differential correction age).

Recommended additional input for higher fidelity:
- **UBX-NAV-DOP** (`ubxbin.NavDOP`): GDOP/PDOP/TDOP/HDOP/VDOP (all scaled by 0.01).
- **UBX-NAV-SIG** (`ubxbin.NavSig`): per-signal “used” (`NavSigPrUsed`) and per-signal correction source (`CorrSource`), which can disambiguate OSR (base-station) vs SSR (wide-area) and identify specific services/transports (e.g. SBAS, RTCM, SPARTN, CLAS).
- **UBX-NAV-SAT** (`ubxbin.NavSat`): per-SV “used” (`NavSatSVUsed`) and per-SV correction-used flags (e.g. SBAS/RTCM/SPARTN/CLAS), useful as a fallback when NAV-SIG isn’t enabled.

### Mapping: `UBX-NAV-PVT` → `NavEpochMsg`

Epoch association:
- The epoch grouping and `NavEpochMsg` emission point already exist (multi-prot-nav-epoch.md). Quality metadata is populated in `Dispatch` (inline, like existing accuracy population) or synthesized in `FlushNavEpoch` before the return.

Dimensionality (`FixDim`) from `FixType`:

| NAV-PVT fixType | Meaning | FixDim | AuxSrc |
|---|---|---|---|
| 0 | no fix | 0 | 0 |
| 1 | dead reckoning only | 0 | AuxSrcDR |
| 2 | 2D-fix | FixDim2D | 0 |
| 3 | 3D-fix | FixDim3D | 0 |
| 4 | GNSS + dead reckoning | FixDim3D (typically) | AuxSrcDR |
| 5 | time-only fix | FixDimTimeOnly | 0 |

Additionally, if `Flags.headVehValid` is set, add `AuxSrcINS` to `AuxSrc`. This flag indicates the receiver is in sensor fusion mode with an IMU/gyro contributing to the navigation solution. It applies to ADR/UDR products (e.g. F9R) and is never set on pure GNSS or timing receivers.

Fix level (`FixLevel`) and correction presence:
- If `Flags.gnssFixOK` is **not** set, treat the epoch as "no fix": set `FixLevel=FixLevelNone` and leave the rest unset.
- If `FixType` indicates dead reckoning only (`fixType=1`), set `FixLevel=FixLevelNone` (and `AuxSrcDR` from the table above) and do not set correction assertions.
- Otherwise, set `FixLevel` primarily from `Flags.carrSoln` (carrier phase range solution status):
  - `carrSoln = fixed` → `FixLevelCarrierFixed`
  - `carrSoln = float` → `FixLevelCarrierFloat`
  - `carrSoln = none`:
    - if `Flags.diffSoln` is set → `FixLevelCodeCorrected` and assert `CorrUsed`
    - else → `FixLevelCode` with no correction assertions

Correction (`Correction`) disambiguation:
- **Do not** infer `CorrKind` from `NAV-PVT` alone (it only tells you “differential corrections applied”, not the correction style or delivery).
- Prefer inferring `Correction` from **NAV-SIG** in the same epoch:
  - any *used* signal with `CorrSource` in `{RTCM2, RTCM3OSR}` → `CorrBaseStation | CorrRTCM`
  - any *used* signal with `CorrSource` in `{RTCM3SSR}` → `CorrWideArea | CorrRTCM`
  - any *used* signal with `CorrSource` in `{SBAS}` → `CorrSBAS`
  - any *used* signal with `CorrSource` in `{SPARTN}` → `CorrSPARTN`
  - any *used* signal with `CorrSource` in `{CLAS}` → `CorrCLAS`
  - other wide-area sources (e.g. `QZSSSLAS`, `BeiDou`) → `CorrWideArea`
  - if conflicting styles appear (both base-station and wide-area), leave style bits unset (but `CorrRTCM` may still be asserted if RTCM is known).
- If NAV-SIG is unavailable but NAV-SAT is present, use the per-SV correction-used flags as a weaker fallback. NAV-SAT can identify wide-area sources (`NavSatSbasCorrUsed`, `NavSatSlasCorrUsed`, `NavSatSpartnCorrUsed`, `NavSatClasCorrUsed`), but `NavSatRtcmCorrUsed` does not distinguish base-station vs wide-area; if only RTCM is flagged, assert `CorrRTCM` without a style bit.

Accuracy (`Acc`):

Already implemented in `ubxpv.go`. Each position/velocity conversion function populates the corresponding accuracy fields on `curNavEpochMsg` inline:
- `posGeoNavPVT`: `Acc.Hor` (HAcc mm), `Acc.Vert` (VAcc mm)
- `velGeoNavPVT`: `Acc.Speed` (SAcc mm/s), `Acc.Course` (HeadAcc 1e-5 deg)
- `posECEFNavPosECEF`: `Acc.Pos` (PAcc cm)
- `posGeoNavPosLLH`: `Acc.Hor` (HAcc mm), `Acc.Vert` (VAcc mm)
- `velECEFNavVelECEF`: `Acc.Speed` (SAcc cm/s)
- `velGeoNavVelNED`: `Acc.Speed` (SAcc cm/s), `Acc.Course` (CAcc 1e-5 deg)

NAV-PVT is preferred when present because it uses mm/mm/s resolution (vs cm/cm/s for NAV-POSLLH/NAV-VELNED). This is handled by the `MsgPriority` mechanism: `velGeoNavPVT` returns `PriVendorHigh` while `velGeoNavVelNED` returns `PriVendorLow`. No new work needed for accuracy.

Other fields:
- `NumSVUsed`: from `NAV-PVT.NumSV`.
- `DOP.Pos`: from `NAV-PVT.PDOP` (scale 0.01) or from `NAV-DOP.PDOP` (preferred when present for consistency with the other DOPs).
- `DOP.Geom/Hor/Vert/Time`: from `NAV-DOP` (scale 0.01) when available.
- `NumSVTracked`, `SignalsUsed`: derive from the same per-epoch NAV-SAT/NAV-SIG accumulation used to emit `SatellitesMsg` (see `gps/internal/ubx/ubx.go` `satMsg`/`sigMsg` and `gps/internal/ubx/ubxsats.go` `satellitesCombine`), counting unique SVs and the union of signal IDs marked `Used`.
- `DiffAge`: from `NAV-PVT.Flags3` bits 4..1 (`lastCorrectionAge`). The constants are already defined in `ubxbin` (`NavPVTLastCorrectionAge*`) but not currently extracted. This is a bucketed value (12 ranges: 0-1s, 1-2s, 2-5s, ... 120s+), not a continuous measurement. Convert to `time.Duration` using the lower bound of each bucket (e.g. `NavPVTLastCorrectionAge2to5` → 2s). `NavPVTLastCorrectionAgeNotAvailable` (0) leaves `DiffAge` unset.
- `RTCMRefBaseID`: not available from NAV-PVT or any other UBX message in the current plan. NAV-RELPOSNED carries a `refStationId` but is a relative-positioning message unrelated to the correction source. `RTCMRefBaseID` remains unset for UBX binary; it can be populated via NMEA GGA (field 14) when NMEA is enabled alongside binary.

### Where this lives in code

The epoch infrastructure is already in place (multi-prot-nav-epoch.md):
- `PacketProcessor` in `gps/internal/ubx/ubx.go` stores `curNavEpochMsg *gpsprot.NavEpochMsg` for the current epoch.
- `handleNavEpoch` calls `mgr.EpochStarted(p, tRead)` on iTOW change and allocates a fresh `NavEpochMsg`.
- `FlushNavEpoch` (implementing `EpochFlusher`) returns `curNavEpochMsg` with `Tag=UBX` and `PriVendorLow`.
- Accuracy fields are already populated inline in the conversion functions (`ubxpv.go`).

The existing conversion functions in `ubxpv.go` receive `ne *gpsprot.NavEpochMsg` (the `curNavEpochMsg`) and populate accuracy fields on it inline. Quality metadata follows the same pattern:
1. In the existing `case *ubxbin.NavPVT:` in `Dispatch`, populate FixLevel, FixDim, AuxSrc, Correction, NumSVUsed, DOP.Pos, and DiffAge on `curNavEpochMsg` inline (like accuracy). A new function in `ubxpv.go` (e.g. `qualityNavPVT`) receives `ne` and the `NavPVT` and sets the fields. DiffAge is extracted from `Flags3 & NavPVTLastCorrectionAgeMask` and converted to `time.Duration` using the bucket lower bound.
2. Add a `case *ubxbin.NavDOP:` in `Dispatch` to populate DOP fields on `curNavEpochMsg` inline.
3. Correction disambiguation from NAV-SIG/NAV-SAT: as these messages are processed (already in `Dispatch`), accumulate a `CorrKind` bitmask on `curNavEpochMsg` from per-signal/per-SV correction source flags. This enriches the base `CorrUsed` set by NAV-PVT with specific correction style bits (CorrBaseStation, CorrSBAS, CorrRTCM, etc.).

### Message enablement

When `PVTMsgQuality` is set, the UBX message configuration (`gps/internal/ubx/ubxcfgmsg.go`) enables `UBX-NAV-DOP` (GDOP/HDOP/VDOP/TDOP) and ensures `UBX-NAV-PVT` is enabled (the primary source for fix quality, carrSoln, diffSoln, numSV). NAV-SIG and NAV-SAT are already controlled by `SatsMsgFlags`. The cfgval key `KUbxNavDop` already exists in `ubxcfgval/msgkey.go`; it needs to be added to the `msgIDKey` map in `ubxcfgmsg.go`.

## Unicore

The Unicore processor already emits `NavEpochMsg` with position/velocity accuracy via the `NavEpochManager` epoch mechanism. This plan adds solution quality metadata (FixLevel, FixDim, Correction, DOPs, satellite counts) to the existing epoch.

Unicore receivers expose solution metadata primarily through the BESTNAV message, which includes both position and velocity solution types as explicit enums. Unlike UBX, where fix quality must be inferred from multiple flag bits, Unicore's `pos type` field directly encodes the solution technique (code, carrier float, carrier fixed, PPP variants, INS fusion). This makes the mapping to `NavEpochMsg` straightforward from a single message. DOP values require a separate STADOP message.

### Inputs

Quality metadata comes from:
- **BESTNAV** (`uncmsg.BestNav`): already handled in `dispatch()` for position, velocity, and accuracy. New work: populate FixLevel, FixDim, Correction, AuxSrc, NumSV, NumSolnSV, SignalsUsed from the `pos type`, satellite count, and signal mask fields.
- **STADOP** (`uncmsg.StaDOP`): not currently handled. New work: add to `dispatch()` and populate DOPs (GDOP, PDOP, TDOP, HDOP, VDOP, NDOP, EDOP).

### Mapping: BESTNAV `pos type` -> `NavEpochMsg`

The `pos type` field is a single enum that encodes the solution technique. When `p-sol status` is not `SOL_COMPUTED` (0), treat the epoch as "no fix" regardless of `pos type`: set `FixLevel=FixLevelNone` and leave other fields unset.

When `p-sol status` is `SOL_COMPUTED`, map `pos type` as follows:

| pos type | ASCII | Unicore description | AuxSrc | FixLevel | FixDim | Correction |
|----------|-------|---------------------|--------|---------|-----|------------|
| 0 | NONE | No solution | 0 | FixLevelNone | 0 | 0 |
| 1 | FIXEDPOS | Position fixed by FIX command or averaging | 0 | FixLevelCode | FixDimTimeOnly | 0 |
| 2 | FIXEDHEIGHT | Not supported | 0 | FixLevelCode | FixDim2D | 0 |
| 8 | DOPPLER_VELOCITY | Velocity computed using instantaneous Doppler | 0 | 0 | FixDimVelocityOnly | 0 |
| 16 | SINGLE | Single point position | 0 | FixLevelCode | FixDim3D | 0 |
| 17 | PSRDIFF | Pseudorange differential solution | 0 | FixLevelCodeCorrected | FixDim3D | CorrBaseStation |
| 18 | SBAS | Solution using corrections from an SBAS | 0 | FixLevelCodeCorrected | FixDim3D | CorrSBAS |
| 32 | L1_FLOAT | Floating L1 ambiguity solution | 0 | FixLevelCarrierFloat | FixDim3D | CorrBaseStation |
| 33 | IONOFREE_FLOAT | Floating ionosphere-free ambiguity solution | 0 | FixLevelCarrierFloat | FixDim3D | CorrFullDualFreq |
| 34 | NARROW_FLOAT | Floating narrow-lane ambiguity solution | 0 | FixLevelCarrierFloat | FixDim3D | CorrFullDualFreq |
| 48 | L1_INT | Integer L1 ambiguity solution | 0 | FixLevelCarrierFixed | FixDim3D | CorrBaseStation |
| 49 | WIDE_INT | Integer wide-lane ambiguity solution | 0 | FixLevelCarrierFixed | FixDim3D | CorrPartialDualFreq |
| 50 | NARROW_INT | Integer narrow-lane ambiguity solution | 0 | FixLevelCarrierFixed | FixDim3D | CorrFullDualFreq |
| 52 | INS | INS position solution | AuxSrcINS | FixLevelNone | FixDim3D | 0 |
| 53 | INS_PSRSP | INS pseudorange single point solution | AuxSrcINS | FixLevelCode | FixDim3D | 0 |
| 54 | INS_PSRDIFF | INS pseudorange differential solution | AuxSrcINS | FixLevelCodeCorrected | FixDim3D | CorrBaseStation |
| 55 | INS_RTKFLOAT | INS RTK floating point ambiguities solution | AuxSrcINS | FixLevelCarrierFloat | FixDim3D | CorrBaseStation |
| 56 | INS_RTKFIXED | INS RTK fixed ambiguities solution | AuxSrcINS | FixLevelCarrierFixed | FixDim3D | CorrBaseStation |
| 68 | PPP_CONVERGING | PPP solution converging | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverging |
| 69 | PPP | PPP positioning | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverged |
| 70 | PPP_AR | PPP fixed solution (PPP-AR) | 0 | FixLevelCarrierFixed | FixDim3D | CorrPPPConverged |
| 71 | PPP_RTK | PPP fixed solution (PPP-RTK) | 0 | FixLevelCarrierFixed | FixDim3D | CorrPPPRTK |

Notes:
- **FIXEDPOS (1)**: The position was fixed by a user command or averaging. The receiver still tracks GNSS for timing, so `FixDimTimeOnly` since only time/clock is being solved. FixLevel/Correction are not meaningful.
- **FIXEDHEIGHT (2)**: Height is constrained, only horizontal position is solved. Marked as not supported by Unicore but included for completeness.
- **DOPPLER_VELOCITY (8)**: Only a velocity solution from Doppler, without a valid position or time. Use `FixDimVelocityOnly` and leave FixLevel/Correction unset (zero) since this is a velocity-only result.
- **Float variants (32-34)**: L1_FLOAT, IONOFREE_FLOAT, and NARROW_FLOAT differ in the ambiguity parameterization but all represent carrier-phase float solutions. The distinction is not modeled in `FixLevel` (all map to `FixLevelCarrierFloat`). All are base-station solutions (`CorrBaseStation`).
- **Integer variants (48-50)**: L1_INT is a base-station fixed solution (`CorrBaseStation`). WIDE_INT and NARROW_INT represent increasingly full use of dual-frequency ambiguity strategies; map them to `CorrPartialDualFreq` and `CorrFullDualFreq` respectively.
- **INS (52)**: Pure inertial solution with no GNSS contribution. FixLevel/Correction are not meaningful since there is no GNSS fix.
- **INS fusion variants (53-56)**: These are GNSS+INS sensor fusion solutions. `AuxSrcINS` is set to indicate the additional INS contribution. The GNSS quality component follows the same pattern as the non-INS counterpart (PSRSP->Code, PSRDIFF->CodeCorrected, RTKFLOAT->CarrierFloat, RTKFIXED->CarrierFixed).
- **PPP_CONVERGING (68)**: PPP solution that has not yet reached steady-state accuracy. Mapped to `FixLevelCarrierFloat` (ambiguities are float during convergence) with `CorrPPPConverging`.
- **PPP (69)**: Converged PPP solution without ambiguity resolution. This is a carrier-phase float solution (PPP estimates ambiguities as real-valued parameters) that has reached steady-state. Mapped to `FixLevelCarrierFloat` with `CorrPPPConverged`.
- **PPP_AR (70)** and **PPP_RTK (71)**: PPP with integer ambiguity resolution. Both achieve carrier-phase fixed precision via wide-area PPP corrections. PPP_AR uses traditional PPP-AR and maps to `CorrPPPConverged`. PPP_RTK uses additional information enabling rapid ambiguity resolution and asserts `CorrPPPRTK`.

### Accuracy

Already populated by `gps/internal/unc/nav.go`: BESTNAV sigmas -> `Acc.Hor`, `Acc.Vert`, `Acc.GroundSpeed`, `Acc.Speed`; BESTNAVXYZ sigmas -> `Acc.Pos`, `Acc.Speed`. No new work needed.

### Other fields

- **NumSVUsed**: from BESTNAV `#solnSVs`.
- **NumSVTracked**: from BESTNAV `#SVs`.
- **SignalsUsed**: derive from the BESTNAV signal mask fields (`GPS, GLONASS and BDS2 sig mask` and `Galileo&BDS3 sig mask`).
- **DOPs**: GDOP, PDOP, TDOP, HDOP, VDOP from STADOP when available.
- **DiffAge**: from BESTNAV `DiffAge` (float32, seconds, already parsed in `novmsg.Pos`). Convert to `time.Duration`. BESTNAV also has `VDiffAge` for velocity; use the position `DiffAge` (more relevant for correction staleness).
- **RTCMRefBaseID**: from BESTNAV `StnID` (`novmsg.StationID` = `[4]byte`, already parsed in `novmsg.Pos`). Parse the bytes as a decimal string. If the resulting number is <= 4095, set `RTCMRefBaseID`. If >= 4096 (Unicore uses 9xxx values for satellite-based correction services), use the value to enrich `Correction` instead: 9901-9905/9959-9961 (BeiDou B2b) → `CorrWideArea`; 9964 (Galileo E6 HAS) → `CorrWideArea`; 9934-9939 (QZSS L6 MDC) / 9974-9979 (QZSS L6 CLAS) → `CorrCLAS`; 999X (L-band) → `CorrWideArea`. If non-numeric, leave `RTCMRefBaseID` unset (defensive; should not happen for Unicore).

### Where this lives in code

The Unicore processor (`gps/internal/unc/processor.go`) already has epoch detection and `NavEpochMsg` emission via `NavEpochManager`. The existing conversion functions in `gps/internal/unc/nav.go` receive `ne *gpsprot.NavEpochMsg` (the `curEpochMsg`) and populate accuracy fields on it inline. Quality metadata follows the same pattern:
1. Extend `bestNavPosVel` in `nav.go` to populate quality fields (FixLevel, FixDim, Correction, AuxSrc, NumSVUsed, NumSVTracked, SignalsUsed, DiffAge, RTCMRefBaseID) on `ne` from the BESTNAV `pos type`, satellite count, signal mask, `DiffAge`, and `StnID` fields. For quality mapping: handle Unicore-specific values first (52=INS, 70=PPP_AR, 71=PPP_RTK), then delegate to `nov.PosTypeQuality` for shared values (see NovAtel section). For `StnID`, parse as decimal and apply the <= 4095 filter; values >= 4096 enrich `Correction`.
2. Add a `bestNavDOP` function (or similar) in `nav.go` that receives `ne` and populates DOP fields. Wire it from a new `*uncmsg.StaDOP` case in `dispatch()`.

### Message enablement

When `PVTMsgQuality` is set, the Unicore message configuration (`gps/internal/unc/cfgopts.go`) enables `BESTNAVB 1` (if not already enabled by `PVTMsgPos`) and `STADOPB 1` for DOP values. If `PVTMsgPos` already enables BESTNAV, then `PVTMsgQuality` only needs to add STADOP.

## NovAtel

The NovAtel binary protocol is shared by NovAtel OEM7, ByNav, and SinoGNSS (K8/K9) receivers. All three vendors use identical binary message layouts; the `SolStatus` and `PosType` enum value sets vary by vendor. The `novmsg` package defines the superset of values across all three vendors (as untyped constants; see `novatel-variants.md`). All three are handled by a single `PacketProcessor` in `gps/internal/nov/`.

The NovAtel processor already emits `NavEpochMsg` with position/velocity accuracy via the `NavEpochManager` epoch mechanism. This plan adds solution quality metadata (FixLevel, FixDim, Correction, DOPs, satellite counts) to the existing epoch. The primary structural difference from Unicore is that NovAtel uses separate position and velocity messages (BESTPOS + BESTVEL) instead of a combined BESTNAV.

### Inputs

Quality metadata comes from:
- **BESTPOS** (`novmsg.BestPos`, ID 42): already handled in `dispatch()` for position and accuracy. New work: populate FixLevel, FixDim, Correction, AuxSrc, NumSV, NumSolnSV, SignalsUsed from the `pos type`, satellite count, and signal mask fields.
- **PSRDOP** (ID 174): not currently handled. New work: add to `dispatch()` and populate DOPs. Provides GDOP, PDOP, HDOP, TDOP. Does not provide VDOP directly; derive as sqrt(PDOP^2 - HDOP^2).

### Mapping: BESTPOS `pos type` -> `NavEpochMsg`

When `SolStatus` is not `SOL_COMPUTED` (0), treat the epoch as "no fix" regardless of `pos type`: set `FixLevel=FixLevelNone` and leave other fields unset.

When `SolStatus` is `SOL_COMPUTED`, map `pos type` as follows:

| pos type | ASCII | AuxSrc | FixLevel | FixDim | Correction |
|----------|-------|--------|----------|--------|------------|
| 0 | NONE | 0 | FixLevelNone | 0 | 0 |
| 1 | FIXEDPOS | 0 | FixLevelNotMeasured | FixDimTimeOnly | 0 |
| 2 | FIXEDHEIGHT | 0 | FixLevelCode | FixDim2D | 0 |
| 4 | FLOATCONV | 0 | FixLevelCarrierFloat | FixDim3D | CorrBaseStation |
| 5 | WIDELANE | 0 | FixLevelCarrierFixed | FixDim3D | CorrPartialDualFreq |
| 6 | NARROWLANE | 0 | FixLevelCarrierFixed | FixDim3D | CorrFullDualFreq |
| 8 | DOPPLER_VELOCITY | 0 | 0 | FixDimVelocityOnly | 0 |
| 16 | SINGLE | 0 | FixLevelCode | FixDim3D | 0 |
| 17 | PSRDIFF | 0 | FixLevelCodeCorrected | FixDim3D | CorrBaseStation |
| 18 | WAAS | 0 | FixLevelCodeCorrected | FixDim3D | CorrSBAS |
| 19 | PROPAGATED | 0 | FixLevelNone | 0 | 0 |
| 32 | L1_FLOAT | 0 | FixLevelCarrierFloat | FixDim3D | CorrBaseStation |
| 33 | IONOFREE_FLOAT | 0 | FixLevelCarrierFloat | FixDim3D | CorrFullDualFreq |
| 34 | NARROW_FLOAT | 0 | FixLevelCarrierFloat | FixDim3D | CorrFullDualFreq |
| 48 | L1_INT | 0 | FixLevelCarrierFixed | FixDim3D | CorrBaseStation |
| 49 | WIDE_INT | 0 | FixLevelCarrierFixed | FixDim3D | CorrPartialDualFreq |
| 50 | NARROW_INT | 0 | FixLevelCarrierFixed | FixDim3D | CorrFullDualFreq |
| 51 | RTK_DIRECT_INS | AuxSrcINS | FixLevelCarrierFixed | FixDim3D | CorrBaseStation |
| 52 | INS_SBAS | AuxSrcINS | FixLevelCodeCorrected | FixDim3D | CorrSBAS |
| 53 | INS_PSRSP | AuxSrcINS | FixLevelCode | FixDim3D | 0 |
| 54 | INS_PSRDIFF | AuxSrcINS | FixLevelCodeCorrected | FixDim3D | CorrBaseStation |
| 55 | INS_RTKFLOAT | AuxSrcINS | FixLevelCarrierFloat | FixDim3D | CorrBaseStation |
| 56 | INS_RTKFIXED | AuxSrcINS | FixLevelCarrierFixed | FixDim3D | CorrBaseStation |
| 67 | EXT_CONSTRAINED | AuxSrcINS | FixLevelNotMeasured | 0 | 0 |
| 68 | PPP_CONVERGING | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverging |
| 69 | PPP | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverged |
| 70 | OPERATIONAL | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverged |
| 71 | WARNING | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverged |
| 72 | OUT_OF_BOUNDS | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverged |
| 73 | INS_PPP_CONVERGING | AuxSrcINS | FixLevelCarrierFloat | FixDim3D | CorrPPPConverging |
| 74 | INS_PPP | AuxSrcINS | FixLevelCarrierFloat | FixDim3D | CorrPPPConverged |
| 77 | PPP_BASIC_CONVERGING | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverging |
| 78 | PPP_BASIC | 0 | FixLevelCarrierFloat | FixDim3D | CorrPPPConverged |
| 79 | INS_PPP_BASIC_CONVERGING | AuxSrcINS | FixLevelCarrierFloat | FixDim3D | CorrPPPConverging |
| 80 | INS_PPP_BASIC | AuxSrcINS | FixLevelCarrierFloat | FixDim3D | CorrPPPConverged |

Notes:

- **Values shared with Unicore**: Types 0-2, 8, 16-17, 32-34, 48-50, 53-56, 68-69 share the same binary values and semantically equivalent meanings. The mappings match the Unicore table. NovAtel calls value 18 WAAS (Unicore calls it SBAS); both mean SBAS corrections. Value 33 (IONOFREE_FLOAT) is supported by ByNav but reserved in OEM7.
- **FLOATCONV (4)**: ByNav-specific. Floating carrier-phase ambiguity solution that has not yet converged.
- **WIDELANE (5), NARROWLANE (6)**: Legacy NovAtel/ByNav types corresponding to WIDE_INT (49) and NARROW_INT (50) respectively. WIDELANE resolves wide-lane ambiguities to integers; NARROWLANE resolves narrow-lane ambiguities.
- **PROPAGATED (19)**: Position propagated by the Kalman filter without new GNSS observations. No current measurement, so FixLevel is FixLevelNone.
- **RTK_DIRECT_INS (51)**: NovAtel SPAN product. RTK filter initialized directly from INS filter.
- **INS_SBAS (52)**: Differs from Unicore's INS (52), which is pure inertial with no GNSS contribution. NovAtel's INS_SBAS indicates an INS solution with the last GNSS update being SBAS-corrected.
- **EXT_CONSTRAINED (67)**: Position provided by an external source to the INS filter. No GNSS measurement is involved.
- **OPERATIONAL (70), WARNING (71), OUT_OF_BOUNDS (72)**: NovAtel User Accuracy Limitation (UAL) monitoring states for PPP solutions. These indicate whether the PPP solution's estimated accuracy is within user-configured thresholds. The underlying solution technique is still PPP, so all three map to CorrPPPConverged; accuracy degradation is reflected in the Accuracy fields. Note: these values have completely different meanings from Unicore's PPP_AR (70) and PPP_RTK (71).
- **PPP_BASIC variants (77-78, 79-80)**: TerraStar-L service. TerraStar-L provides sub-meter PPP with basic SSR corrections. The receiver still uses carrier-phase measurements in the PPP filter but without the corrections needed for integer ambiguity resolution, so FixLevel is FixLevelCarrierFloat.
- **INS fusion variants** (51-56, 67, 73-74, 79-80): All INS-related types set AuxSrcINS. The GNSS quality component follows the same pattern as the non-INS counterpart.

### SinoGNSS differences

SinoGNSS K8/K9 uses a reduced `PosType` set (no INS types, no PPP_BASIC, no UAL monitoring) and has three values that differ from NovAtel/ByNav. The variant mechanism in `novatel-variants.md` handles this: SinoGNSS uses `SinoPosType` with its own enum values, so the quality mapping operates on the correct vendor-specific types.

The SinoGNSS-specific `pos type` values and their mappings:

- **SINGLE_SMOOTH (9)**: Carrier-smoothed single-point solution. FixLevelCode + FixDim3D (carrier smoothing improves code precision but does not change the fundamental solution technique).
- **FIX_DERIVATION (35)**: Derived/propagated solution. FixLevelCarrierFloat + FixDim3D + CorrBaseStation (a degraded RTK solution).
- **SUPER_WIDE_LANE (51)**: Super wide-lane carrier-fixed solution. FixLevelCarrierFixed + FixDim3D + CorrPartialDualFreq. This value is RTK_DIRECT_INS in NovAtel/ByNav; the variant mechanism ensures the correct type is used for each vendor.

### Accuracy

Already populated by `gps/internal/nov/nav.go`: BESTPOS sigmas -> `Acc.Hor`, `Acc.Vert`; BESTXYZ sigmas -> `Acc.Pos`, `Acc.Speed`. No new work needed.

### Other fields

- **NumSV**: `#SVs` (satellites tracked) from BESTPOS.
- **NumSolnSV**: `#solnSVs` (satellites in solution) from BESTPOS.
- **SignalsUsed**: derive from the BESTPOS signal mask fields (`GPSGLOBDS2Sig` and `GalBDS3Sig`).
- **DOPs**: GDOP, PDOP, HDOP, TDOP from PSRDOP when available. VDOP is not provided by PSRDOP; derive as sqrt(PDOP^2 - HDOP^2).
- **DiffAge**: from BESTPOS `DiffAge` (float32, seconds, already parsed in `novmsg.Pos`). Convert to `time.Duration`.
- **RTCMRefBaseID**: from BESTPOS `StnID` (`novmsg.StationID` = `[4]byte`, already parsed in `novmsg.Pos`). Parse the bytes as a decimal string. If the resulting number is <= 4095, set `RTCMRefBaseID`. If non-numeric (NovAtel uses alphabetic codes for PPP services: "TSTR" = TerraStar-C PRO, "TSTL" = TerraStar-L, "TSX" = TerraStar-X, "OCXH" = Oceanix), use the value to enrich `Correction` with `CorrPPP`. If numeric but >= 4096, leave `RTCMRefBaseID` unset (defensive; should not happen for NovAtel). The variant mechanism ensures SinoGNSS uses its own `PosType` but the `StnID` handling is identical across all NovAtel-format vendors.

### Shared PosType quality mapping

NovAtel OEM7, ByNav, Unicore, and SinoGNSS share a large set of PosType numeric values with identical semantics (see `novatel-variants.md` Step 2 for the untyped constants in `novmsg`). Rather than duplicating the PosType-to-quality mapping in both `nov/` and `unc/`, a shared mapping function lives in `gps/internal/nov/` (since `unc/` already imports `nov/`).

**`gps/internal/nov/quality.go`**:

```go
// PosTypeQuality maps a NovAtel/Unicore PosType numeric value to quality fields.
// Returns false for vendor-specific values that the caller must handle.
func PosTypeQuality(pt uint32) (gpsprot.FixLevel, gpsprot.FixDim, gpsprot.CorrKind, gpsprot.AuxSrc, bool)
```

This function handles values shared across all vendors: 0-2, 8, 16-18, 32-34, 48-50, 53-56, 68-69. It returns `false` for vendor-specific values (OEM7: 4-6, 19, 51-52, 67, 70-80; Unicore: 52, 70-71; SinoGNSS: 9, 35, 51).

**NovAtel** calls `PosTypeQuality(uint32(posType))`; on `false`, handles OEM7-specific values (4=FLOATCONV, 5=WIDELANE, 6=NARROWLANE, 19=PROPAGATED, 51=RTK_DIRECT_INS, 52=INS_SBAS, 67=EXT_CONSTRAINED, 70-72=OPERATIONAL/WARNING/OUT_OF_BOUNDS, 73-74, 77-80). SinoGNSS variant: on `false`, handles SinoGNSS-specific values (9=SINGLE_SMOOTH, 35=FIX_DERIVATION, 51=SUPER_WIDE_LANE).

**Unicore** (`gps/internal/unc/`) handles Unicore-specific values **first** (52=INS, 70=PPP_AR, 71=PPP_RTK), then delegates to `nov.PosTypeQuality(uint32(posVelType))` for everything else.

### Where this lives in code

The NovAtel processor (`gps/internal/nov/processor.go`) already has epoch detection and `NavEpochMsg` emission via `NavEpochManager`. The existing conversion functions in `gps/internal/nov/nav.go` receive `ne *gpsprot.NavEpochMsg` (the `curEpochMsg`) and populate accuracy fields on it inline. Quality metadata follows the same pattern:
1. Add `quality.go` with the shared `PosTypeQuality` function (see above).
2. Extend the BESTPOS handling path: the `PosGeo` function in `nav.go` already receives `ne`; add a companion function (or extend `PosGeo`) to populate quality fields (FixLevel, FixDim, Correction, AuxSrc, NumSVUsed, NumSVTracked, SignalsUsed, DiffAge, RTCMRefBaseID) on `ne`. This calls `PosTypeQuality` first, then handles OEM7/SinoGNSS-specific values on `false`. For `StnID`, parse as decimal and apply the <= 4095 filter; non-numeric values (PPP service codes) enrich `Correction` with `CorrPPP`.
3. Add a PSRDOP handling function in `nav.go` that receives `ne` and populates DOP fields. Wire it from a new `*novmsg.PsrDOP` case in `dispatch()`.

### Message enablement

NovAtel-format receivers use `LOG` commands for message enablement, configured via message files (e.g. `configs/gpsmsg/sinognss.toml`). The relevant tags are `nov-bestposb` (already defined for position) and `nov-psrdopb` (to be added for DOPs). No programmatic configuration like UBX or Unicore is needed.

## Allystar

The Allystar processor already emits `NavEpochMsg` with position/velocity accuracy via the `NavEpochManager` epoch mechanism. This plan adds solution quality metadata (FixLevel, FixDim, DOPs, satellite counts) to the existing epoch. Allystar's protocol is simpler than the others: its primary quality source (NAV-PVT) provides fix dimensionality and dead reckoning status but does not distinguish correction levels (DGNSS, RTK float, RTK fixed). The Allystar protocol does not use programmatic configuration; message enablement is handled via message files (`configs/gpsmsg/allystar.toml`).

### Inputs

Quality metadata comes from:
- **NAV-PVT** (`asbin.NavPvt`, to be added, ID 0x01 0xC1): fix type (`fixType`), `numSV`, `pDop`, position/velocity accuracy estimates, and position/velocity/time data. This is the primary quality source. Not currently parsed.
- **NAV-DOP** (`asbin.NavDop`, ID 0x01 0x04): GDOP/PDOP/TDOP/VDOP/HDOP/NDOP/EDOP (all scaled by 0.01). Already defined in `asbin` but not currently dispatched.

### Mapping: NAV-PVT `fixType` -> `NavEpochMsg`

NAV-PVT's `fixType` provides dimensionality and dead reckoning status but no correction or carrier-phase information. Unlike UBX NAV-PVT, Allystar NAV-PVT has no `flags` field with `gnssFixOK`, `diffSoln`, or `carrSoln` bits -- the two reserved bytes (offsets 21-22) are undocumented. There is also no per-signal or per-satellite correction source message in the Allystar binary protocol.

| fixType | Meaning | AuxSrc | FixLevel | FixDim |
|---------|---------|--------|----------|--------|
| 0 | no fix | 0 | FixLevelNone | 0 |
| 1 | dead reckoning only | AuxSrcDR | FixLevelNone | 0 |
| 2 | 2D-fix | 0 | FixLevelCode | FixDim2D |
| 3 | 3D-fix | 0 | FixLevelCode | FixDim3D |
| 4 | GNSS + dead reckoning | AuxSrcDR | FixLevelCode | FixDim3D |
| 5 | time only fix | 0 | FixLevelCode | FixDimTimeOnly |

Notes:
- **No Correction information**: Allystar NAV-PVT cannot distinguish standalone from corrected solutions. `Correction` is always left unset. NMEA GGA/RMC (when enabled alongside binary) can provide correction-level information via the NMEA synthesis path.
- **FixLevel is always FixLevelCode** for any GNSS-based fix. Without differential/carrier flags, we cannot claim a higher fix level from this message alone.
- **fixType 4 (GNSS + DR)**: Assumed 3D since the GNSS component contributes to the position solution.

### Accuracy

NAV-PVT provides accuracy fields with mm/mm/s resolution (same as UBX NAV-PVT):
- `hAcc` (offset 40, U4 mm) -> `Acc.Hor`
- `vAcc` (offset 44, U4 mm) -> `Acc.Vert`
- `sAcc` (offset 68, U4) -> `Acc.Speed` (units undocumented in 2.3.6 spec; assumed mm/s by analogy with NAV-VELNED which uses cm/s -- needs verification)
- `headAcc` (offset 72, U4 1e-5 deg) -> `Acc.Course`

The existing separate messages (NAV-POSLLH, NAV-POSECEF, NAV-VELNED, NAV-VELECEF) already populate accuracy fields on `curNavEpochMsg`. NAV-PVT provides the same data at the same or better resolution. Since NAV-PVT bundles position, velocity, and quality in a single message, it should use `PriVendorHigh` so its accuracy values take precedence when both NAV-PVT and the separate messages are enabled.

### Other fields

- **NumSVUsed**: from `NAV-PVT.numSV` (offset 23).
- **NumSVTracked**: not available from NAV-PVT. Could potentially be derived from NAV-SVINFO `numCh` if enabled, but that is already handled via `SatellitesMsg` and would require additional plumbing.
- **DOP.Pos**: from `NAV-PVT.pDop` (offset 76, U2, scale 0.01) or from `NAV-DOP.PDOP` (preferred when present for consistency with the other DOPs).
- **DOP.Geom/Hor/Vert/Time**: from NAV-DOP (scale 0.01) when available.
- **SignalsUsed**: not available from Allystar binary messages. No signal mask fields exist.
- **DiffAge**: not available from Allystar binary NAV-PVT. Only available via NMEA GGA (field 13) when NMEA is enabled alongside binary.
- **RTCMRefBaseID**: not available from Allystar binary NAV-PVT. Only available via NMEA GGA (field 14) when NMEA is enabled alongside binary.

### Where this lives in code

The epoch infrastructure is already in place:
- `PacketProcessor` in `gps/internal/as/asproc.go` stores `curNavEpochMsg *gpsprot.NavEpochMsg` for the current epoch.
- `handleNavEpoch` calls `mgr.EpochStarted(p, tRead)` on iTOW change and allocates a fresh `NavEpochMsg`.
- `FlushNavEpoch` (implementing `EpochFlusher`) returns `curNavEpochMsg` with `Tag=ASBIN` and `PriVendorLow`.
- Accuracy fields are already populated inline in the conversion functions (`aspv.go`).

The changes needed:
1. Add `NavPvt` struct to `gps/lib/asbin/nav.go` (88-byte payload with iTOW, time, fixType, numSV, position, velocity, accuracy, pDop fields). Register it in `init()` and move its ID from `other.go` to `nav.go`.
2. Add a `case *asbin.NavPvt:` in `dispatch()` that:
   - Emits position (`PosGeoMsg`), velocity (`VelGeoMsg`), and time (`TimeMsg`) with `PriVendorHigh`.
   - Populates quality fields (FixLevel, FixDim, AuxSrc, NumSVUsed, DOP.Pos) on `curNavEpochMsg` inline via a new function (e.g. `qualityNavPvt`) in `aspv.go`.
   - Populates accuracy fields (Acc.Hor, Acc.Vert, Acc.Speed, Acc.Course) on `curNavEpochMsg`.
3. Add a `case *asbin.NavDop:` in `dispatch()` to populate DOP fields on `curNavEpochMsg` inline. `NavDop` is already defined in `asbin/nav.go` but not dispatched.

### Message enablement

Allystar does not use programmatic configuration. Message enablement is via message files (`configs/gpsmsg/allystar.toml`). Tags `asbin-nav-dop` (already defined) enable NAV-DOP. A new tag `asbin-nav-pvt` would enable NAV-PVT at 1Hz (class 0x01, id 0xC1, rate 1).

### Future enhancement: NAV-AUTO

Allystar's NAV-AUTO message (0x01 0xC0, 32 bytes) provides a richer `fixstate` field that distinguishes DGNSS (5), RTK float (6), and RTK fixed (7) -- information that NAV-PVT's `fixType` cannot provide. It also includes satInView (tracked satellite count) and inline DOPs (PDOP/HDOP/VDOP). However, NAV-AUTO has no iTOW field, which makes it difficult to associate with the existing epoch mechanism. It is also described as being for "automotive application" and may not be supported on all Allystar receivers. A future enhancement could use NAV-AUTO when available to enrich FixLevel and Correction beyond what NAV-PVT provides, but this requires solving the epoch association problem (e.g. by associating with the most recent epoch based on arrival time).

## Phasing

### Phase 1: type definitions (done)

Add the new types to `gps/gpsprot/`:
- `FixLevel`, `FixDim`, `CorrKind`, `AuxSrc` types and constants (in a new file, e.g. `quality.go`)
- `DOP` struct
- `PVTMsgQuality` flag in `PVTMsgFlags`
- Extend `NavEpochMsg` with the new fields (`FixLevel`, `FixDim`, `Correction`, `AuxSrc`, `DOP`, `DiffAge`, `RTCMRefBaseID`, `NumSVUsed`, `NumSVTracked`, `SignalsUsed`)
- Extend `MergeNavEpoch` to merge the new fields

No protocol changes; all existing tests continue to pass unchanged.

### Phase 2: per-protocol implementation

Each subphase is independent and can proceed in parallel.

#### Phase 2a: NMEA

- Extend `parseGGA` to extract quality indicator (field 5), HDOP (field 7), differential age (field 13), and reference station ID (field 14).
- Extend `parseGSA` to extract fix type (field 1) and DOPs (fields 14-16).
- Extend `parseRMC` to extract mode indicator (field 11).
- Accumulate metadata on `NavEpoch.NavEpochMsg` using the merge/synthesis rules in the NMEA section. For RTCMRefBaseID, filter to <= 4095.

#### Phase 2b: UBX

- In existing `case *ubxbin.NavPVT:`, populate quality fields on `curNavEpochMsg` inline, including DiffAge from `Flags3` `lastCorrectionAge` buckets.
- Add `case *ubxbin.NavDOP:` to populate DOP fields on `curNavEpochMsg` inline.
- Accumulate `CorrKind` from NAV-SIG/NAV-SAT correction sources as they are processed.
- Add `PVTMsgQuality` handling to `ubxcfgmsg.go` (enable NAV-DOP, ensure NAV-PVT).

#### Phase 2c: Unicore

- Extend `bestNavPosVel` to populate quality fields on `ne` from BESTNAV `pos type`: handle Unicore-specific values first (52=INS, 70=PPP_AR, 71=PPP_RTK), then delegate to `nov.PosTypeQuality` for shared values. DiffAge and RTCMRefBaseID from the existing `DiffAge`/`StnID` fields in `novmsg.Pos`. For StnID, parse as decimal; <= 4095 sets RTCMRefBaseID, >= 4096 enriches Correction.
- Add STADOP parsing (`uncmsg.StaDOP`) and DOP population.
- Add `PVTMsgQuality` handling to `cfgopts.go`.

#### Phase 2d: NovAtel

- Add `nov/quality.go` with shared `PosTypeQuality` function (used by both NovAtel and Unicore).
- Extend BESTPOS handling path to populate quality fields on `ne` from `pos type` and `sol status`, calling `PosTypeQuality` for shared values and handling OEM7/SinoGNSS-specific values locally. DiffAge and RTCMRefBaseID from the existing `DiffAge`/`StnID` fields in `novmsg.Pos`. For StnID, parse as decimal; <= 4095 sets RTCMRefBaseID, non-numeric PPP service codes enrich Correction with CorrPPP.
- Add PSRDOP parsing (`novmsg.PsrDOP`) and DOP population.
- Add `nov-psrdopb` tag to message files.

#### Phase 2e: Allystar

- Add `NavPvt` struct to `asbin/nav.go` and register it.
- Add `case *asbin.NavPvt:` to `dispatch()` for position, velocity, time, and quality fields.
- Add `case *asbin.NavDop:` to `dispatch()` for DOP fields.
- Add `asbin-nav-pvt` tag to `configs/gpsmsg/allystar.toml`.
- DiffAge and RTCMRefBaseID not available from binary; rely on NMEA GGA when enabled.

#### Phase 2f: Quectel PQTM

- Extend `msgBundleNAV` in `handler.go` to populate quality fields on the epoch from PQTMNAV `SolType`, `SatUsed`, `SatView`, `DiffAge`, `DiffID`.
- Extend `msgBundlePVT` in `handler.go` to populate quality fields on the epoch from PQTMPVT `FixType`, `NumSV`, `HDOP`, `PDOP`. Add epoch parameter to `msgBundlePVT`.
- Add `case *qtmmsg.DOP:` in `HandleSentence` to populate DOP fields on the epoch.
