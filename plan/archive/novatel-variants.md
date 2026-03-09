# NovAtel message variant support

## Context

Multiple vendors emit identical NovAtel binary packets (AA 44 12 sync bytes) with the same binary layout, but with vendor-specific differences:

- **SinoGNSS**: Incompatible PosType enum values (e.g., value 51 = SUPER_WIDE_LANE vs OEM7's RTK_DIRECT_INS). SolStatus is a compatible subset of OEM7.
- **Unicore**: IONUTC uses message ID 6 instead of OEM7's ID 8 (see `novmsg/time.go:8`). This is about the undocumented/unsupported use of NovAtel OEM7 messages on Unicore receivers -- not the native Unicore protocol (which goes through the `unc` package).
- **ByNav**: Compatible with OEM7 for now; variant defined for future use.
- **Port address encoding**: OEM7/ByNav use 0x20=COM1, 0x40=COM2, 0x60=COM3. Unicore uses 1=COM1, 2=COM2, 3=COM3. SinoGNSS uses OEM7 byte values but shows decimal strings in ASCII ("32" not "COM1"). Currently `Port` constants use the Unicore encoding; OEM7/ByNav should be the default.

The structs `Pos[S, P]` and `XYZ[S, P]` are already parameterized on enum types. `MsgHdr` and `Msg` should be similarly parameterized on the port type to handle the port address encoding differences cleanly.

## Approach

1. Parameterize `MsgHdr` and `Msg` on port type; add `Port`, `UnicorePort`, and `SinoPort` types
2. Add `SinoPosType` enum and `SinoBestPos`/`SinoBestXYZ` types to `novmsg`
3. Expose novmsg registries and add parse functions that accept a constructor map
4. `nov` processor holds a complete constructor map, built at construction time: a reference to the global registry for OEM7/ByNav, or a fresh copy with overrides merged in for SinoGNSS/Unicore
5. `nov` defines all four variants; `gpsreg` maps its Vendor to a Variant once
6. Enable the disabled IONUTC test case for Unicore variant

## Step 1: Parameterize MsgHdr and Msg on port type

### Port types in `gps/lib/novmsg/bin.go`

Replace the current `Port` type (iota-based, Unicore encoding) with three variant-specific types. OEM7 is the default and uses the unadorned `Port` name (consistent with `PosType`):

```go
// Port represents the port address encoding used by NovAtel OEM7 and ByNav.
type Port uint8

const (
    COM1 Port = 0x20
    COM2 Port = 0x40
    COM3 Port = 0x60
)

// UnicorePort represents the port address encoding used by Unicore receivers
// (UM980, etc.) when emitting NovAtel-format packets.
type UnicorePort uint8

const (
    UnicoreCOM1 UnicorePort = 1
    UnicoreCOM2 UnicorePort = 2
    UnicoreCOM3 UnicorePort = 3
)

// SinoPort represents the port address encoding used by SinoGNSS receivers.
// Same binary byte values as OEM7 (0x20, 0x40, 0x60) but serialized as
// decimal strings in ASCII ("32", "64", "96") instead of "COM1", "COM2", "COM3".
type SinoPort uint8
// No constants -- same byte values as Port, no SinoGNSS-specific port values needed.
```

Port and UnicorePort get `String()`, `MarshalText()`, `UnmarshalText()` methods that map to canonical names ("COM1", "COM2", "COM3"); the difference is the binary byte value.

SinoPort gets `String()`/`MarshalText()` that returns the decimal representation of the byte value (e.g., `SinoPort(0x20).String()` returns `"32"`), and `UnmarshalText()` that parses decimal strings back. This matches what SinoGNSS receivers actually emit in ASCII, so binary-to-ASCII round-trips work without any fixup hacks.

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

Remove all `fixupHeaderForAscii` port hacks from Bynav tests (TIME, IONUTC, BESTGNSSVEL) since Port round-trips correctly. UM980 tests use `UnicorePort`. K901 (SinoGNSS) tests use `SinoPort` -- decimal port strings round-trip naturally. The bogus K901 BESTVEL test (captured separately, not a matched pair) should be removed.

## Step 2: Untyped PosType constants and SinoGNSS types in novmsg

### Modified: `gps/lib/novmsg/navtypes.go`

Change the existing `PosType` constants from typed to **untyped**. The `PosType` type itself remains (for `String()`/`Parse()`/use in `Pos[S, P]`), but the constants become untyped so they work directly with any vendor's typed enum (`PosType`, `SinoPosType`, `uncmsg.PosVelType`) or `uint32` without casting:

```go
type PosType uint32

const (
    PosNone             = 0
    PosFixedPos         = 1
    PosFixedHeight      = 2
    PosFloatConv        = 4   // OEM7/ByNav only
    PosWideLane         = 5   // OEM7/ByNav only
    PosNarrowLane       = 6   // OEM7/ByNav only
    PosDopplerVelocity  = 8
    PosSingle           = 16
    PosPSRDiff          = 17
    PosWAAS             = 18  // OEM7/ByNav name
    PosSBAS             = 18  // Unicore/SinoGNSS name (same value)
    PosPropagated       = 19  // OEM7/ByNav only
    PosL1Float          = 32
    PosIonoFreeFloat    = 33
    PosNarrowFloat      = 34
    PosL1Int            = 48
    PosWideInt          = 49
    PosNarrowInt        = 50
    // 51: vendor-specific (OEM7=RTK_DIRECT_INS, Sino=SUPER_WIDE_LANE)
    // 52: vendor-specific (OEM7=INS_SBAS, Unicore=INS)
    PosRTKDirectINS     = 51  // OEM7 only
    PosINSSBAS          = 52  // OEM7 only (Unicore 52 = INS, different meaning)
    PosINSPSRSP         = 53
    PosINSPSRDiff       = 54
    PosINSRTKFloat      = 55
    PosINSRTKFixed      = 56
    PosExtConstrained   = 67  // OEM7 only
    PosPPPConverging    = 68
    PosPPP              = 69
    // 70+: vendor-specific (OEM7=OPERATIONAL, Unicore=PPP_AR)
    PosOperational           = 70  // OEM7 only
    PosWarning               = 71  // OEM7 only
    PosOutOfBounds           = 72  // OEM7 only
    PosINSPPPConverging      = 73  // OEM7 only
    PosINSPPP                = 74  // OEM7 only
    PosPPPBasicConverging    = 77  // OEM7 only
    PosPPPBasic              = 78  // OEM7 only
    PosINSPPPBasicConverging = 79  // OEM7 only
    PosINSPPPBasic           = 80  // OEM7 only
)
```

The `String()` and `ParsePosType()` methods on `PosType` remain unchanged (they handle all OEM7 values). `uncmsg.PosVelType` can use these untyped constants directly for its shared values, and only needs typed constants for Unicore-specific values (52=INS, 70=PPP_AR, 71=PPP_RTK).

### New file: `gps/lib/novmsg/sinonav.go`

**SinoPosType** -- only defines constants for the three values that differ from OEM7. For shared values, the untyped constants above work directly:

```go
type SinoPosType uint32

const (
    SinoPosSingleSmooth  SinoPosType = 9   // carrier-smoothed single point
    SinoPosFixDerivation SinoPosType = 35  // derived/propagated RTK
    SinoPosSuperWideLane SinoPosType = 51  // OEM7 has RTK_DIRECT_INS
)
```

With `String()`, `ParseSinoPosType()`, `MarshalText()`, `UnmarshalText()`. The `String()` method handles all values SinoGNSS supports: the three SinoGNSS-specific values above plus the shared values (using the untyped constants in switch cases). Falls back to decimal for unknown values.

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

Processors hold a constructor map and a parse function. The parse function is a closure over the port type, returning a `ParseResult` (which erases the port type parameter):

```go
// ParseResult holds the port-type-independent parts of a parsed Msg[P].
type ParseResult struct {
    Common novmsg.CommonHdr
    Body   novmsg.MsgBody
    Msg    any // original *novmsg.Msg[P] for NativeMsg passthrough
}

type BinPacketProcessor struct {
    packetProcessor
    ctors map[novmsg.MsgID]func() novmsg.MsgBody
    parse func([]byte, map[novmsg.MsgID]func() novmsg.MsgBody) (ParseResult, error)
}

type AsciiPacketProcessor struct {
    packetProcessor
    ctors map[string]func() novmsg.MsgBody
    parse func([]byte, map[string]func() novmsg.MsgBody) (ParseResult, error)
}
```

Generic helper to build the parse closure:

```go
func binParser[P ~uint8]() func([]byte, map[novmsg.MsgID]func() novmsg.MsgBody) (ParseResult, error) {
    return func(pkt []byte, ctors map[novmsg.MsgID]func() novmsg.MsgBody) (ParseResult, error) {
        msg, err := novmsg.ParseBinMsgUsing[P](pkt, ctors)
        if err != nil {
            return ParseResult{}, err
        }
        return ParseResult{Common: msg.Hdr.CommonHdr, Body: msg.Body, Msg: msg}, nil
    }
}
```

(Same pattern for `asciiParser[P]`.)

Constructors default to OEM7. `SetVariant` rebuilds constructor maps and parse functions:

```go
func NewBinPacketProcessor(mgr *gpsprot.NavEpochManager) *BinPacketProcessor {
    return &BinPacketProcessor{
        packetProcessor: packetProcessor{mh: &gpsprot.DefaultHandler{}, mgr: mgr},
        ctors:           novmsg.BinRegistry(),
        parse:           binParser[novmsg.Port](),
    }
}

// SetVariant configures the processor for a specific NovAtel-compatible vendor.
// Must be called before ProcessPacket.
func (p *BinPacketProcessor) SetVariant(v Variant) {
    p.ctors, p.parse = binVariant(v)
}
```

(Same pattern for AsciiPacketProcessor.)

Variant configuration returns both constructor map and parse function:

```go
func binVariant(v Variant) (map[novmsg.MsgID]func() novmsg.MsgBody,
    func([]byte, map[novmsg.MsgID]func() novmsg.MsgBody) (ParseResult, error)) {
    reg := novmsg.BinRegistry()
    switch v {
    case VariantSinoGNSS:
        m := copyBinRegistry(reg)
        m[novmsg.BestPosID] = func() novmsg.MsgBody { return &novmsg.SinoBestPos{} }
        m[novmsg.BestXYZID] = func() novmsg.MsgBody { return &novmsg.SinoBestXYZ{} }
        return m, binParser[novmsg.SinoPort]()
    case VariantUnicore:
        m := copyBinRegistry(reg)
        // Unicore uses message ID 6 for IONUTC instead of OEM7's ID 8
        m[6] = reg[novmsg.IonUTCID]
        delete(m, novmsg.IonUTCID)
        return m, binParser[novmsg.UnicorePort]()
    default: // OEM7, ByNav
        return reg, binParser[novmsg.Port]()
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

ProcessPacket calls the stored parse function:

```go
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
    bytes := []byte(data)
    msgID := BinPacketFormat.MsgID(bytes)
    err := p.processPacket(bytes, tRead, TagBinary, msgID,
        func(pkt []byte) (ParseResult, error) {
            return p.parse(pkt, p.ctors)
        })
    return msgID, err
}
```

(Same pattern for AsciiPacketProcessor.)

`processPacket` changes signature to use `ParseResult`:

```go
func (p *packetProcessor) processPacket(bytes []byte, tRead time.Time, tag gpsprot.Tag, msgID string,
    parser func([]byte) (ParseResult, error)) error {
    res, err := parser(bytes)
    if err != nil {
        return err
    }
    handled, err := p.dispatch(&res.Common, res.Body, tRead, tag)
    if err != nil {
        return err
    }
    if handled {
        return nil
    }
    nmh := p.GetNativeMsgHandler()
    if nmh != nil {
        return nmh.NativeMsg(tag, msgID, res.Msg, tRead)
    }
    return nil
}
```

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

## Step 5: SetVendor helper and CreatePacketProcessors change in gpsreg

### Modified: `gps/gpsreg/reg.go`

Change `CreatePacketProcessors` to take `Vendor` instead of `nmeaNumbering`. When vendor is not `VendorUnknown`, call `SetVendor` before returning:

```go
// CreatePacketProcessors creates packet processors for all registered protocols.
// A shared NavEpochManager coordinates epoch handling across protocols.
func CreatePacketProcessors(vendor Vendor) map[gpsprot.Tag]gpsprot.PacketProcessor {
    mgr := gpsprot.NewNavEpochManager()
    nmeaPP := nmea.NewPacketProcessor(mgr)
    nmeaPP.AddExtHandler(quectel.NewHandler())
    procs := map[gpsprot.Tag]gpsprot.PacketProcessor{
        // ... same as now ...
    }
    if vendor != VendorUnknown {
        SetVendor(procs, vendor)
    }
    return procs
}
```

`SetVendor` is exported separately for the desktop GUI, where vendor is determined after processor construction:

```go
type novVariantSetter interface {
    SetVariant(nov.Variant)
}

// SetVendor configures vendor-specific behavior on packet processors.
// Called by CreatePacketProcessors when vendor is known at construction,
// or separately when the vendor is determined later (e.g. desktop GUI).
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

- `time/app/daemon/gps.go`: Pass vendor directly to `CreatePacketProcessors(vendor)` instead of looking up NMEA numbering separately
- `internal/gpscmd/gpscmd.go`, `replay_test.go`, `gpscfg_test.go`: Change `CreatePacketProcessors(nil)` to `CreatePacketProcessors(VendorUnknown)` (or `0`)

## Step 6: Enable IONUTC test

Remove `disable: true` from the UM980 IONUTC test case in `gps/lib/novmsg/time_test.go:86`. The test needs to be updated to work with the Unicore variant (binary ID 6 instead of 8). Either:
- Test the binary parse using `ParseBinMsgUsing` with a Unicore constructor map
- Or adjust the test to parse the raw binary payload directly (since the body format is identical, only the header ID differs)

Remove the `XXX` comment from `novmsg/time.go:8` and replace with a clear note about the ID difference.

## Files summary

| File | Change |
|------|--------|
| `gps/lib/novmsg/bin.go` | Port, UnicorePort, SinoPort types; parameterize BinaryHdr[P]; ParseBinMsgUsing; refactor ParseBinMsg |
| `gps/lib/novmsg/common.go` | Parameterize MsgHdr[P], Msg[P]; add BinRegistry, AsciiRegistry |
| `gps/lib/novmsg/ascii.go` | Parameterize AsciiHdr; ParseAsciiMsgUsing; refactor ParseAsciiMessage |
| `gps/lib/novmsg/navtypes.go` | Change PosType constants to untyped; add PosSBAS alias |
| `gps/lib/novmsg/sinonav.go` | New: SinoPosType (3 constants only), SinoBestPos, SinoBestXYZ |
| `gps/lib/novmsg/sinonav_test.go` | New: round-trip tests |
| `gps/lib/novmsg/nav_test.go` | Remove bogus K901 BESTVEL test; remove port fixup hacks from Bynav tests |
| `gps/lib/novmsg/time_test.go` | Remove port fixup hacks from Bynav tests; enable IONUTC test |
| `gps/lib/novmsg/time.go` | Update IONUTC comment |
| `gps/internal/nov/processor.go` | ParseResult type; use *CommonHdr in dispatch/handleEpoch; variant type (4 variants), SetVariant, ctors maps + parse closures, dispatch |
| `gps/internal/nov/nav.go` | Generic dispatch helpers |
| `gps/internal/nov/time.go` | Use *CommonHdr instead of *MsgHdr |
| `gps/gpsreg/reg.go` | Change CreatePacketProcessors to take Vendor; add SetVendor, novVariantFor |
| `time/app/daemon/gps.go` | Pass vendor to CreatePacketProcessors |
| `internal/gpscmd/gpscmd.go` | Update CreatePacketProcessors call |
| `internal/gpscmd/replay_test.go` | Update CreatePacketProcessors call |
| `gps/app/gpscfg/gpscfg_test.go` | Update CreatePacketProcessors call |

## Implementation order

1. Port/UnicorePort/SinoPort types + parameterize MsgHdr[P]/Msg[P]/BinaryHdr[P] + update all parse/serialize functions + ParseResult + update processor to use *CommonHdr + update tests
2. SinoPosType enum + SinoBestPos/SinoBestXYZ in novmsg (+ tests)
3. BinRegistry/AsciiRegistry + ParseBinMsgUsing/ParseAsciiMsgUsing in novmsg
4. Variant (all four) + SetVariant + constructor maps + dispatch in nov processor
5. CreatePacketProcessors(Vendor) + SetVendor in gpsreg + update all callers
6. Enable IONUTC test, update comments
7. `make test`

## Verification

- `go test -v ./gps/lib/novmsg/` -- Port type round-trip, SinoPosType round-trip, SinoBestPos/SinoBestXYZ parse, IONUTC test
- `go test -v ./gps/internal/nov/` -- Dispatch tests with VariantSinoGNSS
- `make test` -- Full test suite
