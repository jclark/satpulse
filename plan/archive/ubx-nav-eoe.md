# UBX-NAV-EOE epoch support

Issue: #218 (remaining tasks: configuration enablement and epoch flushing)

## Background

UBX-NAV-EOE (End of Epoch) is a small message (4-byte payload, just iTOW) that the receiver outputs after all NAV-class and NMEA messages for a navigation epoch. It provides an explicit epoch boundary signal, replacing the current heuristic of detecting epoch changes via iTOW transitions in the next epoch's first message.

### What already exists

- **Binary decoding**: `ubxbin.NavEOE` struct in `gps/lib/ubxbin/nav.go:39-44`. Embeds `NavITOW`, implements `NavMsg` interface (has `NavEpoch() uint32`).
- **Cfgval key**: `KUbxNavEoe` (0x15F) in `gps/lib/ubxcfgval/msgkey.go:93`.
- **PVTMsgEpoch flag**: defined in `gps/gpsprot/configtarget.go:119`, included in `PVTMsgAny`.
- **CLI support**: `--pvt-out epoch` parses to `PVTMsgEpoch` (see `internal/gpscmd/gpsflags.go`).
- **EndOfEpoch API**: `NavEpochManager.EndOfEpoch(tRead)` in `gps/gpsprot/msg.go:732`. Flushes immediately when exactly one processor is active. NMEA already uses this.
- **FlushNavEpoch**: `PacketProcessor.FlushNavEpoch` in `gps/internal/ubx/ubx.go:86-95` calls `p.flushSats()` first, then returns the accumulated `NavEpochMsg`. So satellite messages are flushed as part of epoch flushing.

### Current epoch detection

In `gps/internal/ubx/ubx.go:70-84`, `handleNavEpoch` is called for every message implementing `ubxbin.NavMsg` (any message with an iTOW). When the iTOW changes, it calls `mgr.EpochStarted(p, tRead)` which flushes the previous epoch. This means:
- The previous epoch is only flushed when the first message of the *next* epoch arrives.
- NAV-SAT and NAV-SIG already go through `handleNavEpoch` because they embed `NavITOW`.
- NAV-EOE also embeds `NavITOW`, so it will go through `handleNavEpoch` too (same iTOW as the epoch it terminates, so no new epoch start).

## Changes needed

### 1. Handle NAV-EOE in ProcessPacket to flush epochs

**File**: `gps/internal/ubx/ubx.go`

After `handleNavEpoch` runs (which correctly sees NAV-EOE as part of the current epoch since it has the same iTOW), check if the message is a `*ubxbin.NavEOE` and call `p.mgr.EndOfEpoch(tRead)`.

This should happen *before* `Dispatch` since NAV-EOE doesn't produce any protocol-agnostic messages. Add it in `ProcessPacket` after the `handleNavEpoch` call, around line 50-51:

```go
if nm, ok := m.(ubxbin.NavMsg); ok {
    p.handleNavEpoch(nm, tRead)
}
if _, ok := m.(*ubxbin.NavEOE); ok {
    p.mgr.EndOfEpoch(tRead)
    return msgID, nil
}
```

The `EndOfEpoch` call flushes all active processors unconditionally (both UBX and NMEA if both are active), since NAV-EOE marks the end of the epoch for all protocols on the receiver. `FlushNavEpoch` calls `flushSats()` first, so pending satellite messages are also flushed by EOE.

### 2. Enable NAV-EOE in UBX configuration when PVTMsgEpoch is set

This needs to work on both configuration paths.

#### 2a. msgChanges.pvt() -- shared by both paths

**File**: `gps/internal/ubx/ubxcfgmsg.go`, function `pvt()` (line 115)

Add NAV-EOE enablement at the end of `pvt()`, after the existing `NavTimeLSID` block (line 197-199). Gate it with a protocol version check (NAV-EOE was introduced in protocol version 18.0):

```go
if ver.protVerAtLeast(18, 0) {
    mc.pvtMsg(ubxbin.NavEOEID, flags&gpsprot.PVTMsgEpoch != 0, off)
}
```

This covers both paths:
- **Gen 8 (old path)**: `setMsg1()` -> `msgChanges.pvt()` -> `setMsgChanges()` -> `addMsgRateRequest()` sends a `UBX-CFG-MSG` packet with the msgID and rate. This works with any msgID, no lookup needed.
- **Gen 9+ (cfgval path)**: `txnBuilder.messagesBuild()` -> `msgChanges.options()` -> `pvt()`, then `msgChanges.items(port)` converts rates to cfgval items by looking up msgIDs in `msgIDKey`.

#### 2b. Add NavEOEID to msgIDKey -- needed for Gen 9+

**File**: `gps/internal/ubx/ubxcfgmsg.go`, `msgIDKey` map (line 383)

Add the mapping so the cfgval path can convert the message rate to a configuration item:

```go
ubxbin.NavEOEID: ucv.KUbxNavEoe,
```

Add it after the `NavVelNEDID` entry or in alphabetical order among the NAV message entries.

## Testing

- Test that the configuration path enables NAV-EOE: add a test case in `ubxcfgmsg_test.go` (or wherever `pvt()` is tested) verifying that `PVTMsgEpoch` produces a rate entry for `NavEOEID`.
- Test that NAV-EOE triggers epoch flush: write a test in `ubx_test.go` that sends NAV messages followed by NAV-EOE and verifies that `NavEpochMsg` is emitted after EOE rather than waiting for the next epoch's first message.
- Existing tests should continue to pass since NAV-EOE handling is additive.

## Hardware testing

Use `satpulsetool gps` with `--pvt-out epoch` on a u-blox receiver to verify NAV-EOE is enabled and received. The local machine has a ZED-F9P on `/dev/ttyACM0`.
