# Phase 7a: Message file send

## Goal
Allow the desktop app to load TOML message files and send messages to the receiver. This phase covers loading, tag selection, sending, and send progress display. Response handling is deferred to phase 7b.

## Prerequisites
- Phase 5b (tab-based layout). The `gps/msgfile` package refactoring is already complete.
- Add `MsgCount int` field to `msgfile.TagDesc` and populate it in `collectDescs`. This gives the desktop app per-tag message counts without duplicating iteration logic. Done on master since it's a library change.

## Reference documents
- [ui-panel-message-file.md](ui-panel-message-file.md) -- UI design for the Message File panel

## Steps

### 1. Unified connection/activity state

Replace the current separate `connected` boolean and `statusText` with a single backend-driven state machine. This provides a write lock (preventing concurrent writes from the Configure and Messages panels) and a single source of truth for UI state.

#### States

```go
type ConnState string

const (
    StateDisconnected ConnState = "disconnected"
    StateConnecting   ConnState = "connecting"
    StateConnected    ConnState = "connected"
    StateConfiguring  ConnState = "configuring"
    StateSending      ConnState = "sending"
    StatePausing      ConnState = "pausing" // added in phase 7b
)
```

#### Transitions

- **Disconnected -> Connecting**: `Connect()` called.
- **Connecting -> Connected**: probe completes successfully.
- **Connecting -> Disconnected**: probe fails or user disconnects.
- **Connected -> Configuring**: `ApplyConfig()` or `ReadConfig()` called.
- **Configuring -> Connected**: config operation completes (success or error).
- **Connected -> Sending**: `SendMsgFile()` called.
- **Sending -> Connected**: send completes (success or error), or `CancelMsgSend()` called.
- **Connected -> Disconnected**: `Disconnect()` called.
- **Sending -> Disconnected**: connection lost during send.
- **Configuring -> Disconnected**: connection lost during config.

Phase 7b adds:
- **Sending -> Pausing**: last message written, tail read begins.
- **Pausing -> Connected**: tail read timeout expires, or cancelled by a new operation.
- **Pausing -> Disconnected**: connection lost during tail read.

#### Write lock semantics

`ApplyConfig`, `ReadConfig`, and `SendMsgFile` all check state under `a.mu`. If state is not `Connected` (or `Pausing`, added in phase 7b), they return an error. If `Pausing`, they cancel the tail reader before proceeding. On success they transition to `Configuring` or `Sending`. This prevents concurrent writes to the connection because only one caller can transition from `Connected`.

Wails dispatches each frontend call in its own goroutine, so concurrent calls are possible. The state check under the mutex handles this correctly.

#### Event

The backend emits `gps:state` with the new `ConnState` value on every transition. The frontend subscribes and stores the value. This replaces the frontend's `connected` boolean and `statusText` string.

#### Backend field

Add `state ConnState` to the `App` struct (replacing the `sending bool` from the original design). State transitions set `a.state` under `a.mu`, then emit `gps:state` after releasing the lock. The event must not be emitted under `a.mu` -- Wails event dispatch could re-enter App methods, causing deadlock.

#### Frontend changes

Replace `connected` (boolean) and `statusText` (string) with `connState` (string). The connection bar, tab enable/disable logic, and button enable/disable logic all derive from `connState`:

- Connection dot: green when not `disconnected`.
- Status text: derived from `connState` (e.g., `"Connected"`, `"Configuring..."`, `"Sending..."`).
- Configure Apply button: disabled unless `connState === "connected"`.
- Messages Send button: disabled unless `connState === "connected"` (plus the existing file/tag checks). Phase 7b widens this to also allow `"pausing"`.

### 2. Backend API

The backend exposes three new methods and one event type for message file operations.

#### LoadMsgFile

```go
// MsgFileTag is a tag from a loaded message file.
type MsgFileTag struct {
    Tag   string `json:"tag"`
    Desc  string `json:"desc,omitempty"`
    MsgCount int `json:"msgCount"`
}

// MsgFileInfo is the result of loading a message file.
type MsgFileInfo struct {
    Path string       `json:"path"`
    Tags []MsgFileTag `json:"tags"`
}

// LoadMsgFile opens a file dialog, loads the selected TOML message file,
// and returns the available tags. Returns nil if the user cancels the dialog.
func (a *App) LoadMsgFile() (*MsgFileInfo, error)
```

Behavior:
- Opens a native file dialog via `runtime.OpenFileDialog` filtered to `*.toml`.
- If the user cancels, returns `(nil, nil)`.
- Calls `msgfile.Load(path)` to parse and validate.
- Calls `mf.TagDescs()` to get tag/description pairs.
- Stores the `*msgfile.Parsed` and path on the `App` struct (protected by `a.mu`).
- Returns `MsgFileInfo` with the path and tags.
- Loading a new file replaces any previously loaded file.

The file dialog and file loading are combined into one call because the frontend never needs to specify a path directly -- the user always picks via the dialog.

#### SendMsgFile

```go
// SendMsgFile sends messages for the given tag from the loaded message file.
// Returns immediately. Progress is reported via gps:msgsend events.
// Returns an error if state is not Connected, no file is loaded, or the tag is invalid.
func (a *App) SendMsgFile(tag string) error
```

Behavior:
- Checks `a.state` under `a.mu`. If `Pausing`, cancels the tail reader first. If not `Connected` (after any cancellation), returns error.
- Calls `mf.TaggedMsgs([]string{tag})` then `msgfile.ToRaw(msgs)` to get `[]RawMsg`.
- Returns an error synchronously if conversion fails (e.g., no messages for the tag).
- Transitions state to `Sending`.
- Starts an async send goroutine. Returns nil to the caller.

Send goroutine detail:
- For each `RawMsg`: write bytes to `conn`, emit a `"sent"` event, then if `Delay > 0` emit a `"delaying"` event, sleep, and emit a `"delayed"` event.
- On write error: emit an error event, transition to `Connected` (or `Disconnected` if connection lost), return.
- On context cancellation: emit a cancelled event, transition to `Connected`, return.
- On completion: emit a done event, transition to `Connected`.
- The send goroutine does not read packets or process responses (that is phase 7b).

The send goroutine also logs each message via `a.lg.Info(...)` so the Logging panel shows per-message detail.

#### CancelMsgSend

```go
// CancelMsgSend cancels an in-progress send operation.
// Stops writing, emits a "cancelled" msgsend event, transitions to Connected.
// Returns an error if state is not Sending.
func (a *App) CancelMsgSend() error
```

The send goroutine uses a cancel context stored on `App`. `CancelMsgSend` cancels it. The goroutine checks the context between writes and during inter-message delays.

#### gps:msgsend event

```go
// MsgSendEvent is emitted as "gps:msgsend" during SendMsgFile.
type MsgSendEvent struct {
    // Status is "sent", "delaying", "delayed", "done", "cancelled", or "error".
    Status  string `json:"status"`
    Current int    `json:"current,omitempty"` // 1-based index of current message
    Total   int    `json:"total,omitempty"`
    Error   string `json:"error,omitempty"`
}
```

Events emitted per message:
- `{status: "sent", current: N, total: M}` after writing the message.
- `{status: "delaying", current: N, total: M}` before the delay (only if delay > 0).
- `{status: "delayed", current: N, total: M}` after the delay completes.

End-of-send events:
- `{status: "done", current: M, total: M}` on successful completion.
- `{status: "cancelled", current: K, total: M}` on cancel.
- `{status: "error", current: K, total: M, error: "..."}` on failure.

The frontend builds the progress line from these events (for single-message tags, the message number is omitted):
- `"sent"`: `Sending message N...`
- `"delaying"`: `Sending message N...pausing...`
- `"delayed"`: `Sending message N...pausing...done`
- `"done"` on the last message (without delay): `Sending message N...done`

### 3. Backend implementation

Add to `App` struct:
- `state ConnState` -- unified connection/activity state (replaces any separate `sending` or `connected` tracking)
- `msgFile *msgfile.Parsed` -- loaded file (nil if none)
- `msgFilePath string` -- path of loaded file
- `sendCancel context.CancelFunc` -- cancels the send goroutine (nil when idle)

Implement `LoadMsgFile` and `SendMsgFile` as described above.

The send goroutine:
```
for i, rm := range rawMsgs {
    write rm.Bytes to conn
    if error: emit error event, set state to Connected or Disconnected, return
    log "sent message" with index
    emit sent event
    if rm.Delay > 0:
        emit delaying event
        sleep rm.Delay
        emit delayed event
}
emit done event
set state to Connected
```

On disconnect (`closeLocked`): sets state to `Disconnected`. If a send is in progress, the connection close will cause the next write to fail, which stops the goroutine.

Refactor existing code to use the state machine:
- `Connect`: set state to `Connecting` at start, `Connected` after probe.
- `Disconnect` / `closeLocked`: set state to `Disconnected`.
- `ApplyConfig` / `ReadConfig`: set state to `Configuring` at start, `Connected` on return.

### 4. Frontend: Messages tab

Add a fourth tab **Messages** to the tab bar (after Configuration). The tab is always enabled (not gated on connection or receiver identification).

Implement `MsgFilePanel` component following [ui-panel-message-file.md](ui-panel-message-file.md).

Add to `App` state:
- `connState` -- replaces `connected` and `statusText`
- `msgFilePath`, `msgFileTags`, `selectedTagIndex`
- `sendLines` -- left side of send/response display
- `sendState: 'idle' | 'sending' | 'done' | 'error'`

Subscribe to `gps:state` events to update `connState`. Subscribe to `gps:msgsend` events to update `sendLines` and `sendState`.

Wire up:
- Open button calls `LoadMsgFile()`, updates path and tags on success. Selects index 0 if the file has a default (empty) tag or exactly one tag; otherwise no initial selection. Clears `sendLines`.
- Clicking a tag row updates `selectedTagIndex`.
- Send button calls `SendMsgFile(msgFileTags[selectedTagIndex].tag)`. Clears `sendLines` before calling.
- Send button disabled when: `connState !== 'connected'` (phase 7b widens to also allow `'pausing'`), or no file loaded, or no tag selected.
- Cancel button alongside Send. Calls `CancelMsgSend()`. Enabled when `connState === 'sending'`.
- Apply button in Configure panel: disabled when `connState !== 'connected'`.

The send/response display shows the left side only (send progress lines) in this phase. The right side (receiver responses) is added in phase 7b.

## Testing (Playwright)

### Without hardware
- Verify the Messages tab appears in the tab bar.
- Switch to the Messages tab; verify Open button is visible.
- Verify Send button is disabled when not connected.

### With hardware
- Connect to a receiver.
- Switch to the Messages tab.
- Click Open, select a test message file.
- Verify tags and descriptions are displayed.
- Click a tag row; verify it becomes selected.
- Click Send; verify progress display updates (`Sending message 1...done`, `Sending message 2...done`, ...).
- Verify completion: all lines show done.
- Verify per-message logs appear in the Logging panel.
- Load a different file; verify previous tags are replaced and send lines are cleared.
- Verify write lock: while sending, switch to Configuration tab and verify Apply is disabled.

### State machine
- Connect; verify status shows "Connected".
- Start a send; verify status shows "Sending...".
- After send completes; verify status returns to "Connected".
- Disconnect; verify status shows "Disconnected".

## Result
Users can load and send TOML message files from a dedicated tab, matching the `satpulsetool gps -m` workflow. The unified connection/activity state provides a write lock between the Configure and Messages panels. Responses are not yet displayed (see phase 7b).

## Files changed
- `desktop/app.go` (ConnState, state machine, LoadMsgFile, SendMsgFile, MsgSendEvent, refactor existing methods)
- `desktop/frontend/src/msgfile-panel.tsx` (new, Messages tab content)
- `desktop/frontend/src/app.tsx` (add Messages tab, connState replaces connected/statusText, gps:state subscription)
- `desktop/frontend/src/connection-panel.tsx` (use connState instead of connected boolean)
- `desktop/frontend/src/config-panel.tsx` (disable Apply when connState !== 'connected')
