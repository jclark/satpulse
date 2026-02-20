# NovAtel message variant support

## Context

Multiple vendors emit identical NovAtel binary packets (AA 44 12 sync bytes) with the same binary layout, but with vendor-specific differences:

- **SinoGNSS**: Incompatible PosType enum values (e.g., value 51 = SUPER_WIDE_LANE vs OEM7's RTK_DIRECT_INS). SolStatus is a compatible subset of OEM7.
- **Unicore**: IONUTC uses message ID 6 instead of OEM7's ID 8 (see `novmsg/time.go:8`). This is about the undocumented/unsupported use of NovAtel OEM7 messages on Unicore receivers -- not the native Unicore protocol (which goes through the `unc` package).
- **ByNav**: Compatible with OEM7 for now; variant defined for future use.
- **Port address encoding**: OEM7/ByNav use 0x20=COM1, 0x40=COM2, 0x60=COM3. Unicore uses 1=COM1, 2=COM2, 3=COM3. SinoGNSS uses OEM7 byte values but shows decimal strings in ASCII ("32" not "COM1"). Currently `Port` constants use the Unicore encoding; OEM7/ByNav should be the default.

The structs `Pos[S, P]` and `XYZ[S, P]` are already parameterized on enum types. `MsgHdr` and `Msg` should be similarly parameterized on the port type to handle the port address encoding differences cleanly.

## Approach

1. Parameterize `MsgHdr` and `Msg` on port type; add `OEM7Port` and `UnicorePort` types
2. Add `SinoPosType` enum and `SinoBestPos`/`SinoBestXYZ` types to `novmsg`
3. Expose novmsg registries and add parse functions that accept a constructor map
4. `nov` processor holds a complete constructor map, built at construction time: a reference to the global registry for OEM7/ByNav, or a fresh copy with overrides merged in for SinoGNSS/Unicore
5. `nov` defines all four variants; `gpsreg` maps its Vendor to a Variant once
6. Enable the disabled IONUTC test case for Unicore variant

## Step 1: Parameterize MsgHdr and Msg on port type

### Port types in `gps/lib/novmsg/bin.go`

Replace the current `Port` type (iota-based, Unicore encoding) with two variant-specific types. OEM7 is the default:

```go
// OEM7Port represents the port address encoding used by NovAtel OEM7 and ByNav.
type OEM7Port uint8

const (
    OEM7COM1 OEM7Port = 0x20
    OEM7COM2 OEM7Port = 0x40
    OEM7COM3 OEM7Port = 0x60
)

// UnicorePort represents the port address encoding used by Unicore receivers
// (UM980, etc.) when emitting NovAtel-format packets.
type UnicorePort uint8

const (
    UnicoreCOM1 UnicorePort = 1
    UnicoreCOM2 UnicorePort = 2
    UnicoreCOM3 UnicorePort = 3
)
```

Each type gets `String()`, `MarshalText()`, `UnmarshalText()` methods. Both map to the same canonical names ("COM1", "COM2", "COM3"); the difference is the binary byte value.

SinoGNSS uses OEM7 port bytes but decimal strings in ASCII ("32" not "COM1"). For SinoGNSS, `OEM7Port` is used -- its `String()` returns "COM1" etc., and the existing ASCII output from SinoGNSS receivers ("32") would need a `fixupHeaderForAscii` in tests since the receiver doesn't conform to OEM7 ASCII naming.

### Parameterized MsgHdr and Msg in `gps/lib/novmsg/common.go`

```go
type MsgHdr[P ~uint8] struct {
    Port P
    CommonHdr
}

type Msg[P ~uint8] struct {
    Hdr  MsgHdr[P]
    Body MsgBody
}
```

### Parse and serialize functions

`ParseBinMsg` and `ParseAsciiMessage` become generic on port type:

```go
func ParseBinMsg[P ~uint8](packet []byte) (*Msg[P], error)
func ParseAsciiMessage[P ~uint8](packet []byte) (*Msg[P], error)
func SerializeBinMsg[P ~uint8](msg *Msg[P]) ([]byte, error)
func SerializeAsciiMsg[P ~uint8](msg *Msg[P]) ([]byte, error)
```

`BinaryHdr` is parameterized on port type:

```go
type BinaryHdr[P ~uint8] struct {
    Sync1        byte
    Sync2        byte
    Sync3        byte
    HeaderLength byte
    MsgID        MsgID
    MsgType      uint8
    Port         P
    // ... rest unchanged
}
```

`binary.Read` works with `BinaryHdr[P]` for any `P ~uint8` since the layout is identical (1 byte for Port regardless of type).

### Processor changes in `gps/internal/nov/`

No processor logic uses Port -- `dispatch`, `handleEpoch`, `timeMsgFromTime`, `dispatchIonUTC`, `msgHdrTime` all access only `CommonHdr` fields (Week, MillisecondsOfWeek, TimeStatus). These functions take `*CommonHdr` instead of `*MsgHdr`:

```go
func (p *packetProcessor) dispatch(common *CommonHdr, body MsgBody, ...) (bool, error)
func (p *packetProcessor) handleEpoch(common *CommonHdr, ...)
func timeMsgFromTime(common *CommonHdr, m *Time, ...) (*gpsprot.TimeMsg, error)
```

The processor's `processPacket` extracts `msg.Hdr.CommonHdr` and passes it through. The port type parameter is confined to the parse/serialize layer and tests.

### Test changes

Remove all `fixupHeaderForAscii` port hacks from Bynav tests (TIME, IONUTC, BESTGNSSVEL) since OEM7Port round-trips correctly. UM980 tests use `UnicorePort`. The bogus K901 BESTVEL test (captured separately, not a matched pair) should be removed.

## Step 2: SinoGNSS PosType and message types in novmsg

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

## Step 3: Registry access and map-based parse functions in novmsg

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

## Step 4: Variant support in nov processor

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

Constructors take no variant (default is OEM7). `SetVariant` rebuilds the constructor maps:

```go
func NewBinPacketProcessor() *BinPacketProcessor {
    return &BinPacketProcessor{
        packetProcessor: packetProcessor{mh: &gpsprot.DefaultHandler{}},
        ctors:           novmsg.BinRegistry(), // OEM7 default: use global registry directly
    }
}

// SetVariant configures the processor for a specific NovAtel-compatible vendor.
// Must be called before ProcessPacket.
func (p *BinPacketProcessor) SetVariant(v Variant) {
    p.ctors = binCtorsFor(v)
}
```

(Same pattern for AsciiPacketProcessor with `asciiCtorsFor`.)

Constructor map building:

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

## Step 5: SetVendor helper in gpsreg

### Modified: `gps/gpsreg/reg.go`

`CreatePacketProcessors` signature is **unchanged** -- it keeps taking `nmeaNumbering`. Add a separate `SetVendor` function that configures vendor-specific behavior on existing processors:

```go
type novVariantSetter interface {
    SetVariant(nov.Variant)
}

// SetVendor configures vendor-specific behavior on packet processors.
// This can be called after CreatePacketProcessors when the vendor
// is determined later (e.g. after auto-detection).
func SetVendor(procs map[gpsprot.Tag]gpsprot.PacketProcessor, vendor Vendor) {
    // NMEA satellite numbering
    if nmeaPP, ok := procs[nmea.Tag].(*nmea.PacketProcessor); ok {
        if numbering := FindNMEASVNumbering(vendor); numbering != nil {
            nmeaPP.SetSVNumbering(numbering)
        }
    }
    // NovAtel variant
    novVar := novVariantFor(vendor)
    for _, pp := range procs {
        if vs, ok := pp.(novVariantSetter); ok {
            vs.SetVariant(novVar)
        }
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

- `time/app/daemon/gps.go`: After `CreatePacketProcessors(nil)`, call `gpsreg.SetVendor(procs, vendor)` when vendor is known
- No changes needed to `internal/gpscmd/gpscmd.go`, `replay_test.go`, or `gpscfg_test.go` -- they use default (OEM7) behavior

## Step 6: Enable IONUTC test

Remove `disable: true` from the UM980 IONUTC test case in `gps/lib/novmsg/time_test.go:86`. The test needs to be updated to work with the Unicore variant (binary ID 6 instead of 8). Either:
- Test the binary parse using `ParseBinMsgUsing` with a Unicore constructor map
- Or adjust the test to parse the raw binary payload directly (since the body format is identical, only the header ID differs)

Remove the `XXX` comment from `novmsg/time.go:8` and replace with a clear note about the ID difference.

## Files summary

| File | Change |
|------|--------|
| `gps/lib/novmsg/bin.go` | OEM7Port, UnicorePort types; parameterize BinaryHdr[P]; ParseBinMsgUsing; refactor ParseBinMsg |
| `gps/lib/novmsg/common.go` | Parameterize MsgHdr[P], Msg[P]; add BinRegistry, AsciiRegistry |
| `gps/lib/novmsg/ascii.go` | Parameterize AsciiHdr; ParseAsciiMsgUsing; refactor ParseAsciiMessage |
| `gps/lib/novmsg/sinonav.go` | New: SinoPosType, SinoBestPos, SinoBestXYZ |
| `gps/lib/novmsg/sinonav_test.go` | New: round-trip tests |
| `gps/lib/novmsg/nav_test.go` | Remove bogus K901 BESTVEL test; remove port fixup hacks from Bynav tests |
| `gps/lib/novmsg/time_test.go` | Remove port fixup hacks from Bynav tests; enable IONUTC test |
| `gps/lib/novmsg/time.go` | Update IONUTC comment |
| `gps/internal/nov/processor.go` | Use *CommonHdr in dispatch/handleEpoch; variant type (4 variants), SetVariant, ctors maps, dispatch |
| `gps/internal/nov/nav.go` | Generic dispatch helpers |
| `gps/internal/nov/time.go` | Use *CommonHdr instead of *MsgHdr |
| `gps/gpsreg/reg.go` | Add SetVendor helper, novVariantFor |
| `time/app/daemon/gps.go` | Call SetVendor when vendor is known |

## Implementation order

1. OEM7Port/UnicorePort types + parameterize MsgHdr[P]/Msg[P]/BinaryHdr[P] + update all parse/serialize functions + update processor to use *CommonHdr + update tests
2. SinoPosType enum + SinoBestPos/SinoBestXYZ in novmsg (+ tests)
3. BinRegistry/AsciiRegistry + ParseBinMsgUsing/ParseAsciiMsgUsing in novmsg
4. Variant (all four) + SetVariant + constructor maps + dispatch in nov processor
5. SetVendor helper in gpsreg + call from daemon
6. Enable IONUTC test, update comments
7. `make test`

## Verification

- `go test -v ./gps/lib/novmsg/` -- Port type round-trip, SinoPosType round-trip, SinoBestPos/SinoBestXYZ parse, IONUTC test
- `go test -v ./gps/internal/nov/` -- Dispatch tests with VariantSinoGNSS
- `make test` -- Full test suite
