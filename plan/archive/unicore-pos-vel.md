# Unicore position and velocity messages

## Hardware

UM980 connected to `/dev/ttyUSB0` at 115200 baud.

## Overview

Add BESTNAV (geodetic, ID 2118) and BESTNAVXYZ (ECEF, ID 240) message support for the Unicore UM980, producing protocol-neutral `PosGeoMsg`, `VelGeoMsg`, `PosECEFMsg`, `VelECEFMsg`, and `NavEpochMsg`.

Both messages carry position AND velocity in a single message with separate solution status/type fields for each. This is different from UBX where position and velocity come in separate messages.

## Step 0: TOML message file

Create `configs/gpsmsg/um980.toml` with `[[line]]` commands for capturing matched ASCII/binary pairs and for general message control. Unicore commands use `MSGNAMEB rate` for binary and `MSGNAMEA rate` for ASCII, and `UNLOG MSGNAME` to disable.

```toml
# Unicore UM980 GPS message file

[default.line]
delay = 0.1

# BESTNAV binary at 1Hz
[[line]]
tag = "bestnav-bin"
description = "Enable BESTNAV binary at 1Hz"
text = "BESTNAVB 1"

[[line]]
tag = "bestnav-bin-off"
description = "Disable BESTNAV binary"
text = "UNLOG BESTNAVB"

# BESTNAV ASCII at 1Hz
[[line]]
tag = "bestnav-ascii"
description = "Enable BESTNAV ASCII at 1Hz"
text = "BESTNAVA 1"

[[line]]
tag = "bestnav-ascii-off"
description = "Disable BESTNAV ASCII"
text = "UNLOG BESTNAVA"

# BESTNAVXYZ binary at 1Hz
[[line]]
tag = "bestnavxyz-bin"
description = "Enable BESTNAVXYZ binary at 1Hz"
text = "BESTNAVXYZB 1"

[[line]]
tag = "bestnavxyz-bin-off"
description = "Disable BESTNAVXYZ binary"
text = "UNLOG BESTNAVXYZB"

# BESTNAVXYZ ASCII at 1Hz
[[line]]
tag = "bestnavxyz-ascii"
description = "Enable BESTNAVXYZ ASCII at 1Hz"
text = "BESTNAVXYZB 1"

[[line]]
tag = "bestnavxyz-ascii-off"
description = "Disable BESTNAVXYZ ASCII"
text = "UNLOG BESTNAVXYZA"

# Combined tags for capture workflows
[[line]]
tag = "bestnav"
description = "Enable BESTNAV binary+ASCII at 1Hz"
text = "BESTNAVB 1"
[[line]]
tag = "bestnav"
text = "BESTNAVA 1"

[[line]]
tag = "bestnav-off"
description = "Disable BESTNAV binary+ASCII"
text = "UNLOG BESTNAVB"
[[line]]
tag = "bestnav-off"
text = "UNLOG BESTNAVA"

[[line]]
tag = "bestnavxyz"
description = "Enable BESTNAVXYZ binary+ASCII at 1Hz"
text = "BESTNAVXYZB 1"
[[line]]
tag = "bestnavxyz"
text = "BESTNAVXYZA 1"

[[line]]
tag = "bestnavxyz-off"
description = "Disable BESTNAVXYZ binary+ASCII"
text = "UNLOG BESTNAVXYZB"
[[line]]
tag = "bestnavxyz-off"
text = "UNLOG BESTNAVXYZA"
```

After creating the file, capture test data:

```bash
# Enable BESTNAV binary+ASCII, capture 5 seconds
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/um980.toml -t bestnav --capture 5 --packet-log /tmp/bestnav.jsonl

# Enable BESTNAVXYZ binary+ASCII
satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/um980.toml -t bestnavxyz --capture 5 --packet-log /tmp/bestnavxyz.jsonl
```

Extract matched binary/ASCII pairs using jq for test data.

## Step 1: Supporting types in uncmsg

New file: `gps/lib/uncmsg/navtypes.go`. The `SolStatus` and `PosVelType` enums are Unicore-specific (NovAtel has overlapping but different value sets, so these cannot be shared via `novmsg`). Both are `uint32` in binary and symbolic strings in ASCII.

### SolStatus

```go
type SolStatus uint32

const (
	SolComputed      SolStatus = 0
	InsufficientObs  SolStatus = 1
	NoConvergence    SolStatus = 2
	Singularity      SolStatus = 3
	CovTrace         SolStatus = 4
)
```

With `String()`, `ParseSolStatus()`, `MarshalText()`, `UnmarshalText()` following the `TimeRef` pattern in `common.go`.

### PosVelType

Values from `plan/solution-quality.md` Unicore section:

```go
type PosVelType uint32

const (
	PosVelNone            PosVelType = 0  // NONE
	PosVelFixedPos        PosVelType = 1  // FIXEDPOS
	PosVelFixedHeight     PosVelType = 2  // FIXEDHEIGHT
	PosVelDopplerVelocity PosVelType = 8  // DOPPLER_VELOCITY
	PosVelSingle          PosVelType = 16 // SINGLE
	PosVelPSRDiff         PosVelType = 17 // PSRDIFF
	PosVelSBAS            PosVelType = 18 // SBAS
	PosVelL1Float         PosVelType = 32 // L1_FLOAT
	PosVelIonoFreeFloat   PosVelType = 33 // IONOFREE_FLOAT
	PosVelNarrowFloat     PosVelType = 34 // NARROW_FLOAT
	PosVelL1Int           PosVelType = 48 // L1_INT
	PosVelWideInt         PosVelType = 49 // WIDE_INT
	PosVelNarrowInt       PosVelType = 50 // NARROW_INT
	PosVelINS             PosVelType = 52 // INS
	PosVelINSPSRSP        PosVelType = 53 // INS_PSRSP
	PosVelINSPSRDiff      PosVelType = 54 // INS_PSRDIFF
	PosVelINSRTKFloat     PosVelType = 55 // INS_RTKFLOAT
	PosVelINSRTKFixed     PosVelType = 56 // INS_RTKFIXED
	PosVelPPPConverging   PosVelType = 68 // PPP_CONVERGING
	PosVelPPP             PosVelType = 69 // PPP
	PosVelPPPAR           PosVelType = 70 // PPP_AR
	PosVelPPPRTK          PosVelType = 71 // PPP_RTK
)
```

With `String()`, `ParsePosVelType()`, `MarshalText()`, `UnmarshalText()`. Note: value 69 is lowercase `ppp` in the Unicore spec; the parser should accept both `ppp` and `PPP`.

This is the complete set from Unicore protocol spec v1.13 Table 7-168.

### StationID

The BESTNAV station ID field is `[4]byte` in binary and a quoted string `"0"` in ASCII. A custom type with text marshaling to handle the quotes:

```go
type StationID [4]byte

func (s *StationID) UnmarshalText(text []byte) error {
	// Strip surrounding quotes: "0" -> 0
	t := string(text)
	if len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"' {
		t = t[1 : len(t)-1]
	}
	copy(s[:], t)
	return nil
}
```

### Test

Add unit tests in `gps/lib/uncmsg/navtypes_test.go` for enum text round-tripping.

## Step 2: BESTNAV struct in uncmsg

New file: `gps/lib/uncmsg/nav.go`. BESTNAV is a fixed-length message (120 bytes body) using `binary.Read` for binary and `fieldenc.Decode` for ASCII.

```go
const BestNavID MsgID = 2118

type BestNav struct {
	PSolStatus   SolStatus   // position solution status
	PosType      PosVelType  // position type
	Lat          float64            // degrees
	Lon          float64            // degrees
	Hgt          float64            // height above MSL (meters)
	Undulation   float32            // geoid undulation (meters)
	DatumID      uint32             // datum ID (61 = WGS84)
	LatSigma     float32            // latitude std dev (meters)
	LonSigma     float32            // longitude std dev (meters)
	HgtSigma     float32            // height std dev (meters)
	StnID        StationID   // base station ID
	DiffAge      float32            // differential data age (seconds)
	SolAge       float32            // solution age (seconds)
	NumSVs       uint8              // satellites tracked
	NumSolnSVs   uint8              // satellites in solution
	Reserved1    uint8
	Reserved2    uint8
	Reserved3    uint8
	ExtSolStat   uint8              // extended solution status
	GalBDS3Sig   uint8              // Galileo/BDS3 signal mask
	GPSGLOBDS2Sig uint8             // GPS/GLONASS/BDS2 signal mask
	VSolStatus   SolStatus   // velocity solution status
	VelType      PosVelType  // velocity type
	VLatency     float32            // velocity latency (seconds)
	VDiffAge     float32            // velocity differential age (seconds)
	HorSpd       float64            // horizontal speed (m/s)
	TrkGnd       float64            // track over ground (degrees from true north)
	VertSpd      float64            // vertical speed (m/s), positive = up
	VertSpdSigma float32            // vertical speed std dev (m/s)
	HorSpdSigma  float32            // horizontal speed std dev (m/s)
}

func (m *BestNav) ID() (MsgID, string) {
	return BestNavID, "BESTNAVA"
}
```

Remove `BestNavID` from `other.go` and its `idNameMap` entry. Add `regMsg[BestNav]("BESTNAV")` in `init()`.

### Test

New file: `gps/lib/uncmsg/nav_test.go`. Tests follow the `novmsg.dataTestCase` / `uncmsg.dataTestCase` pattern, using captured binary+ASCII packet pairs to verify round-trip parsing. This requires the shared `dataTestCase` / `testDataBin` / `testDataAscii` test infrastructure. Check if uncmsg has its own version; if not, mirror the novmsg pattern.

## Step 3: BESTNAVXYZ struct in uncmsg

Add to `gps/lib/uncmsg/nav.go`:

```go
const BestNavXYZID MsgID = 240

type BestNavXYZ struct {
	PSolStatus   SolStatus
	PosType      PosVelType
	PX           float64            // ECEF X (meters)
	PY           float64            // ECEF Y (meters)
	PZ           float64            // ECEF Z (meters)
	PXSigma      float32            // X std dev (meters)
	PYSigma      float32            // Y std dev (meters)
	PZSigma      float32            // Z std dev (meters)
	VSolStatus   SolStatus
	VelType      PosVelType
	VX           float64            // ECEF VX (m/s)
	VY           float64            // ECEF VY (m/s)
	VZ           float64            // ECEF VZ (m/s)
	VXSigma      float32            // VX std dev (m/s)
	VYSigma      float32            // VY std dev (m/s)
	VZSigma      float32            // VZ std dev (m/s)
	StnID        StationID
	VLatency     float32            // velocity latency (seconds)
	DiffAge      float32            // differential age (seconds)
	SolAge       float32            // solution age (seconds)
	NumSVs       uint8
	NumSolnSVs   uint8
	NumGGL1      uint8              // satellites with L1/G1/B1 signals
	NumSolnMulti uint8              // satellites with multi-frequency
	Reserved     uint8
	ExtSolStat   uint8
	GalBDS3Sig   uint8
	GPSGLOBDS2Sig uint8
}

func (m *BestNavXYZ) ID() (MsgID, string) {
	return BestNavXYZID, "BESTNAVXYZA"
}
```

Remove `BestNavXYZID` from `other.go`. Add `regMsg[BestNavXYZ]("BESTNAVXYZ")` in `init()`.

### Test

Add test cases to `nav_test.go` using captured BESTNAVXYZ binary+ASCII pairs.

## Step 4: PVT message enablement

Update `generatePVTMsgCommands()` in `gps/internal/unc/cfgopts.go`:

```go
func generatePVTMsgCommands(flags gpsprot.PVTMsgFlags, enabledGNSS gpsprot.GNSSSet) []string {
	var cmds []string
	off := flags&gpsprot.PVTMsgOff != 0

	// RECTIMEB for Time/TimePulse (existing)
	if flags&(gpsprot.PVTMsgTime|gpsprot.PVTMsgTimePulse) != 0 {
		cmds = append(cmds, "RECTIMEB 1")
	} else if off {
		cmds = append(cmds, "UNLOG RECTIMEB")
	}

	// BESTNAV / BESTNAVXYZ for position and velocity
	// BESTNAV provides geodetic (LLH) position + velocity
	// BESTNAVXYZ provides ECEF position + velocity
	wantPos := flags&gpsprot.PVTMsgPos != 0
	wantVel := flags&gpsprot.PVTMsgVel != 0
	wantECEF := flags&gpsprot.PVTMsgECEF != 0
	if wantPos || wantVel {
		if wantECEF {
			cmds = append(cmds, "BESTNAVXYZB 1")
		} else {
			cmds = append(cmds, "BESTNAVB 1")
		}
	} else if off {
		cmds = append(cmds, "UNLOG BESTNAVB")
		cmds = append(cmds, "UNLOG BESTNAVXYZB")
	}

	// UTC messages for leap seconds (existing)
	// ...existing code...
}
```

Both position and velocity are in a single BESTNAV/BESTNAVXYZ message, so requesting either `pos` or `vel` enables the same message. The `ecef` flag selects BESTNAVXYZ vs BESTNAV.

### Test

Update `cfgopts_test.go` with test cases for pos, vel, pos+vel, ecef, and off flags.

## Step 5: Extraction functions

New file: `gps/internal/unc/nav.go`. Following the UBX pattern in `ubxpv.go`: extraction functions take an `*uncmsg` struct, return a `*gpsprot` message, and accumulate accuracy into `*gpsprot.NavEpochMsg`.

### Unit conversions

BESTNAV/BESTNAVXYZ use floating-point (float64/float32) in meters and m/s, unlike UBX which uses integers in cm or mm. The gpsprot types use fixed-point (`Length` in micrometers, `Speed` in micrometers/second, `Angle` in nanodegrees).

```go
func metersToLength(v float64) gpsprot.Length {
	return gpsprot.Length(v * float64(gpsprot.Meter))
}

func mpsToSpeed(v float64) gpsprot.Speed {
	return gpsprot.Speed(v * float64(gpsprot.MeterPerSecond))
}

func degreesToAngle(v float64) gpsprot.Angle {
	return gpsprot.Angle(v * float64(gpsprot.Degree))
}
```

Note: Degree constant may not exist in gpsprot; check `types.go`. If not, use `Angle(v * 1e9)` since Angle is in nanodegrees and `DegreesFromFloat` is the constructor.

### BESTNAV extraction

A single function extracts both position and velocity from BESTNAV, since both are in the same message. Position and velocity have independent solution status, so either can be nil.

```go
func bestNavPosVel(ne *gpsprot.NavEpochMsg, m *uncmsg.BestNav) (*gpsprot.PosGeoMsg, *gpsprot.VelGeoMsg) {
	var posG *gpsprot.PosGeoMsg
	var velG *gpsprot.VelGeoMsg
	if m.PSolStatus == uncmsg.SolComputed {
		ne.Acc.Hor.Set(metersToLength(math.Sqrt(
			float64(m.LatSigma)*float64(m.LatSigma) +
			float64(m.LonSigma)*float64(m.LonSigma))))
		ne.Acc.Vert.Set(metersToLength(float64(m.HgtSigma)))
		posG = &gpsprot.PosGeoMsg{
			LatLon:      [2]gpsprot.Angle{degreesToAngle(m.Lat), degreesToAngle(m.Lon)},
			Height:      opt.Make(metersToLength(float64(m.Hgt) + float64(m.Undulation))),
			HeightMSL:   opt.Make(metersToLength(float64(m.Hgt))),
			NativeMsgID: "BESTNAV",
		}
	}
	if m.VSolStatus == uncmsg.SolComputed {
		ne.Acc.GroundSpeed.Set(mpsToSpeed(float64(m.HorSpdSigma)))
		ne.Acc.Speed.Set(mpsToSpeed(math.Sqrt(
			float64(m.HorSpdSigma)*float64(m.HorSpdSigma) +
			float64(m.VertSpdSigma)*float64(m.VertSpdSigma))))
		velG = &gpsprot.VelGeoMsg{
			GroundSpeed: opt.Make(mpsToSpeed(m.HorSpd)),
			Course:      opt.Make(degreesToAngle(m.TrkGnd)),
			NativeMsgID: "BESTNAV",
		}
	}
	return posG, velG
}
```

Note: BESTNAV provides ground speed + track (course) + vertical speed, but NOT NED components directly. VelNED is left unset. VelGeoMsg.Speed3D is not set either since BESTNAV doesn't provide it (could compute from HorSpd and VertSpd but better to only expose what the receiver reports).

### BESTNAVXYZ extraction

```go
func bestNavXYZPosVel(ne *gpsprot.NavEpochMsg, m *uncmsg.BestNavXYZ) (*gpsprot.PosECEFMsg, *gpsprot.VelECEFMsg) {
	var posE *gpsprot.PosECEFMsg
	var velE *gpsprot.VelECEFMsg
	if m.PSolStatus == uncmsg.SolComputed {
		ne.Acc.Pos.Set(metersToLength(math.Sqrt(
			float64(m.PXSigma)*float64(m.PXSigma) +
			float64(m.PYSigma)*float64(m.PYSigma) +
			float64(m.PZSigma)*float64(m.PZSigma))))
		posE = &gpsprot.PosECEFMsg{
			Pos:         gpsprot.Point3D{metersToLength(m.PX), metersToLength(m.PY), metersToLength(m.PZ)},
			NativeMsgID: "BESTNAVXYZ",
		}
	}
	if m.VSolStatus == uncmsg.SolComputed {
		ne.Acc.Speed.Set(mpsToSpeed(math.Sqrt(
			float64(m.VXSigma)*float64(m.VXSigma) +
			float64(m.VYSigma)*float64(m.VYSigma) +
			float64(m.VZSigma)*float64(m.VZSigma))))
		velE = &gpsprot.VelECEFMsg{
			Vel:         [3]gpsprot.Speed{mpsToSpeed(m.VX), mpsToSpeed(m.VY), mpsToSpeed(m.VZ)},
			NativeMsgID: "BESTNAVXYZ",
		}
	}
	return posE, velE
}
```

### Test

New file: `gps/internal/unc/nav_test.go`. Test extraction functions directly with constructed uncmsg structs, verifying gpsprot message fields and accuracy accumulation. Follows the `ubxpv_test.go` pattern.

## Step 6: Epoch tracking and NavEpochMsg

Add epoch tracking to `packetProcessor` in `gps/internal/unc/processor.go`, following the UBX pattern.

### Epoch identification

Unicore messages carry `(Week, MillisecondsOfWeek)` in the header. Messages in the same epoch have the same pair. Combined as `uint64(week)<<32 | uint64(ms)` for comparison.

```go
type packetProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh          gpsprot.MsgHandler // never nil; initialized to &gpsprot.DefaultHandler{}
	curEpoch    uint64              // (week<<32 | ms) + 1; 0 = no epoch
	curEpochMsg *gpsprot.NavEpochMsg
	curEpochTag gpsprot.Tag
}
```

### Which messages participate

All Unicore messages share the same header with `(Week, MillisecondsOfWeek)`, so all dispatched messages can participate in epoch tracking -- not just navigation messages. This differs from UBX, where only NAV-class messages have iTOW. RecTime, SatsInfo, UTC, BESTNAV, and BESTNAVXYZ all carry the same timing header and should have the same `(Week, Ms)` within an epoch. This needs to be verified empirically.

If confirmed, `handleNavEpoch` is called for every dispatched message, and the epoch flush happens when any message arrives with a new `(Week, Ms)`. This simplifies the design: no need to classify messages as "navigation" vs "non-navigation".

### Epoch boundary handling

```go
func (p *packetProcessor) handleEpoch(hdr *uncmsg.MsgHdr, tag gpsprot.Tag, tRead time.Time) {
	e := uint64(hdr.Week)<<32 | uint64(hdr.MillisecondsOfWeek)
	e++ // avoid zero
	if e != p.curEpoch {
		p.flushEpoch(tRead)
		p.curEpoch = e
		p.curEpochMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
		p.curEpochTag = tag
	}
}

func (p *packetProcessor) flushEpoch(tRead time.Time) {
	if p.curEpochMsg != nil {
		p.curEpochMsg.Tag = p.curEpochTag
		p.mh.NavEpoch(p.curEpochMsg, tRead)
	}
	p.curEpochMsg = nil
}
```

Since binary and ASCII packets have different tags (`TagBinary` vs `TagAscii`), the tag of the first message in the epoch is stored and used for the `NavEpochMsg`.

## Step 7: Wire dispatch and DefaultHandler invariant

### DefaultHandler invariant

Initialize `mh` to `&gpsprot.DefaultHandler{}` in both constructors so it is never nil. This eliminates all nil checks on `mh` throughout the processor.

```go
func NewBinPacketProcessor() *BinPacketProcessor {
	return &BinPacketProcessor{packetProcessor{mh: &gpsprot.DefaultHandler{}}}
}

func NewAsciiPacketProcessor() *AsciiPacketProcessor {
	return &AsciiPacketProcessor{packetProcessor{mh: &gpsprot.DefaultHandler{}}}
}
```

Update `processPacket` to remove the `mh != nil` guard:

```go
func (p *packetProcessor) processPacket(bytes []byte, tRead time.Time, tag gpsprot.Tag, msgID string,
	parser func([]byte) (*uncmsg.Msg, error)) error {
	msg, err := parser(bytes)
	if err != nil {
		return err
	}
	handled, err := p.dispatch(msg, tRead, tag)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return nmh.NativeMsg(tag, msgID, msg, tRead)
	}
	return nil
}
```

### Dispatch

Update `dispatch()` to handle `BestNav` and `BestNavXYZ` using the same early-return pattern as existing cases. Every dispatched message calls `handleEpoch` at the top, since all messages carry the same timing header.

```go
func (p *packetProcessor) dispatch(msg *uncmsg.Msg, tRead time.Time, tag gpsprot.Tag) (bool, error) {
	p.handleEpoch(&msg.Hdr, tag, tRead)
	h := p.mh
	switch body := msg.Body.(type) {
	case *uncmsg.BestNav:
		posG, velG := bestNavPosVel(p.curEpochMsg, body)
		if posG != nil {
			posG.Tag = tag
			h.PosGeo(posG, tRead)
		}
		if velG != nil {
			velG.Tag = tag
			h.VelGeo(velG, tRead)
		}
		return posG != nil || velG != nil, nil
	case *uncmsg.BestNavXYZ:
		posE, velE := bestNavXYZPosVel(p.curEpochMsg, body)
		if posE != nil {
			posE.Tag = tag
			h.PosECEF(posE, tRead)
		}
		if velE != nil {
			velE.Tag = tag
			h.VelECEF(velE, tRead)
		}
		return posE != nil || velE != nil, nil
	case *uncmsg.RecTime:
		// ...existing early-return...
	case *uncmsg.SatsInfo:
		// ...existing early-return...
	// ...existing UTC cases with early-return...
	default:
		return false, nil
	}
}
```

Each case handles its own dispatch and returns immediately. No central dispatch block needed.

## Implementation order

1. **Step 0**: Create um980.toml, capture test data
2. **Step 1**: Add SolStatus, PosVelType, StationID to uncmsg
3. **Step 2**: BestNav struct in uncmsg + tests
4. **Step 3**: BestNavXYZ struct in uncmsg + tests
5. **Step 4**: PVT config in cfgopts.go + tests
6. **Step 5**: Extraction functions in unc/nav.go + tests
7. **Step 6+7**: Epoch tracking + dispatch wiring + tests
8. `make test` to confirm nothing broken

Steps 1-3 are pure library code with no behavioral changes. Step 4 enables configuration. Steps 5-7 wire everything together.

## Notes

### Height convention

BESTNAV's `Hgt` field is height above MSL. Ellipsoidal height = `Hgt + Undulation`. PosGeoMsg.Height is above WGS-84 ellipsoid; PosGeoMsg.HeightMSL is above mean sea level.

### Velocity convention

BESTNAV's `VertSpd` is positive upward. VelGeoMsg.VelNED[2] (down component) would be `-VertSpd`. Since BESTNAV doesn't provide NED components directly, we don't set VelNED. We only set GroundSpeed and Course.

### Accuracy from sigma values

BESTNAV provides separate lat/lon/hgt sigmas rather than combined H/V accuracy:
- `Acc.Hor` = sqrt(LatSigma^2 + LonSigma^2)
- `Acc.Vert` = HgtSigma

BESTNAVXYZ provides separate X/Y/Z sigmas:
- `Acc.Pos` = sqrt(PXSigma^2 + PYSigma^2 + PZSigma^2)

### Separate position and velocity validity

Each message has independent position and velocity solution status. `posGeoBestNav` returns nil when position is invalid; `velGeoBestNav` returns nil when velocity is invalid. Both can independently succeed or fail.

### Future: high-rate messages

BESTNAVH (ID 2119) and BESTNAVXYZH (ID 242) are slave-antenna variants for dual-antenna receivers. They use the same field layout. When needed, they can share the same extraction functions.
