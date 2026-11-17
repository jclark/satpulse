# Time mode section rework

## Goal
Replace the minimal static/mobile selector in the Time mode section with a full three-mode selector (mobile, survey-in, fixed position) that handles mode properties, survey operations, survey message enablement, and fixed-position parameters in a single cohesive section.

## Prerequisite
config-restructure. message-config (for PVT survey message flag).

## Reference documents
- [ui-panel-configuration.md](ui-panel-configuration.md) -- Time mode property group
- [message-semantics.md](message-semantics.md) -- PVTMsgSurvey semantics
- `gps/gpsprot/configtarget.go` -- Mode, Survey, ConfigProps, ConfigOptions
- `docs/man/satpulsetool-gps.1.md` -- CLI options for --mobile, --survey, --fixed-pos-ecef, --fixed-pos-acc

## Background

### Properties vs options
The GPS configuration model splits between *properties* (persistent receiver state that can be read back) and *options* (one-shot instructions for a configuration run). The time mode section straddles both:

**Property** (`ConfigProps.Mode`):
- `static` (bool) -- whether the receiver assumes the antenna is stationary
- `fixedPosECEF` ([3]float64) -- ECEF coordinates for fixed position
- `fixedPosLLH` ([2]float64) + `height` (float64) -- lat/lon/height for fixed position
- `fixedPosAcc` (float64) -- accuracy of fixed position in meters
- `posType` -- which coordinate system (ECEF or LLH) is used

The mode property is read back from the receiver and populated on Refresh.

**Options** (`ConfigOptions`):
- `Survey` struct -- parameters for a survey-in operation:
  - `Flags` (`SurveyAgain`) -- do a survey even if one was done already
  - `MinDur` -- minimum survey duration
  - `AccLimit` -- required accuracy
- `SetStatic` (bool) -- ensure static mode without changing fixed position

**Message flag** (`ConfigOptions.PVTMsg`):
- `PVTMsgSurvey` -- enable survey progress messages

### How the CLI maps to the model
The CLI presents three mutually exclusive modes:

| CLI | Mode property | Options |
|---|---|---|
| `--mobile` | `{static: false}` | -- |
| `--survey` | `{static: true}` | `Survey{Flags: SurveyAgain, MinDur: ..., AccLimit: ...}` |
| `--fixed-pos-ecef X,Y,Z` | `{static: true, posType: ECEF, fixedPosECEF: [...], fixedPosAcc: ...}` | -- |

Survey is the subtle case: selecting survey sets the mode property to static *and* sets the Survey option to trigger a new survey. The survey parameters (time, accuracy) are options, not properties -- they are instructions for this run, not persistent receiver state.

### Coordinate systems
The `Mode` struct supports two position types:
- **ECEF** (Earth-Centered, Earth-Fixed) -- three Cartesian coordinates in meters. This is what the CLI currently supports via `--fixed-pos-ecef`.
- **LLH** (Latitude, Longitude, Height) -- two angles in degrees plus height in meters. The `ConfigProps` JSON marshaling and unmarshaling already support `fixedPosLLH` and `height` fields.

The desktop UI supports both coordinate entry modes for fixed position.

### Survey progress
Survey-in progress is monitored via `SurveyMsg` events (already displayed in the Monitor tab's Survey section from layout-rework). The time mode section does not display live survey progress -- it configures whether to *start* a survey and with what parameters.

Enabling `PVTMsgSurvey` tells the receiver to emit survey progress messages. The UI automatically includes this flag when the user selects survey mode, so the Monitor tab shows progress.

## Concept

### Layout
All controls are always visible. Controls are greyed out when their parent mode is not selected, following the same pattern as message groups (NMEA/RTCM three-way selectors grey out children but preserve selections).

```
Mode:  ( ) Mobile  ( ) Survey-in  ( ) Fixed position

Survey-in                                  [greyed unless Survey-in]
  Survey time (s)      [         ]         placeholder "2000"       [ ] Do a new survey
  Survey accuracy (m)  [         ]         placeholder "20"
  [x] Report survey progress

Fixed position                             [greyed unless Fixed position]
  Coordinates:  (x) ECEF  ( ) Lat/Lon/Height
  X (m)  [           ]    Latitude (deg)   [         ]
  Y (m)  [           ]    Longitude (deg)  [         ]
  Z (m)  [           ]    Height (m)       [         ]
  Position accuracy (m)  [20  ]
```

Two levels of greying for fixed position: the whole group is greyed unless Fixed position is selected, and within it the ECEF column is greyed when LLH is selected and vice versa. Position accuracy is shared and active whenever Fixed position is selected.

### Survey parameters are placeholders, not readback
Survey time and accuracy are one-shot options with no readback. Their defaults (2000s, 20m) are shown as placeholder text, not pre-filled values. This makes them visually distinct from readback-populated fields (which show normal text). On Apply, empty fields use the placeholder defaults.

### Readback population
When config is read back, the mode property determines the selector state:
- `static: false` -> Mobile selected
- `static: true` with `fixedPosECEF` -> Fixed position selected, ECEF radio selected, ECEF fields populated with readback values
- `static: true` with `fixedPosLLH` -> Fixed position selected, LLH radio selected, LLH and height fields populated
- `static: true` with no position -> no mode selected; display info note "Receiver is in stationary mode"

The subtle case is `static: true` with no position. This typically means the receiver completed a survey or was set to static mode. Since we cannot distinguish from readback alone, we do not pre-select Survey-in (which means "start a *new* survey on Apply"). The user must explicitly select a mode to make changes.

If readback includes `fixedPosAcc`, it populates the position accuracy field.

### Survey message interaction with PVT section
Survey progress (`PVTMsgSurvey`) belongs here, not in the Messages > PVT group -- it is contextually tied to survey-in. The PVT section in Messages already excludes survey per spec.

When the user checks "Report survey progress", the flag is OR'd into `Opts.PVTMsg` alongside whatever the Messages > PVT group contributes. They are independent and both feed into the same `Opts.PVTMsg` on Apply.

### Validation
- Survey time: must be a positive number when survey is selected and field is non-empty.
- Survey accuracy: must be >= 0.001 when survey is selected and field is non-empty.
- ECEF X, Y, Z: must be valid numbers when fixed + ECEF is selected.
- LLH latitude: -90 to 90 degrees when fixed + LLH is selected.
- LLH longitude: -180 to 180 degrees when fixed + LLH is selected.
- LLH height: must be a valid number when fixed + LLH is selected.
- Position accuracy: must be >= 0.001 when fixed position is selected.

---

## Steps

### 1. Add state variables for time mode
In `config-panel.tsx`, replace the single `mode` string state with:
- `timeMode`: `'' | 'mobile' | 'survey' | 'fixed'` -- three-way selector plus empty (no selection)
- `surveyTime`: string (empty; placeholder shows `2000`)
- `surveyAcc`: string (empty; placeholder shows `20`)
- `surveyAgain`: boolean (default `false`) -- maps to `SurveyAgain` flag
- `surveyReport`: boolean (default `true`)
- `coordSystem`: `'ecef' | 'llh'` (default `'ecef'`)
- `fixedECEF`: `[string, string, string]` (X, Y, Z in meters)
- `fixedLLH`: `[string, string, string]` (lat, lon, height in meters)
- `fixedPosAcc`: string (empty; placeholder shows `20`)

Remove the old `mode` state variable.

### 2. Update readback population
In `populateFromConfig`, parse the mode object from readback:
- `mode.static === false` -> set `timeMode` to `'mobile'`
- `mode.static === true` with `fixedPosECEF` -> set `timeMode` to `'fixed'`, `coordSystem` to `'ecef'`, populate ECEF fields from readback values
- `mode.static === true` with `fixedPosLLH` -> set `timeMode` to `'fixed'`, `coordSystem` to `'llh'`, populate LLH fields and height
- `mode.static === true` with no position -> set `timeMode` to `''`
- Populate `fixedPosAcc` from `mode.fixedPosAcc` if present

### 3. Render the time mode section
Replace the current Time mode `CollapsibleSection` content:

**Mode radio group:** three radios in a row (Mobile, Survey-in, Fixed position).

**Survey-in group:** labelled group with all controls disabled unless `timeMode === 'survey'`. Contains survey time input (placeholder "2000") with "Do a new survey" checkbox aligned to its right, survey accuracy input (placeholder "20"), and report survey progress checkbox.

**Fixed position group:** labelled group with all controls disabled unless `timeMode === 'fixed'`. Contains:
- Coordinate system radio (ECEF / Lat/Lon/Height)
- Two side-by-side columns: ECEF fields (X, Y, Z) on the left, LLH fields (Latitude, Longitude, Height) on the right. ECEF column disabled when LLH is selected and vice versa.
- Position accuracy field below, spanning full width.

When `timeMode` is empty and readback showed `static: true`, show a small info line: "Receiver is in stationary mode."

Use the same grid layout and styling patterns as the time pulse section.

### 4. Update validation
Extend `validateFields` with the new fields. Validation only applies when the relevant mode is selected:
- Survey fields validated only when `timeMode === 'survey'`
- ECEF fields validated only when `timeMode === 'fixed'` and `coordSystem === 'ecef'`
- LLH fields validated only when `timeMode === 'fixed'` and `coordSystem === 'llh'`
- Position accuracy validated only when `timeMode === 'fixed'`

### 5. Update Apply handler
In `handleApply`, build the mode/survey portion of the config target based on `timeMode`:

**Mobile:**
- `props.mode = { static: false }`

**Survey-in:**
- `props.mode = { static: true }`
- `opts.Survey = { Flags: surveyAgain ? 1 : 0, MinDur: ..., AccLimit: ... }` using field values or placeholder defaults
- If `surveyReport` is checked, OR `PVTMsgSurvey` into `opts.PVTMsg`

**Fixed position (ECEF):**
- `props.mode = { static: true, fixedPosECEF: [x, y, z], fixedPosAcc: ... }`

**Fixed position (LLH):**
- `props.mode = { static: true, fixedPosLLH: [lat, lon], height: ..., fixedPosAcc: ... }`

**No selection (empty):**
- Do not include `mode` in props.

Note: check how `ConfigOptions.Survey` serializes in JSON. `MinDur` is `time.Duration` (nanoseconds as integer), `AccLimit` is `Length` (micrometers as integer). The frontend must produce matching integer values.

### 6. Update pending changes count
Update `countPendingChanges` for the new time mode fields. Changes are pending when:
- `timeMode` differs from readback-derived mode
- Fixed position coordinates or accuracy differ from readback values
- `timeMode === 'survey'` always counts as pending (survey is an action, not readback state)

### 7. Reset survey fields after Apply
After Apply completes (success or failure):
- Clear `surveyTime` and `surveyAcc` to empty (back to placeholder defaults)
- Reset `surveyAgain` to `false`
- Reset `surveyReport` to `true`
- Do not reset `timeMode` -- the mode property persists and will be updated by any post-Apply readback

This mirrors how Save/Reset radio groups reset to defaults after Apply.

### 8. Export PVTMsgSurvey from msg-flags.ts
Ensure `PVTMsgSurvey` is exported from `msg-flags.ts`. The time mode section needs it for the survey report checkbox.

## Result
The Time mode section supports all three positioning modes with appropriate controls: mobile (no parameters), survey-in (time, accuracy, progress reporting), and fixed position (ECEF or LLH coordinates with accuracy). All controls are always visible with greyed-out states. Survey parameters use placeholder text to distinguish them from readback values. Survey message enablement is integrated directly into the section.

## Files changed
- `desktop/frontend/src/config-panel.tsx` -- reworked time mode section, new state, updated apply/readback/validation
- `desktop/frontend/src/msg-flags.ts` -- export `PVTMsgSurvey`

## Testing -- Playwright

### Mode selector
- Verify the three radio options (Mobile, Survey-in, Fixed position) are visible.
- Select Mobile; verify survey and fixed position fields are greyed out.
- Select Survey-in; verify survey fields are enabled, fixed position fields greyed.
- Select Fixed position; verify fixed position fields are enabled, survey fields greyed.

### Survey-in fields
- Select Survey-in; verify fields are empty with placeholder text showing defaults.
- Enter survey time 300; verify the field shows normal text.
- Enter invalid survey accuracy 0; verify validation error.
- Uncheck "Report survey progress"; verify the checkbox unchecks.
- Clear survey time; verify placeholder "2000" reappears.

### Fixed position ECEF
- Select Fixed position; verify ECEF is the default coordinate system.
- Verify ECEF fields are enabled, LLH fields are greyed.
- Enter X, Y, Z coordinates; verify fields accept input.
- Enter position accuracy 0; verify validation error.

### Fixed position LLH
- Select Fixed position, then select LLH coordinate system.
- Verify LLH fields are enabled, ECEF fields are greyed.
- Enter latitude 91; verify validation error.
- Enter valid coordinates; verify no validation error.

### Readback
- Connect and refresh; if receiver is in mobile mode, verify Mobile is selected.
- If receiver has a fixed ECEF position, verify Fixed position is selected with ECEF radio active and coordinates populated as normal text.

### Apply and reset
- Select Survey-in, enter time 300; click Apply.
- After Apply completes, verify survey time field is cleared back to placeholder.
- Verify mode selector still reflects the post-Apply readback state.
