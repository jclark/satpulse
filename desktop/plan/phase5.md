# Phase 5: Configuration panel

## Goal
Build the full configuration panel with collapsible sections, config readback on open, message configuration (NMEA/RTCM/binary output control), and presets.

## Prerequisite
Phase 1 (panel layout). Phase 4 (receiver panel) is helpful — receiver identity is already known, so the config panel can focus on properties and messages.

## Reference documents
- [ui-panel-configuration.md](ui-panel-configuration.md) - full configuration panel design (properties, messages, operations, apply flow, validation)
- [backend.md](backend.md) - configuration API calls

## Concept

### What --show-config does
`satpulsetool gps --show-config` runs a `ConfigTarget` with `Get` set to the properties bitmask (signals, mode, time pulse, time GNSS, cable delay) but no `ForceProbe` and no configuration changes. This reads back the current receiver configuration so it can be displayed and edited.

This is separate from the receiver probe (phase 4). The probe identifies the hardware; config readback retrieves current property values.

### Single Configure method
All configuration operations use a single `Configure` Wails binding that takes a map and maps it to a `gpsprot.ConfigTarget`. The frontend builds a plain object with whatever fields are needed:
- **Readback**: `get` = properties list
- **Apply**: `props` with changed property values
- **Save**: `save: true` or `saveAll: true`
- **Reset/reload/factory reset**: `reset` field

The backend is a thin wrapper: map -> `ConfigTarget` -> `gpscfg.Configure` -> result DTO. No separate methods for different operations.

### When to read config
Config readback should be triggered by opening the configuration panel (or clicking Refresh within it). It should not run automatically on connect — that would add unnecessary delay and the user may not even open the config panel.

---

## Phase 5a: Panel restructure, readback, and validation

Restructure the existing config panel into collapsible sections, add config readback, and improve the editing experience. This phase uses the existing backend API (`ApplyConfig`, `SaveConfig`, `ResetConfig`) — no backend changes needed.

### Steps

#### 1. Collapsible sections
Create a reusable `CollapsibleSection` component (heading + chevron toggle + animated content). Use Tailwind for styling, no external library.

Restructure `config-panel.tsx` into two top-level collapsible sections:
1. Properties (existing fields: time pulse, mode, GNSS/signals, cable delay, etc.)
2. Persistent Operations (existing: save, reload, reset, factory reset)

Section collapse/expand state is remembered (component state).

#### 2. Config readback on panel open
When the configuration panel is first opened (or becomes visible):
- Call `Configure` with `get` to fetch current property values
- Populate property controls with the returned values
- Show a timestamp for the last successful readback
- Show partial results if readback is incomplete

A Refresh button in the panel triggers the same readback.

This requires a new backend method or extending the existing API to support readback-only calls. The current `DetectReceiver` already reads config as a side effect of probing, but a dedicated readback path via `Configure` with just `get` is cleaner.

#### 3. Sticky action bar
Add a sticky action bar at the top (or bottom) of the config panel with:
- `Refresh` button (triggers config readback)
- `Apply` button (calls `ApplyConfig` with changed properties)
- Operation status indicator (idle / applying / success / error)
- Pending changes summary (count of edited fields)

#### 4. Simple/advanced signal editing
Add a simple mode to the GNSS/signals subgroup:
- Simple: per-constellation enable/disable toggles
- Advanced: full signal-level picker (existing `signal-picker.tsx`)
- Toggle between modes with a switch
- Switching modes preserves intent where possible

#### 5. Validation and interlocks
- Mode-specific fields enabled only when their mode is selected
- Inline validation for numeric fields (range checks)
- Conflicting inputs blocked before Apply with inline reasons

#### 6. Confirmation dialog for destructive operations
- Factory reset requires a confirmation dialog before executing
- Cancel returns to normal state without action

### Result (5a)
Configuration panel is restructured with collapsible sections, config readback on open, sticky action bar, simple/advanced signal modes, inline validation, and confirmation for destructive operations. All existing property configuration continues to work.

### Files changed (5a)
- `desktop/app.go` (readback-only Configure method)
- `desktop/frontend/src/config-panel.tsx` (major restructure)
- `desktop/frontend/src/collapsible-section.tsx` (new reusable component)
- `desktop/frontend/src/signal-picker.tsx` (simple mode added)
- `desktop/frontend/src/app.tsx` (config readback integration)

### Testing — Playwright (5a)

#### Collapsible sections
- Verify Properties and Persistent Operations sections are visible.
- Click a section header to collapse it; verify content is hidden.
- Click again to expand; verify content is shown.

#### Sticky action bar
- Scroll down in a long config panel; verify Refresh and Apply buttons remain visible.
- Verify pending changes count updates when a field is edited.

#### Config readback
- Connect to a receiver.
- Open the config panel; verify config readback runs and property fields are populated.
- Click Refresh; verify properties are re-read.
- Verify readback timestamp is shown.

#### Simple/advanced signal editing
- Verify simple mode shows per-constellation toggles.
- Switch to advanced mode; verify full signal picker appears.
- Switch back to simple mode; verify constellation toggles reflect the selection.

#### Validation
- Enter an out-of-range value in a numeric field; verify inline error appears.
- Verify Apply button is disabled (or shows warning) when validation errors exist.

#### Destructive operations
- Click Factory Reset; verify a confirmation dialog appears.
- Cancel the dialog; verify nothing happens.

---

## Phase 5b: Message configuration

Add a Messages section to the configuration panel with output message controls, master toggles, and presets. The `gpsprot` package already defines all the message flag types (`NMEAMsgFlags`, `RTCMMsgFlags`, `PVTMsgFlags`, `SatsMsgFlags`, `RawMsgFlags`) and `ConfigOptions` already has fields for them — this phase just needs to expose them through the Wails adapter and build the UI.

### Steps

#### 1. Extend ConfigUpdate DTO for message fields
Add message configuration fields to the `ConfigUpdate` struct in `app.go` so the frontend can pass message flags through to `ConfigOptions`:
- NMEA message enables (RMC, GGA, GSA, GSV, ZDA, VTG, Other)
- RTCM message enables (MSM4, MSM7, ARP, Other, Lax)
- PVT message enables (position, velocity, time, time pulse, leap second, survey, TAI, ECEF, time pulse after, off)
- Satellite message enables (satellite positions, signal strengths)
- Raw message enables (observations, navigation data)

Map each field to the corresponding `gpsprot` flag in `ApplyConfig`.

#### 2. Add Messages collapsible section
Add a third top-level collapsible section between Properties and Persistent Operations. Build the Messages section UI as specified in `ui-panel-configuration.md`:

**Standard protocol messages:**
- NMEA with master toggle gating child checkboxes (RMC, GGA, GSA, GSV, ZDA, VTG, Other)
- RTCM with master toggle gating child checkboxes (MSM4, MSM7, ARP, Other)

**Information-content outputs:**
- PVT information (position, velocity, navigation time, time-pulse time, leap-second, survey progress) with modifiers (TAI, ECEF, time message after pulse)
- Satellite information (satellite positions, signal strengths)
- Raw measurement information (raw observations, raw navigation data)

#### 3. Master toggles
- Master toggle gates child controls (greyed/disabled when off)
- Toggling master on enables child checkboxes
- Toggling master off disables child checkboxes but preserves their state

#### 4. Presets
Add preset buttons inside the Messages section:
- `NMEA preset` - ticks standard NMEA set
- `Binary preset` - ticks standard binary set
- `Daemon preset` - ticks the daemon's expected message set

Clicking a preset updates the relevant checkboxes. User can still adjust after applying a preset.

### Result (5b)
Configuration panel has a full Messages section with NMEA/RTCM/binary output controls, master toggles, and presets. Combined with 5a, this completes the configuration panel.

### Files changed (5b)
- `desktop/app.go` (ConfigUpdate extended with message fields, mapping to ConfigOptions)
- `desktop/frontend/src/config-panel.tsx` (Messages section added)

### Testing — Playwright (5b)

#### Messages section
- Verify NMEA master toggle exists; toggle it off and verify child checkboxes are disabled/greyed.
- Toggle it on; verify child checkboxes become interactive.
- Same for RTCM master toggle.
- Click a preset button; verify relevant checkboxes change state.
- Modify a checkbox after preset; verify it stays modified.

#### Apply with messages
- Select some message checkboxes and click Apply; verify the operation succeeds.
- Verify message selections are included in the configuration request.
