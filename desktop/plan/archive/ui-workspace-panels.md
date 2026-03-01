# Desktop UI workspace overview

## Purpose
Define the top-level UI model for the desktop app.

This document is intentionally high-level and panel-oriented.
Detailed behavior for each panel lives in separate panel UI documents.

## Design

### Primary UI model
Use a single-window workspace with resizable tiled panels.

Design goals:
- simultaneous visibility of key operational views,
- less clutter than classic free-floating multi-window tools,
- fast panel show/hide.

### Panel set
The workspace is composed of these panels:
1. Connection Panel
2. Sky View Panel
3. Signal View Panel
4. Configuration Panel
5. Packet Monitor Panel
6. Logging Panel

See per-panel specs:
- `desktop/plan/ui-panel-connection.md`
- `desktop/plan/ui-panel-sky-view.md`
- `desktop/plan/ui-panel-signal-view.md`
- `desktop/plan/ui-panel-configuration.md`
- `desktop/plan/ui-panel-packet-monitor.md`
- `desktop/plan/ui-panel-logging.md`

### Panel visibility
- every panel can be shown/hidden via a `Panels` menu.
- hidden panels continue receiving state updates unless explicitly paused.

### Layout and resizing
- panels are tiled in nested horizontal/vertical splits.
- split dividers are draggable to resize.
- no overlapping or floating windows.

### Default layout
- Top strip: Connection Panel (full width, not resizable vertically)
- Left column: Sky View (top), Signal View (bottom)
- Right column: Configuration Panel
- Bottom strip: Logging Panel
- Packet Monitor hidden by default

### Collapsible sections inside panels
Large panels (especially Configuration, Logging) must use internal collapsible sections to reduce cognitive load.

### Progress visibility requirement
Configuration can take around 15 seconds.
During operations:
- Logging Panel must remain accessible,
- progress state should be visible without changing workspace context,
- user should not need to switch tabs to confirm activity.

### Data and event principle
UI panels consume semantic backend events and decide presentation in frontend.

Cross-reference:
- backend/frontend event architecture is defined in `desktop/plan/backend.md`.

## Implementation

### Layout library
Use `react-resizable-panels` (npm: `react-resizable-panels`) for the panel layout.

This library provides:
- `PanelGroup` (horizontal or vertical split container),
- `Panel` (resizable child),
- `PanelResizeHandle` (draggable divider).

Nested `PanelGroup` elements create the grid. The Connection strip and Logging strip are fixed-direction horizontal panels; the middle area splits vertically into left/right columns, each of which splits horizontally for sub-panels.

Works with Preact via `preact/compat`. No built-in styling; resize handles are styled with Tailwind.

### Panel show/hide
Each panel's visibility is controlled by a boolean in app state. When hidden, the `Panel` is removed from its `PanelGroup` and the remaining panels fill the space. Panel components stay mounted (via CSS `display:none` or conditional wrapper) so they keep receiving events.

### State management
Panel content state (log entries, packet entries, satellite data, config form state) lives in the top-level `App` component or a shared context, not inside individual panel components. This ensures hidden panels don't lose state.

## Non-goals
- Floating or popout windows.
- Drag-to-reorder panels.
- Layout presets or saved custom layouts (defer to later).
- Defining backend message schemas in this document.
