# UI panel: message file

## Purpose
Allow the user to load a TOML message file, select a tag, and send its messages to the receiver. This panel brings the `satpulsetool gps -m` workflow to the desktop app.

## Design

### Contents
- file path display with Open button
- tag selector
- Send button
- send/response display

### File selection
An Open button opens a native file dialog (Wails `OpenFileDialog`) filtered to `.toml` files. On success, the file path is displayed and the backend loads and validates the file. On load error, show a native error dialog (`runtime.MessageDialog`) with the error text; the panel state is unchanged.

The file path persists across tab switches but not across app restarts.

### Tag selector
After a successful load, display the tags returned by the backend as a table of clickable rows with three columns:
- **Tag** -- tag name. The empty (default) tag is displayed as "(default)" in italics.
- **Description** -- description (if present).
- **# Messages** -- number of messages with this tag.

Exactly one tag is selected at a time. Clicking a row selects it (highlighted background). A tag is initially selected only if the file has a default (empty) tag or exactly one tag. Otherwise no tag is initially selected.

The empty tag (if present) appears first in the list. 

### Send and Cancel buttons
Send and Cancel buttons below the tag selector, side by side.

**Send** button states:
- **disabled** when not connected or sending or configuring, or no file loaded, or no tag selected. (msgfile-response adds `pausing` as an enabled state alongside `connected`.)
- **enabled** otherwise.

**Cancel** button states:
- **enabled** when a send is in progress (`connState === "sending"`).
- **disabled** otherwise.

Clicking Send calls the backend with the selected tag. A tag with multiple messages sends all of them in order. Clicking Cancel stops an in-progress send.

### Send/response display

Below the Send button, two side-by-side areas show send progress (left) and receiver responses (right). Sending and receiving are asynchronous -- the left side is under our control, the right side shows what the receiver sends back.

#### Left side: send progress

Each message gets a line as it is sent:
- Multi-message tag: `Sending message 1...done`, `Sending message 2...done`, etc. The current message shows `Sending message N...` until the write completes. If the message has a delay, the line shows `Sending message N...delaying...` during the delay, then `Sending message N...delaying...done` when the delay finishes.
- Single message tag: `Sending message...done`.
- On write error: the current line shows the error in red (e.g., `Sending message 3... write error: connection closed`).

#### Right side: receiver responses

Shows packets classified by the backend `PacketAnalyzer` as relevant to the send. Each response is formatted using two rules:

**Text-based protocols** (NMEA, proprietary ASCII, etc.): the raw packet text is shown in monospace. If the packet matched a sent message (`responseTo >= 0`), an interpretation line follows based on the kind: "Message N accepted" (ACK), "Message N rejected" (NAK, red), or "Response to message N" (poll response). For single-message tags, the number is omitted.

**Binary protocols** (UBX): the interpretation is shown directly:
- **Accepted**: `Message 1 accepted` (normal font). Displayed when the receiver ACKs a sent message.
- **Rejected**: `Message 1 rejected` (normal font, red). Displayed when the receiver NAKs a sent message.
- **Poll response**: `Received UBX-CFG-TP5` (normal font), or `Received UBX-CFG-TP5 in response to message N` for multi-message tags. For single-message tags, the "in response to" suffix is omitted. The user can inspect the decoded contents via the Messages section in the Monitor tab.
- **Unmatched ACK/NAK**: raw protocol detail in monospace (e.g., `UBX-ACK-ACK: 06-01`).

**Background traffic** (standard NMEA navigation sentences): not shown.

Response updates come from backend events (not polling). For single-message tags, the message number is omitted (e.g., `Message accepted` instead of `Message 1 accepted`).

#### Persistence

The send/response display persists after completion. It is cleared when the user starts a new send or loads a new file.

#### Error handling

A send-level error (e.g., connection lost) shows in the left side on the failing message line. The error persists until the next send attempt or file load.

### Interaction with other panels
- Per-message send logs appear in the Logging panel via the normal `gps:log` event stream.
- Poll response data can be decoded via the Messages section in the Monitor tab (live-messages).

### Future enhancement
The panel could add inline decoding of poll responses, allowing the user to click a response line to see decoded packet contents without switching tabs. This would reuse the same `DecodePacket` backend call and modal from the Messages section (live-messages).

### Data dependencies
- backend: load file, get tags, send messages, send progress events, response events
- connection state (for disabling Send)

### Notes
- The panel does not require receiver identification (unlike Configuration). Any connected receiver can receive messages.
- A file can be loaded while disconnected; only Send requires a connection.
- Loading a new file replaces the previous one. There is no multi-file support.
- Only one tag is sent at a time. To send multiple tags, the user sends them one at a time, choosing the order.

## Implementation

### Component
`MsgFilePanel` component, rendered as a tab content area (fourth tab: "Messages", after Configuration).

### Tab behavior
The Messages tab is always visible in the tab bar (not disabled when disconnected or unidentified). The Send button inside the panel handles the connection check.

### Wails bindings
See msgfile-send for the backend API design. The frontend calls:
- `LoadMsgFile()` -- opens dialog, returns tag list or error
- `SendMsgFile(tag)` -- starts sending the selected tag's messages; progress via events
- listens for `gps:msgsend` events for send progress (left side)
- listens for `gps:response` events for receiver responses (right side)

### Events

#### gps:msgsend
Send progress events (see msgfile-send). Used to update the left side:
- `sent` -- append "Sending message N..." line (omit number for single-message tags)
- `delaying` -- update current line to "Sending message N...delaying..."
- `delayed` -- update current line to "Sending message N...delaying...done"
- `done` -- mark the last message line as done (for last message or single message without delay)
- `error` -- show error on the current line

#### gps:response
Response events emitted by the backend during a send. The event payload is a `ResponsePacket` (see msgfile-response) with structured fields -- no pre-formatted display text. Fields:
- `kind`: `"ack"`, `"nak"`, `"poll-response"`, `"unmatched-ack"`, `"unmatched-nak"`, `"possible-reply"`
- `tag`: protocol tag from the received packet (e.g. `"UBX"`, `"NMEA"`)
- `msgID`: message ID from the received packet (e.g. `"06-01"`)
- `responseTo`: 0-based index of the sent message this responds to, or -1 if unmatched
- `text`: raw packet text for text-based protocols (absent for binary protocols)

The frontend formats each response using two rules:
- If `text` is set: show `text` in monospace. If `responseTo >= 0`, add an interpretation line below based on kind ("Message N accepted", "Message N rejected", or "Response to message N").
- If `text` is absent: show the interpretation directly -- "Message N accepted/rejected" for `ack`/`nak`, "Received TAG-MSGID" (or "Received TAG-MSGID in response to message N" for multi-message tags) for `poll-response`, `tag`+`msgID` in monospace for `unmatched-ack`/`unmatched-nak`.

### State
- `msgFilePath: string` -- loaded file path (empty if none)
- `msgFileTags: TagInfo[]` -- tags from loaded file
- `selectedTagIndex: number` -- index into `msgFileTags` (-1 if no selection)
- `sendLines: SendLine[]` -- left side lines (one per message being sent)
- `responseLines: ResponseLine[]` -- right side lines (accumulated responses)
- `sendState: 'idle' | 'sending' | 'done' | 'error'`

`SendLine`: `{status: 'sending' | 'delaying' | 'done' | 'error', index: number, total: number, error?: string}`

`ResponseLine`: the `ResponsePacket` event payload stored directly: `{kind: string, tag: string, msgID: string, responseTo: number, text: string}`

State lives in the top-level App component so it persists across tab switches. Both `sendLines` and `responseLines` are cleared on new send or file load.
