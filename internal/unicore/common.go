package unicore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/jclark/satpulse/internal/fieldenc"
)

// MsgID represents a Unicore message identifier (uint16)
type MsgID uint16

// Message IDs
const (
	PPSStatusID MsgID = 9000
)

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

// BinaryHeader represents the 24-byte header of a Unicore binary packet
type BinaryHeader struct {
	Sync1          byte   // 0xAA
	Sync2          byte   // 0x44
	Sync3          byte   // 0xB5
	CPUIdlePercent byte   // CPU idle percentage (0-100)
	MessageID      MsgID  // Message identifier
	MessageLength  uint16 // Length of data payload (not including header or CRC)
	TimingHeader          // Embedded timing header
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

// AsciiHeader represents the header fields from a Unicore ASCII message
// Fields are processed by fieldenc in struct field order
type AsciiHeader struct {
	MessageName string // Message name (e.g., "PPSSTATUSA")
	CPUIdle     byte   // CPU idle percentage
	TimingHeader       // Embedded timing fields
}

// Msg interface that all Unicore messages must implement
type Msg interface {
	ID() MsgID
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

// UnknownMsg represents an unrecognized binary message
type UnknownMsg struct {
	MsgID   MsgID
	Payload []byte
}

func (m *UnknownMsg) ID() MsgID { return m.MsgID }

// UnknownAsciiMsg represents an unrecognized ASCII message
type UnknownAsciiMsg struct {
	MsgID   MsgID
	Payload string
}

func (m *UnknownAsciiMsg) ID() MsgID { return m.MsgID }

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

// ParseBinMsg parses a Unicore binary message from bytes.
// It assumes the checksums were already verified.
func ParseBinMsg(packet []byte) (MessageHeader, Msg, error) {
	n := len(packet)
	minLen := headerLength + crcLength
	if n < minLen {
		return MessageHeader{}, nil, fmt.Errorf("UNCB message too short (length %d bytes)", n)
	}

	// Parse binary header
	var binHeader BinaryHeader
	headerReader := bytes.NewReader(packet[:headerLength])
	err := binary.Read(headerReader, binary.LittleEndian, &binHeader)
	if err != nil {
		return MessageHeader{}, nil, fmt.Errorf("parsing UNCB header: %v", err)
	}

	// Extract message header info
	msgHeader := MessageHeader{
		CPUIdlePercent: binHeader.CPUIdlePercent,
		TimingHeader:   binHeader.TimingHeader,
	}

	// Extract message ID and payload length
	msgID := MsgID(binHeader.MessageID)
	payloadLen := int(binHeader.MessageLength)

	// Calculate expected total length
	expectedLen := headerLength + payloadLen + crcLength
	if n != expectedLen {
		return MessageHeader{}, nil, fmt.Errorf("UNCB message length mismatch: got %d, expected %d", n, expectedLen)
	}

	// Extract payload
	payload := packet[headerLength : headerLength+payloadLen]

	// Look up message constructor
	ctor := msgMap[msgID]
	if ctor == nil {
		return msgHeader, &UnknownMsg{MsgID: msgID, Payload: payload}, nil
	}

	// Create and populate message
	msg := ctor()
	r := bytes.NewReader(payload)
	err = binary.Read(r, binary.LittleEndian, msg)
	if err != nil {
		return MessageHeader{}, nil, fmt.Errorf("parsing UNCB-%s: %v", msgID.String(), err)
	}

	// Check for trailing bytes
	_, err = r.ReadByte()
	if err != io.EOF {
		return MessageHeader{}, nil, fmt.Errorf("parsing UNCB-%s: trailing bytes", msgID.String())
	}

	return msgHeader, msg, nil
}

// SerializeBinMsg serializes a Unicore message with header into binary format
func SerializeBinMsg(header MessageHeader, msg Msg) ([]byte, error) {
	var payload []byte
	var err error

	if uMsg, ok := msg.(*UnknownMsg); ok {
		payload = uMsg.Payload
	} else {
		// Serialize the message payload
		buf := new(bytes.Buffer)
		err = binary.Write(buf, binary.LittleEndian, msg)
		if err != nil {
			return nil, err
		}
		payload = buf.Bytes()
	}

	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("unicore-%s payload too long (%d bytes)", msg.ID().String(), len(payload))
	}

	// Create binary header
	binHeader := BinaryHeader{
		Sync1:          sync1,
		Sync2:          sync2,
		Sync3:          sync3,
		CPUIdlePercent: header.CPUIdlePercent,
		MessageID:      msg.ID(),
		MessageLength:  uint16(len(payload)),
		TimingHeader:   header.TimingHeader,
	}

	// Serialize header
	headerBuf := new(bytes.Buffer)
	err = binary.Write(headerBuf, binary.LittleEndian, &binHeader)
	if err != nil {
		return nil, err
	}

	// Combine header and payload
	packet := append(headerBuf.Bytes(), payload...)

	// Calculate and append CRC
	crc := crc32(packet)
	crcBytes := make([]byte, crcLength)
	binary.LittleEndian.PutUint32(crcBytes, crc)
	packet = append(packet, crcBytes...)

	return packet, nil
}

// ParseAsciiMessage parses a Unicore ASCII message from bytes.
// It assumes the packet format scanner has validated the packet structure
// and checksums were already verified.
func ParseAsciiMessage(packet []byte) (MessageHeader, Msg, error) {
	asciiMsg := string(packet)
	
	// Remove # prefix and \r\n suffix
	asciiMsg = asciiMsg[1 : len(asciiMsg)-2]
	
	// Split header from rest at first semicolon
	headerPart, rest, _ := strings.Cut(asciiMsg, ";")
	
	// Determine data part based on packet ending
	var dataPart string
	if rest == "" {
		// Case (a): packet ends with semicolon - no data part
		dataPart = ""
	} else {
		// Case (b) or (c): packet has data and ends with '*' + hex digits
		dataPart, _, _ = strings.Cut(rest, "*")
	}
	
	// Parse header fields
	headerFields := strings.Split(headerPart, ",")
	
	// Parse header using fieldenc
	var asciiHeader AsciiHeader
	err := fieldenc.Decode(headerFields, &asciiHeader)
	if err != nil {
		return MessageHeader{}, nil, fmt.Errorf("parsing header: %v", err)
	}
	
	// Convert to MessageHeader
	msgHeader := MessageHeader{
		CPUIdlePercent: asciiHeader.CPUIdle,
		TimingHeader:   asciiHeader.TimingHeader,
	}
	
	// Look up message ID by name
	msgID, ok := asciiNameIDMap[asciiHeader.MessageName]
	if !ok {
		return MessageHeader{}, nil, fmt.Errorf("unknown message type: %s", asciiHeader.MessageName)
	}
	
	// Look up message constructor
	ctor := msgMap[msgID]
	if ctor == nil {
		return msgHeader, &UnknownAsciiMsg{MsgID: msgID, Payload: dataPart}, nil
	}
	
	// Parse data fields
	// strings.Split("", ",") returns []string{""} but we want []string{} for empty data
	// so that fieldenc.Decode gets no fields instead of one empty field
	var dataFields []string
	if dataPart == "" {
		dataFields = []string{}
	} else {
		dataFields = strings.Split(dataPart, ",")
	}
	
	// Create and populate message
	msg := ctor()
	err = fieldenc.Decode(dataFields, msg)
	if err != nil {
		return MessageHeader{}, nil, fmt.Errorf("parsing %s data: %v", asciiHeader.MessageName, err)
	}
	
	return msgHeader, msg, nil
}

// BinPacketMsgID returns the MsgID of a packet
func BinPacketMsgID(packet []byte) MsgID {
	if len(packet) < 6 {
		return 0
	}
	return MsgID(binary.LittleEndian.Uint16(packet[4:6]))
}

func init() {
	// Register known message types
	regMsg[PPSSTATUS]("PPSSTATUS")
}
