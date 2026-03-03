# Web redesign and shared components

Introduce a design token system and build shared Preact components that both the web dashboard and desktop GUI can consume. Builds on the `webui/packages/shared/` package from [web-toolchain.md](web-toolchain.md). Uses the replay dev server from [gpsprot-sse.md](gpsprot-sse.md) for rapid visual iteration.

## Introduce semantic design tokens

Add a token system to the shared package and migrate the web dashboard to use it. The dev server enables rapid visual iteration for this work.

**Step 1: audit the web dashboard.** Document inconsistent spacing, font sizes, arbitrary values, repeated class patterns, and missing hierarchy in the current dashboard code. This gives a concrete checklist for the migration.

**Step 2: define the design system.** Minimal, for the web dashboard first. The desktop GUI becomes a consumer later with potentially different visual treatment (denser, different colour values, different spacing). The token and component architecture must support this: components use semantic tokens only, never hardcoded values, so a different app can provide different `:root` values and get a different look from the same components.

*Colour tokens* -- semantic CSS custom properties registered via Tailwind v4 `@theme`:
- Surfaces: `--color-surface-1`, `--color-surface-2`, `--color-surface-3`
- Borders: `--color-border-subtle`, `--color-border-strong`
- Text: `--color-text-primary`, `--color-text-secondary`, `--color-text-muted`
- Accents: `--color-brand`, `--color-success`, `--color-warning`, `--color-danger`

*Typography scale:*
- Font families: sans-serif (Inter or system) for UI, monospace (JetBrains Mono or system) for telemetry data
- Size scale with clear hierarchy (headings, body, caption/label)
- Weight usage rules

*Spacing scale:*
- Base unit and consistent scale
- Rules for section spacing vs component spacing vs inline spacing

*Shape and depth:*
- Border radius scale
- Shadow scale (if any -- the dashboard may be flat)
- Border usage rules

The shared package provides `tokens.css` with `@theme` directives defining the token names and scales. The dashboard provides `:root` values and `@media (prefers-color-scheme: dark)` overrides.

**Step 3: component primitives.** Built for the web dashboard, placed in the shared package:
- **Card** -- surface container with border
- **Badge** -- status indicators using semantic tokens
- **Table** family -- `Table`, `TableHead`, `TableBody`, `TableRow`, `Th`, `Td` with automatic monospace for telemetry data
- **Button** -- primary, secondary, danger, ghost variants
- **Input** -- styled form inputs

All primitives use only semantic tokens, never raw colour values or arbitrary Tailwind classes.

**Step 4: migrate the dashboard.** Replace hardcoded colours (`bg-white dark:bg-gray-800`, `border-l-4 border-orange-500`, etc.), inconsistent spacing, and repeated class patterns with tokens and component primitives. Use the audit checklist from step 1.

**Verify:** web dashboard looks the same (or better) after this step.

## Dashboard design vision

Before building individual components, establish a cohesive design for the web dashboard as a whole. The current dashboard is primitive -- a flat grid of cards, each mapped 1:1 to an SSE event, with minimal visual hierarchy and no considered information architecture.

This step produces a concrete design (mockups or detailed description) covering:
- **Information hierarchy**: what does the user need to see first? Fix status and position are primary; raw telemetry is secondary. The current flat grid treats everything equally.
- **Layout**: how do the components relate spatially? The sky view and signal graph are complementary (both about satellites) and should be adjacent. The map and position data are complementary. Quality/status is glanceable and should be persistent. How does this adapt from phone to desktop?
- **Visual language**: beyond tokens (which handle colours/typography), what is the overall feel? Density, whitespace, card vs borderless, how much chrome. The desktop GUI's direction (compact, data-dense, flat) may or may not be right for the web dashboard -- the web version might need to breathe more, especially on mobile.
- **Interaction model**: what is passive (just watching data flow) vs interactive (clicking, hovering, expanding)? The sky view cursor lens and cross-highlighting need to fit naturally.

The replay dev server is essential for this step -- it lets us iterate on the design rapidly without hardware. This vision then guides all subsequent component work. Each component is built to fit the whole, not designed in isolation.

## Solution quality status bar

A compact status bar component displaying `NavEpochMsg` quality metadata.

**Badges:** synthesized from the combination of FixLevel, FixDim, CorrKind, and AuxSrc into user-meaningful labels (e.g. "RTK Fixed", "3D", "SBAS"). Not a 1:1 mapping from any single field. The specific badge set, labels, and visual treatment are designed during the design vision step with the dev server and real data.

**Numeric telemetry:** horizontal accuracy, vertical accuracy, HDOP/PDOP, differential age, satellite count (used/tracked). All fields optional, monospace.

## Map component

Rework the desktop GUI's map panel into a shared component with flexible sizing.

The current implementation (`satpulse-desktop/desktop/frontend/src/map-panel.tsx`) is hardcoded to 320x256px with a 2x2 tile grid at zoom 16. The shared version:
- Accepts width/height as props or fills its container
- Computes tile grid dynamically: `Math.ceil(dimension / 256) + 1` tiles per axis
- Fixed zoom level (prop with default 16), no pan/zoom interaction for now
- Same anchor/re-centre logic using actual viewport dimensions instead of constants
- Position dot/arrow, no-fix overlay carried over from current design
- Consumes `PosGeoMsg` (lat/lon) and `VelGeoMsg` (course, ground speed) from epoch data
- Click behaviour via callback prop (Wails overrides with `BrowserOpenURL`, web default uses `window.open`)

## Clock component (SVG rewrite)

Rewrite the LED clock as pure SVG so it scales to any size:
- Seven-segment digit geometry as SVG paths
- Active segments use `text-primary`/`success` tokens, ghost segments use `border-subtle`/`border-strong` tokens
- Fixed viewBox (e.g. `0 0 320 256`), scales via `preserveAspectRatio`
- No dependency on the DSEG7 web font
- Same layout: date top, HH:MM large centre, UTC offset and SS bottom
- Consumes `TimeMsg` from epoch data, handles TAI-to-local conversion via shared `timefmt`

## Sky view rewrite

Replace label-based sky view with compact dot-based design:
- Coloured dots by constellation, opacity by CN0, filled/hollow for used/unused
- Works at 300-320px (fits alongside clock and map)

**Cursor lens:** circular magnified region follows cursor, ~80px diameter, showing enlarged dots with SVID labels. Reveals detail in tight clusters. Implemented as a second SVG `<g>` with `<clipPath>`, rendering same satellites at higher zoom translated to cursor position.

**Cross-highlighting with signal bar graph:** hover a dot to highlight the corresponding bar (and vice versa). Bidirectional. Connects spatial view with signal strength view.

## Signal bar graph

Move existing `SignalGraph` from current code into shared package. Bars labelled by SVID, coloured by constellation, height by CN0. Participates in cross-highlighting with sky view.
