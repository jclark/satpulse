# Navigation summary panel

## Goal
Add a navigation summary block to the top of the Monitor tab (see [ui-monitor-tab.md](ui-monitor-tab.md)) that shows the current navigation state at a glance. This is the "dashboard" summary -- the first thing the user reads to understand where they are, how good the fix is, and what the receiver is doing.

Not collapsible. Always visible when data is available.

---

## Phase 1: position, velocity, time

### Design

A compact block at the top of the Monitor tab showing position, velocity, and time. Several lines of dense, horizontally arranged values. No definition list, no field labels where the meaning is self-evident from context. Monospace tabular numbers for numeric values.

#### Position and time line
- **Lat, lon** -- e.g. "48.12345678N 11.56789012E"
- **Height** -- e.g. "523.4m" (ellipsoidal height)
- **Height MSL** -- e.g. "489.1m MSL"
- **UTC date/time** -- e.g. "2024-01-15 14:23:07"
- **Leap seconds** -- e.g. "LS 37"

#### Velocity line
- **Ground speed** -- e.g. "1.23 m/s"
- **Course over ground** -- e.g. "247.3deg"

#### Empty state
When no data has been received, the summary block is not rendered. It does not occupy space.

### Implementation

#### Prerequisite
Position/velocity messages (from `plan/position-velocity-messages.md`). `NavEpochMsg` emission (from `plan/nav-epoch.md`).

#### Update strategy
Position, velocity, and time messages are buffered during each navigation epoch. When `NavEpochMsg` arrives (marking the epoch boundary), the most recent buffered values are displayed. This produces one update per epoch (typically 1 Hz).

If only `PosECEFMsg` is available (no `PosGeoMsg`), convert to lat/lon/height using the existing `ECEFtoLLH` backend binding.

#### State
- `summaryPos: {lat, lon, height?, heightMSL?} | null`
- `summaryVel: {groundSpeed?, course?} | null`
- `summaryTime: {utc, leapSeconds?} | null`

State is cleared on disconnect.

#### Files changed
- `desktop/frontend/src/nav-summary.tsx` (new component)
- `desktop/frontend/src/app.tsx` (add to Monitor tab, wire up epoch-synced buffering)

#### Testing -- Playwright
- Connect to a receiver; verify position, velocity, and time appear.
- Verify values update once per second (not faster).
- Disconnect; verify the summary clears.

---

## Phase 2: solution metadata

### Design

Extends the summary block with fix quality, corrections, accuracy, DOPs, and satellite counts from `NavEpochMsg`.

#### Fix state line
The qualitative state of the solution. Changes slowly (on fix transitions, correction acquisition/loss).

Contents, left to right:
- **Fix quality** -- text badge: "No Fix", "Code", "Corrected", "Float", "Fixed", "Not Measured". Colored by severity (e.g. red for no fix, yellow for code, green for fixed).
- **Dimensionality** -- "2D", "3D", "Time Only", "Vel Only". Omitted when not applicable.
- **Correction keywords** -- only the leaf bits of the `CorrKind` bitmask. Rendered as small badges or keywords. Examples:
  - RTK fixed with RTCM: `Fixed` `3D` `Base Stn` `RTCM`
  - SBAS corrected: `Corrected` `3D` `SBAS`
  - PPP-RTK converged: `Fixed` `3D` `PPP-RTK` `Converged`
  - Standalone: `Code` `3D` (no correction keywords)
- **Auxiliary sources** -- "DR" and/or "INS" badges when `AuxSrc` flags are set.

##### Correction leaf-bit rendering

The `CorrKind` partial order means that when a more specific bit is set, all its ancestors are also set. For display, only show the most specific (leaf) bits -- those that have no set descendants.

The partial order tree, with display labels:

```
CorrUsed ("Corrected")
+-- CorrBaseStation ("Base Stn")
|   +-- CorrPartialDualFreq ("Wide-Lane")
|   |   +-- CorrFullDualFreq ("Dual-Freq")
+-- CorrWideArea ("Wide-Area")
|   +-- CorrSBAS ("SBAS")
|   +-- CorrCLAS ("CLAS")
|   +-- CorrSPARTN ("SPARTN")
|   +-- CorrPPP ("PPP")
|       +-- CorrPPPRTK ("PPP-RTK")
|       +-- CorrPPPConverging ("Converging")
|       +-- CorrPPPConverged ("Converged")
+-- CorrRTCM ("RTCM")
```

Algorithm: for each set bit, check whether any of its children are also set. If so, suppress the parent. Display only unsuppressed set bits.

Examples:
- `CorrBaseStation | CorrRTCM | CorrUsed` -> display "Base Stn", "RTCM"
- `CorrSBAS | CorrWideArea | CorrUsed` -> display "SBAS"
- `CorrPPP | CorrPPPRTK | CorrPPPConverged | CorrWideArea | CorrUsed` -> display "PPP-RTK", "Converged"
- `CorrUsed` alone -> display "Corrected"

#### Accuracy and satellites line
- **Horizontal accuracy** -- e.g. "+/-0.014m hor". Omitted if not available.
- **Vertical accuracy** -- e.g. "+/-0.021m vert". Omitted if not available.
- **3D position accuracy** -- e.g. "+/-0.025m 3D". Omitted if not available.
- **Speed accuracy** -- e.g. "+/-0.05m/s spd". Omitted if not available.
- **SVs used/tracked** -- e.g. "12/24 SVs".

#### DOPs line (optional)
Only shown when DOP values are available. May be merged into the accuracy line if space permits.

- **HDOP**, **VDOP**, **PDOP**, **GDOP**, **TDOP** -- e.g. "HDOP 0.8  VDOP 1.2  PDOP 1.4". Only show DOPs that are available.

#### Signals used (optional, may be deferred)
Small colored dots (using the signal strength band color scheme) indicating which frequency bands are contributing to the solution.

#### Visual style
- Background: subtle colored strip or left border that reflects fix quality (red/yellow/green gradient).
- Correction keywords: small pill-shaped badges with muted background colors.

### Implementation

#### Prerequisite
`NavEpochMsg` metadata fields populated (from `plan/solution-metadata.md`).

#### Update strategy
Same as phase 1 -- display updates on `NavEpochMsg` arrival. The additional metadata fields (`Quality`, `Dim`, `Correction`, `AuxSrc`, `Acc`, `DOP`, `NumSVUsed`, `NumSVTracked`, `SignalsUsed`) come directly from `NavEpochMsg`.

Many fields are optional (`opt.Val`) and may not be available for all receivers or configurations. Lines/values that have no data are simply omitted.

#### Correction leaf-bit function
A small helper function that takes a `CorrKind` bitmask and returns the list of display labels for the leaf bits. The parent-child relationships are encoded as a static table.

#### Files changed
- `desktop/frontend/src/nav-summary.tsx` (extend existing component with metadata lines)

#### Testing -- Playwright
- Connect to a receiver; verify fix quality badge appears with correct color.
- Verify correction keywords show only leaf bits (e.g. "SBAS" not "SBAS Wide-Area Corrected").
- Verify accuracy and DOP values appear when available.
- Verify SV count appears.
- Verify lines with no data are omitted (not shown as blank or dashes).
