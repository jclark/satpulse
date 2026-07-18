# Message File Response Delta Plan

## Purpose

This document is a delta plan for bringing the useful parts of `origin/msgfile-response` into the current desktop implementation.

It is not a replacement for the existing message-tab plan. The current desktop flow already has:

- `SendMsgFile` with a coordinator/worker split
- session-gated `gps:response` events
- a Messages tab response list and decode pane
- a race-free ownership model around `conn.Write`, packet subscription, and response handling

The delta is about restoring the smarter response-driven waiting behavior from the old correlator design while preserving the current locking and UX semantics.

## What Must Stay True

These are non-negotiable constraints:

1. `a.mu` continues to protect only App lifecycle state: connection state, cancel funcs, session invalidation, and related fields.
2. The worker goroutine remains the sole owner of:
   - `conn.Write`
   - response-correlation state
   - line buffering for unrecognized packets
   - the packet broadcast subscription
   - all decisions about when it is safe to send the next message
3. `ReadConfig`, `ApplyConfig`, disconnect, and a new `SendMsgFile` must still be able to invalidate the response worker through `cancelWorkerLocked()`.
4. The frontend UX must remain the same as today:
   - `StateSending` ends promptly after the last write plus mandatory send delay
   - `finishSend()` still returns the UI to `Connected` immediately
   - the worker may continue tailing responses after the UI is back in `Connected`
5. A single received packet may be both an ACK and payload-bearing data. The backend must support that directly.

## Current Gap

The `msgfile-response` branch is now merged into `desktop-gui`. The library-level changes (`Correlator`, `WaitLimit`, per-protocol analyzers) are available. However, `desktop/app.go` still references the old `PacketAnalyzer` API and will not compile. The remaining gaps in `desktop/app.go` are:

1. Waiting is blunt.
   The worker always uses a fixed tail after the command stream closes. It does not know when:
   - the next write would be ambiguous if sent too early
   - all expected responses have already arrived

2. Response classification is one-dimensional.
   `PacketAnalyzer` returns one `Kind`, which forces a packet to be either:
   - ack
   - other response
   - maybe response
   - not response

That model cannot represent "this packet ACKed message N and also contains data worth showing".

## Proposed Delta

### 1. Replace the worker's analyzer with the Correlator

The `msgfile-response` branch already provides the `Correlator` type in `gps/msgfile/correlate.go` with the exact API needed. Now that the branch is merged, the desktop worker should use it directly.

The Correlator's exported API:

```go
cor := msgfile.NewCorrelator()
cor.NotifyMsgSent(rm)                              // record a sent message
cor.CorrelatePacket(tag, data) msgfile.Correlation  // classify a received packet
cor.ReadyToSend(rm) bool                           // safe to send without ACK ambiguity?
cor.CanAcceptMore() bool                           // any responses still expected?
cor.Missing() (missingAck, missingData []*RawMsg)  // unmet expectations
```

### 2. Use the Correlator's two-axis result type

The `Correlation` struct already separates ACK outcome from display relevance via two independent fields:

```go
type Correlation struct {
    Ack          AckKind        // AckNone, AckAck, AckNak, AckOther
    NakError     string         // meaningful when Ack == AckNak
    InResponseTo *RawMsg        // non-nil when Ack != AckNone
    Relevance    RelevanceLevel // LevelAckOnly .. LevelSoleResponse
}
```

The two axes are:

- **Ack axis**: `Ack != AckNone` means the packet acknowledges a sent message. `InResponseTo` identifies which one.
- **Relevance axis**: `Relevance >= LevelMaybeResponse` means the packet contains displayable content.

A packet can satisfy both conditions simultaneously (e.g. a PQTM query response that is both `AckAck` and `LevelSoleResponse`). The worker must check both axes independently when deciding what events to emit.

### 3. Reintroduce smarter waiting inside the worker only

Do not move waiting logic into the coordinator and do not consult correlation state under `a.mu`.

Instead, the worker should become the only place that decides:

- when the next message may be written
- when the post-send response tail is complete

That preserves the current race-free property:

- the same goroutine that performs `conn.Write`
- is the goroutine that calls `NotifySent`
- is the goroutine that processes incoming packets
- is the goroutine that decides whether correlation ambiguity has cleared

### 4. Keep the current coordinator/worker ownership split

The coordinator remains a control-plane goroutine. It should still own:

- `sendCancel`
- `gps:msgsend` progress events
- `finishSend()`
- the `StateSending -> Connected` transition

The worker remains an IO/correlation goroutine. It should still own:

- `conn.Write`
- packet subscription
- response correlation state
- send pacing
- post-send draining

No new mutex sharing should be introduced between them.

## Proposed Flow

### Send setup

`SendMsgFile` setup remains structurally the same:

1. Under `a.mu`, verify `StateConnected`, capture connection-related fields, cancel any old coordinator, cancel and invalidate any old worker.
2. Outside the lock, resolve the selected tag into `rawMsgs`.
3. Create `sendCtx` and `workerCtx`.
4. Under `a.mu`, re-check `StateConnected`, publish:
   - `a.state = StateSending`
   - `a.sendCancel = sendCancel`
   - `a.workerCancel = workerCancel`
   - incremented `respSession`
5. Emit `gps:state = Sending`.
6. Start the worker.
7. Start the coordinator.

No change is needed to the locking pattern here.

### Coordinator behavior

The coordinator should still emit the same user-visible `gps:msgsend` lifecycle as today.

The difference is that each per-message request to the worker now includes pacing semantics.

For each message, the coordinator sends a `sendStepReq` and waits for the reply. The reply means the entire send step is complete: write, delay, and pacing (if not the last message). While waiting, the coordinator cannot emit fine-grained progress events like `"delaying"` because it does not know the internal timing of the worker's step.

The simplest approach is to drop the `"delaying"` / `"delayed"` sub-states from `gps:msgsend`. The coordinator emits `"sent"` when the reply arrives (meaning write + delay + pacing are all done). The frontend progress display simplifies accordingly: it shows which message was last sent and whether sending is in progress, without a separate "delaying" state.

If fine-grained delay reporting is needed later, the worker could send interim notifications on a separate channel. That is not required for this delta.

For the final message, the worker still waits for the delay but does not pace (there is no next message). The reply arrives after the delay completes.

Then the coordinator behaves exactly as it does now:

- emit `"done"`
- close the worker command stream
- call `finishSend()`

This preserves the current UX semantics: the UI goes back to `Connected` promptly after the last send step, not after the tail drain finishes.

### Worker behavior

The worker still owns the packet subscription and still holds `portLock` for its whole lifetime.

The difference is that its lifetime becomes response-aware instead of fixed-tail-only.

For each command message:

1. Write `rm.Bytes` to the connection.
2. On success, call `cor.NotifySent(rm)`.
3. Continue processing packets while waiting for:
   - the configured `Delay`
   - and, if there is a next message, `cor.ReadyToSend(next)` or deadline expiry
4. Reply to the coordinator with success/error once that step is complete.

After the coordinator closes the command stream, the worker enters a smarter tail phase:

- keep processing incoming packets
- stop as soon as `cor.CanAcceptMore()` is false
- otherwise stop when the tail deadline expires
- on timeout, optionally emit "missing response" rows based on `cor.Missing()`

This is where the typical wait should become shorter than today: the worker can exit as soon as correlation says there is nothing more to wait for.

## Worker/Coordinator Protocol

The current `writeReq{rm, reply}` shape is too small for smarter pacing. The worker needs to know whether there is a next message so it can call `ReadyToSend(next)` before replying.

Use a command struct along these lines:

```go
type sendStepReq struct {
    rm       msgfile.RawMsg
    next     *msgfile.RawMsg   // nil for the last message
    reply    chan<- sendStepResult
}

type sendStepResult struct {
    err error
}
```

The worker derives the pacing deadline from `rm.WaitLimit` and also updates a running deadline for the tail phase. No deadline needs to be passed from the coordinator.

The result stays minimal. A reply means "this send step is complete" (write + delay + pacing), not merely "the bytes were written".

The worker loop still multiplexes:

- send-step requests
- incoming packets
- worker cancellation
- post-close tail timeout

## Wait Budget

The `msgfile-response` branch already added `WaitLimit time.Duration` to `RawMsg`. Now that the branch is merged, this field is available.

The worker uses `WaitLimit` to bound all pacing waits:

- **Per-message pacing deadline**: after writing message `i`, the worker computes a deadline from `rm.WaitLimit`. It processes packets until `ReadyToSend(next)` returns true or the deadline expires, whichever comes first.
- **Tail drain deadline**: the worker tracks a running deadline as `max(deadline, time.Now().Add(rm.WaitLimit))` across all messages. After the coordinator closes the command stream, the worker drains until `CanAcceptMore()` returns false or this deadline expires.

This follows the same pattern as `internal/gpscmd/gpscmd.go:sendAllMsgs` on the branch.

## ACK + Data Handling

This is the easy part and should stay simple.

The backend should emit up to two response rows for one packet:

1. an ACK row, if the packet acknowledges a specific sent message
2. a payload row, if the packet also contains displayable content

That avoids a frontend redesign. The current Messages tab already handles an ordered stream of response rows well.

Consequences:

- the frontend `ResponseLine` type can stay row-oriented
- the backend event builder must be able to produce multiple `ResponseEvent`s from a single packet
- auto-select logic should prefer the first payload-bearing row, not specifically the first `"other"` row

## Frontend Delta

Frontend changes should be deliberately small.

### What should not change

- tag selection / armed / dotted row behavior
- send button enablement
- state-driven transition back to `Connected`
- session gating model
- decode panel structure

### What should change

1. `gps:response` handling should treat the first payload-bearing row as auto-selectable.
   Today the first `"other"` row is auto-selected. With ACK+data split into two emitted rows, that rule should become:
   - auto-select the first row with packet/text content the user can inspect

2. ACK rows remain non-clickable.

3. Payload rows remain clickable.

No larger UI redesign is needed for this delta.

## Locking and Cancellation Semantics

### App mutex

Do not introduce new correlation or pacing state under `a.mu`.

`a.mu` continues to cover only:

- connection state
- `sendCancel`
- `workerCancel`
- `respSession`
- connection teardown / startup handoff

### Session invalidation

The current `respSession` model remains correct and should be kept:

- each new `SendMsgFile` creates a new response session
- `cancelWorkerLocked()` cancels the worker and bumps the session atomically
- the worker checks `sessionCurrent(session)` before emitting response events
- the frontend still filters by exact session match

Smarter waiting must fit inside that existing model.

### Config / disconnect interaction

No semantic change:

- `ReadConfig` and `ApplyConfig` still call `cancelWorkerLocked()` before starting config traffic
- disconnect still cancels both coordinator and worker
- a new send still cancels any old worker before starting a new session

The worker must remain disposable at any point.

## Missing Response Reporting

The old correlator had a `Missing()` concept. That is useful, but the desktop UI should use it carefully.

Recommended behavior:

- only emit missing-response rows when the tail wait expires
- do not emit them on normal early completion
- format them as terminal status rows, not packet rows

Example rows:

- `Message 2: no response received`
- `Message 4: no data response received`

This is optional for the first implementation pass. Smarter pacing and smarter early completion matter more.

## Note on `RawMsg.source` field

The merged branch changes `RawMsg.source` from type `responsePattern` to `requestAnalyzer`. This is an unexported field internal to `gps/msgfile`. It drives the Correlator's `analyzeRequest` method and does not affect any code in `desktop/app.go`. No action needed, but be aware of the type change if reading `msgfile` internals.

## Implementation Order

The `msgfile-response` branch is now merged into `desktop-gui`. The Correlator, `WaitLimit`, and all library-level changes are already available. The remaining work is in `desktop/app.go` and the frontend.

1. Update the worker in `desktop/app.go` to use `msgfile.NewCorrelator()` instead of `msgfile.NewPacketAnalyzer()`. This is a mechanical rename to restore compilation:
   - `NewPacketAnalyzer()` -> `NewCorrelator()`
   - `pa.NotifySent(rm)` -> `cor.NotifyMsgSent(rm)`
   - `pa.Analyze(tag, data)` -> `cor.CorrelatePacket(tag, data)`
   - `msgfile.PacketAnalysis` -> `msgfile.Correlation`
   - Filter: `result.Kind != msgfile.NotResponse` -> check `Ack` and `Relevance` axes
2. Update `makeResponseEvent` to map from `Correlation` fields, emitting up to two `ResponseEvent`s per packet (ACK row + payload row).
3. Change the coordinator/worker request protocol from `writeReq` to `sendStepReq` so a worker reply means "send step complete" (write + delay + pacing).
4. Add pacing logic to the worker: after write + delay, process packets until `ReadyToSend(next)` or deadline.
5. Replace the fixed 3-second tail with a `CanAcceptMore()` loop bounded by the running deadline.
6. Keep `finishSend()` timing as it is today: immediately after the last send step, before the worker's final tail has necessarily ended.
7. Drop `"delaying"` / `"delayed"` sub-states from `gps:msgsend` events (coordinator no longer has visibility into delay timing). This also requires updating the frontend: remove `"delaying"` and `"delayed"` from the `MsgSendEvent` status type in TypeScript and update the status rendering logic in `msgfile-panel.tsx` so it no longer expects or displays those states.
8. Make the small frontend change for first payload auto-selection.
9. Add tests around pacing, tail completion, cancellation, and ACK+data dual emission.

## Tests

### Backend tests

Add tests for:

- `ReadyToSend(next)` blocking until ambiguity clears
- `ReadyToSend(next)` unblocking on timeout
- `CanAcceptMore()` reaching false before deadline
- final tail timeout with `Missing()` results
- one packet producing both ACK and payload output
- session invalidation suppressing stale events after:
  - `ReadConfig`
  - `ApplyConfig`
  - disconnect
  - a new send

### Frontend tests

Add tests for:

- UI returns to `Connected` promptly after the last send step, while late responses may still append
- first payload-bearing response row is auto-selected
- ACK rows remain non-clickable
- payload rows remain clickable and decode correctly

## Summary

The delta is:

- keep the current race-free coordinator/worker architecture
- restore smarter, response-aware waiting inside the worker
- preserve the current prompt `Connected` transition in the UI
- support a packet being both ACK and displayable data by emitting two rows

The crucial implementation rule is simple: the worker, and only the worker, owns the correlation state that drives pacing.
