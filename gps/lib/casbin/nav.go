package casbin

const (
	NavDopID     MsgID = clsNav | (0x01 << 8)
	NavSolID     MsgID = clsNav | (0x02 << 8)
	NavPvID      MsgID = clsNav | (0x03 << 8)
	NavTimeUTCID MsgID = clsNav | (0x10 << 8)
	NavClockID   MsgID = clsNav | (0x11 << 8)
	NavGPSInfoID MsgID = clsNav | (0x20 << 8)
	NavBDSInfoID MsgID = clsNav | (0x21 << 8)
	NavGLNInfoID MsgID = clsNav | (0x22 << 8)
)

// NavMsg is implemented by NAV messages that have a RunTime field
type NavMsg interface {
	Msg
	NavEpoch() uint32
}

// NavRunTime is embedded in NAV messages to provide epoch tracking
type NavRunTime struct {
	RunTime uint32 // ms since boot/reset
}

func (m *NavRunTime) NavEpoch() uint32 {
	return m.RunTime
}

// NavSol is NAV-SOL (0x01 0x02) - reduced PVT navigation information (72 bytes)
type NavSol struct {
	NavRunTime
	PosValid NavPosValid
	VelValid NavVelValid
	TimeSrc  GNSSID
	System   NavSystem
	NumSV    uint8
	NumSVGPS uint8
	NumSVBDS uint8
	NumSVGLN uint8
	_        uint16 // reserved
	Week     uint16
	TOW      float64 // seconds
	ECEFX    float64 // meters
	ECEFY    float64 // meters
	ECEFZ    float64 // meters
	PAcc     float32 // m², variance of 3D position
	ECEFVX   float32 // m/s
	ECEFVY   float32 // m/s
	ECEFVZ   float32 // m/s
	SAcc     float32 // (m/s)², variance of 3D velocity
	PDOP     float32
}

func (m *NavSol) ID() MsgID { return NavSolID }

// NavPv is NAV-PV (0x01 0x03) - geodetic position and velocity (80 bytes)
type NavPv struct {
	NavRunTime
	PosValid NavPosValid
	VelValid NavVelValid
	System   NavSystem
	NumSV    uint8
	NumSVGPS uint8
	NumSVBDS uint8
	NumSVGLN uint8
	_        uint8 // reserved
	PDOP     float32
	Lon      float64 // deg
	Lat      float64 // deg
	Height   float32 // m, ellipsoidal
	SepGeoid float32 // m, geoid separation (ellipsoidal minus MSL)
	HAcc     float32 // m^2, variance of horizontal position accuracy
	VAcc     float32 // m^2, variance of vertical position accuracy
	VelN     float32 // m/s
	VelE     float32 // m/s
	VelU     float32 // m/s (UP, not down -- negate for NED)
	Speed3D  float32 // m/s
	Speed2D  float32 // m/s, ground speed
	Heading  float32 // deg
	SAcc     float32 // (m/s)^2, variance of ground speed accuracy
	CAcc     float32 // deg^2, variance of heading accuracy
}

func (m *NavPv) ID() MsgID { return NavPvID }

// NavDop is NAV-DOP (0x01 0x01) - dilution of precision (28 bytes)
type NavDop struct {
	NavRunTime
	PDOP float32
	HDOP float32
	VDOP float32
	NDOP float32
	EDOP float32
	TDOP float32
}

func (m *NavDop) ID() MsgID { return NavDopID }

type NavPosValid uint8

const (
	NavPosInvalid NavPosValid = iota
	NavPosExternal
	NavPosRoughEstimate
	NavPosMaintaining
	NavPosDeadReckoning
	NavPosQuickMode
	NavPos2D
	NavPos3D
	NavPosGNSSDR // GNSS+DR integrated navigation
)

type NavVelValid uint8

const (
	NavVelInvalid NavVelValid = iota
	NavVelExternal
	NavVelRoughEstimate
	NavVelMaintaining
	NavVelReckoning
	NavVelQuickMode
	NavVel2D
	NavVel3D
	NavVelGNSSDR // GNSS+DR integrated navigation
)

type NavSystem uint8

const (
	NavSystemGPS NavSystem = 1 << iota
	NavSystemBDS
	NavSystemGLN
)

// NavTimeUTC is NAV-TIMEUTC (0x01 0x10) - UTC time information (24 bytes)
type NavTimeUTC struct {
	NavRunTime
	TAcc      float32 // s², time estimation accuracy
	MsErr     float32 // ms, residual error after rounding
	_         uint16  // padding (observed in real packets)
	Year      uint16  // 1999-2099
	Month     uint8   // 1-12
	Day       uint8   // 1-31
	Hour      uint8   // 0-23
	Min       uint8   // 0-59
	Sec       uint8   // 0-59
	Valid     NavTimeUTCValid
	TimeSrc   GNSSID
	DateValid NavDateValid
}

func (m *NavTimeUTC) ID() MsgID { return NavTimeUTCID }

type NavTimeUTCValid uint8

const (
	NavTimeUTCTOWValid  NavTimeUTCValid = 1 << iota // UTC TOW valid
	NavTimeUTCWeekValid                             // UTC week number valid
	NavTimeUTCLeapValid                             // UTC leap second valid
)

type NavDateValid uint8

const (
	NavDateInvalid NavDateValid = iota
	NavDateExternal
	NavDateFromSatellite
	NavDateMultipleSats // reliable
)

// NavClock is NAV-CLOCK (0x01 0x11) - clock solution information (64 bytes)
type NavClock struct {
	NavRunTime
	FreqBias float32        // s², clock drift
	TAcc     float32        // 1/c², time accuracy variance
	FAcc     float32        // 1/c², frequency accuracy variance
	Systems  [3]NavClockSys // GPS=0, BDS=1, GLN=2
}

func (m *NavClock) ID() MsgID { return NavClockID }

// NavClockSys contains per-system clock information
type NavClockSys struct {
	TOW   float64 // ms, time of week
	DtUTC float32 // s, fractional seconds diff between sat time and UTC
	Wn    uint16  // week number
	Leaps int8    // UTC leap seconds
	Valid uint8   // time validity flag
}

// NavSatInfoFixed is the common fixed part for NAV-GPSINFO, NAV-BDSINFO, NAV-GLNINFO
type NavSatInfoFixed struct {
	NavRunTime
	NumViewSV uint8  // number of visible satellites
	NumFixSV  uint8  // number of satellites used for fix
	System    GNSSID // system type
	_         uint8  // reserved
}

// NavSVInfo is common satellite info structure used by NAV-GPSINFO, NAV-BDSINFO, NAV-GLNINFO
type NavSVInfo struct {
	Chn     uint8 // channel number
	SVID    uint8 // satellite ID
	Flags   NavSVFlags
	Quality NavSVQuality
	CNO     uint8   // dB-Hz
	Elev    int8    // degrees, -90 to 90
	Azim    int16   // degrees, 0 to 360
	PRRes   float32 // meters, pseudorange residual
}

type NavSVFlags uint8

const (
	NavSVUsed           NavSVFlags = 1 << iota // satellite used in solution
	_                                          // reserved
	_                                          // reserved
	_                                          // reserved
	NavSVPredictInvalid                        // prediction info invalid
	_                                          // reserved
	// bits 6-7: prediction source
	// 00 = reserved
	// 01 = based on almanac
	// 10 = reserved
	// 11 = based on ephemeris
)

const NavSVOrbitMask NavSVFlags = 0xC0

const (
	NavSVOrbitAlmanac   NavSVFlags = 0x40
	NavSVOrbitEphemeris NavSVFlags = 0xC0
)

type NavSVQuality uint8

const (
	NavSVQualityPRValid             NavSVQuality = 1 << iota // pseudorange valid
	NavSVQualityCPValid                                      // carrier phase valid
	NavSVQualityHalfCycleValid                               // half-cycle ambiguity valid
	NavSVQualityHalfCycleSubtracted                          // half-cycle subtracted
	_                                                        // reserved
	NavSVQualityFreqValid                                    // carrier frequency valid
)

// NavGPSInfo is NAV-GPSINFO (0x01 0x20) - GPS satellite information
type NavGPSInfo struct {
	NavSatInfoFixed
	SVs []NavSVInfo
}

func (m *NavGPSInfo) ID() MsgID { return NavGPSInfoID }

func (m *NavGPSInfo) InitVaryingPart(payloadLen int) (err error) {
	n, err := sliceLen(m, payloadLen, 8, 12)
	if err == nil {
		m.SVs = make([]NavSVInfo, n)
	}
	return
}

func (m *NavGPSInfo) FixedPart() any   { return &m.NavSatInfoFixed }
func (m *NavGPSInfo) VaryingPart() any { return &m.SVs }

var _ VaryingMsg = (*NavGPSInfo)(nil)

// NavBDSInfo is NAV-BDSINFO (0x01 0x21) - BDS satellite information
type NavBDSInfo struct {
	NavSatInfoFixed
	SVs []NavSVInfo
}

func (m *NavBDSInfo) ID() MsgID { return NavBDSInfoID }

func (m *NavBDSInfo) InitVaryingPart(payloadLen int) (err error) {
	n, err := sliceLen(m, payloadLen, 8, 12)
	if err == nil {
		m.SVs = make([]NavSVInfo, n)
	}
	return
}

func (m *NavBDSInfo) FixedPart() any   { return &m.NavSatInfoFixed }
func (m *NavBDSInfo) VaryingPart() any { return &m.SVs }

var _ VaryingMsg = (*NavBDSInfo)(nil)

// NavGLNInfo is NAV-GLNINFO (0x01 0x22) - GLONASS satellite information
type NavGLNInfo struct {
	NavSatInfoFixed
	SVs []NavSVInfo
}

func (m *NavGLNInfo) ID() MsgID { return NavGLNInfoID }

func (m *NavGLNInfo) InitVaryingPart(payloadLen int) (err error) {
	n, err := sliceLen(m, payloadLen, 8, 12)
	if err == nil {
		m.SVs = make([]NavSVInfo, n)
	}
	return
}

func (m *NavGLNInfo) FixedPart() any   { return &m.NavSatInfoFixed }
func (m *NavGLNInfo) VaryingPart() any { return &m.SVs }

var _ VaryingMsg = (*NavGLNInfo)(nil)

// Nav2TOW is embedded in NAV2 messages to provide epoch tracking via GPS TOW.
type Nav2TOW struct {
	TOW int32 // GPS time of week in ms
}

func (m *Nav2TOW) NavEpoch() uint32 { return uint32(m.TOW) }

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

type Nav2GnssMask uint8

const (
	Nav2GnssGPS Nav2GnssMask = 1 << iota
	Nav2GnssBDS
	Nav2GnssGLN
	Nav2GnssGAL
	Nav2GnssQZSS
	Nav2GnssSBAS
	Nav2GnssIRNSS
)

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

const (
	Nav2DopID     MsgID = clsNav2 | (0x01 << 8)
	Nav2SolID     MsgID = clsNav2 | (0x02 << 8)
	Nav2PvhID     MsgID = clsNav2 | (0x03 << 8)
	Nav2TimeUTCID MsgID = clsNav2 | (0x05 << 8)
	Nav2SigID     MsgID = clsNav2 | (0x06 << 8)
)

// Nav2Sol is NAV2-SOL (0x11 0x02) - ECEF position and velocity (72 bytes)
type Nav2Sol struct {
	Nav2TOW
	Wn         uint16
	_          uint16 // reserved
	FixFlags   PVTValid
	VelFlags   Nav2VelFlags
	_          uint8 // reserved
	GnssMask   Nav2GnssMask
	NumFixTot  uint8
	NumFixGPS  uint8
	NumFixBDS  uint8
	NumFixGLN  uint8
	NumFixGAL  uint8
	NumFixQZSS uint8
	NumFixSBAS uint8
	NumFixIRN  uint8
	_          uint32  // reserved
	X          float64 // m, ECEF X
	Y          float64 // m, ECEF Y
	Z          float64 // m, ECEF Z
	PAcc       float32 // m, 3D position accuracy (std dev)
	VX         float32 // m/s, ECEF X velocity
	VY         float32 // m/s, ECEF Y velocity
	VZ         float32 // m/s, ECEF Z velocity
	SAcc       float32 // m/s, 3D speed accuracy (std dev)
	PDOP       float32
}

func (m *Nav2Sol) ID() MsgID { return Nav2SolID }

// Nav2Pvh is NAV2-PVH (0x11 0x03) - geodetic position and velocity (88 bytes)
type Nav2Pvh struct {
	Nav2TOW
	Wn         uint16
	_          uint16 // reserved
	FixFlags   PVTValid
	VelFlags   Nav2VelFlags
	_          uint8 // reserved
	GnssMask   Nav2GnssMask
	NumFixTot  uint8
	NumFixGPS  uint8
	NumFixBDS  uint8
	NumFixGLN  uint8
	NumFixGAL  uint8
	NumFixQZSS uint8
	NumFixSBAS uint8
	NumFixIRN  uint8
	_          uint32  // reserved
	Lon        float64 // deg
	Lat        float64 // deg
	Height     float32 // m, ellipsoidal
	SepGeoid   float32 // m, geoid separation
	VelE       float32 // m/s, East velocity
	VelN       float32 // m/s, North velocity
	VelU       float32 // m/s, Up velocity (negate for NED down)
	Speed3D    float32 // m/s
	Speed2D    float32 // m/s, ground speed
	Heading    float32 // deg
	HAcc       float32 // m, horizontal position accuracy (std dev)
	VAcc       float32 // m, vertical position accuracy (std dev)
	SAcc       float32 // m/s, 3D speed accuracy (std dev)
	CAcc       float32 // deg, heading accuracy (std dev)
}

func (m *Nav2Pvh) ID() MsgID { return Nav2PvhID }

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

// Nav2TimeUTC is NAV2-TIMEUTC (0x11 0x05) - UTC time information (20 bytes)
type Nav2TimeUTC struct {
	TAcc    float32 // ns, time accuracy estimate
	Subms   int32   // ms, fractional ms (scale 2^-30)
	Subcs   int8    // ms, centisecond error (-5 to 5 ms)
	Cs      uint8   // centiseconds (0-99)
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

func (m *Nav2TimeUTC) ID() MsgID { return Nav2TimeUTCID }

type Nav2TimeFlags uint8

const (
	Nav2TimeTOWValid Nav2TimeFlags = 1 << iota
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

// Nav2SigFixed is the fixed part of NAV2-SIG (8 bytes)
type Nav2SigFixed struct {
	TOW       uint32 // GPS TOW in ms
	_         uint8  // reserved
	NumTrkTot uint8
	NumFixTot uint8
	_         uint8 // reserved
}

func (m *Nav2SigFixed) NavEpoch() uint32 { return m.TOW }

// Nav2SigInfo is a per-signal entry in NAV2-SIG (16 bytes each)
type Nav2SigInfo struct {
	GNSSID    GNSSID
	SVID      uint8  // satellite ID (raw PRN, except QZSS=PRN-192)
	SigID     SigID  // signal band ID
	FreqID    uint8  // GLONASS frequency ID; undefined for other constellations
	PRRes     int16  // dm, pseudorange residual
	CNO       uint8  // dBHz
	TrkInd    uint8  // signal quality
	CorFlags  uint8  // correction flag
	SolFlags  uint8  // solution flag
	Chn       uint8  // tracking channel number
	Elev      uint8  // deg
	Azim      uint16 // deg
	IonoDelay int16  // dm, ionosphere delay correction
}

// Nav2Sig is NAV2-SIG (0x11 0x06) - per-signal tracking information.
// The receiver appends undocumented trailing bytes after the signal entries;
// AllowTrailingBytes permits ParseMsg to accept them.
type Nav2Sig struct {
	Nav2SigFixed
	Sigs []Nav2SigInfo
}

func (m *Nav2Sig) ID() MsgID { return Nav2SigID }

func (m *Nav2Sig) InitVaryingPart(payloadLen int) error {
	m.Sigs = make([]Nav2SigInfo, m.NumTrkTot)
	return nil
}

func (m *Nav2Sig) FixedPart() any      { return &m.Nav2SigFixed }
func (m *Nav2Sig) VaryingPart() any    { return &m.Sigs }
func (m *Nav2Sig) AllowTrailingBytes() {}

var _ VaryingMsg = (*Nav2Sig)(nil)
var _ AllowTrailing = (*Nav2Sig)(nil)

func init() {
	regMsg[NavDop]("DOP")
	regMsg[NavSol]("SOL")
	regMsg[NavPv]("PV")
	regMsg[NavTimeUTC]("TIMEUTC")
	regMsg[NavClock]("CLOCK")
	regMsg[NavGPSInfo]("GPSINFO")
	regMsg[NavBDSInfo]("BDSINFO")
	regMsg[NavGLNInfo]("GLNINFO")
	regMsg[Nav2Dop]("DOP")
	regMsg[Nav2Sol]("SOL")
	regMsg[Nav2Pvh]("PVH")
	regMsg[Nav2TimeUTC]("TIMEUTC")
	regMsg[Nav2Sig]("SIG")
}
