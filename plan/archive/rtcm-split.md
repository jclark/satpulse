# Split RTCM into lib and internal packages

Prerequisite for correction daemon support (#221).  The daemon needs
access to RTCM field extraction functions (`ReferenceStationID`,
`IsMultipleMessage`, message type) but cannot import
`gps/internal/rtcm` from outside the `gps/internal/` tree.

Split into `gps/lib/rtcmbin` (pure wire-format types and extraction)
and `gps/internal/rtcm` (packet format + processor), following the
existing pattern where `gps/lib/ubxbin` / `gps/lib/casbin` hold
wire-format definitions with no internal dependencies, and
`gps/internal/ubx` / `gps/internal/casic` hold the `PacketFormat`
and `PacketProcessor` implementations that depend on `gpsprot`.

## gps/lib/rtcmbin

Pure RTCM wire-format package.  Zero internal dependencies (no
`gpsprot` import), only stdlib + `crc24q`.

Types and functions:

- `MsgType` type (uint16) with `String()` and `IsMSM()` methods
- `ExtractMsgType[B Bytes](pkt B) MsgType`
- `ReferenceStationID[B Bytes](pkt B) (uint16, bool)`
- `IsMultipleMessage[B Bytes](pkt B) bool` -- true when the MSM
  multiple-message bit is set (more messages follow for this epoch)
- `IsCommonMsgType(MsgType) bool`
- `Message` struct (`MsgType` + `Payload`) and `ParseMessage`
- `Bytes` type constraint (`string | []byte`)
- `PreambleByte` constant (0xD3)
- `ARPMsgType`, `GLONASSBiasMsgType` constants
- `Checksum(pkt []byte) uint32` -- CRC-24Q over packet minus
  trailing 3 bytes
- `ExtractChecksum(pkt []byte) uint32` -- reads trailing 3 bytes
  as CRC-24Q

## gps/internal/rtcm

Retains everything that depends on `gpsprot`:

- `packetFormat` struct implementing `gpsprot.PacketFormat` and
  `gpsprot.MultiPacketFormat` (scan state machine: `Next`,
  `IsFinal`, `MsgID`; checksum: `ExtractChecksum`,
  `ComputeChecksum`, `RescanOnBadChecksum`; multi-packet:
  `HasMore`)
- `PacketFormat` variable and `Tag` constant
- `MSMMsgType(gnss gpsprot.GNSS, msm int) MsgType` -- stays here
  because it takes `gpsprot.GNSS`
- `PacketProcessor` struct, `NewPacketProcessor`,
  `ProcessPacket`, `NativeOnly`

The `packetFormat` methods call into `rtcmbin` for extraction:
- `MsgID` calls `rtcmbin.ExtractMsgType`
- `HasMore` calls `rtcmbin.IsMultipleMessage`
- Checksum methods call `rtcmbin.Checksum` / `rtcmbin.ExtractChecksum`

## Consumer updates

### gps/internal/ubx/ubxcfgmsg.go

Uses `rtcm.MSMMsgType` and `rtcm.GLONASSBiasMsgType`.  `MSMMsgType`
stays in `gps/internal/rtcm` (needs `gpsprot.GNSS`).
`GLONASSBiasMsgType` moves to `rtcmbin`.  Add a second import for
`rtcmbin` alongside the existing `rtcm` import.

### gps/gpsreg/reg.go

No change needed -- already imports `gps/internal/rtcm` which still
exports `Tag`, `PacketFormat`, and `NewPacketProcessor`.

### Test files

- `gps/internal/rtcm/rtcm_test.go` -- tests for `PacketFormat`,
  `PacketProcessor`, scan state machine stay here.  Pure extraction
  tests (`TestReferenceStationID`, `TestIsMSM`, `TestIsCommonMsgType`,
  `TestHasMore` renamed to `TestIsMultipleMessage`) move to
  `gps/lib/rtcmbin/rtcmbin_test.go`.
- `gps/scan/rtcm_test.go` -- uses `rtcm.PacketFormat`, no change.
- `gps/scan/ubx_test.go` -- uses `rtcm.PacketFormat`, no change.
- `gps/app/corrsink/corrsink_test.go` -- uses `rtcm.PacketFormat`
  and `rtcm.PreambleByte`.  Change `PreambleByte` import to
  `rtcmbin.PreambleByte`.

## docs/internals.md

Update the `gps/internal/rtcm` entry and add a `gps/lib/rtcmbin`
entry:

```
`gps/lib/rtcmbin` defines RTCM binary wire-format types: message type
extraction, reference station ID extraction, multiple-message flag
detection, and CRC-24Q checksumming.

`gps/internal/rtcm` implements `gps/gpsprot` abstractions for the
RTCM protocol. It uses `gps/lib/rtcmbin` for field extraction.
```

## Implementation order

1. Create `gps/lib/rtcmbin/rtcmbin.go` with pure extraction
   types and functions.
2. Update `gps/internal/rtcm/rtcm.go` to import `rtcmbin` and
   delegate extraction to it.
3. Move pure-extraction tests to `gps/lib/rtcmbin/rtcmbin_test.go`.
4. Update `gps/internal/ubx/ubxcfgmsg.go` import.
5. Update `gps/app/corrsink/corrsink_test.go` import for
   `PreambleByte`.
6. Update `docs/internals.md`.
7. `make test` to verify.

## Verification

- `make test` passes.
- `go vet ./...` clean.
- `gps/lib/rtcmbin` has no imports from `gps/gpsprot` or
  `gps/internal/`.
