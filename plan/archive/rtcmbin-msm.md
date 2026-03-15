# Plan: MSM message decoding in rtcmbin

## MSM message format (from RTCM 10403.2 section 3.5.15.3)

RTCM MSM (Multiple Signal Messages) carry GNSS observation data. Message types
1071-1077 are GPS, 1081-1087 GLONASS, 1091-1097 Galileo, 1101-1107 SBAS,
1111-1117 QZSS, 1121-1127 BeiDou, 1131-1137 NavIC. The last digit (1-7) is the
MSM level, which determines what data fields are present.

Every MSM message has three blocks: Header, Satellite Data, Signal Data.

### Header (169 + X bits)

All MSM levels share the same header:

| Field                    | DF    | Type   | Bits |
|--------------------------|-------|--------|------|
| Message Number           | DF002 | uint12 | 12   |
| Reference Station ID     | DF003 | uint12 | 12   |
| GNSS Epoch Time          |       | uint30 | 30   |
| Multiple Message Bit     | DF393 | bit    | 1    |
| IODS                     | DF409 | uint3  | 3    |
| Reserved                 | DF001 | bit(7) | 7    |
| Clock Steering Indicator | DF411 | uint2  | 2    |
| External Clock Indicator | DF412 | uint2  | 2    |
| Divergence-free Smooth.  | DF417 | bit    | 1    |
| Smoothing Interval       | DF418 | bit(3) | 3    |
| Satellite Mask           | DF394 | bit(64)| 64   |
| Signal Mask              | DF395 | bit(32)| 32   |
| Cell Mask                | DF396 | bit(X) | X    |

Where:
- Nsat = popcount(Satellite Mask) - number of observed satellites
- Nsig = popcount(Signal Mask) - number of signal types
- X = Nsat * Nsig (max 64) - size of cell mask
- Ncell = popcount(Cell Mask) - number of satellite/signal combinations with data

The satellite mask has 64 bits, one per possible satellite ID for the constellation.
The signal mask has 32 bits, one per possible signal type. The cell mask has
Nsat*Nsig bits indicating which satellite/signal combinations are actually present.

### Satellite data

Satellite data has one value per satellite (Nsat values). Fields are grouped by
type using "internal looping": all values of the first field, then all values of
the second field, etc.

| Field          | DF    | Type  | Bits | MSM levels |
|----------------|-------|-------|------|------------|
| Range integer  | DF397 | uint8 | 8    | 4,5,6,7    |
| Extended info  |       | uint4 | 4    | 5,7        |
| Range modulo   | DF398 | uint10| 10   | all        |
| Phase rate     | DF399 | int14 | 14   | 5,7        |

### Signal data

Signal data has one value per cell (Ncell values), also using internal looping.
MSM1-5 and MSM6-7 use different bit widths for several fields:

**MSM1-5 (standard resolution):**

| Field        | DF    | Type  | Bits | MSM levels |
|--------------|-------|-------|------|------------|
| Pseudorange  | DF400 | int15 | 15   | 1,3,4,5    |
| Phase range  | DF401 | int22 | 22   | 2,3,4,5    |
| Lock time    | DF402 | uint4 | 4    | 2,3,4,5    |
| Half-cycle   | DF420 | bit   | 1    | 2,3,4,5    |
| CNR          | DF403 | uint6 | 6    | 4,5        |
| Phase rate   | DF404 | int15 | 15   | 5          |

**MSM6-7 (high resolution):**

| Field        | DF    | Type   | Bits | MSM levels |
|--------------|-------|--------|------|------------|
| Pseudorange  | DF405 | int20  | 20   | 6,7        |
| Phase range  | DF406 | int24  | 24   | 6,7        |
| Lock time    | DF407 | uint10 | 10   | 6,7        |
| Half-cycle   | DF420 | bit    | 1    | 6,7        |
| CNR          | DF408 | uint10 | 10   | 6,7        |
| Phase rate   | DF404 | int15  | 15   | 7          |

### Internal looping example

For MSM4 satellite data with Nsat=3 (fields: DF397 uint8, DF398 uint10):

    DF397[sat0] DF397[sat1] DF397[sat2] DF398[sat0] DF398[sat1] DF398[sat2]

NOT `{DF397, DF398}` repeated per satellite.

### Pseudorange/phase range reconstruction

Standard precision (MSM1-5):
- Pseudorange(i) = c/1000 * (Nms + Rough_range/1024 + 2^-24 * Fine_Pseudorange(i))
- PhaseRange(i) = c/1000 * (Nms + Rough_range/1024 + 2^-29 * Fine_PhaseRange(i))

High precision (MSM6-7):
- Pseudorange(i) = c/1000 * (Nms + Rough_range/1024 + 2^-29 * Fine_Pseudorange(i))
- PhaseRange(i) = c/1000 * (Nms + Rough_range/1024 + 2^-31 * Fine_PhaseRange(i))

PhaseRangeRate(i) = Rough_PhaseRangeRate + 0.0001 * Fine_PhaseRangeRate(i), m/s

---

## Implementation plan

### Step 1: Export Reader from bitsenc

**File:** `gps/lib/bitsenc/bitsenc.go`

- Rename `bitReader` to `Reader` and export it
- Add `NewReader(data []byte) *Reader`
- Add `(*Reader).Read(v any) error` method (same logic as current package-level `Read`)
- Remove the package-level `Read` function

The `Reader` is the bit-stream analogue of `io.Reader` - it maintains position across
multiple `Read` calls.

### Step 2: Extend Reader

**File:** `gps/lib/bitsenc/bitsenc.go`

Export raw read methods:
- `(*Reader).Uint(n int) (uint64, error)` - read n unsigned bits
- `(*Reader).Int(n int) (int64, error)` - read n bits with sign extension

Change default field handling in `readStruct`. Currently no-tag means skip.
New semantics:
- No `bits` tag: use the native size of the type (uint8=8, uint16=16, uint32=32,
  uint64=64, int8=8, int16=16, int32=32, int64=64, bool=1). Struct fields
  (named or embedded) are recursed into.
- `bits:"N"`: read N bits (override native size), same as today.
- `bits:"var"`: variable-width field. See `VarBits` interface below.

Slice fields: when a field is a slice type, read `len(slice)` elements. Bit width
per element comes from the `bits:"N"` tag if present, otherwise from the native
element type size. This models the internal looping layout - each slice field is
one "column".

Two new interfaces:

```go
// VarBits yields the bit width for each bits:"var" field in declaration order.
type VarBits interface {
    VarBits() iter.Seq[int]
}

// SliceSizer allocates slices based on already-read fields.
// Called when readStruct encounters a nil slice on the root struct.
type SliceSizer interface {
    SizeSlices()
}
```

**VarBits**: when `readStruct` starts on a struct, it checks if the struct
implements `VarBits` (via `sv.Addr()`). If so, it obtains the iterator.
Each time it encounters a `bits:"var"` field, it pulls the next value from
the iterator to get the bit width. Width 0 is valid (no bits read, field
stays zero value).

**SliceSizer**: `readStruct` threads a reference to the root struct through
recursive calls. When it encounters a nil slice, it checks if the root struct
implements `SliceSizer` and calls `SizeSlices()`. The implementation
must allocate **all** slices, including in nested structs. For fields not
present in this message variant, allocate a zero-length slice (`make([]T, 0)`),
not nil. This ensures `SizeSlices` is only ever called once - after it runs,
no nil slices remain, so the trigger condition never fires again.
After the call, `readStruct` reads `len(slice)` elements for each slice
(0 elements for absent fields, N elements for present ones).

Processing order (same for both `MSM` and `MSMHiRes`):
1. `MSMHeader` (embedded struct) - recurse, read fields natively
2. `CellMask` (`bits:"var"`) - `VarBits` yields `nsat*nsig`, reads that many bits
3. `Sat` (named struct) - recurse; first nil slice triggers `SizeSlices()` which
   allocates all slices in both `Sat` and `Sig`; then reads slice elements
4. `Sig` (named struct) - recurse; slices already allocated, reads elements

### Step 3: Update rtcmbin to use bitsenc.Reader

**File:** `gps/lib/rtcmbin/rtcmbin.go`

- Update `ParseMsg` MT1005/1006 cases to use `bitsenc.NewReader(...).Read(&msg)`

### Step 4: Add MSM types

**File:** `gps/lib/rtcmbin/mt.go` (append to existing)

```go
// Tags only where bit width differs from native type size.
type MSMHeader struct {
    MsgHdr
    StationID     uint16 `bits:"12"`
    EpochTime     uint32 `bits:"30"`
    MultipleMsg   bool
    IODS          uint8  `bits:"3"`
    Reserved      uint8  `bits:"7"`
    ClockSteering uint8  `bits:"2"`
    ExtClock      uint8  `bits:"2"`
    DivFree       bool
    Smoothing     uint8  `bits:"3"`
    SatMask       uint64
    SigMask       uint32
}

// Satellite data - same for all MSM levels. Zero-length slices = field not present.
type MSMSatData struct {
    RangeInt  []uint8            // DF397, MSM4-7
    ExtInfo   []uint8  `bits:"4"`  // MSM5/7
    RangeMod  []uint16 `bits:"10"` // DF398, all MSM
    PhaseRate []int16  `bits:"14"` // DF399, MSM5/7
}

// Standard-resolution signal data (MSM1-5).
type MSMSigData struct {
    Pseudorange []int16  `bits:"15"` // DF400
    PhaseRange  []int32  `bits:"22"` // DF401
    LockTime    []uint8  `bits:"4"`  // DF402
    HalfCycle   []bool             // DF420
    CNR         []uint8  `bits:"6"`  // DF403
    PhaseRate   []int16  `bits:"15"` // DF404
}

// High-resolution signal data (MSM6-7).
type MSMHiResSigData struct {
    Pseudorange []int32  `bits:"20"` // DF405
    PhaseRange  []int32  `bits:"24"` // DF406
    LockTime    []uint16 `bits:"10"` // DF407
    HalfCycle   []bool             // DF420
    CNR         []uint16 `bits:"10"` // DF408
    PhaseRate   []int16  `bits:"15"` // DF404
}

Two concrete types, no generics:

```go
// MSM is an MSM1-5 message (standard resolution signal data).
type MSM struct {
    MSMHeader
    CellMask uint64 `bits:"var"`
    Sat      MSMSatData
    Sig      MSMSigData
}

// MSMHiRes is an MSM6-7 message (high resolution signal data).
type MSMHiRes struct {
    MSMHeader
    CellMask uint64 `bits:"var"`
    Sat      MSMSatData
    Sig      MSMHiResSigData
}
```

Shared helpers on MSMHeader:

```go
// cellMaskBits returns the number of bits in the cell mask.
func (h *MSMHeader) cellMaskBits() int {
    return bits.OnesCount64(h.SatMask) * bits.OnesCount32(h.SigMask)
}

// sizeSatSlices allocates satellite data slices based on MSM level.
func (h *MSMHeader) sizeSatSlices(sat *MSMSatData) {
    nsat := bits.OnesCount64(h.SatMask)
    level := h.MSMLevel()
    sat.RangeInt = make([]uint8, boolN(level >= 4, nsat))
    sat.ExtInfo = make([]uint8, boolN(level == 5 || level == 7, nsat))
    sat.RangeMod = make([]uint16, nsat)
    sat.PhaseRate = make([]int16, boolN(level == 5 || level == 7, nsat))
}
```

VarBits and SizeSlices on each type (thin wrappers around shared helpers):

```go
func (m *MSM) VarBits() iter.Seq[int] {
    return func(yield func(int) bool) { yield(m.cellMaskBits()) }
}
func (m *MSM) SizeSlices() {
    m.sizeSatSlices(&m.Sat)
    m.Sig.sizeSlices(m.MSMLevel(), bits.OnesCount64(m.CellMask))
}

func (m *MSMHiRes) VarBits() iter.Seq[int] {
    return func(yield func(int) bool) { yield(m.cellMaskBits()) }
}
func (m *MSMHiRes) SizeSlices() {
    m.sizeSatSlices(&m.Sat)
    m.Sig.sizeSlices(m.MSMLevel(), bits.OnesCount64(m.CellMask))
}
```

Slice allocation per signal type:

```go
// sizeSlices allocates all slices. Absent fields get zero-length slices (not nil)
// so that bitsenc never encounters a nil slice after SizeSlices runs.
func (s *MSMSigData) sizeSlices(level, ncell int) {
    s.Pseudorange = make([]int16, boolN(level >= 1 && level != 2, ncell))
    s.PhaseRange = make([]int32, boolN(level >= 2, ncell))
    s.LockTime = make([]uint8, boolN(level >= 2, ncell))
    s.HalfCycle = make([]bool, boolN(level >= 2, ncell))
    s.CNR = make([]uint8, boolN(level >= 4, ncell))
    s.PhaseRate = make([]int16, boolN(level >= 5, ncell))
}

func (s *MSMHiResSigData) sizeSlices(level, ncell int) {
    s.Pseudorange = make([]int32, ncell)
    s.PhaseRange = make([]int32, ncell)
    s.LockTime = make([]uint16, ncell)
    s.HalfCycle = make([]bool, ncell)
    s.CNR = make([]uint16, ncell)
    s.PhaseRate = make([]int16, boolN(level >= 7, ncell))
}

// boolN returns n if cond is true, 0 otherwise.
func boolN(cond bool, n int) int { ... }
```

GNSS constellation type:

```go
type GNSS string

const (
    GPS     GNSS = "GPS"
    GLONASS GNSS = "GLONASS"
    GALILEO GNSS = "GALILEO"
    SBAS    GNSS = "SBAS"
    QZSS    GNSS = "QZSS"
    BEIDOU  GNSS = "BEIDOU"
    IRNSS   GNSS = "IRNSS"
)
```

Methods on MSMHeader:
- `GNSS() GNSS` - constellation from message type (107x=GPS, 108x=GLONASS, etc.)
- `MSMLevel() int` - MSM level 1-7 from last digit
- `Satellites() []uint8` - satellite IDs from set bits in SatMask
- `Signals() []uint8` - signal IDs from set bits in SigMask

### Step 5: Parse MSM in ParseMsg

**File:** `gps/lib/rtcmbin/rtcmbin.go`

```go
if mt.IsMSM() {
    r := bitsenc.NewReader([]byte(payload))
    if int(mt % 10) >= 6 {
        var msg MSMHiRes
        if err := r.Read(&msg); err != nil {
            return nil, fmt.Errorf("rtcm MSM %d: %w", mt, err)
        }
        return &msg, nil
    }
    var msg MSM
    if err := r.Read(&msg); err != nil {
        return nil, fmt.Errorf("rtcm MSM %d: %w", mt, err)
    }
    return &msg, nil
}
```

### Step 6: Tests

**File:** `gps/lib/bitsenc/bitsenc_test.go`
- Update `TestReadSkipsUntagged` - untagged fields now read at native size, not skip
- Test slice reading (nil = 0 elements, non-nil = len elements)
- Test VarBits and SliceSizer interfaces
- Test Uint/Int methods

**File:** `gps/lib/rtcmbin/mt_test.go` (append to existing)
- Test mask helper methods (Satellites, Signals, GNSS, MSMLevel)
- Test with real MSM packets (to be provided)

### Verification

- `go test -v ./gps/lib/bitsenc/...`
- `go test -v ./gps/lib/rtcmbin/...`
- Test with real MSM packet data once available

### MSM4 data

```
{"t":"2026-03-15T09:53:58.189505Z","tag":"RTCM","msg":"1074","bin":"d300a84324d2088045020020041d5c40000000002020010063b6d3fa88948e969ea29294889beeeef5e483f002a68678f0f05e797e480f830c5023c301fe35ecd0da9e08af144bf583cf77ba3f6c9ec7fe467c6963afce7e011e200cef80ca3883c1700b3e178e0bfe244d7aad51ebf8884470813909400001fb0407dd0dbfdb4b7f4f2ff9a1b0011e3e4431f8f73bff9ffffff541ffffff94000016dcde1655347138d336d784f1d4d300d2d0f5"}
{"t":"2026-03-15T09:53:58.206307Z","tag":"RTCM","msg":"1084","bin":"d3005543c4d20b1259c20020418c0100000000002080000057d252320a42525402d13db3af99f46098e8de7690f428dd4209c5c7b077f834f5f31f208abce27ae704c8d617b7a05c06d000007dfffff00054f638e5844d00b1eb60"}
{"t":"2026-03-15T09:53:58.215476Z","tag":"RTCM","msg":"1094","bin":"d300a64464d20880450200200184043200000000200101007ffffd595551496145747502516849affdc37f893f8dbfc56853d235a9075e76e01de8433a075b900e0a5022184f516f93094668b0d8e520cd8a004d100722002273f9cba3ec1957aa08feadc77b47ddee10c049692146fc852582064ef80c7ee06f0706a0e81c931871a55e0d30f71845da5c07ffd7f5ee7fffffffff7f800002ebec7a179d7dbb71ba191dbf0b5e9620991956"}
{"t":"2026-03-15T09:53:58.232283Z","tag":"RTCM","msg":"1114","bin":"d3004d45a4d20880450200203100000000000000200041007bdde1deba1a0e58f299f19fe188c0f1bf5842b1e1e30afc791bfaeb2feb7560000007197fbe649eff56fbfe92fff0afff00b2bb57665beeeabdac"}
{"t":"2026-03-15T09:53:58.23992Z","tag":"RTCM","msg":"1124","bin":"d300dc4644d2087f6a42002037f80000000000002082014171c71c71c71c7043c3bbf42bbbc413ba6aa1ef0f6aada915f0ea59a47178141829a64d1c94c93542493f423f187d223f7685f10a96c81d869aeb7f3f7e90fbe42f5c6878d0b30bc6254c1cfa9ff61be8ff24a84dbca1338f04be8c1233404804811b25fef14dfb923fed3ec16d480531c8141017b5ce5ee0277b82da01ded80c8ee0393900bad0019537fa65e195a405a6001683cfed95ffb2987ead47e4603ffffffffffffffffffffffffe5d48000000598e37639575d4f3f35d6db4d76cd3cf6595b8fb8e00a95dd9"}
{"t":"2026-03-15T09:53:58.261318Z","tag":"RTCM","msg":"1124","bin":"d301304644d2087f6a4200200006014043e000002082014171c6fbefba1befbbca429a925bcc33caba58c560eda0739a38f9751676989ea93ffa7e140fa825203625390ac997002aea5004c029982348c6414b92feac04e215c023bfdd7005c041c0af8071ffd466b463a0daf1b9632f05ae7a16fcbe00a7f58f97d3e468709139212c3b882250f08e976246b201b46807e6e021030166b386df1618c1786301c17e030629f4190d106031c17b8a863500011cf80711002359823a3c014548056da02d7000f722043b100ac6619f520318fe0f96e83bffc0c69f8349a5fa46f7e945ffa612fe9359fa34d0899bc270c4095a6e23f8788adbbffffffffffffffffffffffffffbfff7f99777ffffe0000000000617df8e58db5d765b4d74573e5adb85f4512513bd0490c6dcb3cb2c50cafc0f805a8c12"}
{"t":"2026-03-15T09:53:58.290337Z","tag":"RTCM","msg":"1124","bin":"d300494644d2087f6a400020000000000000001c2082014165904faf2f0d24d6c0fbc6f571e84453c08170ffbac87f74edfd3c1ff892a04c0a4129c6049ed3d3890fffffff010c71e98eba00b6f0a6"}
{"t":"2026-03-15T09:53:59.18623Z","tag":"RTCM","msg":"1005","bin":"d300133ed4d203bd55b51d7f8e2e1f5ad403808e27daf56b52"}
```