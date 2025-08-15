package uncmsg

import (
	"fmt"
)

// Message IDs for time-related messages
const (
	RecTimeID   MsgID = 102
	PPSStatusID MsgID = 9000
	GPSUTCID    MsgID = 19
	GALUTCID    MsgID = 20
	BD3UTCID    MsgID = 22
	BDSUTCID    MsgID = 2012
)

// ClockStatus represents the receiver clock model status
type ClockStatus uint32

const (
	ClockStatusValid   ClockStatus = 0
	ClockStatusInvalid ClockStatus = 3
)

// UTCStatus represents the UTC time validity status
type UTCStatus uint32

const (
	UTCStatusInvalid UTCStatus = 0
	UTCStatusValid   UTCStatus = 1
	UTCStatusWarning UTCStatus = 2
)

// RecTime represents receiver time information from a Unicore receiver.
// Message ID: 102
// RECTIMEA/B messages contain receiver clock and UTC time information.
type RecTime struct {
	ClockStatus ClockStatus // Clock model status (0=VALID, 3=INVALID)
	Offset      float64     // Receiver clock offset relative to GPS time (s)
	OffsetStd   float64     // Standard deviation of clock offset (s)
	UTCOffset   float64     // GPS time offset relative to UTC (s)
	UTCYear     uint32      // UTC year
	UTCMonth    uint8       // UTC month
	UTCDay      uint8       // UTC day
	UTCHour     uint8       // UTC hour
	UTCMin      uint8       // UTC minute
	UTCMs       uint32      // UTC millisecond
	UTCStatus   UTCStatus   // UTC status (0=INVALID, 1=VALID, 2=WARNING)
}

// ID returns the message ID for RECTIME
func (r *RecTime) ID() (MsgID, string) {
	return RecTimeID, "RECTIMEA"
}

// MarshalText implements encoding.TextMarshaler
func (c ClockStatus) MarshalText() ([]byte, error) {
	switch c {
	case ClockStatusValid:
		return []byte("VALID"), nil
	case ClockStatusInvalid:
		return []byte("INVALID"), nil
	default:
		return nil, fmt.Errorf("unknown ClockStatus: %d", c)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler
func (c *ClockStatus) UnmarshalText(text []byte) error {
	switch string(text) {
	case "VALID":
		*c = ClockStatusValid
	case "INVALID":
		*c = ClockStatusInvalid
	default:
		return fmt.Errorf("unknown ClockStatus: %s", string(text))
	}
	return nil
}

// MarshalText implements encoding.TextMarshaler
func (u UTCStatus) MarshalText() ([]byte, error) {
	switch u {
	case UTCStatusInvalid:
		return []byte("INVALID"), nil
	case UTCStatusValid:
		return []byte("VALID"), nil
	case UTCStatusWarning:
		return []byte("WARNING"), nil
	default:
		return nil, fmt.Errorf("unknown UTCStatus: %d", u)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler
func (u *UTCStatus) UnmarshalText(text []byte) error {
	switch string(text) {
	case "INVALID":
		*u = UTCStatusInvalid
	case "VALID":
		*u = UTCStatusValid
	case "WARNING":
		*u = UTCStatusWarning
	default:
		return fmt.Errorf("unknown UTCStatus: %s", string(text))
	}
	return nil
}

// PPSStatus represents the PPS status information from a Unicore receiver.
// Message ID: 9000
// It only supports 1 Hz output.
type PPSStatus struct {
	Status       uint32 // PPS output status. 0: No PPS output, 1: Accuracy not guaranteed, 2: Accuracy ~100-1000 ns, 3: Accuracy < 100 ns
	Week         uint32 // PPS week (aligned with GPS Week)
	MsSecInWeek  uint32 // PPS milliseconds in the week (aligned with GPS MsCount)
	PPSPulseErr  int32  // PPS phase error (ns)
	OffsetTime   int32  // PPS offset time relative to observation time (0.01 ns)
	ConfigInfo   uint32 // PPS configuration information (Hex)
	Register     uint32 // Hardware register status
	TimeEstErr   int32  // DeltaT time estimated error
	InnerQuality uint32 // Inner positioning and time quality indicator
	PPSInnerSta  uint32 // PPS inner status
	PPSCfgSta    uint32 // PPS config status
	PPSStage     uint32 // PPS stage
	Reserved1    uint32 // Reserved
	Reserved2    uint32 // Reserved
	Reserved3    uint32 // Reserved
}

// ID returns the message ID for PPSSTATUS
func (p *PPSStatus) ID() (MsgID, string) {
	return PPSStatusID, "PPSSTATUSA"
}

// GPSUTC represents GPS UTC parameters from a Unicore receiver.
// Message ID: 19
// Parameters for converting GPS Time to UTC.
type GPSUTC struct {
	UTCWn     uint32  // UTC reference week number
	Tot       uint32  // Reference time of UTC parameters
	A0        float64 // Clock bias of GPST relative to UTC
	A1        float64 // Clock rate of GPST relative to UTC
	WnLsf     uint32  // Future week number for leap second
	Dn        uint32  // Future day number for leap second
	DeltatLs  int32   // Current leap seconds
	DeltatLsf int32   // Future leap seconds
	DeltatUTC uint32  // Time offset of GPST relative to UTC
	Reserved  uint32  // Reserved
}

// ID returns the message ID for GPSUTC
func (g *GPSUTC) ID() (MsgID, string) {
	return GPSUTCID, "GPSUTCA"
}

// GALUTC represents Galileo UTC parameters from a Unicore receiver.
// Message ID: 20
// Parameters for converting Galileo Time to UTC.
type GALUTC struct {
	A0        float64 // Clock bias of Galileo time relative to UTC
	A1        float64 // Clock rate of Galileo time relative to UTC
	DeltatLs  int32   // Current leap seconds
	Tot       uint32  // Reference time of UTC parameters
	UTCWn     uint32  // UTC reference week number
	WnLsf     uint32  // Future week number for leap second
	Dn        uint32  // Future day number for leap second
	DeltatLsf int32   // Future leap seconds
	DA0g      float64 // Constant term for Galileo to GPS time conversion
	DA1g      float64 // First order term for Galileo to GPS time conversion
	UlT0g     uint32  // Reference second for Galileo to GPS time conversion
	UlWN0g    uint32  // Reference week for Galileo to GPS time conversion
}

// ID returns the message ID for GALUTC
func (g *GALUTC) ID() (MsgID, string) {
	return GALUTCID, "GALUTCA"
}

// BD3UTC represents BDS-3 UTC parameters from a Unicore receiver.
// Message ID: 22
// Parameters for converting BDS-3 Time to UTC.
type BD3UTC struct {
	UTCWn     uint32  // UTC reference week number
	Tot       uint32  // Reference time of UTC parameters
	A0        float64 // Clock bias of BDST relative to UTC
	A1        float64 // Clock drift of BDST relative to UTC
	A2        float64 // Clock drift rate of BDST relative to UTC
	WnLsf     uint32  // Future week number for leap second
	Dn        uint32  // Future day number for leap second
	DeltatLs  int32   // Current leap seconds
	DeltatLsf int32   // Future leap seconds
	Reserved1 uint32  // Reserved
	Reserved2 uint32  // Reserved
}

// ID returns the message ID for BD3UTC
func (b *BD3UTC) ID() (MsgID, string) {
	return BD3UTCID, "BD3UTCA"
}

// BDSUTC represents BDS UTC parameters from a Unicore receiver.
// Message ID: 2012
// Parameters for converting BDS Time to UTC.
type BDSUTC struct {
	Reserved1 uint32  // Reserved
	Reserved2 uint32  // Reserved
	A0        float64 // Clock bias of BDT relative to UTC
	A1        float64 // Clock rate of BDT relative to UTC
	WnLsf     uint32  // Future week number for leap second
	Dn        uint32  // Future day number for leap second
	DeltatLs  int32   // Current leap seconds
	DeltatLsf int32   // Future leap seconds
	Reserved3 uint32  // Reserved
	Reserved4 uint32  // Reserved
}

// ID returns the message ID for BDSUTC
func (b *BDSUTC) ID() (MsgID, string) {
	return BDSUTCID, "BDSUTCA"
}

func init() {
	// Register known message types
	regMsg[RecTime]("RECTIME")
	regMsg[PPSStatus]("PPSSTATUS")
	regMsg[GPSUTC]("GPSUTC")
	regMsg[GALUTC]("GALUTC")
	regMsg[BD3UTC]("BD3UTC")
	regMsg[BDSUTC]("BDSUTC")
}
