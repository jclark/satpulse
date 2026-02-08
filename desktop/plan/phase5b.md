# Phase 5b: Layout rework -- tabs + collapsible panels

## Context

The current layout uses `react-resizable-panels` for everything, resulting in panels that stretch to fill space (Time panel claims 80% of screen for 6 rows of data), a confusing three-column layout, and a "Panels" dropdown that doesn't map well to the actual UX.

This replaces that with a tab-based layout inspired by GNSS receiver tools (u-center, etc.) where monitoring, packet inspection, and configuration are separate activities.

## Prerequisite

Phase 5a (config panel restructure). The config panel already has collapsible sections -- this phase changes where it lives and how the overall layout works.

## New design

```
┌──────────────────────────────────────────────┐
│ SatPulse GPS   ZED-F9P (FW 2.01)            │
│ Device [...........] Speed [9600] [Connect]  │
├──────────────────────────────────────────────┤
│ [Monitor]  [Packets]  [Configuration]         │
├──────────────────────────────────────────────┤
│                                              │
│  Tab content (fills available space)         │
│                                              │
├──────────────────────────────────────────────┤
│ Activity log (always visible, resizable)     │
├──────────────────────────────────────────────┤
│ Status bar                                   │
└──────────────────────────────────────────────┘
```

### Connection bar (top, fixed)
- Same as now: device, speed, connect/disconnect
- Receiver identity string shown when connected (already implemented): "ZED-F9P (FW 2.01 PROTVER 29.0)"
- Remove "Panels" dropdown -- no longer needed
- Remove `panelVisibility` and `onTogglePanel` props

### Tab bar
Three tabs, simple `<button>` elements, no library needed:
- **Monitor** (default) -- live telemetry sections
- **Packets** -- full-height packet log with bottom toolbar
- **Configuration** -- disabled until receiver is identified

State: `activeTab: 'monitor' | 'packets' | 'config'`

### Monitor tab content
Vertically stacked collapsible sections, NOT resizable panels. Each section sizes to its content when expanded, shows only its header when collapsed. Sections:

- **Time** -- key-value data, always expanded by default
- **Survey** -- key-value data, collapsed by default, auto-expands on first survey data

Future phases will add more sections here: Messages (phase 5d), Sky view and Signal levels (phase 6).

The key insight: Time and Survey have small fixed content, so they should be fixed-height when expanded (sized to content). This is just CSS flexbox, not a panel library.

Each section has a clickable header bar (title + collapse arrow). Clicking toggles expand/collapse. Use a `CollapsibleSection`-style component with `expanded: boolean` and `onToggle` callback. The existing `CollapsibleSection` component (used inside config panel) already does this -- the monitor tab sections can use the same component or a variant with slightly different styling (a panel-header style bar rather than the config section's uppercase label).

### Packets tab content
Full-height scrolling packet log. The entire tab content area is the packet display.

**Bottom toolbar** with:
- **Freeze/Resume** toggle -- when frozen, new packets are still received and buffered but the display stops updating and auto-scroll is disabled. Visual indicator when frozen (e.g. button changes colour, shows "Frozen" label). Clicking Resume scrolls to the latest packet and resumes live updates.
- **Search/filter** -- text input to filter displayed packets
- **Clear** -- clears the packet buffer

The bottom placement keeps the toolbar near the eye when scrolling through packet data.

### Configuration tab content
- Scrollable area with collapsible sections (Time pulse, Time mode, Constellations, Other, Persistent operations) -- same as current `ConfigPanel` internals
- **No** sticky top bar with Close/Refresh/Apply
- Instead: pinned **bottom action bar** with Refresh and Apply buttons (dialog-style)
- Readback triggers on first switch to the config tab (same `hasReadback` logic, but keyed on `activeTab === 'config'` transition instead of `connected`)
- The tab itself is disabled until `receiver.status === 'identified'`
- Tab content stays mounted across tab switches (preserves form state silently, no unsaved-changes warning)

### Activity log
The activity log is **always visible** -- it cannot be collapsed or hidden. It sits below the tab content area and above the status bar.

Implementation: a `<div>` with a fixed initial height (e.g. 150px), `min-height: 60px`, `max-height: 50vh`, and CSS `resize: vertical` so the user can drag to make it taller or shorter but never remove it entirely. The native resize handle appears in the bottom-right corner. `overflow: hidden` on the log container ensures the resize handle works correctly, with `overflow-y: auto` on the inner scrollable area.

This replaces `react-resizable-panels` entirely -- no panel library needed anywhere.

### What gets removed
- `react-resizable-panels` -- removed entirely (uninstall the package)
- `receiver-panel.tsx` -- deleted (receiver identity already in connection bar)
- `PanelVisibility` type and all panel show/hide state (`panels`, `togglePanel`, `showLeft`/`showCenter`/`showRight`/`showMiddle`)
- `panelLabels` array and Panels dropdown menu in connection bar
- `separatorH` / `separatorV` CSS class constants
- All `<Group>`, `<Panel>`, `<Separator>` usage in `app.tsx`

### What stays
- `CollapsibleSection` component (used in both config tab and monitor tab)
- All panel content components: `TimePanel`, `SurveyPanel`, `MonitorPanel`, `ConfigPanel`, `LoggingPanel`
- Signal picker dialog
- Toast system
- Receiver identity display in connection bar (already implemented)

## Current state of the code

Key things to know about the existing codebase before implementing:

- `app.tsx` uses `react-resizable-panels` (`Group`, `Panel`, `Separator`) for a three-column layout with the activity log below
- `connection-panel.tsx` has a "Panels" dropdown with checkboxes for each panel, controlled by `PanelVisibility` state
- `receiver-panel.tsx` exports `ReceiverState` type (used by app.tsx) and a `ReceiverPanel` component -- the type must be preserved (move to app.tsx or a types file), the component is deleted
- `config-panel.tsx` has a sticky top action bar (Refresh/Apply) and triggers readback on `connected` change -- needs bottom action bar and readback trigger on tab switch
- `monitor-panel.tsx` is a simple packet log with Clear button and auto-scroll -- needs Freeze/Resume, search filter, and bottom toolbar
- `time-panel.tsx` and `survey-panel.tsx` each render their own `<h2>` headings and handle the "waiting for data" state internally -- these need collapsible headers
- `collapsible-section.tsx` manages its own open/close state with `useState(defaultOpen)` -- monitor tab sections need external state control for survey auto-expand
- `logging-panel.tsx` has its own toolbar with level filter, component filter, search, and clear -- this stays as-is, wrapped in the always-visible log container

## Files changed
1. `desktop/frontend/src/app.tsx` -- major rewrite: tab state, flexbox layout, no panel library
2. `desktop/frontend/src/connection-panel.tsx` -- remove Panels dropdown and panel visibility props
3. `desktop/frontend/src/config-panel.tsx` -- action bar from top to bottom, readback on tab switch instead of connect
4. `desktop/frontend/src/monitor-panel.tsx` -- add Freeze/Resume, search filter, move toolbar to bottom
5. `desktop/frontend/src/time-panel.tsx` -- add collapsible header
6. `desktop/frontend/src/survey-panel.tsx` -- add collapsible header, support external expand trigger
7. `desktop/frontend/src/receiver-panel.tsx` -- delete (move `ReceiverState` type to app.tsx)

## Steps
1. Move `ReceiverState` type from `receiver-panel.tsx` to `app.tsx`, delete `receiver-panel.tsx`
2. Update `TimePanel` and `SurveyPanel` to use collapsible headers (clickable bar with collapse arrow)
3. Update `MonitorPanel` -- add Freeze/Resume toggle, search filter, move toolbar to bottom
4. Update `ConfigPanel` -- move action bar to bottom, accept `visible: boolean` prop to trigger readback on tab switch
5. Rewrite `app.tsx` layout: connection bar, tab bar, tab content area, always-visible log, status bar. Remove all `react-resizable-panels` usage
6. Simplify `connection-panel.tsx` -- remove Panels dropdown and all panel visibility props
7. Uninstall `react-resizable-panels` package
8. Test in browser
