# Cross-protocol navigation epochs

Prerequisite: [nav-epoch.md](nav-epoch.md) (adds `NavEpochMsg` and `MsgHandler.NavEpoch`), [nav-epoch-accum.md](nav-epoch-accum.md) section 1 (defines `MsgPriority` and priority-based merge).

Related: [solution-quality.md](solution-quality.md) (populates `NavEpochMsg` fields), [nmea-extensibility.md](nmea-extensibility.md) (extended NMEA sentence handlers), [nmea-ext-handler-epoch.md](nmea-ext-handler-epoch.md) (epoch lifecycle on NMEA `PacketProcessor`).

## Problem

Navigation solutions arrive as groups of messages sharing a common epoch. A single receiver may output both a binary protocol (UBX, Allystar, Unicore) and standard NMEA simultaneously. Each protocol carries different subsets of the solution metadata:

- **Binary protocols** typically provide accuracy estimates, DOPs, and (for some) fix quality/carrier solution status.
- **NMEA** provides fix quality (GGA quality indicator), fix dimensionality (GSA fix type), DOPs (GSA PDOP/HDOP/VDOP), correction indicators (RMC mode), and richer satellite data (per-signal GSV).

For example, Allystar binary provides position, velocity, time, accuracy, and DOPs, but has no fix quality or fix type indicator in its currently implemented messages. NMEA GGA/GSA fills that gap.

The nav-epoch.md plan has each protocol's `PacketProcessor` independently emitting its own `NavEpochMsg`. This means downstream consumers receive multiple `NavEpochMsg` values per physical navigation epoch, each with partial metadata. This defeats the purpose of `NavEpochMsg` as the single point where applications can react to a complete epoch with all available metadata.

The real-world multi-protocol scenarios are always **one binary protocol + NMEA** on the same receiver. Two binary protocols on the same receiver is not a practical case. The Quectel extended-NMEA case (PQTM sentences alongside standard NMEA) is handled within a single `PacketProcessor` by the nmea-extensibility plan.

## Design: NavEpochManager

A `NavEpochManager` coordinates epoch handling across protocol processors. It is a shared object, passed to each `PacketProcessor`, that:

1. Tracks which processors are actively participating in epochs.
2. Triggers cross-protocol flushes when any protocol detects an epoch boundary.
3. Merges per-protocol `NavEpochMsg` contributions into a single emission.

### Interface

```go
// EpochFlusher is implemented by PacketProcessors that participate in
// epoch coordination. The manager calls FlushNavEpoch when an epoch
// boundary is detected. The processor returns its accumulated NavEpochMsg
// (or nil if it has nothing to contribute), a MsgPriority indicating
// the protocol band (generic for NMEA, vendor for binary/PQTM), and
// its MsgHandler for emission.
//
// Invariant: all participating processors must share the same MsgHandler
// (set via SetAllMsgHandlers). The manager picks the MsgHandler from an
// arbitrary non-nil contributor to emit the merged NavEpochMsg.
type EpochFlusher interface {
    FlushNavEpoch(tRead time.Time) (*NavEpochMsg, MsgPriority, MsgHandler)
}

// NavEpochManager coordinates navigation epoch handling across multiple
// protocol processors. Each PacketProcessor receives a reference to the
// shared manager in its constructor instead of directly calling
// MsgHandler.NavEpoch.
type NavEpochManager struct {
    active map[EpochFlusher]struct{} // processors that have reported epochs
}
```

### Protocol interaction

Each `PacketProcessor` that participates in epochs receives a `*NavEpochManager` in its constructor. The processor calls a single method:

```go
manager.EpochStarted(flusher EpochFlusher, tRead time.Time)
```

A processor calls `EpochStarted` every time it detects the start of a new epoch (iTOW change for binary protocols, time-of-day change in RMC/GGA for NMEA). The processor should call `EpochStarted` *before* it starts accumulating data for the new epoch.

The manager's behavior:

1. If the caller is already in the active set, this means the caller's previous epoch is complete and a new one is starting. The manager flushes: it calls `FlushNavEpoch(tRead)` on every processor in the active set, merges the non-nil results, takes the `MsgHandler` from any non-nil contributor, and emits a single `NavEpochMsg` via that handler's `NavEpoch` method. All processors share the same `MsgHandler`, so it doesn't matter which one is used. The active set is then cleared.
2. The caller is added to the (now possibly empty) active set.

This means a processor's first epoch is never flushed eagerly -- the manager doesn't know it's complete until the processor calls `EpochStarted` again. This matches the existing iTOW-change approach. In multi-protocol mode, whichever protocol's next epoch starts first triggers the flush for all protocols.

### End-of-epoch messages

Explicit end-of-epoch messages (UBX NAV-EOE, Quectel PQTMEOE) can trigger immediate flush. The processor signals this via:

```go
manager.EndOfEpoch(tRead time.Time)
```

An end-of-epoch message marks the end of the epoch for all protocols on that receiver, not just the protocol that carries it. UBX NAV-EOE is documented as being output after all NAV and NMEA messages for the epoch. PQTMEOE similarly marks the end of all NMEA output (standard and PQTM extended) for the epoch, and the Quectel LG290P has no binary protocol, so it covers everything.

The manager therefore flushes unconditionally: it calls `FlushNavEpoch(tRead)` on every processor in the active set, merges the non-nil results, emits the merged `NavEpochMsg`, and clears the active set. This avoids the one-epoch latency of waiting for the next epoch to start.

### Active set lifecycle

The active set is transient -- it is built up during an epoch and cleared on flush. This means:

- At startup, the active set is empty.
- As protocols start producing epochs, they add themselves via `EpochStarted`.
- On flush, the active set is cleared. Protocols re-register on their next epoch.
- A protocol that stops producing epochs (e.g. NMEA gets disabled) simply stops calling `EpochStarted` and naturally drops out of subsequent epochs.

### Merge rules

NMEA processors return a generic-band priority (`PriGenericHigh`); vendor-specific protocol processors return a vendor-band priority (`PriVendorLow`). Since `PriGenericHigh` (2) < `PriVendorLow` (3), binary protocol fields always take precedence when both set the same field.

When multiple protocols contribute to the same epoch, the merge produces a single `NavEpochMsg`:

- **`Accuracy` fields**: merged using the existing `mergeOpt` pattern. Higher priority overwrites; lower priority fills unset fields only. This requires a new `Accuracy.Merge` method.
- **`Tag`**: from the higher-priority contributor (i.e. the binary protocol).
- **`StartTime`**: the earliest `StartTime` across all contributors (the first message read in the physical epoch, regardless of protocol).

```go
// Merge incorporates fields from other into a based on priority.
func (a *Accuracy) Merge(other *Accuracy, dstPri, srcPri MsgPriority) {
    mergeOpt(&a.Pos, &other.Pos, dstPri, srcPri)
    mergeOpt(&a.Hor, &other.Hor, dstPri, srcPri)
    mergeOpt(&a.Vert, &other.Vert, dstPri, srcPri)
    mergeOpt(&a.Speed, &other.Speed, dstPri, srcPri)
    mergeOpt(&a.GroundSpeed, &other.GroundSpeed, dstPri, srcPri)
    mergeOpt(&a.Course, &other.Course, dstPri, srcPri)
}
```

The manager's flush iterates its active set, collects non-nil `NavEpochMsg` results, and merges them pairwise. `MergeNavEpoch` takes two `NavEpochMsg`/priority pairs and returns the merged result:

```go
// MergeNavEpoch merges two NavEpochMsg values by priority. The higher-priority
// message provides Tag; Accuracy fields are merged with mergeOpt semantics;
// StartTime is the earliest.
func MergeNavEpoch(a *NavEpochMsg, aPri MsgPriority, b *NavEpochMsg, bPri MsgPriority) (*NavEpochMsg, MsgPriority) {
    if a == nil {
        return b, bPri
    }
    if b == nil {
        return a, aPri
    }
    merged := *a
    merged.Acc.Merge(&b.Acc, aPri, bPri)
    if bPri >= aPri {
        merged.Tag = b.Tag
        aPri = bPri
    }
    if b.StartTime.Before(merged.StartTime) {
        merged.StartTime = b.StartTime
    }
    return &merged, aPri
}
```

**Equal-priority merge order**: when two contributors have the same priority, the `>=` tie-break in `MergeNavEpoch` makes the `Tag` result depend on iteration order of the active set (a `map`), which is nondeterministic. In the designed usage this cannot happen: the only multi-protocol case is one binary protocol (`PriVendorLow`) + NMEA (`PriGenericHigh`), which have different priorities. There is one known edge case: Unicore UM980 has an undocumented ability to emit NovAtel-format messages, which would produce two vendor-priority contributors. If users enable this, which `Tag` gets selected in the merge is nondeterministic, which is acceptable. Note: #132 (adding `Tag` to `ProcessPacket` so related formats like Unicore binary/ASCII can share a single `PacketProcessor`) would reduce the number of active processors but does not affect the equal-priority case, which involves genuinely different vendors' protocols.

### Epoch identity and correlation

In multi-protocol mode, the manager does not attempt to correlate epoch identities across protocols (e.g. matching iTOW to NMEA time-of-day). Instead, it relies on the fact that all messages from the same physical epoch arrive as a burst from the receiver, and the first message of the next epoch from any protocol triggers the flush. This is a temporal correlation, not a semantic one.

This works because:
- A single receiver's epoch messages (both binary and NMEA) are generated from the same navigation solution and output in a burst.
- The inter-epoch gap (typically 100ms-1s depending on rate) is much larger than the intra-epoch message spacing (typically < 10ms).
- The scanner processes packets in arrival order, so all epoch N packets from both protocols are processed before any epoch N+1 packet.

### Where NavEpochManager lives

`NavEpochManager`, `EpochFlusher`, and `Accuracy.Merge` all go in `gps/gpsprot/msg.go` alongside the existing `MsgHandler`, `NavEpochMsg`, `mergeOpt`, and `SetAllMsgHandlers`.

The manager is passed to each `PacketProcessor` constructor, not set after construction. There is no `SetNavEpochManager` method or interface addition. `CreatePacketProcessors` creates the manager internally and forwards it to each protocol's `NewPacketProcessor`. RTCM is the exception -- its constructor is unchanged since it does not participate in epoch coordination.

### Changes to PacketProcessors

Each `PacketProcessor` that currently emits `NavEpochMsg` directly changes to go through the manager instead. All participating processors:
- Accept `*gpsprot.NavEpochManager` as a constructor parameter (stored in a `mgr` field)
- Implement `FlushNavEpoch(tRead time.Time) (*NavEpochMsg, MsgPriority, MsgHandler)` (satisfies `EpochFlusher`)

**UBX** (`gps/internal/ubx/ubx.go`):
- `NewPacketProcessor(mgr)`: stores the manager.
- `handleNavEpoch`: on iTOW change, calls `mgr.EpochStarted(p, tRead)`, then starts accumulating the new epoch. The `prevNavMsgs`/`curNavMsgs`/`nEpochsSeen` updates happen after the EpochStarted call as before.
- `FlushNavEpoch`: calls `p.flushSats()` first (satellite pair coordination), then returns `curNavEpochMsg` with `Tag = Tag`, `PriVendorLow`, and `p.mh`. Clears `curNavEpochMsg`.
- NAV-EOE: deferred. `ubxbin` has no parser for NAV-EOE yet (only cfgval keys `KUbxNavEoe`/`KUbxNav2Eoe` exist). When added, a single `mgr.EndOfEpoch(tRead)` call is all that's needed.

**Allystar binary** (`gps/internal/as/asproc.go`):
- `NewPacketProcessor(mgr)`: stores the manager.
- Same pattern as UBX but simpler (no satellite coordination in flush).
- `FlushNavEpoch`: returns `curNavEpochMsg` with `Tag = Tag`, `PriVendorLow`, and `p.mh`.

**NMEA** (`gps/internal/nmea/nmea.go`):
- `NewPacketProcessor(mgr)`: stores the manager.
- `handleEpoch` is the central dispatch point. It currently handles two cases:
  - `epoch == nil`: EOE signal (from `ExtSentenceHandler` returning `(bundle, nil, nil)`)
  - `epoch != p.curNavEpoch`: new epoch boundary
- With the manager, these become:
  - `epoch == nil`: call `mgr.EndOfEpoch(tRead)`
  - `epoch != p.curNavEpoch`: call `mgr.EpochStarted(p, tRead)`
- `FlushNavEpoch`: returns `&p.curNavEpoch.NavEpochMsg` with `Tag = Tag`, `PriGenericHigh`, and `p.mh`. Clears `curNavEpoch`.
- The `satellitesBuffer` is unaffected -- it continues to flush independently via idle timeout and repeated GSV keys.
- NMEA priority is `PriGenericHigh` (value 2). This is less than `PriVendorLow` (3), so binary protocol fields always win. But `PriGenericHigh` > `PriGenericLow` (1), so NMEA's richer sentences (GGA position with altitude) correctly win over simpler ones (RMC position) within the NMEA epoch itself (already handled by `NavEpochAccum`).

**Unicore** (`gps/internal/unc/processor.go`):
- Changes go on the unexported `packetProcessor` type (shared by `BinPacketProcessor` and `AsciiPacketProcessor`).
- `NewBinPacketProcessor(mgr)` and `NewAsciiPacketProcessor(mgr)`: pass to inner `packetProcessor`.
- `FlushNavEpoch` on `*packetProcessor`: returns `curEpochMsg` with `Tag = curEpochTag`, `PriVendorLow`, and `p.mh`.
- `handleEpoch`: calls `mgr.EpochStarted(p, tRead)` where `p` is `*packetProcessor`. Since `FlushNavEpoch` is also on `*packetProcessor`, the same pointer is used as the `EpochFlusher` interface value, so `isActive` identity comparison works correctly.
- Only one of binary or ASCII will be active for a given receiver, so no conflict between the two outer types.

**NovAtel** (`gps/internal/nov/processor.go`):
- Identical structure to Unicore (shared `packetProcessor` inner type). Same changes.

**CASIC** (`gps/internal/casic/casproc.go`):
- `NewPacketProcessor(mgr)`: stores the manager.
- CASIC already has epoch tracking via `handleNavEpoch` (RunTime change) and flushes satellite data on epoch boundaries. It does not yet build a `NavEpochMsg`, but it participates in the manager so its epochs are coordinated with NMEA.
- `handleNavEpoch`: on RunTime change, calls `mgr.EpochStarted(p, tRead)`.
- `FlushNavEpoch`: calls `p.satAccum.epochChange(...)` for satellite flushing, then returns `nil, PriVendorLow, p.mh` (no `NavEpochMsg` contribution yet). Adding `NavEpochMsg` population is a separate task.

### Test changes

Since the manager is a constructor parameter for individual `PacketProcessor`s, existing tests that verify `NavEpochMsg` emission create a `NavEpochManager` via `gpsprot.NewNavEpochManager()` and pass it to the processor constructor. Affected test files:

- `gps/internal/ubx/ubx_test.go` (`TestNavEpochHandling`, `TestNavEpochMsg`)
- `gps/internal/as/asproc_test.go` (`TestNavEpochMsg`)
- `gps/internal/nmea/nmea_test.go` (`TestEpochBoundary`, `TestEpochGGAVTGSameEpoch`, `TestExtHandlerEpochBoundary`, `TestExtHandlerEOE`)
- `gps/internal/unc/processor_test.go` (`TestEpochTracking`, `TestEpochTagFromFirstMessage`, `TestSameEpochNoFlush`)
- `gps/internal/nov/processor_test.go` (same tests as Unicore)

Each test creates a `NavEpochManager`, passes it to the processor constructor, then calls `pp.SetMsgHandler(recorder)` as before. The manager obtains the `MsgHandler` from the processor at flush time.

### Changes to wiring code

`CreatePacketProcessors` creates the `NavEpochManager` internally and passes it to each protocol's constructor. Callers do not need to know about the manager. The callers that create packet processors are:

**`gpsevent.NewDispatcher`** (`time/internal/gpsevent/dispatcher.go`):
- Calls `CreatePacketProcessors(nmeaNumbering)`. The dispatcher receives the already-wired processors and sets `MsgHandler` on them as before.

**`gps/app/gpscfg`** (`gps/app/gpscfg/gpscfg.go`):
- `gpscfg.Configure` never emits `NavEpochMsg` -- it only uses `NativeMsgHandler` during probing. No changes needed inside `gpscfg`.

**`internal/gpscmd`** (`internal/gpscmd/gpscmd.go`):
- Calls `CreatePacketProcessors(nil)`.

**`gps/gpsreg.CreatePacketProcessors`** (`gps/gpsreg/reg.go`):
- Signature: `CreatePacketProcessors(nmeaNumbering []gpsprot.NMEASVNumberingRange)`.
- Creates a `NavEpochManager` internally and passes it to each protocol's `NewPacketProcessor` call.

### Interaction with existing Idle mechanism

The `Idle` call (triggered by inter-packet timeout in the scanner) currently triggers `satellitesBuffer.flush` in NMEA. It is not used for epoch flushing and should not be -- `Idle` is a heuristic timeout unsuitable for the epoch mechanism, which requires deterministic, reliable flush triggers (`EpochStarted` and `EndOfEpoch`). The `NavEpochManager` has no interaction with `Idle`.

### NMEA tRead parameter note

There is a pre-existing inconsistency: NMEA's `flushEpoch` passes `p.curNavEpoch.StartTime` as the `tRead` to `mh.NavEpoch`, while binary protocols pass the read time of the boundary-triggering message. With the manager, all protocols use the boundary `tRead` uniformly (the `tRead` passed to `EpochStarted` or `EndOfEpoch`), which matches the binary protocol semantics and is arguably more correct -- `StartTime` is already carried inside `NavEpochMsg` itself.

## Implementation order

1. `Accuracy.Merge` + tests (`gps/gpsprot/msg.go`, `gps/gpsprot/msg_test.go`)
2. `EpochFlusher`, `NavEpochManager` + tests (`gps/gpsprot/msg.go`, `gps/gpsprot/msg_test.go`)
3. `CreatePacketProcessors` creates `NavEpochManager` internally (`gps/gpsreg/reg.go`)
4. Allystar processor (simplest, good prototype)
5. CASIC processor (similar to Allystar but returns nil NavEpochMsg)
6. UBX processor (adds satellite flush in `FlushNavEpoch`)
7. Unicore processor (embedded `packetProcessor` pattern)
8. NovAtel processor (identical to Unicore)
9. NMEA processor (most complex: EOE signal path)

## What this plan does NOT cover

- **Protocol-specific metadata population**: that is covered by solution-quality.md. This plan only addresses the cross-protocol coordination layer.
- **CASIC NavEpochMsg population**: CASIC participates in epoch coordination but does not yet build a `NavEpochMsg`. Adding metadata population is a separate task.
- **UBX NAV-EOE parsing**: `ubxbin` has no parser for this message yet. Adding the parser is a separate task; wiring it to `manager.EndOfEpoch` is trivial once the parser exists.
- **Epoch timeout**: there is no timeout mechanism to flush an epoch that has been accumulating too long (e.g. because the receiver went silent or disconnected after producing one epoch's worth of messages). The last epoch before silence is never emitted. Note that `Idle` is not suitable for this -- it is a heuristic used for satellite flushing, not a reliable epoch mechanism. A proper epoch timeout would need to be added to the manager.
