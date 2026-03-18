# Techtotop SDBP: complete DAT message support

Finish the SDBP protocol implementation for the Techtotop/Taidou T303-5D timing
receiver. Currently only time messages (GPST, BDST, GALT, TPPS) are
handled. Each phase delivers end-to-end testable functionality.

Reference implementation: CASIC in `gps/internal/casic/` and
`gps/lib/casbin/`.

Hardware: Techtotop/Taidou T303-5D on `/dev/ttyUSB0` at 115200 baud.
Use `satpulsetool gps -d /dev/ttyUSB0 -s 115200 -m configs/gpsmsg/techtotop.toml`
for all hardware commands that need `-t`.

## Common infrastructure: NavMsg and epoch tracking

All DAT messages except TPPS and TSURV share a leading
`LocalTimestamp uint32` field identifying the navigation epoch (ms,
matching integer TOW). Use this for epoch tracking, following the CASIC
pattern:

**sdbpbin (`gps/lib/sdbpbin/dat.go`):**

Define a `NavMsg` interface (like `casbin.NavMsg`):
```
type NavMsg interface {
    Msg
    NavEpoch() uint32
}
```

Define `DatNavHeader` embedded struct (like `casbin.NavRunTime`):
```
type DatNavHeader struct {
    LocalTimestamp uint32
}
func (m *DatNavHeader) NavEpoch() uint32 { return m.LocalTimestamp }
```

Embed `DatNavHeader` as first field in all nav DAT structs. The existing
`DatGNSST` already has `LocalTimestamp` as first field -- refactor to
embed `DatNavHeader` instead.

**sdbpproc.go (`gps/internal/sdbp/sdbpproc.go`):**

Add epoch tracking fields to `PacketProcessor` (like CASIC):
- `curNavEpoch uint32` (0 = no epoch seen)
- `curNavEpochMsg *gpsprot.NavEpochMsg`

Add `handleNavEpoch(nm sdbpbin.NavMsg, tRead time.Time)` -- same
pattern as `casic.handleNavEpoch`: increment epoch value by 1 (so zero
means "not seen"), detect change, call `mgr.EpochStarted()`.

Implement `gpsprot.EpochFlusher` (`FlushNavEpoch`).

In `ProcessPacket`, check for `NavMsg` before dispatch (like CASIC
line 37-39):
```
if nm, ok := m.(sdbpbin.NavMsg); ok {
    p.handleNavEpoch(nm, tRead)
}
```

This integrates naturally -- existing GPST/BDST/GALT will start
triggering epoch changes once they embed `DatNavHeader`.

## Field layouts reference

**DatUTCT2** (06:1F, 31 bytes):
- LocalTimestamp U32, Valid U8 (B0:HMS B1:YMD B2:leap forecast
  B3:leap corr), RefGrid U8 (0=none 1=BDS 2=GPS 3=GLO 4=GAL),
  Year U16, Month U8, Day U8, Hour U8, Min U8, Sec U8,
  SecFrac I32 (ns), Accuracy U32 (ns),
  LeapSec I8 (current GPS-UTC offset),
  LeapCountdown I32 (>0=countdown, 0=occurring, -1=done),
  LeapChange I8, LeapYear U16, LeapMonth U8, LeapDay U8

**DatLLA3** (06:1D, 64 bytes):
- LocalTimestamp U32, CoordSys U8, Valid U8, TrackedSats U8,
  FixSats U8, Year U16, Month U8, Day U8, Hour U8, Min U8,
  SecMs U16, Lon F64 (deg), Lat F64 (deg), AltMSL F32 (m),
  GeoidSep F32 (m), HAcc U32 (mm), VAcc U32 (mm),
  GroundSpeed F32 (m/s), SpeedAcc U32 (cm/s),
  Heading F32 (deg), HeadingAcc U32 (0.01 deg)

**DatECEF2** (06:1B, 75 bytes):
- LocalTimestamp U32, Valid U8, TrackedSats U8, FixSats U8,
  Year U16, Month U8, Day U8, Hour U8, Min U8, SecMs U16,
  X/Y/Z F64 (m), XAcc/YAcc/ZAcc U32 (mm),
  VX/VY/VZ F32 (m/s), VXAcc/VYAcc/VZAcc U32 (cm/s)

**DatNED3** (06:1E, 40 bytes):
- LocalTimestamp U32, CoordSys U8, Valid U8, TrackedSats U8,
  FixSats U8, Year U16, Month U8, Day U8, Hour U8, Min U8,
  SecMs U16, VN/VE/VD F32 (m/s), VNAcc/VEAcc/VDAcc U32 (cm/s)

**DatDOP** (06:13, 16 bytes):
- LocalTimestamp U32, Valid U8, FixSats U8,
  GDOP/PDOP/HDOP/VDOP/TDOP U16 (scale 0.01)

**DatTSURV** (06:40, 25 bytes, no LocalTimestamp):
- Status U8 (0=idle, 1=in progress, 2=complete),
  ObsTime U32 (s), ObsCount U32,
  AvgX/Y/Z I32 (cm), AvgVariance U32 (10^-4 m^2)

**DatSAT** (06:30, variable, VaryingMsg):
- Fixed: LocalTimestamp U32, SatCount U8
- Per-sat (14 bytes): GNSSID U8, SatID U8, OrbitID I8, SignalID U8,
  CN0 U8, Elev I8 (deg), Azim U16 (deg),
  PRResidual I32 (cm), Flags U16 (B7=used in fix)

## sdbpbin test procedure for new message types

For each new sdbpbin struct, write a parse test using a real captured
packet:

1. Enable the message on the receiver using `satpulsetool gps -t`
2. Capture packets: `satpulsetool gps --capture 5 --packet-log <file>`
3. Decode to find the hex: `satpulsetool decode --packet-log <file>`
4. Copy a real packet hex string into a test case in
   `gps/lib/sdbpbin/common_test.go`
5. Parse with `sdbpbin.ParseMsg()`, assert the struct fields match
   expected values (cross-check against decoded output)
6. Serialize round-trip: `Serialize(parsed)` should reproduce the
   original packet bytes

This ensures the struct field layout matches the actual receiver
output. Do this for every new message type added in each phase.

## Phase 1: Leap second and UTC time (DatUTCT2)

End-to-end: enable DAT-UTCT2 + DAT-GPSU on receiver, run satpulsed,
see `time` and `leapSecond` events in the event log. Note: if no
future leap second is scheduled, UTCT2 may not produce leapSecond
events (only when leap forecast valid bits are set). The GNSSU
messages always carry DeltaTLS/DeltaTLSF so they should always
produce a leapSecond event.

### 1a. sdbpbin structs

`gps/lib/sdbpbin/dat.go`:
- Add `DatNavHeader` and `NavMsg` interface
- Refactor `DatGNSST` to embed `DatNavHeader`
- Add `DatUTCT2` struct (embeds `DatNavHeader`), register in `init()`
- Add `DatGNSSU` shared struct for GPSU/GALU/BDSU (35 bytes, no
  LocalTimestamp -- these are broadcast UTC parameters, not nav epoch):
  A0 F64 (s), A1 F64 (s/s), A2 F64 (s/s^2),
  TOT U32 (s), WNOT U16 (week),
  DeltaTLS I8 (current leap offset, s),
  WNLSF U16 (week of next leap), DN U8 (day),
  DeltaTLSF I8 (leap offset after event, s)
- Add `DatBDSU` (06:2C), `DatGPSU` (06:2D), `DatGALU` (06:2E),
  each embedding `DatGNSSU`, register in `init()`

### 1b. Conversion functions

`gps/internal/sdbp/sdbptime.go`:
- `timeDatUTCT2(m) -> *TimeMsg` -- build UTCTime from date/time
  fields + SecFrac, set Accuracy, GNSS from RefGrid
- `leapDatUTCT2(m) -> *LeapSecondMsg` -- extract leap second info
  when valid (bits 2+3 of Valid), build `ptime.LeapSecond`
- `leapDatGNSSU(m *DatGNSSU, gnss) -> *LeapSecondMsg` -- extract
  leap second from UTC parameters. DeltaTLS is current offset,
  DeltaTLSF is offset after next event. WNLSF/DN identify when.
  Convert WNLSF/DN to `ptime.LeapSecond.OffChangeTime`.
  DN interpretation differs: BDS uses 0-6 (Sun-Sat), GPS/GAL
  use 1-7 (Mon-Sun).

### 1c. Wire up dispatch

`gps/internal/sdbp/sdbpproc.go`:
- Add epoch tracking infrastructure (DatNavHeader, handleNavEpoch,
  FlushNavEpoch) -- needed because DatGNSST refactor means existing
  time messages now trigger epoch changes
- Add `DatUTCT2` case to dispatch: emit TimeMsg and LeapSecondMsg
- Add `DatBDSU`, `DatGPSU`, `DatGALU` cases: emit LeapSecondMsg
  (these are not nav epoch messages)

### 1d. Message tags

`configs/gpsmsg/techtotop.toml`:
- Add `sdbp-dat-utct2` / `sdbp-dat-utct2-off` tags
- Add `sdbp-dat-gpsu` / `sdbp-dat-gpsu-off` tags
- Add `sdbp-dat-galu` / `sdbp-dat-galu-off` tags
- Add `sdbp-dat-bdsu` / `sdbp-dat-bdsu-off` tags

### 1e. Tests and verification

- Unit test: UTCT2 -> TimeMsg conversion, leap second extraction
- Unit test: GNSSU -> LeapSecondMsg conversion
- Serialize round-trip tests for new structs
- `make test`

### 1f. Hardware verification (MANDATORY)

Each phase is NOT COMPLETE until hardware verification passes.

1. Enable messages on receiver:
   `satpulsetool gps -t sdbp-dat-utct2,sdbp-dat-gpsu`
2. Capture packets:
   `satpulsetool gps --capture 5 --packet-log /tmp/phase1.jsonl`
3. Decode to verify struct layout:
   `satpulsetool decode --packet-log /tmp/phase1.jsonl`
   Check: DAT-UTCT2 fields (date/time, SecFrac, LeapSec) are
   plausible. DAT-GPSU fields (DeltaTLS, A0/A1) are plausible.
4. Add real captured packet hex to sdbpbin parse + round-trip tests
   (see "sdbpbin test procedure" above) for DatUTCT2 and DatGPSU.
   Run `make test` again.
5. End-to-end: run satpulsed with event logging, check for:
   - `time` events with utcTime from DAT-UTCT2
   - `leapSecond` events from DAT-GPSU
   Use the `hardware-test-gps-msgs` skill.
6. Disable test messages:
   `satpulsetool gps -t sdbp-dat-utct2-off,sdbp-dat-gpsu-off`

## Phase 2: Position, velocity, DOP, and NavEpoch

End-to-end: enable LLA3 + ECEF2 + NED3 + DOP, run satpulsed, see
PosGeo, PosECEF, VelGeo, VelECEF, and NavEpoch events.

### 2a. sdbpbin structs

`gps/lib/sdbpbin/dat.go`:
- Add `DatLLA3`, `DatECEF2`, `DatNED3`, `DatDOP` (all embed
  `DatNavHeader`), register in `init()`

### 2b. Conversion functions

`gps/internal/sdbp/sdbppv.go` (new file):
- `posGeoDatLLA3(m) -> *PosGeoMsg` -- F64 deg -> nanodeg, F32 m -> um
- `velGeoDatLLA3(m) -> *VelGeoMsg` -- GroundSpeed, Course
- `posECEFDatECEF2(m) -> *PosECEFMsg` -- F64 m -> um
- `velECEFDatECEF2(m) -> *VelECEFMsg` -- F32 m/s -> um/s
- `velGeoDatNED3(m) -> *VelGeoMsg` -- NED F32 m/s -> um/s
- `dopDatDOP(msg *NavEpochMsg, m *DatDOP)` -- accumulate DOP into
  epoch msg (U16 * 0.01), set NumSVUsed

### 2c. Wire up dispatch

`gps/internal/sdbp/sdbpproc.go`:
- Add cases for LLA3, ECEF2, NED3, DOP
- LLA3 emits PosGeoMsg + VelGeoMsg
- ECEF2 emits PosECEFMsg + VelECEFMsg
- NED3 emits VelGeoMsg
- DOP accumulates into curNavEpochMsg
- NavEpoch is emitted via FlushNavEpoch when epoch changes

### 2d. Message tags

`configs/gpsmsg/techtotop.toml`:
- Add `sdbp-dat-lla3` / `sdbp-dat-lla3-off`
- Add `sdbp-dat-ned3` / `sdbp-dat-ned3-off`
- (ECEF2 and DOP tags already exist)

### 2e. Tests and verification

- Unit tests for all conversion functions
- Serialize round-trip tests for new structs
- `make test`

### 2f. Hardware verification (MANDATORY)

1. Enable messages: `satpulsetool gps -t sdbp-dat-lla3,sdbp-dat-ecef2,sdbp-dat-ned3,sdbp-dat-dop`
2. Capture: `satpulsetool gps --capture 5 --packet-log /tmp/phase2.jsonl`
3. Decode and verify field values are plausible
4. Add real captured packets to sdbpbin parse + round-trip tests
5. End-to-end with `hardware-test-gps-msgs` skill, check:
   - PosGeo lat/lon plausible
   - PosECEF vs known reference
   - VelGeo/VelECEF near zero (stationary)
   - NavEpoch with DOP values, ~1/s rate
6. Disable test messages

## Phase 3: Satellite info (DatSAT)

End-to-end: enable DAT-SAT, run satpulsed, see SatsInfo events.

### 3a. sdbpbin struct

`gps/lib/sdbpbin/dat.go`:
- Add `DatSAT` with `DatSATFixed` (embeds `DatNavHeader`) +
  `DatSATEntry` repeat. Implement `VaryingMsg` interface.

### 3b. Conversion function

`gps/internal/sdbp/sdbpsats.go` (new file):
- `satsDatSAT(m) -> *SatellitesMsg`
- Extract CN0, elevation, azimuth, used flag (Flags bit 7)

**GNSS ID mapping** (SDBP Appendix A -> `gpsprot.GNSS`):

| SDBP GNSS ID | Constellation | gpsprot.GNSS |
|---|---|---|
| 0 | BDS | gpsprot.BDS |
| 1 | GPS | gpsprot.GPS |
| 2 | QZSS | gpsprot.QZSS |
| 3 | SBAS | gpsprot.SBAS |
| 4 | Galileo | gpsprot.GAL |
| 5 | GLONASS | gpsprot.GLO |

**Signal ID mapping** (SDBP signal ID -> `gpsprot.Signal`):

BDS (GNSS ID 0):
| ID | Signal | gpsprot.Signal |
|----|--------|----------------|
| 0 | B1I | SigBDSB1I |
| 1 | B1C | SigBDSB1C |
| 2 | B2I | SigBDSB2I |
| 3 | B2A | SigBDSB2a |
| 4 | B2B | SigBDSB2b |
| 5 | B3I | SigBDSB3I |

GPS (GNSS ID 1):
| ID | Signal | gpsprot.Signal |
|----|--------|----------------|
| 0 | L1CA | SigGPSL1CA |
| 1 | L1C | SigGPSL1C |
| 2 | L2C | SigGPSL2C |
| 3 | L2P | SigGPSL2P |
| 4 | L5 | SigGPSL5 |

QZSS (GNSS ID 2):
| ID | Signal | gpsprot.Signal |
|----|--------|----------------|
| 0 | L1CA | SigQZSSL1CA |
| 1 | L1C | SigQZSSL1C |
| 2 | L2C | SigQZSSL2C |
| 3 | L2P | (no gpsprot equivalent) |
| 4 | L5 | SigQZSSL5 |

SBAS (GNSS ID 3):
| ID | Signal | gpsprot.Signal |
|----|--------|----------------|
| 0 | L1CA | SigSBASL1CA |

Galileo (GNSS ID 4):
| ID | Signal | gpsprot.Signal |
|----|--------|----------------|
| 0 | E1 | SigGALE1 |
| 1 | E5A | SigGALE5a |
| 2 | E5B | SigGALE5b |

GLONASS (GNSS ID 5):
| ID | Signal | gpsprot.Signal |
|----|--------|----------------|
| 0 | G1 | SigGLOL1 |
| 1 | G2 | SigGLOL2 |

Implement as a lookup table indexed by [gnssID][signalID].

**Signal ID 7** (undocumented): appears across all constellations for
satellites known from almanac/ephemeris but not actually tracked.
Always CN0=0. Not in Appendix A. Confirmed by cross-checking with
NMEA GSV: sigID=7 entries never appear in GSV output. These entries
are skipped (no signal data to report).

**SignalID mapping** (for `SatellitesMsg.Signals[].SignalID`):
Use the corresponding `gpsprot.SigID*` constants. E.g.:
- GPS L1CA -> SigIDGPSL1CA ("L1 C/A")
- GAL E1 -> SigIDGALE1 ("E1")
- BDS B1I -> SigIDBDSB1I ("B1I")
- GLO G1 -> SigIDGLOL1 ("L1")
etc.

### 3c. Wire up dispatch

Add DatSAT case to dispatch, emit SatellitesMsg.

### 3d. Message tags

`configs/gpsmsg/techtotop.toml`:
- Add `sdbp-dat-sat` / `sdbp-dat-sat-off`

### 3e. Tests and verification

- Unit test for SAT conversion
- Serialize round-trip tests
- `make test`

### 3f. Hardware verification (MANDATORY)

1. Enable messages: `satpulsetool gps -t sdbp-dat-sat`
2. Capture: `satpulsetool gps --capture 5 --packet-log /tmp/phase3.jsonl`
3. Decode and verify field values are plausible
4. Add real captured packet to sdbpbin parse + round-trip test
5. End-to-end with `hardware-test-gps-msgs` skill, check SatsInfo events
6. Cross-check: enable DAT-SAT + NMEA GSV + GSA, capture packets,
   and verify:
   - Same satellites appear in both (matching GNSS + PRN)
   - CN0 values are similar between DAT-SAT and GSV
   - Satellites marked as used in DAT-SAT (Flags bit 7) match GSA PRNs
   - Signal IDs map to expected signals for each constellation
7. Disable test messages

## Phase 4: Survey progress, config polls, and TMODE

End-to-end: set receiver to survey mode, see SurveyProgress events,
query and decode TMODE config.

### 4a. sdbpbin structs

`gps/lib/sdbpbin/dat.go`:
- Add `DatTSURV` (no DatNavHeader -- not part of nav epoch)

`gps/lib/sdbpbin/cfg.go`:
- Add `CfgTMODE` (03:43), `CfgTMODE2` (03:45), `CfgDELAY2` (03:44)
- Register in `init()`

### 4b. Conversion function

`gps/internal/sdbp/sdbpproc.go` (inline or small helper):
- `surveyDatTSURV(m) -> *SurveyMsg`
- Position cm -> um, ObsTime, ObsCount
- InProgress = Status==1, Valid = Status==2

### 4c. Wire up dispatch

Add DatTSURV case (no epoch handling).

### 4d. Message tags

`configs/gpsmsg/techtotop.toml`:
- `sdbp-dat-tsurv` / `sdbp-dat-tsurv-off`
- `get-tmode` - query CFG-TMODE (receiver-specific)
- `get-survey` - alias for get-tmode
- `survey` - set autonomous survey mode (default: 2000s, 20m accuracy)
- `survey-off` - set normal positioning mode
- `get-fixed-pos` - query CFG-TMODE
- `fixed-pos-example` - set fixed position mode (example ECEF coords)
- `fixed-pos-off` - clear fixed position (back to normal)
- `get-delay` - query CFG-DELAY2

### 4e. Tests and verification

- Serialize round-trip tests for new structs
- `make test`

### 4f. Hardware verification (MANDATORY)

1. Enable messages: `satpulsetool gps -t sdbp-dat-tsurv`
2. Capture and decode DatTSURV, verify fields plausible
3. Add real captured packet to sdbpbin parse + round-trip test
4. `satpulsetool gps -t survey` to start survey mode
5. End-to-end with `hardware-test-gps-msgs` skill, check
   SurveyProgress events
6. `satpulsetool gps -t get-tmode` to verify decoded config response
7. `satpulsetool gps -t survey-off` to restore normal mode
8. Disable test messages
