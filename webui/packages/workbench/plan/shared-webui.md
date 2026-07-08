# Shared web UI package

Create `@satpulse/webui`, a standalone Preact component library. The desktop GUI consumes it immediately; the web dashboard ([web-toolchain.md](../../plan/web-toolchain.md)) consumes it later. Components are built against gpsprot types, so once [gpsprot-sse.md](../../plan/gpsprot-sse.md) eliminates the SSE intermediary layer, the dashboard can plug in directly.

Depends on [semantic-tokens.md](semantic-tokens.md) -- shared components must use semantic tokens only.

## Package setup

Create `webui/` at the repo root. Source-only package (no build step); Vite in each consumer compiles the TypeScript directly.

```
webui/
  package.json              # @satpulse/webui
  tokens.css                # @theme block with token names (no values)
  timefmt.ts                # time formatting utilities
  components/
    cx.ts                   # class name combiner
    Button.tsx
    Input.tsx
    Select.tsx
    Card.tsx
    Badge.tsx
    DataView.tsx            # MonitorDataView (key-value definition list)
    sat-helpers.ts          # colorClassFor, opacityClassFor, simplifySignals
    SkyView.tsx             # polar satellite plot (pure SVG)
    SignalGraph.tsx          # CN0 bar graph (pure SVG)
```

Desktop frontend adds a `file:` dependency in `desktop/frontend/package.json`:

```json
"dependencies": {
  "@satpulse/webui": "file:../../webui"
}
```

## Token extraction

The `@theme` block (token names) moves from desktop's `style.css` to `webui/tokens.css`. Desktop's `style.css` imports it and provides `:root` values:

```css
@import "@satpulse/webui/tokens.css";
@import "tailwindcss";

:root { /* desktop light mode values */ }
@media (prefers-color-scheme: dark) {
  :root { /* desktop dark mode values */ }
}
```

The web dashboard later provides its own `:root` values.

## Components to extract

### Types

No separate `types.ts` needed. TypeScript interfaces for gpsprot messages (`SVInfo`, `LookAngles`, `SignalInfo`, `SatellitesMsg`, `TimeMsg`, `SurveyMsg`, etc.) live in `gps/ts/gpsprot.ts` and are published as the `@satpulse/gps` package. Both the desktop frontend and shared components import directly from `@satpulse/gps/gpsprot`.

### Time formatting (`timefmt.ts`)

`formatUTCLocal`, `formatTAI`, `formatDateTime`, `parseTAITime`, `taiToUTC`, and the `DateTime` type. The desktop version is a superset of `web/timefmt.ts`.

### Component primitives

`cx`, `Button`, `Input`, `Select`, `Card`, `Badge`, `MonitorDataView`, and helper functions (`labeledControlText`, `fieldLabelText`). Currently in `desktop/frontend/src/ui.tsx`. The `Config*` components (`ConfigGroup`, `ConfigSubGroup`, `ConfigSubSubGroup`) stay in the desktop frontend since they're config-tab specific.

### Viz components

Sky view and signal graph. Render as pure SVG with `viewBox`/`preserveAspectRatio` -- they fill their container, no hardcoded pixel sizes. Satellite helpers (`simplifySignals`, `colorClassFor`, `opacityClassFor`) use constellation token classes.

## Ongoing: new shared components

New components are built in `@satpulse/webui` as part of desktop GUI work:

- **Signal graph** (signal-strength.md) -- build directly in shared
- **Sky view** (sky-view.md) -- build in shared, desktop wraps in panel
- **Map** (map.md) -- rework to be resizable: dynamic tile grid (`Math.ceil(dimension / 256) + 1` tiles per axis), fill container, click behaviour via callback prop
- **Clock** -- SVG rewrite: seven-segment paths, fixed viewBox scaling, no DSEG7 font dependency
- **Navigation summary** (nav-summary.md) -- shared component consuming `NavEpochMsg`
- **Survey panel** -- requires adding LLH fields to `gpsprot.SurveyMsg` (backend computes ECEF-to-LLH), removing the `ECEFtoLLH` Go binding dependency so the component is reusable

## What stays in the desktop frontend

Everything Wails-coupled or desktop-specific:

- `app.tsx` -- Wails event wiring, state management
- `connection-panel.tsx` -- serial port selection, connect/disconnect
- `config-panel.tsx` -- receiver configuration (Go bindings)
- `msgfile-panel.tsx` -- message file loading/sending (Go bindings)
- `monitor-panel.tsx`, `logging-panel.tsx` -- desktop-specific
- `collapsible-section.tsx`, `three-way-selector.tsx` -- desktop UI widgets

## Verify

- `npm install` in `desktop/frontend/` resolves `@satpulse/webui` via `file:` link
- `wails dev` builds and runs with shared component imports
- Tailwind scans shared package source and generates correct utility CSS
- Desktop GUI looks and works identically after extraction
- Shared viz components resize correctly in different containers
