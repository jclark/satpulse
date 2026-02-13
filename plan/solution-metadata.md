# Enhanced metadata about navigation solution

## Motivation

Modern GNSS receivers expose a wide variety of "fix type" and "quality" indicators across different proprietary protocols and NMEA sentences. Unfortunately, these indicators are inconsistent, overlapping, and often mix multiple conceptual dimensions into a single enum. For example, a single vendor code may conflate measurement type (code vs carrier), correction architecture (RTK vs SBAS vs PPP), ambiguity state (float vs fixed), convergence state (PPP converging vs converged), or even whether the solution is GNSS-based at all. This makes it difficult to build a clean, vendor-neutral abstraction layer that can represent solution quality consistently across devices.

To address this, we introduce a structured NavEpochMsg type that models the navigation solution along a small number of orthogonal axes. Since this system is fundamentally about GNSS receivers, GNSS is the implicit baseline -- it is always present when a GNSS-based fix is available. The primary ordered axis FixQuality describes the precision class of the GNSS solution (NoFix -> Code -> CodeCorrected -> CarrierFloat -> CarrierFixed), with a special value FixQualityNotMeasured for positions not derived from any navigation solution (e.g. manual input or simulated). An AuxSrc bitmask captures additional sources that contributed to the solution (dead reckoning, INS). Additional independent enums describe correction metadata (CorrKind, a bitmask of correction assertions) and solution dimensionality (FixDim). This approach separates estimator state from raw measurement state (e.g. per-satellite tracking data) and from the low-latency time/position/velocity messages. The result is a normalized, future-proof solution metadata model that can be mapped conservatively from diverse vendor protocols while remaining semantically clean and extensible.

In addition to inconsistency, there is a practical streaming/latency constraint. The core navigation outputs -- time, position, and velocity -- must be emitted with essentially zero added latency, because downstream consumers (timing, logging, UI, control loops) often depend on them immediately when they appear. However, many receivers do not provide all "fix quality" metadata in the same message as the solution itself. Instead, the metadata is scattered across multiple protocol messages (or arrives in a different sentence within the same epoch), which means a normalized view of solution quality is inherently an aggregation problem and may only be complete once the remaining messages for that epoch have arrived (and in the worst case, not until the first message of the next epoch establishes the boundary).

NavEpochMsg is therefore designed as a synthesized message emitted at the end of a navigation epoch, summarizing the solution's classification and context without holding back the low-latency navigation results. A receiver-specific adapter can combine information from GGA/GSA/GNS (or proprietary status blocks, PPP/RTK state messages, DOP reports, etc.) and emit one NavEpochMsg when the metadata for an epoch is sufficiently known. Because these properties typically change slowly (fix transitions, correction acquisition, ambiguity resolution, PPP convergence), the epoch-end emission is acceptable and avoids contaminating the timing-critical data path, while still giving applications a coherent, vendor-neutral description of the current navigation mode and quality.

## Type definitions

```
// NavEpochMsg is emitted once at the end of each navigation epoch, after
// all time/position/velocity messages for that epoch have been dispatched.
// It summarizes the classification and quality context of the epoch's
// solution, synthesized from one or more raw receiver messages.
//
// GNSS is the implicit baseline source when FixQuality indicates a GNSS-based
// fix. AuxSrc captures additional sources (e.g. DR/INS).
// The zero value of each field means "not provided" or "not applicable".
// CorrKind is a bitmask (not an enum) and its bits are related by a partial
// order (see CorrKind docs).
type NavEpochMsg struct {
	Quality     FixQuality  `json:"quality,omitzero"`
	Dim         FixDim      `json:"dim,omitzero"`
	Correction  CorrKind    `json:"correction,omitzero"` // meaningful when Quality >= FixQualityCodeCorrected
	AuxSrc      AuxSrc      `json:"auxSrc,omitzero"`
	NumSVUsed    opt.Val[uint16] `json:"numSVUsed,omitzero"`
	NumSVTracked opt.Val[uint16] `json:"numSVTracked,omitzero"`
	SignalsUsed  SignalSet       `json:"signalsUsed,omitzero"`

	GDOP opt.Val[float64] `json:"gdop,omitzero"`
	PDOP opt.Val[float64] `json:"pdop,omitzero"`
	HDOP opt.Val[float64] `json:"hdop,omitzero"`
	VDOP opt.Val[float64] `json:"vdop,omitzero"`
	TDOP opt.Val[float64] `json:"tdop,omitzero"`

	Tag          Tag             `json:"tag,omitzero"`
}

// AuxSrc is a bitmask of additional data sources that contributed to the
// navigation solution. GNSS contribution is implicit when FixQuality indicates
// a GNSS-based fix.
type AuxSrc uint8

const (
	AuxSrcDR        AuxSrc = 1 << iota // dead reckoning (e.g. wheel ticks, motion model)
	AuxSrcINS                          // inertial navigation system
)

// FixQuality is the primary, ordered axis describing the precision class of
// the GNSS navigation solution. Higher values represent higher intrinsic
// precision. The zero value means "not provided" or "not applicable".
type FixQuality uint8

const (
	// FixQualityNoFix indicates that no valid GNSS solution is available.
	FixQualityNoFix FixQuality = iota + 1

	// FixQualityNotMeasured indicates that the receiver reports a position that
	// is not based on any measurement (e.g. manual input or simulated). No
	// component of the PVT solution is being computed from observations.
	// CorrKind and Dim do not apply.
	FixQualityNotMeasured

	// FixQualityCode indicates an uncorrected code-based GNSS solution
	// (e.g. standalone SPS or single point positioning).
	FixQualityCode

	// FixQualityCodeCorrected indicates a code-based GNSS solution with
	// corrections applied (e.g. DGPS or SBAS). This improves accuracy but
	// remains limited by code measurement precision.
	FixQualityCodeCorrected

	// FixQualityCarrierFloat indicates a carrier-phase-based solution with
	// ambiguities estimated as float (non-integer) values. This includes
	// RTK float and classical PPP solutions prior to ambiguity fixing.
	FixQualityCarrierFloat

	// FixQualityCarrierFixed indicates a carrier-phase-based solution with
	// integer ambiguities resolved and constrained. This includes RTK fixed
	// and PPP-AR/PPP-RTK solutions.
	FixQualityCarrierFixed
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

For proprietary protocols (UBX, Unicore, etc.), the receiver must be told to output the underlying messages that feed the metadata synthesis. A new `PVTMsgMeta` flag in `PVTMsgFlags` (`gps/gpsprot/configtarget.go`) requests solution metadata messages. It is a message flag (not an option), included in `PVTMsgAny`, and parsed as `"meta"` on the CLI.

## Protocol mapping: NMEA

The NMEA sentences currently handled by the `nmea` package are RMC, ZDA, GSV and GSA. Of these, GGA and GSA carry solution metadata. GGA is not currently parsed for metadata (only `numSV` is extracted). GSA currently extracts only used SVIDs and GNSS; it ignores the fix mode and DOP fields.

The relevant sentences and their fields are:

### GGA (Global Positioning System Fix Data)

| Field | Content | Maps to |
|-------|---------|---------|
| 5 | GPS quality indicator | AuxSrc, Quality, Correction |
| 6 | Number of satellites in use | NumSVUsed |
| 7 | HDOP | HDOP |

GGA quality indicator mapping:

| GGA | AuxSrc | Quality | Correction |
|-----|--------|---------|------------|
| 0 | 0 | FixQualityNoFix | 0 |
| 1 | 0 | FixQualityCode | 0 |
| 2 | 0 | FixQualityCodeCorrected | CorrUsed |
| 3 | 0 | FixQualityCodeCorrected | CorrUsed |
| 4 | 0 | FixQualityCarrierFixed | CorrBaseStation |
| 5 | 0 | FixQualityCarrierFloat | CorrBaseStation |
| 6 | AuxSrcDR | FixQualityNoFix | 0 |
| 7 | 0 | FixQualityNotMeasured | 0 |
| 8 | 0 | FixQualityNotMeasured | 0 |

Notes:
- GGA quality 2 is "Differential GPS fix" but many receivers report SBAS under quality 2 as well, so we cannot determine base-station vs wide-area. Use `CorrUsed` only.
- GGA quality 3 has a muddled history ("PPS fix" originally, sometimes reinterpreted as wide-area). It is not reliably used across receivers, so we treat it the same as quality 2: `CorrUsed` only.
- GGA quality 4 is "Real Time Kinematic" (fixed integers) and quality 5 is "Float RTK". Both are unambiguously base-station/network-referenced solutions (`CorrBaseStation`).

### GSA (GNSS DOP and Active Satellites)

| Field | Content | Maps to |
|-------|---------|---------|
| 0 | Fix selection mode (M/A) | (not used) |
| 1 | Fix type (1/2/3) | Dim |
| 2-13 | Satellite IDs used | (already used for SatellitesMsg) |
| 14 | PDOP | PDOP |
| 15 | HDOP | HDOP |
| 16 | VDOP | VDOP |
| 17 | System ID (optional, NMEA 4.10+) | (already used) |

GSA fix type mapping:

| GSA fix | Dim |
|---------|-----|
| 1 | no position fix (consistent with FixQualityNoFix) |
| 2 | FixDim2D |
| 3 | FixDim3D |

### RMC (Recommended Minimum Specific GNSS Data)

| Field | Content | Maps to |
|-------|---------|---------|
| 11 | Mode indicator (NMEA 2.3+) | AuxSrc, Quality, Correction |

RMC mode indicator mapping:

| RMC mode | AuxSrc | Quality | Correction |
|----------|--------|---------|------------|
| N | 0 | FixQualityNoFix | 0 |
| A | 0 | FixQualityCode | 0 |
| D | 0 | FixQualityCodeCorrected | CorrUsed |
| P | 0 | FixQualityCodeCorrected | CorrWideArea |
| R | 0 | FixQualityCarrierFixed | CorrBaseStation |
| F | 0 | FixQualityCarrierFloat | CorrBaseStation |
| E | AuxSrcDR | FixQualityNoFix | 0 |
| M | 0 | FixQualityNotMeasured | 0 |
| S | 0 | FixQualityNotMeasured | 0 |

Notes:
- RMC mode D (differential) is ambiguous: it could be either base-station or wide-area. Use `CorrUsed` only.
- The extended mode indicators R, F, P are part of later NMEA 4.x revisions. When present, they are authoritative (see Synthesis).
- RMC mode P indicates wide-area corrections (e.g. SBAS/PPP). When both GGA and RMC are present, we merge the best information from each (see Synthesis).

### Synthesis

A single NavEpochMsg per epoch is assembled by merging the best available data from GGA, RMC and GSA within the existing `satellitesBuffer` flush cycle.

Neither GGA nor RMC is strictly superior. Each contributes distinct information:
- **GGA** provides NumSVUsed, HDOP, and distinguishes RTK fixed (4) vs float (5).
- **RMC** mode P indicates wide-area corrections, whereas GGA quality 2 vs 3 is unreliable for distinguishing base-station vs wide-area on some receivers.
- **GSA** provides Dim and DOPs (PDOP, HDOP, VDOP) independently of the quality source.

Merge rules:
- **AuxSrc**: only set when there is a positive claim of an additional contributing source (DR, INS). Pure GNSS fixes leave AuxSrc as zero since GNSS is implicit.
- **AuxSrc, Quality, Correction**: if RMC carries an extended mode indicator (R, F, or P), it is authoritative for all three fields and takes precedence over GGA. R/F set `CorrBaseStation`; P sets `CorrWideArea`. Otherwise, prefer GGA when present (e.g. GGA quality 4/5 overrides RMC mode A/D for RTK fixed/float); fall back to RMC only if GGA is absent. For corrected-code solutions (GGA quality 2/3, RMC mode D), set `CorrUsed` only since the correction style is ambiguous.
- **Dim**: from GSA fix type.
- **DOPs**: PDOP and VDOP from GSA; HDOP from GSA (preferred) or GGA.
- **NumSVUsed**: from GGA.

The changes needed are:

1. Extend `parseGGA` to extract quality indicator (field 5) and HDOP (field 7) in addition to numSV.
2. Extend `parseGSA` to extract fix type (field 1) and DOPs (fields 14-16).
3. Extend `parseRMC` to extract mode indicator (field 11).
4. Add a separate `navEpochBuffer` (distinct from `satellitesBuffer`) that collects GGA, RMC and GSA metadata fields within an epoch. Both GGA and RMC carry UTC time of day (field 0). The buffer tracks the current time of day and flushes the previous epoch's metadata when a different time of day arrives. GSA has no time of day but arrives in the same epoch as the preceding GGA/RMC.
5. On flush, `navEpochBuffer` synthesizes a `NavEpochMsg` from the buffered metadata using the merge rules above and emits it via `MsgHandler.NavEpoch`.

## UBX

UBX has a clean notion of a navigation epoch via the NAV message `iTOW` field (“GPS time of week of the navigation epoch”). Most UBX NAV messages embed `iTOW`; the decoder already treats these as belonging to an epoch via `ubxbin.NavMsg` and `PacketProcessor.handleNavEpoch` in `gps/internal/ubx/ubx.go`. This makes UBX a good fit for emitting one synthesized `NavEpochMsg` per epoch in `flushNavEpoch`.

### Inputs (epoch-keyed by `iTOW`)

Minimum useful input:
- **UBX-NAV-PVT** (`ubxbin.NavPVT`): fix classification (`FixType`), status flags (`Flags`), carrier-solution status (`Flags` bits 6..7), differential flag (`Flags` bit 1), `NumSV`, and `PDOP`.

Recommended additional input for higher fidelity:
- **UBX-NAV-DOP** (`ubxbin.NavDOP`): GDOP/PDOP/TDOP/HDOP/VDOP (all scaled by 0.01).
- **UBX-NAV-SIG** (`ubxbin.NavSig`): per-signal “used” (`NavSigPrUsed`) and per-signal correction source (`CorrSource`), which can disambiguate OSR (base-station) vs SSR (wide-area) and identify specific services/transports (e.g. SBAS, RTCM, SPARTN, CLAS).
- **UBX-NAV-SAT** (`ubxbin.NavSat`): per-SV “used” (`NavSatSVUsed`) and per-SV correction-used flags (e.g. SBAS/RTCM/SPARTN/CLAS), useful as a fallback when NAV-SIG isn’t enabled.

### Mapping: `UBX-NAV-PVT` → `NavEpochMsg`

Epoch association:
- Use `NavMsg.NavEpoch()` (currently `NavITOW.ITOW`) to group per-epoch metadata; emit the synthesized `NavEpochMsg` when `PacketProcessor` observes the epoch change (same place it currently flushes accumulated NAV-SAT/NAV-SIG state).

Dimensionality (`Dim`) from `FixType`:

| NAV-PVT fixType | Meaning | Dim | AuxSrc |
|---|---|---|---|
| 0 | no fix | 0 | 0 |
| 1 | dead reckoning only | 0 | AuxSrcDR |
| 2 | 2D-fix | FixDim2D | 0 |
| 3 | 3D-fix | FixDim3D | 0 |
| 4 | GNSS + dead reckoning | FixDim3D (typically) | AuxSrcDR |
| 5 | time-only fix | FixDimTimeOnly | 0 |

Additionally, if `Flags.headVehValid` is set, add `AuxSrcINS` to `AuxSrc`. This flag indicates the receiver is in sensor fusion mode with an IMU/gyro contributing to the navigation solution. It applies to ADR/UDR products (e.g. F9R) and is never set on pure GNSS or timing receivers.

Quality (`Quality`) and correction presence:
- If `Flags.gnssFixOK` is **not** set, treat the epoch as "no fix": set `Quality=FixQualityNoFix` and leave the rest unset.
- If `FixType` indicates dead reckoning only (`fixType=1`), set `Quality=FixQualityNoFix` (and `AuxSrcDR` from the table above) and do not set correction assertions.
- Otherwise, set `Quality` primarily from `Flags.carrSoln` (carrier phase range solution status):
  - `carrSoln = fixed` → `FixQualityCarrierFixed`
  - `carrSoln = float` → `FixQualityCarrierFloat`
  - `carrSoln = none`:
    - if `Flags.diffSoln` is set → `FixQualityCodeCorrected` and assert `CorrUsed`
    - else → `FixQualityCode` with no correction assertions

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

Other fields:
- `NumSVUsed`: from `NAV-PVT.NumSV`.
- `PDOP`: from `NAV-PVT.PDOP` (scale 0.01) or from `NAV-DOP.PDOP` (preferred when present for consistency with the other DOPs).
- `GDOP/HDOP/VDOP/TDOP`: from `NAV-DOP` (scale 0.01) when available.
- `NumSVTracked`, `SignalsUsed`: derive from the same per-epoch NAV-SAT/NAV-SIG accumulation used to emit `SatellitesMsg` (see `gps/internal/ubx/ubx.go` `satMsg`/`sigMsg` and `gps/internal/ubx/ubxsats.go` `satellitesCombine`), counting unique SVs and the union of signal IDs marked `Used`.

### Where this lives in code

To implement the UBX mapping in the existing UBX pipeline:
1. Extend `PacketProcessor` in `gps/internal/ubx/ubx.go` with per-epoch cached pointers for `*ubxbin.NavPVT` and (optionally) `*ubxbin.NavDOP`.
2. In `Dispatch`, cache those messages when seen (in addition to the existing `TimeMsg`/satellite handling).
3. In `flushNavEpoch` (which currently only calls `flushSats()`), also synthesize and emit a `NavEpochMsg` using the cached messages and any accumulated NAV-SAT/NAV-SIG state, then clear the cached pointers for the next epoch.

This keeps the zero-latency time/position/velocity messages (e.g. `timeNavPVT` in `gps/internal/ubx/ubxtime.go`) separate from the epoch-end metadata synthesis, matching the design goals above.

### Message enablement

When `PVTMsgMeta` is set, the UBX message configuration (`gps/internal/ubx/ubxcfgmsg.go`) enables `UBX-NAV-DOP` (GDOP/HDOP/VDOP/TDOP) and ensures `UBX-NAV-PVT` is enabled (the primary source for fix quality, carrSoln, diffSoln, numSV). NAV-SIG and NAV-SAT are already controlled by `SatsMsgFlags`.

## Unicore

Unicore receivers expose solution metadata primarily through the BESTNAV message, which includes both position and velocity solution types as explicit enums. Unlike UBX, where fix quality must be inferred from multiple flag bits, Unicore's `pos type` field directly encodes the solution technique (code, carrier float, carrier fixed, PPP variants, INS fusion). This makes the mapping to `NavEpochMsg` straightforward from a single message. DOP values require a separate STADOP message.

### Inputs

Minimum useful input:
- **BESTNAV** (`uncmsg.BestNav`): solution status (`p-sol status`), position type (`pos type`), number of SVs tracked (`#SVs`), number of SVs used in solution (`#solnSVs`), extended solution status flags, and signal masks.

Recommended additional input for higher fidelity:
- **STADOP** (`uncmsg.StaDOP`): GDOP, PDOP, TDOP, HDOP, VDOP, NDOP, EDOP for the BESTNAV solution.

### Mapping: BESTNAV `pos type` -> `NavEpochMsg`

The `pos type` field is a single enum that encodes the solution technique. When `p-sol status` is not `SOL_COMPUTED` (0), treat the epoch as "no fix" regardless of `pos type`: set `Quality=FixQualityNoFix` and leave other fields unset.

When `p-sol status` is `SOL_COMPUTED`, map `pos type` as follows:

| pos type | ASCII | Unicore description | AuxSrc | Quality | Dim | Correction |
|----------|-------|---------------------|--------|---------|-----|------------|
| 0 | NONE | No solution | 0 | FixQualityNoFix | 0 | 0 |
| 1 | FIXEDPOS | Position fixed by FIX command or averaging | 0 | FixQualityCode | FixDimTimeOnly | 0 |
| 2 | FIXEDHEIGHT | Not supported | 0 | FixQualityCode | FixDim2D | 0 |
| 8 | DOPPLER_VELOCITY | Velocity computed using instantaneous Doppler | 0 | 0 | FixDimVelocityOnly | 0 |
| 16 | SINGLE | Single point position | 0 | FixQualityCode | FixDim3D | 0 |
| 17 | PSRDIFF | Pseudorange differential solution | 0 | FixQualityCodeCorrected | FixDim3D | CorrBaseStation |
| 18 | SBAS | Solution using corrections from an SBAS | 0 | FixQualityCodeCorrected | FixDim3D | CorrSBAS |
| 32 | L1_FLOAT | Floating L1 ambiguity solution | 0 | FixQualityCarrierFloat | FixDim3D | CorrBaseStation |
| 33 | IONOFREE_FLOAT | Floating ionosphere-free ambiguity solution | 0 | FixQualityCarrierFloat | FixDim3D | CorrFullDualFreq |
| 34 | NARROW_FLOAT | Floating narrow-lane ambiguity solution | 0 | FixQualityCarrierFloat | FixDim3D | CorrFullDualFreq |
| 48 | L1_INT | Integer L1 ambiguity solution | 0 | FixQualityCarrierFixed | FixDim3D | CorrBaseStation |
| 49 | WIDE_INT | Integer wide-lane ambiguity solution | 0 | FixQualityCarrierFixed | FixDim3D | CorrPartialDualFreq |
| 50 | NARROW_INT | Integer narrow-lane ambiguity solution | 0 | FixQualityCarrierFixed | FixDim3D | CorrFullDualFreq |
| 52 | INS | INS position solution | AuxSrcINS | FixQualityNoFix | FixDim3D | 0 |
| 53 | INS_PSRSP | INS pseudorange single point solution | AuxSrcINS | FixQualityCode | FixDim3D | 0 |
| 54 | INS_PSRDIFF | INS pseudorange differential solution | AuxSrcINS | FixQualityCodeCorrected | FixDim3D | CorrBaseStation |
| 55 | INS_RTKFLOAT | INS RTK floating point ambiguities solution | AuxSrcINS | FixQualityCarrierFloat | FixDim3D | CorrBaseStation |
| 56 | INS_RTKFIXED | INS RTK fixed ambiguities solution | AuxSrcINS | FixQualityCarrierFixed | FixDim3D | CorrBaseStation |
| 68 | PPP_CONVERGING | PPP solution converging | 0 | FixQualityCarrierFloat | FixDim3D | CorrPPPConverging |
| 69 | PPP | PPP positioning | 0 | FixQualityCarrierFloat | FixDim3D | CorrPPPConverged |
| 70 | PPP_AR | PPP fixed solution (PPP-AR) | 0 | FixQualityCarrierFixed | FixDim3D | CorrPPPConverged |
| 71 | PPP_RTK | PPP fixed solution (PPP-RTK) | 0 | FixQualityCarrierFixed | FixDim3D | CorrPPPRTK |

Notes:
- **FIXEDPOS (1)**: The position was fixed by a user command or averaging. The receiver still tracks GNSS for timing, so `FixDimTimeOnly` since only time/clock is being solved. Quality/Correction are not meaningful.
- **FIXEDHEIGHT (2)**: Height is constrained, only horizontal position is solved. Marked as not supported by Unicore but included for completeness.
- **DOPPLER_VELOCITY (8)**: Only a velocity solution from Doppler, without a valid position or time. Use `FixDimVelocityOnly` and leave Quality/Correction unset (zero) since this is a velocity-only result.
- **Float variants (32-34)**: L1_FLOAT, IONOFREE_FLOAT, and NARROW_FLOAT differ in the ambiguity parameterization but all represent carrier-phase float solutions. The distinction is not modeled in `FixQuality` (all map to `FixQualityCarrierFloat`). All are base-station solutions (`CorrBaseStation`).
- **Integer variants (48-50)**: L1_INT is a base-station fixed solution (`CorrBaseStation`). WIDE_INT and NARROW_INT represent increasingly full use of dual-frequency ambiguity strategies; map them to `CorrPartialDualFreq` and `CorrFullDualFreq` respectively.
- **INS (52)**: Pure inertial solution with no GNSS contribution. Quality/Correction are not meaningful since there is no GNSS fix.
- **INS fusion variants (53-56)**: These are GNSS+INS sensor fusion solutions. `AuxSrcINS` is set to indicate the additional INS contribution. The GNSS quality component follows the same pattern as the non-INS counterpart (PSRSP->Code, PSRDIFF->CodeCorrected, RTKFLOAT->CarrierFloat, RTKFIXED->CarrierFixed).
- **PPP_CONVERGING (68)**: PPP solution that has not yet reached steady-state accuracy. Mapped to `FixQualityCarrierFloat` (ambiguities are float during convergence) with `CorrPPPConverging`.
- **PPP (69)**: Converged PPP solution without ambiguity resolution. This is a carrier-phase float solution (PPP estimates ambiguities as real-valued parameters) that has reached steady-state. Mapped to `FixQualityCarrierFloat` with `CorrPPPConverged`.
- **PPP_AR (70)** and **PPP_RTK (71)**: PPP with integer ambiguity resolution. Both achieve carrier-phase fixed precision via wide-area PPP corrections. PPP_AR uses traditional PPP-AR and maps to `CorrPPPConverged`. PPP_RTK uses additional information enabling rapid ambiguity resolution and asserts `CorrPPPRTK`.

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

When `PVTMsgMeta` is set, the Unicore message configuration (`gps/internal/unc/cfgopts.go`) enables `BESTNAVB 1` (if not already enabled by `PVTMsgPos`) and `STADOPB 1` for DOP values. If `PVTMsgPos` already enables BESTNAV, then `PVTMsgMeta` only needs to add STADOP.
