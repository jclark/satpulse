# ExtSentenceHandler epoch support

Follows: [nmea-extensibility.md](nmea-extensibility.md) (defines `ExtSentenceHandler`, `MsgBundle`, Quectel handler).

Enables: [nmea-pos-vel-epoch.md](nmea-pos-vel-epoch.md) (NMEA position/velocity extraction and epoch detection).

## Problem

The current `ExtSentenceHandler` interface is:

```go
type ExtSentenceHandler interface {
    HandleSentence(flags nmeamsg.SentenceSyntaxFlags, payload string, epoch *gpsprot.NavEpochMsg) (
        bundle *gpsprot.MsgBundle, eoe bool,
    )
}
```

The processor passes `&p.navEpoch` (a value-type field, always non-nil) and the handler writes accuracy fields directly into it. This has two problems:

1. **No epoch boundary signalling.** The handler can say "end of epoch" via `eoe`, but cannot say "this message starts a new epoch". For standard NMEA, epoch boundaries are detected by time-of-day changes in RMC/GGA. PQTM messages (PVT, NAV, VEL) also carry time-of-day, but the current interface gives the handler no way to signal a boundary.

2. **Write-before-boundary ordering.** The handler receives `*NavEpochMsg` and writes accuracy into it *before* the processor can detect that a new epoch has started. If a PQTM message from epoch N+1 arrives while epoch N is current, the handler writes N+1's accuracy into epoch N's `NavEpochMsg`. The processor then flushes, emitting epoch N with wrong data.

The handler must continue writing directly to `*NavEpochMsg` (not return accuracy through a separate struct), because `NavEpochMsg` will grow to include fix quality, DOPs, corrections, and other metadata. Duplicating those fields in a return struct does not scale.

The core constraint: the handler needs to write to the correct epoch's `NavEpochMsg`, which means epoch boundaries must be resolved before the handler writes. But only the handler knows whether its message's time-of-day differs from the current epoch's.

## Design

### NavEpoch struct

```go
// NavEpoch tracks epoch state. It embeds the NavEpochMsg that will be
// emitted at the end of the epoch, plus a TimeOfDay field for boundary
// detection. Exported because ExtSentenceHandler implementations (in
// other packages) receive and return it.
type NavEpoch struct {
    gpsprot.NavEpochMsg
    TimeOfDay string // UTC time-of-day string from the sentence; "" means no time yet
}
```

The zero value of `TimeOfDay` (`""`) means "no time-of-day yet" — this handles the case where a message without time (e.g. EPE) starts an epoch before any timed message arrives.

### Revised interface

```go
type ExtSentenceHandler interface {
    HandleSentence(flags nmeamsg.SentenceSyntaxFlags, payload string, epoch *NavEpoch) (
        *gpsprot.MsgBundle, *NavEpoch, error,
    )
}
```

The handler receives the current `*NavEpoch` (which may be nil if no epoch is in progress) and returns the epoch that should be current after this message. The handler decides whether to start a new epoch based on its message's time-of-day vs `epoch.TimeOfDay`. The error return allows handlers to report parse failures for recognized sentences (e.g. `qtmmsg.ParsePeriodicMsg` version mismatch or field decode errors).

Return value semantics:
- `(nil, nil, nil)` — not handled.
- `(nil, nil, err)` — recognized but parse failed.
- `(bundle, sameEpoch, nil)` — handled; message belongs to the current epoch.
- `(bundle, newEpoch, nil)` — handled; message starts a new epoch.
- `(bundle, nil, nil)` — handled; end of epoch, flush.

### CheckEpoch helper

A shared helper called by messages that carry a time-of-day and want to participate in the epoch. Both ext handlers and built-in NMEA parsers (in nmea-pos-vel-epoch.md) call this.

```go
// CheckEpoch is called by a message handler that participates in the
// epoch. tod is the message's time-of-day, or "" if the message has
// none. If the time-of-day matches the current epoch, it returns the
// same epoch. If the epoch has no time-of-day yet, it sets it.
// Otherwise (nil epoch or time-of-day mismatch), it allocates a new
// epoch.
func CheckEpoch(epoch *NavEpoch, tod string) *NavEpoch {
    if epoch != nil {
        if tod == "" || epoch.TimeOfDay == tod {
            return epoch
        }
        if epoch.TimeOfDay == "" {
            epoch.TimeOfDay = tod
            return epoch
        }
    }
    return &NavEpoch{TimeOfDay: tod}
}
```

Messages with time-of-day (PVT, NAV, VEL) pass it; messages without (EPE, DOP, SVINStatus) pass `""`. A handler for a message that has nothing to do with epochs would not call `CheckEpoch`, but all current Quectel messages participate in epochs.

### Epoch lifecycle on PacketProcessor

Replace `navEpoch gpsprot.NavEpochMsg` (value type) with `curNavEpoch *NavEpoch` (pointer, nil when no epoch in progress).

```go
func (p *PacketProcessor) handleEpoch(epoch *NavEpoch, tRead time.Time) {
    if epoch == nil {
        // EOE: flush current epoch
        p.flushEpoch()
        return
    }
    if epoch != p.curNavEpoch {
        // New epoch: flush old, install new
        p.flushEpoch()
        epoch.StartTime = tRead
        p.curNavEpoch = epoch
    }
}

func (p *PacketProcessor) flushEpoch() {
    if p.curNavEpoch != nil && p.mh != nil {
        p.curNavEpoch.Tag = Tag
        p.mh.NavEpoch(&p.curNavEpoch.NavEpochMsg, p.curNavEpoch.StartTime)
    }
    p.curNavEpoch = nil
}
```

The ext handler loop in `ProcessPacket` becomes:

```go
for _, eh := range p.extHandlers {
    bundle, epoch, err := eh.HandleSentence(sen.SyntaxFlags, sen.Payload, p.curNavEpoch)
    // Handler returned nothing useful.
    if bundle == nil && epoch == nil {
        if err != nil {
            return msgID, err
        }
        continue
    }
    // Handler participated in the epoch.
    p.handleEpoch(epoch, tRead)
    if bundle != nil {
        bundle.Dispatch(p.mh, tRead)
        return msgID, nil
    }
}
```

### Quectel handler adaptation

`HandleSentence` takes `*NavEpoch` and returns `(*MsgBundle, *NavEpoch, error)`:

The error from `qtmmsg.ParsePeriodicMsg` is propagated (not discarded). All messages call `CheckEpoch`: messages with time-of-day (PVT, NAV, VEL, EOE) pass it; messages without (EPE, DOP, SVINStatus) pass `""`. After `CheckEpoch`, messages that contribute epoch metadata (accuracy from NAV, VEL, EPE; DOPs from DOP) write to the resulting epoch. EOE returns `(bundle, nil, nil)`.

The internal helpers (`msgBundlePVT`, `msgBundleNAV`, `msgBundleVEL`, `accEPE`) change their `epoch` parameter from `*gpsprot.NavEpochMsg` to `*NavEpoch` (or `*gpsprot.NavEpochMsg` obtained from the `NavEpoch`).

## Implementation steps

### Step 1: NavEpoch, CheckEpoch, interface, epoch lifecycle

Add `NavEpoch` struct and `CheckEpoch` to `nmea.go`. Change the `ExtSentenceHandler` interface (including error return). Replace `navEpoch gpsprot.NavEpochMsg` with `curNavEpoch *NavEpoch` on `PacketProcessor`. Add `handleEpoch` and `flushEpoch`. Update the ext handler loop in `ProcessPacket` to propagate errors.

Update `mockExtHandler` in `nmea_test.go` to implement the new interface.

Run `make test`.

### Step 2: Adapt Quectel handler

Update `gps/internal/quectel/handler.go`: change `HandleSentence` signature, propagate `ParsePeriodicMsg` error, call `CheckEpoch` for all messages (passing time-of-day or `""`), return `(*MsgBundle, *NavEpoch, error)`. Update helpers to write accuracy via the `NavEpoch`.

Update `gps/internal/quectel/handler_test.go`: tests pass `*NavEpoch` and check accuracy on the returned epoch.

Run `make test`.

### Hook for solution-quality.md

Parsers and handlers receive `*NavEpoch` which embeds `NavEpochMsg`. When solution-quality.md adds fix level, DOPs, and corrections, those values go into the embedded `NavEpochMsg`.

### Hook for multi-prot-nav-epoch.md

When `NavEpochManager` is implemented, the flush path is refactored: the NMEA processor calls `manager.EpochStarted(p, tRead)` on epoch change and implements `FlushNavEpoch() *NavEpochMsg`.
