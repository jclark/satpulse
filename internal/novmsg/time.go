package novmsg

import "fmt"

// Message IDs for time-related messages
const (
	TimeID MsgID = 101
)

// ClockStatus represents the receiver clock model status
type ClockStatus uint32

const (
	ClockStatusValid      ClockStatus = iota // The clock model is valid
	ClockStatusConverging                    // The clock model is near validity
	ClockStatusIterating                     // The clock model is iterating towards validity
	ClockStatusInvalid                       // The clock model is not valid
)

// MarshalText implements encoding.TextMarshaler
func (c ClockStatus) MarshalText() ([]byte, error) {
	switch c {
	case ClockStatusValid:
		return []byte("VALID"), nil
	case ClockStatusConverging:
		return []byte("CONVERGING"), nil
	case ClockStatusIterating:
		return []byte("ITERATING"), nil
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
	case "CONVERGING":
		*c = ClockStatusConverging
	case "ITERATING":
		*c = ClockStatusIterating
	case "INVALID":
		*c = ClockStatusInvalid
	default:
		return fmt.Errorf("unknown ClockStatus: %s", string(text))
	}
	return nil
}

// UTCStatus represents the UTC time validity status
type UTCStatus uint32

const (
	UTCStatusInvalid UTCStatus = 0
	UTCStatusValid   UTCStatus = 1
	UTCStatusWarning UTCStatus = 2
)

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

// Time represents receiver time information from a NovAtel receiver.
// Message ID: 101
// TIMEA/B messages contain receiver clock and UTC time information.
type Time struct {
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

func (r *Time) ID() (MsgID, string) {
	return TimeID, "TIMEA"
}

func init() {
	// Register known message types
	regMsg[Time]("TIMEA")
}
