# CASIC V6 NAV2 position, velocity, and solution quality

CASIC V6 (class 0x11 NAV2) firmware runs on ZKW F8 dual-band receivers (e.g. ATGM332D-6N74). This plan covers adding full NAV2 message support: binary parsing in `casbin`, extraction and dispatch in `internal/casic`, position/velocity messages, nav epoch, and solution quality.

Prerequisite: position/velocity message types (`PosGeoMsg`, `PosECEFMsg`, `VelGeoMsg`, `VelECEFMsg`) and `MsgHandler` methods must already exist in `gpsprot` (see [position-velocity-messages.md](position-velocity-messages.md)).

Prerequisite: `curNavEpochMsg` on the CASIC `PacketProcessor` must already exist (see [casic-pos-vel-qual.md](casic-pos-vel-qual.md) step 2a).

## Phase 0: Empirical verification (completed)

Verified on ATGM332D-6N74 (V6 firmware), 2026-03-03. Full notes in [casic-nav2-notes.md](casic-nav2-notes.md).

### NAV2-SAT not supported

NAV2-SAT (0x11 0x04) enable via CFG-MSG is ACK'd but the receiver never outputs NAV2-SAT messages. NAV2-SIG is a superset (per-signal with correction and solution flags) and works. Use NAV2-SIG for both satellite info and correction disambiguation.

### 0a: GNSSID mapping (verified)

The `gnssid` field in NAV2-SIG uses the **same** numbering as V5 `casbin.GNSSID` and `Nav2TimeSrc`, extended with GAL, QZSS, SBAS, IRNSS:

| gnssid | System |
|--------|--------|
| 0 | GPS |
| 1 | BDS |
| 2 | GLONASS |
| 3 | Galileo |
| 4 | QZSS |
| 5 | SBAS (not observed, assumed) |
| 6 | IRNSS (not observed, assumed) |

Verified by cross-referencing NAV2-SIG `gnssid` + `sigid` with NMEA GSV talker IDs and CFG-NAVBAND bit positions. Confirmed with Galileo-only test (all entries showed GNSSID=3).

### 0b: Signal ID mapping (verified)

SigID = CFG-NAVBAND bit position (not sequential numbering). Confirmed by observing: GPS SigID=0 (bit 0, L1C/A), GLO SigID=5 (bit 5, L1), GAL SigID=7 (bit 7, E1), BDS SigID=10/11/14 (bits 10/11/14, B1I GEO/MEO/B1C), QZSS SigID=19 (bit 19, L1C/A).

### 0c: SVID offsets (verified)

QZSS SVIDs are PRN-192 (RINEX convention), matching `gpsprot.SVID.Num`. GPS, BDS, GLONASS, Galileo use raw PRNs (no offset). SBAS and IRNSS could not be verified (no satellites in view).

## Key differences from V5

| Aspect | V5 NAV (0x01) | V6 NAV2 (0x11) |
|--------|---------------|-----------------|
| Epoch key | `RunTime` (U4, ms since boot) | `tow` (I4, GPS TOW in ms) |
| Accuracy units | Variances (m^2, (m/s)^2, deg^2) -- need `math.Sqrt` | Standard deviations (m, m/s, deg) -- use directly |
| Time accuracy | TAcc = variance scaled by 1/c^2 -- complex conversion | TAcc = nanoseconds (direct) |
| GNSS IDs | Single scheme (GPS=0, BDS=1, GLN=2) | Two schemes: scalar (same as V5, extended) and bitmask (see below) |
| Satellite info | Per-constellation messages (GPSINFO, BDSINFO, GLNINFO) | Single multi-constellation message (NAV2-SIG) |
| Constellations | GPS, BDS, GLONASS | Adds Galileo, QZSS, SBAS, IRNSS |
| Fix quality | 0-8 scale | Extended 0-15 (adds DGPS=8, RTK float=9, RTK fixed=10, timing fixed=15) |
| Geodetic msg | NAV-PV (80 bytes) | NAV2-PVH (88 bytes, adds per-constellation counts + separate accuracy) |
| ECEF msg | NAV-SOL (72 bytes) | NAV2-SOL (72 bytes, restructured) |

## Part 1: casbin message parsing

NAV2 messages use GPS TOW in integer milliseconds for epoch keying instead of RunTime. A new `Nav2TOW` embedded type provides the `NavMsg` interface.

### 1a: Nav2TOW epoch key type

Add to `casbin/nav.go`:

```go
// Nav2TOW is embedded in NAV2 messages to provide epoch tracking via GPS TOW.
type Nav2TOW struct {
	TOW int32 // GPS time of week in ms
}

func (m *Nav2TOW) NavEpoch() uint32 { return uint32(m.TOW) }
```

This implements the existing `NavMsg` interface. The `PacketProcessor.handleNavEpoch` will track NAV2 epochs via TOW changes, just as it tracks V5 epochs via RunTime changes.

### 1b: Nav2FixFlags type

The V6 fix flags extend V5 `posValid` with values 8-15. Add a new type to `casbin/nav.go`:

```go
type Nav2FixFlags uint8

const (
	Nav2FixInvalid       Nav2FixFlags = 0
	Nav2FixExternal      Nav2FixFlags = 1
	Nav2FixRoughEstimate Nav2FixFlags = 2
	Nav2FixHold          Nav2FixFlags = 3
	Nav2FixDeadReckoning Nav2FixFlags = 4
	Nav2FixQuickMode     Nav2FixFlags = 5
	Nav2Fix2D            Nav2FixFlags = 6
	Nav2Fix3D            Nav2FixFlags = 7
	Nav2FixDGPS          Nav2FixFlags = 8
	Nav2FixRTKFloat      Nav2FixFlags = 9
	Nav2FixRTKFixed      Nav2FixFlags = 10
	Nav2FixTimingFixed   Nav2FixFlags = 15
)
```

A `Nav2VelFlags` type uses the same PVT Valid Flag scale (section 3.7.1) for velocity:

```go
type Nav2VelFlags uint8

const (
	Nav2VelInvalid       Nav2VelFlags = 0
	Nav2VelExternal      Nav2VelFlags = 1
	Nav2VelRoughEstimate Nav2VelFlags = 2
	Nav2VelHold          Nav2VelFlags = 3
	Nav2VelDeadReckoning Nav2VelFlags = 4
	Nav2VelQuickMode     Nav2VelFlags = 5
	Nav2Vel2D            Nav2VelFlags = 6
	Nav2Vel3D            Nav2VelFlags = 7
	Nav2VelDGPS          Nav2VelFlags = 8
	Nav2VelRTKFloat      Nav2VelFlags = 9
	Nav2VelRTKFixed      Nav2VelFlags = 10
)
```

The velocity threshold for valid output is `Nav2Vel2D` (same as V5's `NavVel2D`).

### 1c: Nav2GnssMask type

The `fixGnssMask` bitmask in NAV2-SOL/PVH (section 3.9.2/3.9.3) uses this bit order:

```go
type Nav2GnssMask uint8

const (
	Nav2GnssGPS  Nav2GnssMask = 1 << iota
	Nav2GnssBDS
	Nav2GnssGLN
	Nav2GnssGAL
	Nav2GnssQZSS
	Nav2GnssSBAS
	Nav2GnssIRNSS
)
```

### 1d: SigID type

The NAV2-SIG `SigID` field identifies the signal band. Unlike UBX where SigID is relative to the GNSS ID, CASIC SigID values are globally unique. Add to `casbin/nav.go`:

```go
type SigID uint8

const (
	SigGPSL1CA   SigID = 0
	SigGPSL5     SigID = 2
	SigSBASL1    SigID = 3
	SigSBASL5    SigID = 4
	SigGLOL1     SigID = 5
	SigGALE1     SigID = 7
	SigGALE5a    SigID = 8
	SigBDSB1IGEO SigID = 10
	SigBDSB1IMEO SigID = 11
	SigBDSB1C    SigID = 14
	SigBDSB2a    SigID = 15
	SigQZSSL1CA  SigID = 19
	SigQZSSL5    SigID = 21
	SigNAVICL5   SigID = 23
)
```

### 1e: Nav2Sol struct (0x11 0x02, 72 bytes)

Add to `casbin/nav.go`. Move `Nav2SolID` from `other.go` to `nav.go`. Register in `init()`.

```go
// Nav2Sol is NAV2-SOL (0x11 0x02) - ECEF position and velocity (72 bytes)
type Nav2Sol struct {
	Nav2TOW
	Wn         uint16
	_          uint16       // reserved
	FixFlags   Nav2FixFlags
	VelFlags   Nav2VelFlags
	_          uint8        // reserved
	GnssMask   Nav2GnssMask
	NumFixTot  uint8
	NumFixGPS  uint8
	NumFixBDS  uint8
	NumFixGLN  uint8
	NumFixGAL  uint8
	NumFixQZSS uint8
	NumFixSBAS uint8
	NumFixIRN  uint8
	_          uint32       // reserved
	X          float64      // m, ECEF X
	Y          float64      // m, ECEF Y
	Z          float64      // m, ECEF Z
	PAcc       float32      // m, 3D position accuracy (std dev, NOT variance)
	VX         float32      // m/s, ECEF X velocity
	VY         float32      // m/s, ECEF Y velocity
	VZ         float32      // m/s, ECEF Z velocity
	SAcc       float32      // m/s, 3D speed accuracy (std dev, NOT variance)
	PDOP       float32
}

func (m *Nav2Sol) ID() MsgID { return Nav2SolID }
```

### 1f: Nav2Pvh struct (0x11 0x03, 88 bytes)

Add to `casbin/nav.go`. Move `Nav2PvhID` from `other.go` to `nav.go`. Register in `init()`.

```go
// Nav2Pvh is NAV2-PVH (0x11 0x03) - geodetic position and velocity (88 bytes)
type Nav2Pvh struct {
	Nav2TOW
	Wn         uint16
	_          uint16       // reserved
	FixFlags   Nav2FixFlags
	VelFlags   Nav2VelFlags
	_          uint8        // reserved
	GnssMask   Nav2GnssMask
	NumFixTot  uint8
	NumFixGPS  uint8
	NumFixBDS  uint8
	NumFixGLN  uint8
	NumFixGAL  uint8
	NumFixQZSS uint8
	NumFixSBAS uint8
	NumFixIRN  uint8
	_          uint32       // reserved
	Lon        float64      // deg
	Lat        float64      // deg
	Height     float32      // m, ellipsoidal
	SepGeoid   float32      // m, geoid separation
	VelE       float32      // m/s, East velocity
	VelN       float32      // m/s, North velocity
	VelU       float32      // m/s, Up velocity (negate for NED down)
	Speed3D    float32      // m/s
	Speed2D    float32      // m/s, ground speed
	Heading    float32      // deg
	HAcc       float32      // m, horizontal position accuracy (std dev)
	VAcc       float32      // m, vertical position accuracy (std dev)
	SAcc       float32      // m/s, 3D speed accuracy (std dev)
	CAcc       float32      // deg, heading accuracy (std dev)
}

func (m *Nav2Pvh) ID() MsgID { return Nav2PvhID }
```

### 1g: Nav2Dop struct (0x11 0x01, 24 bytes)

Add to `casbin/nav.go`. Move `Nav2DopID` from `other.go` to `nav.go`. Register in `init()`.

NAV2-DOP has no `tow` field. It does not implement `NavMsg`.

```go
// Nav2Dop is NAV2-DOP (0x11 0x01) - dilution of precision (24 bytes)
type Nav2Dop struct {
	PDOP float32
	HDOP float32
	VDOP float32
	NDOP float32
	EDOP float32
	TDOP float32
}

func (m *Nav2Dop) ID() MsgID { return Nav2DopID }
```

### 1h: Nav2TimeUTC struct (0x11 0x05, 20 bytes)

Add to `casbin/nav.go`. Move `Nav2TimeUTCID` from `other.go` to `nav.go`. Register in `init()`.

NAV2-TIMEUTC does not have a standard epoch key field (no `tow` or `RunTime`). It does not implement `NavMsg`.

```go
// Nav2TimeUTC is NAV2-TIMEUTC (0x11 0x05) - UTC time information (20 bytes)
type Nav2TimeUTC struct {
	TAcc    float32       // ns, time accuracy estimate
	Subms   int32         // ms, fractional ms (scale 2^-30)
	Subcs   int8          // ms, centisecond error (-5 to 5 ms)
	Cs      uint8         // centiseconds (0-99)
	Year    uint16
	Month   uint8
	Day     uint8
	Hour    uint8
	Min     uint8
	Sec     uint8
	TFlags  Nav2TimeFlags
	TimeSrc Nav2TimeSrc
	LeapSec int8
}

type Nav2TimeFlags uint8

const (
	Nav2TimeTOWValid  Nav2TimeFlags = 1 << iota
	Nav2TimeWNValid
	Nav2TimeLeapValid
	Nav2TimeReliable
)

type Nav2TimeSrc uint8

const (
	Nav2TimeSrcGPS Nav2TimeSrc = iota
	Nav2TimeSrcBDS
	Nav2TimeSrcGLN
	Nav2TimeSrcGAL
	Nav2TimeSrcIRN
)

func (m *Nav2TimeUTC) ID() MsgID { return Nav2TimeUTCID }
```

### 1i: Nav2Sig struct (0x11 0x06, variable)

Add to `casbin/nav.go`. Move `Nav2SigID` from `other.go` to `nav.go`. Register in `init()`.

NAV2-SIG replaces NAV2-SAT (which the ATGM332D-6N74 ACKs but never outputs). NAV2-SIG is a superset: per-signal rather than per-satellite, with additional correction source and solution flag fields. It is used for both satellite info extraction and correction disambiguation.

```go
// Nav2SigFixed is the fixed part of NAV2-SIG (8 bytes)
type Nav2SigFixed struct {
	TOW       uint32 // GPS TOW in ms
	_         uint8  // reserved
	NumTrkTot uint8
	NumFixTot uint8
	_         uint8  // reserved
}

// Nav2SigInfo is a per-signal entry in NAV2-SIG (16 bytes each)
type Nav2SigInfo struct {
	GNSSID    uint8  // GNSS ID (same numbering as V5: GPS=0, BDS=1, GLN=2, GAL=3, QZSS=4, SBAS=5, IRNSS=6)
	SVID      uint8  // satellite ID (raw PRN, except QZSS=PRN-192)
	SigID     SigID  // signal band ID (see SigID constants)
	FreqID    uint8  // GLONASS frequency ID [-7,+6] mapped to [1,14]; undefined for other constellations
	PRRes     int16  // dm, pseudorange residual
	CNO       uint8  // dBHz
	TrkInd    uint8  // signal quality (section 3.7.2, same as Nav2SVInfo.Quality)
	CorFlags  uint8  // correction flag (see below)
	SolFlags  uint8  // solution flag (see below)
	Chn       uint8  // tracking channel number
	Elev      uint8  // deg
	Azim      uint16 // deg
	IonoDelay int16  // dm, ionosphere delay correction
}

// Nav2Sig is NAV2-SIG (0x11 0x06) - per-signal tracking information
type Nav2Sig struct {
	Nav2SigFixed
	Sigs []Nav2SigInfo
}

func (m *Nav2Sig) ID() MsgID { return Nav2SigID }

func (m *Nav2Sig) InitVaryingPart(payloadLen int) (err error) {
	n, err := sliceLen(m, payloadLen, 8, 16)
	if err == nil {
		m.Sigs = make([]Nav2SigInfo, n)
	}
	return
}

func (m *Nav2Sig) FixedPart() any   { return &m.Nav2SigFixed }
func (m *Nav2Sig) VaryingPart() any { return &m.Sigs }

var _ VaryingMsg = (*Nav2Sig)(nil)
```

**Correction flag (`CorFlags`) bit definitions:**

| Bits | Field | Values |
|------|-------|--------|
| B[2:0] | Correction source | 0=NULL, 1=SBAS, 2=BDS (B2b PPP), 3=RTCM2, 4=OSR, 5=SSR |
| B[6:4] | Ionosphere model | 0=NULL, 1=GPS, 2=SBAS, 3=BD2, 4=GAL, 5=BD3, 7=Dual freq |

**Solution flag (`SolFlags`) bit definitions:**

| Bits | Description |
|------|-------------|
| B0 | Pseudorange used in solution |
| B1 | Carrier phase used |
| B2 | Doppler used |
| B3 | Pseudorange smoothing applied |
| B[7:4] | Satellite solution status: 0=DISABLE, 1=NULL, 2-3=INVISIBLE, 4=ALM, 5=LTE, 6=AGNSS, 7=EPH |

The `CorFlags` correction source field (bits 2:0) is used for correction disambiguation in Part 2.

## Part 2: internal/casic extraction and dispatch

### Epoch keying for NAV2 messages

NAV2-SOL and NAV2-PVH embed `Nav2TOW` which implements `NavMsg`. Their `NavEpoch()` returns `uint32(tow)`. The existing `handleNavEpoch` in `casproc.go` already calls `NavEpoch()` on any `casbin.NavMsg`, so NAV2-SOL and NAV2-PVH will trigger epoch changes via TOW changes.

NAV2-DOP has no epoch key -- it is associated with the current epoch on arrival (same approach as NMEA GSA).

NAV2-TIMEUTC has no standard epoch key. It is dispatched independently without epoch association (same as V5 NavTimeUTC which dispatches TimeMsg directly).

NAV2-SIG has a `TOW` field (U4, ms) in its fixed header. It implements `NavMsg` via a method on its fixed part:

```go
func (m *Nav2SigFixed) NavEpoch() uint32 { return m.TOW }
```

Since `Nav2Sig` embeds `Nav2SigFixed`, the promoted `NavEpoch()` method satisfies the `NavMsg` interface on the outer struct. Note: `Nav2SigFixed.TOW` is U4 (unsigned), while `Nav2TOW.TOW` is I4 (signed) -- both return `uint32` from `NavEpoch()` so epoch tracking works identically.

### 2a: NAV2-TIMEUTC time extraction

New extraction function in `castime.go`:

```go
func timeNav2TimeUTC(m *casbin.Nav2TimeUTC) *gpsprot.TimeMsg
```

Returns a `TimeMsg` with `NativeMsgID = "NAV2-TIMEUTC"`.

**Validity check**: Only construct a valid UTC time when `TFlags` has both `Nav2TimeTOWValid` and `Nav2TimeReliable`. Otherwise return a TimeMsg with nil UTCTime (same pattern as V5 `timeNavTimeUTC`).

**Fractional time construction**: The V6 TIMEUTC message splits fractional seconds across three fields:
- `Cs` (U1): integer centiseconds 0-99, giving 10ms resolution
- `Subcs` (I1): centisecond error -5 to +5 ms
- `Subms` (I4, scale 2^-30): sub-millisecond fraction -0.5 to +0.5 ms

Full fractional milliseconds: `float64(m.Cs)*10 + float64(m.Subcs) + float64(m.Subms) * math.Exp2(-30)`

Convert to nanoseconds for `ptime.UTC`: `nanos := int32(math.Round(fracMs * 1e6))`

Then: `u := ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, nanos)`

This differs from V5 `timeNavTimeUTC` which uses `MsErr` (a float32 residual in ms) and the V5 peculiar `TAcc` encoding (variance scaled by 1/c^2).

**Accuracy**: V6 `TAcc` is in nanoseconds (direct standard deviation, not variance). Convert: `t.Accuracy = time.Duration(m.TAcc)`

**GNSS mapping**: `Nav2TimeSrc` uses the same numbering as the unified `casicGNSSIDToGNSS` function (see section 2e): `casicGNSSIDToGNSS(uint8(m.TimeSrc))`.

#### Dispatch integration

Add `case *casbin.Nav2TimeUTC:` to `dispatch()`. Same pattern as existing `NavTimeUTC` case:

```go
case *casbin.Nav2TimeUTC:
	tm := timeNav2TimeUTC(mt)
	if tm == nil {
		return false
	}
	if p.mh != nil {
		tm.Tag = Tag
		p.mh.Time(tm, tRead)
	}
	return true
```

### 2b: NAV2-SOL position/velocity extraction

New file: `gps/internal/casic/caspv2.go` (and `caspv2_test.go`), or extend `caspv.go`.

Two extraction functions:

- `posECEFNav2Sol(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sol) *gpsprot.PosECEFMsg` -- returns nil when `FixFlags < Nav2Fix2D`. Position: `X/Y/Z` (float64, m) -> `Point3D` (Length in micrometres). Accuracy: `PAcc` (m, direct -- no sqrt) -> `ne.Acc.Pos`. Sets `NativeMsgID = "NAV2-SOL"`.
- `velECEFNav2Sol(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sol) *gpsprot.VelECEFMsg` -- returns nil when `VelFlags < Nav2Vel2D`. Velocity: `VX/VY/VZ` (float32, m/s) -> `[3]Speed`. Accuracy: `SAcc` (m/s, direct) -> `ne.Acc.Speed`. Sets `NativeMsgID = "NAV2-SOL"`.

Quality fields on `ne` (using `PriVendorLow`):
- `FixLevel`, `FixDim`, `AuxSrc`, `Correction`: from `FixFlags` (see mapping table below)
- `NumSVUsed`: from `NumFixTot`
- `DOP.Pos`: from `PDOP`

Unit conversions (same helper functions as V5, but no sqrt for accuracy):
- Position: `gpsprot.Meters(m.X)` etc. (float64 m -> Length)
- Velocity: `gpsprot.MetersPerSecondFromFloat(float64(m.VX))` etc. (float32 m/s -> Speed)
- Accuracy position: `gpsprot.Meters(float64(m.PAcc))` (direct, NOT `math.Sqrt`)
- Accuracy speed: `gpsprot.MetersPerSecondFromFloat(float64(m.SAcc))` (direct)

Unlike V5's `NavSol`, NAV2-SOL does not provide full time (no `Week`+`TOW` as float64 seconds). The `tow` is I4 milliseconds, useful only for epoch keying. No `TimeMsg` is extracted from NAV2-SOL (use NAV2-TIMEUTC instead).

#### Dispatch integration

Add `case *casbin.Nav2Sol:` to `dispatch()`:

```go
case *casbin.Nav2Sol:
	posE := posECEFNav2Sol(p.curNavEpochMsg, mt)
	velE := velECEFNav2Sol(p.curNavEpochMsg, mt)
	if p.mh != nil {
		if posE != nil {
			posE.Tag = Tag
			posE.Priority = gpsprot.PriVendorLow
			p.mh.PosECEF(posE, tRead)
		}
		if velE != nil {
			velE.Tag = Tag
			velE.Priority = gpsprot.PriVendorLow
			p.mh.VelECEF(velE, tRead)
		}
	}
	return true
```

### 2c: NAV2-PVH position/velocity extraction

Added to `caspv2.go` and `caspv2_test.go`.

Two extraction functions:

- `posGeoNav2Pvh(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Pvh) *gpsprot.PosGeoMsg` -- returns nil when `FixFlags < Nav2Fix2D`. Position: `Lat/Lon` (float64, deg) -> `[2]Angle` (nanodegrees). Height: `Height` (m) -> `Length`. HeightMSL: `Height - SepGeoid`. Accuracy: `HAcc` (m, direct) -> `ne.Acc.Hor`; `VAcc` (m, direct) -> `ne.Acc.Vert`. Sets `NativeMsgID = "NAV2-PVH"`.
- `velGeoNav2Pvh(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Pvh) *gpsprot.VelGeoMsg` -- returns nil when `VelFlags < Nav2Vel2D`. Velocity: `VelN`, `VelE`, `-VelU` (note negation for NED down) -> `VelNED`. `Speed2D` -> `GroundSpeed`. `Speed3D` -> `Speed3D`. `Heading` -> `Course`. Accuracy: `SAcc` (m/s, direct) -> `ne.Acc.Speed`; `CAcc` (deg, direct) -> `ne.Acc.Course`. Sets `NativeMsgID = "NAV2-PVH"`.

Quality fields on `ne` (using a separate `qualityFromNav2FixFlags` function):
- Same `FixFlags` mapping as NAV2-SOL (see mapping table below)
- `NumSVUsed`: from `NumFixTot`
- No PDOP (NAV2-PVH does not have a PDOP field -- PDOP comes from NAV2-SOL or NAV2-DOP)

The `qualityFromNav2FixFlags` function is shared between NAV2-SOL and NAV2-PVH. Unlike V5's `qualityFromPosValid` which takes a `pdop` parameter, the V6 version does not set `DOP.Pos` (that comes from NAV2-DOP or NAV2-SOL's PDOP field separately).

Unit conversions (same helper functions as V5, but no sqrt for accuracy):
- Lat/Lon: `gpsprot.DegreesFromFloat(m.Lat)` etc. (float64 deg -> Angle)
- Height: `gpsprot.Meters(float64(m.Height))` (float32 m -> Length)
- Velocity: `gpsprot.MetersPerSecondFromFloat(float64(m.VelN))` etc.
- Course: `gpsprot.DegreesFromFloat(float64(m.Heading))`
- Accuracy: `gpsprot.Meters(float64(m.HAcc))` (direct, NOT `math.Sqrt`)

#### Dispatch integration

Add `case *casbin.Nav2Pvh:` to `dispatch()`. Messages dispatched at `PriVendorHigh` because NAV2-PVH has richer accuracy (separate HAcc/VAcc/CAcc) than NAV2-SOL:

```go
case *casbin.Nav2Pvh:
	posG := posGeoNav2Pvh(p.curNavEpochMsg, mt)
	velG := velGeoNav2Pvh(p.curNavEpochMsg, mt)
	if posG == nil && velG == nil {
		return false
	}
	if p.mh != nil {
		if posG != nil {
			posG.Tag = Tag
			posG.Priority = gpsprot.PriVendorHigh
			p.mh.PosGeo(posG, tRead)
		}
		if velG != nil {
			velG.Tag = Tag
			velG.Priority = gpsprot.PriVendorHigh
			p.mh.VelGeo(velG, tRead)
		}
	}
	return true
```

### 2d: NAV2-DOP extraction

Function: `dopNav2Dop(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Dop)` -- same implementation as V5's `dopNavDop`:

```go
func dopNav2Dop(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Dop) {
	ne.DOP.Pos = opt.Make(float64(m.PDOP))
	ne.DOP.Hor = opt.Make(float64(m.HDOP))
	ne.DOP.Vert = opt.Make(float64(m.VDOP))
	ne.DOP.Time = opt.Make(float64(m.TDOP))
}
```

Values are direct float32 (no scaling). NDOP/EDOP are not represented in `gpsprot.DOP`.

NAV2-SOL also has a `PDOP` field. When both NAV2-DOP and NAV2-SOL are enabled, `posECEFNav2Sol` sets `ne.DOP.Pos` from its PDOP. `dopNav2Dop` may overwrite this with HDOP/VDOP/TDOP in addition. Since both write to the same `ne`, the last writer wins for PDOP -- this is fine as they should have the same value.

#### Dispatch integration

Add `case *casbin.Nav2Dop:` to `dispatch()`. NAV2-DOP does not implement `NavMsg` (no epoch key), so it is not tracked by `handleNavEpoch`. It writes to `p.curNavEpochMsg` directly:

```go
case *casbin.Nav2Dop:
	if p.curNavEpochMsg != nil {
		dopNav2Dop(p.curNavEpochMsg, mt)
	}
	return true
```

### 2e: NAV2-SIG satellite and correction extraction

NAV2-SIG replaces NAV2-SAT (which the receiver ACKs but never outputs). NAV2-SIG provides per-signal data with all constellations in a single message, plus correction source and solution flags.

#### GNSS ID mapping (verified)

The `gnssid` field uses the same numbering as V5 `casbin.GNSSID` and `Nav2TimeSrc`:

| gnssid | System | gpsprot.GNSS |
|--------|--------|--------------|
| 0 | GPS | GPS |
| 1 | BDS | BDS |
| 2 | GLONASS | GLO |
| 3 | Galileo | GAL |
| 4 | QZSS | QZSS |
| 5 | SBAS | SBAS |
| 6 | IRNSS | NAVIC |

Since this is the same numbering as `Nav2TimeSrc` (extended with QZSS/SBAS/IRNSS), a single mapping function suffices. The existing V5 `gnssIDToGNSS` can be extended:

```go
func casicGNSSIDToGNSS(id uint8) gpsprot.GNSS {
	switch id {
	case 0:
		return gpsprot.GPS
	case 1:
		return gpsprot.BDS
	case 2:
		return gpsprot.GLO
	case 3:
		return gpsprot.GAL
	case 4:
		return gpsprot.QZSS
	case 5:
		return gpsprot.SBAS
	case 6:
		return gpsprot.NAVIC
	}
	return 0
}
```

This replaces both the V5 `gnssIDToGNSS` (which handles 0-2) and the planned `nav2TimeSrcToGNSS` / `nav2GNSSIDToGNSS` functions, since all three use the same numbering.

#### Signal ID mapping (verified)

The `SigID` field identifies the signal band. Unlike UBX where SigID is relative to the GNSS ID, CASIC SigID values are globally unique (see section 1d), so a flat map suffices. CASIC does not distinguish I/Q components, so for L5 signals use the data component (same convention as UBX).

Map `casbin.SigID` to `gpsprot.SignalID` in `internal/casic`:

```go
var casicSigIDMap = map[casbin.SigID]gpsprot.SignalID{
	casbin.SigGPSL1CA:   gpsprot.SigIDGPSL1CA,
	casbin.SigGPSL5:     gpsprot.SigIDGPSL5I,
	casbin.SigSBASL1:    gpsprot.SigIDGPSL1CA, // SBAS uses GPS L1 C/A
	casbin.SigSBASL5:    gpsprot.SigIDGPSL5I,
	casbin.SigGLOL1:     gpsprot.SigIDGLOL1,
	casbin.SigGALE1:     gpsprot.SigIDGALE1,
	casbin.SigGALE5a:    gpsprot.SigIDGALE5a,
	casbin.SigBDSB1IGEO: gpsprot.SigIDBDSB1I,
	casbin.SigBDSB1IMEO: gpsprot.SigIDBDSB1I,
	casbin.SigBDSB1C:    gpsprot.SigIDBDSB1C,
	casbin.SigBDSB2a:    gpsprot.SigIDBDSB2a,
	casbin.SigQZSSL1CA:  gpsprot.SigIDQZSSL1CA,
	casbin.SigQZSSL5:    gpsprot.SigIDQZSSL5I,
	casbin.SigNAVICL5:   gpsprot.SigIDNAVICL5,
}
```

#### SVID handling (verified)

GPS, BDS, GLONASS, Galileo, QZSS use raw satellite numbers that match `gpsprot.SVID.Num` directly. SBAS was not observed, but the receiver almost certainly sends raw PRNs (120-158), so subtract 100 to get `SVID.Num` (20-58).

#### Satellite info extraction

New function: `satsNav2Sig(m *casbin.Nav2Sig) *gpsprot.SatellitesMsg`

NAV2-SIG is per-signal, so multiple entries may exist for the same satellite (e.g. BDS B1I + B1C). Group entries by GNSSID+SVID to build per-satellite records with multiple `SignalInfo` entries:

Follow the same pattern as UBX `satellitesNavSig()` in `ubxsats.go`: group per-signal entries by SVID using a `sigIndex` map, building per-satellite records with multiple `SignalInfo` entries.

For each signal in `m.Sigs` with `CNO > 0`:
- Map `GNSSID` to `gpsprot.GNSS` using `casicGNSSIDToGNSS`
- Convert `SVID` to `gpsprot.SVID` (direct for GPS/BDS/GLO/GAL/QZSS; subtract 100 for SBAS, same as UBX `gnssSVID`)
- Map `(GNSSID, SigID)` to `gpsprot.SignalID` using `casicSigIDMap`
- Set `Used` from `SolFlags & 0x01 != 0` (pseudorange used in solution)
- LookAngles from `Elev`/`Azim` (from first signal seen for each SV)
- Each signal becomes a `SignalInfo` entry with `SignalID`, `CN0`, `Used`
- Set `sv.Used = true` if any signal is used (same loop as UBX)

Set `UsedValidity = SatelliteUsedSignal` (same as V5).

#### Correction disambiguation

NAV2-SIG provides per-signal `CorFlags` (bits 2:0 = correction source) that can enrich the `Correction` bitmask on `curNavEpochMsg`. This is analogous to UBX NAV-SIG's `CorrSource`.

| CorFlags bits 2:0 | Description | CorrKind |
|--------------------|-------------|----------|
| 0 | NULL (no corrections) | (none) |
| 1 | SBAS | CorrSBAS |
| 2 | BDS (B2b PPP) | CorrWideArea |
| 3 | RTCM2 | CorrRTCM |
| 4 | OSR (observation-space) | CorrBaseStation \| CorrRTCM |
| 5 | SSR (state-space) | CorrWideArea \| CorrRTCM |

As NAV2-SIG signals are processed, accumulate `CorrKind` on `curNavEpochMsg` from per-signal correction sources of signals marked as used (`SolFlags` bit 0 = PR used). This enriches the base correction set from `FixFlags`.

New function: `corrFromNav2Sig(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sig)`

```go
func corrFromNav2Sig(ne *gpsprot.NavEpochMsg, m *casbin.Nav2Sig) {
	if ne == nil {
		return
	}
	for i := range m.Sigs {
		sig := &m.Sigs[i]
		if sig.SolFlags&0x01 == 0 { // not used in solution
			continue
		}
		switch sig.CorFlags & 0x07 { // bits 2:0
		case 1: // SBAS
			ne.Correction |= gpsprot.CorrSBAS | gpsprot.CorrUsed
		case 2: // BDS B2b PPP
			ne.Correction |= gpsprot.CorrWideArea | gpsprot.CorrUsed
		case 3: // RTCM2
			ne.Correction |= gpsprot.CorrRTCM | gpsprot.CorrUsed
		case 4: // OSR
			ne.Correction |= gpsprot.CorrBaseStation | gpsprot.CorrRTCM | gpsprot.CorrUsed
		case 5: // SSR
			ne.Correction |= gpsprot.CorrWideArea | gpsprot.CorrRTCM | gpsprot.CorrUsed
		}
	}
}
```

Also accumulate `SignalsUsed` from per-signal `SigID` and `SolFlags` (bit 0 = PR used), similar to UBX NAV-SIG. For each used signal, map the GNSSID+SigID pair to `gpsprot.Signal` and set the bit in `ne.SignalsUsed`.

#### Dispatch integration

Add `case *casbin.Nav2Sig:` to `dispatch()`:

```go
case *casbin.Nav2Sig:
	msg := satsNav2Sig(mt)
	if msg != nil && p.mh != nil {
		p.mh.Satellites(msg, tRead)
	}
	corrFromNav2Sig(p.curNavEpochMsg, mt)
	return true
```

### fixFlags -> NavEpochMsg mapping (NAV2-SOL and NAV2-PVH)

| fixFlags | Description | AuxSrc | FixLevel | FixDim | Correction |
|----------|-------------|--------|----------|--------|------------|
| 0 | Invalid | 0 | FixLevelNone | 0 | 0 |
| 1 | External input | 0 | FixLevelNotMeasured | 0 | 0 |
| 2 | Rough estimate | 0 | FixLevelNone | 0 | 0 |
| 3 | Maintaining / hold | 0 | FixLevelNone | 0 | 0 |
| 4 | Dead reckoning | AuxSrcDR | FixLevelNone | 0 | 0 |
| 5 | Quick mode | 0 | FixLevelCode | FixDim3D | 0 |
| 6 | 2D positioning | 0 | FixLevelCode | FixDim2D | 0 |
| 7 | 3D positioning | 0 | FixLevelCode | FixDim3D | 0 |
| 8 | DGPS | 0 | FixLevelCodeCorrected | FixDim3D | CorrUsed |
| 9 | RTK float | 0 | FixLevelCarrierFloat | FixDim3D | CorrBaseStation |
| 10 | RTK fixed | 0 | FixLevelCarrierFixed | FixDim3D | CorrBaseStation |
| 15 | Timing fixed position | 0 | FixLevelNotMeasured | FixDimTimeOnly | 0 |

Notes:
- Values 0-7 share the same semantics as V5 `posValid` (same `qualityFromPosValid` logic applies)
- Values 8-10 and 15 are new in V6 and need a new `qualityFromNav2FixFlags` function
- DGPS (8) sets `CorrUsed` only; the `fixFlags` alone does not distinguish base-station vs wide-area. NAV2-SIG `CorFlags` can disambiguate (see 2f above)
- RTK float (9) and fixed (10) assert `CorrBaseStation | CorrUsed`
- Fixed timing position (15) is a user-set or averaged position where only the clock is solved

Implementation approach: `qualityFromNav2FixFlags` handles all 0-15 values. For values 0-7, the logic matches V5's `qualityFromPosValid`. For 8-10 and 15, add new cases. Unlike V5, the V6 function does not take a `pdop` parameter (PDOP comes from NAV2-DOP or NAV2-SOL separately):

```go
func qualityFromNav2FixFlags(ne *gpsprot.NavEpochMsg, ff casbin.Nav2FixFlags, numSV uint8) {
	switch ff {
	default: // 0=Invalid, 2=RoughEstimate, 3=Hold
		ne.FixLevel = gpsprot.FixLevelNone
	case casbin.Nav2FixExternal:
		ne.FixLevel = gpsprot.FixLevelNotMeasured
	case casbin.Nav2FixDeadReckoning:
		ne.FixLevel = gpsprot.FixLevelNone
		ne.AuxSrc = gpsprot.AuxSrcDR
	case casbin.Nav2FixQuickMode, casbin.Nav2Fix3D:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.FixDim = gpsprot.FixDim3D
	case casbin.Nav2Fix2D:
		ne.FixLevel = gpsprot.FixLevelCode
		ne.FixDim = gpsprot.FixDim2D
	case casbin.Nav2FixDGPS:
		ne.FixLevel = gpsprot.FixLevelCodeCorrected
		ne.FixDim = gpsprot.FixDim3D
		ne.Correction |= gpsprot.CorrUsed
	case casbin.Nav2FixRTKFloat:
		ne.FixLevel = gpsprot.FixLevelCarrierFloat
		ne.FixDim = gpsprot.FixDim3D
		ne.Correction |= gpsprot.CorrBaseStation | gpsprot.CorrUsed
	case casbin.Nav2FixRTKFixed:
		ne.FixLevel = gpsprot.FixLevelCarrierFixed
		ne.FixDim = gpsprot.FixDim3D
		ne.Correction |= gpsprot.CorrBaseStation | gpsprot.CorrUsed
	case casbin.Nav2FixTimingFixed:
		ne.FixLevel = gpsprot.FixLevelNotMeasured
		ne.FixDim = gpsprot.FixDimTimeOnly
	}
	ne.NumSVUsed.Set(uint16(numSV))
}
```

### GNSS numbering schemes in V6

V6 uses **two** GNSS numbering schemes (vs V5 which has one):

**1. Scalar GNSS ID** (used in `Nav2TimeSrc`, `gnssid` field, and V5 `GNSSID`):

| Value | System | gpsprot.GNSS |
|-------|--------|--------------|
| 0 | GPS | GPS |
| 1 | BDS | BDS |
| 2 | GLONASS | GLO |
| 3 | Galileo | GAL |
| 4 | QZSS | QZSS |
| 5 | SBAS | SBAS |
| 6 | IRNSS | NAVIC |

Empirical verification confirmed this is the same numbering everywhere: V5 `GNSSID` (0-2), V6 `Nav2TimeSrc` (0-4), and V6 NAV2-SIG `gnssid` (0-6). A single mapping function `casicGNSSIDToGNSS` handles all cases, replacing the V5-only `gnssIDToGNSS`.

**2. `Nav2GnssMask`** (bitmask) -- used in NAV2-SOL/PVH `fixGnssMask`:

| Bit | System |
|-----|--------|
| 0 | GPS |
| 1 | BDS |
| 2 | GLONASS |
| 3 | Galileo |
| 4 | QZSS |
| 5 | SBAS |
| 6 | IRNSS |

Same ordering as the scalar GNSS ID, just as a bitmask.

### FlushNavEpoch considerations

The existing `FlushNavEpoch` returns `PriVendorLow` for the `NavEpochMsg`. With V6 support, this should remain `PriVendorLow` -- the priority on the `NavEpochMsg` itself does not change. The `PriVendorHigh` priority is on individual `PosGeoMsg`/`VelGeoMsg` from NAV2-PVH (which override NAV2-SOL's `PriVendorLow` messages in the downstream merger).

The `FlushNavEpoch` method needs a V6-aware satellite flush. Since V6 doesn't use `satAccum`, the `p.satAccum.epochChange()` call is harmless (it only flushes if V5 satellite data was accumulated). V6 satellite data from NAV2-SIG is emitted immediately in dispatch.

### Testing pattern

Tests in `caspv2_test.go` construct casbin structs with known values, call extraction functions, and verify:
- gpsprot message fields (coordinates, velocities, course)
- Accuracy accumulation on `NavEpochMsg` (direct values, no sqrt)
- Quality field mapping from `fixFlags` (extended 0-15 scale)
- Nil return for invalid fix flags
- VelU negation for NED convention
- Correction disambiguation from NAV2-SIG `CorFlags`

### NAV2 messages not implemented

The following NAV2 messages exist but are not implemented in this plan:

- **NAV2-SAT (0x11 0x04, variable)**: per-satellite info. The ATGM332D-6N74 ACKs the CFG-MSG enable but never outputs this message. NAV2-SIG is a superset and is used instead.
- **NAV2-STATUS (0x11 0x00, 48 bytes)**: ephemeris and UTC/ionosphere validity per constellation. Not needed for position/velocity pipeline. Could be useful for diagnostics (number of valid ephemerides, PRN bitmasks per system).
- **NAV2-CLK (0x11 0x07, 20 bytes)**: receiver time bias and frequency bias. Not needed unless clock-domain processing is added.
- **NAV2-RVT (0x11 0x08, 52 bytes)**: raw receiver time information. Not needed for position/velocity.
- **NAV2-RTC (0x11 0x09, 32 bytes)**: RTC time information. Not needed for position/velocity.

The message IDs for these are already defined in `casbin/other.go` and registered in `idNameMap`, so they will be parsed as `UnknownMsg` and logged but not dispatched.

### Fields not available from V6 CASIC binary

- **SignalsUsed**: derive from NAV2-SIG per-signal `SigID` and `SolFlags` (bit 0 = PR used), analogous to UBX NAV-SIG. SigID = CFG-NAVBAND bit position (verified).
- **NumSVTracked**: NAV2-SIG `NumTrkTot` counts signal tracks, not satellites, so overcounts with multi-frequency. Not populated.
- **DiffAge, RTCMRefBaseID**: not available from V6 CASIC binary. Only available via NMEA GGA when enabled alongside binary.

### Message enablement

Message file tags in `configs/gpsmsg/atgm332d-v6.toml` (all already defined):
- `casbin-nav2-sol`: enable NAV2-SOL
- `casbin-nav2-timeutc`: enable NAV2-TIMEUTC
- `casbin-nav2-pvh`: enable NAV2-PVH
- `casbin-nav2-dop`: enable NAV2-DOP
- `casbin-nav2-sat`: enable NAV2-SAT (ACK'd but receiver does not output; kept for future firmware)
- `casbin-nav2-sig`: enable NAV2-SIG

### Message registration

Move the following IDs from `other.go` to `nav.go` and register with `regMsg`:

```go
// in nav.go, after existing init()
func init() {
	// ... existing V5 registrations ...
	regMsg[Nav2Dop]("DOP")
	regMsg[Nav2Sol]("SOL")
	regMsg[Nav2Pvh]("PVH")
	regMsg[Nav2TimeUTC]("TIMEUTC")
	regMsg[Nav2Sig]("SIG")
}
```

The ID constants (`Nav2DopID`, `Nav2SolID`, `Nav2PvhID`, `Nav2TimeUTCID`, `Nav2SigID`) move from `other.go` to `nav.go`. Their `idNameMap` entries in `other.go`'s `init()` are removed (they're now registered by `regMsg`). `Nav2SatID` stays in `other.go` (not implemented).

### V5/V6 coexistence

A single `PacketProcessor` handles both V5 NAV and V6 NAV2 messages. There is no conflict because:
- V5 uses class 0x01, V6 uses class 0x11 -- different MsgIDs, different type assertions in `dispatch()`
- Both use the same `curNavEpochMsg` and `NavEpochManager` -- epoch tracking via `NavMsg.NavEpoch()` works for both `NavRunTime` (V5) and `Nav2TOW` (V6)
- A real receiver sends only V5 or V6, never both, so there's no interleaving concern

### Implementation order

1. 1a (Nav2TOW) + 1b (Nav2FixFlags) + 1c (Nav2GnssMask) + 1d (SigID) -- types, `make test`
2. 1e (Nav2Sol) + 1f (Nav2Pvh) + 1g (Nav2Dop) -- structs, `make test`
3. 1h (Nav2TimeUTC) -- struct, `make test`
4. 1i (Nav2Sig) -- variable-length struct, `make test`
5. 2a (NAV2-TIMEUTC time extraction + dispatch) -- `make test`
6. 2b (NAV2-SOL extraction + dispatch) -- `make test`
7. 2c (NAV2-PVH extraction + dispatch) -- `make test`
8. 2d (NAV2-DOP extraction) -- `make test`
9. 2e (NAV2-SIG satellite + correction extraction + dispatch) -- `make test`
