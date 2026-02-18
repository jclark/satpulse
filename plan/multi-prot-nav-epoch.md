# Cross-protocol navigation epochs

Prerequisite: [nav-epoch.md](nav-epoch.md) (adds `NavEpochMsg` and `MsgHandler.NavEpoch`).

Related: [solution-metadata.md](solution-metadata.md) (populates `NavEpochMsg` fields), [nmea-extensibility.md](nmea-extensibility.md) (extended NMEA sentence handlers), [nmea-ext-handler-epoch.md](nmea-ext-handler-epoch.md) (epoch lifecycle on NMEA `PacketProcessor`).

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
// (or nil if it has nothing to contribute).
type EpochFlusher interface {
    FlushNavEpoch(tRead time.Time) *NavEpochMsg
}

// NavEpochManager coordinates navigation epoch handling across multiple
// protocol processors. Each PacketProcessor receives a reference to the
// shared manager and uses it instead of directly calling MsgHandler.NavEpoch.
type NavEpochManager struct {
    mh     MsgHandler
    active map[EpochFlusher]struct{} // processors that have reported epochs
}
```

### Protocol interaction

Each `PacketProcessor` that participates in epochs is given a reference to the shared `NavEpochManager`. The processor calls a single method:

```go
manager.EpochStarted(flusher EpochFlusher, tRead time.Time)
```

A processor calls `EpochStarted` every time it detects the start of a new epoch (iTOW change for binary protocols, time-of-day change in RMC/GGA for NMEA). The processor should call `EpochStarted` *before* it starts accumulating data for the new epoch.

The manager's behavior:

1. If the caller is already in the active set, this means the caller's previous epoch is complete and a new one is starting. The manager flushes: it calls `FlushNavEpoch(tRead)` on every processor in the active set, merges the non-nil results, emits a single `NavEpochMsg` via `MsgHandler.NavEpoch`, and clears the active set.
2. The caller is added to the (now possibly empty) active set.

This means a processor's first epoch is never flushed eagerly -- the manager doesn't know it's complete until the processor calls `EpochStarted` again. This matches the existing iTOW-change approach. In multi-protocol mode, whichever protocol's next epoch starts first triggers the flush for all protocols.

### End-of-epoch messages

Explicit end-of-epoch messages (UBX NAV-EOE, Quectel PQTMEOE) can trigger immediate flush. The processor signals this via:

```go
manager.EndOfEpoch(tRead time.Time)
```

The manager's behavior depends on the size of the active set:
- **One active processor**: the manager calls `FlushNavEpoch(tRead)` on that processor, emits the result, and clears the active set. This avoids the one-epoch latency of waiting for the next epoch to start.
- **Multiple active processors**: `EndOfEpoch` is a no-op. The manager cannot flush because the other protocol may not have finished its contribution yet. The flush will happen when any protocol's next `EpochStarted` call arrives.

### Active set lifecycle

The active set is transient -- it is built up during an epoch and cleared on flush. This means:

- At startup, the active set is empty.
- As protocols start producing epochs, they add themselves via `EpochStarted`.
- On flush, the active set is cleared. Protocols re-register on their next epoch.
- A protocol that stops producing epochs (e.g. NMEA gets disabled) simply stops calling `EpochStarted` and naturally drops out of subsequent epochs.

### Merge rules

When multiple protocols contribute to the same epoch, the manager merges their `NavEpochMsg` values field by field. The general rule is: **prefer the non-NMEA (binary) protocol's values when both provide the same field.**

Rationale: binary protocols typically provide higher-resolution data (mm accuracy vs NMEA's lack of metric accuracy, finer-grained fix quality enums). NMEA's contribution is valuable when the binary protocol lacks a field entirely.

### Epoch identity and correlation

In multi-protocol mode, the manager does not attempt to correlate epoch identities across protocols (e.g. matching iTOW to NMEA time-of-day). Instead, it relies on the fact that all messages from the same physical epoch arrive as a burst from the receiver, and the first message of the next epoch from any protocol triggers the flush. This is a temporal correlation, not a semantic one.

This works because:
- A single receiver's epoch messages (both binary and NMEA) are generated from the same navigation solution and output in a burst.
- The inter-epoch gap (typically 100ms-1s depending on rate) is much larger than the intra-epoch message spacing (typically < 10ms).
- The scanner processes packets in arrival order, so all epoch N packets from both protocols are processed before any epoch N+1 packet.

### Where NavEpochManager lives

`NavEpochManager` lives in `gps/gpsprot/` alongside `MsgHandler` and `NavEpochMsg`. It is created by the code that wires up `PacketProcessor`s (i.e. `gpscfg.Configure` and `gpsevent.NewDispatcher`), which already has access to all processors and the `MsgHandler`.

### Changes to PacketProcessors

Each `PacketProcessor` that currently emits `NavEpochMsg` directly (or will per the nav-epoch and solution-metadata plans) changes to go through the manager instead:

**UBX** (`gps/internal/ubx/ubx.go`):
- `handleNavEpoch`: on iTOW change, calls `manager.EpochStarted(p, tRead)`, then starts accumulating the new epoch.
- Implements `FlushNavEpoch(tRead) *NavEpochMsg`: flushes sats, returns `curNavEpochMsg`, clears it.
- NAV-EOE support: when a NAV-EOE packet is received, calls `manager.EndOfEpoch(tRead)`.

**Allystar binary** (`gps/internal/as/asproc.go`):
- Same pattern as UBX: `EpochStarted` on iTOW change, implements `FlushNavEpoch`.

**NMEA** (`gps/internal/nmea/nmea.go`):
- Epoch boundaries are detected by `CheckEpoch` (from nmea-ext-handler-epoch.md). When `handleEpoch` detects a new epoch, it calls `manager.EpochStarted(p, tRead)` instead of flushing directly.
- Implements `FlushNavEpoch`: returns the `NavEpochMsg` from `curNavEpoch`, clears it.
- The `satellitesBuffer` is unaffected -- it continues to flush independently via idle timeout and repeated GSV keys.

**CASIC** (`gps/internal/casic/casproc.go`):
- CASIC currently does not emit `NavEpochMsg` and only uses epoch boundaries for satellite flushing. It would need `NavEpochMsg` support added to participate. This can be deferred since CASIC is the legacy Allystar protocol.

**Unicore** (`gps/internal/unc/processor.go`):
- Same pattern: `EpochStarted` on GPS-time change in BESTNAV, implements `FlushNavEpoch`.

### Changes to wiring code

**`gpscfg.Configure`** (`gps/app/gpscfg/gpscfg.go`):
- Creates a `NavEpochManager` with the shared `MsgHandler`.
- Passes the manager to each `PacketProcessor` (via a new setter method or constructor parameter).

**`gpsevent.NewDispatcher`** (`time/internal/gpsevent/dispatcher.go`):
- Same: creates a `NavEpochManager` and passes it to each processor.

### Interaction with existing Idle mechanism

The `Idle` call (triggered by inter-packet timeout in the scanner) currently triggers `satellitesBuffer.flush` in NMEA. It should also be forwarded to the `NavEpochManager` as a hint that the current burst may be complete. However, `Idle` is not the primary flush trigger for `NavEpochMsg` -- epoch change is. `Idle` could serve as a fallback flush trigger for the very last epoch before the receiver goes silent (e.g. on disconnect), but this is an edge case that can be handled later.

## What this plan does NOT cover

- **Detailed merge implementation**: the exact Go code for field-by-field merging. The merge rules above define the semantics; implementation is straightforward.
- **Protocol-specific metadata population**: that is covered by solution-metadata.md. This plan only addresses the cross-protocol coordination layer.
- **CASIC NavEpochMsg support**: deferred since CASIC is legacy.
- **Multiple receivers**: this plan assumes a single receiver with one binary protocol + NMEA. Multiple receivers would each have their own `NavEpochManager`.
