package asbin

const (
	NavTimeID    MsgID = clsNav | (0x05 << 8)
	NavTimeUtcID MsgID = clsNav | (0x21 << 8)
	NavClockID   MsgID = clsNav | (0x22 << 8)
	NavClock2ID  MsgID = clsNav | (0x23 << 8)
	NavSvInfoID  MsgID = clsNav | (0x30 << 8)
	NavSvinID    MsgID = clsNav | (0x31 << 8)
)

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
	NavSys  NavTimeSys   // GNSS system: 0=GPS, 1=BD, 2=Glonass, 3=Galileo
	Flags   NavTimeFlags // Validity flags: bit0=week, bit1=second, bit2=leapsecond
	Fractow int16        // ns, Fractional part of time of week
	RefTow  uint32       // ms, GNSS time of week
	Week    uint16       // GNSS week number
	LeapSec int16        // s, Leap seconds to UTC
	TimeErr uint32       // ns, Estimated time error
}

func (m *NavTime) ID() MsgID { return NavTimeID }

// NAV-CLOCK (0x01 0x22)
type NavClock struct {
	ITow uint32 // ms, GNSS time of week
	ClkB int32  // ns, Clock bias
	ClkD int32  // ns/s, Clock drift
	TAcc uint32 // ns, Time accuracy estimate
	FAcc uint32 // ps/s, Frequency accuracy estimate
}

func (m *NavClock) ID() MsgID { return NavClockID }

// NAV-SVIN (0x01 0x31) - Survey-in status
// This is not in 2.3.6 protocol spec, but is in satrack 1.31 app
type NavSvin struct {
	ITow       uint32 // ms, GNSS time of week
	PosUsed    uint32 // Number of position samples used
	MeanStdDev uint32 // 0.1mm, Mean standard deviation
	Valid      uint8  // 0=not valid, 1=valid
	Status     uint8  // 0=not finish, 1=finish (survey complete)
}

func (m *NavSvin) ID() MsgID { return NavSvinID }

// NavTimeUTCFlags defines validity flag bits for NavTimeUTC.ValidFlag
type NavTimeUTCFlags uint8

const (
	NavTimeUTCFlagTowValid NavTimeUTCFlags = 1 << iota // Bit 0: Valid time of week
	NavTimeUTCFlagWknValid                             // Bit 1: Valid week number
	NavTimeUTCFlagUtcValid                             // Bit 2: Valid UTC time
)

// NavTimeUTCStandard extracts UTC standard from ValidFlag (bits 4-7)
func (f NavTimeUTCFlags) UTCStandard() uint8 { return uint8(f >> 4) }

// NAV-TIMEUTC (0x01 0x21)
type NavTimeUTC struct {
	ITow      uint32          // ms, GNSS time of week
	TAcc      uint32          // ns, Time accuracy estimate
	Nano      int32           // ns, Nanoseconds of second (-500M to 500M)
	Year      uint16          // Year (1999-2099)
	Month     uint8           // Month (1-12)
	Day       uint8           // Day (1-31)
	Hour      uint8           // Hour (0-23)
	Min       uint8           // Minute (0-59)
	Sec       uint8           // Second (0-59)
	ValidFlag NavTimeUTCFlags // Validity flags and UTC standard
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
	NavSVInfoQualityNoSignal         NavSVInfoQuality = 0
	NavSVInfoQualitySearching        NavSVInfoQuality = 3
	NavSVInfoQualityAcquired         NavSVInfoQuality = 4
	NavSVInfoQualityCodeLocked       NavSVInfoQuality = 5
	NavSVInfoQualityCodeCarrierLocked NavSVInfoQuality = 6
)

// NavSVInfoSat contains per-satellite data in NAV-SVINFO
type NavSVInfoSat struct {
	Svid            uint16           // Satellite ID
	Flags           NavSVInfoFlags   // Bitmask
	Quality         NavSVInfoQuality // Quality indicator
	Cno             uint8   // dBHz, Carrier to noise ratio
	Elev            int8    // degrees, Elevation
	Azim            int16   // degrees, Azimuth
	PrRes           int32   // cm, Pseudo range residual
	PseudorangeRate float32 // m/s, Pseudo range rate
	Pseudorange     float64 // m, Pseudo range
}

// NavSVInfoFixed contains the fixed header of NAV-SVINFO
type NavSVInfoFixed struct {
	ITow  uint32 // ms, GNSS time of week
	NumCh uint32 // Number of channels
}

// NAV-SVINFO (0x01 0x30) - Variable length
type NavSVInfo struct {
	NavSVInfoFixed
	Sats []NavSVInfoSat
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

func init() {
	regMsg[NavTime]("TIME")
	regMsg[NavTimeUTC]("TIMEUTC")
	regMsg[NavClock]("CLOCK")
	regMsg[NavSVInfo]("SVINFO")
	regMsg[NavSvin]("SVIN")
}
