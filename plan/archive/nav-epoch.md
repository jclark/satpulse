# NavEpoch message

## Motivation

Navigation solutions arrive as groups of messages sharing a common epoch (e.g. the same iTOW in UBX). Downstream consumers currently have no way to know when all messages for a given epoch have been dispatched. An explicit epoch-boundary marker enables consumers to batch per-epoch processing and, in a future step, will carry solution metadata (fix quality, corrections, DOPs) synthesized from multiple messages within the epoch.

This plan adds an empty `NavEpochMsg` that marks the end of a navigation epoch. It contains only the `Tag` field (identifying the protocol). Metadata fields from the [solution-quality plan](solution-quality.md) will be added later. Cross-protocol epoch coordination (merging metadata from binary + NMEA protocols) is handled by the [multi-prot-nav-epoch plan](multi-prot-nav-epoch.md).

## Type definition

```go
// NavEpochMsg is emitted once at the end of each navigation epoch, after
// all time/position/velocity messages for that epoch have been dispatched.
// Future fields will carry solution metadata (fix quality, corrections, DOPs).
type NavEpochMsg struct {
	Tag Tag `json:"tag,omitzero"`
}
```

## Changes

### 1. Add `NavEpochMsg` and extend `MsgHandler`

In `gps/gpsprot/msg.go`:

- Add the `NavEpochMsg` struct (as above).
- Add `NavEpoch(msg *NavEpochMsg, tRead time.Time)` to the `MsgHandler` interface.
- Add the empty method to `DefaultHandler`.
- Add the fan-out method to `MultiHandler`.

Types that embed `DefaultHandler` will satisfy the expanded interface without changes:
- `time/internal/timemsg/timemsg.go`
- `time/internal/obs/observer.go`
- `time/internal/gpsevent/dispatcher.go` (embeds `DefaultHandler`, has explicit methods for others)
- `gps/app/gpscfg/gpscfg.go`
- `internal/gpscmd/replay_test.go`
- Various test handlers in `gps/internal/ubx/ubx_test.go`, `gps/internal/nmea/nmeasats_test.go`, `gps/internal/nmea/nmea_test.go`, `gps/internal/casic/cassats_test.go`, `time/internal/obs/observer_test.go`

Types that do NOT embed `DefaultHandler` and need an explicit method added:
- `time/internal/sseobs/sse.go` (`SSEObserver`)
- `time/internal/promobs/prometheus.go` (`PrometheusObserver`)

### 2. Emit `NavEpochMsg` from UBX `flushNavEpoch`

In `gps/internal/ubx/ubx.go`:

- In `flushNavEpoch`, after the existing `flushSats()` call, emit a `NavEpochMsg` via `p.mh.NavEpoch(...)`.
- Use `p.curNavEpochTStart` as the `tRead` argument.
- Guard against the initial state where no epoch has been seen yet (`curNavEpoch == 0`).

This is the only protocol that gets wired up in this plan. NMEA and Unicore will be added separately.

### 3. Add a test

In `gps/internal/ubx/ubx_test.go`:

- Add `NavEpoch` tracking to the existing `testMsgHandler` (record the messages and count).
- Add a test that feeds UBX NAV messages from two different epochs and verifies that a `NavEpochMsg` is emitted at the epoch boundary (when the second epoch's first message arrives), with `Tag == "UBX"`.
- Verify that no `NavEpochMsg` is emitted before the first epoch boundary.
