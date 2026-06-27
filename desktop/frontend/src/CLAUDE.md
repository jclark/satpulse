# Frontend code style

## Semantic design tokens

All colour in this frontend is expressed through semantic tokens defined in
`style.css`. Never use hardcoded Tailwind colour classes (`bg-gray-*`,
`text-blue-500`, `border-zinc-*`, `dark:*`, etc.). Always use the
corresponding semantic token utility class.

Dark mode is handled entirely by `:root` overrides in `style.css`. There
must be no `dark:` prefixed colour classes anywhere in component code.

### Available tokens

Use ONLY these tokens for colour. Do not invent new tokens without adding
them to `style.css` first.

**Surfaces** (backgrounds):
- `surface-1` -- app background
- `surface-2` -- panels, cards, inputs, elevated surfaces
- `surface-3` -- hover states, alternating rows, tertiary backgrounds

**Borders**:
- `border-subtle` -- dividers, structural lines, quiet borders
- `border-strong` -- input borders, focused states, emphasis

**Text**:
- `text-primary` -- body text, data values, enabled interactive labels
- `text-secondary` -- labels, section headers, table column headers, field labels
- `text-muted` -- disabled text, placeholders, de-emphasised content

**Interactive**:
- `accent` -- active tabs, primary buttons, focus rings, checkbox/radio tint
- `accent-hover` -- hover state for accent elements
- `control` -- secondary button backgrounds, interactive controls
- `control-hover` -- hover state for controls

**Status**:
- `success` -- connected, valid, positive states
- `warning` -- pending, caution states
- `danger` -- errors, destructive actions
- `info` -- informational status text (not interactive -- use `accent` for interactive blue)

**GNSS constellations** (SVG fills for satellite visualisation):
- `gnss-gps` -- GPS, SBAS
- `gnss-galileo` -- Galileo
- `gnss-beidou` -- BeiDou
- `gnss-glonass` -- GLONASS
- `gnss-qzss` -- QZSS
- `gnss-navic` -- NavIC/IRNSS
- `gnss-unknown` -- unknown constellation

**Correction protocols** (RTCM/SPARTN badge fills):
- `rtcm` -- RTCM correction messages (blue)
- `spartn` -- SPARTN correction messages (red)

**Clock display**:
- `clock-digit` -- active clock digit colour
- `clock-ghost` -- faded/ghost digit colour

### Usage in Tailwind classes

Tokens map to Tailwind utilities via `@theme`:
- Backgrounds: `bg-surface-1`, `bg-surface-2`, `bg-accent`, `bg-danger`, etc.
- Text: `text-text-primary`, `text-text-secondary`, `text-text-muted`, `text-warning`, etc.
- Borders: `border-border-subtle`, `border-border-strong`, `border-danger`, etc.
- Fills: `fill-gnss-gps`, `fill-gnss-galileo`, etc.
- Rings: `ring-accent`, `ring-danger`, etc.

### Choosing the right token

Assign tokens by semantic role, not by visual similarity. Two elements that
happen to look the same shade today may need different tokens if they serve
different purposes. Conversely, elements using different shades may map to
the same token if they serve the same role.

Examples:
- Section subheadings ("Survey-in", "Save") -> `text-text-secondary` (structural labels)
- Field labels ("Period (s)") -> `text-text-secondary` (labels)
- Table column headers ("Tag", "Message") -> `text-text-secondary` (labels)
- Enabled checkbox/radio text -> `text-text-primary` (interactive, not muted)
- Disabled interactive labels -> `text-text-muted` (disabled)
- Status info ("Receiver is in stationary mode") -> `text-info` (informational, not `accent`)
- Empty-state placeholders ("No position data") -> `text-text-muted`

## Component primitives

Use the components from `ui.tsx` instead of repeating raw class strings.
Import what you need:

- **`Button`** -- `variant`: `primary` | `secondary` | `danger` | `ghost`; `size`: `sm` | `md`
- **`Input`** -- styled text input with optional `invalid` prop
- **`Select`** -- styled select with optional `invalid` prop
- **`Card`** -- surface container with subtle border (`bg-surface-2`)
- **`Badge`** -- status pill; `tone`: `default` | `info` | `success` | `warning` | `error`
- **`ConfigGroup`** -- collapsible section with chevron
- **`ConfigSubGroup`** -- indented section header with `disabled` prop
- **`ConfigSubSubGroup`** -- grid layout (label + content) with `disabled` prop
- **`MonitorDataView`** -- definition list for telemetry key-value rows

Helper functions:
- `labeledControlText(disabled)` -- returns text classes for checkbox/radio labels
- `fieldLabelText(disabled)` -- returns text classes for field labels
- `cx(...)` -- class name combiner

## General style

- Flat UI: no shadows except floating elements (dialogs, popovers)
- Dense layout: `text-xs` for data, `text-sm` for controls
- Sans-serif for UI; `font-mono` only for raw telemetry data
- No arbitrary pixel values for sizing; use the Tailwind spacing scale
