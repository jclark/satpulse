# Section-based dirty tracking and discard

## Goal
Track user interactions per UI section so that Apply only sends changed config properties. Show a descriptive pending-changes label listing dirty sections. Add a Discard button to revert the form to the last readback state. Remove the Refresh button (nothing external can change the receiver while the GUI holds the connection).

## Prerequisite
config-restructure.

## Problem
After a readback populates the form, `handleApply` sends every non-empty property to the backend, even if the user changed nothing. The `countPendingChanges` function does per-field dirty checking for a numeric badge, but `handleApply` ignores it. The count is also confusing -- it is not clear what counts as "one change" (e.g. unchecking a time pulse checkbox is one change, but the user thinks of it as part of editing time pulse settings).

## Design

### Properties vs options
The `ConfigTarget` sent on Apply has two parts: `Props` (config properties) and `Opts` (config options). Dirty tracking affects them differently:

- **Properties** (`ConfigProps`): dirty tracking controls what gets included in `Props`. Only properties from touched sections are sent. This matters because setting a property -- even to its current value -- may have side effects at the receiver level.
- **Options** (`ConfigOptions`): dirty tracking does NOT affect what gets included in `Opts`. Options default to zero values which have no effect, so sending them is harmless. The existing message group `*Change` flags and save/reset defaults already prevent unintended options. Dirty tracking for options only affects the pending-changes display and whether Discard is greyed out.

### Section-based grouping
Dirty tracking is per UI section, not per field. If the user interacts with anything in a section, the entire section is marked dirty and all its properties are sent on Apply. This matches the user's mental model: "I changed time pulse settings" rather than "I changed 3 individual fields."

The property sections and their associated `ConfigProps` properties:

| UI section | ConfigProps properties | PropIDs |
|---|---|---|
| Time pulse | timePulse (period, width, alignToGNSS, onlyWhenLocked, polarityRising), timeGNSS, antennaCableDelay | `PropIDTimePulse \| PropIDTimeGNSS \| PropIDAntennaCableDelay` |
| Time mode | mode | `PropIDMode` |
| Signals | signalsEnabled | `PropIDSignalsEnabled` |
| Other | minElevation | `PropIDMinElevation` |

The option sections (messages, persistent operations) already have their own change tracking mechanisms and are not changed by this plan.

### Interaction-based dirty detection
Each property section has a boolean "touched" flag, initially false. The flag is set to true when the user interacts with any field in the section (typing in an input, changing a select, clicking a checkbox). Once set, it stays true until Discard or a new readback.

This is the same approach the message groups already use with their `*Change` flags (e.g. `nmeaChange`, `rtcmChange`).

Interaction tracking is preferred over value comparison because config properties are an abstraction over the receiver's internal state. Setting a property to its current value may still produce subtle differences in the receiver's underlying state, so a user who deliberately touches a field should have that change sent.

### Pending changes display
Replace `"{N} pending"` with a descriptive label listing dirty section names, e.g.:
- "Changes pending to time pulse, signals, messages"
- "Changes pending to time mode"

The label collects names from:
- Property sections whose touched flag is true
- "messages" if any message group change flag is set
- "save" if save type is non-default
- "reset" if reset type is non-default

If nothing is pending, the label is empty (hidden).

### Discard button
A Discard button replaces the Refresh button in the action bar. It is always visible but greyed out when nothing is pending. When clicked:
- Restores all property form fields to the last readback values (re-runs `populateFromConfig`)
- Clears all property section touched flags
- Resets message group change flags to false
- Resets save/reset radio groups to defaults (0)

This is a purely local operation -- it does not re-read from the receiver. Since nothing external can change the receiver while the GUI holds the connection, there is no need for a Refresh button that re-reads.

### Action bar layout

```
Changes pending to time pulse, signals  [Discard]     [Apply]
```

The pending label and Discard button sit together on the left, making it clear what Discard applies to. Apply is on the right.

### Apply sends only touched property sections
`handleApply` builds `Props` by only including properties from sections whose touched flag is true. If the time pulse section is not touched, none of its properties are included. If touched, all of its properties are included.

Within a touched section, individual fields that are empty/unset are still omitted (e.g. if time pulse is touched but cableDelay is empty, cableDelay is not sent). The touched flag gates whether to consider the section; within it, the existing non-empty guards still apply.

Options (`Opts`) are always built the same way as before -- the touched flags do not affect them.

## Steps

### 1. Add per-section touched state
Add boolean state variables for each property section:

```typescript
const [timePulseTouched, setTimePulseTouched] = useState(false);
const [timeModeTouched, setTimeModeTouched] = useState(false);
const [signalsTouched, setSignalsTouched] = useState(false);
const [otherTouched, setOtherTouched] = useState(false);
```

### 2. Wire up interaction handlers
For each form field, set the section's touched flag on user interaction. For example, the PPS period input's `onInput` handler becomes:

```typescript
onInput={e => { setTimePulseTouched(true); setPpsPeriod((e.target as HTMLInputElement).value); }}
```

Similarly for all other fields in each section. Signal checkbox toggling sets `signalsTouched`, mode select sets `timeModeTouched`, minElev input sets `otherTouched`.

### 3. Remove `countPendingChanges`, add `pendingLabel`
Remove the `countPendingChanges` function and `pendingCount` variable. Compute `pendingLabel` by collecting the names of touched/changed sections:

```typescript
const pendingSections: string[] = [];
if (timePulseTouched) pendingSections.push('time pulse');
if (timeModeTouched) pendingSections.push('time mode');
if (signalsTouched) pendingSections.push('signals');
if (otherTouched) pendingSections.push('other');
if (nmeaChange || rtcmChange || pvtChange || satsChange || rawChange) pendingSections.push('messages');
if (saveType) pendingSections.push('save');
if (resetType) pendingSections.push('reset');
const pendingLabel = pendingSections.length > 0
    ? 'Changes pending to ' + pendingSections.join(', ')
    : '';
```

### 4. Replace Refresh with Discard in the action bar
Remove the Refresh button. Add a Discard button that is always visible but disabled when `!pendingLabel`:

```tsx
<div class="shrink-0 flex items-center gap-2 px-4 py-2 border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
    {pendingLabel && (
        <span class="text-[10px] text-amber-500 font-medium">{pendingLabel}</span>
    )}
    <span class="ml-auto" />
    <button class={btnClass} disabled={!connected || !pendingLabel} onClick={handleDiscard}>
        Discard
    </button>
    <button class={btnPrimary} disabled={!connected || hasErrors || applying || !pendingLabel} onClick={handleApply}>
        {applying ? 'Applying...' : 'Apply'}
    </button>
</div>
```

Note: Apply is also disabled when nothing is pending, since there is nothing to send.

### 5. Implement handleDiscard
Add a `handleDiscard` function that restores form state from readback and clears all dirty flags:

```typescript
const handleDiscard = () => {
    // Restore property fields from last readback
    if (configProps) populateFromConfig(configProps);
    // Clear property touched flags
    setTimePulseTouched(false);
    setTimeModeTouched(false);
    setSignalsTouched(false);
    setOtherTouched(false);
    // Clear message change flags
    setNmeaChange(false);
    setRtcmChange(false);
    setPvtChange(false);
    setSatsChange(false);
    setRawChange(false);
    // Reset persistent operations
    setSaveType(0);
    setResetType(0);
};
```

### 6. Update handleApply to use touched flags
Change `handleApply` to only include properties from touched sections:

```typescript
const handleApply = async () => {
    const props: Record<string, any> = {};
    if (timePulseTouched) {
        if (ppsPeriod !== '' || ppsWidth !== '') {
            props.timePulse = { ... };
        }
        if (timeGNSS) props.timeGNSS = timeGNSS;
        if (cableDelay !== '') props.antennaCableDelay = ...;
    }
    if (timeModeTouched && mode) {
        props.mode = { static: mode === 'static' };
    }
    if (signalsTouched && selectedSignals.size > 0) {
        props.signalsEnabled = signalSetToMap(selectedSignals);
    }
    if (otherTouched) {
        if (minElev !== '') props.minElevation = ...;
    }
    // Opts built as before -- not affected by touched flags
    const opts: Record<string, any> = {};
    // ... existing opts logic unchanged ...
};
```

### 7. Clear touched flags after successful Apply
After a successful Apply, clear all touched flags. The values in the form now represent what was sent to the receiver, so they are the new baseline:

```typescript
if (r.ok) {
    setTimePulseTouched(false);
    setTimeModeTouched(false);
    setSignalsTouched(false);
    setOtherTouched(false);
    // Message change flags are already reset by existing code
}
```

### 8. Remove the Refresh button; clear touched flags on new readback
Remove the Refresh button from the action bar. Keep the `doReadback` function, `reading` state, and `hasReadback` ref -- the automatic readback on first tab switch while connected is still needed. On disconnect, `hasReadback` resets to false so that reconnecting (possibly to a different receiver) triggers a fresh readback. There is no need for a manual Refresh button since nothing external can change the receiver while the GUI holds the connection.

When a new readback occurs (e.g. after reconnect), clear all touched flags and message change flags in addition to repopulating the form. The old form state is stale -- the new readback is the baseline. This can be done in `populateFromConfig` or in the effect that calls it.

## Result
- Apply only sends properties from sections the user actually interacted with
- The pending label tells the user exactly which sections will be affected: "Changes pending to time pulse, signals, messages"
- Discard button lets the user revert all changes without re-reading from the receiver
- Refresh button removed (redundant -- receiver cannot change externally)
- No backend changes required -- `ConfigProps.UnmarshalJSON` already handles partial property sets via the `valid` bitmask

## Files changed
- `desktop/frontend/src/config-panel.tsx` -- add per-section touched flags, replace `countPendingChanges` with `pendingLabel`, replace Refresh with Discard, update `handleApply`

## Testing -- Playwright

### No changes after readback
- Connect and wait for config tab to populate
- Verify no "Changes pending" label is shown
- Verify Discard button is greyed out
- Verify Apply button is greyed out

### Single section touched
- After readback, change only the PPS period field
- Verify "Changes pending to time pulse" appears
- Verify Discard and Apply are enabled
- Click Apply; verify only time pulse properties are sent

### Multiple sections touched
- Change mode to mobile and edit a signal checkbox
- Verify "Changes pending to time mode, signals" appears

### Messages included in label
- Check a message group's change checkbox (e.g. enable NMEA)
- Verify "messages" appears in the label alongside any touched property sections

### Persistent operations in label
- Select Save: Changes
- Verify "save" appears in the label

### Discard restores form
- After readback, change PPS period and mode
- Verify "Changes pending to time pulse, time mode" appears
- Click Discard
- Verify form fields are restored to readback values
- Verify no "Changes pending" label is shown
- Verify Discard is greyed out

### Touched flag clears after Apply
- Change PPS period; click Apply
- After success, verify no "Changes pending" label
- Verify Discard is greyed out
