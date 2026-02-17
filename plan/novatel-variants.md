# NovAtel message variant support

## Context

Multiple vendors emit identical NovAtel binary packets (AA 44 12 sync bytes) with the same binary layout, but with vendor-specific differences:

- **SinoGNSS**: Incompatible PosType enum values (e.g., value 51 = SUPER_WIDE_LANE vs OEM7's RTK_DIRECT_INS). SolStatus is a compatible subset of OEM7.
- **Unicore**: IONUTC uses message ID 6 instead of OEM7's ID 8 (see `novmsg/time.go:8`). This is about the undocumented/unsupported use of NovAtel OEM7 messages on Unicore receivers -- not the native Unicore protocol (which goes through the `unc` package).
- **ByNav**: Compatible with OEM7 for now; variant defined for future use.

The structs `Pos[S, P]` and `XYZ[S, P]` are already parameterized on enum types. We need a mechanism to use vendor-specific types and message ID mappings based on the receiver vendor.

## Approach

1. Add `SinoPosType` enum and `SinoBestPos`/`SinoBestXYZ` types to `novmsg`
2. Expose novmsg registries and add parse functions that accept a constructor map
3. `nov` processor holds a complete constructor map, built at construction time: a reference to the global registry for OEM7/ByNav, or a fresh copy with overrides merged in for SinoGNSS/Unicore
4. `nov` defines all four variants; `gpsreg` maps its Vendor to a Variant once
5. Enable the disabled IONUTC test case for Unicore variant

## Step 1: SinoGNSS PosType and message types in novmsg

### New file: `gps/lib/novmsg/sinonav.go`

**SinoPosType enum** -- same approach as existing `PosType` but with SinoGNSS-specific values:

```go
type SinoPosType uint32

const (
    SinoPosNone             SinoPosType = 0
    SinoPosFixedPos         SinoPosType = 1
    SinoPosDopplerVelocity  SinoPosType = 8
    SinoPosSingleSmooth     SinoPosType = 9   // SinoGNSS-specific
    SinoPosSingle           SinoPosType = 16
    SinoPosPSRDiff          SinoPosType = 17
    SinoPosSBAS             SinoPosType = 18  // OEM7 calls this WAAS
    SinoPosNarrowFloat      SinoPosType = 34
    SinoPosFixDerivation    SinoPosType = 35  // SinoGNSS-specific
    SinoPosWideInt          SinoPosType = 49
    SinoPosNarrowInt        SinoPosType = 50
    SinoPosSuperWideLane    SinoPosType = 51  // OEM7 has RTK_DIRECT_INS
    SinoPosPPPConverging    SinoPosType = 68
    SinoPosPPP              SinoPosType = 69
)
```

With `String()`, `ParseSinoPosType()`, `MarshalText()`, `UnmarshalText()`.

Verify enum values against SinoGNSS docs at `~/gps-protocol-docs/` during implementation.

**SinoBestPos and SinoBestXYZ** -- embed generic types with SinoGNSS PosType, shared SolStatus:

```go
type SinoBestPos struct {
    Pos[SolStatus, SinoPosType]
}

func (m *SinoBestPos) ID() (MsgID, string) {
    return BestPosID, "BESTPOSA"
}

type SinoBestXYZ struct {
    XYZ[SolStatus, SinoPosType]
}

func (m *SinoBestXYZ) ID() (MsgID, string) {
    return BestXYZID, "BESTXYZA"
}
```

NOT registered via `init()` -- they share IDs with BestPos/BestXYZ and are only used when the SinoGNSS variant builds its constructor map.

### New file: `gps/lib/novmsg/sinonav_test.go`

- SinoPosType enum round-trip tests (String/Parse)
- SinoBestPos and SinoBestXYZ binary+ASCII round-trip tests

## Step 2: Registry access and map-based parse functions in novmsg

### Modified: `gps/lib/novmsg/common.go`

Expose the global registries so the nov processor can reference or copy them:

```go
// BinRegistry returns the global binary message constructor map (by MsgID).
func BinRegistry() map[MsgID]func() MsgBody { return msgIDMap }

// AsciiRegistry returns the global ASCII message constructor map (by wire name).
func AsciiRegistry() map[string]func() MsgBody { return msgNameMap }
```

### Modified: `gps/lib/novmsg/bin.go`

Add `ParseBinMsgUsing` that accepts a constructor map. Refactor `ParseBinMsg` to call it:

```go
func ParseBinMsg(packet []byte) (*Msg, error) {
    return ParseBinMsgUsing(packet, msgIDMap)
}

func ParseBinMsgUsing(packet []byte, ctors map[MsgID]func() MsgBody) (*Msg, error) {
    // ... existing header parsing unchanged ...
    ctor := ctors[msgID]
    // ... rest unchanged ...
}
```

### Modified: `gps/lib/novmsg/ascii.go`

Same pattern:

```go
func ParseAsciiMessage(packet []byte) (*Msg, error) {
    return ParseAsciiMsgUsing(packet, msgNameMap)
}

func ParseAsciiMsgUsing(packet []byte, ctors map[string]func() MsgBody) (*Msg, error) {
    // ... existing header parsing unchanged ...
    ctor := ctors[asciiHdr.MessageName]
    // ... rest unchanged ...
}
```

## Step 3: Variant support in nov processor

### Modified: `gps/internal/nov/processor.go`

All four variants defined. OEM7 is the default:

```go
type Variant int

const (
    VariantOEM7 Variant = iota
    VariantSinoGNSS
    VariantUnicore  // NovAtel-format messages on Unicore hardware (undocumented)
    VariantByNav
)
```

Processors hold a complete constructor map:

```go
type BinPacketProcessor struct {
    packetProcessor
    ctors map[novmsg.MsgID]func() novmsg.MsgBody
}

type AsciiPacketProcessor struct {
    packetProcessor
    ctors map[string]func() novmsg.MsgBody
}
```

Constructor map building at creation time:

```go
func binCtorsFor(v Variant) map[novmsg.MsgID]func() novmsg.MsgBody {
    reg := novmsg.BinRegistry()
    switch v {
    case VariantSinoGNSS:
        m := copyBinRegistry(reg)
        m[novmsg.BestPosID] = func() novmsg.MsgBody { return &novmsg.SinoBestPos{} }
        m[novmsg.BestXYZID] = func() novmsg.MsgBody { return &novmsg.SinoBestXYZ{} }
        return m
    case VariantUnicore:
        m := copyBinRegistry(reg)
        // Unicore uses message ID 6 for IONUTC instead of OEM7's ID 8
        m[6] = reg[novmsg.IonUTCID]
        delete(m, novmsg.IonUTCID)
        return m
    default: // OEM7, ByNav
        return reg
    }
}

func copyBinRegistry(reg map[novmsg.MsgID]func() novmsg.MsgBody) map[novmsg.MsgID]func() novmsg.MsgBody {
    m := make(map[novmsg.MsgID]func() novmsg.MsgBody, len(reg))
    for k, v := range reg {
        m[k] = v
    }
    return m
}
```

ASCII overrides: SinoGNSS needs overrides for "BESTPOSA"/"BESTXYZA". Unicore ASCII uses the same name "IONUTCA" so no ASCII override needed for Unicore.

ProcessPacket passes the map to the parse function:

```go
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
    bytes := []byte(data)
    msgID := BinPacketFormat.MsgID(bytes)
    err := p.processPacket(bytes, tRead, TagBinary, msgID,
        func(pkt []byte) (*novmsg.Msg, error) {
            return novmsg.ParseBinMsgUsing(pkt, p.ctors)
        })
    return msgID, err
}
```

(Same pattern for AsciiPacketProcessor.)

### Dispatch additions

Add SinoBestPos/SinoBestXYZ cases. Factor shared logic into generic helpers in `gps/internal/nov/nav.go`:

```go
func dispatchBestPos[P ~uint32](h gpsprot.MsgHandler, ne *gpsprot.NavEpochMsg,
    m *novmsg.Pos[novmsg.SolStatus, P], tag gpsprot.Tag, tRead time.Time) (bool, error)

func dispatchBestXYZ[P ~uint32](h gpsprot.MsgHandler, ne *gpsprot.NavEpochMsg,
    m *novmsg.XYZ[novmsg.SolStatus, P], tag gpsprot.Tag, tRead time.Time) (bool, error)
```

Dispatch becomes:
```go
case *novmsg.BestPos:
    return dispatchBestPos(h, p.curEpochMsg, &m.Pos, tag, tRead)
case *novmsg.SinoBestPos:
    return dispatchBestPos(h, p.curEpochMsg, &m.Pos, tag, tRead)
case *novmsg.BestXYZ:
    return dispatchBestXYZ(h, p.curEpochMsg, &m.XYZ, tag, tRead)
case *novmsg.SinoBestXYZ:
    return dispatchBestXYZ(h, p.curEpochMsg, &m.XYZ, tag, tRead)
```

## Step 4: Pass vendor through gpsreg

### Modified: `gps/gpsreg/reg.go`

Change `CreatePacketProcessors` to take `Vendor` instead of `nmeaNumbering`. Derives both NMEA numbering and nov variant from the vendor:

```go
func CreatePacketProcessors(vendor Vendor) map[gpsprot.Tag]gpsprot.PacketProcessor {
    nmeaNumbering := FindNMEASVNumbering(vendor)
    novVar := novVariantFor(vendor)
    // ...
    return map[gpsprot.Tag]gpsprot.PacketProcessor{
        // ...
        nov.TagBinary: nov.NewBinPacketProcessor(novVar),
        nov.TagAscii:  nov.NewAsciiPacketProcessor(novVar),
    }
}

func novVariantFor(v Vendor) nov.Variant {
    switch v {
    case VendorSinoGNSS:
        return nov.VariantSinoGNSS
    case VendorUnicore:
        return nov.VariantUnicore
    case VendorBynav:
        return nov.VariantByNav
    default:
        return nov.VariantOEM7
    }
}
```

### Modified callers

- `time/app/daemon/gps.go`: Simplify `CreatePacketProcessors()` to pass vendor directly
- `internal/gpscmd/gpscmd.go`: Pass `gpsreg.VendorUnknown`
- `internal/gpscmd/replay_test.go`: Pass `gpsreg.VendorUnknown`
- `gps/app/gpscfg/gpscfg_test.go`: Pass `gpsreg.VendorUnknown`

## Step 5: Enable IONUTC test

Remove `disable: true` from the UM980 IONUTC test case in `gps/lib/novmsg/time_test.go:86`. The test needs to be updated to work with the Unicore variant (binary ID 6 instead of 8). Either:
- Test the binary parse using `ParseBinMsgUsing` with a Unicore constructor map
- Or adjust the test to parse the raw binary payload directly (since the body format is identical, only the header ID differs)

Remove the `XXX` comment from `novmsg/time.go:8` and replace with a clear note about the ID difference.

## Files summary

| File | Change |
|------|--------|
| `gps/lib/novmsg/sinonav.go` | New: SinoPosType, SinoBestPos, SinoBestXYZ |
| `gps/lib/novmsg/sinonav_test.go` | New: round-trip tests |
| `gps/lib/novmsg/common.go` | Add BinRegistry, AsciiRegistry |
| `gps/lib/novmsg/bin.go` | Add ParseBinMsgUsing, refactor ParseBinMsg |
| `gps/lib/novmsg/ascii.go` | Add ParseAsciiMsgUsing, refactor ParseAsciiMessage |
| `gps/lib/novmsg/time.go` | Update IONUTC comment |
| `gps/lib/novmsg/time_test.go` | Enable IONUTC test |
| `gps/internal/nov/processor.go` | Variant type (4 variants), ctors maps, dispatch |
| `gps/internal/nov/nav.go` | Generic dispatch helpers |
| `gps/gpsreg/reg.go` | CreatePacketProcessors takes Vendor, novVariantFor |
| `time/app/daemon/gps.go` | Simplify to pass vendor |
| `internal/gpscmd/gpscmd.go` | Pass VendorUnknown |
| `internal/gpscmd/replay_test.go` | Pass VendorUnknown |
| `gps/app/gpscfg/gpscfg_test.go` | Pass VendorUnknown |

## Implementation order

1. SinoPosType enum + SinoBestPos/SinoBestXYZ in novmsg (+ tests)
2. BinRegistry/AsciiRegistry + ParseBinMsgUsing/ParseAsciiMsgUsing in novmsg
3. Variant (all four) + constructor maps + dispatch in nov processor
4. CreatePacketProcessors signature change in gpsreg + all callers
5. Enable IONUTC test, update comments
6. `make test`

## Verification

- `go test -v ./gps/lib/novmsg/` -- SinoPosType round-trip, SinoBestPos/SinoBestXYZ parse, IONUTC test
- `go test -v ./gps/internal/nov/` -- Dispatch tests with VariantSinoGNSS
- `make test` -- Full test suite
