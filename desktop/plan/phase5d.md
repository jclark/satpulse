# Phase 5d: Live messages panel

## Goal
Add a new Messages panel that shows live packet statistics -- a count tree grouped by protocol and message type -- with an inspector that shows the most recent instance of a selected message type. This replaces the static "Protocols: NMEA, UBX" line that was removed in phase 5b with something genuinely useful.

## Prerequisite
Phase 5b (layout rework). The telemetry layout from 5b provides the panel slots.

## Motivation
When configuring a GPS receiver, a key question is "what messages is it actually sending?" The current packet monitor shows a raw firehose that scrolls too fast to be useful. The messages panel answers this question at a glance: protocol-level counts, per-message-type counts, and the ability to inspect the latest instance of any message type.

This is a monitoring panel (passive, live-updating), not a configuration panel.

## Concept

### Packet statistics tree
As packets arrive via the existing `gps:packet` event, the panel maintains running counts grouped by protocol tag (e.g. NMEA, UBX, RTCM) and then by message type within each protocol. The display is a compact tree:

```
NMEA  347
  GGA   52
  GSA   78
  GSV  142
  RMC   52
  ZDA   23
UBX   891
  NAV-PVT    204
  NAV-SAT    102
  TIM-TP     204
  TIM-TM2    177
  ...
```

Counts update live. The tree is always sorted by protocol name, then by message name within each protocol.

### Message inspector
Clicking a message type in the tree shows the most recent packet of that type in an inspector area below the tree. For NMEA, this shows the raw sentence. For binary protocols, it shows the hex dump and/or decoded fields if available.

The inspector updates live -- if the selected message type is still arriving, the inspector shows the latest instance. This is much more useful than the raw packet log because you see one clean packet instead of a blur.

### Last-message store
The panel keeps a map of `tag:msg -> last PacketEntry` in component state. Each incoming packet updates the count and overwrites the stored last-packet for its type. Memory is bounded because there is at most one stored packet per message type.

### Relationship to packet monitor
The messages panel does not replace the packet monitor. The packet monitor (phase 5b: collapsed by default, with freeze) shows raw packet ordering and timing for debugging. The messages panel shows what message types are flowing and lets you inspect the latest instance. They serve different purposes.

---

## Steps

### 1. Packet stats accumulator
Create a `usePacketStats` hook (or inline in the panel) that processes the `packetEntries` array and maintains:
- `Map<string, {count: number, msgs: Map<string, {count: number, last: PacketEntry}>}>` -- keyed by protocol tag, then by message name.
- Updated on each new packet entry.

The packet's `tag` field gives the protocol (e.g. "NMEA", "UBX"). The `msg` field gives the message type (e.g. "GGA", "NAV-PVT"). If `msg` is empty, use a generic label like "(unknown)".

### 2. Messages panel component
Create `messages-panel.tsx` with:
- A tree view of protocol -> message type with live counts.
- Each row is clickable to select it for the inspector.
- The currently selected row is highlighted.
- A Clear button resets all counts and clears the last-message store.
- A total packet count in the header.

### 3. Message inspector area
Below the stats tree, a fixed-height (or resizable) area shows the last packet of the selected message type:
- Timestamp of the packet.
- Raw content (ASCII for NMEA, hex for binary).
- The inspector updates live when new packets of the selected type arrive.
- When nothing is selected, shows a hint: "Click a message type to inspect".

### 4. Add to telemetry layout
Add the Messages panel to the `react-resizable-panels` layout in `app.tsx`. It fits naturally as a panel in the telemetry area, alongside Time/Survey and the packet monitor. Exact placement TBD based on how the layout feels, but likely as a third column or below Time/Survey on the left:

```
┌──────────────────────────────────────────────┐
│  Connection bar                              │
├────────────┬───────────────┬─────────────────┤
│            │               │                 │
│ Time       │  Messages     │ Packet monitor  │
│            │  (stats tree  │ (collapsed)     │
│────────────│   + inspector)│                 │
│ Survey     │               │                 │
│ (collapsed)│               │                 │
├────────────┴───────────────┴─────────────────┤
│  Activity log                                │
└──────────────────────────────────────────────┘
```

Add "Messages" to the Panels menu so it can be toggled on/off.

### 5. Wire up packet events
The Messages panel receives the same `packetEntries` array (or subscribes to the same `gps:packet` event) as the packet monitor. No backend changes needed -- all the data is already available.

## Result
A new Messages panel shows live packet statistics grouped by protocol and message type, with an inspector for the most recent packet of any selected type. This gives immediate visibility into what the receiver is actually sending.

## Files changed
- `desktop/frontend/src/messages-panel.tsx` (new component)
- `desktop/frontend/src/app.tsx` (add Messages panel to layout, Panels menu)
- `desktop/frontend/src/connection-panel.tsx` (add Messages to panel labels)

## Testing -- Playwright

### Stats tree
- Connect to a receiver; verify protocol groups appear with counts.
- Verify message types appear under each protocol with individual counts.
- Verify counts increment as packets arrive.

### Inspector
- Click a message type; verify the inspector shows the latest packet.
- Verify the inspector updates when a new packet of the selected type arrives.
- Click a different message type; verify the inspector switches.

### Clear
- Click Clear; verify all counts reset to zero.
- Verify new packets start accumulating fresh counts.

### Panel toggle
- Verify Messages appears in the Panels menu.
- Toggle it off; verify the panel disappears.
- Toggle it on; verify the panel reappears with current stats.
