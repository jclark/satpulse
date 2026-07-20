# Config panel restructure, readback, and validation (done)

**Status: complete**

## Goal
Restructure the existing config panel into collapsible sections, add config readback, and improve the editing experience.

## Prerequisite
layout-shell. receiver-info.

## Reference documents
- [ui-panel-configuration.md](ui-panel-configuration.md) - full configuration panel design
- [backend.md](backend.md) - configuration API calls

## Concept

### What --show-config does
`satpulsetool gps --show-config` runs a `ConfigTarget` with `Get` set to the properties bitmask (signals, mode, time pulse, time GNSS, cable delay) but no `ForceProbe` and no configuration changes. This reads back the current receiver configuration so it can be displayed and edited.

This is separate from the receiver probe (receiver-info). The probe identifies the hardware; config readback retrieves current property values.

### Single Configure method
All configuration operations use a single `Configure` Wails binding that takes a map and maps it to a `gpsprot.ConfigTarget`. The frontend builds a plain object with whatever fields are needed:
- **Readback**: `get` = properties list
- **Apply**: `props` with changed property values
- **Save**: `save: true` or `saveAll: true`
- **Reset/reload/factory reset**: `reset` field

The backend is a thin wrapper: map -> `ConfigTarget` -> `gpscfg.Configure` -> result DTO. No separate methods for different operations.

### When to read config
Config readback should be triggered by opening the configuration panel (or clicking Refresh within it). It should not run automatically on connect -- that would add unnecessary delay and the user may not even open the config panel.

## Steps

### 1. Collapsible sections
Create a reusable `CollapsibleSection` component (heading + chevron toggle + animated content). Use Tailwind for styling, no external library.

Restructure `config-panel.tsx` into two top-level collapsible sections:
1. Properties (existing fields: time pulse, mode, GNSS/signals, cable delay, etc.)
2. Persistent Operations (existing: save, reload, reset, factory reset)

Section collapse/expand state is remembered (component state).

### 2. Config readback on panel open
When the configuration panel is first opened (or becomes visible):
- Call `Configure` with `get` to fetch current property values
- Populate property controls with the returned values
- Show a timestamp for the last successful readback
- Show partial results if readback is incomplete

A Refresh button in the panel triggers the same readback.

This requires a new backend method or extending the existing API to support readback-only calls. The current `DetectReceiver` already reads config as a side effect of probing, but a dedicated readback path via `Configure` with just `get` is cleaner.

### 3. Sticky action bar
Add a sticky action bar at the top (or bottom) of the config panel with:
- `Refresh` button (triggers config readback)
- `Apply` button (calls `ApplyConfig` with changed properties)
- Operation status indicator (idle / applying / success / error)
- Pending changes summary (count of edited fields)

### 4. Simple/advanced signal editing
Add a simple mode to the GNSS/signals subgroup:
- Simple: per-constellation enable/disable toggles
- Advanced: full signal-level picker (existing `signal-picker.tsx`)
- Toggle between modes with a switch
- Switching modes preserves intent where possible

### 5. Validation and interlocks
- Mode-specific fields enabled only when their mode is selected
- Inline validation for numeric fields (range checks)
- Conflicting inputs blocked before Apply with inline reasons

### 6. Confirmation dialog for destructive operations
- Factory reset requires a confirmation dialog before executing
- Cancel returns to normal state without action

## Result
Configuration panel is restructured with collapsible sections, config readback on open, sticky action bar, simple/advanced signal modes, inline validation, and confirmation for destructive operations. All existing property configuration continues to work.

## Files changed
- `desktop/app.go` (readback-only Configure method)
- `desktop/frontend/src/config-panel.tsx` (major restructure)
- `desktop/frontend/src/collapsible-section.tsx` (new reusable component)
- `desktop/frontend/src/signal-picker.tsx` (simple mode added)
- `desktop/frontend/src/app.tsx` (config readback integration)

## Testing -- Playwright

### Collapsible sections
- Verify Properties and Persistent Operations sections are visible.
- Click a section header to collapse it; verify content is hidden.
- Click again to expand; verify content is shown.

### Sticky action bar
- Scroll down in a long config panel; verify Refresh and Apply buttons remain visible.
- Verify pending changes count updates when a field is edited.

### Config readback
- Connect to a receiver.
- Open the config panel; verify config readback runs and property fields are populated.
- Click Refresh; verify properties are re-read.
- Verify readback timestamp is shown.

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
