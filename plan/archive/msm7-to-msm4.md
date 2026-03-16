# MSM7 to MSM4 conversion

## Context

RTCM MSM7 is the high-resolution twin of MSM5 (full pseudoranges, phase ranges,
phase range rates, and CNR). MSM4 is the standard-resolution full message
(pseudoranges, phase ranges, and CNR -- no phase range rates). Converting MSM7
to MSM4 involves precision reduction and dropping phase range rate fields.

The goal is an in-memory conversion function. The MSM struct types (`MSMHiRes`,
`MSM`, `MSMHeader`, `MSMSatData`, `MSMSigData`, `MSMHiResSigData`) already
exist in `gps/lib/rtcmbin/mt.go`. Serialization to wire format will use
`bitsenc.Write` (see `plan/bitsenc-write.md`).

New code goes in `gps/lib/rtcmbin/mt.go`.

## Existing types (from mt.go)

```go
// Input type
type MSMHiRes struct {
    MSMHeader
    CellMask uint64        `bits:"var"`
    Sat      MSMSatData
    Sig      MSMHiResSigData
}

// Output type
type MSM struct {
    MSMHeader
    CellMask uint64        `bits:"var"`
    Sat      MSMSatData
    Sig      MSMSigData
}
```

`MSMSatData` is shared between both types. Satellite-level slices that don't
apply to MSM4 (ExtInfo, PhaseRate) will have length 0.

Signal data differs:

```go
type MSMHiResSigData struct {            type MSMSigData struct {
    Pseudorange []int32  `bits:"20"`         Pseudorange []int16  `bits:"15"`
    PhaseRange  []int32  `bits:"24"`         PhaseRange  []int32  `bits:"22"`
    LockTime    []uint16 `bits:"10"`         LockTime    Uint8Slice `bits:"4"`
    HalfCycle   []bool                       HalfCycle   []bool
    CNR         []uint16 `bits:"10"`         CNR         Uint8Slice `bits:"6"`
    PhaseRate   []int16  `bits:"15"`         PhaseRate   []int16  `bits:"15"`
}                                        }
```

## Conversion function

```go
// MSM4From7 converts an MSM7 message (MSMHiRes) to MSM4 (MSM).
// Returns an error if the input is not MSM7 (MSMLevel != 7) or contains
// reserved DF407 lock time values (705-1023).
// The input must be internally consistent (slice lengths match masks),
// as guaranteed by the parser; this is not re-validated.
func MSM4From7(m7 *MSMHiRes) (*MSM, error)
```

### Header

Check that `m7.MSMLevel() == 7`; return an error otherwise (MSMHiRes is also
used for MSM6, and `MsgNum -= 3` would produce an MSM3 number for MSM6 input).

Copy MSMHeader and change message number: `MsgNum -= 3`
(e.g. GPS 1077 becomes 1074, Galileo 1097 becomes 1094).

### CellMask

Copy as-is.

### Satellite data

Copy the MSMSatData struct. Set ExtInfo and PhaseRate to zero-length slices
(MSM4 doesn't have these). Keep RangeInt and RangeMod unchanged.

### Signal data conversions

Ncell = len(m7.Sig.Pseudorange).

Rounding helper shared by the pseudorange and phase range conversions:

```go
// roundShift rounds v by shifting right n bits, rounding half away from zero.
func roundShift(v int32, n uint) int32 {
    half := int32(1) << (n - 1)
    if v >= 0 {
        return (v + half) >> n
    }
    return (v + half - 1) >> n
}
```

#### Fine pseudorange: DF405 (int20) to DF400 (int15)

MSM7 scale is 2^-29 ms, MSM4 scale is 2^-24 ms. Divide by 32 with rounding.

- Invalid sentinel: DF405 == -2^19 (-524288) maps to DF400 == -2^14 (-16384)
- Otherwise: `DF400 = roundShift(DF405, 5)`, clamped to [-16383, 16383]

#### Fine phase range: DF406 (int24) to DF401 (int22)

MSM7 scale is 2^-31 ms, MSM4 scale is 2^-29 ms. Divide by 4 with rounding.

- Invalid sentinel: DF406 == -2^23 (-8388608) maps to DF401 == -2^21 (-2097152)
- Otherwise: `DF401 = roundShift(DF406, 2)`, clamped to [-2097151, 2097151]

#### CNR: DF408 (uint10) to DF403 (uint6)

MSM7 resolution is 1/16 dB-Hz, MSM4 resolution is 1 dB-Hz. Divide by 16.

- Zero (not computed) stays zero.
- Otherwise: `DF403 = (DF408 + 8) / 16`, clamped to [1, 63]

#### Lock time indicator: DF407 (uint10) to DF402 (uint4)

Two-step conversion through minimum lock time in milliseconds.

**Step 1**: Decode DF407 to minimum lock time in ms:

| Indicator (i) | Coeff (k) | Min lock time (ms) |
|---------------:|----------:|-------------------:|
| 0-63 | 1 | i |
| 64-95 | 2 | 2i - 64 |
| 96-127 | 4 | 4i - 256 |
| 128-159 | 8 | 8i - 768 |
| 160-191 | 16 | 16i - 2048 |
| 192-223 | 32 | 32i - 5120 |
| 224-255 | 64 | 64i - 12288 |
| 256-287 | 128 | 128i - 28672 |
| 288-319 | 256 | 256i - 65536 |
| 320-351 | 512 | 512i - 147456 |
| 352-383 | 1024 | 1024i - 327680 |
| 384-415 | 2048 | 2048i - 720896 |
| 416-447 | 4096 | 4096i - 1572864 |
| 448-479 | 8192 | 8192i - 3407872 |
| 480-511 | 16384 | 16384i - 7340032 |
| 512-543 | 32768 | 32768i - 15728640 |
| 544-575 | 65536 | 65536i - 33554432 |
| 576-607 | 131072 | 131072i - 71303168 |
| 608-639 | 262144 | 262144i - 150994944 |
| 640-671 | 524288 | 524288i - 318767104 |
| 672-703 | 1048576 | 1048576i - 671088640 |
| 704 | 2097152 | 2097152i - 1409286144 |
| 705-1023 | | reserved |

**Step 2**: Encode minimum lock time to DF402. Find the largest indicator
whose threshold is <= the minimum lock time.

| Indicator | Min lock time (ms) |
|----------:|-------------------:|
| 0 | 0 |
| 1 | 32 |
| 2 | 64 |
| 3 | 128 |
| 4 | 256 |
| 5 | 512 |
| 6 | 1024 |
| 7 | 2048 |
| 8 | 4096 |
| 9 | 8192 |
| 10 | 16384 |
| 11 | 32768 |
| 12 | 65536 |
| 13 | 131072 |
| 14 | 262144 |
| 15 | 524288 |

Helper functions:

```go
// lockTimeMsFromDF407 returns the minimum lock time in ms for a DF407
// indicator value. Returns (ms, true) for valid indicators 0-704, or
// (0, false) for reserved values 705-1023.
func lockTimeMsFromDF407(ind uint16) (uint32, bool)

// lockTimeToDF402 returns the DF402 indicator for a lock time in ms.
func lockTimeToDF402(ms uint32) uint8
```

`MSM4From7` returns an error if any DF407 value in the input is reserved.

#### Half-cycle ambiguity

Copy as-is.

#### Fine phase range rate

Set PhaseRate to zero-length slice (MSM4 has no phase range rate fields).

## Error handling

`MSM4From7` validates its input and returns errors for invalid data:
non-MSM7 input and reserved DF407 lock time values. The signal data
conversions clamp values to the target field ranges, so the output is always
well-formed. The downstream `bitsenc.Writer` writes low bits without range
checking (see `plan/bitsenc-write.md`), so it is this function's job to
ensure all values fit their fields before serialization.

`MSM4From7` assumes the input is structurally consistent: slice lengths match
the masks (SatMask, SigMask, CellMask), as guaranteed by the parser. This
invariant is not re-validated. The function only accepts parser-produced
`*MSMHiRes` values.

## Follow-on work

Producing wire-format RTCM packets requires an additional framing step not
covered by this plan: prepending the 3-byte header (preamble 0xD3 + 10-bit
payload length) and appending the 3-byte CRC24Q. This is a small function
that sits on top of `bitsenc.Write` output.

## Verification

- Unit test the lock time indicator conversion round-trip: for each DF407
  value 0-704, convert to ms then to DF402, and verify the DF402 minimum
  lock time is <= the DF407 minimum lock time.
- Unit test `MSM4From7` with a hand-constructed MSMHiRes covering: normal
  values, invalid sentinels, boundary values near clamp limits, zero CNR.
- Round-trip test: parse a real MSM7 packet into MSMHiRes, convert to MSM,
  verify header fields and signal count are preserved. Real MSM4 test data
  is in `plan/rtcmbin-msm.md`.
- `go test -v ./gps/lib/rtcmbin/`
