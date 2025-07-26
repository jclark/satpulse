package unicore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
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

// ParseTimeStatus converts an ASCII time status string to TimeStatus enum
func ParseTimeStatus(s string) TimeStatus {
	switch s {
	case "UNKNOWN":
		return TimeStatusUnknown
	case "COARSE":
		return TimeStatusCoarse
	case "FINE":
		return TimeStatusFine
	default:
		return TimeStatusUnknown
	}
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
	TimeRef            byte       // Reference time (GPST or BDST)
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

var msgMap = make(map[MsgID]func() Msg)
var idNameMap = make(map[MsgID]string)

// String returns a string representation of the message ID
func (mid MsgID) String() string {
	name := idNameMap[mid]
	if name != "" {
		return name
	}
	return fmt.Sprintf("%d", uint16(mid))
}

// UnknownMsg represents an unrecognized message
type UnknownMsg struct {
	MsgID   MsgID
	Payload []byte
}

func (m *UnknownMsg) ID() MsgID { return m.MsgID }

// regMsg registers a message type with its ID and name
func regMsg[T any, PT interface {
	ID() MsgID
	*T
}](idName string) {
	m := PT(new(T))
	mid := m.ID()
	msgMap[mid] = func() Msg { return PT(new(T)) }
	idNameMap[mid] = idName
}

// ParseMsg parses a Unicore binary message from bytes.
// It assumes the checksums were already verified.
func ParseMsg(packet []byte) (MessageHeader, Msg, error) {
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

// SerializeMsg serializes a Unicore message with header into binary format
func SerializeMsg(header MessageHeader, msg Msg) ([]byte, error) {
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

// PacketMsgID returns the MsgID of a packet
func PacketMsgID(packet []byte) MsgID {
	if len(packet) < 6 {
		return 0
	}
	return MsgID(binary.LittleEndian.Uint16(packet[4:6]))
}

func init() {
	// Register known message types
	regMsg[PPSSTATUS]("PPSSTATUS")
}
