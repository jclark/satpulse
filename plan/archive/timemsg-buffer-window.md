# Time-message buffer window sizing

`timemsg.Buffer` keeps messages within a `readWindow` that is currently
hardcoded to `5 * time.Second` at all four construction sites. This is
a latent bug on the controller path: reset mode requests
`cfg.Reset.PulseWindow` consecutive messages (default 5) via
`GetPostTimeMessages`, and at 1 Hz a 5 s window only just-barely-
coincidentally holds 5 messages. Slow-startup message rates, or any
configured `PulseWindow > 5`, silently underfill the request and stall
reset mode without an obvious diagnostic.

The fix has two parts: provision the buffer from the controller's
actual needs when one is present, and move the magic-number floor out
of the call sites into the `timemsg` package itself.

## Changes

### timemsg owns the floor

The semantics of `NewBuffer`'s `readWindow` argument shift from "the
exact window to use" to "the minimum window the caller requires". The
buffer is then free to pick any window at least that large. Internally
it has its own private floor (an unexported `minReadWindow` constant),
and the actual `readWindow` it runs with is
`max(callerRequested, minReadWindow)`.

The floor's value is **3 s** for this change. The genuine internal
need is shorter (`detectInvalidLeap` needs the most recent 2 same-type
entries, ~2 s at 1 Hz; `getPulseCorrectionLast` looks back ~1 s for a
matching refTime), so 3 s is comfortable headroom for those consumers
while being a noticeable trim from the previous 5 s literal. A more
aggressive reduction is deferred to the follow-up step below.

This puts the responsibility for "below this, the buffer doesn't store
enough recent history to be useful" inside `timemsg`, where it
belongs. Callers say what they need; they do not have to know or
reproduce the package's internal lower bound. There is no exported
const for them to refer to: the package having a floor is an
implementation detail of how it serves the contract, not part of the
contract itself.

### Controller publishes its requirement

Add an exported method on `*phcsync.Controller`:

```go
// RequiredMsgWindow returns the minimum time-message buffer window the
// controller needs based on its current config.
func (c *Controller) RequiredMsgWindow() time.Duration {
    n := c.cfg.Reset.PulseWindow
    return time.Duration(n+2) * time.Second   // +2 s margin
}
```

The `+2 s` margin absorbs message-rate jitter so a `PulseWindow = N`
config reliably has `N` consecutive entries on hand at 1 Hz.

### Wire-up at the call sites

- [time/internal/gpsevent/dispatcher.go:55](time/internal/gpsevent/dispatcher.go#L55) --
  if `controller != nil`, pass `controller.RequiredMsgWindow()`;
  otherwise pass `0`. The serial-timing path (no controller) does not
  consult buffer history, so it has no minimum of its own to assert,
  and the package floor takes effect.
- [time/internal/gpsevent/event_log_replay.go:157](time/internal/gpsevent/event_log_replay.go#L157) --
  controller is already created before the buffer; pass
  `ctrl.RequiredMsgWindow()`.
- [time/internal/gpsevent/replay_test.go:30](time/internal/gpsevent/replay_test.go#L30) --
  same as event_log_replay.go.
- [time/internal/syncsim/syncsim.go:235](time/internal/syncsim/syncsim.go#L235) --
  buffer is currently created **before** the controller. Reorder:
  create the controller first (it does not need the buffer;
  `SetTimeMsgBuffer` is a separate call), then size the buffer from
  `ctrl.RequiredMsgWindow()`, then `ctrl.SetTimeMsgBuffer(buf)`.

After the change, no `5 * time.Second` literal remains in the
construction sites; the only places the value lives are the unexported
`minReadWindow` definition (now 3 s) inside `timemsg` and the `+2 s`
margin inside `RequiredMsgWindow`.

## Follow-up: revisit the floor

The 3 s floor is conservative. The serial-timing path -- the only
caller without a controller-derived requirement, and therefore the
only one for whom the floor is load-bearing -- reads no buffer
history at all: it consumes time messages via the `MsgUTCTimer`
callback fired synchronously from `Buffer.Time`, which uses only the
just-arrived message and a single dedup field on the buffer. So
`readWindow = 0` would, in principle, work for that path today.

A subsequent change should:

1. Audit every consumer of `Buffer` (the in-package callers and any
   external interface methods) and document, in the `timemsg`
   package doc, the minimum window each one needs.
2. Decide whether `minReadWindow` is still needed at all, or whether
   the package can simply trust callers to pass what they need (with
   the documented per-consumer minimums as the basis).
3. If a floor is retained, derive its value from the audit rather
   than picking a comfortable round number.

Doing the audit alongside the present fix risks scope creep and
couples the bug fix to a design decision; landing the conservative
3 s now and revisiting in isolation is cleaner.
