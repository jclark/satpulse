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

## Steps

### 1. Config readback via Configure
The frontend calls the single `Configure` method with `get` set to the desired properties list. The backend maps this to a `ConfigTarget` with the corresponding `Get` bitmask, calls `gpscfg.Configure`, and returns the readback values.

### 2. Collapsible sections
Create a reusable `CollapsibleSection` component (heading + chevron toggle + animated content). Use Tailwind for styling, no external library.

Restructure `config-panel.tsx` into three top-level collapsible sections:
1. Properties (existing fields: time pulse, mode, GNSS/signals, cable delay, etc.)
2. Messages (new, initially empty placeholder)
3. Persistent Operations (existing: save, reload, reset, factory reset)

Section collapse/expand state is remembered (local storage or component state).

### 3. Config readback on panel open
When the configuration panel is first opened (or becomes visible):
- Call `Configure` with `get` to fetch current property values
- Populate property controls with the returned values
- Show a timestamp for the last successful readback
- Show partial results if readback is incomplete

A Refresh button in the panel triggers the same readback.

### 4. Sticky action bar
Add a sticky action bar at the top (or bottom) of the config panel with:
- `Refresh` button (calls `Configure` with `get`)
- `Apply` button (calls `Configure` with `props`)
- Operation status indicator (idle / applying / success / error)
- Pending changes summary (count of edited fields)

### 5. Message configuration support
Extend the `Configure` method's input map to support output message configuration fields:
- NMEA message enables (RMC, GGA, GSA, GSV, ZDA, VTG, etc.)
- RTCM message enables (MSM4, MSM7, ARP, etc.)
- Binary information-content outputs (position, velocity, time, satellites, raw obs, etc.)

These map to `gpsprot.ConfigTarget` fields. The backend just maps the input to the `ConfigTarget` — no new methods needed. The exact fields depend on what `gpsprot` exposes — may require additions to the `gps/` packages.

### 6. Frontend: messages section
Build the Messages section UI as specified in `ui-panel-configuration.md`:
- Standard protocol messages (NMEA with master toggle, RTCM with master toggle)
- Information-content outputs (PVT, satellite, raw measurement controls)
- Master toggles gate child checkboxes

### 7. Frontend: presets
Add preset buttons inside the Messages section:
- `NMEA preset` - ticks standard NMEA set
- `Binary preset` - ticks standard binary set
- `Daemon preset` - ticks the daemon's expected message set

Clicking a preset updates the relevant checkboxes. User can still adjust after applying a preset.

### 8. Simple/advanced signal editing
Add a simple mode to the GNSS/signals subgroup:
- Simple: per-constellation enable/disable toggles
- Advanced: full signal-level picker (existing `signal-picker.tsx`)
- Toggle between modes with a switch
- Switching modes preserves intent where possible

### 9. Validation and interlocks
- Mode-specific fields enabled only when their mode is selected
- Master message toggles gate child controls
- Inline validation for numeric fields (range checks)
- Conflicting inputs blocked before Apply with inline reasons

## Result
Configuration panel is fully featured: collapsible sections, config readback on open, message configuration with presets, simple/advanced signal modes, inline validation. This is the core workflow of the desktop app.

## Files changed
- `desktop/app.go` (Configure method extended for message config fields)
- `desktop/frontend/src/config-panel.tsx` (major rewrite)
- `desktop/frontend/src/collapsible-section.tsx` (new reusable component)
- `desktop/frontend/src/signal-picker.tsx` (simple mode added)
- `desktop/frontend/src/app.tsx` (config readback integration)

## Testing (Playwright)

### Collapsible sections
- Verify Properties, Messages, and Persistent Operations sections are visible.
- Click a section header to collapse it; verify content is hidden.
- Click again to expand; verify content is shown.
- Verify collapse state survives navigating away and back (if applicable).

### Sticky action bar
- Scroll down in a long config panel; verify Refresh and Apply buttons remain visible.
- Verify pending changes count updates when a field is edited.

### Config readback
- Connect to a receiver.
- Open the config panel; verify config readback runs and property fields are populated.
- Click Refresh; verify properties are re-read.
- Verify readback timestamp is shown.

### Messages section
- Verify NMEA master toggle exists; toggle it off and verify child checkboxes are disabled/greyed.
- Toggle it on; verify child checkboxes become interactive.
- Same for RTCM master toggle.
- Click a preset button; verify relevant checkboxes change state.
- Modify a checkbox after preset; verify it stays modified.

### Simple/advanced signal editing
- Verify simple mode shows per-constellation toggles.
- Switch to advanced mode; verify full signal picker appears.
- Switch back to simple mode; verify constellation toggles reflect the selection.

### Validation
- Enter an out-of-range value in a numeric field; verify inline error appears.
- Verify Apply button is disabled (or shows warning) when validation errors exist.

### Destructive operations
- Click Factory Reset; verify a confirmation dialog appears.
- Cancel the dialog; verify nothing happens.

## Backend gaps
Message configuration depends on `gpsprot.ConfigTarget` supporting message fields. If the `gps/` packages don't yet expose this, those packages need extension first. This may be the largest backend effort in the entire plan.
