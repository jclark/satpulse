# Phase 5b: Layout rework

## Goal
Rework the panel layout to make better use of screen real estate. Configuration becomes a slide-down overlay rather than a competing panel. Telemetry panels (Time, Survey, packet monitor) are the main view. All panels gain collapsible support.

## Prerequisite
Phase 5a (config panel restructure). The config panel already has collapsible sections and a sticky action bar -- this phase changes *where* it lives, not its internal structure.

## Motivation
The current three-column layout (Receiver+Config | Time+Survey | Packet monitor) has problems:
- Receiver and Time/Survey panels contain very little data but claim full columns of space.
- Configuration is the most complex panel but is crammed into a narrow column.
- The packet monitor is always visible and takes a full column, but it is a diagnostic tool most users do not need to watch constantly.
- The activity log claims 25% of vertical space by default.
- All panels have the same visual weight -- nothing signals which is the primary workspace.

## Concept

### Two modes of interaction
The UI has two fundamentally different interaction modes:

1. **Monitoring** (passive) -- watching live data: time, survey progress, packet traffic, future sky view and signal bars. These panels coexist naturally in a split layout.
2. **Configuration** (active) -- editing receiver settings and applying changes. This is a deliberate action that wants full width and is not done simultaneously with monitoring.

Configuration becomes a slide-down overlay that covers the telemetry area but leaves the activity log visible (so you can watch progress during Apply).

### Receiver identity
The receiver panel currently shows 5 lines of text (vendor, hardware, firmware, GNSS, protocols) in a full panel. This is reduced to a compact identity string in the connection bar: "ZED-F9P (FW 2.01 PROTVER 29.0)". The supported-GNSS list is not displayed -- instead it feeds into the config panel's constellation picker to grey out unsupported constellations. The protocols line is removed entirely.

### Panel collapse
`react-resizable-panels` supports `collapsible={true}` and `collapsedSize` on Panel components, with imperative `collapse()`/`expand()` via refs. All panels gain this support so users can collapse anything they do not need by dragging the separator past the minimum size.

---

## Steps

### 1. Enable collapsible on all panels
Add `collapsible={true}` to every `Panel` in `app.tsx`. Use `collapsedSize="0%"` for panels that have no header (Time, Survey, Packet monitor). For the activity log, use a `collapsedSize` equal to its toolbar height (approx 36px expressed as a percentage of the parent Group) so the toolbar (level filter, search, clear) remains visible when collapsed -- this contradicts a bare `0%` and must be handled explicitly. This is a low-risk change that immediately makes the layout more flexible.

Store panel refs (`useRef<ImperativePanelHandle>`) so components can call `collapse()`/`expand()` imperatively.

### 2. Receiver identity in connection bar
Remove the Receiver panel from the resizable layout. Instead, display a compact receiver identity string in the connection bar when connected:
- Format: `"ZED-F9P (FW 2.01 PROTVER 29.0)"` -- hardware + firmware, no vendor prefix
- Shows "Identifying..." during probe, nothing when disconnected
- Remove `supportedGNSS` and `packetFormats` from the receiver display

Pass `supportedGNSS` to the config panel so it can grey out unsupported constellations in the signal picker.

Remove `receiver-panel.tsx` or repurpose it as a simple inline component.

Update `PanelVisibility` and `panelLabels`:
- Remove `receiver` and `config` from the `PanelVisibility` interface (in both `app.tsx` and `connection-panel.tsx`).
- Remove `panels.config` from the initial `useState` call -- config is no longer a panel.
- Remove the `{id: 'receiver', label: 'Receiver'}` and `{id: 'config', label: 'Configure'}` entries from `panelLabels`.
- "Configure" is removed from the Panels menu -- it is accessed via the button in the connection bar (step 6). The Panels menu controls visibility of telemetry panels only.
- Remove the `showLeft` computed variable and the left-column `Panel` that currently wraps Receiver+Config.

### 3. Survey panel starts collapsed, auto-expands
Survey panel starts collapsed (`collapsedSize="0%"`). When the first `SurveyMsg` arrives with data, call `surveyPanelRef.current.expand()` to open it. The user can still manually collapse/expand it after that.

The separator between Time and Survey is always present so the user can drag it open before data arrives if desired.

### 4. Packet monitor: collapsed by default, add freeze
Change the packet monitor's initial state to collapsed.

Add a Freeze/Resume toggle button to the packet monitor toolbar:
- When frozen, new packets are still received and buffered but the display stops updating and auto-scroll is disabled.
- The freeze button shows a visual indicator (e.g. changes colour, shows "Frozen" label).
- Clicking Resume scrolls to the latest packet and resumes live updates.

### 5. Activity log: starts small
Change the activity log's default size from 25% to a smaller value (10-15%), or start collapsed. The toolbar (level filter, search, clear) remains visible when collapsed at a header-only size so users can see it exists and expand it.

Note: this requires the activity log `Panel` to use a non-zero `collapsedSize` (enough for the toolbar height, ~36px). The percentage depends on the parent Group's height -- calculate as `(36 / expectedHeight) * 100` or use a fixed pixel value if the library supports it. This is distinct from step 1's `collapsedSize="0%"` used for panels without headers.

### 6. Configuration as slide-down overlay
Extract the config panel from the resizable panel layout. It becomes a slide-down overlay triggered by a "Configure" button in the connection bar.

**State**: Add `const [configOpen, setConfigOpen] = useState(false)` to `app.tsx`. This replaces the old `panels.config` boolean. The overlay renders the `ConfigPanel` component unconditionally (so it keeps state across open/close) but uses CSS to show/hide.

**Trigger**: A "Configure" button in the connection bar (next to Connect/Disconnect). Only enabled when connected. Clicking toggles `configOpen`.

**DOM structure**: The current layout has a single vertical `PanelGroup` containing all panels. After the rework, the structure is:

```
<div class="flex flex-col h-screen">
  <!-- connection bar -->
  <ConnectionPanel ... />

  <!-- telemetry wrapper: position: relative so overlay can cover it -->
  <div class="relative flex-1 overflow-hidden">
    <!-- telemetry panels (always mounted) -->
    <PanelGroup direction="vertical">
      <Panel> <!-- telemetry area: horizontal split -->
        <PanelGroup direction="horizontal">
          <Panel> <!-- left: Time/Survey vertical split -->
            <PanelGroup direction="vertical">
              <Panel id="time">...</Panel>
              <PanelResizeHandle />
              <Panel id="survey" collapsible>...</Panel>
            </PanelGroup>
          </Panel>
          <PanelResizeHandle />
          <Panel id="monitor" collapsible>...</Panel>
        </PanelGroup>
      </Panel>
    </PanelGroup>

    <!-- config overlay: sibling of the PanelGroup, NOT inside it -->
    <div class="absolute inset-0 transition-transform duration-300 bg-zinc-900"
         style={{ transform: configOpen ? 'translateY(0)' : 'translateY(-100%)' }}>
      <ConfigPanel ... />
    </div>
  </div>

  <!-- activity log: OUTSIDE the telemetry wrapper, so overlay doesn't cover it -->
  <PanelGroup direction="vertical">
    <Panel id="logging" collapsible collapsedSize={...}>
      <ActivityLog ... />
    </Panel>
  </PanelGroup>

  <!-- status bar -->
  <StatusBar ... />
</div>
```

Key points:
- The telemetry wrapper `div` has `position: relative` and `overflow: hidden`. This is the positioning parent for the overlay.
- The overlay `div` is a sibling of the telemetry `PanelGroup`, not a child of any `Panel`. It uses `position: absolute; inset: 0` to cover exactly the telemetry area.
- The activity log `PanelGroup` is outside the telemetry wrapper, so it remains visible below the overlay.
- The `overflow: hidden` on the wrapper clips the overlay when it slides up (`translateY(-100%)`).
- The config panel is no longer inside any `Panel` or `PanelGroup` -- it is a plain `div` with its own scrolling.

**Close**: A "Close" or "Done" button within the config overlay, or the same "Configure" button in the connection bar acting as a toggle.

**Config readback**: Triggered when `configOpen` transitions to `true` (as already implemented in 5a).

**Activity log visibility**: The activity log is outside the telemetry wrapper in the DOM, so it remains visible and shows messages during Apply operations.

### 7. Supported GNSS feeds into constellation picker
Pass the `supportedGNSS` array from receiver identification into the config panel and through to the signal picker. The data flow is:

1. `ReceiverState.supportedGNSS` (already populated by auto-identify in phase 5a) is available in `app.tsx` as `receiver.supportedGNSS`.
2. `app.tsx` passes `supportedGNSS` as a new prop to `ConfigPanel`.
3. `ConfigPanel` passes `supportedGNSS` through to `SignalPicker` as a new prop.
4. `SignalPicker` receives `supportedGNSS: string[]` alongside its existing `signalCatalog` and `selectedSignals` props. For each constellation in `signalCatalog`, if its GNSS ID is not in `supportedGNSS`, the constellation's checkbox and all its signals are rendered disabled with `opacity-50` and a title attribute "Not supported by this receiver". Disabled constellations cannot be toggled.

### 8. Rework telemetry layout
With Receiver and Configuration removed from the panel layout, the telemetry area becomes:

```
┌──────────────────────────────────────────────┐
│  SatPulse GPS  [Panels] [Configure]          │
│  ZED-F9P FW 2.01    Device... Speed... [Conn]│
├──────────────────────────────────────────────┤
│                                              │
│   Time            │  Packet monitor          │
│                   │  (collapsed by default)  │
│───────────────────│                          │
│   Survey          │                          │
│   (collapsed,     │                          │
│    auto-expands)  │                          │
│                                              │
├──────────────────────────────────────────────┤
│  Activity log (small by default)             │
├──────────────────────────────────────────────┤
│  Status bar                                  │
└──────────────────────────────────────────────┘
```

Two columns: Time/Survey on the left, Packet monitor on the right. Both columns use `react-resizable-panels` with collapsible support. Future panels (Sky view, Signal bars) will be added to this layout in later phases.

## Result
The UI has a clean monitoring view with Time, Survey, and (collapsed) packet monitor. Configuration opens as a slide-down overlay with full width. The activity log is always visible for watching operation progress. All panels can be collapsed by dragging separators. The receiver identity is a compact line in the connection bar.

## Files changed
- `desktop/frontend/src/app.tsx` (major layout restructure, overlay logic, panel refs)
- `desktop/frontend/src/connection-panel.tsx` (receiver identity line, Configure button)
- `desktop/frontend/src/receiver-panel.tsx` (removed or reduced to inline component)
- `desktop/frontend/src/monitor-panel.tsx` (freeze button)
- `desktop/frontend/src/config-panel.tsx` (minor -- receives supportedGNSS prop)
- `desktop/frontend/src/signal-picker.tsx` (grey out unsupported constellations)

## Testing -- Playwright

### Collapsible panels
- Verify all panels can be collapsed by resizing.
- Verify collapsed panels can be expanded by dragging.

### Receiver identity
- Connect to a receiver; verify identity string appears in connection bar.
- Verify the old receiver panel is gone.
- Disconnect; verify identity string disappears.

### Survey auto-expand
- Verify survey panel starts collapsed.
- Simulate a survey message; verify the panel expands automatically.

### Packet monitor
- Verify packet monitor starts collapsed.
- Expand it; verify packets are displayed.
- Click Freeze; verify display stops updating.
- Click Resume; verify display resumes and scrolls to latest.

### Configuration overlay
- Verify Configure button appears in connection bar when connected.
- Click Configure; verify overlay slides down covering telemetry.
- Verify activity log remains visible below the overlay.
- Click Close; verify overlay slides up and telemetry is visible again.
- Verify telemetry continued updating while overlay was open.

### Supported GNSS
- Connect to a receiver that does not support all constellations.
- Open configuration; verify unsupported constellations are greyed out.
