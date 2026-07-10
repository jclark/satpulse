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

### 1. Coordinator/worker send architecture

Sending uses two goroutines split along a control-plane / IO boundary:

**Coordinator goroutine** -- owns the send lifecycle after `SendMsgFile` has published `StateSending`: the `sendCancel` lifecycle, UI-facing `gps:msgsend` events, and the final `finishSend` transition back to `Connected` or `Disconnected`. Does not touch `conn.Write`, `PacketAnalyzer`, or the broadcast subscriber. Communicates with the worker via a `writeReqCh` channel, sending one `writeReq` per message and receiving a write error (or nil) back. Closes `writeReqCh` when done (success, error, or cancel) to tell the worker no more writes are coming. All exit paths call `finishSend`.

**Worker goroutine** -- owns `conn.Write`, `PacketAnalyzer`, line buffer, and the broadcast subscriber. The critical `Write` -> `NotifySent` sequence happens in one goroutine, eliminating the correlation race by construction. Runs a select loop over:
- `writeReqCh`: receives a `writeReq`, calls `conn.Write(req.rm.Bytes)`, then `pa.NotifySent(req.rm)` on success, and sends the error back on `req.reply`. When closed, starts a 3-second tail timer.
- Broadcast subscriber: receives packets, processes through line buffer and `pa.Analyze()`, emits `gps:response` events for non-`NotResponse` results.
- Tail timer: exits when it fires.
- `workerCtx.Done()`: exits immediately on cancellation.

Packet processing follows the same pattern as `responseHandler` in `internal/gpscmd/response.go`:
- Recognized packets (`pkt.Format != nil`): flush line buffer, call `Analyze(pkt.Tag(), pkt.Data)`.
- Unrecognized packets (`pkt.Format == nil`): feed raw bytes into a line buffer that accumulates printable ASCII and flushes complete lines through `Analyze(gpsprot.EmptyTag, line)`.

For each `Analyze` result where `Kind != NotResponse`, the worker checks `sessionCurrent(session)` and, if still valid, constructs and emits a `gps:response` event. `NotResponse` packets are silently dropped.

#### Types

```go
// writeReq is sent from the coordinator to the worker for each message.
type writeReq struct {
    rm    msgfile.RawMsg
    reply chan<- error
}
```

#### `SendMsgFile` setup

`SendMsgFile` does the setup work before either goroutine starts:

1. Under `a.mu`, verify `StateConnected`, capture `msgFile` / `conn` / `pb` / `connCtx`, cancel any previous coordinator (`sendCancel`) and invalidate any previous worker via `cancelWorkerLocked()`.
2. Outside the lock, resolve the selected tag into `rawMsgs`.
3. Create `sendCtx` from `a.ctx` and `workerCtx` from `connCtx`.
4. Under `a.mu` again, re-check `StateConnected`, then:
   - set `a.state = StateSending`
   - store `a.sendCancel = sendCancel`
   - store `a.workerCancel = workerCancel`
   - allocate `session := int(a.respSession.Add(1))`
5. Emit `gps:state = Sending`.
6. Start the worker goroutine.
7. Start the coordinator goroutine.

This means the coordinator goroutine does not perform the `Connected -> Sending` transition itself; it starts after state, cancel funcs, and session have already been published.

#### Coordinator goroutine flow

```
replyCh := make(chan error, 1)
emit "started" (with session, total)   // frontend latches session here
for each rawMsg:
    check sendCtx cancel -> emit "cancelled", return
    send writeReq{rm, reply} to writeReqCh
    wait for reply
    if write error -> emit "error", return
    emit "sent"
    if rm.Delay > 0:
        emit "delaying"
        wait rm.Delay or sendCtx cancel
        if cancelled -> emit "cancelled", return
        emit "delayed"
emit "done"
// defer: close(writeReqCh); finishSend()
```

The `"started"` event is the frontend's control-plane latch point for the new response session. It is emitted before the coordinator issues the first `writeReq`, so responses caused by the new send are associated with a latched session on the frontend. The frontend still filters every `gps:response` by exact session match and also clears the latch synchronously before any RPC that invalidates the worker.

#### Coordinator exit: `finishSend`

All coordinator exit paths close `writeReqCh` (via defer, before `finishSend`) and then call `finishSend`. This clears `sendCancel` and sets state to `Connected` (or `Disconnected` if the connection was lost) atomically under one mutex acquisition:

```go
func (a *App) finishSend() {
    a.mu.Lock()
    a.sendCancel = nil
    s := StateConnected
    if a.state == StateDisconnected || (a.connCtx != nil && a.connCtx.Err() != nil) {
        s = StateDisconnected
    }
    a.state = s
    a.mu.Unlock()
    runtime.EventsEmit(a.ctx, "gps:state", s)
}
```

#### Worker goroutine flow

```
sub := pb.Subscribe()
defer pb.Unsubscribe(sub)
pa := NewPacketAnalyzer()
var tailCh <-chan time.Time

select loop:
    case req, ok := <-writeReqCh:
        if !ok -> start 3s tail timer, nil out writeReqCh, continue
        err := conn.Write(req.rm.Bytes)
        if err == nil -> pa.NotifySent(req.rm)
        req.reply <- err

    case pkt := <-sub:
        process packet (line buffer / analyze)
        if Kind != NotResponse && sessionCurrent(session) -> emit gps:response

    case <-tailCh:
        return

    case <-workerCtx.Done():
        return
```

#### Worker lifecycle and `cancelWorkerLocked`

`workerCancel` is stored on App (`a.workerCancel`). `workerCtx` is derived from `connCtx`, so disconnect cancels it automatically. Cancellation uses the `cancelWorkerLocked()` helper, which atomically cancels the worker and bumps the session counter to invalidate any in-flight emissions:

```go
func (a *App) cancelWorkerLocked() {
    if a.workerCancel != nil {
        a.workerCancel()
        a.workerCancel = nil
        a.respSession.Add(1)
    }
}
```

`cancelWorkerLocked` is called by:
- `SendMsgFile`: cancels any old worker before starting a new one.
- `ReadConfig` / `ApplyConfig`: cancel the worker before starting config operations (prevents it from misclassifying config traffic as message responses).
- `closeLocked`: cancel the worker on disconnect.

The worker does not touch `a.state`, `a.sendCancel`, or any other App fields under the mutex. It only emits `gps:response` events (gated by `sessionCurrent`).

#### Session counter

`respSession` is an `atomic.Int32` on App. Each `SendMsgFile` increments it (via `respSession.Add(1)`) under the mutex and captures the value. The worker checks `sessionCurrent(session)` before emitting any `gps:response` event, suppressing stale emissions at the source:

```go
func (a *App) sessionCurrent(session int) bool {
    return int(a.respSession.Load()) == session
}
```

Both `emitResponse` and `flushWorkerLine` check `sessionCurrent` before emitting. This is the backend half of session gating. The frontend provides the other half (see section 4).

`cancelWorkerLocked` bumps `respSession` atomically alongside cancelling the worker context, so any in-flight emissions from a cancelled worker are immediately invalidated -- there is no window between cancellation and session invalidation.

#### App fields

```go
type App struct {
    // ... existing fields ...
    sendCancel   context.CancelFunc  // cancels coordinator goroutine
    workerCancel context.CancelFunc  // cancels worker goroutine
    respSession  atomic.Int32        // incremented each SendMsgFile and cancelWorkerLocked
}
```

#### Updates to other methods

- `closeLocked`: calls `cancelWorkerLocked()` (cancels worker and bumps session) and cancels `sendCancel`.
- `ReadConfig` / `ApplyConfig`: only accept `StateConnected`. Call `cancelWorkerLocked()` before proceeding.
- `CancelMsgSend`: only handle `StateSending`. Cancel `sendCancel`. The coordinator exits, closes `writeReqCh`, worker transitions to tail.
- Remove `StatePausing` entirely.
- Remove `readResponses` method (its logic is now in the worker goroutine's select loop).

### 2. gps:response event

The worker goroutine emits `gps:response` events. This is a separate event from `gps:packet` -- the worker does not modify the packet event pipeline. The backend resolves all packet details (including line buffering) so the frontend receives clean, display-ready data.

```go
// ResponseEvent is emitted as "gps:response" during SendMsgFile.
type ResponseEvent struct {
    Session    int    `json:"session"`               // session counter to filter stale events
    Kind       string `json:"kind"`                  // "ack", "other", "maybe"
    ResponseTo int    `json:"responseTo"`             // 0-based index of sent message, or -1
    AckError   string `json:"ackError,omitempty"`     // ack only: empty = accepted, non-empty = rejected
    MsgCount   int    `json:"msgCount,omitempty"`     // ack only: total messages sent for this tag
    Tag        string `json:"tag,omitempty"`          // protocol tag (e.g. "UBX", "NMEA")
    MsgID      string `json:"msgID,omitempty"`        // message ID (e.g. "MON-VER", "PQTMVERNO")
    Text       string `json:"text,omitempty"`         // full text for text protocols
    Bin        string `json:"bin,omitempty"`           // hex string for binary protocols
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

The Messages tab has a Send button with ephemeral send status, and a two-pane area below:

```
+--- Send [Sending...] --------+
+--- wider ----------+-narrow--+
| 1 accepted         | {       |  <- decode
| UBX-MON-VER b562.. |   ...   |
| Message rejected.. |         |
+--------------------+---------+
```

**Send status**: an ephemeral text label next to the Send button. Shows "Sending..." with dots during send, error text on failure, and disappears when done and connected. Not a separate line list.

**Left pane** (wider, scrollable): response lines only, in arrival order:
- `ack` accepted: "Message N accepted" in `text-success` (or "Message accepted" for single-message tags).
- `ack` rejected: "Message N rejected: reason" in `text-danger`.
- `other`: for text protocols, the raw text. For binary, "TAG-MSGID" with trailing hex in muted text. Clickable.
- `maybe`: same format as `other` but in `text-text-muted`. Clickable.
- Colour distinguishes line types -- no indentation.
- When no results and no tag armed: placeholder "Select a tag and click Send".

**Right pane** (`w-72`, fixed narrow): decode only.
- For binary protocols, calls `DecodePacket(bin, false)` and displays the JSON result. If the result has only a `payload` key, unwraps it.
- For text protocols, empty for now (future: wire up novmsg/uncmsg/qtmmsg decoders).
- When nothing is selected: placeholder text "Click a response to view".

Auto-select: the first `other` response is automatically selected when it arrives, so the decode panel is immediately populated. The user can click a different response to switch.

#### Tag row states

The tag table has two visual states for a row:
- **Selected** (background highlight): ready to send. Send button enabled. No results showing.
- **Has results** (dot marker, no highlight): results are displayed for this row. Send button disabled.

Flow:
- Click a tag row: row gets selected (highlight). Any previous dot/results/send status clear. Send enables.
- Click Send: row loses highlight, gains dot. Send disables. Responses appear in the left pane.
- Sending finishes: state returns to Connected. Dot remains. Response reader continues for 3 seconds collecting late responses.
- Click any tag row (including the dotted one): dot clears, results clear. Row gets selected. Send enables.
- Switch tabs: dot and results stay (visible when user returns).

#### Send button

Send is enabled only when `connState === "connected"` and a tag row is selected (highlighted). It is not enabled during sending.

### 4. Frontend state

In the App component:

```typescript
export interface ResponseLine {
    session: number;         // session counter to filter stale events
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
- `sendLines: SendLine[]` -- updated on each `gps:msgsend` event. Used to derive the ephemeral send status text next to the Send button (not displayed as individual lines).
- `responseLines: ResponseLine[]` -- appended on each `gps:response` event. Displayed in the left pane.
- `selectedResponseIndex: number` -- index into `responseLines` of the selected response, or -1.
- `selectedTagIndex: number` -- the highlighted tag row (-1 = none selected).
- `activeTagIndex: number` -- the tag row with the dot (-1 = no results).
- `tagArmed: boolean` -- true when a tag row is selected and ready to send. Set true on tag row click, false on Send click.

Clear `sendLines`, `responseLines`, `selectedResponseIndex`, `activeTagIndex`, and decode result on tag row click. Set `tagArmed = true`.

On Send click: set `activeTagIndex = selectedTagIndex`, `tagArmed = false`. Clear `sendLines`, `responseLines`, `selectedResponseIndex`, and decode result.

On file load: clear everything. Auto-select the first tag if there is only one tag or if the first tag is the default (empty) tag.

#### Session gating

Session gating has two layers -- backend and frontend -- to close all race windows:

**Backend**: the worker checks `sessionCurrent(session)` before emitting `gps:response` events. `cancelWorkerLocked()` bumps the session atomically alongside cancellation, so a cancelled worker's in-flight emissions are immediately invalid. This suppresses stale events at the source.

**Frontend**: uses `respSessionRef` (`useRef`) to filter events that slip through (for example, events already in the Wails event queue when the backend session was bumped):

- `gps:msgsend` handler: on `"started"` status, latches `respSessionRef.current = session`. This is the frontend's authoritative session-adoption point for a new send.
- `gps:response` handler: drops events where `evt.session !== respSessionRef.current`.
- `gps:state` handler: clears `respSessionRef.current = 0` on `"configuring"` or `"disconnected"` transitions.

#### Synchronous `clearRespSession`

The frontend clears `respSessionRef` synchronously before RPCs that invalidate the session, not just reactively on state events. This closes the window between when the backend cancels the worker (bumping the session) and when the corresponding `gps:state` event arrives at the frontend.

A `clearRespSession` callback (`() => { respSessionRef.current = 0 }`) is passed as a prop to both `MsgFilePanel` and `ConfigPanel`. It is called:
- `MsgFilePanel.handleOpen`: before loading a new file.
- `MsgFilePanel.handleTagClick`: before clearing results for a new tag selection.
- `ConfigPanel.doReadback`: before calling `ReadConfig()`.
- `ConfigPanel` apply handler: before calling `ApplyConfig()`.
- `handleConnect` (in App): before calling `Disconnect()`.

Add `session` field to `MsgSendEvent`:

```typescript
export interface SendLine {
    session: number;
    status: 'started' | 'sent' | 'delaying' | 'delayed' | 'done' | 'cancelled' | 'error';
    // ... existing fields ...
}
```

### 5. Response reading timing

The worker goroutine handles both serial writes and response reading in one select loop:

- During sending: the coordinator sends `writeReq` values to the worker. The worker performs `conn.Write` then `pa.NotifySent` atomically (same goroutine, no channel race). Between write requests, the worker processes incoming packets from the broadcast subscriber, classifying and emitting responses as they arrive.
- After the last message: the coordinator closes `writeReqCh` and calls `finishSend` (state returns to Connected). The worker detects the closed channel and starts a 3-second tail timer to collect late responses. Some receivers respond slowly, especially after configuration changes that trigger internal resets.
- When the tail timer expires or `workerCtx` is cancelled, the worker unsubscribes and exits.

`SendMsgFile` only accepts `Connected`. `ReadConfig` and `ApplyConfig` only accept `Connected`.

`CancelMsgSend` only handles `Sending`. It cancels the coordinator's context. The coordinator exits, closes `writeReqCh` (via defer), the worker sees the closed channel and transitions to tail.

#### Invariants

1. `StateSending` is exclusive -- only one coordinator runs at a time.
2. `ReadConfig` and `ApplyConfig` only start from `Connected`. They call `cancelWorkerLocked()` first (stops any lingering worker and bumps the session so its emissions are invalid).
3. No operation can install a new `sendCancel` before the coordinator publishes Connected, because `finishSend` clears `sendCancel` and sets Connected atomically under one lock.
4. `Write` and `NotifySent` are structurally ordered -- they execute sequentially in the worker goroutine, so the PacketAnalyzer only learns about a message after the corresponding `conn.Write` has succeeded.
5. Session counter prevents stale events at two layers: the backend gates emissions via `sessionCurrent()` (lock-free `atomic.Int32` check), and the frontend gates via `respSessionRef`. The frontend adopts the session from the `"started"` event and clears it synchronously before any RPC that invalidates the worker (`ReadConfig`, `ApplyConfig`, `Disconnect`, file load, tag change). That closes the frontend-side acceptance window for stale events.

## Testing (Playwright)

### With hardware
- Connect to a receiver.
- Switch to Messages tab; load a message file.
- Verify Send is disabled (no tag selected).
- Click a tag row; verify it highlights and Send enables.
- Click Send; verify the tag row loses highlight and gains a dot. Send disables.
- Verify ephemeral send status shows "Sending..." next to the button during send.
- Verify the left pane shows response lines (ack, other, maybe).
- Verify accepted messages show "Message N accepted" in green.
- Verify rejected messages show "Message N rejected: reason" in red.
- For single-message tags, verify the number is omitted ("Message accepted").
- Verify `other` responses for text protocols show raw text and are clickable.
- Verify `other` responses for binary protocols show TAG-MSGID with trailing hex and are clickable.
- Verify the first `other` response is auto-selected and the right pane shows decode.
- Click a different `other` response; verify the right pane updates.
- Verify `maybe` responses appear in muted text and are clickable.
- For binary responses, verify the decode section shows JSON from `DecodePacket`.
- Verify state returns to Connected after sending finishes. Late responses may still appear for a few seconds.
- Click a tag row; verify dot clears, results clear, row highlights, Send enables.
- Load a new file; verify all state is cleared.

## Result
Users see per-message response feedback in the Messages tab when sending messages: an ephemeral send status next to the button, a response list in the left pane, and a decode panel in the right pane for inspecting binary responses. The interaction flow is simple: select a tag, send, view results, select again to send more.

## Frontend changes

Remove `'pausing'` from the `ConnState` type in `app.tsx`. Remove the `pausing` entry from `connStateLabel`. Remove the `connState === 'pausing'` check in `handleTabChange` (simplify to just `setActiveTab(tab)`).

In `msgfile-panel.tsx`: remove the `connState === 'pausing'` check and `CancelMsgSend` call in `handleTagClick`.

Remove unused `CancelMsgSend` imports from both files if no longer referenced.

## Files changed
- `desktop/app.go` (coordinator/worker goroutines, writeReq type, finishSend, cancelWorkerLocked, sessionCurrent, atomic.Int32 respSession, ResponseEvent session field, MsgSendEvent session field and "started" status, CancelMsgSend, ReadConfig/ApplyConfig, remove StatePausing, remove readResponses)
- `desktop/frontend/src/msgfile-panel.tsx` (two-pane layout, tag row states, response list, decode panel, clearRespSession prop, remove pausing cancel)
- `desktop/frontend/src/app.tsx` (ResponseLine type with session, respSessionRef, session gating in gps:response handler, "started" event session adoption in gps:msgsend handler, clearRespSession on configuring/disconnected state, remove Pausing from ConnState)
- `desktop/frontend/src/config-panel.tsx` (clearRespSession prop, called before ReadConfig and ApplyConfig)
