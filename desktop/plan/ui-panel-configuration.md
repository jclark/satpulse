# UI panel: configuration

## Purpose
Define the user-facing design and behavior of the Configuration panel.

This document is the main specification for how configuration works in the UI.

## Design

### User mental model
The panel combines two different kinds of controls:
- **Properties**: receiver state values that can be read back and shown as current values.
- **Options/operations**: one-shot instructions for the configuration run (not persistent current state).

The UI must make this distinction obvious while still allowing a single Apply action.

### Top-level layout
Use a single Configuration panel with three collapsible top-level sections:
1. Properties
2. Messages
3. Persistent Operations

Read-only receiver identity (for example name/model/version) is not part of this panel and should live in connection/device details UI.

Panel-level behavior:
- sticky action bar with `Refresh`, `Apply`, and operation status.
- section collapse/expand state is remembered.
- panel stays usable on smaller screens (single-column collapse behavior).

### Load and refresh behavior
When the panel opens (or `Refresh` is clicked):
- fetch current property values and populate property controls.
- do not infer "current" value for one-shot options.
- show timestamp for last successful readback.

If readback is partial:
- populate available properties,
- mark unavailable fields as `Not reported by receiver`.

### Section 1: Properties
Properties are stateful and readback-backed.

#### Property groups
Use collapsible subgroups:

##### Time pulse
- Time reference GNSS (`time-gnss`)
- Pulse period
- Pulse width
- Align pulse to GNSS time
- Only when locked
- Polarity
- Antenna cable delay

##### Time mode
Single mode selector with mode-specific fields:
- Mobile
- Survey-in
- Fixed position

Mode-specific controls:
- Survey-in: survey time, survey accuracy.
- Fixed position: fixed position (ECEF), fixed position accuracy.

##### GNSS / bands / signals
Two editing modes in the same subgroup:
- **Simple**: enable/disable per GNSS constellation.
- **Advanced**: explicit signal-level selection (bands/signals model aligned with `gpsprot/signal.go` and satpulsetool GPS model).

Behavior:
- simple mode should cover common use quickly.
- advanced mode is optional and discoverable.
- switching modes preserves user intent where possible and shows conflicts when exact mapping is not possible.

#### Save behavior in Properties
Include checkbox:
- `Save changed settings on apply` (maps to minimal save behavior; optional per apply run).

### Section 2: Messages
This section controls output-message configuration and is unified into one area with sub-subsections.

#### Messages section structure

##### A) Standard protocol messages
Controls are protocol-message centric.

###### NMEA
- master checkbox: `Control NMEA messages`
- when off: all child NMEA message checkboxes are disabled/greyed out.
- when on: child checkboxes enabled (for example RMC/GGA/GSA/GSV/ZDA/VTG/Other).

###### RTCM
- master checkbox: `Control RTCM messages`
- when off: RTCM child controls disabled/greyed out.
- when on: child checkboxes enabled (for example MSM4/MSM7/ARP/Other/lax option as applicable).

##### B) Information-content outputs
Controls are information-intent centric (not protocol packet names):

###### PVT information
Use noun-phrase labels for primary information:
- Position
- Velocity
- Navigation solution time
- Time-pulse time
- Leap-second announcement
- Survey progress

PVT modifiers are shown as modifiers (not primary information items):
- Output time as TAI
- Output position in ECEF
- Ensure a time message after time pulse

###### Satellite information
- Satellite positions
- Signal strengths per satellite signal

###### Raw measurement information
- Raw observations
- Raw navigation data

##### C) Presets
Preset controls live inside Messages:
- `NMEA preset`
- `Binary preset`
- `Daemon preset`

Preset behavior:
- clicking a preset updates relevant message checkboxes.
- `Daemon preset` is a convenience button that ticks/unticks its mapped selection set.
- users can still manually adjust individual selections after applying a preset.

### Section 3: Persistent operations
One-shot operations that affect persistent memory or restart behavior.

Controls:
- Save all current settings (`save-all`)
- Reload configuration
- Reset (cold)
- Factory reset

Behavior:
- these are explicit run directives, not readback state.
- destructive operations require confirmation.
- controls auto-clear after apply completes (whether success or failure), with result summary retained.

### Apply flow
Single Apply executes all selected changes in one run.

#### On Apply
- collect edited Properties,
- collect Messages selections,
- collect Persistent Operations,
- submit one configuration request.

#### During Apply (may take ~15s)
- disable duplicate Apply.
- show progress state in action bar.
- surface step/progress information in Logging panel and inline status summary.

#### On result
- refresh/populate property controls from returned actual configured values.
- show per-group outcomes for messages/operations (`applied`, `partial`, `failed`, with details).
- keep changed selections visible long enough for user review.

### Validation and interlocks
- master message control toggles gate child controls (greyed when disabled).
- mode-specific property fields are enabled only when their mode is selected.
- conflicting inputs are blocked before Apply with clear inline reasons.
- numeric/range validation is shown inline at field level.

### UX for large configuration surface
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
Local component state tracks edits. On Apply, diff against last-known values to build the `ConfigUpdate` DTO. On result, update last-known values from the response.

### Wails binding
A single `Configure(map)` method handles all configuration operations. The frontend builds a plain object describing the desired `ConfigTarget` — probe, readback, property changes, save, reset — and the backend maps it to `gpsprot.ConfigTarget` and calls `gpscfg.Configure`. One method, one DTO shape.

### Backend gaps
The `gpsprot` package already defines all message flag types (`NMEAMsgFlags`, `RTCMMsgFlags`, `PVTMsgFlags`, `SatsMsgFlags`, `RawMsgFlags`) and `ConfigOptions` has fields for them. The only gap is the Wails adapter: the `ConfigUpdate` DTO in `app.go` needs message fields added and mapped to `ConfigOptions`. No `gps/` package changes needed.
