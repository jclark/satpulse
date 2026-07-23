package asbin

const (
	NavPosEcefID MsgID = clsNav | (0x01 << 8)
	NavPosLlhID  MsgID = clsNav | (0x02 << 8)
	NavDopID     MsgID = clsNav | (0x04 << 8)
	NavTimeID    MsgID = clsNav | (0x05 << 8)
	NavVelEcefID MsgID = clsNav | (0x11 << 8)
	NavVelNedID  MsgID = clsNav | (0x12 << 8)
	NavTimeUtcID MsgID = clsNav | (0x21 << 8)
	NavClockID   MsgID = clsNav | (0x22 << 8)
	NavClock2ID  MsgID = clsNav | (0x23 << 8)
	NavSvInfoID  MsgID = clsNav | (0x30 << 8)
	NavSvinID    MsgID = clsNav | (0x31 << 8)
	NavAutoID    MsgID = clsNav | (0xC0 << 8)
)

// NavMsg is implemented by NAV messages that have an iTOW field
type NavMsg interface {
	Msg
	NavEpoch() uint32
}

// NavITOW is embedded in NAV messages to provide epoch tracking via iTOW
type NavITOW struct {
	ITow uint32 `json:"iTow"` // ms, GNSS time of week
}

// NavEpoch returns the iTOW value as the epoch identifier
func (m *NavITOW) NavEpoch() uint32 {
	return m.ITow
}

// NAV-POSECEF (0x01 0x01)
type NavPosEcef struct {
	NavITOW
	EcefX int32  `json:"ecefX"` // cm, ECEF X coordinate
	EcefY int32  `json:"ecefY"` // cm, ECEF Y coordinate
	EcefZ int32  `json:"ecefZ"` // cm, ECEF Z coordinate
	PAcc  uint32 `json:"pAcc"`  // cm, Position accuracy estimate
}

func (m *NavPosEcef) ID() MsgID { return NavPosEcefID }

// NAV-POSLLH (0x01 0x02)
type NavPosLlh struct {
	NavITOW
	Lon    int32  `json:"lon"`    // 1e-7 deg, Longitude
	Lat    int32  `json:"lat"`    // 1e-7 deg, Latitude
	Height int32  `json:"height"` // mm, Height above ellipsoid
	HMSL   int32  `json:"hMSL"`   // mm, Height above mean sea level
	HAcc   uint32 `json:"hAcc"`   // mm, Horizontal accuracy estimate
	VAcc   uint32 `json:"vAcc"`   // mm, Vertical accuracy estimate
}

func (m *NavPosLlh) ID() MsgID { return NavPosLlhID }

// NAV-DOP (0x01 0x04)
type NavDop struct {
	NavITOW
	GDOP uint16 `json:"gDOP"` // 0.01, Geometric DOP
	PDOP uint16 `json:"pDOP"` // 0.01, Position DOP
	TDOP uint16 `json:"tDOP"` // 0.01, Time DOP
	VDOP uint16 `json:"vDOP"` // 0.01, Vertical DOP
	HDOP uint16 `json:"hDOP"` // 0.01, Horizontal DOP
	NDOP uint16 `json:"nDOP"` // 0.01, Northing DOP
	EDOP uint16 `json:"eDOP"` // 0.01, Easting DOP
}

func (m *NavDop) ID() MsgID { return NavDopID }

// NavTimeSys defines the GNSS system values for NavTime.NavSys
type NavTimeSys byte

const (
	NavTimeSysGPS NavTimeSys = iota
	NavTimeSysBeiDou
	NavTimeSysGLONASS
	NavTimeSysGalileo
)

// NavTimeFlags defines the validity flag bits for NavTime.Flags
type NavTimeFlags byte

const (
	NavTimeFlagWeekValid    NavTimeFlags = 1 << iota // Bit 0: Week number valid
	NavTimeFlagSecondValid                           // Bit 1: Second of week valid
	NavTimeFlagLeapSecValid                          // Bit 2: Leap seconds valid
)

// NAV-TIME (0x01 0x05)
type NavTime struct {
	NavSys  NavTimeSys   `json:"navSys"`  // GNSS system: 0=GPS, 1=BD, 2=Glonass, 3=Galileo
	Flags   NavTimeFlags `json:"flag"`    // Validity flags: bit0=week, bit1=second, bit2=leapsecond
	Fractow int16        `json:"Fractow"` // ns, Fractional part of time of week
	RefTow  uint32       `json:"refTow"`  // ms, GNSS time of week
	Week    uint16       `json:"Week"`    // GNSS week number
	LeapSec int16        `json:"leapSec"` // s, Leap seconds to UTC
	TimeErr uint32       `json:"timeErr"` // ns, Estimated time error
}

func (m *NavTime) ID() MsgID        { return NavTimeID }
func (m *NavTime) NavEpoch() uint32 { return m.RefTow }

// NAV-VELECEF (0x01 0x11)
type NavVelEcef struct {
	NavITOW
	EcefVX int32  `json:"ecefVX"` // cm/s, ECEF X velocity
	EcefVY int32  `json:"ecefVY"` // cm/s, ECEF Y velocity
	EcefVZ int32  `json:"ecefVZ"` // cm/s, ECEF Z velocity
	SAcc   uint32 `json:"sAcc"`   // cm/s, Speed accuracy estimate
}

func (m *NavVelEcef) ID() MsgID { return NavVelEcefID }

// NAV-VELNED (0x01 0x12)
type NavVelNed struct {
	NavITOW
	VelN    int32  `json:"velN"`    // cm/s, North velocity
	VelE    int32  `json:"velE"`    // cm/s, East velocity
	VelD    int32  `json:"velD"`    // cm/s, Down velocity
	Speed   uint32 `json:"speed"`   // cm/s, Speed (3D)
	GSpeed  uint32 `json:"gSpeed"`  // cm/s, Ground speed (2D)
	Heading int32  `json:"heading"` // 1e-5 deg, Heading of motion (2D)
	SAcc    uint32 `json:"sAcc"`    // cm/s, Speed accuracy estimate
	CAcc    uint32 `json:"cAcc"`    // 1e-5 deg, Course/heading accuracy estimate
}

func (m *NavVelNed) ID() MsgID { return NavVelNedID }

// NAV-CLOCK (0x01 0x22)
type NavClock struct {
	NavITOW
	ClkB int32  `json:"clkB"` // ns, Clock bias
	ClkD int32  `json:"clkD"` // ns/s, Clock drift
	TAcc uint32 `json:"tAcc"` // ns, Time accuracy estimate
	FAcc uint32 `json:"fAcc"` // ps/s, Frequency accuracy estimate
}

func (m *NavClock) ID() MsgID { return NavClockID }

// NAV-SVIN (0x01 0x31) - Survey-in status
// This is not in 2.3.6 protocol spec, but is in satrack 1.31 app
type NavSvin struct {
	NavITOW
	PosUsed    uint32 `json:"posUsed"`    // Number of position samples used
	MeanStdDev uint32 `json:"meanStdDev"` // 0.1mm, Mean standard deviation
	Valid      uint8  `json:"valid"`      // 0=not valid, 1=valid
	Status     uint8  `json:"status"`     // 0=not finish, 1=finish (survey complete)
}

func (m *NavSvin) ID() MsgID { return NavSvinID }

// UTCStandard identifies the UTC standard used for time.
type UTCStandard uint8

const (
	UTCStandardNotAvailable UTCStandard = 0
	UTCStandardNTSC         UTCStandard = 1 // National Time Service Center, China (BeiDou)
	UTCStandardUSNO         UTCStandard = 2 // U.S. Naval Observatory (GPS)
	UTCStandardEU           UTCStandard = 4 // European Laboratory (Galileo)
	UTCStandardSU           UTCStandard = 5 // Former Soviet Union (GLONASS)
	UTCStandardIndia        UTCStandard = 6
)

// NavTimeUTCFlags defines validity flag bits for NavTimeUTC.ValidFlag
type NavTimeUTCFlags uint8

const (
	NavTimeUTCFlagTowValid NavTimeUTCFlags = 1 << iota // Bit 0: Valid time of week
	NavTimeUTCFlagWknValid                             // Bit 1: Valid week number
	NavTimeUTCFlagUtcValid                             // Bit 2: Valid UTC time
)

func (f NavTimeUTCFlags) UTCStandard() UTCStandard { return UTCStandard(f >> 4) }

// NAV-TIMEUTC (0x01 0x21)
type NavTimeUTC struct {
	NavITOW
	TAcc      uint32          `json:"tAcc"`      // ns, Time accuracy estimate
	Nano      int32           `json:"nano"`      // ns, Nanoseconds of second (-500M to 500M)
	Year      uint16          `json:"year"`      // Year (1999-2099)
	Month     uint8           `json:"month"`     // Month (1-12)
	Day       uint8           `json:"day"`       // Day (1-31)
	Hour      uint8           `json:"hour"`      // Hour (0-23)
	Min       uint8           `json:"min"`       // Minute (0-59)
	Sec       uint8           `json:"sec"`       // Second (0-59)
	ValidFlag NavTimeUTCFlags `json:"ValidFlag"` // Validity flags and UTC standard
}

func (m *NavTimeUTC) ID() MsgID { return NavTimeUtcID }

// NavSVInfoFlags defines the flag bits for NavSVInfoSat.Flags
type NavSVInfoFlags int8

const (
	NavSVInfoFlagUnhealthy NavSVInfoFlags = 0x01 // Satellite is unhealthy
	NavSVInfoFlagAlmanac   NavSVInfoFlags = 0x02 // Has almanac data
	NavSVInfoFlagEphemeris NavSVInfoFlags = 0x04 // Has ephemeris data
	NavSVInfoFlagUsed      NavSVInfoFlags = 0x08 // Used in solution
)

// NavSVInfoQuality defines quality indicator values for NavSVInfoSat.Quality
type NavSVInfoQuality int8

const (
	NavSVInfoQualityNoSignal          NavSVInfoQuality = 0
	NavSVInfoQualitySearching         NavSVInfoQuality = 3
	NavSVInfoQualityAcquired          NavSVInfoQuality = 4
	NavSVInfoQualityCodeLocked        NavSVInfoQuality = 5
	NavSVInfoQualityCodeCarrierLocked NavSVInfoQuality = 6
)

// NavSVInfoSat contains per-satellite data in NAV-SVINFO
type NavSVInfoSat struct {
	Svid            uint16           `json:"svid"`            // Satellite ID
	Flags           NavSVInfoFlags   `json:"flags"`           // Bitmask
	Quality         NavSVInfoQuality `json:"quality"`         // Quality indicator
	Cno             uint8            `json:"cno"`             // dBHz, Carrier to noise ratio
	Elev            int8             `json:"elev"`            // degrees, Elevation
	Azim            int16            `json:"azim"`            // degrees, Azimuth
	PrRes           int32            `json:"prRes"`           // cm, Pseudo range residual
	PseudorangeRate float32          `json:"pseudorangeRate"` // m/s, Pseudo range rate
	Pseudorange     float64          `json:"pseudorange"`     // m, Pseudo range
}

// NavSVInfoFixed contains the fixed header of NAV-SVINFO
type NavSVInfoFixed struct {
	NavITOW
	NumCh uint32 `json:"numCh"` // Number of channels
}

// NAV-SVINFO (0x01 0x30) - Variable length
type NavSVInfo struct {
	NavSVInfoFixed
	Sats []NavSVInfoSat `json:"sats"`
}

func (m *NavSVInfo) ID() MsgID { return NavSvInfoID }

func (m *NavSVInfo) InitForLen(payloadLen int) error {
	n, err := sliceLen(m, payloadLen, 8, 24)
	if err != nil {
		return err
	}
	m.Sats = make([]NavSVInfoSat, n)
	return nil
}

func (m *NavSVInfo) Parts() (fixed any, slice any) {
	return &m.NavSVInfoFixed, m.Sats
}

// NavAutoFixState defines the fix state values for NavAuto.FixState
type NavAutoFixState = uint8

const (
	NavAutoFixNone     NavAutoFixState = 0
	NavAutoFixAided    NavAutoFixState = 1
	NavAutoFixClkBias  NavAutoFixState = 2
	NavAutoFix2D       NavAutoFixState = 3
	NavAutoFix3D       NavAutoFixState = 4
	NavAutoFixDGNSS    NavAutoFixState = 5
	NavAutoFixRTKFloat NavAutoFixState = 6
	NavAutoFixRTKFixed NavAutoFixState = 7
)

// NAV-AUTO (0x01 0xC0) - fix state, position, speed, DOPs, satellite counts
// Unlike other NAV messages, NAV-AUTO has no iTOW field.
type NavAuto struct {
	FixState  NavAutoFixState `json:"fixstate"`  // Fix state
	Year      uint16          `json:"year"`      // UTC year
	Month     uint8           `json:"month"`     // UTC month (1-12)
	Day       uint8           `json:"day"`       // UTC day (1-31)
	Hour      uint8           `json:"hour"`      // UTC hour (0-23)
	Min       uint8           `json:"min"`       // UTC minute (0-59)
	Sec       uint8           `json:"sec"`       // UTC second (0-59)
	Lon       int32           `json:"lon"`       // 1e-7 deg, Longitude
	Lat       int32           `json:"lat"`       // 1e-7 deg, Latitude
	Alt       int32           `json:"alt"`       // mm, Altitude above ellipsoid
	Speed     uint16          `json:"speed"`     // cm/s, Speed
	Heading   int16           `json:"heading"`   // 1e-2 deg, Heading
	PDOP      uint16          `json:"pDOP"`      // 0.01, Position DOP
	HDOP      uint16          `json:"hDOP"`      // 0.01, Horizontal DOP
	VDOP      uint16          `json:"vDOP"`      // 0.01, Vertical DOP
	SatInUse  uint8           `json:"satInUse"`  // Number of satellites used in solution
	SatInView uint8           `json:"satInView"` // Number of satellites in view
}

func (m *NavAuto) ID() MsgID { return NavAutoID }

func init() {
	regMsg[NavPosEcef]("POSECEF")
	regMsg[NavPosLlh]("POSLLH")
	regMsg[NavDop]("DOP")
	regMsg[NavTime]("TIME")
	regMsg[NavVelEcef]("VELECEF")
	regMsg[NavVelNed]("VELNED")
	regMsg[NavTimeUTC]("TIMEUTC")
	regMsg[NavClock]("CLOCK")
	regMsg[NavSVInfo]("SVINFO")
	regMsg[NavSvin]("SVIN")
	regMsg[NavAuto]("AUTO")
}
