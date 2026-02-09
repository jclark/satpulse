# Phase 5c: Message configuration

## Goal
Add a Messages section to the configuration tab with output message controls for standardized protocols (NMEA, RTCM) and proprietary messages (PVT, satellites, raw). This controls *what the receiver sends*.

UI design: [ui-panel-configuration.md](ui-panel-configuration.md) (Messages section).
Flag semantics: [message-semantics.md](message-semantics.md).

## Prerequisite
Phase 5b (layout rework -- configuration is now a tab). Phase 5a (config panel has collapsible sections and Apply flow). `ConfigOptions` uses `opt.Val[T]` with `omitzero`. `ConfigProps` and `PropIDs` have `UnmarshalJSON` (done in `configprops-json.md`). `ConfigTarget` is fully JSON round-trippable.

Steps 1-2 are prerequisites (refactor only, no new functionality). They replace hand-rolled DTOs with direct use of GPS subsystem types. Test after completing steps 1-2 to verify existing config panel still works before adding message controls in steps 3+.

## Context
`ConfigTarget` is directly JSON-deserializable. Its three fields -- `Props` (ConfigProps), `Get` (PropIDs), and `Opts` (ConfigOptions) -- all support JSON round-trip. The Wails adapter can accept a `ConfigTarget` and pass it straight through to `gpscfg.Configure` with no DTO or translation layer.

The current `app.go` has two problems:
1. `ConfigUpdate` is a hand-rolled DTO with manual setter-method translation for every property. Replaced by accepting `ConfigTarget` directly.
2. `ReceiverInfo` conflates receiver identity (from connect) with config readback (from config tab). `ReadConfig` should return `ConfigProps` directly, not wrap it in `ReceiverInfo`.

---

## Steps

### 1. Separate ReceiverInfo from config readback

`ReceiverInfo` and config readback are two different operations that happen at different times:
- **Receiver identity** (`probeWorker`) -- runs on connect, emits `ReceiverInfo` via `gps:receiver` event.
- **Config readback** (`ReadConfig`) -- runs when the config tab opens, returns current property values.

Currently `app.go` defines its own `ReceiverInfo` and `ProbeResult` DTOs that repack fields from `gpsprot.ReceiverInfo` and `gpsprot.ConfigProps`. The `DetectReceiver` method duplicates `probeWorker` and is unused by the frontend. The thin-layer approach: use the GPS subsystem types directly.

**Delete** the desktop-local `ReceiverInfo`, `ProbeResult`, `DetectReceiver`, `configPropsToMap`, `SignalInfo`, `GNSSInfo`, and `GetAllSignals`. These are all hand-rolled DTOs or translations that the GPS subsystem types already handle.

**`probeWorker`** -- emit `gpsprot.ReceiverInfo` directly as the `gps:receiver` event payload (plus `PacketFormatsDetected`). Use a small envelope struct for the event since we need ok/error/packetFormats:
```go
type ReceiverEvent struct {
    OK            bool                          `json:"ok"`
    Error         string                        `json:"error,omitempty"`
    Info          opt.Val[gpsprot.ReceiverInfo]  `json:",omitzero"`
    PacketFormats []string                      `json:"packetFormats,omitempty"`
}
```

**`GetReceiverState`** -- return `ReceiverEvent` (cached from probeWorker).

**`ReadConfig`** -- return `(*gpsprot.ConfigProps, error)`. Wails serializes `ConfigProps` using its `MarshalJSON`, so the frontend receives the same JSON shape it will send back. On error, Wails rejects the JS promise.

```go
func (a *App) ReadConfig() (*gpsprot.ConfigProps, error) {
    ...
    return rslt.ConfigProps, nil
}
```

Update the frontend `doReadback` to use try/catch (already has one) and consume `ConfigProps` JSON directly from `ReadConfig` (e.g. `signalsEnabled` is `{"GPS": ["L1", "L5"]}`, `timePulse` is `{period: 1.0, width: 0.0001, ...}`, `mode` is `{static: true}`).

### 2. Replace ConfigUpdate with ConfigTarget

Delete `ConfigUpdate` and `buildSignalSet` from `app.go`. Replace `ApplyConfig` with a method that accepts `gpsprot.ConfigTarget` directly:

```go
func (a *App) ApplyConfig(cfg gpsprot.ConfigTarget) Result {
    a.mu.Lock()
    conn := a.conn
    a.mu.Unlock()
    if conn == nil {
        return Result{Error: "not connected"}
    }
    if cfg.NoOp() {
        return Result{Error: "no configuration changes specified"}
    }
    return a.runConfig(&cfg)
}
```

No translation, no DTO. The frontend sends JSON matching `ConfigTarget`'s shape. Wails deserializes it using the existing `UnmarshalJSON` methods.

Update the frontend `handleApply` to build `ConfigTarget`-shaped JSON. Property fields go under `Props` using the `ConfigProps` JSON format (same shape as received from `ReadConfig`). Options go under `Opts`. Example:

```typescript
const cfg: Record<string, any> = {};
// Properties
const props: Record<string, any> = {};
if (...) props.timePulse = { period: ..., width: ..., ... };
if (...) props.mode = { static: true };
if (...) props.signalsEnabled = {"GPS": ["L1", "L5"], ...};
cfg.Props = props;
// Options (messages, save, reset)
const opts: Record<string, any> = {};
if (...) opts.PVTMsg = pvtFlags;
cfg.Opts = opts;
```

The `SaveConfig` and `ResetConfig` methods remain unchanged -- they build `ConfigTarget` directly in Go.

Signal selection changes from index-based to name-based. The frontend tracks signals as `Set<string>` where each entry is `"GNSS:Signal"` (e.g. `"GPS:L1"`, `"GAL:E5a"`). This uniquely identifies a signal with no integer indices to keep in sync.

Conversion to/from the `map[string][]string` wire format (`{"GPS": ["L1", "L5"], ...}`) used by `ConfigProps.signalsEnabled` is straightforward:
- Readback: iterate map entries, add `"${gnss}:${sig}"` to set.
- Apply: split each set entry on `:`, group into map.

`GetAllSignals(gnss []string) map[string][]string` returns the full signal catalog for the given GNSS constellations (from `ReceiverEvent.Info.SupportedGNSS`). The frontend converts this to `Set<string>` to know what's available. `SignalInfo`, `GNSSInfo` types are deleted.

### 3. Export flag constants for the frontend
Create `desktop/frontend/src/msg-flags.ts` with integer constants matching the Go iota definitions in `gps/gpsprot/configtarget.go`:

```typescript
// Source: gps/gpsprot/configtarget.go -- keep in sync.

// NMEA
export const NMEAMsgRMC   = 1 << 0
export const NMEAMsgGGA   = 1 << 1
export const NMEAMsgGSA   = 1 << 2
export const NMEAMsgGSV   = 1 << 3
export const NMEAMsgZDA   = 1 << 4
export const NMEAMsgVTG   = 1 << 5
export const NMEAMsgOther = 1 << 15

// RTCM
export const RTCMMsgMSM4  = 1 << 0
export const RTCMMsgMSM7  = 1 << 3
export const RTCMMsgARP   = 1 << 4
export const RTCMMsgLax   = 1 << 5
export const RTCMMsgOther = 1 << 15

// PVT
export const PVTMsgPos            = 1 << 0
export const PVTMsgVel            = 1 << 1
export const PVTMsgTime           = 1 << 2
export const PVTMsgTimePulse      = 1 << 3
export const PVTMsgLeapSecond     = 1 << 4
export const PVTMsgSurvey         = 1 << 5
export const PVTMsgTAI            = 1 << 6
export const PVTMsgECEF           = 1 << 7
export const PVTMsgTimePulseAfter = 1 << 8
export const PVTMsgOff            = 1 << 9

// Sats
export const SatsMsgSat    = 1 << 0
export const SatsMsgSignal = 1 << 1

// Raw
export const RawMsgObs     = 1 << 0
export const RawMsgNavData = 1 << 1
```

### 4. Build three-way selector component
Create a reusable `ThreeWaySelector` component for NMEA and RTCM. Three states: Don't configure / Configure / Disable. Rendered as a segmented control or radio group. The component:
- accepts current state and onChange callback
- renders the three options with clear labels
- is used in the group box border/header area

The three-way state maps to the JSON value sent to the backend:
- Don't configure -> `NMEAMsg`/`RTCMMsg` field omitted from JSON (`opt.Val` not set)
- Configure -> field present with `selected flags | Other`
- Disable -> field present with value 0

### 5. Build NMEA group UI
Bordered group box labelled "NMEA" with three-way selector in the header.

Children (enabled only in Configure state, greyed but preserved in other states):
- Checkboxes: RMC, GGA, GSA, GSV, ZDA, VTG

On Apply in Configure state, the frontend computes: `selected | NMEAMsgOther`.

### 6. Build RTCM group UI
Bordered group box labelled "RTCM" with three-way selector in the header.

Children (enabled only in Configure state, greyed but preserved in other states):
- MSM type: three-way radio -- None / MSM4 / MSM7
- Fallback checkbox (enabled only when MSM4 or MSM7 selected): "Use other MSM type if preferred isn't available"
- ARP checkbox (independent of MSM selection)

On Apply in Configure state, the frontend computes: `selected | RTCMMsgLax (if checked) | RTCMMsgOther`.

### 7. Build PVT group UI
Bordered group box labelled "PVT" with configure checkbox in the border.

Children (enabled only when checked), organised into two domain subgroups rendered as inner bordered boxes:

**Time** subgroup -- left column for information flags, right-aligned modifier:
- Navigation time checkbox
- Time-pulse time checkbox
  - Indented: Ensure message after pulse checkbox (greyed unless Time-pulse time is checked)
- Leap second checkbox
- Right-aligned: Prefer TAI checkbox (greyed unless Navigation time or Time-pulse time is checked)

**Position & velocity** subgroup -- left column for information flags, right-aligned modifier:
- Position checkbox
- Velocity checkbox
- Right-aligned: Prefer ECEF checkbox (greyed unless Position or Velocity is checked)

After the subgroups, standalone checkbox:
- Turn off unselected PVT messages

On Apply when configured, the frontend OR's the selected flags and sends as `PVTMsg`. When not configured, omits the field (zero = don't configure).

### 8. Build Satellites group UI
Bordered group box labelled "Satellites" with configure checkbox in the border.

Children (enabled only when checked):
- Satellite positions checkbox
- Signals checkbox

Unchecked -> field omitted; checked -> `SatsMsg: selected flags`.

### 9. Build Raw group UI
Bordered group box labelled "Raw" with configure checkbox in the border.

Children (enabled only when checked):
- Observations (RINEX .obs) checkbox
- Navigation data (RINEX .nav) checkbox

Unchecked -> field omitted; checked -> `RawMsg: selected flags`.

### 10. Assemble Messages section
Add a "Messages" collapsible section to the config panel between Properties and Persistent Operations. Inside, render all five message groups: NMEA, RTCM, PVT, Satellites, Raw.

### 11. Wire Apply
Extend the Apply handler in `config-panel.tsx` to collect message state from the form. Each group computes its integer flag value based on UI state. The frontend builds a `ConfigTarget`-shaped object:
- Message fields go under `Opts` (they are `ConfigOptions` fields).
- Property fields go under `Props` (they are `ConfigProps` fields, using ConfigProps JSON format).

Message controls are one-shot options, not readback-backed properties. They are NOT reset after apply (user can re-apply the same config).

### 12. Presets
Add preset buttons inside the Messages section, below the message groups:
- `NMEA preset` -- sets NMEA to Configure with a standard set of messages (RMC, GGA, GSA, GSV, ZDA)
- `Binary preset` -- sets proprietary message groups to a standard selection
- `Daemon preset` -- sets the message selection matching the daemon's startup configuration (from `time/app/daemon/gps.go`)

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
