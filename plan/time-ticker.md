# TimeTicker: one filled TimeMsg per epoch

Related: [msg-bundle.md](msg-bundle.md), [webui.md](webui.md) (phase A3).

## Problem

The current SSE time event path (`SSEObserver.Time`) has two problems:

1. **Premature emission.** It emits immediately on each valid `TimeMsg`, rounding to the nearest second. At sub-second PVT rates (e.g. 10Hz), a NavSolution at 09:00:00.6 rounds to 09:00:01 -- a second that hasn't happened yet. This was implemented before the NavEpoch concept existed.

2. **No reusable filled TimeMsg.** The computation (leap second conversion, rounding, dedup) is done inline in `SSEObserver.Time` and produces a `TimeSSE`. There is no reusable component that produces a filled `TimeMsg` with both TAI and UTC populated. The desktop GUI needs this for its `gps:time` Wails event, and the webui.md plan (phase A3) will need it when `TimeSSE` is eventually replaced by direct `TimeMsg` serialisation.

## Design

### TimeTicker

A `TimeTicker` in `gpsprot` produces one filled-in `TimeMsg` per navigation epoch, aligned with the NavEpoch boundary. It implements `MsgHandler` for `Time`, `LeapSecond`, and `NavEpoch`.

```go
type TimeTicker struct {
    DefaultHandler
    h    MsgHandler
    time opt.Val[TimeMsg]
    ls   ptime.LeapSecond
}
```

The `h` field is the downstream handler to which filled `TimeMsg`s are forwarded. It is set at construction time.

**`LeapSecond(msg *LeapSecondMsg, _ time.Time)`**: updates `ls` via `msg.UpdateLeapSecond(&t.ls)`.

**`Time(msg *TimeMsg, tRead time.Time)`**: filters, fills, and forwards one `TimeMsg` per epoch:

1. Filter: skip if `msg.Ref == PrePulse`.
2. Skip if a time is already stored this epoch (`t.time.IsSet()`).
3. Create a new `TimeMsg` (copy from `*msg`, not a mutation).
4. Fill derived fields on the copy (see below).
5. Store via `t.time.Set(copy)`.
6. Forward immediately: `t.h.Time(&copy, tRead)`.

Forwarding happens immediately because applications need the current time as soon as possible. The one-per-epoch guarantee comes from the `t.time.IsSet()` gate in step 2.

**`NavEpoch(msg *NavEpochMsg, _ time.Time)`**: clears: `t.time = opt.Val[TimeMsg]{}`, preparing for the next epoch.

### Fill logic

`TimeTicker` fills in missing fields on the copy using its stored leap second. This is a private method on `TimeTicker` (or inline in `TimeTicker.Time`), not a method on `TimeMsg` — it depends on the external leap second state.

Three fills, each conditional:

1. **UTCOffset**: fill from `ls.StateAt(taiTime).UTCOffset` only if currently zero. Trust the receiver's value when present.
2. **TAITime**: round to millisecond if set. If zero, compute from `ls.UTCtoTime(*m.UTCTime)` (already rounded in step 3).
3. **UTCTime**: round `TimeOfDay` to millisecond if set. If nil, compute from TAITime using the leap second.

Rounding to millisecond aligns with nominal PVT rates (always a factor of 1000ms). This avoids the premature-rounding problem: a NavSolution at 999795255ns rounds to 1000ms = 1.000s (correct), while one at 600ms rounds to 600ms (not an integral second).

Order of operations:

1. Round TAITime to millisecond (if set).
2. Round UTCTime.TimeOfDay to millisecond (if set).
3. If TAITime is zero and UTCTime is set: `TAITime = ls.UTCtoTime(*UTCTime)`.
4. If UTCTime is nil and TAITime is set: compute UTCTime from TAITime using the leap second.
5. Fill UTCOffset from `ls.StateAt(TAITime)` if zero. (Must come after step 3 so TAITime is available.)

After filling, the `TimeMsg` is self-contained: TAITime, UTCTime, and UTCOffset are all populated. Any consumer can serialise it directly. The frontend reads whichever fields it needs without leap second arithmetic.

### Consumers

The downstream `MsgHandler` passed to `TimeTicker` receives filled `TimeMsg`s via its `Time` method. The consumer's `Time` handler is called at most once per epoch, with a fully populated `TimeMsg`.

**SSE backend** (`sseobs`): creates `TimeTicker` with a `timeHandler` adapter as the downstream handler. `SSEObserver.Time` routes raw `TimeMsg`s into the `TimeTicker`. The `timeHandler.Time` method receives filled `TimeMsg`s and checks whether the time is an integral second (`taiTime.Round(time.Second) == taiTime`); if so, constructs a `TimeSSE` and sends it. At 1Hz every epoch produces a time event; at 10Hz only 1 in 10 does. This replaces the current `SSEObserver.Time` handler (which did inline filtering/computation) and the `lastTime` deduplication field. The `TimeSSE` intermediary type remains for now; it is eliminated later by [webui.md](webui.md) phase A3.

**Desktop GUI** (`desktop/app.go`): creates `TimeTicker` with its handler. The handler's `Time` method emits the filled `TimeMsg` as the `gps:time` Wails event.

## Changes by package

### `gps/gpsprot`

- Add `TimeTicker` struct with `DefaultHandler`, `MsgHandler`, `opt.Val[TimeMsg]`, `ptime.LeapSecond`.
- Add constructor: `NewTimeTicker(h MsgHandler, ls ptime.LeapSecond) *TimeTicker`.
- Add `TimeTicker.Time`, `TimeTicker.LeapSecond`, `TimeTicker.NavEpoch` methods.

### `time/internal/sseobs`

- Create `TimeTicker` with a `timeHandler` adapter as downstream handler (not embedded -- `TimeTicker` is a separate stage that feeds into the observer).
- `SSEObserver.Time` routes raw `TimeMsg`s through the `TimeTicker`. A `timeHandler` adapter receives filled `TimeMsg`s, checks integral second, constructs and sends `TimeSSE`.
- Remove `lastTime` field -- deduplication is now implicit (one per epoch, integral-second check).
- `ls` field may still be needed for `FormatTime` in `TimeSSE` construction and for survey.
- `TimeSSE` struct remains; removed later by [webui.md](webui.md) phase A3.

### Desktop GUI (future)

- Create `TimeTicker` with the app's handler as downstream.
- The handler's `Time` method emits the filled `TimeMsg` as `gps:time` Wails event.
