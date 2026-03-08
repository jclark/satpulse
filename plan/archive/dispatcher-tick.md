# Centralize TimeTicker in Dispatcher

## Problem

The `TimeTicker` (which produces one filled `TimeMsg` per epoch) is currently owned privately by `SSEObserver`. The upcoming track log feature needs access to the filled `TimeMsg`, and any future observer that needs time would have to duplicate the same `TimeTicker` setup. This should be centralized in the Dispatcher, following the same pattern as PV accumulation.

## Design

### New `Tick` method on Observer

Add to `obs.Observer`:

```go
Tick(msg *gpsprot.TimeMsg, tRead time.Time)
```

Called once per epoch by the Dispatcher's `TimeTicker` with a filled `TimeMsg` (both TAI and UTC populated, rounded to millisecond). It fires as soon as the first valid `TimeMsg` arrives in the epoch. The `TimeTicker` depends on `NavEpoch` to reset its state for the next epoch.

Add no-op implementation to `DefaultObserver`. Add fan-out to `MultiObserver`.

### Dispatcher changes (`time/internal/gpsevent/dispatcher.go`)

1. Add `timeTicker gpsprot.TimeTicker` field to `Dispatcher`.
2. Initialize in `NewDispatcher` with `NewTimeTicker(handler, d.ls)` — using the Dispatcher's existing leap second — and a downstream handler that calls `d.obs.Tick(msg, tRead)`.
3. In `Dispatcher.Time()`: also route to `d.timeTicker.Time(msg, tRead)`.
4. In `Dispatcher.NavEpoch()`: also call `d.timeTicker.NavEpoch(msg, tRead)`.
5. In `Dispatcher.LeapSecond()`: also call `d.timeTicker.LeapSecond(msg, tRead)`.

The downstream handler can be a small unexported type (e.g. `tickHandler`) with a pointer back to the Dispatcher, or a closure — whichever is simpler.

### SSEObserver changes (`time/internal/sseobs/sse.go`)

1. Remove `timeTicker` field.
2. Remove `timeHandler` type.
3. Remove `Time()` method (raw TimeMsg no longer needed — `DefaultHandler` provides the no-op).
4. Add `Tick()` method containing the logic currently in `timeHandler.Time()`: check integral second, construct `TimeSSE`, send.
5. Remove `timeTicker.NavEpoch()` call from `NavEpochPV()`.
6. Remove `timeTicker.LeapSecond()` call from `LeapSecond()`.
7. Remove `SetLeapSecond()` method (the Dispatcher's TimeTicker now owns the leap second). The `ls` field stays if still needed for `FormatTime` in `TimeSSE` construction.

## Changes by file

| File | Change |
|------|--------|
| `time/internal/obs/observer.go` | Add `Tick` to `Observer` interface, `DefaultObserver`, `MultiObserver` |
| `time/internal/obs/observer_test.go` | Add `Tick` to `mockObserver`; add `TestMultiObserver_Tick` |
| `time/internal/gpsevent/dispatcher.go` | Add `timeTicker` field; route `Time`, `NavEpoch`, `LeapSecond` to it; add tick handler |
| `time/internal/sseobs/sse.go` | Remove private `TimeTicker`; replace `Time()` with `Tick()`; remove `timeHandler` |
| `time/internal/sseobs/sse_test.go` | Update `TestSSEObserver_Events` to deliver `Tick` instead of raw `Time` |
