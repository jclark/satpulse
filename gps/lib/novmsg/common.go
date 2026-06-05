package novmsg

import (
	"fmt"
	"math"
	"strconv"
)

// TimeStatus represents the GPS Reference Time Status field in NovAtel messages
type TimeStatus byte

const (
	TimeStatusUnknown            TimeStatus = 20  // Time validity is unknown
	TimeStatusApprox             TimeStatus = 60  // Time is set approximately
	TimeStatusCoarseAdjusting    TimeStatus = 80  // Time is being adjusted and is valid to coarse precision
	TimeStatusCoarse             TimeStatus = 100 // Time is valid to coarse precision
	TimeStatusCoarsesteering     TimeStatus = 120 // Time is valid to coarse precision and is being steered
	TimeStatusFreewheeling       TimeStatus = 130 // Position is being propagated by an inertial system
	TimeStatusFineAdjusting      TimeStatus = 140 // Time is being adjusted and is valid to fine precision
	TimeStatusFine               TimeStatus = 160 // Time is valid to fine precision
	TimeStatusFineBackupSteering TimeStatus = 170 // Time is valid to fine precision and is being steered by backup source
	TimeStatusFineSteering       TimeStatus = 180 // Time is valid to fine precision and is being steered
)

// String returns the ASCII representation of TimeStatus
func (ts TimeStatus) String() string {
	switch ts {
	case TimeStatusUnknown:
		return "UNKNOWN"
	case TimeStatusApprox:
		return "APPROXIMATE"
	case TimeStatusCoarseAdjusting:
		return "COARSEADJUSTING"
	case TimeStatusCoarse:
		return "COARSE"
	case TimeStatusCoarsesteering:
		return "COARSESTEERING"
	case TimeStatusFreewheeling:
		return "FREEWHEELING"
	case TimeStatusFineAdjusting:
		return "FINEADJUSTING"
	case TimeStatusFine:
		return "FINE"
	case TimeStatusFineBackupSteering:
		return "FINEBACKUPSTEERING"
	case TimeStatusFineSteering:
		return "FINESTEERING"
	default:
		return fmt.Sprintf("%d", ts)
	}
}

// ParseTimeStatus converts an ASCII time status string to TimeStatus enum
func ParseTimeStatus(s string) (TimeStatus, error) {
	switch s {
	case "UNKNOWN":
		return TimeStatusUnknown, nil
	case "APPROXIMATE":
		return TimeStatusApprox, nil
	case "COARSEADJUSTING":
		return TimeStatusCoarseAdjusting, nil
	case "COARSE":
		return TimeStatusCoarse, nil
	case "COARSESTEERING":
		return TimeStatusCoarsesteering, nil
	case "FREEWHEELING":
		return TimeStatusFreewheeling, nil
	case "FINEADJUSTING":
		return TimeStatusFineAdjusting, nil
	case "FINE":
		return TimeStatusFine, nil
	case "FINEBACKUPSTEERING":
		return TimeStatusFineBackupSteering, nil
	case "FINESTEERING":
		return TimeStatusFineSteering, nil
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

// MsgHdr is the parsed message header, parameterized on port type.
// OEM7/ByNav use Port, Unicore uses UnicorePort, SinoGNSS uses SinoPort.
type MsgHdr[P ~uint8] struct {
	Port P
	CommonHdr
}

type CommonHdr struct {
	Sequence           uint16     // Sequence number
	IdleTime           Percentage // Idle time percentage
	TimeStatus         TimeStatus // GPS Reference Time Status
	Week               uint16     // Week number
	MillisecondsOfWeek GPSec      // Milliseconds of the week
	RecvStatus         uint32     // Receiver status
	Reserved           uint16
	Version            uint16
}

type GPSec uint32

// MarshalText implements encoding.TextMarshaler for fieldenc support
// Converts milliseconds to floating-point seconds with 3 decimal places
func (g GPSec) MarshalText() ([]byte, error) {
	seconds := float64(g) / 1000.0
	return []byte(fmt.Sprintf("%.3f", seconds)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
// Parses floating-point seconds and converts to milliseconds
func (g *GPSec) UnmarshalText(text []byte) error {
	seconds, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return err
	}
	*g = GPSec(seconds * 1000.0)
	return nil
}

// Percentage uses values in the range 0-200 to represent a percentage
type Percentage uint8

// MarshalText implements encoding.TextMarshaler for fieldenc support
// Converts byte percentage to float string (e.g., 199 → "99.5")
func (p Percentage) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%.1f", float64(p)/2.0)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
// Parses float string to byte percentage (e.g., "99.5" → 199)
func (p *Percentage) UnmarshalText(text []byte) error {
	val, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return err
	}
	*p = Percentage(math.Round(val*2))
	return nil
}


// MsgBody interface that all NovAtel messages must implement
type MsgBody interface {
	// ID returns the message identifiers:
	// - MsgID: numeric identifier (0 for unknown ASCII messages)
	// - string: ASCII wire format name (e.g., "BESTPOSA", "HEADINGA")
	//   For known messages, this includes the "A" suffix.
	//   For unknown messages, this is exactly what was received (may or may not have "A").
	ID() (MsgID, string)
}


// Msg combines header and body, parameterized on port type.
type Msg[P ~uint8] struct {
	Hdr  MsgHdr[P]
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

// AsciiMsgID returns the binary MsgID for an ASCII wire name (e.g. "BESTPOSA"),
// or 0 if the name is not a known message. It is the inverse of the ASCII name
// in MsgBody.ID, used to map an ASCII packet's wire name back to its canonical
// (suffix-less) name via String.
func AsciiMsgID(name string) MsgID {
	return nameIDMap[name]
}

// BinRegistry returns the global binary message constructor map (by MsgID).
func BinRegistry() map[MsgID]func() MsgBody { return msgIDMap }

// AsciiRegistry returns the global ASCII message constructor map (by wire name).
func AsciiRegistry() map[string]func() MsgBody { return msgNameMap }

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
	// Always register with the wire format name (includes "A" suffix)
	msgNameMap[name] = ctor
}
