# Shared web UI package

Create `@satpulse/webui`, a standalone Preact component library. The desktop GUI consumes it immediately; the web dashboard ([web-toolchain.md](../../plan/web-toolchain.md)) consumes it later. Components are built against gpsprot types, so once [gpsprot-sse.md](../../plan/gpsprot-sse.md) eliminates the SSE intermediary layer, the dashboard can plug in directly.

Depends on [semantic-tokens.md](semantic-tokens.md) -- shared components must use semantic tokens only.

## Package setup

Create `webui/` at the repo root. Source-only package (no build step); Vite in each consumer compiles the TypeScript directly.

```
webui/
  package.json              # @satpulse/webui
  src/
    tokens.css              # @theme block with token names (no values)
    types.ts                # TypeScript interfaces matching gpsprot JSON
    timefmt.ts              # time formatting utilities
    components/
      Badge.tsx
      Button.tsx
      Card.tsx
      Input.tsx
      Table.tsx
    viz/
      sky-view.tsx          # polar satellite plot (pure SVG)
      signal-graph.tsx      # CN0 bar graph (pure SVG)
      helpers.ts            # colorClassFor, opacityClassFor, simplifySignals
```

Desktop frontend adds a `file:` dependency in `desktop/frontend/package.json`:

```json
"dependencies": {
  "@satpulse/webui": "file:../../webui"
}
```

## Token extraction

The `@theme` block (token names) moves from desktop's `style.css` to `webui/src/tokens.css`. Desktop's `style.css` imports it and provides `:root` values:

```css
@import "@satpulse/webui/src/tokens.css";
@import "tailwindcss";

:root { /* desktop light mode values */ }
@media (prefers-color-scheme: dark) {
  :root { /* desktop dark mode values */ }
}
```

The web dashboard later provides its own `:root` values.

## Components to extract

### Types (`types.ts`)

TypeScript interfaces matching gpsprot JSON serialisation:

- `SVInfo`, `LookAngles`, `SignalInfo` (from `SatellitesMsg`)
- `SatellitesMsg`, `TimeMsg`, `SurveyMsg`

Source: currently defined manually in `desktop/frontend/src/app.tsx`.

### Time formatting (`timefmt.ts`)

`formatUTCLocal`, `formatTAI`, `formatDateTime`, `parseTAITime`, `taiToUTC`, and the `DateTime` type. The desktop version is a superset of `web/timefmt.ts`.

### Component primitives

Badge, Button, Card/Section, Input, Table family. Move from `desktop/frontend/src/components/` to `webui/src/components/`.

### Viz components

Sky view and signal graph. Render as pure SVG with `viewBox`/`preserveAspectRatio` -- they fill their container, no hardcoded pixel sizes. Satellite helpers (`simplifySignals`, `colorClassFor`, `opacityClassFor`) use constellation token classes.

## Ongoing: new shared components

New components are built in `@satpulse/webui` as part of desktop GUI work:

- **Signal graph** (signal-strength.md) -- build directly in shared
- **Sky view** (sky-view.md) -- build in shared, desktop wraps in panel
- **Map** (map.md) -- rework to be resizable: dynamic tile grid (`Math.ceil(dimension / 256) + 1` tiles per axis), fill container, click behaviour via callback prop
- **Clock** -- SVG rewrite: seven-segment paths, fixed viewBox scaling, no DSEG7 font dependency
- **Navigation summary** (nav-summary.md) -- shared component consuming `NavEpochMsg`

## What stays in the desktop frontend

Everything Wails-coupled or desktop-specific:

- `app.tsx` -- Wails event wiring, state management
- `connection-panel.tsx` -- serial port selection, connect/disconnect
- `config-panel.tsx` -- receiver configuration (Go bindings)
- `msgfile-panel.tsx` -- message file loading/sending (Go bindings)
- `survey-panel.tsx` -- uses `ECEFtoLLH` Go binding
- `monitor-panel.tsx`, `logging-panel.tsx` -- desktop-specific
- `collapsible-section.tsx`, `three-way-selector.tsx` -- desktop UI widgets

## Verify

- `npm install` in `desktop/frontend/` resolves `@satpulse/webui` via `file:` link
- `wails dev` builds and runs with shared component imports
- Tailwind scans shared package source and generates correct utility CSS
- Desktop GUI looks and works identically after extraction
- Shared viz components resize correctly in different containers
