package sdbpbin

import "fmt"

const (
	DatDOPID   MsgID = clsDAT | (0x13 << 8)
	DatBDSTID  MsgID = clsDAT | (0x16 << 8)
	DatGPSTID  MsgID = clsDAT | (0x17 << 8)
	DatGALTID  MsgID = clsDAT | (0x19 << 8)
	DatECEF2ID MsgID = clsDAT | (0x1B << 8)
	DatLLA3ID  MsgID = clsDAT | (0x1D << 8)
	DatNED3ID  MsgID = clsDAT | (0x1E << 8)
	DatUTCT2ID MsgID = clsDAT | (0x1F << 8)
	DatBDSUID  MsgID = clsDAT | (0x2C << 8)
	DatGPSUID  MsgID = clsDAT | (0x2D << 8)
	DatGALUID  MsgID = clsDAT | (0x2E << 8)
	DatSATID   MsgID = clsDAT | (0x30 << 8)
	DatTSURVID MsgID = clsDAT | (0x40 << 8)
	DatTPPSID  MsgID = clsDAT | (0x41 << 8)
)

// NavMsg is implemented by DAT messages that participate in nav epoch tracking.
type NavMsg interface {
	Msg
	NavEpoch() uint32
}

// DatNavHeader is embedded in DAT messages that have a leading LocalTimestamp field.
type DatNavHeader struct {
	LocalTimestamp uint32
}

// NavEpoch returns the epoch identifier (LocalTimestamp).
func (m *DatNavHeader) NavEpoch() uint32 { return m.LocalTimestamp }

// DatTimeValid is a bitmask for DAT-*T validity flags.
type DatTimeValid uint8

const (
	DatTimeTowValid DatTimeValid = 1 << iota
	DatTimeWeekValid
)

// TimeAccuracyInvalid is the sentinel value for unknown time accuracy.
const TimeAccuracyInvalid = 0xFFFFFFFF

// DatGNSST is the common layout for DAT-BDST, DAT-GPST, and DAT-GALT (19 bytes).
type DatGNSST struct {
	DatNavHeader
	Valid        DatTimeValid
	Week         uint16
	TOW          float64 // seconds of week
	TimeAccuracy uint32  // nanoseconds; TimeAccuracyInvalid means unknown
}

// TimeValid reports whether the time fields are valid.
func (m *DatGNSST) TimeValid() bool {
	return m.Valid&(DatTimeTowValid|DatTimeWeekValid) == DatTimeTowValid|DatTimeWeekValid
}

// DatBDST is DAT-BDST (class 0x06, id 0x16). BeiDou time.
type DatBDST struct{ DatGNSST }

func (m *DatBDST) ID() MsgID { return DatBDSTID }

// DatGPST is DAT-GPST (class 0x06, id 0x17). GPS time.
type DatGPST struct{ DatGNSST }

func (m *DatGPST) ID() MsgID { return DatGPSTID }

// DatGALT is DAT-GALT (class 0x06, id 0x19). Galileo time.
type DatGALT struct{ DatGNSST }

func (m *DatGALT) ID() MsgID { return DatGALTID }

// UTCType identifies the UTC realization used when TimeRef is UTC.
type UTCType uint8

const (
	UTCInvalid UTCType = 0 // time reference != UTC or no UTC correction
	UTCBDS     UTCType = 1
	UTCGPS     UTCType = 2
	UTCGLO     UTCType = 3
	UTCGAL     UTCType = 4
)

// DatTPPS is DAT-TPPS (class 0x06, id 0x41).
type DatTPPS struct {
	PPSIndex    uint8
	TOW         float64 // pulse time, seconds of week
	Week        uint16
	PPSResidual int32   // picoseconds
	TimeRef     TimeRef // time reference
	UTCType     UTCType // UTC source
}

func (m *DatTPPS) ID() MsgID { return DatTPPSID }

// DatUTCT2Valid is a bitmask for DatUTCT2 validity flags.
type DatUTCT2Valid uint8

const (
	DatUTCT2HMS          DatUTCT2Valid = 1 << iota // bit 0: H/M/S valid
	DatUTCT2YMD                                    // bit 1: Y/M/D valid
	DatUTCT2LeapForecast                           // bit 2: leap second forecast valid
	DatUTCT2LeapCorr                               // bit 3: leap second correction valid
)

// DatUTCT2RefGrid identifies the reference time grid for DatUTCT2.
type DatUTCT2RefGrid uint8

const (
	DatUTCT2RefNone DatUTCT2RefGrid = iota
	DatUTCT2RefBDS
	DatUTCT2RefGPS
	DatUTCT2RefGLO
	DatUTCT2RefGAL
)

// DatUTCT2 is DAT-UTCT2 (class 0x06, id 0x1F, 31 bytes).
type DatUTCT2 struct {
	DatNavHeader
	Valid         DatUTCT2Valid
	RefGrid       DatUTCT2RefGrid
	Year          uint16
	Month         uint8
	Day           uint8
	Hour          uint8
	Min           uint8
	Sec           uint8
	SecFrac       int32  // nanoseconds
	Accuracy      uint32 // nanoseconds
	LeapSec       int8   // current GPS-UTC offset (seconds)
	LeapCountdown int32  // >0=countdown, 0=occurring, -1=done
	LeapChange    int8   // +1 or -1
	LeapYear      uint16
	LeapMonth     uint8
	LeapDay       uint8
}

func (m *DatUTCT2) ID() MsgID { return DatUTCT2ID }

// DatGNSSU is the shared layout for DAT-GPSU, DAT-GALU, DAT-BDSU (35 bytes).
// These carry broadcast UTC parameters, not per-epoch nav data.
type DatGNSSU struct {
	A0        float64 // seconds
	A1        float64 // s/s
	A2        float64 // s/s^2
	TOT       uint32  // seconds
	WNOT      uint16  // week
	DeltaTLS  int8    // current leap offset (seconds since GNSS epoch)
	WNLSF     uint16  // week of next leap
	DN        uint8   // day number
	DeltaTLSF int8    // leap offset after event
}

// DatBDSU is DAT-BDSU (class 0x06, id 0x2C). BeiDou UTC parameters.
type DatBDSU struct{ DatGNSSU }

func (m *DatBDSU) ID() MsgID { return DatBDSUID }

// DatGPSU is DAT-GPSU (class 0x06, id 0x2D). GPS UTC parameters.
type DatGPSU struct{ DatGNSSU }

func (m *DatGPSU) ID() MsgID { return DatGPSUID }

// DatGALU is DAT-GALU (class 0x06, id 0x2E). Galileo UTC parameters.
type DatGALU struct{ DatGNSSU }

func (m *DatGALU) ID() MsgID { return DatGALUID }

// DatDOPMeter is 1 meter in DatDOP units.
const DatDOPMeter = 100

// DatValid is the Validity Flag byte in DAT nav messages.
// The low nibble encodes position type; the high nibble encodes fix quality.
type DatValid byte

// Position type values (low nibble).
const (
	DatValidPosUnavailable DatValid = iota
	DatValidPosDR
	DatValidPos2D
	DatValidPos3D
	DatValidPosTiming
	DatValidPosType DatValid = 0x0F
)

// Fix quality values (high nibble).
const (
	DatValidFixUnavailable  DatValid = iota << 4
	DatValidFixOpen
	DatValidFixDifferential
	DatValidFixAuthorized
	DatValidFixRTKFixed
	DatValidFixRTKFloat
	DatValidFixDR
	DatValidFixQual DatValid = 0xF0
)

// DatDOP is DAT-DOP (class 0x06, id 0x13, 16 bytes).
type DatDOP struct {
	DatNavHeader
	Valid   DatValid
	FixSats uint8
	GDOP    uint16 // scale 0.01
	PDOP    uint16
	HDOP    uint16
	VDOP    uint16
	TDOP    uint16
}

func (m *DatDOP) ID() MsgID { return DatDOPID }

// DatECEF2 is DAT-ECEF2 (class 0x06, id 0x1B, 75 bytes).
type DatECEF2 struct {
	DatNavHeader
	Valid       DatValid
	TrackedSats uint8
	FixSats     uint8
	Year        uint16
	Month       uint8
	Day         uint8
	Hour        uint8
	Min         uint8
	SecMs       uint16  // seconds * 1000
	X           float64 // meters
	Y           float64
	Z           float64
	XAcc        uint32 // mm
	YAcc        uint32
	ZAcc        uint32
	VX          float32 // m/s
	VY          float32
	VZ          float32
	VXAcc       uint32 // cm/s
	VYAcc       uint32
	VZAcc       uint32
}

func (m *DatECEF2) ID() MsgID { return DatECEF2ID }

// DatLLA3 is DAT-LLA3 (class 0x06, id 0x1D, 64 bytes).
type DatLLA3 struct {
	DatNavHeader
	CoordSys    uint8
	Valid       DatValid
	TrackedSats uint8
	FixSats     uint8
	Year        uint16
	Month       uint8
	Day         uint8
	Hour        uint8
	Min         uint8
	SecMs       uint16  // seconds * 1000
	Lon         float64 // degrees
	Lat         float64
	AltMSL      float32 // meters
	GeoidSep    float32 // meters
	HAcc        uint32  // mm
	VAcc        uint32  // mm
	GroundSpeed float32 // m/s
	SpeedAcc    uint32  // cm/s
	Heading     float32 // degrees
	HeadingAcc  uint32  // 0.01 degrees
}

func (m *DatLLA3) ID() MsgID { return DatLLA3ID }

// DatNED3 is DAT-NED3 (class 0x06, id 0x1E, 40 bytes).
type DatNED3 struct {
	DatNavHeader
	CoordSys    uint8
	Valid       DatValid
	TrackedSats uint8
	FixSats     uint8
	Year        uint16
	Month       uint8
	Day         uint8
	Hour        uint8
	Min         uint8
	SecMs       uint16
	VN          float32 // m/s
	VE          float32
	VD          float32
	VNAcc       uint32 // cm/s
	VEAcc       uint32
	VDAcc       uint32
}

func (m *DatNED3) ID() MsgID { return DatNED3ID }

// GNSSID identifies a GNSS constellation in DAT-SAT.
type GNSSID uint8

const (
	BDS  GNSSID = 0
	GPS  GNSSID = 1
	QZSS GNSSID = 2
	SBAS GNSSID = 3
	GAL  GNSSID = 4
	GLO  GNSSID = 5
)

// SignalID identifies a signal within a constellation in DAT-SAT.
type SignalID uint8

// BDS signal IDs
const (
	SigBDSB1I SignalID = 0
	SigBDSB1C SignalID = 1
	SigBDSB2I SignalID = 2
	SigBDSB2a SignalID = 3
	SigBDSB2b SignalID = 4
	SigBDSB3I SignalID = 5
)

// GPS signal IDs
const (
	SigGPSL1CA SignalID = 0
	SigGPSL1C  SignalID = 1
	SigGPSL2C  SignalID = 2
	SigGPSL2P  SignalID = 3
	SigGPSL5   SignalID = 4
)

// QZSS signal IDs
const (
	SigQZSSL1CA SignalID = 0
	SigQZSSL1C  SignalID = 1
	SigQZSSL2C  SignalID = 2
	SigQZSSL2P  SignalID = 3
	SigQZSSL5   SignalID = 4
)

// SBAS signal IDs
const (
	SigSBASL1CA SignalID = 0
)

// Galileo signal IDs
const (
	SigGALE1  SignalID = 0
	SigGALE5a SignalID = 1
	SigGALE5b SignalID = 2
)

// GLONASS signal IDs
const (
	SigGLOG1 SignalID = 0
	SigGLOG2 SignalID = 1
)

// DatSATFixed is the fixed part of DAT-SAT (class 0x06, id 0x30).
type DatSATFixed struct {
	DatNavHeader
	SatCount uint8
}

// DatSATEntry is a per-satellite entry in DAT-SAT (14 bytes).
type DatSATEntry struct {
	GNSSID     GNSSID
	SatID      uint8
	OrbitID    int8
	SignalID   SignalID
	CN0        uint8
	Elev       int8   // degrees
	Azim       uint16 // degrees
	PRResidual int32  // cm
	Flags      uint16 // bit 7 = used in fix
}

// DatSAT is DAT-SAT (class 0x06, id 0x30, variable length).
type DatSAT struct {
	DatSATFixed
	Sats []DatSATEntry
}

func (m *DatSAT) ID() MsgID { return DatSATID }

func (m *DatSAT) InitVaryingPart(payloadLen int) error {
	want := 5 + int(m.SatCount)*14
	if payloadLen < want {
		return fmt.Errorf("DAT-SAT payload %d bytes, want %d for %d sats", payloadLen, want, m.SatCount)
	}
	m.Sats = make([]DatSATEntry, m.SatCount)
	return nil
}

func (m *DatSAT) FixedPart() any   { return &m.DatSATFixed }
func (m *DatSAT) VaryingPart() any { return &m.Sats }

var _ VaryingMsg = (*DatSAT)(nil)

// DatTSURV is DAT-TSURV (class 0x06, id 0x40, 25 bytes).
// Timing autonomous survey status. No LocalTimestamp (not part of nav epoch).
type DatTSURV struct {
	Status      uint8  // 0=idle, 1=in progress, 2=complete
	ObsTime     uint32 // seconds
	ObsCount    uint32
	AvgX        int32  // 10^-2 m (cm)
	AvgY        int32
	AvgZ        int32
	AvgVariance uint32 // 10^-4 m^2
}

func (m *DatTSURV) ID() MsgID { return DatTSURVID }

func init() {
	regMsg[DatBDST]("BDST")
	regMsg[DatGPST]("GPST")
	regMsg[DatGALT]("GALT")
	regMsg[DatTPPS]("TPPS")
	regMsg[DatUTCT2]("UTCT2")
	regMsg[DatBDSU]("BDSU")
	regMsg[DatGPSU]("GPSU")
	regMsg[DatGALU]("GALU")
	regMsg[DatDOP]("DOP")
	regMsg[DatECEF2]("ECEF2")
	regMsg[DatLLA3]("LLA3")
	regMsg[DatNED3]("NED3")
	regMsg[DatSAT]("SAT")
	regMsg[DatTSURV]("TSURV")
}
