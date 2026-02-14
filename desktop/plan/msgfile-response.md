# Message file response handling

## Goal
Add response display to the Messages tab when sending messages. After sending, the user sees per-message ACK/NAK results and other receiver responses in the right side of the send/response display, as defined in [ui-panel-message-file.md](ui-panel-message-file.md).

## Prerequisites
- msgfile-send.
- Done on master since they are library changes:
  - `PacketAnalyzer` in `gps/msgfile` (see [gpscmd-response-rework.md](../../plan/gpscmd-response-rework.md)).
  - Add `PollResponse` kind to `PacketAnalyzer.Analyze` for UBX data responses that match the class/ID of a sent poll command. Without this, poll responses are classified as `Background` and not shown to the user.
  - Remove `Text` field from `PacketAnalysis` -- formatting is the caller's responsibility, not the analyzer's.
  - Remove `Label()` method from `RawMsg` -- the caller formats from structured fields (`Tag`, `Index`, `Count`).
  - Add `Index` field to `RawMsg` (0-based index within the tag) so callers can identify which sent message a response matches.

## Reference documents
- [ui-panel-message-file.md](ui-panel-message-file.md) -- UI design (send/response display, right side)
- [gpscmd-response-rework.md](../../plan/gpscmd-response-rework.md) -- PacketAnalyzer design

## Steps

### 1. Integrate PacketAnalyzer into send flow

Modify the send goroutine from msgfile-send to use `msgfile.PacketAnalyzer`:

- Create a `PacketAnalyzer` before the send loop.
- Subscribe to the packet broadcast (`a.pb`) so the send goroutine can read incoming packets.
- Call `NotifySent(rm)` for each message after writing it.
- During the delay between messages (and after the last message), read packets from the subscriber:
  - Recognized packets (`pkt.Format != nil`): call `Analyze`, emit `gps:response` for non-`Background` results.
  - Unrecognized packets (`pkt.Format == nil`): feed the raw data through a line buffer that accumulates printable ASCII chars and flushes on newline (same logic as `responsePrinter.handleUnrecognized` in `internal/gpscmd/response.go`). Each flushed line is emitted as a `gps:response` event with `kind: "possible-reply"`, `tag: ""`, and `data` set to the buffered line.

msgfile-send's goroutine only writes; this plan adds reading.

### 2. gps:response event

The send goroutine emits `gps:response` events. This is a separate event from `gps:packet` -- the send goroutine does not modify the packet event pipeline. The backend packages up structured data from `PacketAnalysis` and `scan.Packet` without making display decisions.

```go
// ResponsePacket is emitted as "gps:response" during SendMsgFile.
type ResponsePacket struct {
    Kind       string `json:"kind"`              // "ack", "nak", "poll-response",
                                                  // "unmatched-ack", "unmatched-nak",
                                                  // "possible-reply"
    Tag        string `json:"tag"`               // protocol tag from scan.Packet (e.g. "UBX", "NMEA")
    MsgID      string `json:"msgID,omitempty"`   // message ID from scan.Packet
    ResponseTo int    `json:"responseTo"`         // 0-based index of sent message, or -1
    Text       string `json:"text,omitempty"`    // raw packet text for text-based protocols
}
```

The backend constructs `ResponsePacket` from `PacketAnalysis` and the original `scan.Packet`:
- `Kind`: maps directly from `PacketAnalysis.Kind` (using lowercase string equivalents). `Background` packets are not emitted.
- `Tag`: from `pkt.Tag()`.
- `MsgID`: from the packet's parsed message ID (protocol-specific).
- `ResponseTo`: from `PacketAnalysis.AckFor.Index` (0-based) when matched, -1 otherwise.
- `Text`: the raw packet text for text-based protocols (e.g. NMEA, proprietary ASCII). Not set for binary protocols (e.g. UBX). Present on any kind -- a text protocol can have ACKs, NAKs, poll responses, etc.

For unrecognized packets (line-buffered text), the backend emits a `ResponsePacket` with `kind: "possible-reply"`, `tag: ""`, and `text` set to the buffered line.

The frontend knows the total message count from the tag it sent, so it can decide whether to show "Message 3 accepted" or just "Message accepted". All formatting is the frontend's responsibility.

### 3. Frontend: response display in Messages tab

Subscribe to `gps:response` events and append each `ResponsePacket` to `responseLines` state in the App component.

Update `MsgFilePanel` to display the right side of the send/response display. The frontend formats each response line using two rules:

1. If `text` is set (text-based protocol): show `text` in monospace. If `responseTo >= 0`, add an interpretation line below it based on `kind`: "Message N accepted" (`ack`), "Message N rejected" (`nak`, red), or "Response to message N" (`poll-response`). For single-message tags, omit the number (e.g. "Message accepted", "Response to message").
2. If `text` is absent (binary protocol): show the interpretation directly:
   - `ack`/`nak`: "Message N accepted" or "Message N rejected" using `responseTo` (0-based, display as 1-based). Omit the number if the tag has only one message. `nak` lines are red.
   - `poll-response`: "Received " + a formatted name from `tag` and `msgID`. For multi-message tags with `responseTo >= 0`: "Received UBX-CFG-TP5 in response to message N". For single-message tags: "Received UBX-CFG-TP5".
   - `unmatched-ack`/`unmatched-nak`: formatted protocol detail from `tag` and `msgID` in monospace (e.g. "UBX-ACK-NAK: 06-01").
   - `possible-reply`: should not occur without `text`.

Clear `responseLines` on new send or file load (same as `sendLines`).

### 4. Response reading timing

The send goroutine needs to read responses both during inter-message delays and after the final message. Design:

- After writing each message and emitting `"sent"`, enter a read loop that runs for the delay duration.
- The read loop pulls packets from the broadcast subscriber, filters for recognized packets (`pkt.Format != nil`), and calls `Analyze`.
- After the read window closes, proceed to the next message.

After the last message is written, the goroutine emits `"done"` and transitions to `Pausing`. It then continues reading for a few seconds to collect late responses. Some receivers are slow to respond, especially for configuration changes that trigger internal resets. The goroutine emits `gps:response` events during this tail period. When the tail timeout expires, the goroutine transitions to `Connected`.

The `Pausing` state is not a write lock. `SendMsgFile`, `ApplyConfig`, and `ReadConfig` accept `Pausing` as a valid starting state -- they cancel the tail reader (via the send context) before proceeding. This avoids two goroutines emitting `gps:response` events simultaneously.

Extend `CancelMsgSend` to also accept the `Pausing` state: it cancels the tail reader and transitions to `Connected`.

Update the frontend Send button logic: enabled when `connState === "connected" || connState === "pausing"` (plus existing file/tag checks). This lets the user start a new send without waiting for the tail timeout. Cancel remains enabled only during `"sending"`.

The frontend receives `"pausing"` via `gps:state`. The only visible effect is a subtle indicator in the response display that responses may still be arriving (e.g. a faint "listening..." line or blinking dot that disappears when `"connected"` arrives).

## Testing (Playwright)

### With hardware
- Connect to a receiver.
- Switch to the Messages tab; load a message file with UBX/CASBIN/ASBIN commands.
- Click Send; verify the right side shows per-message responses (e.g. `Message 1 accepted`, `Message 2 rejected`).
- For single-message tags, verify the number is omitted (e.g. `Message accepted`).
- Verify rejected responses are visually distinct (red).
- Verify poll responses show `Received UBX-...`.
- Verify unmatched ACK/NAK and possible-reply lines show in monospace.
- Load a new file; verify `responseLines` are cleared.
- Send again; verify previous `responseLines` are cleared before new responses appear.

## Result
Users see per-message response feedback in the Messages tab when sending messages, matching the send/response display design in ui-panel-message-file.md.

## Files changed
- `desktop/app.go` (send goroutine with PacketAnalyzer, ResponseEvent, gps:response emission)
- `desktop/frontend/src/msgfile-panel.tsx` (right side response display)
- `desktop/frontend/src/app.tsx` (responseLines state, gps:response subscription)
