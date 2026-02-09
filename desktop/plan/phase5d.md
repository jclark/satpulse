# Phase 5d: Live messages section

## Goal
Add a Messages collapsible section to the Monitor tab that shows live packet statistics -- a count tree grouped by protocol and message type. Clicking a message type opens a modal with a decoded snapshot of the most recent packet.

## Prerequisite
Phase 5b (tab-based layout rework). The Monitor tab from 5b provides the home for this section.

## Motivation
When configuring a GPS receiver, a key question is "what messages is it actually sending?" The Packets tab shows a raw firehose that scrolls too fast to be useful. The Messages section answers this question at a glance: protocol-level counts, per-message-type counts, and the ability to drill into any message type for a decoded view.

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

The stats tree is the only content in the Messages section -- it stays compact and information-dense. There is no inline inspector panel.

### Message detail modal
Clicking a message type in the stats tree opens a modal overlay showing a decoded snapshot of the most recent packet of that type. The modal shows a static snapshot -- it does not update live, so you can study the content in detail.

The content depends on the packet type:
- **ASCII packet** (e.g. NMEA): shows the raw ASCII sentence.
- **Binary packet** (e.g. UBX, CASIC, Unicore, NovAtel): the frontend sends the stored hex bytes to a backend `DecodePacket` method. If decoding succeeds, the modal shows the returned object pretty-printed as indented JSON. If decoding fails (unknown format, unknown message, checksum error), the modal shows the raw hex.

The modal is a simple centered overlay with a semi-transparent backdrop. Dismissed with Escape, click-outside, or an X button. The modal header shows the protocol tag, message type, and timestamp.

### Decoded packet service
The backend provides a `DecodePacket(hex string, out bool) DecodeResult` method (Wails-bound) that:
1. Decodes the hex string to bytes.
2. Calls `gpsdecode.Decode()` with the registered packet formats.
3. Returns the `DecodeResult` struct directly -- Wails marshals it as a JSON object.

The frontend receives a plain object and is responsible for pretty-printing it (`JSON.stringify(obj, null, 2)` in a `<pre>` block). The `out` parameter indicates packet direction (true = sent to receiver, false = received from receiver).

### Last-message store
The section keeps a map of `tag:msg -> last PacketEntry` in component state. Each incoming packet updates the count and overwrites the stored last-packet for its type. Memory is bounded because there is at most one stored packet per message type.

### Relationship to Packets tab
The Messages section does not replace the Packets tab. The Packets tab (phase 5b: with freeze/resume and search) shows raw packet ordering and timing for debugging. The Messages section shows what message types are flowing and lets you drill into decoded detail. They serve different purposes.

---

## Steps

### 1. Backend: DecodePacket method
Add a `DecodePacket(hex string, out bool)` method on the `App` struct:
- Decode hex string to `[]byte`.
- Call `gpsdecode.Decode(gpsreg.PacketFormats, data, out)`.
- Return the `*gpsdecode.DecodeResult` (or nil/error if decoding fails).

Import `gps/gpsdecode` and `gps/gpsreg`. The method is thin -- just hex decoding and one function call.

### 2. Packet stats accumulator
Create a `usePacketStats` hook (or inline in the component) that processes the `packetEntries` array and maintains:
- `Map<string, {count: number, msgs: Map<string, {count: number, last: PacketEntry}>}>` -- keyed by protocol tag, then by message name.
- Updated on each new packet entry.

The packet's `tag` field gives the protocol (e.g. "NMEA", "UBX"). The `msg` field gives the message type (e.g. "GGA", "NAV-PVT"). If `msg` is empty, use a generic label like "(unknown)".

### 3. Messages section component
Create `messages-panel.tsx` with:
- A tree view of protocol -> message type with live counts.
- Each row is clickable to open the detail modal.
- A Clear button resets all counts and clears the last-message store.
- A total packet count in the header.

### 4. Message detail modal
In `messages-panel.tsx` (or a separate `packet-detail-modal.tsx`):
- Fixed-position overlay with backdrop.
- Header: protocol tag, message type, timestamp.
- Body: for ASCII packets, show the raw sentence. For binary packets, call `DecodePacket` with the stored hex, then show the result as pretty-printed JSON in a monospace `<pre>` block. If decoding fails, show the hex.
- Dismiss: Escape key, click outside, or X button.

### 5. Add to Monitor tab
Add the Messages section as a collapsible section in the Monitor tab (in `app.tsx`), below Time and Survey. It uses the same `CollapsibleSection` component. Collapsed by default -- expands when the user clicks it.

### 6. Wire up packet events
The Messages section receives the same `packetEntries` array (or subscribes to the same `gps:packet` event) as the Packets tab. No backend changes needed for the stats -- all the data is already available.

## Result
A new Messages collapsible section in the Monitor tab shows live packet statistics grouped by protocol and message type. Clicking any message type opens a modal with a decoded snapshot -- structured JSON for binary protocols, raw ASCII for text protocols. This gives immediate visibility into what the receiver is sending and the ability to drill into detail without cluttering the Monitor tab layout.

## Files changed
- `desktop/app.go` (add `DecodePacket` method)
- `desktop/frontend/src/messages-panel.tsx` (new component: stats tree + modal)
- `desktop/frontend/src/app.tsx` (add Messages section to Monitor tab)

## Testing -- Playwright

### Stats tree
- Connect to a receiver; verify protocol groups appear with counts.
- Verify message types appear under each protocol with individual counts.
- Verify counts increment as packets arrive.

### Detail modal
- Click a message type; verify the modal opens with decoded content.
- For a binary message type (e.g. UBX NAV-PVT): verify the modal shows pretty-printed JSON fields.
- For an ASCII message type (e.g. NMEA GGA): verify the modal shows the raw sentence.
- Verify the modal does not update live (content is a snapshot).
- Dismiss the modal with Escape; verify it closes.
- Dismiss with click-outside; verify it closes.

### Clear
- Click Clear; verify all counts reset to zero.
- Verify new packets start accumulating fresh counts.

### Collapse
- Verify the Messages section header is visible in the Monitor tab.
- Collapse the section; verify the content hides.
- Expand it; verify stats are still live.
