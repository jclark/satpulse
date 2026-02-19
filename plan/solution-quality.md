# Solution quality

Prerequisite: [nav-epoch.md](nav-epoch.md) (adds the empty `NavEpochMsg` with `MsgHandler.NavEpoch`, emitted at UBX epoch boundaries).

Related: [multi-prot-nav-epoch.md](multi-prot-nav-epoch.md) (cross-protocol epoch coordination via `NavEpochManager`, which merges per-protocol `NavEpochMsg` contributions when binary + NMEA are both active).

## Motivation

Modern GNSS receivers expose a wide variety of "fix type" and "quality" indicators across different proprietary protocols and NMEA sentences. Unfortunately, these indicators are inconsistent, overlapping, and often mix multiple conceptual dimensions into a single enum. For example, a single vendor code may conflate measurement type (code vs carrier), correction architecture (RTK vs SBAS vs PPP), ambiguity state (float vs fixed), convergence state (PPP converging vs converged), or even whether the solution is GNSS-based at all. This makes it difficult to build a clean, vendor-neutral abstraction layer that can represent solution quality consistently across devices.

This plan adds metadata fields to the existing `NavEpochMsg` to model the navigation solution along a small number of orthogonal axes. Since this system is fundamentally about GNSS receivers, GNSS is the implicit baseline -- it is always present when a GNSS-based fix is available. The primary ordered axis FixLevel describes the technique used to compute the GNSS solution, ordered by increasing quality (None -> Code -> CodeCorrected -> CarrierFloat -> CarrierFixed), with a special value FixLevelNotMeasured for positions not derived from any navigation solution (e.g. manual input or simulated). An AuxSrc bitmask captures additional sources that contributed to the solution (dead reckoning, INS). Additional independent enums describe correction metadata (CorrKind, a bitmask of correction assertions) and solution dimensionality (FixDim). This approach separates estimator state from raw measurement state (e.g. per-satellite tracking data) and from the low-latency time/position/velocity messages. The result is a normalized, future-proof solution metadata model that can be mapped conservatively from diverse vendor protocols while remaining semantically clean and extensible.

The core navigation outputs -- time, position, and velocity -- must be emitted with essentially zero added latency, because downstream consumers (timing, logging, UI, control loops) often depend on them immediately when they appear. However, many receivers do not provide all "fix quality" metadata in the same message as the solution itself. Instead, the metadata is scattered across multiple protocol messages (or arrives in a different sentence within the same epoch), which means a normalized view of solution quality is inherently an aggregation problem and may only be complete once the remaining messages for that epoch have arrived (and in the worst case, not until the first message of the next epoch establishes the boundary).

The `NavEpochMsg` is already emitted at the end of each navigation epoch (see nav-epoch.md). This plan populates it with synthesized metadata from one or more raw receiver messages. A receiver-specific adapter can combine information from GGA/GSA/GNS (or proprietary status blocks, PPP/RTK state messages, DOP reports, etc.) and populate the `NavEpochMsg` fields when the metadata for an epoch is sufficiently known. Because these properties typically change slowly (fix transitions, correction acquisition, ambiguity resolution, PPP convergence), the epoch-end emission is acceptable and avoids contaminating the timing-critical data path, while still giving applications a coherent, vendor-neutral description of the current navigation mode and quality.

Position and velocity accuracy estimates also belong in `NavEpochMsg` rather than in the individual PVT messages. While many binary protocols (UBX, Allystar) bundle accuracy with position/velocity in the same message, this is not universal. The Quectel LG290P provides accuracy in a separate PQTMEPE message (estimated position error: north, east, down, 2D, 3D) with no position or velocity data. NMEA provides no metric accuracy at all (only DOPs). Since accuracy is not always available at the time position/velocity messages are emitted, it fits naturally into the epoch-end synthesis alongside DOPs and fix quality. For protocols that do bundle accuracy, the values are simply cached and included when the epoch flushes.

Time accuracy, however, stays in `TimeMsg` rather than moving into `NavEpochMsg.Accuracy`. The reasoning is that time accuracy is a property of the time measurement itself, not of the navigation epoch. Time messages come from two distinct sources: navigation-epoch messages (e.g. UBX NAV-TIMEGPS, NAV-PVT) and non-epoch messages (e.g. UBX TIM-TOS, which is a post-pulse timing message unrelated to any navigation epoch). Putting time accuracy in `NavEpochMsg` would either exclude non-epoch time messages or require them to awkwardly write into an epoch they don't belong to. Furthermore, no protocol provides time accuracy separately from the time message itself -- unlike position accuracy (PQTMEPE), there is no "time accuracy only" message that would need epoch-level aggregation. Each time message carries its own accuracy estimate, so `TimeMsg.Accuracy` is the natural home.

## Type definitions

The existing `NavEpochMsg` (which currently contains only `Tag`) is extended with metadata fields. New supporting types (`FixLevel`, `FixDim`, `CorrKind`, `AuxSrc`, `Accuracy`, `DOP`) are added to `gps/gpsprot/`.

```
// NavEpochMsg fields added to the existing struct (Tag already present from nav-epoch.md):
//
// GNSS is the implicit baseline source when FixLevel indicates a GNSS-based
// fix. AuxSrc captures additional sources (e.g. DR/INS).
// The zero value of each field means "not provided" or "not applicable".
// CorrKind is a bitmask (not an enum) and its bits are related by a partial
// order (see CorrKind docs).
type NavEpochMsg struct {
	FixLevel    FixLevel    `json:"fixLevel,omitzero"`
	FixDim      FixDim      `json:"fixDim,omitzero"`
	Correction  CorrKind    `json:"correction,omitzero"` // meaningful when FixLevel >= FixLevelCodeCorrected
	AuxSrc      AuxSrc      `json:"auxSrc,omitzero"`
	Acc         Accuracy    `json:"acc,omitzero"`
	DOP         DOP         `json:"dop,omitzero"`
	NumSVUsed    opt.Val[uint16] `json:"numSVUsed,omitzero"`
	NumSVTracked opt.Val[uint16] `json:"numSVTracked,omitzero"`
	SignalsUsed  SignalSet       `json:"signalsUsed,omitzero"`
	Tag          Tag             `json:"tag,omitzero"` // already present
}

// Accuracy holds estimated accuracy of the navigation solution. Fields are
// opt.Val because different protocols provide different subsets. Accuracy
// may be synthesized from multiple messages within an epoch (e.g. Quectel
// PQTMEPE provides position accuracy separately from PQTMPVT).
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

## Protocol mapping: NMEA

NMEA does not currently emit `NavEpochMsg` (nav-epoch.md only wired up UBX). This plan adds NMEA epoch detection and metadata population together.

The NMEA sentences currently handled by the `nmea` package are RMC, ZDA, GSV and GSA. Of these, GGA and GSA carry solution metadata. GGA is not currently parsed for metadata (only `numSV` is extracted). GSA currently extracts only used SVIDs and GNSS; it ignores the fix mode and DOP fields.

The relevant sentences and their fields are:

### GGA (Global Positioning System Fix Data)

| Field | Content | Maps to |
|-------|---------|---------|
| 5 | GPS quality indicator | AuxSrc, FixLevel, Correction |
| 6 | Number of satellites in use | NumSVUsed |
| 7 | HDOP | DOP.Hor |

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

The changes needed are:

1. Extend `parseGGA` to extract quality indicator (field 5) and HDOP (field 7) in addition to numSV.
2. Extend `parseGSA` to extract fix type (field 1) and DOPs (fields 14-16).
3. Extend `parseRMC` to extract mode indicator (field 11).
4. Add a separate `navEpochBuffer` (distinct from `satellitesBuffer`) that collects GGA, RMC and GSA metadata fields within an epoch. Both GGA and RMC carry UTC time of day (field 0). The buffer tracks the current time of day and flushes the previous epoch's metadata when a different time of day arrives. GSA has no time of day but arrives in the same epoch as the preceding GGA/RMC.
5. On flush, `navEpochBuffer` synthesizes a `NavEpochMsg` from the buffered metadata using the merge rules above and emits it via `MsgHandler.NavEpoch`.

## UBX

The UBX `PacketProcessor` already emits an empty `NavEpochMsg` at each epoch boundary in `flushNavEpoch` (from nav-epoch.md). This plan populates its metadata fields.

### Inputs (epoch-keyed by `iTOW`)

Minimum useful input:
- **UBX-NAV-PVT** (`ubxbin.NavPVT`): fix classification (`FixType`), status flags (`Flags`), carrier-solution status (`Flags` bits 6..7), differential flag (`Flags` bit 1), `NumSV`, and `PDOP`.

Recommended additional input for higher fidelity:
- **UBX-NAV-DOP** (`ubxbin.NavDOP`): GDOP/PDOP/TDOP/HDOP/VDOP (all scaled by 0.01).
- **UBX-NAV-SIG** (`ubxbin.NavSig`): per-signal “used” (`NavSigPrUsed`) and per-signal correction source (`CorrSource`), which can disambiguate OSR (base-station) vs SSR (wide-area) and identify specific services/transports (e.g. SBAS, RTCM, SPARTN, CLAS).
- **UBX-NAV-SAT** (`ubxbin.NavSat`): per-SV “used” (`NavSatSVUsed`) and per-SV correction-used flags (e.g. SBAS/RTCM/SPARTN/CLAS), useful as a fallback when NAV-SIG isn’t enabled.

### Mapping: `UBX-NAV-PVT` → `NavEpochMsg`

Epoch association:
- The epoch grouping and `NavEpochMsg` emission point already exist (nav-epoch.md). Metadata is populated in `flushNavEpoch` before the existing emission call.

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
- From **NAV-PVT**: `HAcc` (mm) → `Acc.Hor`, `VAcc` (mm) → `Acc.Vert`, `SAcc` (mm/s) → `Acc.Speed`, `HeadAcc` (1e-5 deg) → `Acc.Course`.
- From **NAV-POSECEF**: `PAcc` (cm) → `Acc.Pos`.
- From **NAV-POSLLH**: `HAcc` (mm) → `Acc.Hor`, `VAcc` (mm) → `Acc.Vert`.
- From **NAV-VELECEF**: `SAcc` (cm/s) → `Acc.Speed`.
- From **NAV-VELNED**: `SAcc` (cm/s) → `Acc.Speed`, `GSpeed` (cm/s) → `Acc.GroundSpeed`, `CAcc` (1e-5 deg) → `Acc.Course`.
- When multiple sources provide the same field within an epoch, prefer NAV-PVT (highest resolution: mm/mm/s) over NAV-POSLLH/NAV-VELNED (cm/cm/s).

Other fields:
- `NumSVUsed`: from `NAV-PVT.NumSV`.
- `DOP.Pos`: from `NAV-PVT.PDOP` (scale 0.01) or from `NAV-DOP.PDOP` (preferred when present for consistency with the other DOPs).
- `DOP.Geom/Hor/Vert/Time`: from `NAV-DOP` (scale 0.01) when available.
- `NumSVTracked`, `SignalsUsed`: derive from the same per-epoch NAV-SAT/NAV-SIG accumulation used to emit `SatellitesMsg` (see `gps/internal/ubx/ubx.go` `satMsg`/`sigMsg` and `gps/internal/ubx/ubxsats.go` `satellitesCombine`), counting unique SVs and the union of signal IDs marked `Used`.

### Where this lives in code

`flushNavEpoch` already emits an empty `NavEpochMsg` (nav-epoch.md). To populate it with metadata:
1. Extend `PacketProcessor` in `gps/internal/ubx/ubx.go` with per-epoch cached pointers for `*ubxbin.NavPVT` and (optionally) `*ubxbin.NavDOP`.
2. In `Dispatch`, cache those messages when seen (in addition to the existing `TimeMsg`/satellite handling).
3. In `flushNavEpoch`, synthesize the metadata fields from the cached messages and any accumulated NAV-SAT/NAV-SIG state, populate the `NavEpochMsg`, then clear the cached pointers for the next epoch.

This keeps the zero-latency time/position/velocity messages (e.g. `timeNavPVT` in `gps/internal/ubx/ubxtime.go`) separate from the epoch-end metadata synthesis, matching the design goals above.

### Message enablement

When `PVTMsgQuality` is set, the UBX message configuration (`gps/internal/ubx/ubxcfgmsg.go`) enables `UBX-NAV-DOP` (GDOP/HDOP/VDOP/TDOP) and ensures `UBX-NAV-PVT` is enabled (the primary source for fix quality, carrSoln, diffSoln, numSV). NAV-SIG and NAV-SAT are already controlled by `SatsMsgFlags`.

## Unicore

Unicore does not currently emit `NavEpochMsg` (nav-epoch.md only wired up UBX). This plan adds Unicore epoch detection and metadata population together.

Unicore receivers expose solution metadata primarily through the BESTNAV message, which includes both position and velocity solution types as explicit enums. Unlike UBX, where fix quality must be inferred from multiple flag bits, Unicore's `pos type` field directly encodes the solution technique (code, carrier float, carrier fixed, PPP variants, INS fusion). This makes the mapping to `NavEpochMsg` straightforward from a single message. DOP values require a separate STADOP message.

### Inputs

Minimum useful input:
- **BESTNAV** (`uncmsg.BestNav`): solution status (`p-sol status`), position type (`pos type`), number of SVs tracked (`#SVs`), number of SVs used in solution (`#solnSVs`), extended solution status flags, and signal masks.

Recommended additional input for higher fidelity:
- **STADOP** (`uncmsg.StaDOP`): GDOP, PDOP, TDOP, HDOP, VDOP, NDOP, EDOP for the BESTNAV solution.

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

From **BESTNAV**: `lat sigma` (m) and `lon sigma` (m) can be combined into `Acc.Hor` (horizontal accuracy as sqrt(lat_sigma^2 + lon_sigma^2)), `hgt sigma` (m) → `Acc.Vert`, `Horspd std` (m/s) → `Acc.GroundSpeed`, `Verspd std` (m/s) contributes to `Acc.Speed` (3D speed accuracy as sqrt(horspd_std^2 + verspd_std^2)).

From **BESTNAVXYZ**: `P-X/P-Y/P-Z sigma` (m) can be combined into `Acc.Pos` (3D position accuracy as sqrt(sx^2 + sy^2 + sz^2)), `V-X/V-Y/V-Z sigma` (m/s) can be combined into `Acc.Speed`.

### Other fields

- **NumSVUsed**: from BESTNAV `#solnSVs`.
- **NumSVTracked**: from BESTNAV `#SVs`.
- **SignalsUsed**: derive from the BESTNAV signal mask fields (`GPS, GLONASS and BDS2 sig mask` and `Galileo&BDS3 sig mask`).
- **DOPs**: GDOP, PDOP, TDOP, HDOP, VDOP from STADOP when available.

### Epoch association

BESTNAV includes a GPS reference time in its header (week number and milliseconds into the week). The `PacketProcessor` can use this to group messages into epochs, similar to UBX's `iTOW`-based epoch detection. STADOP also includes a GPS time header. The metadata is emitted when the epoch changes (i.e. when a new BESTNAV arrives with a different time).

### Where this lives in code

To implement the Unicore mapping in the existing Unicore pipeline:
1. Extend `PacketProcessor` in `gps/internal/unc/processor.go` to cache the most recent `BestNav` and (optionally) `StaDOP` messages per epoch.
2. In `dispatch()`, recognize BESTNAV and STADOP message IDs and cache their parsed content.
3. Add epoch-flush logic (triggered by a new BESTNAV with a different GPS time) that synthesizes a `NavEpochMsg` from the cached messages and emits it via `MsgHandler.NavEpoch`, then clears the cache.

### Message enablement

When `PVTMsgQuality` is set, the Unicore message configuration (`gps/internal/unc/cfgopts.go`) enables `BESTNAVB 1` (if not already enabled by `PVTMsgPos`) and `STADOPB 1` for DOP values. If `PVTMsgPos` already enables BESTNAV, then `PVTMsgQuality` only needs to add STADOP.
