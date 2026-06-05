package uncmsg

import (
	"fmt"

	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// MsgID represents a Unicore message identifier (uint16)
type MsgID uint16

// Reuse novmsg.TimeStatus
const (
	TimeStatusUnknown = novmsg.TimeStatusUnknown
	TimeStatusCoarse  = novmsg.TimeStatusCoarse
	TimeStatusFine    = novmsg.TimeStatusFine
)

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

// TimingHdr contains timing and status information from Unicore message headers
type TimingHdr struct {
	TimeRef            TimeRef // Reference time (GPS or BDS)
	TimeStatus         novmsg.TimeStatus
	Week               uint16
	MillisecondsOfWeek uint32
	Version            uint32 // Release version
	Reserved           byte
	LeapSec            byte   // Leap second
	DelayMs            uint16 // Output delay
}

// MsgHdr contains the useful header information from Unicore messages
// This can be populated from both binary and ASCII formats
type MsgHdr struct {
	CPUIdlePercent byte // CPU idle percentage (maps to IdleTime in ASCII)
	TimingHdr           // Embedded timing info
}

// MsgBody interface that all Unicore message bodies must implement
type MsgBody interface {
	// ID returns the message identifiers:
	// - MsgID: numeric identifier (0 for unknown ASCII messages)
	// - string: ASCII wire format name (e.g., "VERSIONA", "PPSSTATUSA")
	//   For known messages, this includes the "A" suffix.
	//   For unknown messages, this is exactly what was received (may or may not have "A").
	ID() (MsgID, string)
}

// Msg combines a message header with its body
type Msg struct {
	Hdr  MsgHdr
	Body MsgBody
}

var msgIDMap = make(map[MsgID]func() MsgBody)
var msgNameMap = make(map[string]func() MsgBody)
var idNameMap = make(map[MsgID]string)
var nameIDMap = make(map[string]MsgID)

// String returns a string representation of the message ID
func (mid MsgID) String() string {
	name := idNameMap[mid]
	if name != "" {
		return name
	}
	return fmt.Sprintf("%d", uint16(mid))
}

// AsciiMsgID returns the binary MsgID for an ASCII wire name (e.g. "OBSVMA"),
// or 0 if the name is not a known message. It is the inverse of the ASCII name
// in MsgBody.ID, used to map an ASCII packet's wire name back to its canonical
// (suffix-less) name via String.
func AsciiMsgID(name string) MsgID {
	return nameIDMap[name]
}

// regMsg registers a message type with its ID and name
func regMsg[T any, PT interface {
	ID() (MsgID, string)
	*T
}](idName string) {
	m := PT(new(T))
	id, name := m.ID()
	ctor := func() MsgBody { return PT(new(T)) }
	// Only register binary message ID if it's non-zero (ASCII-only messages have id=0)
	if id != 0 {
		msgIDMap[id] = ctor
		idNameMap[id] = idName
		nameIDMap[name] = id
	}
	// Always register with the wire format name (includes "A" suffix except for MODE)
	msgNameMap[name] = ctor
}
