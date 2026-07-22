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
	RunTime uint32 `json:"runTime"` // ms since boot/reset
}

func (m *NavRunTime) NavEpoch() uint32 {
	return m.RunTime
}

// NavSol is NAV-SOL (0x01 0x02) - reduced PVT navigation information (72 bytes)
type NavSol struct {
	NavRunTime
	PosValid NavPosValid `json:"posValid"`
	VelValid NavVelValid `json:"velValid"`
	TimeSrc  GNSSID      `json:"timeSrc"`
	System   NavSystem   `json:"system"`
	NumSV    uint8       `json:"numSV"`
	NumSVGPS uint8       `json:"numSVGPS"`
	NumSVBDS uint8       `json:"numSVBDS"`
	NumSVGLN uint8       `json:"numSVGLN"`
	_        uint16      // reserved
	Week     uint16      `json:"week"`
	TOW      float64     `json:"tow"`    // seconds
	ECEFX    float64     `json:"ecefX"`  // meters
	ECEFY    float64     `json:"ecefY"`  // meters
	ECEFZ    float64     `json:"ecefZ"`  // meters
	PAcc     float32     `json:"pAcc"`   // m², variance of 3D position
	ECEFVX   float32     `json:"ecefVX"` // m/s
	ECEFVY   float32     `json:"ecefVY"` // m/s
	ECEFVZ   float32     `json:"ecefVZ"` // m/s
	SAcc     float32     `json:"sAcc"`   // (m/s)², variance of 3D velocity
	PDOP     float32     `json:"pDop"`
}

func (m *NavSol) ID() MsgID { return NavSolID }

// NavPv is NAV-PV (0x01 0x03) - geodetic position and velocity (80 bytes)
type NavPv struct {
	NavRunTime
	PosValid NavPosValid `json:"posValid"`
	VelValid NavVelValid `json:"velValid"`
	System   NavSystem   `json:"system"`
	NumSV    uint8       `json:"numSV"`
	NumSVGPS uint8       `json:"numSVGPS"`
	NumSVBDS uint8       `json:"numSVBDS"`
	NumSVGLN uint8       `json:"numSVGLN"`
	_        uint8       // reserved
	PDOP     float32     `json:"pDop"`
	Lon      float64     `json:"lon"`      // deg
	Lat      float64     `json:"lat"`      // deg
	Height   float32     `json:"height"`   // m, ellipsoidal
	SepGeoid float32     `json:"sepGeoid"` // m, geoid separation (ellipsoidal minus MSL)
	HAcc     float32     `json:"hAcc"`     // m^2, variance of horizontal position accuracy
	VAcc     float32     `json:"vAcc"`     // m^2, variance of vertical position accuracy
	VelN     float32     `json:"velN"`     // m/s
	VelE     float32     `json:"velE"`     // m/s
	VelU     float32     `json:"velU"`     // m/s (UP, not down -- negate for NED)
	Speed3D  float32     `json:"speed3D"`  // m/s
	Speed2D  float32     `json:"speed2D"`  // m/s, ground speed
	Heading  float32     `json:"heading"`  // deg
	SAcc     float32     `json:"sAcc"`     // (m/s)^2, variance of ground speed accuracy
	CAcc     float32     `json:"cAcc"`     // deg^2, variance of heading accuracy
}

func (m *NavPv) ID() MsgID { return NavPvID }

// NavDop is NAV-DOP (0x01 0x01) - dilution of precision (28 bytes)
type NavDop struct {
	NavRunTime
	PDOP float32 `json:"pDop"`
	HDOP float32 `json:"hDop"`
	VDOP float32 `json:"vDop"`
	NDOP float32 `json:"nDop"`
	EDOP float32 `json:"eDop"`
	TDOP float32 `json:"tDop"`
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
	TAcc      float32         `json:"tAcc"`  // s², time estimation accuracy
	MsErr     float32         `json:"msErr"` // ms, residual error after rounding
	_         uint16          // padding (observed in real packets)
	Year      uint16          `json:"year"`  // 1999-2099
	Month     uint8           `json:"month"` // 1-12
	Day       uint8           `json:"day"`   // 1-31
	Hour      uint8           `json:"hour"`  // 0-23
	Min       uint8           `json:"min"`   // 0-59
	Sec       uint8           `json:"sec"`   // 0-59
	Valid     NavTimeUTCValid `json:"valid"`
	TimeSrc   GNSSID          `json:"timeSrc"`
	DateValid NavDateValid    `json:"dateValid"`
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
	FreqBias float32        `json:"freqBias"` // s², clock drift
	TAcc     float32        `json:"tAcc"`     // 1/c², time accuracy variance
	FAcc     float32        `json:"fAcc"`     // 1/c², frequency accuracy variance
	Systems  [3]NavClockSys `json:"systems"`  // GPS=0, BDS=1, GLN=2
}

func (m *NavClock) ID() MsgID { return NavClockID }

// NavClockSys contains per-system clock information
type NavClockSys struct {
	TOW   float64 `json:"tow"`   // ms, time of week
	DtUTC float32 `json:"dtUtc"` // s, fractional seconds diff between sat time and UTC
	Wn    uint16  `json:"wn"`    // week number
	Leaps int8    `json:"leaps"` // UTC leap seconds
	Valid uint8   `json:"valid"` // time validity flag
}

// NavSatInfoFixed is the common fixed part for NAV-GPSINFO, NAV-BDSINFO, NAV-GLNINFO
type NavSatInfoFixed struct {
	NavRunTime
	NumViewSV uint8  `json:"numViewSv"` // number of visible satellites
	NumFixSV  uint8  `json:"numFixSv"`  // number of satellites used for fix
	System    GNSSID `json:"system"`    // system type
	_         uint8  // reserved
}

// NavSVInfo is common satellite info structure used by NAV-GPSINFO, NAV-BDSINFO, NAV-GLNINFO
type NavSVInfo struct {
	Chn     uint8        `json:"chn"`  // channel number
	SVID    uint8        `json:"svid"` // satellite ID
	Flags   NavSVFlags   `json:"flags"`
	Quality NavSVQuality `json:"quality"`
	CNO     uint8        `json:"cno"`   // dB-Hz
	Elev    int8         `json:"elev"`  // degrees, -90 to 90
	Azim    int16        `json:"azim"`  // degrees, 0 to 360
	PRRes   float32      `json:"prRes"` // meters, pseudorange residual
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
	NavSatInfoFixed
	SVs []NavSVInfo `json:"SVs"`
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
	SVs []NavSVInfo `json:"SVs"`
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
	SVs []NavSVInfo `json:"SVs"`
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
	TOW int32 `json:"tow"` // GPS time of week in ms
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
	Wn         uint16       `json:"wn"`
	_          uint16       // reserved
	FixFlags   PVTValid     `json:"fixFlags"`
	VelFlags   Nav2VelFlags `json:"velFlags"`
	_          uint8        // reserved
	GnssMask   Nav2GnssMask `json:"fixGnssMask"`
	NumFixTot  uint8        `json:"numFixTot"`
	NumFixGPS  uint8        `json:"numFixGps"`
	NumFixBDS  uint8        `json:"numFixBds"`
	NumFixGLN  uint8        `json:"numFixGln"`
	NumFixGAL  uint8        `json:"numFixGal"`
	NumFixQZSS uint8        `json:"numFixQzs"`
	NumFixSBAS uint8        `json:"numFixSbs"`
	NumFixIRN  uint8        `json:"numFixIrn"`
	_          uint32       // reserved
	X          float64      `json:"x"`    // m, ECEF X
	Y          float64      `json:"y"`    // m, ECEF Y
	Z          float64      `json:"z"`    // m, ECEF Z
	PAcc       float32      `json:"pAcc"` // m, 3D position accuracy (std dev)
	VX         float32      `json:"vx"`   // m/s, ECEF X velocity
	VY         float32      `json:"vy"`   // m/s, ECEF Y velocity
	VZ         float32      `json:"vz"`   // m/s, ECEF Z velocity
	SAcc       float32      `json:"sAcc"` // m/s, 3D speed accuracy (std dev)
	PDOP       float32      `json:"pDop"`
}

func (m *Nav2Sol) ID() MsgID { return Nav2SolID }

// Nav2Pvh is NAV2-PVH (0x11 0x03) - geodetic position and velocity (88 bytes)
type Nav2Pvh struct {
	Nav2TOW
	Wn         uint16       `json:"wn"`
	_          uint16       // reserved
	FixFlags   PVTValid     `json:"fixFlags"`
	VelFlags   Nav2VelFlags `json:"velFlags"`
	_          uint8        // reserved
	GnssMask   Nav2GnssMask `json:"fixGnssMask"`
	NumFixTot  uint8        `json:"numFixTot"`
	NumFixGPS  uint8        `json:"numFixGps"`
	NumFixBDS  uint8        `json:"numFixBds"`
	NumFixGLN  uint8        `json:"numFixGln"`
	NumFixGAL  uint8        `json:"numFixGal"`
	NumFixQZSS uint8        `json:"numFixQzs"`
	NumFixSBAS uint8        `json:"numFixSbs"`
	NumFixIRN  uint8        `json:"numFixIrn"`
	_          uint32       // reserved
	Lon        float64      `json:"lon"`      // deg
	Lat        float64      `json:"lat"`      // deg
	Height     float32      `json:"height"`   // m, ellipsoidal
	SepGeoid   float32      `json:"sepGeoid"` // m, geoid separation
	VelE       float32      `json:"velE"`     // m/s, East velocity
	VelN       float32      `json:"velN"`     // m/s, North velocity
	VelU       float32      `json:"velU"`     // m/s, Up velocity (negate for NED down)
	Speed3D    float32      `json:"speed3D"`  // m/s
	Speed2D    float32      `json:"speed2D"`  // m/s, ground speed
	Heading    float32      `json:"heading"`  // deg
	HAcc       float32      `json:"hAcc"`     // m, horizontal position accuracy (std dev)
	VAcc       float32      `json:"vAcc"`     // m, vertical position accuracy (std dev)
	SAcc       float32      `json:"sAcc"`     // m/s, 3D speed accuracy (std dev)
	CAcc       float32      `json:"cAcc"`     // deg, heading accuracy (std dev)
}

func (m *Nav2Pvh) ID() MsgID { return Nav2PvhID }

// Nav2Dop is NAV2-DOP (0x11 0x01) - dilution of precision (24 bytes)
type Nav2Dop struct {
	PDOP float32 `json:"pDop"`
	HDOP float32 `json:"hDop"`
	VDOP float32 `json:"vDop"`
	NDOP float32 `json:"nDop"`
	EDOP float32 `json:"eDop"`
	TDOP float32 `json:"tDop"`
}

func (m *Nav2Dop) ID() MsgID { return Nav2DopID }

// Nav2TimeUTC is NAV2-TIMEUTC (0x11 0x05) - UTC time information (20 bytes)
type Nav2TimeUTC struct {
	TAcc    float32       `json:"tAcc"`  // ns, time accuracy estimate
	Subms   int32         `json:"subms"` // ms, fractional ms (scale 2^-30)
	Subcs   int8          `json:"subcs"` // ms, centisecond error (-5 to 5 ms)
	Cs      uint8         `json:"cs"`    // centiseconds (0-99)
	Year    uint16        `json:"year"`
	Month   uint8         `json:"month"`
	Day     uint8         `json:"day"`
	Hour    uint8         `json:"hour"`
	Min     uint8         `json:"minute"`
	Sec     uint8         `json:"second"`
	TFlags  Nav2TimeFlags `json:"tFlagx"`
	TimeSrc Nav2TimeSrc   `json:"tSrc"`
	LeapSec int8          `json:"leapSec"`
}

func (m *Nav2TimeUTC) ID() MsgID { return Nav2TimeUTCID }

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

// Nav2SigFixed is the fixed part of NAV2-SIG (8 bytes)
type Nav2SigFixed struct {
	TOW       uint32 `json:"tow"` // GPS TOW in ms
	_         uint8  // reserved
	NumTrkTot uint8  `json:"numTrkTot"`
	NumFixTot uint8  `json:"numFixTot"`
	_         uint8  // reserved
}

func (m *Nav2SigFixed) NavEpoch() uint32 { return m.TOW }

// Nav2SigInfo is a per-signal entry in NAV2-SIG (16 bytes each)
type Nav2SigInfo struct {
	GNSSID    GNSSID `json:"gnssid"`
	SVID      uint8  `json:"svid"`      // satellite ID (raw PRN, except QZSS=PRN-192)
	SigID     SigID  `json:"sigid"`     // signal band ID
	FreqID    uint8  `json:"freqid"`    // GLONASS frequency ID; undefined for other constellations
	PRRes     int16  `json:"prResi"`    // dm, pseudorange residual
	CNO       uint8  `json:"cn0"`       // dBHz
	TrkInd    uint8  `json:"trkind"`    // signal quality
	CorFlags  uint8  `json:"corFlagx"`  // correction flag
	SolFlags  uint8  `json:"solFlagx"`  // solution flag
	Chn       uint8  `json:"chn"`       // tracking channel number
	Elev      uint8  `json:"eleDeg"`    // deg
	Azim      uint16 `json:"aziDeg"`    // deg
	IonoDelay int16  `json:"ionoDelay"` // dm, ionosphere delay correction
}

// Nav2Sig is NAV2-SIG (0x11 0x06) - per-signal tracking information.
// The receiver appends undocumented trailing bytes after the signal entries;
// AllowTrailingBytes permits ParseMsg to accept them.
type Nav2Sig struct {
	Nav2SigFixed
	Sigs []Nav2SigInfo `json:"sigs"`
}

func (m *Nav2Sig) ID() MsgID { return Nav2SigID }

func (m *Nav2Sig) InitVaryingPart(payloadLen int) error {
	m.Sigs = make([]Nav2SigInfo, m.NumTrkTot)
	return nil
}

func (m *Nav2Sig) FixedPart() any   { return &m.Nav2SigFixed }
func (m *Nav2Sig) VaryingPart() any { return &m.Sigs }
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
