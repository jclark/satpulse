# Simplify NavEpoch merging

Related: [msg-bundle.md](msg-bundle.md).

## Problem

`MergeNavEpoch` is incremental and pairwise: it takes two `*NavEpochMsg` values with explicit `MsgPriority` parameters, returns a new merged value, and the caller threads the priority through repeated calls. This is more complex than needed since `NavEpochManager.flush` always merges all results at once (typically two: one NMEA, one vendor binary).

## Design

Change `MergeNavEpoch` to a one-shot variadic function with first-wins semantics:

```go
func MergeNavEpoch(msgs ...*NavEpochMsg) *NavEpochMsg
```

The caller orders arguments by priority (highest first). Priority is implicit in argument order, not explicit in the API. This is consistent with how PV Merge works (first of a given priority wins).

The function mutates and returns the first argument, filling in missing information from subsequent arguments. This is safe because `NavEpochManager` receives each `*NavEpochMsg` directly from a `PacketProcessor` via `FlushNavEpoch`.

Merge semantics:

- **Scalar fields** (Tag, FixLevel, FixDim): first non-zero value wins.
- **Optional fields** (Accuracy, DOP, DiffAge, RTCMRefBaseID, NumSVUsed, NumSVTracked): fill from later arguments using `opt.Val.Fill`.
- **Bitmask fields** (Correction, AuxSrc, SignalsUsed): union all.
- **StartTime**: earliest.

## Changes

### `gps/gpsprot`

- Replace `MergeNavEpoch(a *NavEpochMsg, aPri MsgPriority, b *NavEpochMsg, bPri MsgPriority) (*NavEpochMsg, MsgPriority)` with `MergeNavEpoch(msgs ...*NavEpochMsg) *NavEpochMsg`.
- Delete `mergeOpt` (no longer used after PV Merge changes in msg-bundle.md remove the other caller).
- `Accuracy.Merge` and `DOP.Merge` change to fill-only methods (no priority params), using `opt.Val.Fill`.

### `gps/gpsprot` (NavEpochManager)

- `NavEpochManager.flush` collects all non-nil results, sorts by `MsgPriority` descending (from `FlushNavEpoch`), and calls `MergeNavEpoch` once.
