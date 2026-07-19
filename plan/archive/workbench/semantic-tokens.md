# Semantic token migration

Migrate the entire desktop frontend from hardcoded Tailwind colour classes to semantic design tokens. This is a prerequisite for [shared-webui.md](shared-webui.md) -- shared components must use only semantic tokens so that each consumer app can provide different visual treatment.

The `desktop-gui-ag` branch has exploratory work on tokens and component primitives. Use it as reference (not merge/cherry-pick) -- the token system and component primitive APIs are a good starting point, but the migration was partial and not reviewed.

## Token system

Two-layer CSS custom properties via Tailwind 4 `@theme`. The `@theme` block registers token names that Tailwind recognises as utility values (e.g. `bg-surface-1`, `text-text-primary`, `border-border-subtle`). The `:root` block provides concrete colour values for the desktop app.

These tokens are designed to work for both the desktop GUI and the web dashboard. The web dashboard's shared components (sky view, signal graph, property cards) map cleanly onto the same semantic names -- `surface-2` for card backgrounds, `text-primary`/`text-secondary` for data/label hierarchy, `border-subtle`/`border-strong` for SVG grid lines, `gnss-*` for constellation colours. Dashboard-specific styling (borders, shadows, layout) stays in dashboard code, not in shared components.

**Colour tokens:**

| Token | Light | Dark | Usage |
|-------|-------|------|-------|
| `surface-1` | zinc-50 | zinc-950 | App background |
| `surface-2` | white | zinc-900 | Panels, cards, inputs |
| `surface-3` | zinc-100 | zinc-800 | Hover states, alternating rows |
| `border-subtle` | zinc-200 | zinc-700 | Dividers, structural lines |
| `border-strong` | zinc-400 | zinc-500 | Input borders, focused states |
| `text-primary` | zinc-900 | zinc-100 | Body text, data values |
| `text-secondary` | zinc-600 | zinc-400 | Labels, section headers, table headers |
| `text-muted` | zinc-400 | zinc-600 | Disabled, placeholder |
| `accent` | blue-600 | blue-600 | Active tabs, primary buttons, checkbox/radio tint, focus rings |
| `accent-hover` | blue-700 | blue-500 | Hover state for accent |
| `control` | zinc-200 | zinc-700 | Secondary button backgrounds, interactive controls |
| `control-hover` | zinc-300 | zinc-600 | Hover state for controls |
| `success` | emerald-500 | emerald-500 | Connected, valid |
| `warning` | amber-500 | amber-500 | Pending states |
| `danger` | rose-500 | rose-500 | Errors, destructive actions |
| `info` | blue-500 | blue-400 | Informational status text |

**Constellation colours** (SVG fills for satellite viz):

| Token | Colour | GNSS |
|-------|--------|------|
| `gnss-gps` | blue | GPS, SBAS |
| `gnss-galileo` | green | Galileo |
| `gnss-beidou` | red | BeiDou |
| `gnss-glonass` | fuchsia | GLONASS |
| `gnss-qzss` | amber | QZSS |
| `gnss-navic` | yellow | NavIC/IRNSS |
| `gnss-unknown` | gray | Unknown |

## Component primitives

Based on `desktop-gui-ag` component APIs. Replace repeated class patterns across the desktop frontend:

- **Button** -- `primary`, `secondary`, `danger`, `ghost` variants; `sm`, `md` sizes. Primary uses `accent`/`accent-hover`. Secondary uses `control`/`control-hover` (subtle darkening on hover, never changes colour). Danger uses `danger` colours.
- **Input** -- styled form input
- **Card** -- surface container with border
- **Badge** -- status indicators: `default`, `success`, `warning`, `error`, `info`
- **PVTMsgTable** -- titled telemetry data table used in the PVT panel. Combines a section heading, muted sans-serif column headers, and a mono data body into one component. Handles the empty state ("No position data"). Used three times for Position, Velocity, Time.
- The message file table uses raw HTML `<table>` elements directly -- it is a different kind of table (sans-serif data, secondary headers, clickable rows, sticky header) with no overlap with `PVTMsgTable`. No generic `Table` component family -- each table context owns its own styling.

## Migration scope

Every `.tsx` file in `desktop/frontend/src/` needs converting:

This is not a mechanical find-and-replace of colour values. Each element must be assigned the token that matches its semantic role, not just its current shade. The configuration panel ([ui-panel-configuration.md](ui-panel-configuration.md)) is the most complex case -- it has a deep hierarchy of top-level sections, collapsible subgroups, mode-specific fields, three-way selectors gating child controls, and group boxes with configure checkboxes. For example:

- Section subheadings ("Survey-in", "Save", "Reset") use `text-gray-600 dark:text-gray-300` -- these are `text-secondary` (structural labels)
- Field labels ("Period (s)", "Survey time (s)") use `text-gray-500 dark:text-gray-400` -- these are `text-secondary`
- Table column headers ("Tag", "Message") use `text-gray-500 dark:text-gray-400` -- these are `text-secondary` (they are labels, same as any other). Visual distinction from data comes from the `Th` component (font weight, borders, background), not from a different colour token.
- Enabled interactive labels (checkbox/radio text) use `text-gray-700 dark:text-gray-200` -- these are `text-primary` (interactive, not muted)
- Disabled interactive labels use `text-gray-400 dark:text-gray-500` -- these are `text-muted`
- Status info ("Receiver is in stationary mode") uses `text-blue-500` -- this is `info` (not `accent` -- it is informational, not interactive)

Two elements that happen to use the same gray today might map to different tokens based on what they are. Conversely, elements using different grays might map to the same token if they serve the same semantic purpose.

Specific changes:

- Replace `bg-gray-*`, `dark:bg-gray-*` with `bg-surface-*`
- Replace `text-gray-*`, `dark:text-gray-*` with `text-text-*` based on semantic role
- Replace `border-gray-*`, `dark:border-gray-*` with `border-border-*`
- Replace `fill-blue-600 dark:fill-blue-400` etc. with `fill-gnss-gps` etc.
- Replace repeated button/input/table class strings with component primitives
- Remove all `dark:` prefixes from colour classes (dark mode handled by `:root` values)

## Design guidelines

From `desktop-gui-ag` design-system.md (refined):

- **No hardcoded colours** in components -- semantic tokens only
- **No arbitrary pixel values** for sizing -- use Tailwind spacing scale
- **Flat UI** -- no shadows except floating elements (dialogs, popovers)
- **Font usage** -- sans-serif for UI; monospace (`font-mono`) only for raw telemetry data
- **Dense layout** -- `text-xs` dominant for data, `text-sm` for controls

## After migration: aesthetic tuning

With semantic tokens in place, the overall look and feel can be improved by adjusting `:root` values in one place rather than hunting through component files. This is a separate step from the migration itself -- the migration is mechanical (swap classes, enforce consistency), the tuning is visual (do these shades work well together?).

## Verify

- No hardcoded `bg-gray-*`, `text-gray-*`, `border-gray-*`, `dark:` colour classes remain in components
- Dark mode works correctly via `:root` token values
- Component primitives used consistently (no raw button/input/table class strings)
- Surfaces, text, and borders at the same semantic level are consistent everywhere

## Resolved: role alias tokens and component abstractions

The base semantic tokens (`text-primary`, `text-secondary`, `text-muted`) describe visual emphasis level -- prominent, moderate, subdued. They are a named palette of visual weight, and they work well for that purpose.

Some elements have a clear semantic role where the visual treatment is a design decision that could legitimately go multiple ways. For example, a heading could be made prominent (bold, darker) or subdued (lighter, uppercase) -- two valid visual approaches to the same semantic role. A **role alias token** captures this: it is defined in terms of a base semantic token, giving the role an independent tuning point.

Criteria for when a role alias token is warranted:

1. **Clear semantic role** -- the element is a distinct kind of thing, not arbitrary text
2. **Multiple plausible visual strategies** -- you could legitimately take different approaches to rendering that role (different colour, weight, size, style)
3. **Not already encapsulated in a component** -- if a component (`PVTMsgTable`, `Badge`, etc.) already captures the semantic role, the component is the abstraction and handles its own styling with base tokens internally

Most cases that looked like they needed role alias tokens are better served by components (see follow-ons below). One role alias token is needed now:

| Role alias | Defined as | Usage |
|------------|-----------|-------|
| `text-placeholder` | `text-muted` | Empty state messages ("Waiting for time", "No position data", "No ports found") -- distinct from disabled text, could be rendered as lighter, italic, smaller, with icon, etc. |

Role alias tokens are defined in CSS as references to base tokens (e.g. `--color-text-placeholder: var(--color-text-muted)`). They can share the same concrete value today but are independently adjustable because they mean different things.

## Follow-ons

### New component primitives

- **`ConfigSubGroup`** -- config panel sub-groups: a title ("NMEA", "Survey-in", "Save") with child controls and a `disabled` prop that flows to both heading and children. Currently ~10 instances of bare `<div>` with `text-xs font-semibold text-text-secondary` heading + indented children. The heading's enabled/disabled styling becomes component-internal.

### Rename

- **`MonitorPanel`** -> **`PacketPanel`** -- the component name should match its tab label ("Packets").
