# Legacy RTCM observables (MT1001-1004, MT1009-1012) and system parameters (MT1013)

## Context

The [gps/lib/rtcmbin](../gps/lib/rtcmbin/) package currently decodes a narrow set of RTCM messages (MT1005/1006 station coordinates, MT1230 GLONASS code-phase biases, and MSM1-7). We want to extend it to handle the eight legacy observable messages plus the system-parameters announcement message:

- **GPS observables**: MT1001 (basic L1), MT1002 (extended L1), MT1003 (basic L1+L2), MT1004 (extended L1+L2)
- **GLONASS observables**: MT1009, MT1010, MT1011, MT1012 (same four variants)
- **System parameters**: MT1013 — announces which messages a base station transmits and at what interval

These are still widely emitted by reference stations that don't use MSM, and MT1013 is a useful complement for identifying a base station's message mix. Decoding them is needed for compatibility with legacy base stations and for completeness of the `gps/lib/rtcmbin` surface.

## Key design issue: AoS vs SoA

Legacy RTCM messages lay out per-satellite data **array-of-structs** on the wire — all fields for satellite 1, then all fields for satellite 2, etc. MSM messages lay out per-satellite data **struct-of-arrays** (all sat-1 DF397 values, then all sat-1 DF398 values, etc.). The existing `bitsenc` codec ([gps/lib/bitsenc/bitsenc.go:144-174](../gps/lib/bitsenc/bitsenc.go#L144-L174)) only handles primitive-element slices via `readField` / `writeField`; it does not recurse into struct elements. We must extend `bitsenc` to support `[]Struct` before the clean reflection-driven approach works for these messages.

## Plan

### 1. Extend `bitsenc` to support slices of structs

Files: [gps/lib/bitsenc/bitsenc.go](../gps/lib/bitsenc/bitsenc.go), [gps/lib/bitsenc/bitsenc_test.go](../gps/lib/bitsenc/bitsenc_test.go).

**Read path** — in `readStruct` at [bitsenc.go:145-174](../gps/lib/bitsenc/bitsenc.go#L145-L174), after the nil-slice SizeSlices trigger, check `elemKind`:

- If `reflect.Struct`: reject a `bits:"..."` tag (no element bit-width for structs); iterate `fv.Len()` elements and call `readStruct(r, fv.Index(j), root, varNext)` recursively for each.
- Otherwise: existing primitive path.

**Write path** — symmetric change in `writeStruct` at [bitsenc.go:321](../gps/lib/bitsenc/bitsenc.go#L321).

**Tests** — add a `bitsenc_test.go` case that round-trips a struct containing a `[]InnerStruct` field (InnerStruct has a few `bits:"N"` fields covering signed/unsigned).

Notes: SizeSlices is still called on the root only, so the per-element struct fields must not themselves need variable allocation. Our legacy RTCM structs are fixed-width per satellite, so this is fine.

### 2. Add message types to [mt.go](../gps/lib/rtcmbin/mt.go)

Shared headers (placed after MT1230, before MSM definitions). JSON tags follow the MT1005 lowerCamel convention.

Both headers embed `MsgHdrStationID` (defined in [rtcmbin.go](../gps/lib/rtcmbin/rtcmbin.go)) so they inherit both the message-type field and DF003 with `RefStationID()` — no per-MT registry needed.

```go
// LegacyGPSHeader is common to MT1001-1004.
type LegacyGPSHeader struct {
    MsgHdrStationID                              // DF002 + DF003
    Epoch       uint32 `bits:"30" json:"epoch"`       // DF004 GPS TOW ms
    MultipleMsg bool              `json:"multipleMsg"` // DF005
    Nsat        uint8  `bits:"5"  json:"nsat"`        // DF006
    DivFree     bool              `json:"divFree"`     // DF007
    Smoothing   uint8  `bits:"3"  json:"smoothing"`   // DF008
}

// LegacyGLONASSHeader is common to MT1009-1012.
type LegacyGLONASSHeader struct {
    MsgHdrStationID                              // DF002 + DF003
    Epoch       uint32 `bits:"27" json:"epoch"`       // DF034 GLONASS tk (27 bits, note wider in MSM)
    MultipleMsg bool              `json:"multipleMsg"` // DF005
    Nsat        uint8  `bits:"5"  json:"nsat"`        // DF035
    DivFree     bool              `json:"divFree"`     // DF036
    Smoothing   uint8  `bits:"3"  json:"smoothing"`   // DF037
}
```

Per-satellite structs. Embedding follows **wire order** — MT1004Sat embeds MT1002Sat (not MT1003Sat) and MT1012Sat embeds MT1010Sat (not MT1011Sat) because DF014/DF015 and DF044/DF045 appear in the wire *before* the L2 fields. Bit widths and signedness per RTCM SC-104 v3.2:

```go
// GPS L1 core fields (DF009-DF013); base for MT1001 and MT1003 via embedding.
type MT1001Sat struct {
    SatID         uint8  `bits:"6"  json:"satID"`         // DF009
    L1Code        bool              `json:"l1Code"`        // DF010 (C/A vs P)
    L1Pseudorange uint32 `bits:"24" json:"l1Pseudorange"` // DF011
    L1PhaseRange  int32  `bits:"20" json:"l1PhaseRange"`  // DF012 signed
    L1LockTime    uint8  `bits:"7"  json:"l1LockTime"`    // DF013
}

// MT1002 = MT1001 + L1 modulus + L1 CNR. Also base for MT1004.
type MT1002Sat struct {
    MT1001Sat
    L1Ambiguity uint8 `bits:"8" json:"l1Ambiguity"` // DF014
    L1CNR       uint8 `bits:"8" json:"l1CNR"`       // DF015
}

// MT1003 = MT1001 + L2 core (DF016-DF019).
type MT1003Sat struct {
    MT1001Sat
    L2Code        uint8 `bits:"2"  json:"l2Code"`        // DF016
    L2L1PRDiff    int16 `bits:"14" json:"l2L1PRDiff"`    // DF017 signed
    L2PhaseRange  int32 `bits:"20" json:"l2PhaseRange"`  // DF018 signed
    L2LockTime    uint8 `bits:"7"  json:"l2LockTime"`    // DF019
}

// MT1004 = MT1002 + L2 core + L2 CNR (wire order DF009..DF020).
type MT1004Sat struct {
    MT1002Sat
    L2Code       uint8 `bits:"2"  json:"l2Code"`        // DF016
    L2L1PRDiff   int16 `bits:"14" json:"l2L1PRDiff"`    // DF017 signed
    L2PhaseRange int32 `bits:"20" json:"l2PhaseRange"`  // DF018 signed
    L2LockTime   uint8 `bits:"7"  json:"l2LockTime"`    // DF019
    L2CNR        uint8 `bits:"8"  json:"l2CNR"`         // DF020
}

// GLONASS L1 core (DF038-DF043); base for MT1009 and MT1011 via embedding.
type MT1009Sat struct {
    SatID         uint8  `bits:"6"  json:"satID"`         // DF038
    L1Code        bool              `json:"l1Code"`        // DF039
    FreqChan      uint8  `bits:"5"  json:"freqChan"`      // DF040
    L1Pseudorange uint32 `bits:"25" json:"l1Pseudorange"` // DF041 (25 bits, wider than GPS DF011)
    L1PhaseRange  int32  `bits:"20" json:"l1PhaseRange"`  // DF042 signed
    L1LockTime    uint8  `bits:"7"  json:"l1LockTime"`    // DF043
}
// FreqChan holds the raw 5-bit DF040 code (0..20). Per spec this encodes
// GLONASS frequency channel numbers -7..+13 (code 0 = -7, code 7 = 0, code 20 = +13);
// consumers are responsible for the code-to-channel translation.

// MT1010 = MT1009 + L1 ambiguity (7 bits) + CNR. Also base for MT1012.
type MT1010Sat struct {
    MT1009Sat
    L1Ambiguity uint8 `bits:"7" json:"l1Ambiguity"` // DF044 (7 bits, narrower than GPS DF014)
    L1CNR       uint8 `bits:"8" json:"l1CNR"`       // DF045
}

// MT1011 = MT1009 + L2 core (DF046-DF049).
type MT1011Sat struct {
    MT1009Sat
    L2Code       uint8 `bits:"2"  json:"l2Code"`        // DF046
    L2L1PRDiff   int16 `bits:"14" json:"l2L1PRDiff"`    // DF047 signed; see note below
    L2PhaseRange int32 `bits:"20" json:"l2PhaseRange"`  // DF048 signed
    L2LockTime   uint8 `bits:"7"  json:"l2LockTime"`    // DF049
}

// MT1012 = MT1010 + L2 core + L2 CNR (wire order DF038..DF050).
type MT1012Sat struct {
    MT1010Sat
    L2Code       uint8 `bits:"2"  json:"l2Code"`        // DF046
    L2L1PRDiff   int16 `bits:"14" json:"l2L1PRDiff"`    // DF047 signed
    L2PhaseRange int32  `bits:"20" json:"l2PhaseRange"`  // DF048 signed
    L2LockTime   uint8  `bits:"7"  json:"l2LockTime"`    // DF049
    L2CNR        uint8 `bits:"8"  json:"l2CNR"`         // DF050
}
```

**DF047 is int14.** The MT1011/MT1012 satellite tables mislabel it as `uint14` ([RTCM_SC-104_v3.2.md:4880](../../../gps-protocol-docs/rtcm/RTCM_SC-104_v3.2.md#L4880), [line 4914](../../../gps-protocol-docs/rtcm/RTCM_SC-104_v3.2.md#L4914)); the authoritative field definition at [line 2185](../../../gps-protocol-docs/rtcm/RTCM_SC-104_v3.2.md#L2185) is `int14` with `0x2000 = -163.84 m` as the "invalid L2" sentinel — which only makes sense under two's-complement. Model DF047 as `int16` with `bits:"14"` and include a fixture covering the `0x2000` sentinel to lock in signed decoding.

Message types (one per MT):

```go
type MT1001 struct { LegacyGPSHeader;     Sat []MT1001Sat `json:"sat"` }
type MT1002 struct { LegacyGPSHeader;     Sat []MT1002Sat `json:"sat"` }
type MT1003 struct { LegacyGPSHeader;     Sat []MT1003Sat `json:"sat"` }
type MT1004 struct { LegacyGPSHeader;     Sat []MT1004Sat `json:"sat"` }
type MT1009 struct { LegacyGLONASSHeader; Sat []MT1009Sat `json:"sat"` }
type MT1010 struct { LegacyGLONASSHeader; Sat []MT1010Sat `json:"sat"` }
type MT1011 struct { LegacyGLONASSHeader; Sat []MT1011Sat `json:"sat"` }
type MT1012 struct { LegacyGLONASSHeader; Sat []MT1012Sat `json:"sat"` }
```

Each message implements `SizeSlices` to allocate `Sat` from `Nsat`:

```go
func (m *MT1001) SizeSlices() { m.Sat = make([]MT1001Sat, m.Nsat) }
// ...one per message type (eight observables + MT1013 = nine total).
```

**MT1013 System Parameters** (Table 3.5-15) — fixed header followed by `Nm` 29-bit announcement records:

```go
// MT1013 System Parameters announces the messages this station transmits.
type MT1013 struct {
    MsgHdrStationID                                    // DF002 + DF003
    MJD          uint16 `bits:"16" json:"mjd"`          // DF051 Modified Julian Day
    SecondsOfDay uint32 `bits:"17" json:"secondsOfDay"` // DF052 UTC
    Nm           uint8  `bits:"5"  json:"nm"`           // DF053
    LeapSeconds  uint8  `bits:"8"  json:"leapSeconds"`  // DF054 GPS-UTC
    Announce     []MT1013Announce `json:"announce"`
}

// MT1013Announce is a single message-ID announcement record.
type MT1013Announce struct {
    MsgID      uint16 `bits:"12" json:"msgID"`      // DF055
    Sync       bool              `json:"sync"`      // DF056
    TxInterval uint16 `bits:"16" json:"txInterval"` // DF057 (0.1s units)
}

func (m *MT1013) SizeSlices() { m.Announce = make([]MT1013Announce, m.Nm) }
```

MT1013 uses the same `[]struct` path added to bitsenc in step 1.

### 3. Wire into `ParseMsg` dispatcher and station-ID helper

File: [gps/lib/rtcmbin/rtcmbin.go](../gps/lib/rtcmbin/rtcmbin.go).

- Extend the switch in `ParseMsg` ([rtcmbin.go:71-106](../gps/lib/rtcmbin/rtcmbin.go#L71-L106)) with nine new cases (MT1001-1004, MT1009-1013) following the MT1230 pattern (`bitsenc.NewReader(payload).Read(&msg)` path, since these need SizeSlices-based variable-length decoding).
- No `hasStationID` allow-list to update. `ReferenceStationID` now uses `ParseMsg` + a type assertion against the `StationIDer` interface ([rtcmbin.go](../gps/lib/rtcmbin/rtcmbin.go)), and any message type that embeds `MsgHdrStationID` picks up `RefStationID()` for free. The new `LegacyGPSHeader`, `LegacyGLONASSHeader`, and `MT1013` all embed `MsgHdrStationID`, so station-ID extraction works automatically for MT1001-1004, 1009-1012, and 1013.

### 3a. Register as common message types in the scanner

File: [gps/internal/rtcm/rtcm.go](../gps/internal/rtcm/rtcm.go).

`commonMsgTypes` ([rtcm.go:17-32](../gps/internal/rtcm/rtcm.go#L17-L32)) drives `RescanOnBadChecksum` ([rtcm.go:133-135](../gps/internal/rtcm/rtcm.go#L133-L135)): if a packet with a bad checksum claims to be a "common" MT, the scanner assumes packet framing is intact and doesn't rescan; otherwise it treats the bad CRC as a framing error and rescans for a preamble. Since the plan's goal is compatibility with legacy base stations, the new MTs must be recognised as common — otherwise every 1001-1004/1009-1013 packet with a transient bit error would trigger a byte-by-byte rescan through the rest of that packet.

Insert 1001-1004, 1009-1013 into the list in sorted order (the list is binary-searched by `isCommonMsgType`). Update the corresponding test in [gps/internal/rtcm/rtcm_test.go](../gps/internal/rtcm/rtcm_test.go) if it enumerates the expected set.

### 4. Tests

File: [gps/lib/rtcmbin/mt_test.go](../gps/lib/rtcmbin/mt_test.go).

**Fixtures must come from real captures, not self-generated bytes.** A round-trip test on a struct that was encoded by the same codec it decodes only proves self-consistency; it does not validate field order, widths, or signedness against RTCM. Two independent real-capture sources:

- **UM982 capture** — [gps/testdata/packets/unicore/UM982/rtcm-legacy.jsonl](../gps/testdata/packets/unicore/UM982/rtcm-legacy.jsonl), 10 packets each of MT1001-1004 and MT1009-1012. Primary source covering all eight MTs.
- **Centipede NTRIP capture** (a French public CORS network) — an independent second source that emits MT1004, MT1012, and MT1013. Extract these three packets directly into `mt_test.go` as hex constants:

```go
// MT1004 captured from the Centipede NTRIP network (French CORS).
const rtcm1004Centipede = "d300a53ec0001220afa2a1104808861b6e7fd3ab3fbe457157fca8a9fce6b02d3dfea15dfdfe02bb3feb863825cc024e7ff4abeff37061c7ff7c38b261c023733fa3e0ff6006377ffc12492abd819661fd170ffc9c435bbfe1163b514e04481fe8f85fde408e37fec90383e2400bd97f50a6fea9025b3fee4aa20a8ec16b43fa45f7f6384a39ffaf7570e291fbe81fd4e93fc43f36bbfd23cbbdc0e04d35fe9579feaa0bd1dfef00d7d7a4"

// MT1012 captured from the Centipede NTRIP network.
const rtcm1012Centipede = "d300ab3f400000caa3050841a34246046343fa7ab40581bea1ff26199c894550913bfe8305fe2436467fdf886d7d96dc1883ffa1bf7fab0bd94ff7a2901c507a01b09dea26d01dc0c8337c618365bde0015e8ffa4b2404007219ff5468a8c6a0003481fe8e988002000000001c0181309c0794bfa7af408703b2aff429089a61c4049f7fe9691012815bc3fd12651a3ce9c0b3d3fa2bd7f870499cff70a1288f94900b72fe96c9ffe402d9ffd60049ca77"

// MT1013 captured from the Centipede NTRIP network.
const rtcm1013Centipede = "d300093f5000eed69479004898988b"
```

- Extract one representative packet per MT from the UM982 JSONL into hex constants too (eight `rtcmXXXXUm982` constants for MT1001-1004, MT1009-1012).
- Parse-assert tests: spot-check reference station ID, Nsat, first-satellite SatID, pseudorange/phaserange plausibility on every fixture.
- Round-trip tests: parse -> `SerializeMsg` -> compare bytes for every fixture.
- Cross-source assertion: MT1004 and MT1012 get one fixture from each source; both must decode without error. This is the spec-grounded check — two independent encoders agreeing on layout is far stronger than a codec round-tripping its own output.
- DF047 signedness: at least one GLONASS fixture (MT1011 or MT1012) must exercise a value where signed-vs-unsigned interpretation differs (bit 13 set, i.e. raw value >= 0x2000). The 0x2000 "invalid L2" sentinel is ideal.
- `Nsat == 0` is an edge case; synthesise it as a codec-self-consistency round-trip if neither capture covers it.

Bitsenc extension also gets its own unit test (see step 1).

## Verification

- `make test` — runs the full suite including new rtcmbin and bitsenc tests.
- `go test -v -run 'TestMT100[1-4]|TestMT101[0-3]' ./gps/lib/rtcmbin` — focused check on new types.
- `go test -v -run TestRoundTripSliceOfStruct ./gps/lib/bitsenc` — focused check on codec extension.
- End-to-end sanity check across all 80 captured packets:
  `out/darwin_arm64/satpulsetool annotate gps/testdata/packets/unicore/UM982/rtcm-legacy.jsonl | jq -c 'select(.payload == null)'`
  — any packet where `.payload` is missing indicates the new decoder rejected it. Expect empty output.
- Spot-check a decoded payload for each MT:
  `out/darwin_arm64/satpulsetool annotate gps/testdata/packets/unicore/UM982/rtcm-legacy.jsonl | jq -c 'select(.msg == "1012") | .payload' | head -1` (repeat for 1001..1004, 1009..1011) — sat IDs, nsat, and phase-range values should look plausible.
- MT1013 direct decode (UM982 capture doesn't contain 1013):
  `out/darwin_arm64/satpulsetool decode --bin d300093f5000eed69479004898988b` — expect a payload with `stationID`, `mjd`, `secondsOfDay`, `nm`, `leapSeconds`, and a 3-entry `announce` array (centipede's MT1013 is 15 bytes = 70+29*3 bits payload).

## Critical files

- [gps/lib/bitsenc/bitsenc.go](../gps/lib/bitsenc/bitsenc.go) — extend slice handling in `readStruct` + `writeStruct`.
- [gps/lib/bitsenc/bitsenc_test.go](../gps/lib/bitsenc/bitsenc_test.go) — new test for slice-of-struct.
- [gps/lib/rtcmbin/mt.go](../gps/lib/rtcmbin/mt.go) — new headers, per-sat structs, message types, SizeSlices methods.
- [gps/lib/rtcmbin/rtcmbin.go](../gps/lib/rtcmbin/rtcmbin.go) — nine new `case` clauses in `ParseMsg`. No `hasStationID` edits needed (now handled via `MsgHdrStationID` embedding + `StationIDer` interface).
- [gps/lib/rtcmbin/mt_test.go](../gps/lib/rtcmbin/mt_test.go) — parse + round-trip tests with hex testdata.
- [gps/internal/rtcm/rtcm.go](../gps/internal/rtcm/rtcm.go) — nine additions to `commonMsgTypes` (sorted).
- [gps/internal/rtcm/rtcm_test.go](../gps/internal/rtcm/rtcm_test.go) — update if it enumerates the common-MT set.
