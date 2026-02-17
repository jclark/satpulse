# NovAtel BESTPOS/BESTXYZ message support

## Hardware

UM980 connected to `/dev/ttyUSB0` at 115200 baud. UM980 outputs BESTPOS and BESTXYZ using NovAtel binary packet format (AA 44 12 sync bytes), not Unicore format.

## Overview

Add BESTPOS (geodetic position, NovAtel ID 42) and BESTXYZ (ECEF position+velocity, NovAtel ID 241) message support to `novmsg` (parsing library) and `nov` (processor).

BESTPOS and BESTXYZ have identical binary layouts across NovAtel OEM7, ByNav, SinoGNSS, and Unicore. The only difference is vendor-specific SolStatus and PosType/VelType enum value sets. We use generic structs `novmsg.Pos[S, P]` and `novmsg.XYZ[S, P]` parameterized by these enum types, and vendor-specific types embed them.

### Layering

`novmsg` and `uncmsg` are library layer packages; they cannot import domain layer packages like `gpsprot`. Generic shared types (`Pos[S,P]`, `XYZ[S,P]`, `StationID`, `DatumID`, `HexByte`) live in `novmsg`. Generic extraction helpers (`PosGeo`, `PosECEFXYZ`, `VelECEFXYZ`) live in `gps/internal/nov` (domain layer), which can import `gpsprot`. `gps/internal/unc` imports `gps/internal/nov` to use them.

## Spec

ByNav spec of BESTPOS (message ID 42).

### ASCII example

```
#BESTPOSA,COM3,0,0.0,FINESTEERING,1975,393343.000,00000000,0000,113;SOL_COMPUTED,SINGLE,28.23315179260,112.87713400113,79.7665,17.0381,WGS84,1.2642,1.6209,2.1834,"0",0.000,0.022,28,27,27,27,0,00,30,13*DB49BF3D
```

### Body fields

| # | Field | Description | Binary format | Binary bytes | Binary offset |
|---|-------|-------------|---------------|-------------|---------------|
| 1 | Header | See standard header | - | H | 0 |
| 2 | sol stat | Solution status (Table 4-1) | Enum | 4 | H+0 |
| 3 | pos type | Position type (Table 4-2) | Enum | 4 | H+4 |
| 4 | lat | Latitude (deg) | Double | 8 | H+8 |
| 5 | lon | Longitude (deg) | Double | 8 | H+16 |
| 6 | hgt | Height above mean sea level (m) | Double | 8 | H+24 |
| 7 | undulation | Geoid undulation (m) | Float | 4 | H+32 |
| 8 | datum id | Datum ID number | Enum | 4 | H+36 |
| 9 | lat sigma | Latitude std dev (m) | Float | 4 | H+40 |
| 10 | lon sigma | Longitude std dev (m) | Float | 4 | H+44 |
| 11 | hgt sigma | Height std dev (m) | Float | 4 | H+48 |
| 12 | stn id | Base station ID, 0 for single point | Char | 4 | H+52 |
| 13 | diff age | Differential age (s) | Float | 4 | H+56 |
| 14 | sol age | Solution age (s) | Float | 4 | H+60 |
| 15 | #SVs | Satellites tracked | Uchar | 1 | H+64 |
| 16 | #solnSVs | Satellites used in solution | Uchar | 1 | H+65 |
| 17 | #solnL1SVs | Satellites with L1/E1/B1 signals in solution | Uchar | 1 | H+66 |
| 18 | #solnMultiSVs | Satellites with multi-frequency signals in solution | Uchar | 1 | H+67 |
| 19 | reserved | Reserved | Uchar | 1 | H+68 |
| 20 | ext sol stat | Extended solution status (Table 4-3) | Hex | 1 | H+69 |
| 21 | Galileo/BDS sig mask | Galileo and BeiDou signal mask (Table 4-5) | Hex | 1 | H+70 |
| 22 | GPS/GLO sig mask | GPS and GLONASS signal mask (Table 4-4) | Hex | 1 | H+71 |
| 23 | CRC | 32-bit CRC | Hex | 4 | H+72 |

Body length: 72 bytes (+ 4 byte CRC).

### Solution status (Table 4-1)

| Value | Name | Description |
|-------|------|-------------|
| 0 | SOL_COMPUTED | Solution computed |
| 1 | INSUFFICIENT_OBS | Insufficient observations |
| 2 | NO_CONVERGENCE | No convergence |
| 3 | SINGULARITY | Singularity at parameters matrix |
| 4 | COV_TRACE | Covariance trace exceeds maximum (trace > 1000 m) |
| 5 | TEST_DIST | Test distance exceeded (max 3 rejections if distance > 10 km) |
| 6 | COLD_START | Not yet converged from cold start |
| 7 | V_H_LIMIT | Height or velocity limits exceeded |
| 8 | VARIANCE | Variance exceeds limits |
| 9 | RESIDUALS | Residuals are too large |
| 13 | INTEGRITY_WARNING | Large residuals make position unreliable |
| 18 | PENDING | Not enough satellites to verify FIX position |
| 19 | INVALID_FIX | Fixed position entered via FIX command is not valid |
| 20 | UNAUTHORIZED | Position type is unauthorized |
| 22 | INVALID_RATE | Selected logging rate not supported by this solution type |

### Position type (Table 4-2)

| Value | Name | Description |
|-------|------|-------------|
| 0 | NONE | No solution |
| 1 | FIXEDPOS | Position fixed by FIX position command |
| 2 | FIXEDHEIGHT | Position fixed by FIX height/auto command |
| 4 | FLOATCONV | Floating carrier phase ambiguity solution |
| 5 | WIDELANE | Wide lane ambiguity solution |
| 6 | NARROWLANE | Narrow lane ambiguity solution |
| 8 | DOPPLER_VELOCITY | Velocity computed using instantaneous Doppler |
| 16 | SINGLE | Single point solution |
| 17 | PSRDIFF | Pseudorange differential |
| 18 | WAAS | SBAS solution |
| 19 | PROPAGATED | Propagated by Kalman filter without new observations |
| 32 | L1_FLOAT | L1 float solution |
| 33 | IONOFREE_FLOAT | Ionosphere-free floating point |
| 34 | NARROW_FLOAT | RTK float with unresolved carrier phase ambiguities |
| 48 | L1_INT | Reserved |
| 49 | WIDE_INT | RTK fixed, wide-lane integers |
| 50 | NARROW_INT | RTK fixed, narrow-lane integers |
| 51 | RTK_DIRECT_INS | RTK initialized directly through INS |
| 52 | INS_SBAS | INS position after antenna calibration |
| 53 | INS_PSRSP | INS pseudorange single point (no DGPS) |
| 54 | INS_PSRDIFF | INS pseudorange differential |
| 55 | INS_RTKFLOAT | INS RTK float |
| 56 | INS_RTKFIXED | INS RTK fixed |
| 68 | PPP_CONVERGING | Converging PPP solution |
| 69 | PPP | Converged PPP solution |
| 70 | OPERATIONAL | Solution accuracy within UAL operational limit |
| 71 | WARNING | Solution accuracy outside UAL operational limit but within warning limit |
| 72 | OUT_OF_BOUNDS | Solution accuracy outside UAL limits |
| 73 | INS_PPP_CONVERGING | Converging INS PPP solution |
| 74 | INS_PPP | Converged INS PPP solution |
| 77 | PPP_BASIC_CONVERGING | Converging PPP solution (TerraStar-L) |
| 78 | PPP_BASIC | Converged PPP solution (TerraStar-L) |
| 79 | INS_PPP_BASIC_CONVERGING | Converging INS PPP solution (TerraStar-L) |
| 80 | INS_PPP_BASIC | Converged INS PPP solution (TerraStar-L) |

## BESTXYZ spec

OEM7 BESTXYZ (message ID 241). Binary layout is identical across OEM7, ByNav, SinoGNSS (BESTXYZ, ID 241), and Unicore (BESTNAVXYZ, ID 240).

### ASCII example

```
#BESTXYZA,USB1,0,59.0,FINESTEERING,2209,502264.000,02000020,44cf,16809;
SOL_COMPUTED,PPP,-1632848.2165,-3662158.6200,4944901.2721,0.0134,0.0197,0.0259,
SOL_COMPUTED,PPP,0.0007,0.0011,0.0009,0.0055,0.0069,0.0103,"TSTR",0.250,14.000,
0.000,44,40,40,39,0,00,7f,37*868939b6
```

### Body fields

| # | Field | Description | Binary format | Binary bytes | Binary offset |
|---|-------|-------------|---------------|-------------|---------------|
| 1 | Header | See standard header | - | H | 0 |
| 2 | P-sol status | Solution status | Enum | 4 | H+0 |
| 3 | pos type | Position type | Enum | 4 | H+4 |
| 4 | P-X | Position X-coordinate (m) | Double | 8 | H+8 |
| 5 | P-Y | Position Y-coordinate (m) | Double | 8 | H+16 |
| 6 | P-Z | Position Z-coordinate (m) | Double | 8 | H+24 |
| 7 | P-X sigma | Std dev of P-X (m) | Float | 4 | H+32 |
| 8 | P-Y sigma | Std dev of P-Y (m) | Float | 4 | H+36 |
| 9 | P-Z sigma | Std dev of P-Z (m) | Float | 4 | H+40 |
| 10 | V-sol status | Velocity solution status | Enum | 4 | H+44 |
| 11 | vel type | Velocity type | Enum | 4 | H+48 |
| 12 | V-X | Velocity X (m/s) | Double | 8 | H+52 |
| 13 | V-Y | Velocity Y (m/s) | Double | 8 | H+60 |
| 14 | V-Z | Velocity Z (m/s) | Double | 8 | H+68 |
| 15 | V-X sigma | Std dev of V-X (m/s) | Float | 4 | H+76 |
| 16 | V-Y sigma | Std dev of V-Y (m/s) | Float | 4 | H+80 |
| 17 | V-Z sigma | Std dev of V-Z (m/s) | Float | 4 | H+84 |
| 18 | stn ID | Base station ID | Char | 4 | H+88 |
| 19 | V-latency | Velocity latency (s) | Float | 4 | H+92 |
| 20 | diff_age | Differential age (s) | Float | 4 | H+96 |
| 21 | sol_age | Solution age (s) | Float | 4 | H+100 |
| 22 | #SVs | Satellites tracked | Uchar | 1 | H+104 |
| 23 | #solnSVs | Satellites in solution | Uchar | 1 | H+105 |
| 24 | #ggL1 | Satellites with L1/E1/B1 in solution | Uchar | 1 | H+106 |
| 25 | #solnMultiSVs | Satellites with multi-frequency in solution | Uchar | 1 | H+107 |
| 26 | Reserved | Reserved | Char | 1 | H+108 |
| 27 | ext sol stat | Extended solution status | Hex | 1 | H+109 |
| 28 | Galileo/BDS sig mask | Galileo and BeiDou signal mask | Hex | 1 | H+110 |
| 29 | GPS/GLO sig mask | GPS and GLONASS signal mask | Hex | 1 | H+111 |
| 30 | CRC | 32-bit CRC | Hex | 4 | H+112 |

Body length: 112 bytes (+ 4 byte CRC).

SolStatus and PosType/VelType enums are the same as BESTPOS (Tables 4-1 and 4-2).

## Step 0: TOML message file (done)

BESTPOS and BESTXYZ entries have been added to `configs/gpsmsg/um980.toml` and test captures verified.

### BESTPOS capture

Binary (NOVB, msg ID 42) and ASCII (NOVA, #BESTPOSA) packet pairs captured successfully.

```bash
out/amd64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/um980.toml -t nov-bestpos --capture 5 --packet-log /tmp/bestpos.jsonl
```

Captured packets: `/tmp/bestpos.jsonl` (5 binary+ASCII pairs from UM980).

### BESTXYZ capture

Binary (NOVB, msg ID 241) and ASCII (NOVA, #BESTXYZA) packet pairs captured successfully.

```bash
out/amd64/satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/um980.toml -t nov-bestxyz --capture 5 --packet-log /tmp/bestxyz.jsonl
```

Captured packets: `/tmp/bestxyz.jsonl` (5 binary+ASCII pairs from UM980). Note: BESTPOS packets also appear in this capture since they were already enabled.

## Step 1: Shared types and generic Pos (done)

### Shared types in novmsg

Created `gps/lib/novmsg/navtypes.go` with types moved from `uncmsg`:

- `StationID [4]byte` -- binary: 4 bytes, ASCII: quoted string `"0"`
- `DatumID uint32` -- binary: uint32 (61 = WGS84), ASCII: symbolic `"WGS84"`
- `HexByte uint8` -- binary: byte, ASCII: hex-formatted integer `%02x`

Updated `gps/lib/uncmsg/navtypes.go` to use type aliases:
```go
type DatumID = novmsg.DatumID
type HexByte = novmsg.HexByte
type StationID = novmsg.StationID
const DatumWGS84 = novmsg.DatumWGS84
```

### Generic Pos in novmsg

Generic position struct in `gps/lib/novmsg/navtypes.go`, parameterized by vendor-specific SolStatus (S) and PosType (P) enums:

```go
type Pos[S, P ~uint32] struct {
	PSolStatus    S
	PosType       P
	Lat           float64
	Lon           float64
	Hgt           float64
	Undulation    float32
	DatumID       DatumID
	LatSigma      float32
	LonSigma      float32
	HgtSigma      float32
	StnID         StationID
	DiffAge       float32
	SolAge        float32
	NumSVs        uint8
	NumSolnSVs    uint8
	NumSolnL1SVs  uint8
	NumSolnMulti  uint8
	Reserved      uint8
	ExtSolStat    HexByte
	GalBDS3Sig    HexByte
	GPSGLOBDS2Sig HexByte
}
```

72 bytes body: 4 (S) + 4 (P) + 64 (position fields) = 72. `binary.Read` and `fieldenc` both work correctly with instantiated generic structs.

Note: `Reserved` is `uint8` (not `HexByte`) because the receiver formats it as decimal in ASCII ("1" not "01").

### BestNav embedding

Updated `gps/lib/uncmsg/nav.go` to embed `Pos`:

```go
type BestNav struct {
	novmsg.Pos[SolStatus, PosVelType]
	VSolStatus   SolStatus
	VelType      PosVelType
	VLatency     float32
	VDiffAge     float32
	HorSpd       float64
	TrkGnd       float64
	VertSpd      float64
	VertSpdSigma float32
	HorSpdSigma  float32
}
```

Embedded field name is `Pos`. Promoted fields: `m.PSolStatus`, `m.Lat`, etc.

### Generic XYZ in novmsg

Generic ECEF position+velocity struct in `gps/lib/novmsg/navtypes.go`. Binary layout is identical across OEM7 BESTXYZ (ID 241), SinoGNSS BESTXYZ (ID 241), and Unicore BESTNAVXYZ (ID 240):

```go
type XYZ[S, P ~uint32] struct {
	PSolStatus    S
	PosType       P
	PX            float64
	PY            float64
	PZ            float64
	PXSigma       float32
	PYSigma       float32
	PZSigma       float32
	VSolStatus    S
	VelType       P
	VX            float64
	VY            float64
	VZ            float64
	VXSigma       float32
	VYSigma       float32
	VZSigma       float32
	StnID         StationID
	VLatency      float32
	DiffAge       float32
	SolAge        float32
	NumSVs        uint8
	NumSolnSVs    uint8
	NumSolnL1SVs  uint8
	NumSolnMulti  uint8
	Reserved      uint8
	ExtSolStat    HexByte
	GalBDS3Sig    HexByte
	GPSGLOBDS2Sig HexByte
}
```

112 bytes body. Vendor-specific enum fields (S, P) appear at both the start (PSolStatus, PosType) and middle (VSolStatus, VelType) of the struct, which is why a generic type is needed rather than a non-generic embedded body.

### BestNavXYZ embedding

Updated `gps/lib/uncmsg/nav.go` to embed `XYZ`:

```go
type BestNavXYZ struct {
	novmsg.XYZ[SolStatus, PosVelType]
}
```

Embedded field name is `XYZ`. Renamed `NumGGL1` to `NumSolnL1SVs` (consistent with `Pos`).

### PosGeo in nov

Generic extraction helper in `gps/internal/nov/nav.go`:

```go
func PosGeo[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.Pos[S, P], nativeMsgID string) *gpsprot.PosGeoMsg
```

This lives in `gps/internal/nov` (domain layer) because it imports `gpsprot`. Callers pass the embedded struct directly: `nov.PosGeo(ne, &m.Pos, "BESTNAV")`. Go infers the type parameters from the argument.

### PosECEFXYZ and VelECEFXYZ in nov (TODO)

Generic extraction helpers for XYZ messages, following the same pattern as `PosGeo`:

```go
func PosECEFXYZ[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.XYZ[S, P], nativeMsgID string) *gpsprot.PosECEFMsg
func VelECEFXYZ[S, P ~uint32](ne *gpsprot.NavEpochMsg, b *novmsg.XYZ[S, P], nativeMsgID string) *gpsprot.VelECEFMsg
```

Updated `gps/internal/unc/nav.go` to call these instead of inline conversion.

## Step 2: NovAtel-specific types in novmsg

Add NovAtel-specific enums to `gps/lib/novmsg/navtypes.go`:

### SolStatus

NovAtel's solution status enum (Table 4-1). Superset of Unicore's 5-value set.

```go
type SolStatus uint32

const (
	SolComputed      SolStatus = 0
	InsufficientObs  SolStatus = 1
	NoConvergence    SolStatus = 2
	Singularity      SolStatus = 3
	CovTrace         SolStatus = 4
	TestDist         SolStatus = 5
	ColdStart        SolStatus = 6
	VHLimit          SolStatus = 7
	Variance         SolStatus = 8
	Residuals        SolStatus = 9
	IntegrityWarning SolStatus = 13
	Pending          SolStatus = 18
	InvalidFix       SolStatus = 19
	Unauthorized     SolStatus = 20
	InvalidRate      SolStatus = 22
)
```

With `String()`, `ParseSolStatus()`, `MarshalText()`, `UnmarshalText()`.

### PosType

NovAtel's position/velocity type enum (Table 4-2). Based on OEM7/ByNav values.

```go
type PosType uint32

const (
	PosNone                  PosType = 0
	PosFixedPos              PosType = 1
	PosFixedHeight           PosType = 2
	PosDopplerVelocity       PosType = 8
	PosSingle                PosType = 16
	PosPSRDiff               PosType = 17
	PosWAAS                  PosType = 18
	PosPropagated            PosType = 19
	PosL1Float               PosType = 32
	PosNarrowFloat           PosType = 34
	PosL1Int                 PosType = 48
	PosWideInt               PosType = 49
	PosNarrowInt             PosType = 50
	PosRTKDirectINS          PosType = 51
	PosINSSBAS               PosType = 52
	PosINSPSRSP              PosType = 53
	PosINSPSRDiff            PosType = 54
	PosINSRTKFloat           PosType = 55
	PosINSRTKFixed           PosType = 56
	PosExtConstrained        PosType = 67
	PosPPPConverging         PosType = 68
	PosPPP                   PosType = 69
	PosOperational           PosType = 70
	PosWarning               PosType = 71
	PosOutOfBounds           PosType = 72
	PosINSPPPConverging      PosType = 73
	PosINSPPP                PosType = 74
	PosPPPBasicConverging    PosType = 77
	PosPPPBasic              PosType = 78
	PosINSPPPBasicConverging PosType = 79
	PosINSPPPBasic           PosType = 80
)
```

With `String()`, `ParsePosType()`, `MarshalText()`, `UnmarshalText()`.

### Test

Add NovAtel enum round-trip tests to `gps/lib/novmsg/navtypes_test.go`.

## Step 3: BestPos struct in novmsg

New file: `gps/lib/novmsg/nav.go`. BESTPOS embeds the generic `Pos` with NovAtel-specific enum types:

```go
const BestPosID MsgID = 42

type BestPos struct {
	Pos[SolStatus, PosType]
}

func (m *BestPos) ID() (MsgID, string) {
	return BestPosID, "BESTPOSA"
}

func init() {
	regMsg[BestPos]("BESTPOS")
}
```

72 bytes body: the instantiated `Pos[SolStatus, PosType]` is exactly 72 bytes. `binary.Read` and `fieldenc` both work correctly with embedded instantiated generic structs.

### Test

New file: `gps/lib/novmsg/nav_test.go`. Tests use captured binary+ASCII packet pairs from the UM980, following the `dataTestCase` / `testDataBin` / `testDataAscii` pattern.

## Step 4: Extraction and dispatch in nov

### Extraction

In existing `gps/internal/nov/nav.go`, add:

```go
func bestPosPosGeo(ne *gpsprot.NavEpochMsg, m *novmsg.BestPos) *gpsprot.PosGeoMsg {
	if m.PSolStatus != novmsg.SolComputed {
		return nil
	}
	return PosGeo(ne, &m.Pos, "BESTPOS")
```

### DefaultHandler invariant

Initialize `mh` to `&gpsprot.DefaultHandler{}` in both constructors so it is never nil:

```go
func NewBinPacketProcessor() *BinPacketProcessor {
	return &BinPacketProcessor{packetProcessor{mh: &gpsprot.DefaultHandler{}}}
}

func NewAsciiPacketProcessor() *AsciiPacketProcessor {
	return &AsciiPacketProcessor{packetProcessor{mh: &gpsprot.DefaultHandler{}}}
}
```

Update `processPacket` to remove the `mh != nil` guard.

### Epoch tracking

Add epoch fields to `packetProcessor`, same pattern as unc:

```go
type packetProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh          gpsprot.MsgHandler // never nil
	curEpoch    uint64              // (week<<32 | ms) + 1; 0 = no epoch
	curEpochMsg *gpsprot.NavEpochMsg
	curEpochTag gpsprot.Tag
}
```

Add `handleEpoch` and `flushEpoch` methods (identical logic to unc).

### Dispatch

Update `dispatch()` to call `handleEpoch` for every message, and add `BestPos` case:

```go
func (p *packetProcessor) dispatch(hdr *novmsg.MsgHdr, body novmsg.MsgBody, tRead time.Time, tag gpsprot.Tag) (bool, error) {
	p.handleEpoch(hdr, tag, tRead)
	h := p.mh
	switch m := body.(type) {
	case *novmsg.BestPos:
		posG := bestPosPosGeo(p.curEpochMsg, m)
		if posG != nil {
			posG.Tag = tag
			h.PosGeo(posG, tRead)
			return true, nil
		}
		return false, nil
	case *novmsg.Time:
		// ...existing...
	case *novmsg.IonUTC:
		// ...existing...
	}
	return false, nil
}
```

### Test

- `gps/internal/nov/nav_test.go`: extraction tests (sol computed, sol not computed)
- Update processor tests for epoch tracking and BESTPOS dispatch

## Step 5: PVT message enablement (deferred)

BESTPOS is position-only and uses NovAtel packet format. Since BESTNAV already covers position+velocity via Unicore format, cfgopts integration is deferred. BESTPOS is enabled/disabled via TOML message file for manual testing.

## Implementation order

1. **Step 0**: Done -- TOML entries added, capture verified
2. **Step 1**: Done -- shared types, generic `Pos[S, P]` in novmsg, generic `PosGeo` in nov, BestNav embedding, unc refactored
3. **Step 1b**: Add generic `XYZ[S, P]` in novmsg, refactor `BestNavXYZ` to embed it, add generic `PosECEFXYZ`/`VelECEFXYZ` helpers in nov, refactor unc
4. **Step 2**: Add NovAtel SolStatus + PosType enums to novmsg + tests
5. **Step 3**: BestPos struct in novmsg + tests with captured data
6. **Step 4**: Extraction + epoch tracking + dispatch in nov + tests
7. `make test` to confirm everything passes

Steps 1/1b are refactoring with no new functionality. Steps 2-3 are pure library code. Step 4 wires everything together.

## Notes

### Height convention

`Hgt` is height above MSL. Ellipsoidal height = `Hgt + Undulation`. PosGeoMsg.Height is above WGS-84 ellipsoid; PosGeoMsg.HeightMSL is above mean sea level.

### No velocity

BESTPOS is position-only. NovAtel provides velocity via a separate BESTVEL message (ID 99). BESTVEL support can be added later following the same pattern.

### Enum differences across vendors

BESTPOS (NovAtel protocol) is implemented by multiple vendors with the same binary layout but different SolStatus and PosType enum value sets. ByNav SolStatus is a superset of SinoGNSS and Unicore. PosType has incompatible assignments at several values.

**SolStatus**: ByNav is a superset of both SinoGNSS (4 values) and Unicore (5 values). No conflicts.

**PosType comparison across vendors:**

| Value | OEM7 | SinoGNSS | ByNav | Unicore | Description (OEM7) |
|------:|------|----------|-------|---------|--------------------|
| 0 | NONE | NONE | NONE | NONE | No solution |
| 1 | FIXEDPOS | FIXEDPOS | FIXEDPOS | FIXEDPOS | Fixed by FIX position command or position averaging |
| 2 | FIXEDHEIGHT | | FIXEDHEIGHT | FIXEDHEIGHT | Fixed by FIX height/auto command or position averaging |
| 4 | | | FLOATCONV | | |
| 5 | | | WIDELANE | | |
| 6 | | | NARROWLANE | | |
| 8 | DOPPLER_VELOCITY | DOPPLER_VELOCITY | DOPPLER_VELOCITY | DOPPLER_VELOCITY | Velocity computed using instantaneous Doppler |
| 9 | | SINGLE_SMOOTH | | | |
| 16 | SINGLE | SINGLE | SINGLE | SINGLE | Solution using only GNSS satellite data |
| 17 | PSRDIFF | PSRDIFF | PSRDIFF | PSRDIFF | Pseudorange differential (DGPS/DGNSS) corrections |
| 18 | WAAS | SBAS | WAAS | SBAS | Solution using corrections from an SBAS satellite |
| 19 | PROPAGATED | | PROPAGATED | | Propagated by Kalman filter without new observations |
| 32 | L1_FLOAT | | L1_FLOAT | L1_FLOAT | Single-frequency RTK, unresolved float ambiguities |
| 33 | | | IONOFREE_FLOAT | IONOFREE_FLOAT | |
| 34 | NARROW_FLOAT | NARROW_FLOAT | NARROW_FLOAT | NARROW_FLOAT | Multi-frequency RTK, unresolved float ambiguities |
| 35 | | FIX_DERIVATION | | | |
| 48 | L1_INT | | L1_INT | L1_INT | Single-frequency RTK, integer ambiguities resolved |
| 49 | WIDE_INT | WIDE_INT | WIDE_INT | WIDE_INT | Multi-frequency RTK, wide-lane integers |
| 50 | NARROW_INT | NARROW_INT | NARROW_INT | NARROW_INT | Multi-frequency RTK, narrow-lane integers |
| 51 | RTK_DIRECT_INS | **SUPER WIDE_LANE** | RTK_DIRECT_INS | | RTK filter directly initialized from INS filter |
| 52 | INS_SBAS | | INS_SBAS | **INS** | INS position, last update used SBAS solution |
| 53 | INS_PSRSP | | INS_PSRSP | INS_PSRSP | INS position, last update used SINGLE solution |
| 54 | INS_PSRDIFF | | INS_PSRDIFF | INS_PSRDIFF | INS position, last update used PSRDIFF solution |
| 55 | INS_RTKFLOAT | | INS_RTKFLOAT | INS_RTKFLOAT | INS position, last update used float RTK |
| 56 | INS_RTKFIXED | | INS_RTKFIXED | INS_RTKFIXED | INS position, last update used fixed RTK |
| 67 | EXT_CONSTRAINED | | | | INS position, last update used external source |
| 68 | PPP_CONVERGING | PPP_CONVERGING | PPP_CONVERGING | PPP_CONVERGING | Converging TerraStar-C PRO / TerraStar-X |
| 69 | PPP | PPP | PPP | PPP | Converged TerraStar-C PRO / TerraStar-X |
| 70 | OPERATIONAL | | OPERATIONAL | **PPP_AR** | Solution accuracy within UAL operational limit |
| 71 | WARNING | | WARNING | **PPP_RTK** | Solution accuracy outside UAL operational limit |
| 72 | OUT_OF_BOUNDS | | OUT_OF_BOUNDS | | Solution accuracy outside UAL limits |
| 73 | INS_PPP_CONVERGING | | INS_PPP_CONVERGING | | INS position, last update used converging PPP |
| 74 | INS_PPP | | INS_PPP | | INS position, last update used converged PPP |
| 77 | PPP_BASIC_CONVERGING | | PPP_BASIC_CONVERGING | | Converging TerraStar-L |
| 78 | PPP_BASIC | | PPP_BASIC | | Converged TerraStar-L |
| 79 | INS_PPP_BASIC_CONVERGING | | INS_PPP_BASIC_CONVERGING | | INS position, last update used converging TerraStar-L |
| 80 | INS_PPP_BASIC | | INS_PPP_BASIC | | INS position, last update used converged TerraStar-L |

**Incompatible values** (same integer, different meaning):
- **51**: RTK_DIRECT_INS (OEM7/ByNav) vs SUPER WIDE_LANE (SinoGNSS)
- **52**: INS_SBAS (OEM7/ByNav) vs INS (Unicore)
- **70**: OPERATIONAL (OEM7/ByNav) vs PPP_AR (Unicore)
- **71**: WARNING (OEM7/ByNav) vs PPP_RTK (Unicore)

Value 18 (WAAS vs SBAS) is the same thing with different names, not a real conflict.

**ByNav diverges from OEM7** at values 4-6 (FLOATCONV/WIDELANE/NARROWLANE) and 33 (IONOFREE_FLOAT), which OEM7 marks as Reserved. OEM7 has 67 (EXT_CONSTRAINED) which ByNav lacks.

**SinoGNSS diverges from OEM7** at values 9 (SINGLE_SMOOTH), 35 (FIX_DERIVATION), and 51 (SUPER WIDE_LANE vs RTK_DIRECT_INS).

**Unicore diverges from OEM7** at values 52 (INS vs INS_SBAS), 70 (PPP_AR vs OPERATIONAL), and 71 (PPP_RTK vs WARNING).

This means `novmsg.PosType` and `uncmsg.PosVelType` must remain separate types. How to handle the novmsg PosType values (OEM7 baseline vs vendor-specific additions) is TBD.

### Future: BESTVEL

BESTVEL (NovAtel ID 99) provides velocity (ground speed, track, vertical speed) in the same NovAtel packet format. It can share the same SolStatus/PosType enums and follow the same extraction pattern. When added, it would produce VelGeoMsg.
