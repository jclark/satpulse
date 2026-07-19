# UI design: Packets tab

## Purpose
Define the layout and behaviour of the Packets tab. This tab answers "what messages is my receiver sending?" at a glance, with the ability to drill into decoded detail.

## Concept

### Message table (primary view)
The tab shows a full-width tabular view with one row per message type. As packets arrive via the existing `gps:packet` event, the table maintains running counts and the most recent packet for each protocol/message combination.

Columns:

| Column | Content |
|--------|---------|
| Expand | Narrow column at the left edge. Shows a chevron (▸/▾) when there are multiple packets of this type in the active window. Empty otherwise. Does not affect alignment of other columns. |
| Protocol | Protocol tag (e.g. NMEA, UBX, RTCM) |
| Message | Message type within the protocol (e.g. GGA, NAV-PVT) |
| Count | Running count of packets received for this type |
| Last received | Timestamp of the most recent packet, to millisecond precision (HH:MM:SS.mmm) |
| Last message | Raw content of the most recent packet. ASCII packets show the sentence (minus trailing CRLF). Binary packets show compact hex (no spaces between bytes). Not truncated -- uses the full available width. |

Rows are sorted alphabetically by protocol, then by message name within each protocol.

New rows appear automatically as previously unseen message types arrive.

### Active/inactive highlighting
A row is "active" if a packet of that type arrived within the last 1.5 seconds. Active rows are displayed with normal text brightness. Inactive rows are dimmed. This gives an immediate visual sense of which messages are currently flowing versus ones that appeared once and stopped.

### Decode panel
Clicking a row in the message table opens a decode panel below the table showing a decoded snapshot of the most recent packet of that type. The panel is a fixed area at the bottom of the tab -- not a popup. It shows a static snapshot and does not update live. Clicking a different row replaces the panel content. An X button closes the panel.

Panel content depends on the packet type:
- **ASCII packet** (e.g. NMEA): shows the full raw sentence.
- **Binary packet** (e.g. UBX): the frontend sends the stored hex to a backend `DecodePacket` method. If decoding succeeds, the panel shows the result as pretty-printed indented JSON. If decoding fails, the panel shows the raw hex.

Panel header shows protocol tag, message type, and timestamp.

### Decoded packet service
The backend provides a `DecodePacket(hex string, out bool) *gpsdecode.DecodeResult` method (Wails-bound) that:
1. Decodes the hex string to bytes.
2. Calls `gpsdecode.Decode()` with the registered packet formats.
3. Returns the `DecodeResult` struct directly -- Wails marshals it as JSON.

The frontend pretty-prints the result with `JSON.stringify(obj, null, 2)` in a monospace `<pre>` block. The `out` parameter indicates packet direction (true = sent to receiver, false = received from receiver).

### Freeze
A Freeze button pauses the display. While frozen the table shows a snapshot of the state at the moment freeze was pressed. The underlying data continues to accumulate in the background. When unfrozen, the display jumps to the current live state with all counts and timestamps up to date.

### Controls
A Clear button in the toolbar resets all counts, clears stored messages, and removes all rows.

### Data model
Component state maintains a map keyed by `protocol:message` with:
- `count`: running packet count
- `recentEntries`: recent packets for this type. Always contains at least one entry (the last packet seen). Entries older than 1.5s are pruned except the last one, which always stays. The last element is the most recent packet -- used for display and decode.

Memory is bounded -- a small number of recent packets per message type, limited by the active window.

### Expand (multi-message types)
Some message types produce multiple packets per epoch (e.g. NMEA GSV is paged across 3-4 sentences). When a message type has more than one packet in the active window (last 1.5s), the expand column shows a chevron. Clicking it expands the row to show all individual packets from the active window in chronological order.

Expanded child rows show only the timestamp and raw data columns, aligned with the parent's last received and last message columns. Protocol, message, and count are omitted since they match the parent.

The data model stores the recent packets within the active window for each message type (not just the last one). This is bounded by the active window duration -- typically a handful of packets per type.

### Sequential snapshot
A "Snapshot" button in the toolbar captures all packets from the active window (last 1.5s) and displays them in a dialog sorted by arrival time. This gives a chronological view of what arrived in the most recent epoch -- useful for seeing message ordering, burst patterns, and multi-sentence groups like GSV pages.

The dialog is a centered panel with a dimmed backdrop (Wails v2 does not support multiple native windows). It shows a frozen snapshot that does not update live. Controls:
- **Refresh** button recaptures a fresh snapshot from the current active window, replacing the dialog content.
- **Close** button (X) dismisses the dialog. Also dismissed with Escape or click outside.

```
Snapshot -- 14:23:07
Received     Protocol  Message  Data
14:23:07.123  NMEA      GGA      $GPGGA,142307.00,4812.34567,N,...
14:23:07.230  UBX       NAV-PVT  b56201075c00e80720...
14:23:07.234  UBX       NAV-SAT  b562013500...
14:23:07.454  NMEA      GSV      $GPGSV,3,1,12,01,45,090,42,...
14:23:07.455  NMEA      GSV      $GPGSV,3,2,12,14,60,220,38,...
14:23:07.456  NMEA      GSV      $GPGSV,3,3,12,31,15,303,22*4C
14:23:07.789  NMEA      RMC      $GPRMC,142307.00,A,...
                                 [Refresh] [Close]
```

## Layout

Collapsed (default):

```
  Protocol  Message    Count  Last received    Last message
  NMEA      GGA           52  14:23:07.123     $GPGGA,142307.00,4812.34567,N,01156.789,E,...
▸ NMEA      GSV          142  14:23:07.456     $GPGSV,3,3,12,31,15,303,22*4C
  NMEA      RMC           52  14:23:07.789     $GPRMC,142307.00,A,4812.34567,N,01156.789,E,...
  UBX       NAV-PVT      204  14:23:07.234     b56201075c00e80720...
  UBX       NAV-SAT      102  14:23:06.345     b562013500...
  UBX       TIM-TP       204  14:23:06.891     b5620d011000...
                                               [Freeze] [Snapshot] [Clear]
```

Expanded (GSV has 3 sentences in the active window):

```
  Protocol  Message    Count  Last received    Last message
  NMEA      GGA           52  14:23:07.123     $GPGGA,142307.00,4812.34567,N,01156.789,E,...
▾ NMEA      GSV          142  14:23:07.456     $GPGSV,3,3,12,31,15,303,22*4C
                               14:23:07.454     $GPGSV,3,1,12,01,45,090,42,03,30,150,38,...
                               14:23:07.455     $GPGSV,3,2,12,14,60,220,38,17,25,315,30,...
                               14:23:07.456     $GPGSV,3,3,12,31,15,303,22*4C
  NMEA      RMC           52  14:23:07.789     $GPRMC,142307.00,A,4812.34567,N,01156.789,E,...
  UBX       NAV-PVT      204  14:23:07.234     b56201075c00e80720...
```

Full-width monospace. Active rows (packet within last 1.5s) at normal brightness; inactive rows dimmed.

Decode panel (when a row is clicked):

```
  Protocol  Message    Count  Last received    Last message
  NMEA      GGA           52  14:23:07.123     $GPGGA,142307.00,...    <- selected
  ...
+------------------------------------------------------------+
| NMEA GGA  14:23:07.123                                  [X] |
|                                                              |
| $GPGGA,142307.00,4812.34567,N,01156.78901,E,1,12,0.8,...    |
+------------------------------------------------------------+
```

For binary packets the decode panel shows pretty-printed JSON:

```
+------------------------------------------------------------+
| UBX NAV-PVT  14:23:07.234                               [X] |
|                                                              |
| {                                                            |
|   "iTOW": 522187000,                                        |
|   "year": 2026,                                             |
|   "month": 3,                                               |
|   "day": 2,                                                 |
|   "hour": 14,                                               |
|   ...                                                        |
| }                                                            |
+------------------------------------------------------------+
```

## Relationship to Monitor tab
The Packets tab is an operational/debugging tool. The Monitor tab shows interpreted data (position, satellites, signals). They serve different audiences -- the Packets tab is for understanding what the receiver is actually sending at the protocol level.

## Files changed
- `desktop/app.go` (add `DecodePacket` method)
- `desktop/frontend/src/packet-panel.tsx` (rewrite: message table + modal)
- `desktop/frontend/src/app.tsx` (minor wiring changes if needed)
