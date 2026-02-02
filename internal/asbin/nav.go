package asbin

const (
	NavTimeID   MsgID = clsNav | (0x05 << 8)
	NavClockID  MsgID = clsNav | (0x22 << 8)
	NavClock2ID MsgID = clsNav | (0x23 << 8)
	NavSvinID   MsgID = clsNav | (0x31 << 8)
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

func init() {
	regMsg[NavTime]("TIME")
	regMsg[NavClock]("CLOCK")
	regMsg[NavSvin]("SVIN")
}
