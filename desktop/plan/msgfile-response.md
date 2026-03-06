# Message file response handling

## Goal
Add response display to the Messages tab when sending messages. After sending, the user sees per-message ACK/NAK results and other receiver responses alongside the send progress display.

## Prerequisites
- msgfile-send (done).
- `PacketAnalyzer` in `gps/msgfile` (done on master).
- `RawMsg.Index` field (done on master).

## Reference documents
- [gpscmd-response-rework.md](../../plan/gpscmd-response-rework.md) -- PacketAnalyzer design

## Steps

### 1. Integrate PacketAnalyzer into send flow

Modify the send goroutine in `app.go` to use `msgfile.PacketAnalyzer`:

- Create a `PacketAnalyzer` before the send loop.
- Subscribe to the packet broadcast (`a.pb`) so the send goroutine can read incoming packets. Unsubscribe when the goroutine exits.
- Call `NotifySent(rm)` for each message after writing it.
- During the delay between messages (and after the last message), read packets from the subscriber and process them.

Packet processing follows the same pattern as `responseHandler` in `internal/gpscmd/response.go`:
- Recognized packets (`pkt.Format != nil`): flush line buffer, call `Analyze(pkt.Tag(), pkt.Data)`.
- Unrecognized packets (`pkt.Format == nil`): feed raw bytes into a line buffer that accumulates printable ASCII and flushes complete lines through `Analyze(gpsprot.EmptyTag, line)`.

For each `Analyze` result where `Kind != NotResponse`, the backend constructs and emits a `gps:response` event. `NotResponse` packets are silently dropped.

### 2. gps:response event

The send goroutine emits `gps:response` events. This is a separate event from `gps:packet` -- the send goroutine does not modify the packet event pipeline. The backend resolves all packet details (including line buffering) so the frontend receives clean, display-ready data.

```go
// ResponseEvent is emitted as "gps:response" during SendMsgFile.
type ResponseEvent struct {
    Kind       string `json:"kind"`                 // "ack", "other", "maybe"
    ResponseTo int    `json:"responseTo"`            // 0-based index of sent message, or -1
    AckError   string `json:"ackError,omitempty"`    // ack only: empty = accepted, non-empty = rejected
    MsgCount   int    `json:"msgCount,omitempty"`    // ack only: total messages sent for this tag
    Tag        string `json:"tag,omitempty"`         // protocol tag (e.g. "UBX", "NMEA")
    MsgID      string `json:"msgID,omitempty"`       // message ID (e.g. "MON-VER", "PQTMVERNO")
    Text       string `json:"text,omitempty"`        // full text for text protocols
    Bin        string `json:"bin,omitempty"`          // hex string for binary protocols
}
```

The backend constructs `ResponseEvent` from `PacketAnalysis` and the original `scan.Packet`:
- `Kind`: maps from `PacketAnalysis.Kind` -- `AckResponse` -> `"ack"`, `OtherResponse` -> `"other"`, `MaybeResponse` -> `"maybe"`.
- `ResponseTo`: from `PacketAnalysis.RelatedMsg.Index` when matched, -1 otherwise.
- `AckError`: from `PacketAnalysis.AckError` (only for `"ack"` kind).
- `MsgCount`: from `PacketAnalysis.RelatedMsg.Count` when matched (only for `"ack"` kind).
- `Tag`: from `pkt.Format.Tag()` for recognized packets, empty for line-buffered text.
- `MsgID`: from `pkt.Format.MsgID([]byte(pkt.Data))` for recognized packets.
- `Text`/`Bin`: text for text-based protocols and line-buffered lines, hex for binary protocols. Uses the same ascii/bin distinction as `PacketLogEntry`.

### 3. Frontend layout

The Messages tab bottom area has two panes side by side:

```
+--- narrow ---+------------ wider -------------+
| Sent 1...done| $GNGGA,123456.00,5106.94,N,... |  <- raw content
|  1 accepted  |--------------------------------+
| Sent 2...done| {                              |  <- decode
|  2 accepted  |   "msgType": "GGA",            |
|  MON-VER     |   ...                          |
|  Listening...|                                |
+--------------+--------------------------------+
```

**Left pane** (narrow, scrollable): interleaved chronological status display. Lines appear and update as events happen:
- Send lines: "Sending message 1..." -> updates in place to "Sending message 1...done". For single-message tags, omit the number.
- Between send lines, response lines appear in the order they arrive:
  - `ack` accepted: "Message N accepted" in `text-success` (or "Message accepted" for single-message tags).
  - `ack` rejected: "Message N rejected: reason" in `text-danger`.
  - `other`: "Received TAG-MSGID" (e.g. "Received UBX-MON-VER"). Clickable.
  - `maybe`: same format as `other` but in `text-text-muted`. Clickable.
  - For `other`/`maybe` from line-buffered text (empty `tag`): truncated preview of the text. Clickable.
- During tail reading (Pausing state): "Listening..." in muted italic at the bottom, removed when Connected arrives.
- Colour distinguishes line types -- no indentation.

**Right pane** (wider, always visible): two stacked sections:
- **Top**: raw content of the selected response. Single line, monospace. Full text for text protocols; hex for binary protocols.
- **Below**: decode area. For binary protocols, calls `DecodePacket(bin, false)` and displays the JSON result. For text protocols, empty for now (future: wire up novmsg/uncmsg/qtmmsg decoders).
- When nothing is selected: placeholder text "Click a response to view".

Auto-select: the first `other` response is automatically selected when it arrives, so the decode panel is immediately populated. The user can click a different response to switch.

#### Tag row states

The tag table has two visual states for a row:
- **Selected** (background highlight): ready to send. Send button enabled. No results showing.
- **Has results** (dot marker, no highlight): results are displayed for this row. Send button disabled.

Flow:
- Click a tag row: row gets selected (highlight). Any previous dot/results/send status clear. If in Pausing state, cancel it. Send enables.
- Click Send: row loses highlight, gains dot. Send disables. Status lines and responses appear in the left pane.
- Sending finishes: enters Pausing (tail read for late responses). Dot remains.
- Pausing ends (timeout): dot remains, results stay.
- Click any tag row (including the dotted one): dot clears, results clear, pausing cancels if active. Row gets selected. Send enables.
- Switch tabs: cancels pausing if active. Dot and results stay (visible when user returns).
- No Cancel button. The user cancels by clicking a tag row or switching tabs.

#### Send button

Send is enabled only when `connState === "connected"` and a tag row is selected (highlighted). It is not enabled during sending or pausing.

### 4. Frontend state

In the App component:

```typescript
export interface ResponseLine {
    kind: 'ack' | 'other' | 'maybe';
    responseTo: number;      // 0-based index of sent message, or -1
    ackError?: string;       // ack only
    msgCount?: number;       // ack only: total messages sent for this tag
    tag?: string;            // protocol tag
    msgID?: string;          // message ID
    text?: string;           // text protocols
    bin?: string;            // binary protocols (hex)
}
```

State:
- `sendLines: SendLine[]` -- send progress lines, appended/updated on each `gps:msgsend` event.
- `responseLines: ResponseLine[]` -- appended on each `gps:response` event.
- `selectedResponseIndex: number` -- index into `responseLines` of the selected response, or -1.
- `selectedTagIndex: number` -- the highlighted tag row (-1 = none selected).
- `activeTagIndex: number` -- the tag row with the dot (-1 = no results).
- `tagArmed: boolean` -- true when a tag row is selected and ready to send. Set true on tag row click, false on Send click.

Clear `sendLines`, `responseLines`, `selectedResponseIndex`, and `activeTagIndex` on tag row click. Set `tagArmed = true`.

On Send click: set `activeTagIndex = selectedTagIndex`, `tagArmed = false`.

On file load: clear everything and reset `selectedTagIndex`.

Subscribe to `gps:response` in the event listener block alongside `gps:msgsend`. On the first `other` response, auto-set `selectedResponseIndex` if it is still -1.

### 5. Response reading timing

The send goroutine reads responses both during inter-message delays and after the final message:

- After writing each message and emitting `"sent"`, enter a read loop that runs for the delay duration. The loop pulls packets from the broadcast subscriber and processes them through the analyzer.
- After the last message, emit `"done"` and transition to `Pausing`. Continue reading for a few seconds to collect late responses. Some receivers respond slowly, especially after configuration changes that trigger internal resets. Emit `gps:response` events during this tail period.
- When the tail timeout expires, transition to `Connected`.

`SendMsgFile` only accepts `Connected`. The user must cancel pausing first (by clicking a tag row or switching tabs). `ApplyConfig` and `ReadConfig` accept `Pausing` as a valid starting state -- they cancel the tail reader (via the send context) before proceeding.

`CancelMsgSend` accepts both `Sending` and `Pausing`. It cancels the send context. If the state was `Pausing`, it immediately transitions to `Connected`.

The frontend receives `"pausing"` via `gps:state`. The visible effect is "Listening..." in muted italic at the bottom of the left pane. It disappears when `"connected"` arrives.

#### Cancellation from the frontend

There is no Cancel button. Pausing is cancelled by:
- Clicking a tag row: calls `CancelMsgSend` if in Pausing, then clears results and selects the row.
- Switching away from the Messages tab: calls `CancelMsgSend` if in Pausing. Dot and results stay.

#### Race condition handling

Since `SendMsgFile` only accepts `Connected`, and the only way to re-enable Send is clicking a tag row (which cancels pausing first), there is never a second send goroutine competing with an existing one. The `sendGen` generation counter is not needed.

## Testing (Playwright)

### With hardware
- Connect to a receiver.
- Switch to Messages tab; load a message file.
- Verify Send is disabled (no tag selected).
- Click a tag row; verify it highlights and Send enables.
- Click Send; verify the tag row loses highlight and gains a dot. Send disables.
- Verify the left pane shows interleaved send progress and responses.
- Verify accepted messages show "Message N accepted" in green.
- Verify rejected messages show "Message N rejected: reason" in red.
- For single-message tags, verify the number is omitted.
- Verify `other` responses show "Received TAG-MSGID" and are clickable.
- Verify the first `other` response is auto-selected and the right pane shows raw content and decode.
- Click a different `other` response; verify the right pane updates.
- Verify `maybe` responses appear in muted text and are clickable.
- For binary responses, verify the decode section shows JSON from `DecodePacket`.
- Verify "Listening..." appears during Pausing and disappears when Connected.
- Click a tag row; verify dot clears, results clear, pausing cancels, row highlights, Send enables.
- Switch tabs during Pausing; verify pausing cancels but dot and results stay when returning.
- Load a new file; verify all state is cleared.

## Result
Users see per-message response feedback in the Messages tab when sending messages: an interleaved status display alongside a decode panel for inspecting full responses and their decoded content. The interaction flow is simple: select a tag, send, view results, select again to send more.

## Files changed
- `desktop/app.go` (send goroutine with PacketAnalyzer, line buffer, ResponseEvent, gps:response emission, Pausing state, CancelMsgSend)
- `desktop/frontend/src/msgfile-panel.tsx` (two-pane layout, tag row states, interleaved status, decode panel)
- `desktop/frontend/src/app.tsx` (ResponseLine type, state management, gps:response subscription, Pausing in ConnState)
