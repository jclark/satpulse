# Message configuration

## Goal
Add a Messages section to the configuration tab with output message controls for standardized protocols (NMEA, RTCM) and proprietary messages (PVT, satellites, raw). This controls *what the receiver sends*.

UI design: [ui-panel-configuration.md](ui-panel-configuration.md) (Messages section).
Flag semantics: [message-semantics.md](message-semantics.md).

## Prerequisite
layout-rework (configuration is now a tab). config-restructure (collapsible sections and Apply flow). `ConfigOptions` uses `opt.Val[T]` with `omitzero`. `ConfigProps` and `PropIDs` have `UnmarshalJSON` (done in `configprops-json.md`). `ConfigTarget` is fully JSON round-trippable.

Steps 1-2 are prerequisites (refactor only, no new functionality). They replace hand-rolled DTOs with direct use of GPS subsystem types. Test after completing steps 1-2 to verify existing config panel still works before adding message controls in steps 3+.

## Context
`ConfigTarget` is directly JSON-deserializable. Its three fields -- `Props` (ConfigProps), `Get` (PropIDs), and `Opts` (ConfigOptions) -- all support JSON round-trip. The Wails adapter can accept a `ConfigTarget` and pass it straight through to `gpscfg.Configure` with no DTO or translation layer.

The current `app.go` has two problems:
1. `ConfigUpdate` is a hand-rolled DTO with manual setter-method translation for every property. Replaced by accepting `ConfigTarget` directly.
2. `ReceiverInfo` conflates receiver identity (from connect) with config readback (from config tab). `ReadConfig` should return `ConfigProps` directly, not wrap it in `ReceiverInfo`.

---

## Steps

### 1. Separate ReceiverInfo from config readback [done]

Use GPS subsystem types directly instead of hand-rolled DTOs. `ReceiverEvent` envelope struct for `gps:receiver` event. `ReadConfig` returns `*gpsprot.ConfigProps` directly. `GetReceiverState` returns cached `ReceiverEvent`.

### 2. Replace ConfigUpdate with ConfigTarget [done]

`ApplyConfig` accepts `gpsprot.ConfigTarget` directly. Frontend builds `ConfigTarget`-shaped JSON with `Props` and `Opts`. Signal selection uses `Set<string>` with `"GNSS:Signal"` keys.

### 3. Export flag constants for the frontend [done]

`desktop/frontend/src/msg-flags.ts` with integer constants matching Go iota definitions.

### 4. Build three-way selector component [done]

`ThreeWaySelector` component with radio buttons: Leave as-is / Disable / Select. Maps to backend values: skip -> field omitted, configure -> `selected flags | Other`, disable -> 0.

### 5. Build NMEA group UI + wire Apply + assemble Messages section [done]

NMEA group with three-way selector and checkboxes (RMC, GGA, GSA, GSV, ZDA, VTG). Messages collapsible section added to config panel. Apply handler extended with `Opts` for message fields. Each message group exports a wire-value function; `handleApply` collects all of them into `Opts`.

Tested end-to-end on ZED-F9T hardware.

### 6. Build RTCM group UI

RTCM group with three-way selector (same as NMEA).

Children (enabled only in Select state, greyed but preserved in other states):
- MSM type: three-way radio -- None / MSM4 / MSM7
- Fallback checkbox (enabled only when MSM4 or MSM7 selected): "Use other MSM type if preferred isn't available"
- ARP checkbox (independent of MSM selection)

On Apply in Select state, the frontend computes: `selected | RTCMMsgLax (if fallback checked) | RTCMMsgOther`.

Wire into Apply (`Opts.RTCMMsg`) and add to Messages section. Test.

### 7. Build PVT group UI

PVT group with configure checkbox. Children (enabled only when checked), organised into two domain subgroups:

**Time** subgroup:
- Navigation time checkbox
- Time-pulse time checkbox
  - Indented: Ensure message after pulse checkbox (greyed unless Time-pulse time is checked)
- Leap second checkbox
- Prefer TAI checkbox (greyed unless Navigation time or Time-pulse time is checked)

**Position & velocity** subgroup:
- Position checkbox
- Velocity checkbox
- Prefer ECEF checkbox (greyed unless Position or Velocity is checked)

Standalone checkbox:
- Turn off unselected PVT messages

On Apply when configured, OR the selected flags and send as `Opts.PVTMsg`. When not configured, omit (zero = don't configure). Wire into Apply and add to Messages section. Test.

### 8. Build Satellites group UI

Satellites group with configure checkbox. Children (enabled only when checked):
- Satellite positions checkbox
- Signals checkbox

Wire into Apply (`Opts.SatsMsg`) and add to Messages section. Test.

### 9. Build Raw group UI

Raw group with configure checkbox. Children (enabled only when checked):
- Observations (RINEX .obs) checkbox
- Navigation data (RINEX .nav) checkbox

Wire into Apply (`Opts.RawMsg`) and add to Messages section. Test.

### 10. Presets [done]

Add two preset buttons at the top of the Messages section (above the NMEA group). The `speed` prop is threaded from `app.tsx` to `ConfigPanel` so presets can check serial speed.

**Daemon** -- matches `--pvt-out daemon` from `satpulsetool gps`:
- NMEA: change + disable
- RTCM: unchanged
- PVT: change, flags = TimePulse | TimePulseAfter | TAI | LeapSecond | Off
- Sats: change with Sat + Signal if speed >= 19200; otherwise unchanged
- Raw: unchanged

**Minimum** -- only NMEA RMC, everything else off:
- NMEA: change, flags = RMC only
- RTCM: change + disable
- PVT: change, flags = Off (turn off all PVT messages)
- Sats: change, flags = 0 (disable)
- Raw: change, flags = 0 (disable)

Preset behavior:
- clicking a preset updates the relevant message controls in the form
- does not trigger Apply -- the user reviews and clicks Apply
- user can manually adjust individual selections after applying a preset

---

## Result
Configuration tab has a Messages section with controls for all message categories. Three-way selectors for NMEA/RTCM. Configure checkboxes for PVT/Satellites/Raw. PVT has domain subgroups with conditional modifier controls. Presets provide quick configuration. Backend is a direct pass-through -- `ApplyConfig` accepts `ConfigTarget`, `ReadConfig` returns `ConfigProps`. No DTOs or translation.

## Files changed
- `desktop/app.go` (`ReceiverInfo` trimmed to identity only, `ReadConfig` returns `ConfigProps`, `ConfigUpdate` replaced by `ConfigTarget`, `configPropsToMap` deleted)
- `desktop/frontend/src/msg-flags.ts` (new: flag constants mirroring Go definitions)
- `desktop/frontend/src/config-panel.tsx` (Messages section added, Apply builds `ConfigTarget`-shaped JSON)
- `desktop/frontend/src/three-way-selector.tsx` (new reusable component)

## Testing -- Playwright

### Standardized protocol controls
- Verify NMEA group defaults to "Don't configure" with greyed child checkboxes.
- Switch to "Configure"; verify child checkboxes become interactive.
- Check RMC and GGA; switch to "Disable"; verify children are greyed but preserved.
- Switch back to "Configure"; verify RMC and GGA are still checked.
- Switch to "Don't configure"; verify children are greyed.

### RTCM controls
- In RTCM Configure state, select MSM4; verify MSM7 is not selected.
- Select MSM7; verify MSM4 is not selected.
- Select None; verify fallback checkbox is disabled.
- Select MSM4; verify fallback checkbox is enabled.

### PVT controls
- Verify PVT group defaults to unchecked (don't configure) with greyed children.
- Check the configure checkbox; verify domain subgroups become interactive.
- Check Navigation time; verify Prefer TAI becomes enabled.
- Uncheck Navigation time and Time-pulse time; verify Prefer TAI is greyed.
- Check Time-pulse time; verify "Ensure message after pulse" becomes enabled.
- Uncheck Time-pulse time; verify "Ensure message after pulse" is greyed.
- Check Position; verify Prefer ECEF becomes enabled.
- Uncheck Position and Velocity; verify Prefer ECEF is greyed.

### Satellites controls
- Verify Satellites group defaults to unchecked with greyed children.
- Check configure; verify Satellite positions and Signals checkboxes become interactive.

### Raw controls
- Verify Raw group defaults to unchecked with greyed children.
- Check configure; verify Observations and Navigation data checkboxes become interactive.

### Apply with standardized protocols
- Set NMEA to Configure with RMC checked; click Apply; verify success.
- Set RTCM to Disable; click Apply; verify success.
- Set both to "Don't configure"; verify they are not included in the configuration request.

### Apply with proprietary messages
- Check PVT configure, select Navigation time and Position; click Apply; verify success.
- Check Satellites configure, select both children; click Apply; verify success.
- Leave Raw unconfigured; verify it is not included in the configuration request.

### Presets
- Click NMEA preset; verify NMEA switches to Configure with standard messages checked.
- Click Daemon preset; verify appropriate messages are selected.
- Manually adjust a selection after preset; verify the change is preserved.
