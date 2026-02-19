# Navigation epoch accumulator

Prerequisite: [nav-epoch.md](nav-epoch.md) (adds `NavEpochMsg` and `MsgHandler.NavEpoch`).

Related: [solution-quality.md](solution-quality.md) (populates `NavEpochMsg` fields), [multi-prot-nav-epoch.md](multi-prot-nav-epoch.md) (cross-protocol epoch coordination).

## Problem

Within a single navigation epoch, multiple messages may provide the same data. Within a protocol, different native messages carry overlapping fields (e.g. UBX NAV-PVT and NAV-POSLLH both provide horizontal position and accuracy; NAV-PVT and NAV-VELNED both provide velocity and speed accuracy). Across protocols, binary and NMEA messages carry the same position/velocity from the same navigation solution.

Applications receiving `PosGeoMsg`, `VelGeoMsg`, etc. via `MsgHandler` face a problem: multiple messages per epoch, no way to know which to prefer.

## 1. Message priority and NavEpochAccum

### Priority levels

Four priority levels are defined in `gpsprot`, ordered from lowest to highest:

```go
type MsgPriority uint8

const (
    PriGenericLow  MsgPriority = iota + 1
    PriGenericHigh
    PriVendorLow
    PriVendorHigh
)
```

"Generic" is NMEA; "Vendor" is the vendor-specific protocol (which may be binary e.g. UBX or ASCII, PQTM).
Within each band there are two levels for distinguishing message quality within the same protocol.

### Priority field on message types

Each message type in `MsgBundle` (`PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg`, `TimeMsg`) gets a `Priority` field:

```go
type PosGeoMsg struct {
    // ... existing fields ...
    Priority MsgPriority `json:"-"`
}
```

The field is excluded from JSON serialization since it is an internal accumulation concern.

### Merge method on message types

Each message type gets a `Merge` method that incorporates fields from another message of the same type, guided by priority:

```go
func (m *PosGeoMsg) Merge(other *PosGeoMsg)
```

The merge rule is field-by-field:
- If `other` has higher priority than `m`: `other`'s fields overwrite `m`'s fields (both non-optional and `opt.Val` fields).
- If `other` has lower priority than `m`: `other`'s `opt.Val` fields fill in only where `m`'s corresponding field is unset. Non-optional fields are not overwritten.
- If equal priority: same as higher (overwrite).

Implement this by adding a helper generic in T that takes pointers to the Opt fields and the two priorities.

### NavEpochAccum

`NavEpochAccum` lives in `gpsprot`. It implements `MsgHandler` for the Pos/Vel/Time methods. It holds a `MsgBundle` that accumulates the best message of each kind within an epoch.

```go
type NavEpochAccum struct {
    Bundle MsgBundle
}
```

When `NavEpochAccum` receives a message (e.g. via its `PosGeo` method):
- If the corresponding `Bundle` slot is nil, store the message.
- If the slot is non-nil, call `Merge` on the stored message with the new one.

`NavEpochAccum` also implements `NavEpoch` to clear the accumulated Bundle. Applications would typically embed NavEpochAccum, and provide an override than calls
NavEpochAccum at the right time.

### Protocol-specific priority assignments

**UBX:**
- `PosGeoMsg` from NAV-PVT, NAV-POSLLH: `PriVendorLow`
- `PosGeoMsg` from NAV-HPPOSLLH: `PriVendorHigh`
- `PosECEFMsg` from NAV-POSECEF: `PriVendorLow`
- `PosECEFMsg` from NAV-HPPOSECEF: `PriVendorHigh`
- `VelGeoMsg` from NAV-PVT: `PriVendorHigh`
- `VelGeoMsg` from NAV-VELNED: `PriVendorLow`
- `VelECEFMsg` from NAV-VELECEF: `PriVendorLow`

**NMEA:**
- `PosGeoMsg` from RMC: `PriGenericLow`
- `PosGeoMsg` from GGA: `PriGenericHigh`
- `VelGeoMsg` from RMC: `PriGenericLow`
- `VelGeoMsg` from VTG: `PriGenericHigh`

**Quectel, Allystar, Unicore:** all messages `PriVendorLow` (only one source per message kind).

## 2. NavEpochMsg field accumulation

Within a single navigation epoch, multiple native messages contribute fields to the same `NavEpochMsg`. For example, UBX NAV-PVT and NAV-POSLLH both write horizontal accuracy; NMEA GGA and RMC both write correction status. The protocol-specific message handlers need rules for merging these overlapping contributions.

The default rule across all protocols is **first wins**: only write a `NavEpochMsg` field if it is currently empty. This is correct for the common case where multiple messages carry the same value from the same navigation solution. The subsections below document per-protocol exceptions where a message unconditionally overwrites because it has genuinely better data.

### UBX

| Message | NavEpochMsg field |
|---------|-------------------|
| NAV-PVT | Acc.Hor, Acc.Vert, Acc.Speed, Acc.Course |
| NAV-SIG | Correction |
| HPPOSLLH | Acc.Hor, Acc.Vert |
| HPPOSECEF | Acc.Pos |

Everything not in this table uses the default first-wins rule.

### NMEA

**FixDim, DOP, NumSVUsed**: each comes from a single sentence (FixDim from GSA, DOPs from GSA, NumSVUsed from GGA, HDOP from GGA as fallback for GSA). No conflicts; first-wins is sufficient.

**FixLevel, Correction, AuxSrc**: both GGA and RMC contribute these fields. GGA unconditionally overwrites (it distinguishes RTK fixed vs float via quality 4/5, and identifies dead reckoning via quality 6). RMC basic modes (A/D/N/E/M/S) use first-wins only. This works regardless of sentence arrival order.

Additionally, the NMEA handler buffers the RMC mode indicator. At flush time, if the mode is R, F, or P (NMEA 4.x extended modes), it overwrites FixLevel and Correction with the extended mode values, overriding even GGA. Mode P identifies wide-area corrections (CorrWideArea), which GGA cannot express. Modes R/F explicitly identify base-station carrier solutions. AuxSrc is not affected.

### PQTM

PQTM sentences and standard NMEA sentences are interleaved within the same NMEA `PacketProcessor` and accumulate into the same `NavEpoch`.

| Message | NavEpochMsg field |
|---------|-------------------|
| PQTMEPE | Acc.Hor, Acc.Vert, Acc.Pos |
| PQTMVEL | Acc.GroundSpeed, Acc.Speed, Acc.Course |
| PQTMDOP | DOP.* |

PQTMEPE is a dedicated position error message. PQTMVEL is a dedicated velocity message with explicit accuracy fields. PQTMDOP is a dedicated DOP message providing all seven DOP values (GDOP/PDOP/TDOP/VDOP/HDOP/NDOP/EDOP), superseding the partial subsets from GSA, GGA, and PQTMPVT.

**Correction**: PQTMNAV's SolType is more specific than GGA for Correction — it distinguishes SBAS (SolType 2) from pseudorange differential (SolType 5), while GGA quality 2/3 maps both to `CorrUsed`. To handle this without conflicting with GGA's inline overwrite, `NavEpoch` gets a `Corr` field (`CorrKind`). PQTMNAV writes `CorrSBAS` or `CorrBaseStation` into this field. At flush time, if `Corr` is set, it overwrites `Correction` in the `NavEpochMsg` — the same pattern used for RMC extended modes.

Everything not in this table uses the default first-wins rule.

