# Monitor tab layout rework

## Goal
Rework the Monitor tab from vertically stacked collapsible sections into an information-dense layout. Compact the Time and Survey displays into dense single-line summaries. Arrange visual elements (sky view, clock, messages tree) side by side to use horizontal space effectively.

## Prerequisite
signal-strength, sky-view, live-messages.

## Motivation
After live-messages, signal-strength, and sky-view, the Monitor tab has many sections stacked vertically: Time, Survey, Messages, Sky View, and per-constellation signal graphs. The key-value panels (Time, Survey) waste horizontal space with their definition-list formatting. The sky view is square and sits awkwardly in a full-width row. The layout needs to use horizontal space efficiently while remaining scrollable vertically.

## Concept

### Dense time summary
Replace the current Time definition list with a compact single-line format:

```
14:23:07.123 UTC  2024-01-15  TAI 1738079005  LS 37  GPS
```

One line conveys everything the current `TimePanel` shows in 7 rows. The format is self-evident after seeing it once -- no field labels needed. This sits at the top of the Monitor tab as a thin header-style element.

### Dense survey summary
Replace the current Survey definition list with a compact single-line format:

```
Survey: 48.1234°N 11.5678°E  ±1.2m  5432 obs  In Progress
```

Prefixed with "Survey:" since the content isn't self-evident like a time display. Only appears when the receiver provides survey data (which many configurations don't use). Sits just below the time line.

### Middle row: visual elements side by side
Below the compact time/survey summaries, a flex row holds the square/narrow visual elements side by side:
- Sky view (square, fixed aspect ratio)
- Messages stats tree (narrow)
- Space for future panels (analog clock, etc.)

These elements have natural widths and sit comfortably side by side in a flex row. At narrow window widths, they stack vertically.

### Signal graphs at the bottom
Per-constellation signal graphs (from signal-strength) sit below the middle row. Each is wide and short with vertical bars, fitting naturally as stacked full-width rows. They scroll vertically and can be individually collapsed.

### Overall structure
```
┌──────────────────────────────────────┐
│ 14:23:07 UTC  2024-01-15  TAI ...    │ <- Time (one line)
│ Survey: 48.12°N 11.57°E  ±1.2m  ... │ <- Survey (one line, conditional)
├──────────┬──────────┬────────────────┤
│ Sky View │ Messages │ (future        │
│          │ NMEA 347 │  panels)       │
│          │   GGA 52 │                │
│          │ UBX  891 │                │
│          │   NAV 204│                │
├──────────┴──────────┴────────────────┤
│ GPS signal graph (vertical bars)     │
├──────────────────────────────────────┤
│ Galileo signal graph                 │
├──────────────────────────────────────┤
│ BeiDou signal graph                  │
└──────────────────────────────────────┘
```

The whole Monitor tab scrolls vertically. The middle row uses flexbox with `flex-wrap` so elements wrap gracefully at narrow widths.

## Steps

### 1. Compact time display
Create a new compact `TimePanel` (or a variant) that renders time data as a single horizontal line of key values. No definition list, no field labels. Monospace font for the numeric values.

### 2. Compact survey display
Create a new compact `SurveyPanel` (or a variant) that renders survey data as a single line prefixed with "Survey:". Only renders when `surveyMsg` is present.

### 3. Middle row layout
Replace the current collapsible section stack with a flex layout:
- Time and survey summaries at the top (full width, thin).
- A flex row containing the sky view, messages tree, and any future visual panels.
- Signal graph sections below, each full-width.

The sky view uses a fixed-aspect-ratio container. The messages tree takes its natural width. The flex row wraps at narrow widths.

### 4. Remove collapsible wrappers from time/survey
Time and survey are now single-line summaries -- they don't need collapsible sections. They are always visible (when data is present). The collapsible behavior is removed for these two elements.

The sky view, messages tree, and signal graphs retain their collapsible sections.

## Result
The Monitor tab is information-dense: time and survey are single lines at the top, visual elements sit side by side in the middle, and per-constellation signal graphs stack below. Horizontal space is used efficiently. The layout scrolls vertically and wraps gracefully at narrow widths.

## Files changed
- `desktop/frontend/src/time-panel.tsx` (rework to compact single-line format)
- `desktop/frontend/src/survey-panel.tsx` (rework to compact single-line format)
- `desktop/frontend/src/app.tsx` (rework Monitor tab layout: flex rows, remove collapsible wrappers from time/survey)

## Testing -- Playwright

### Layout
- Verify time appears as a single-line summary at the top of the Monitor tab.
- Verify survey appears as a single line (prefixed "Survey:") only when survey data is present.
- Verify sky view and messages tree sit side by side in the middle row.
- Verify signal graphs appear as full-width sections below.

### Responsive
- At wide window width: verify middle row elements sit side by side.
- At narrow window width: verify middle row elements stack vertically.
- Verify the whole Monitor tab scrolls vertically when content overflows.

### Data
- Without satellite data: verify only time/survey summaries and messages tree are visible.
- With satellite data: verify all sections appear in the correct layout.
