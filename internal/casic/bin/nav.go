package bin

const (
	NavSolID     MsgID = clsNav | (0x02 << 8)
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
	PosValid  NavPosValid
	VelValid  NavVelValid
	TimeSrc   NavTimeSrc
	System    NavSystem
	NumSV     uint8
	NumSVGPS  uint8
	NumSVBDS  uint8
	NumSVGLN  uint8
	_         uint16 // reserved
	Week      uint16
	TOW       float64 // seconds
	ECEFX     float64 // meters
	ECEFY     float64 // meters
	ECEFZ     float64 // meters
	PAcc      float32 // m², variance of 3D position
	ECEFVX    float32 // m/s
	ECEFVY    float32 // m/s
	ECEFVZ    float32 // m/s
	SAcc      float32 // (m/s)², variance of 3D velocity
	PDOP      float32
}

func (m *NavSol) ID() MsgID { return NavSolID }

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

type NavTimeSrc uint8

const (
	NavTimeSrcGPS NavTimeSrc = iota
	NavTimeSrcBDS
	NavTimeSrcGLN
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
	TimeSrc   NavTimeSrc
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
	FreqBias float32         // s², clock drift
	TAcc     float32         // 1/c², time accuracy variance
	FAcc     float32         // 1/c², frequency accuracy variance
	Systems  [3]NavClockSys  // GPS=0, BDS=1, GLN=2
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

// NavSVInfo is common satellite info structure used by NAV-GPSINFO, NAV-BDSINFO, NAV-GLNINFO
type NavSVInfo struct {
	Chn     uint8   // channel number
	SVID    uint8   // satellite ID
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
	NavSVQualityPRValid    NavSVQuality = 1 << iota // pseudorange valid
	NavSVQualityCPValid                             // carrier phase valid
	NavSVQualityHalfCycleValid                      // half-cycle ambiguity valid
	NavSVQualityHalfCycleSubtracted                 // half-cycle subtracted
	_                                               // reserved
	NavSVQualityFreqValid                           // carrier frequency valid
)

// NavGPSInfo is NAV-GPSINFO (0x01 0x20) - GPS satellite information
type NavGPSInfo struct {
	NavGPSInfoFixed
	SVs []NavSVInfo
}

type NavGPSInfoFixed struct {
	NavRunTime
	NumViewSV uint8       // number of visible satellites
	NumFixSV  uint8       // number of satellites used for fix
	System    NavInfoSys  // system type
	_         uint8       // reserved
}

type NavInfoSys uint8

const (
	NavInfoSysGPS NavInfoSys = iota
	NavInfoSysBDS
	NavInfoSysGLN
)

func (m *NavGPSInfo) ID() MsgID { return NavGPSInfoID }

func (m *NavGPSInfo) InitVaryingPart(payloadLen int) (err error) {
	n, err := sliceLen(m, payloadLen, 8, 12)
	if err == nil {
		m.SVs = make([]NavSVInfo, n)
	}
	return
}

func (m *NavGPSInfo) FixedPart() any  { return &m.NavGPSInfoFixed }
func (m *NavGPSInfo) VaryingPart() any { return &m.SVs }

var _ VaryingMsg = (*NavGPSInfo)(nil)

// NavBDSInfo is NAV-BDSINFO (0x01 0x21) - BDS satellite information
type NavBDSInfo struct {
	NavBDSInfoFixed
	SVs []NavSVInfo
}

type NavBDSInfoFixed struct {
	NavRunTime
	NumViewSV uint8
	NumFixSV  uint8
	System    NavInfoSys
	_         uint8
}

func (m *NavBDSInfo) ID() MsgID { return NavBDSInfoID }

func (m *NavBDSInfo) InitVaryingPart(payloadLen int) (err error) {
	n, err := sliceLen(m, payloadLen, 8, 12)
	if err == nil {
		m.SVs = make([]NavSVInfo, n)
	}
	return
}

func (m *NavBDSInfo) FixedPart() any  { return &m.NavBDSInfoFixed }
func (m *NavBDSInfo) VaryingPart() any { return &m.SVs }

var _ VaryingMsg = (*NavBDSInfo)(nil)

// NavGLNInfo is NAV-GLNINFO (0x01 0x22) - GLONASS satellite information
type NavGLNInfo struct {
	NavGLNInfoFixed
	SVs []NavSVInfo
}

type NavGLNInfoFixed struct {
	NavRunTime
	NumViewSV uint8
	NumFixSV  uint8
	System    NavInfoSys
	_         uint8
}

func (m *NavGLNInfo) ID() MsgID { return NavGLNInfoID }

func (m *NavGLNInfo) InitVaryingPart(payloadLen int) (err error) {
	n, err := sliceLen(m, payloadLen, 8, 12)
	if err == nil {
		m.SVs = make([]NavSVInfo, n)
	}
	return
}

func (m *NavGLNInfo) FixedPart() any  { return &m.NavGLNInfoFixed }
func (m *NavGLNInfo) VaryingPart() any { return &m.SVs }

var _ VaryingMsg = (*NavGLNInfo)(nil)

func init() {
	regMsg[NavSol]("SOL")
	regMsg[NavTimeUTC]("TIMEUTC")
	regMsg[NavClock]("CLOCK")
	regMsg[NavGPSInfo]("GPSINFO")
	regMsg[NavBDSInfo]("BDSINFO")
	regMsg[NavGLNInfo]("GLNINFO")
}
