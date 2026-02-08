# Phase 5d: Live messages section

## Goal
Add a Messages collapsible section to the Monitor tab that shows live packet statistics -- a count tree grouped by protocol and message type -- with an inspector that shows the most recent instance of a selected message type.

## Prerequisite
Phase 5b (tab-based layout rework). The Monitor tab from 5b provides the home for this section.

## Motivation
When configuring a GPS receiver, a key question is "what messages is it actually sending?" The Packets tab shows a raw firehose that scrolls too fast to be useful. The Messages section answers this question at a glance: protocol-level counts, per-message-type counts, and the ability to inspect the latest instance of any message type.

This is a monitoring section (passive, live-updating), not a configuration section.

## Concept

### Packet statistics tree
As packets arrive via the existing `gps:packet` event, the section maintains running counts grouped by protocol tag (e.g. NMEA, UBX, RTCM) and then by message type within each protocol. The display is a compact tree:

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
The section keeps a map of `tag:msg -> last PacketEntry` in component state. Each incoming packet updates the count and overwrites the stored last-packet for its type. Memory is bounded because there is at most one stored packet per message type.

### Relationship to Packets tab
The Messages section does not replace the Packets tab. The Packets tab (phase 5b: with freeze/resume and search) shows raw packet ordering and timing for debugging. The Messages section shows what message types are flowing and lets you inspect the latest instance. They serve different purposes.

---

## Steps

### 1. Packet stats accumulator
Create a `usePacketStats` hook (or inline in the component) that processes the `packetEntries` array and maintains:
- `Map<string, {count: number, msgs: Map<string, {count: number, last: PacketEntry}>}>` -- keyed by protocol tag, then by message name.
- Updated on each new packet entry.

The packet's `tag` field gives the protocol (e.g. "NMEA", "UBX"). The `msg` field gives the message type (e.g. "GGA", "NAV-PVT"). If `msg` is empty, use a generic label like "(unknown)".

### 2. Messages section component
Create `messages-panel.tsx` with:
- A tree view of protocol -> message type with live counts.
- Each row is clickable to select it for the inspector.
- The currently selected row is highlighted.
- A Clear button resets all counts and clears the last-message store.
- A total packet count in the header.

### 3. Message inspector area
Below the stats tree, a fixed-height area shows the last packet of the selected message type:
- Timestamp of the packet.
- Raw content (ASCII for NMEA, hex for binary).
- The inspector updates live when new packets of the selected type arrive.
- When nothing is selected, shows a hint: "Click a message type to inspect".

### 4. Add to Monitor tab
Add the Messages section as a collapsible section in the Monitor tab (in `app.tsx`), below Time and Survey. It uses the same `CollapsibleSection` component. Collapsed by default -- expands when the user clicks it.

### 5. Wire up packet events
The Messages section receives the same `packetEntries` array (or subscribes to the same `gps:packet` event) as the Packets tab. No backend changes needed -- all the data is already available.

## Result
A new Messages collapsible section in the Monitor tab shows live packet statistics grouped by protocol and message type, with an inspector for the most recent packet of any selected type. This gives immediate visibility into what the receiver is actually sending.

## Files changed
- `desktop/frontend/src/messages-panel.tsx` (new component)
- `desktop/frontend/src/app.tsx` (add Messages section to Monitor tab)

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

### Collapse
- Verify the Messages section header is visible in the Monitor tab.
- Collapse the section; verify the content hides.
- Expand it; verify stats are still live.
