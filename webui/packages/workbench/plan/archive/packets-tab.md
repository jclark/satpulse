# Implementation: Packets tab rewrite

UI design: [ui-packet-tab.md](ui-packet-tab.md).

## Context

The current Packets tab is a raw scrolling log of up to 200 packets that is essentially unusable at typical GPS message rates. The replacement is a message-type table showing one row per protocol/message combination with live counts, timestamps, active highlighting, expand for multi-message types, a decode panel, and a sequential snapshot dialog.

## Files to change

| File | Change |
|------|--------|
| `desktop/app.go` | Add `DecodePacket` method (~10 lines + 2 imports) |
| `desktop/frontend/src/packet-panel.tsx` | Full rewrite |
| `desktop/frontend/src/app.tsx` | Remove `packetEntries` state and `gps:packet` listener; simplify `PacketPanel` props to just `visible` |

## Steps

### 1. Backend: add DecodePacket to app.go

Add imports `"encoding/hex"` and `"github.com/jclark/satpulse/gps/gpsdecode"`.

Add method on `App`:

```go
// DecodePacket decodes a hex-encoded packet and returns the decoded fields.
func (a *App) DecodePacket(hexStr string, out bool) (*gpsdecode.DecodeResult, error) {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}
	_, r, err := gpsdecode.Decode(gpsreg.PacketFormats, b, out)
	if err != nil {
		return nil, nil
	}
	return r, nil
}
```

Returns `nil, nil` on decode failure so frontend gets a null result and falls back to raw hex (no error popup for unknown messages).

### 2. Frontend: rewrite packet-panel.tsx

#### Data model

```typescript
interface MsgTypeState {
    tag: string;                    // Protocol tag ("NMEA", "UBX", etc.)
    msg: string;                    // Message type ("GGA", "NAV-PVT", etc.)
    count: number;                  // Running count since last clear
    recentEntries: PacketLogEntry[]; // Always has at least one entry (the last packet).
                                    // Entries older than 1.5s are pruned, except the
                                    // last one which always stays.
}
```

The last element of `recentEntries` is always the most recent packet -- used for the "Last received" and "Last message" columns, and for the decode panel. Active state is determined by whether the last entry's timestamp is within 1.5s. Expand chevron appears when `recentEntries.length > 1`.

State stored in a `Map<string, MsgTypeState>` keyed by `${tag}:${msg}`.

#### Architecture: useRef for live data, timer-driven rendering

- `liveRef = useRef<Map<string, MsgTypeState>>()` -- mutable accumulator, updated on every packet without triggering re-renders.
- `displayed` state -- snapshot of the live map, updated every 200ms by a `setInterval` timer.
- The timer also prunes `recentEntries` older than 1.5s.
- This avoids re-rendering on every packet (20+ per second) while keeping the UI responsive.

#### Packet ingestion

`PacketPanel` registers its own `gps:packet` event listener in a `useEffect`. Each packet updates `liveRef` directly (increment count, push to recentEntries). No React state update per packet.

#### Active/inactive

A row is active if the last entry's timestamp is within 1.5s of now. Active rows use `text-text-primary`, inactive use `text-text-muted`.

#### Pruning

The 200ms timer prunes each `recentEntries` array: remove entries older than 1.5s, but always keep the last one. This means an inactive row has exactly one entry (the last packet ever seen), while an active row has one or more entries from the current window.

#### Freeze

- `frozenRef` boolean + `frozenSnapshot` ref holding a cloned map.
- On freeze: deep-clone the current `liveRef` map into `frozenSnapshot`, set `displayed` to the snapshot.
- While frozen: timer skips updating `displayed`; packets still accumulate in `liveRef`.
- On unfreeze: clear snapshot, next timer tick resumes live updates.

#### Expand

- `expanded` state: `Set<string>` of expanded keys.
- Chevron shown in a fixed-width left column only when `recentEntries.length > 1`.
- Expanded child rows show only timestamp and raw data, aligned with parent columns.
- Clicking chevron toggles expand (with `stopPropagation` to avoid triggering row click/decode).

#### Decode panel

- Fixed panel below the table (not a modal). `max-height: 40%`, `overflow-y-auto`.
- On row click: set `decodeTarget` state. For ASCII packets, show raw sentence directly. For binary, call `DecodePacket(hex, out)` and show result as `JSON.stringify(result, null, 2)` in a `<pre>` block; on failure show raw hex.
- Header: protocol, message, timestamp, X close button.
- Clicking a different row replaces content. X closes the panel.

#### Sequential snapshot dialog

- "Snapshot" button collects all `recentEntries` across all message types, flattens and sorts by timestamp.
- Displayed in a centered dialog with dimmed backdrop (reuse `signal-picker.tsx` modal pattern: `fixed inset-0 z-50 bg-surface-1/70`).
- Tabular layout: Received | Protocol | Message | Data.
- Refresh button recaptures from current `liveRef` (even if main view is frozen).
- Close via X button, Escape, or click outside.

#### Clear

Resets `liveRef` to empty map, clears freeze state, expanded set, decode target, and snapshot.

#### Sorted rows

`useMemo` on `displayed` map: convert to array, sort by `(tag, msg)` alphabetically. Runs on each 200ms tick over ~10-30 rows.

#### Component structure

```
PacketPanel (flex-col, h-full)
|-- Table area (flex-1, overflow-y-auto)
|   +-- <table> with sticky header, sorted rows, expand children
|-- Decode panel (conditional, shrink-0, border-t, max-height 40%)
|   |-- Header: tag, msg, timestamp, X button
|   +-- <pre> block with decoded content
|-- Toolbar (shrink-0, border-t, flex items-center gap-2)
|   |-- Freeze button
|   |-- Snapshot button
|   +-- Clear button
+-- Snapshot dialog (conditional, fixed overlay)
    |-- Backdrop
    +-- Card with header, table, Refresh/Close buttons
```

#### Props (simplified)

```typescript
interface Props {
    visible: boolean;
}
```

### 3. Update app.tsx

- Remove `packetEntries` state (line 112) and `setPacketEntries`.
- Remove `gps:packet` event listener (lines 202-204) and its cleanup.
- Simplify `PacketPanel` usage (lines 517-521): remove `packetEntries` and `setPacketEntries` props.
- Remove `PacketLogEntry` from the app.tsx exports/imports if it was only used for the state.
- Note: packet data is NOT cleared on disconnect currently, and same behaviour continues -- `PacketPanel` manages its own lifecycle.

### 4. Reuse from existing code

- `formatTime()` from current `packet-panel.tsx` -- keep as-is
- `stripTrailingEOL()` from current `packet-panel.tsx` -- keep as-is
- Modal pattern from `signal-picker.tsx` -- backdrop, Card, Escape handling, stopPropagation
- Chevron SVG from `collapsible-section.tsx` -- rotate-90 pattern
- `Button`, `Card` from `ui.tsx`
- `EventsOn`/`EventsOff` from `../wailsjs/runtime/runtime`
- `DecodePacket` from `../wailsjs/go/main/App` (auto-generated after backend change)

### 5. Build and verify

- Run `wails dev` to regenerate TypeScript bindings for `DecodePacket`.
- Connect to GPS receiver.
- Verify: message types appear with incrementing counts.
- Verify: active rows highlighted, inactive dimmed after 1.5s.
- Verify: expand chevron on GSV (multiple per epoch), shows child rows.
- Verify: click row opens decode panel with ASCII or decoded JSON.
- Verify: freeze pauses display, unfreeze shows updated counts.
- Verify: snapshot dialog shows chronological packet list.
- Verify: clear resets everything.
