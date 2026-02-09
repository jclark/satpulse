# UI panel: configuration

## Purpose
Define the user-facing design and behavior of the Configuration panel.

This document is the main specification for how configuration works in the UI.

## User mental model
The panel combines two different kinds of controls:
- **Properties**: receiver state values that can be read back and shown as current values.
- **Options/operations**: one-shot instructions for the configuration run (not persistent current state).

The UI must make this distinction obvious while still allowing a single Apply action.

## Top-level layout
Use a single Configuration panel with three collapsible top-level sections:
1. Properties
2. Messages
3. Persistent Operations

Read-only receiver identity (for example name/model/version) is not part of this panel and should live in connection/device details UI.

Panel-level behavior:
- sticky action bar with `Refresh`, `Apply`, and operation status.
- section collapse/expand state is remembered.
- panel stays usable on smaller screens (single-column collapse behavior).

## Load and refresh behavior
When the panel opens (or `Refresh` is clicked):
- fetch current property values and populate property controls.
- do not infer "current" value for one-shot options.
- show timestamp for last successful readback.

If readback is partial:
- populate available properties,
- mark unavailable fields as `Not reported by receiver`.

## Properties
Properties are stateful and readback-backed.

### Property groups
Use collapsible subgroups:

#### Time pulse
- Time reference GNSS (`time-gnss`)
- Pulse period
- Pulse width
- Align pulse to GNSS time
- Only when locked
- Polarity
- Antenna cable delay

#### Time mode
Single mode selector with mode-specific fields:
- Mobile
- Survey-in
- Fixed position

Mode-specific controls:
- Survey-in: survey time, survey accuracy, report survey progress (maps to `PVTMsgSurvey`).
- Fixed position: fixed position (ECEF), fixed position accuracy.

#### GNSS / bands / signals
Two editing modes in the same subgroup:
- **Simple**: enable/disable per GNSS constellation.
- **Advanced**: explicit signal-level selection (bands/signals model aligned with `gpsprot/signal.go` and satpulsetool GPS model).

Behavior:
- simple mode should cover common use quickly.
- advanced mode is optional and discoverable.
- switching modes preserves user intent where possible and shows conflicts when exact mapping is not possible.

### Save behavior
See Persistent Operations section below.

## Messages
This section controls output-message configuration. For detailed flag semantics, see [message-semantics.md](message-semantics.md).

All five message groups are rendered directly inside the Messages section. NMEA and RTCM use a three-way selector; PVT, Satellites, and Raw use a configure checkbox. Each is rendered as a labelled bordered group box.

### Standard protocol messages
NMEA and RTCM are rendered as a labelled bordered group box with a three-way selector at the top:
- **Don't configure** (default) -- child controls greyed out but selections preserved
- **Configure** -- child controls enabled and interactive
- **Disable** -- child controls greyed out but selections preserved

The three-way selector is a set of radio buttons or a segmented control. Switching between states preserves child selections so the user can toggle back to Configure without losing their choices.

#### NMEA
- Three-way selector: Don't configure / Configure / Disable
- Child checkboxes (enabled only in Configure state): RMC, GGA, GSA, GSV, ZDA, VTG

#### RTCM
- Three-way selector: Don't configure / Configure / Disable
- Children (enabled only in Configure state):
  - MSM type: three-way radio -- None / MSM4 / MSM7
  - Fallback checkbox (enabled only when MSM4 or MSM7 is selected): "Use other MSM type if preferred isn't available" -- maps to `RTCMMsgLax`
  - ARP checkbox (independent)

### Proprietary messages
PVT, Satellites, and Raw are information-intent centric (not protocol packet names). Each is rendered as a labelled bordered group box with a configure checkbox in the border. When unchecked, child controls are greyed out but selections preserved ("don't configure" -- leave as-is). When checked, child controls are enabled.

#### PVT
- Configure checkbox in border
- Children organised into two domain subgroups, laid out as bordered group boxes within the PVT box.

**Time** subgroup (left column: information flags; right-aligned: modifier):
- Navigation time -- maps to `PVTMsgTime`
- Time-pulse time -- maps to `PVTMsgTimePulse`
  - Ensure message after pulse -- maps to `PVTMsgTimePulseAfter` (indented under Time-pulse time; greyed unless Time-pulse time is checked)
- Leap second -- maps to `PVTMsgLeapSecond`
- Right-aligned modifier: Prefer TAI -- maps to `PVTMsgTAI` (greyed unless Navigation time or Time-pulse time is checked)

**Position & velocity** subgroup (left column: information flags; right-aligned: modifier):
- Position -- maps to `PVTMsgPos`
- Velocity -- maps to `PVTMsgVel`
- Right-aligned modifier: Prefer ECEF -- maps to `PVTMsgECEF` (greyed unless Position or Velocity is checked)

After the subgroups, a standalone checkbox:
- Turn off unselected PVT messages -- maps to `PVTMsgOff`

Note: survey progress (`PVTMsgSurvey`) is not in this section. It belongs in Properties > Time mode > Survey-in, where it is contextually relevant. See the Time mode property group.

#### Satellites
- Configure checkbox in border
- Children (enabled only when checked):
  - Satellite positions -- maps to `SatsMsgSat`
  - Signals -- maps to `SatsMsgSignal`

#### Raw
- Configure checkbox in border
- Children (enabled only when checked):
  - Observations (RINEX .obs) -- maps to `RawMsgObs`
  - Navigation data (RINEX .nav) -- maps to `RawMsgNavData`

### Presets
Preset controls live inside Messages:
- `NMEA preset`
- `Binary preset`
- `Daemon preset`

Preset behavior:
- clicking a preset updates relevant message controls.
- `Daemon preset` is a convenience button that ticks/unticks its mapped selection set.
- users can still manually adjust individual selections after applying a preset.

## Persistent operations
One-shot operations that affect persistent memory or restart behavior. These are selections queued for the next Apply, not immediate actions.

All controls in this section are radio-button groups. Each group defaults to its "do nothing" option. After Apply completes (success or failure), all groups reset to their defaults.

### Save
Label "Save" with radio buttons (maps to `SaveType`):
- **Nothing** (default) -- `SaveNone`
- **Changes** -- save the minimum needed to persist settings changed by this apply run (`SaveMinimal`)
- **All** -- save the entire current running configuration to non-volatile memory (`SaveAll`)

### Reset
Label "Reset" with radio buttons (maps to `ResetType`):
- **Nothing** (default) -- `ResetNone`
- **Reload** -- reload configuration from non-volatile memory; unsaved changes are lost (`ResetReload`)
- **Cold** -- reload configuration from non-volatile memory and discard position/time/satellite data (`ResetCold`)
- **Factory** -- restore non-volatile memory to factory defaults, then cold reset (`ResetFactory`)

### Behavior
- These are explicit run directives, not readback state.
- Destructive options (Cold reset, Factory reset) show an inline warning when selected.
- Apply confirms destructive operations before executing.
- All radio groups reset to their defaults after Apply completes.

## Apply flow
Single Apply executes all selected changes in one run.

### On Apply
- collect edited Properties,
- collect Messages selections,
- collect Persistent Operations,
- submit one configuration request.

### During Apply (may take ~15s)
- disable duplicate Apply.
- show progress state in action bar.
- surface step/progress information in Logging panel and inline status summary.

### On result
- refresh/populate property controls from returned actual configured values.
- show per-group outcomes for messages/operations (`applied`, `partial`, `failed`, with details).
- keep changed selections visible long enough for user review.

## Validation and interlocks
- three-way and two-way controls gate child controls (greyed when disabled).
- mode-specific property fields are enabled only when their mode is selected.
- conflicting inputs are blocked before Apply with clear inline reasons.
- numeric/range validation is shown inline at field level.

## UX for large configuration surface
- all major groups are collapsible.
- default expanded groups: most frequently used (Properties summary + Messages summary).
- advanced groups (signal-level tuning, destructive persistent operations) default collapsed.
- provide a compact summary row at top showing pending edits before Apply.

## Implementation

### Component
`ConfigPanel` component, rendered inside a `Panel` in the right column.

### Existing code
The current `config-panel.tsx` handles properties and signal selection. Receiver identification (detect/probe) is handled separately by the receiver panel (phase 4). The config panel focuses on property readback and editing.

### Collapsible sections
Use a reusable `CollapsibleSection` component (heading + toggle + animated content). Tailwind for styling. No external accordion library needed.

### Form state
Local component state tracks edits. On Apply, diff against last-known values to build a `ConfigTarget`-shaped JSON object. On result, update last-known values from the response.

### Wails binding
`ApplyConfig` accepts `gpsprot.ConfigTarget` directly. The frontend builds a JSON object matching `ConfigTarget`'s shape -- `Props` (ConfigProps JSON), `Get` (PropIDs as string array), `Opts` (ConfigOptions) -- and Wails deserializes it using the existing `UnmarshalJSON` methods. No DTO or translation layer.

### Backend
`ConfigTarget` is fully JSON round-trippable (`ConfigProps`, `PropIDs`, and `ConfigOptions` all have JSON support). The Wails adapter passes `ConfigTarget` straight through to `gpscfg.Configure`. No `gps/` package changes needed.
