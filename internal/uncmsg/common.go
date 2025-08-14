package uncmsg

import (
	"fmt"
)

// MsgID represents a Unicore message identifier (uint16)
type MsgID uint16

// TimeStatus represents the GPS Reference Time Status field in Unicore messages
// Based on Novatel OEM7 time status values
type TimeStatus byte

const (
	TimeStatusUnknown TimeStatus = 20  // Time validity is unknown
	TimeStatusCoarse  TimeStatus = 100 // Time is valid to coarse precision
	TimeStatusFine    TimeStatus = 160 // Time has fine precision
)

// String returns the ASCII representation of TimeStatus
func (ts TimeStatus) String() string {
	switch ts {
	case TimeStatusUnknown:
		return "UNKNOWN"
	case TimeStatusCoarse:
		return "COARSE"
	case TimeStatusFine:
		return "FINE"
	default:
		return fmt.Sprintf("%d", ts)
	}
}

// TimeRef represents the time reference system in Unicore messages
type TimeRef byte

const (
	TimeRefGPS TimeRef = 0 // GPS time reference
	TimeRefBDS TimeRef = 1 // BDS time reference
)

// String returns the ASCII representation of TimeRef
func (tr TimeRef) String() string {
	switch tr {
	case TimeRefGPS:
		return "GPS"
	case TimeRefBDS:
		return "BDS"
	default:
		return fmt.Sprintf("%d", tr)
	}
}

// ParseTimeRef converts an ASCII time ref string to TimeRef enum
func ParseTimeRef(s string) (TimeRef, error) {
	switch s {
	case "GPS":
		return TimeRefGPS, nil
	case "BDS":
		return TimeRefBDS, nil
	default:
		return 0, fmt.Errorf("unknown time reference: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (tr *TimeRef) UnmarshalText(text []byte) error {
	val, err := ParseTimeRef(string(text))
	if err != nil {
		return err
	}
	*tr = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (tr TimeRef) MarshalText() ([]byte, error) {
	return []byte(tr.String()), nil
}

// ParseTimeStatus converts an ASCII time status string to TimeStatus enum
func ParseTimeStatus(s string) (TimeStatus, error) {
	switch s {
	case "UNKNOWN":
		return TimeStatusUnknown, nil
	case "COARSE":
		return TimeStatusCoarse, nil
	case "FINE":
		return TimeStatusFine, nil
	default:
		return 0, fmt.Errorf("unknown time status: %s", s)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (ts *TimeStatus) UnmarshalText(text []byte) error {
	val, err := ParseTimeStatus(string(text))
	if err != nil {
		return err
	}
	*ts = val
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (ts TimeStatus) MarshalText() ([]byte, error) {
	return []byte(ts.String()), nil
}

// TimingHeader contains timing and status information from Unicore message headers
type TimingHeader struct {
	TimeRef            TimeRef    // Reference time (GPS or BDS)
	TimeStatus         TimeStatus // GPS Reference Time Status
	Week               uint16     // Week number
	MillisecondsOfWeek uint32     // Seconds of week (milliseconds)
	Reserved           uint32     // Reserved
	Version            byte       // Release version
	LeapSec            byte       // Leap second
	DelayMs            uint16     // Output delay
}

// MessageHeader contains the useful header information from Unicore messages
// This can be populated from both binary and ASCII formats
type MessageHeader struct {
	CPUIdlePercent byte // CPU idle percentage (maps to IdleTime in ASCII)
	TimingHeader        // Embedded timing info
}

// Msg interface that all Unicore messages must implement
type Msg interface {
	ID() MsgID
}


// ChunkedMsg interface for messages with variable-length structure
// that need to be read/written in chunks
type ChunkedMsg interface {
	Msg
	// Chunks returns a push iterator that yields each chunk for sequential reading/writing
	Chunks() func(yield func(chunk any) bool)
}

var msgMap = make(map[MsgID]func() Msg)
var idNameMap = make(map[MsgID]string)
var asciiNameIDMap = make(map[string]MsgID)

// String returns a string representation of the message ID
func (mid MsgID) String() string {
	name := idNameMap[mid]
	if name != "" {
		return name
	}
	return fmt.Sprintf("%d", uint16(mid))
}

// regMsg registers a message type with its ID and name
func regMsg[T any, PT interface {
	ID() MsgID
	*T
}](idName string) {
	m := PT(new(T))
	mid := m.ID()
	msgMap[mid] = func() Msg { return PT(new(T)) }
	idNameMap[mid] = idName
	asciiNameIDMap[idName+"A"] = mid
}
