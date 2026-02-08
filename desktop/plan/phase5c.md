# Phase 5c: Message configuration

## Goal
Add a Messages section to the configuration overlay with output message controls, master toggles, and presets. This controls *what the receiver sends*.

## Prerequisite
Phase 5b (layout rework -- configuration is now a slide-down overlay). Phase 5a (config panel has collapsible sections and Apply flow).

## Context
The `gpsprot` package already defines all the message flag types (`NMEAMsgFlags`, `RTCMMsgFlags`, `PVTMsgFlags`, `SatsMsgFlags`, `RawMsgFlags`) and `ConfigOptions` already has fields for them -- this phase just needs to expose them through the Wails adapter and build the UI.

---

## Steps

### 1. Extend ConfigUpdate DTO for message fields
Add message configuration fields to the `ConfigUpdate` struct in `app.go` so the frontend can pass message flags through to `ConfigOptions`:
- NMEA message enables (RMC, GGA, GSA, GSV, ZDA, VTG, Other)
- RTCM message enables (MSM4, MSM7, ARP, Other, Lax)
- PVT message enables (position, velocity, time, time pulse, leap second, survey, TAI, ECEF, time pulse after, off)
- Satellite message enables (satellite positions, signal strengths)
- Raw message enables (observations, navigation data)

Map each field to the corresponding `gpsprot` flag in `ApplyConfig`.

### 2. Add Messages collapsible section
Add a third top-level collapsible section between Properties and Persistent Operations in the config panel. Build the Messages section UI as specified in [ui-panel-configuration.md](ui-panel-configuration.md):

**Standard protocol messages:**
- NMEA with master toggle gating child checkboxes (RMC, GGA, GSA, GSV, ZDA, VTG, Other)
- RTCM with master toggle gating child checkboxes (MSM4, MSM7, ARP, Other)

**Information-content outputs:**
- PVT information (position, velocity, navigation time, time-pulse time, leap-second, survey progress) with modifiers (TAI, ECEF, time message after pulse)
- Satellite information (satellite positions, signal strengths)
- Raw measurement information (raw observations, raw navigation data)

### 3. Master toggles
- Master toggle gates child controls (greyed/disabled when off)
- Toggling master on enables child checkboxes
- Toggling master off disables child checkboxes but preserves their state

### 4. Presets
Add preset buttons inside the Messages section:
- `NMEA preset` - ticks standard NMEA set
- `Binary preset` - ticks standard binary set
- `Daemon preset` - ticks the daemon's expected message set

Clicking a preset updates the relevant checkboxes. User can still adjust after applying a preset.

## Result
Configuration overlay has a full Messages section with NMEA/RTCM/binary output controls, master toggles, and presets.

## Files changed
- `desktop/app.go` (ConfigUpdate extended with message fields, mapping to ConfigOptions)
- `desktop/frontend/src/config-panel.tsx` (Messages section added)

## Testing -- Playwright

### Messages section
- Verify NMEA master toggle exists; toggle it off and verify child checkboxes are disabled/greyed.
- Toggle it on; verify child checkboxes become interactive.
- Same for RTCM master toggle.
- Click a preset button; verify relevant checkboxes change state.
- Modify a checkbox after preset; verify it stays modified.

### Apply with messages
- Select some message checkboxes and click Apply; verify the operation succeeds.
- Verify message selections are included in the configuration request.
