# Report constellations and bands used in the solution

Replace `SignalsUsed SignalSet` on `NavEpochMsg` with `GNSSUsed GNSSSet` and `BandsUsed Band`.

## Motivation

The current `SignalsUsed SignalSet` on `NavEpochMsg` has several problems:

1. It tries to report individual signals (e.g. "L2C" vs "L2P") but NovAtel only reports at band level ("GPS L2"), so we fabricate false precision. What users care about is which constellations and frequency bands are used in the solution.

2. NovAtel OEM7, Unicore UM980, and SinoGNSS all have different bitmask layouts for the signal bytes in BESTPOS/BESTXYZ, but the code shares `nov.SignalsUsed()` for all three. The current mapping is also wrong for NovAtel: it fabricates separate L2P/L2C bits that don't exist (NovAtel Table 93 has a single "GPS L2" bit), and misassigns bits 6-7 (NovAtel has GLO L3 + Reserved; Unicore has BDS B1 + BDS B2).

3. CASIC accumulates `SignalsUsed` by mapping CASIC SigIDs to band-level `Signal` constants, losing detail that NAV2-SIG already provides.

The replacement is two fields that we can compute accurately from every protocol:
- `GNSSUsed GNSSSet` -- which constellations contribute to the position solution
- `BandsUsed Band` -- which frequency bands are used, using the same band definitions as `--band`

## Phasing

- **Phase 1**: New types and methods in gpsprot, fix NovAtel/Bynav/Unicore mappings, update CASIC and u-blox, make NAV-SVINFO consistent, remove wrong SinoGNSS call, update consumers.
- **Phase 2**: Add Unicore BESTSAT dispatch and SATSINFO+BESTSAT merge so Unicore gets GNSSUsed/BandsUsed from satellite data (more accurate than the signal mask bytes).

## Phase 1

### Changes to gpsprot

#### NavEpochMsg

Replace:

```go
SignalsUsed SignalSet `json:"signalsUsed,omitzero"`
```

with:

```go
GNSSUsed  GNSSSet `json:"gnssUsed,omitzero"`
BandsUsed Band    `json:"bandsUsed,omitzero"`
```

In `MergeNavEpoch`, union both fields with `|=` (same as the current `SignalsUsed |=`).

#### Band JSON and string support

`Band` marshals as a JSON `[]string`, e.g. `["L1","L5"]`.

Add to gpsprot (in `signal.go`):

- `Band.Items() []string` -- returns the list of band names present in the set. Composites match before components: if both L5 and E5b bits are set, returns `["E5"]` not `["L5","E5b"]`.
- `Band.String() string` -- comma-separated band names.
- `Band.IsZero() bool`
- `Band.MarshalJSON()` / `Band.UnmarshalJSON()`
- `ParseBandName(string) (Band, bool)` -- parses a single band name. Accepts aliases: "L6" maps to E6, "E5" maps to L5|E5b.

#### GNSSSet

Add `GNSSSet.UnmarshalJSON` (currently only has `MarshalJSON`). Add `GNSSSet.IsZero`.

#### SignalIDBand mapping

Add a function:

```go
// SignalIDBand returns the frequency band for a signal identified by GNSS and SignalID.
// Returns 0 for unknown or unmapped signals (e.g. NavIC S-band).
func SignalIDBand(gnss GNSS, id SignalID) Band
```

The mapping is table-driven: an array of `{s string, prefix bool, band Band}`, searched in order (first match wins). Exact matches and longer prefixes come before shorter prefixes in the same family.

```go
var signalIDBandTable = [...]struct {
    s      string
    prefix bool
    band   Band
}{
    // Exact matches for composite bands
    {"E5", false, BandL5 | BandE5b},     // Galileo ALTBOC
    {"B2a+b", false, BandL5 | BandE5b},  // BeiDou B2a+b
    // L1 band
    {"L1", true, BandL1},
    {"E1", true, BandL1},
    {"B1", true, BandL1},
    // L2 band
    {"L2", true, BandL2},
    // L5 band (B2a before B2, E5b before E5)
    {"B2a", true, BandL5},
    {"L5", true, BandL5},
    {"E5a", true, BandL5},
    // E5b band
    {"E5b", true, BandE5b},
    {"B2", true, BandE5b},    // remaining B2: B2I, B2Q, B2b
    {"L3", true, BandE5b},    // GLONASS L3
    // E6 band
    {"E6", true, BandE6},
    {"L6", true, BandE6},
    {"B3", true, BandE6},
}
```

Unmapped signals (e.g. NavIC S-band) return 0. The GNSS parameter is not used for the lookup but makes the API explicit and available for future validation.

#### SatellitesMsg methods

```go
// GNSSUsed returns the set of GNSS constellations used in the solution.
// Returns zero when UsedValidity is SatelliteUsedInvalid.
func (msg *SatellitesMsg) GNSSUsed() GNSSSet

// BandsUsed returns the set of frequency bands used in the solution.
// Returns zero when UsedValidity is SatelliteUsedInvalid.
func (msg *SatellitesMsg) BandsUsed() Band
```

Both respect `UsedValidity`:
- `SatelliteUsedSignal`: collect from signals where `SignalInfo.Used` is true.
- `SatelliteUsedSV`: collect from all signals of SVs where `SVInfo.Used` is true.
- `SatelliteUsedInvalid`: return zero.

`BandsUsed()` uses `SignalIDBand` to convert each used signal's (GNSS, SignalID) to a Band.

### Changes to protocol handlers

#### nov (NovAtel/Bynav)

Rewrite `SignalsUsed()` in `nov/quality.go` to return `(gpsprot.GNSSSet, gpsprot.Band)`.

The correct NovAtel bit layout (from OEM7 docs Table 93 and Table 94):

**gpsGloBds2 byte (Table 93):**

| Bit | Signal | GNSS | Band |
|-----|--------|------|------|
| 0 | GPS L1 | GPS | L1 |
| 1 | GPS L2 | GPS | L2 |
| 2 | GPS L5 | GPS | L5 |
| 3 | Reserved | -- | -- |
| 4 | GLONASS L1 | GLO | L1 |
| 5 | GLONASS L2 | GLO | L2 |
| 6 | GLONASS L3 | GLO | E5b |
| 7 | Reserved | -- | -- |

**galBds3 byte (Table 94):**

| Bit | Signal | GNSS | Band |
|-----|--------|------|------|
| 0 | Galileo E1 | GAL | L1 |
| 1 | Galileo E5a | GAL | L5 |
| 2 | Galileo E5b | GAL | E5b |
| 3 | Galileo ALTBOC | GAL | E5 |
| 4 | BeiDou B1 | BDS | L1 |
| 5 | BeiDou B2 | BDS | E5 |
| 6 | BeiDou B3 | BDS | E6 |
| 7 | Galileo E6 | GAL | E6 |

BeiDou B2 maps to E5 (the superset band) because the single bit does not distinguish B2a (L5) from B2I/B2b (E5b).

Update callers in `nov/nav.go` (`quality()`) to set `ne.GNSSUsed` and `ne.BandsUsed`.

#### nov/nav.go sinoQuality

Remove the `SignalsUsed()` call. SinoGNSS has a different single-byte signal mask layout (Table 3-9) with BDS at bits 3/6/7 and no Galileo coverage. SinoGNSS should derive GNSSUsed/BandsUsed from satellite data when that support is added.

#### unc (Unicore)

Replace the call to `nov.SignalsUsed()` in `unc/nav.go:66` with a Unicore-specific `SignalsUsed()` in `unc/quality.go` returning `(gpsprot.GNSSSet, gpsprot.Band)`.

The Unicore bit layout (from UM980 docs Table 7-172 and Table 7-173):

**gpsGloBds2 byte (Table 7-172):**

| Bit | Signal | GNSS | Band |
|-----|--------|------|------|
| 0 | GPS L1 | GPS | L1 |
| 1 | GPS L2 | GPS | L2 |
| 2 | GPS L5 | GPS | L5 |
| 3 | BDS B3 | BDS | E6 |
| 4 | GLONASS L1 | GLO | L1 |
| 5 | GLONASS L2 | GLO | L2 |
| 6 | BDS B1 | BDS | L1 |
| 7 | BDS B2 | BDS | E5 |

**galBds3 byte (Table 7-173):**

| Bit | Signal | GNSS | Band |
|-----|--------|------|------|
| 0 | Galileo E1 | GAL | L1 |
| 1 | Galileo E5b | GAL | E5b |
| 2 | Galileo E5a | GAL | L5 |
| 3 | Reserved | -- | -- |
| 4 | BDS3 B1I | BDS | L1 |
| 5 | BDS3 B3I | BDS | E6 |
| 6 | BDS3 B2a | BDS | L5 |
| 7 | BDS3 B1C | BDS | L1 |

BDS B2 maps to E5 (same rationale as NovAtel). Phase 2 can provide more accurate data from BESTSAT.

#### casic

Replace the `casicSigToSignal` mapping and `SignalsUsed` accumulation in `corrFromNav2Sig` with calls to `GNSSUsed()` and `BandsUsed()` on the NAV2-SIG-derived `SatellitesMsg`.

The `SatellitesMsg` from NAV2-SIG already has `UsedValidity == SatelliteUsedSignal` with per-signal `SignalInfo.Used` and `SignalInfo.ID`, so the methods extract exactly the right data.

Correction accumulation (`CorrKind` from `CorFlags`) remains in `corrFromNav2Sig` -- only the `SignalsUsed` part moves out. The `casicSigToSignal` map and `SignalSetOf` calls are removed.

#### ubx (u-blox)

u-blox does not currently set `SignalsUsed`. After this change, call `GNSSUsed()` and `BandsUsed()` on the combined NAV-SAT+NAV-SIG `SatellitesMsg` (from `satellitesCombine`) and set the results on `NavEpochMsg`.

When only NAV-SAT is available (no NAV-SIG), the single signal per SV (always L1) gives a valid but limited result.

Make NAV-SVINFO consistent with NAV-SAT: populate `SignalInfo.ID` (derive from GNSS, same as `navSatSignalId`), set `SignalInfo.Used` from `SVInfo.Used`, and claim `SatelliteUsedSignal`. With one signal per SV, per-SV and per-signal used are equivalent.

#### as (Allystar)

Allystar does not currently set `SignalsUsed`. After this change, call `GNSSUsed()` and `BandsUsed()` on the NAV-SVINFO `SatellitesMsg` and set the results on `NavEpochMsg`.

Make NAV-SVINFO consistent with ubx: populate `SignalInfo.ID` (derive from GNSS via `asSVID`), set `SignalInfo.Used` from `SVInfo.Used`, and claim `SatelliteUsedSignal`.

### Changes to consumers

#### gpsflags.go

Remove the local `bandTable` variable and rewrite the `bands` flag type methods to use the new `Band` API:

- `bands.String()`: replace the `bandTable` loop with `Band.Items()` joined by commas. Keep the special cases for zero (`"(none)"`) and `BandAll` (`"all"`). `Items()` already handles composite-before-component ordering.
- `bands.Set()`: replace the `bandTable` lookup loop with `ParseBandName` for each comma-separated word. Return an error if `ParseBandName` returns false.

#### sseobs

`QualitySSE.SignalsUsed` splits into:

```go
GNSSUsed  gpsprot.GNSSSet `json:"gnssUsed,omitzero"`
BandsUsed gpsprot.Band    `json:"bandsUsed,omitzero"`
```

#### TypeScript types (gps/ts)

Replace `SignalSet` with separate fields:

```typescript
gnssUsed?: string[];    // e.g. ["GPS", "GAL"]
bandsUsed?: string[];   // e.g. ["L1", "L5"]
```

#### Frontend (web/dashboard.tsx)

Replace `formatSignalsUsed` with formatters for the two new fields. Display as e.g. "Constellations used: GPS, GAL" and "Bands used: L1, L5".

#### SignalSet

`SignalSet` remains in `gpsprot` unchanged. It is used for configuring which signals to enable/disable.

## Phase 2: Unicore BESTSAT for NavEpochMsg

BESTSAT provides per-satellite signal usage data. The per-satellite `SigMask` is constellation-specific (Tables 7-74 through 7-77). The `SatSys` type identifies the constellation (Table 7-116). Constants and decoding are already in `uncmsg/sats.go`.

### SatSys to GNSS mapping

| SatSys | Value | GNSS |
|--------|-------|------|
| SatSysGPS | 0 | GPS |
| SatSysGLONASS | 1 | GLO |
| SatSysSBAS | 2 | SBAS |
| SatSysGALILEO | 5 | GAL |
| SatSysBEIDOU | 6 | BDS |
| SatSysQZSS | 7 | QZSS |
| SatSysNAVIC | 9 | NAVIC |

### SigMask to Band mapping

**GPS (Table 7-74):**

| Bit | Constant | Band |
|-----|----------|------|
| 0 | SigUsedGPSL1 | L1 |
| 1 | SigUsedGPSL2 | L2 |
| 2 | SigUsedGPSL5 | L5 |

**GLONASS (Table 7-75):**

| Bit | Constant | Band |
|-----|----------|------|
| 0 | SigUsedGLOL1 | L1 |
| 1 | SigUsedGLOL2 | L2 |
| 2 | SigUsedGLOL3 | E5b |

**BeiDou (Table 7-76):**

| Bit | Constant | Band |
|-----|----------|------|
| 0 | SigUsedBDSB1 | L1 |
| 1 | SigUsedBDSB2 | E5 |
| 2 | SigUsedBDSB3 | E6 |

**Galileo (Table 7-77):**

| Bit | Constant | Band |
|-----|----------|------|
| 0 | SigUsedGALE1 | L1 |
| 1 | SigUsedGALE5A | L5 |
| 2 | SigUsedGALE5B | E5b |
| 3 | SigUsedGALALTBOC | E5 |

BDS B2 maps to E5 (same rationale as the signal mask bytes). QZSS and NAVIC signal mask tables are not documented; for those constellations, BESTSAT contributes to GNSSUsed but not BandsUsed.

### Fill/Set priority for GNSSUsed and BandsUsed

BESTPOS/BESTXYZ derive GNSSUsed/BandsUsed from a coarse signal mask (two bytes). BESTSAT derives them from per-satellite data, which is more accurate. Within a single Unicore `packetProcessor`, both messages contribute to the same `NavEpochMsg`, and either may arrive first. The solution is a Fill/Set priority scheme:

- **Fill** (lower priority): sets the field only if it is currently zero. Used by BESTPOS/BESTXYZ signal mask decoding.
- **Set** (higher priority): unconditionally assigns the field, overwriting any previous value. Used by BESTSAT.

This ensures BESTSAT always wins regardless of message arrival order: if BESTSAT arrives first, its values are set and BESTPOS fill is a no-op; if BESTPOS arrives first, its values are filled then overwritten when BESTSAT sets.

In Phase 1, the Unicore `quality()` function unconditionally assigns `ne.GNSSUsed` and `ne.BandsUsed`. Change this to fill semantics:

```go
if ne.GNSSUsed == 0 {
    ne.GNSSUsed = gnssUsed
}
if ne.BandsUsed == 0 {
    ne.BandsUsed = bandsUsed
}
```

### BESTSAT dispatch and NavEpochMsg

Add `*uncmsg.BestSat` to the dispatch switch in `unc/processor.go`. For each satellite entry, map `SatSys` to GNSS (for GNSSUsed) and `SigMask` bits to Band (for BandsUsed). Set the results on `NavEpochMsg` with unconditional assignment (not `|=`), so BESTSAT data replaces any previously filled BESTPOS values.

## Phase 3: Prometheus metrics for constellations and bands used

Add two bitmask-style Prometheus metrics to `time/internal/promobs/prometheus.go`, following the existing `setBitmaskGauge` pattern used by corrections and aux sources.

### Metrics

| Metric | Label | Values | Source |
|---|---|---|---|
| `satpulse_constellation_used` | `gnss` | `gps`, `gal`, `bds`, `glo`, ... | `msg.GNSSUsed` |
| `satpulse_gnss_band_used` | `band` | `l1`, `l2`, `l5`, `e5b`, `e6`, `e5` | `msg.BandsUsed` |

Value is 1 when active, 0 for previously seen but now inactive. Lazily registered on first non-zero value. Label `gnss` reuses the same label name as `satpulse_satellite_used{gnss=...}`.

Label values are lowercased from `GNSSSet.Items()` (via `GNSS.String()`) and `Band.Items()`.

### Changes

Add to `PrometheusObserver` struct:

```go
constellationUsed     *prometheus.GaugeVec
seenConstellationUsed map[string]struct{}
gnssBandUsed          *prometheus.GaugeVec
seenGNSSBandUsed      map[string]struct{}
```

Add to end of `updateSolutionQuality`, after aux sources:

```go
// Constellations used
if msg.GNSSUsed != 0 {
    names := make([]string, 0, 4)
    for _, g := range msg.GNSSUsed.Items() {
        names = append(names, strings.ToLower(g.String()))
    }
    p.seenConstellationUsed = setBitmaskGauge(p, &p.constellationUsed,
        "satpulse_constellation_used", "GNSS constellation used in solution", "gnss",
        names, p.seenConstellationUsed)
} else {
    clearBitmaskGauge(p.constellationUsed, p.seenConstellationUsed)
}
// Bands used
if msg.BandsUsed != 0 {
    names := make([]string, 0, 4)
    for _, b := range msg.BandsUsed.Items() {
        names = append(names, strings.ToLower(b))
    }
    p.seenGNSSBandUsed = setBitmaskGauge(p, &p.gnssBandUsed,
        "satpulse_gnss_band_used", "Frequency band used in solution", "band",
        names, p.seenGNSSBandUsed)
} else {
    clearBitmaskGauge(p.gnssBandUsed, p.seenGNSSBandUsed)
}
```

### Test

Add assertions to `prometheus_replay_test.go` after the correction assertions:

```go
if ne.GNSSUsed != 0 {
    for _, g := range ne.GNSSUsed.Items() {
        assertGaugeLabel(t, mf, "satpulse_constellation_used", "gnss", strings.ToLower(g.String()), 1)
    }
}
if ne.BandsUsed != 0 {
    for _, b := range ne.BandsUsed.Items() {
        assertGaugeLabel(t, mf, "satpulse_gnss_band_used", "band", strings.ToLower(b), 1)
    }
}
```

## Phase 4: Unicore SATSINFO + BESTSAT merge for SatellitesMsg

Merge SATSINFO (per-satellite signal details, `UsedValidity == SatelliteUsedInvalid`) with BESTSAT (which satellites are used in the solution) to produce a `SatellitesMsg` with `UsedValidity == SatelliteUsedSignal`.

### Background

SATSINFO produces a `SatellitesMsg` with rich per-signal data (signal IDs via `sysFreqToSignalID`, C/N0, look angles) but `UsedValidity == SatelliteUsedInvalid`. BESTSAT is a flat list of `BestSatEntry` with `System` (SatSys), `SatID` (PRN), and `SigMask` (bitmask of which bands are used per satellite). It tells us which satellites contribute to the solution but has no signal detail, C/N0, or look angles.

The merge takes SATSINFO's per-signal detail and annotates it with BESTSAT's usage information.

### Buffering and flushing

This follows the same accumulate-and-flush pattern as ubx's NAV-SAT + NAV-SIG merge. Either or both of SATSINFO and BESTSAT may be configured; when only one arrives in an epoch, the processor needs to decide whether to wait for the other or flush what it has. The decision requires observing a complete epoch to know what messages the receiver sends.

#### State

Add to `packetProcessor`:

```go
type satsMsgSet struct {
    satsInfo bool
    bestSat  bool
}
```

```go
// Satellite message accumulation: SATSINFO and BESTSAT are combined
// into a single SatellitesMsg when both are available.
satsInfoMsg  *gpsprot.SatellitesMsg  // from SATSINFO
bestSat      []uncmsg.BestSatEntry   // from BESTSAT
satsTRead    time.Time               // timestamp of first message in the pair
curSatsMsgs  satsMsgSet              // which sat messages seen in current epoch
prevSatsMsgs satsMsgSet              // which sat messages seen in previous complete epoch
nEpochsSeen  int
```

Store the raw `[]BestSatEntry` rather than a converted form, because the merge needs per-satellite `SigMask` to annotate individual signals.

#### Epoch boundary

In `handleEpoch`, when a new epoch is detected (before calling `mgr.EpochStarted`):

```go
p.prevSatsMsgs = p.curSatsMsgs
p.curSatsMsgs = satsMsgSet{}
p.nEpochsSeen++
```

#### Dispatch

When SATSINFO arrives: store in `p.satsInfoMsg`, set `p.curSatsMsgs.satsInfo = true`, record `satsTRead` if zero, call `maybeFlushSats()`.

When BESTSAT arrives: keep the existing Phase 2 `GNSSUsed`/`BandsUsed` set on `NavEpochMsg`. Additionally store `body.Sats` in `p.bestSat`, set `p.curSatsMsgs.bestSat = true`, record `satsTRead` if zero, call `maybeFlushSats()`.

#### maybeFlushSats

Same logic as ubx:

1. Both `satsInfoMsg` and `bestSat` buffered: flush immediately.
2. Only one buffered, `nEpochsSeen < 3`: wait (haven't seen a complete epoch yet).
3. Only one buffered, missing message was in `prevSatsMsgs`: wait (expect it later this epoch).
4. Only one buffered, missing message was *not* in `prevSatsMsgs`: flush what we have (it's not coming).

#### flushSats and FlushNavEpoch

`flushSats()` takes the buffered `satsInfoMsg` and `bestSat`, clears them, calls `satellitesMerge`, and dispatches the result via `h.Satellites()`. Resets `satsTRead`.

`FlushNavEpoch` calls `flushSats()` before returning the `NavEpochMsg`, same as ubx. This is the backstop: if only one message arrived and `maybeFlushSats` decided to wait, the epoch boundary flushes whatever is available.

### satellitesMerge

```go
func satellitesMerge(sats *gpsprot.SatellitesMsg, bestSat []uncmsg.BestSatEntry) *gpsprot.SatellitesMsg
```

When `bestSat` is nil: return `sats` unchanged (`UsedValidity == SatelliteUsedInvalid`).

When `sats` is nil: return nil. BESTSAT alone doesn't carry enough for a useful `SatellitesMsg`.

When both are present:

1. Build a map from `SVID` to `Band` from `bestSat`: for each entry, convert `(SatSys, SatID.PRN())` to `SVID` via `satSysToGNSS` + `prnToNum`, and convert `SigMask` to `Band` via `sigMaskToBand`. Skip entries with unknown systems or invalid PRNs.

2. Copy `sats` (shallow copy of SVs slice; copy signals since we mutate `Used`).

3. For each SV: look up its SVID in the map. If present, set `SVInfo.Used = true`, and for each signal set `SignalInfo.Used = (SignalIDBand(gnss, sig.ID) & usedBands) != 0`. If not present, leave Used flags as false (default).

4. Set `UsedValidity = SatelliteUsedSignal`.

BESTSAT's `SigMask` identifies used *bands*, not individual signals (e.g. `SigUsedGPSL1` means "GPS L1 band" without distinguishing L1C/A from L1C). If SATSINFO shows both L1C/A and L1C and BESTSAT says "L1 used", both signals get marked used. This matches the granularity BESTSAT provides.

### SATSINFO-only fallback

When BESTSAT is not configured, every epoch has SATSINFO but no BESTSAT. After the first 3 epochs, `maybeFlushSats` sees that BESTSAT was not in `prevSatsMsgs` and flushes SATSINFO immediately. The result is `UsedValidity == SatelliteUsedInvalid`, same as the current behavior. No regression.

### NavIC

NavIC satellites are already skipped in `satellitesMsgFromSatsInfo` (firmware PRN bug). They don't appear in the `SatellitesMsg`, so the merge doesn't need to handle them. If BESTSAT includes NavIC entries, those contribute to `GNSSUsed` via Phase 2 but not to `BandsUsed` (`sigMaskToBand` returns 0 for undocumented NavIC tables).
