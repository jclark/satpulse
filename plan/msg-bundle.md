# Replace MsgBundle with Msg interface and PVMsgBundle

Supersedes the `MsgBundle` design from [nmea-extensibility.md](nmea-extensibility.md).
Related: [nav-epoch-accum.md](nav-epoch-accum.md), [nav-epoch-merge.md](nav-epoch-merge.md).

## Problem

`MsgBundle` is a struct with one pointer field per message type. It serves two roles:

1. **Return type from handlers**: `ExtSentenceHandler.HandleSentence` and `parseRMC` return `*MsgBundle` to carry messages produced from a single sentence.
2. **Epoch accumulation**: `NavEpochAccum` holds a `MsgBundle` to accumulate the best PosGeo/PosECEF/VelGeo/VelECEF/Time per epoch.

Both roles have problems:

- **Unwieldy**: every new message type requires a new field, a case in `Dispatch`, and a case in `SetPriority`. The struct grows linearly with message types.
- **Time accumulation is wrong**: `TimeMsg` has a `Ref` field (`NavSolution`, `PrePulse`, `PostPulse`). Merging across different Ref types within an epoch is semantically incorrect.
- **Priority is too broad**: `SetPriority` sets priority on all five types including `TimeMsg`, but only the four pos/vel types are merged by the accumulator.
- **Mutation breaks MultiMsgHandler**: `NavEpochAccum` stores message pointers from `MsgBundle` and calls `Merge` on them in place. When `MultiMsgHandler` dispatches the same message pointer to multiple handlers, the accumulator in one handler mutates the message seen by others. `PVMsgBundle` uses `opt.Val` (values, not pointers), so each accumulator stores its own copy.

## Design

### 1. `Msg` interface

```go
// Msg is implemented by all protocol-agnostic message types.
type Msg interface {
    Dispatch(MsgHandler, time.Time)
}
```

Each message type implements `Dispatch` by calling the corresponding `MsgHandler` method:

```go
func (m *PosGeoMsg) Dispatch(h MsgHandler, t time.Time)    { h.PosGeo(m, t) }
func (m *PosECEFMsg) Dispatch(h MsgHandler, t time.Time)   { h.PosECEF(m, t) }
func (m *VelGeoMsg) Dispatch(h MsgHandler, t time.Time)    { h.VelGeo(m, t) }
func (m *VelECEFMsg) Dispatch(h MsgHandler, t time.Time)   { h.VelECEF(m, t) }
func (m *TimeMsg) Dispatch(h MsgHandler, t time.Time)      { h.Time(m, t) }
func (m *LeapSecondMsg) Dispatch(h MsgHandler, t time.Time) { h.LeapSecond(m, t) }
func (m *SurveyMsg) Dispatch(h MsgHandler, t time.Time)    { h.Survey(m, t) }
func (m *SatellitesMsg) Dispatch(h MsgHandler, t time.Time) { h.Satellites(m, t) }
```

All `*Msg` types implement it: `*TimeMsg`, `*PosGeoMsg`, `*PosECEFMsg`, `*VelGeoMsg`, `*VelECEFMsg`, `*LeapSecondMsg`, `*SurveyMsg`, `*SatellitesMsg`.

### 2. `PVMsg` interface

Only the four pos/vel message types that are accumulated per-epoch need priority:

```go
// PVMsg is implemented by position/velocity message types
// that participate in per-epoch accumulation with priority-based merging.
type PVMsg interface {
    Msg
    SetPriority(MsgPriority)
}
```

Implemented by `*PosGeoMsg`, `*PosECEFMsg`, `*VelGeoMsg`, `*VelECEFMsg`.

`TimeMsg` loses its `Priority` field since it is no longer merged.

### 3. Replace `MsgBundle` return type with `[]Msg`

`ExtSentenceHandler.HandleSentence` changes from returning `*MsgBundle` to `[]Msg`:

```go
HandleSentence(flags nmeamsg.SentenceSyntaxFlags, payload string, epoch *NavEpoch) ([]Msg, *NavEpoch, error)
```

Similarly, `parseRMC` returns `[]Msg`.

The current `*MsgBundle` uses nil vs non-nil-but-empty to distinguish "not handled" from "handled with no messages". With `[]Msg` that distinction is fragile (nil vs empty slice). Instead, a sentinel error `gpsprot.ErrNotHandled` signals that the handler did not recognize the sentence. The return value semantics become clean:

- **Non-nil error** (including `ErrNotHandled`): ignore the other return values.
- **Nil error**: both return values are meaningful and handled uniformly -- iterate `msgs` (empty/nil is fine), set `epoch` (nil is fine).

```go
msgs, epoch, err := eh.HandleSentence(...)
if err != nil {
    if errors.Is(err, gpsprot.ErrNotHandled) {
        continue
    }
    return msgID, err
}
p.handleEpoch(epoch, tRead)
gpsprot.DispatchMsgs(msgs, h, tRead)
```

The caller dispatches via the `Msg.Dispatch` method -- no type switch needed:

```go
func DispatchMsgs(msgs []Msg, h MsgHandler, tRead time.Time) {
    for _, m := range msgs {
        m.Dispatch(h, tRead)
    }
}
```

Adding a new message type only requires implementing `Dispatch` on that type -- no central switch to maintain.

`SetPriority` on a `[]Msg` uses the `PVMsg` interface:

```go
func SetMsgsPriority(msgs []Msg, pri MsgPriority) {
    for _, m := range msgs {
        if pv, ok := m.(PVMsg); ok {
            pv.SetPriority(pri)
        }
    }
}
```

### 4. `PVMsgBundle` and `PVMsgAccum`

```go
// PVMsgBundle holds the accumulated position/velocity messages
// for a single navigation epoch.
type PVMsgBundle struct {
    PosGeo  opt.Val[PosGeoMsg]
    PosECEF opt.Val[PosECEFMsg]
    VelGeo  opt.Val[VelGeoMsg]
    VelECEF opt.Val[VelECEFMsg]
}
```

`PVMsgAccum` replaces `NavEpochAccum`. It embeds `PVMsgBundle` and implements `MsgHandler` for the four pos/vel methods, merging by priority. Its `NavEpoch` method clears the bundle.

```go
type PVMsgAccum struct {
    DefaultHandler
    PVMsgBundle
}
```

The accumulator methods use `opt.Val.Ptr()` (new method on `opt.Val` -- returns `*T` if set, nil if unset) to merge in place without a Get/Set round-trip:

```go
// Ptr returns a pointer to the stored value, or nil if unset.
func (v *Val[T]) Ptr() *T {
    if !v.set {
        return nil
    }
    return &v.val
}
```

Each accumulator method either stores the first message (via `Set`) or merges into the existing value (via `Ptr`):

```go
func (a *PVMsgAccum) PosGeo(msg *PosGeoMsg, _ time.Time) {
    if p := a.PosGeo.Ptr(); p != nil {
        p.Merge(msg)
    } else {
        a.PosGeo.Set(*msg)
    }
}
```

`Set(*msg)` copies the value in, so each accumulator has its own copy -- no shared mutation across `MultiMsgHandler` consumers. `Ptr()` then allows `Merge` to mutate through the pointer efficiently. The same pattern applies to all four methods (`PosGeo`, `PosECEF`, `VelGeo`, `VelECEF`).

`NavEpoch` clears the embedded bundle:

```go
func (a *PVMsgAccum) NavEpoch(_ *NavEpochMsg, _ time.Time) {
    a.PVMsgBundle = PVMsgBundle{}
}
```

### 5. `PVMsgBundle.FillDerived`

A method on `PVMsgBundle` that fills in missing fields using `geopos`:

- PosGeo set, PosECEF missing: compute ECEF from LLH via `geopos.WGS84.LLHtoECEF` (requires Height)
- PosECEF set, PosGeo missing: compute LLH from ECEF via `geopos.WGS84.ECEFtoLLH`
- VelNED set, VelECEF missing (+ position available): `geopos.WGS84.NEDtoECEF`
- VelECEF set, VelNED missing (+ position available): `geopos.WGS84.ECEFtoNED`
- VelNED set, Speed3D missing: compute from `sqrt(n^2 + e^2 + d^2)`

This centralises cross-frame derivation that is currently duplicated in consumers.

Note: `gpsprot` gains a dependency on `geopos`, which is a leaf math package with no further dependencies.

## Changes by package

### `gps/lib/opt`

- Add `Val[T].Ptr() *T` method: returns `&v.val` if set, nil if unset.
- Add `Val[T].Fill(src Val[T])` method: sets `v` to `src` if `v` is not already set.

### `gps/gpsprot`

- Change all four PV Merge methods to first-wins-at-equal-priority semantics. Replace `>=` with a three-way split: `sp > dp` copies everything (`*m = *other`), `sp == dp` fills only missing optional fields (using `opt.Val.Fill`), `sp < dp` does nothing. This ensures all fields in a merged message come from the same priority band. `mergeOpt` stays (still used by `MergeNavEpoch`; deleted by nav-epoch-merge.md).
- Add `Msg` interface (`Dispatch`).
- Add `PVMsg` interface (`SetPriority`).
- Add `ErrNotHandled` sentinel error.
- Add `Dispatch` method to all `*Msg` types.
- Add `SetPriority` method to the four pos/vel types (move from struct field assignment to method).
- Remove `Priority` field and `Merge` method from `TimeMsg`.
- Add `PVMsgBundle` struct.
- Add `PVMsgBundle.FillDerived()`.
- Add `PVMsgAccum` (replaces `NavEpochAccum`).
- Add `DispatchMsgs([]Msg, MsgHandler, time.Time)` helper.
- Add `SetMsgsPriority([]Msg, MsgPriority)` helper.
- Delete `MsgBundle`, its `Dispatch` method, and its `SetPriority` method.
- Delete `NavEpochAccum`.

### `gps/internal/nmea`

- Change `ExtSentenceHandler.HandleSentence` to return `[]gpsprot.Msg`.
- Change `parseRMC` to return `[]gpsprot.Msg`.
- Replace `bundle.Dispatch(h, tRead)` calls with `gpsprot.DispatchMsgs(msgs, h, tRead)`.
- Replace `bundle.SetPriority(pri)` calls with `gpsprot.SetMsgsPriority(msgs, pri)`.
- Update tests (`mockExtHandler`, `epochMockExtHandler`).

### `gps/internal/quectel`

- Change `HandleSentence` to return `[]gpsprot.Msg`.
- Replace `msgBundlePVT`, `msgBundleNAV`, `msgBundleVEL`, `msgBundleSVIN` helpers to return `[]gpsprot.Msg` (append messages to a slice instead of setting struct fields).
- Replace `b.SetPriority(pri)` with `gpsprot.SetMsgsPriority(msgs, pri)`.

### `time/internal/sseobs`

- Change `SSEObserver` to embed `PVMsgAccum` instead of `NavEpochAccum`.
- Change `buildPosVelSSE` to take `*PVMsgBundle`. Use `opt.Val` accessors (`IsSet`/`Get`) instead of nil checks. Access via embedded field (e.g. `o.PVMsgBundle`). Can call `FillDerived` if cross-frame fields are needed.
- Update tests to construct `PVMsgBundle` instead of `MsgBundle`.

## Desktop app

This refactoring enables the desktop app (`satpulse-desktop`) to eliminate its `EpochPVT`, `EpochPos`, `EpochVel`, and `EpochTime` types. Since `Angle`, `Length`, and `Speed` now have JSON marshalling, `PVMsgBundle` can be emitted directly as the `gps:epochPVT` event payload after calling `FillDerived`. The ~100-line `buildEpochPVT` function and all four `Epoch*` type definitions are replaced by a single `FillDerived` call. `EpochTime` is dropped entirely (the frontend does not use it).
