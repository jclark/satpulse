package novmsg

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
)

// First three bytes of a NovAtel binary packet
const (
	Sync1 byte = 0xAA
	Sync2 byte = 0x44
	Sync3 byte = 0x12
)

const standardHeaderLength = 28 // Standard header length for creating new messages
const crcLength = 4
const headerLengthOffset = 3 // Offset to header length field in packet

// MsgID represents a NovAtel binary message identifier (uint16)
type MsgID uint16

type Port uint8

const (
	PortCOM1 Port = iota + 1
	PortCOM2
	PortCOM3
)

func (p Port) String() string {
	switch p {
	case PortCOM1:
		return "COM1"
	case PortCOM2:
		return "COM2"
	case PortCOM3:
		return "COM3"
	default:
		return fmt.Sprintf("%d", p)
	}
}

// ParsePort converts a port string back to Port value
func ParsePort(s string) (Port, error) {
	switch s {
	case "COM1":
		return PortCOM1, nil
	case "COM2":
		return PortCOM2, nil
	case "COM3":
		return PortCOM3, nil
	default:
		// Try to parse as decimal number
		if portNum, err := strconv.ParseUint(s, 10, 8); err == nil {
			return Port(portNum), nil
		}
		return 0, fmt.Errorf("invalid port: %s", s)
	}
}

// BinaryHdr represents the standard 28-byte header of a NovAtel binary packet
// Note: NovAtel headers can have variable length, but this struct represents the standard format
type BinaryHdr struct {
	Sync1         byte // 0xAA
	Sync2         byte // 0x44
	Sync3         byte // 0x12
	HeaderLength  byte
	MessageID     MsgID
	MessageType   byte // Message type/format
	Port          Port
	MessageLength uint16 // Length of payload (not including header or CRC)
	CommonHdr
}

// UnknownBinMsgBody represents an unrecognized binary message
type UnknownBinMsgBody struct {
	MsgID   MsgID
	Payload []byte
}

func (m *UnknownBinMsgBody) ID() (MsgID, string) { return m.MsgID, "" }

// ParseBinMsg parses a NovAtel binary message from bytes.
// It assumes the checksums were already verified.
func ParseBinMsg(packet []byte) (*Msg, error) {
	n := len(packet)
	// Need at least 4 bytes to read header length
	if n < 4 {
		return nil, fmt.Errorf("NOVB message too short (length %d bytes)", n)
	}

	// Read the actual header length from the packet
	headerLen := int(packet[headerLengthOffset])
	minLen := headerLen + crcLength
	if n < minLen {
		return nil, fmt.Errorf("NOVB message too short (length %d bytes, need %d)", n, minLen)
	}

	// Parse binary header
	var binHdr BinaryHdr
	headerReader := bytes.NewReader(packet[:headerLen])
	err := binary.Read(headerReader, binary.LittleEndian, &binHdr)
	if err != nil {
		return nil, fmt.Errorf("parsing NOVB header: %v", err)
	}

	// Extract message header info - create MsgHdr from CommonHdr and Port
	msgHdr := MsgHdr{
		Port:      binHdr.Port.String(),
		CommonHdr: binHdr.CommonHdr,
	}

	// Extract message ID and payload length
	msgID := MsgID(binHdr.MessageID)
	payloadLen := int(binHdr.MessageLength)

	// Calculate expected total length
	expectedLen := headerLen + payloadLen + crcLength
	if n != expectedLen {
		return nil, fmt.Errorf("NOVB message length mismatch: got %d, expected %d", n, expectedLen)
	}

	// Extract payload
	payload := packet[headerLen : headerLen+payloadLen]

	// Look up message constructor
	ctor := msgIDMap[msgID]
	if ctor == nil {
		return &Msg{
			Hdr:  msgHdr,
			Body: &UnknownBinMsgBody{MsgID: msgID, Payload: payload},
		}, nil
	}

	// Create and populate message
	body := ctor()
	r := bytes.NewReader(payload)

	// Read the message (handles both chunked and regular messages)
	err = ReadBinChunked(r, body, "NOVB-"+msgID.String())
	if err != nil {
		return nil, err
	}

	// Check for trailing bytes
	_, err = r.ReadByte()
	if err != io.EOF {
		return nil, fmt.Errorf("parsing NOVB-%s: trailing bytes", msgID.String())
	}

	return &Msg{Hdr: msgHdr, Body: body}, nil
}

// SerializeBinMsg serializes a NovAtel message with header into binary format
func SerializeBinMsg(msg *Msg) ([]byte, error) {
	// Get message ID
	msgID, _ := msg.Body.ID()
	if msgID == 0 {
		return nil, fmt.Errorf("unknown ASCII message cannot be serialized as binary")
	}

	var payload []byte
	var err error

	if uMsg, ok := msg.Body.(*UnknownBinMsgBody); ok {
		payload = uMsg.Payload
	} else {
		// Serialize the message payload
		buf := new(bytes.Buffer)

		// Write the message (handles both chunked and regular messages)
		err = WriteBinChunked(buf, msg.Body, msgID.String())
		if err != nil {
			return nil, err
		}
		payload = buf.Bytes()
	}

	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("novatel-%s payload too long (%d bytes)", msgID.String(), len(payload))
	}

	// Parse port from header
	port, err := ParsePort(msg.Hdr.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %v", msg.Hdr.Port, err)
	}

	// Create binary header
	binHdr := BinaryHdr{
		Sync1:         Sync1,
		Sync2:         Sync2,
		Sync3:         Sync3,
		HeaderLength:  standardHeaderLength,
		MessageID:     msgID,
		MessageType:   0, // Default message type
		Port:          port,
		MessageLength: uint16(len(payload)),
		CommonHdr:     msg.Hdr.CommonHdr,
	}

	// Serialize header
	headerBuf := new(bytes.Buffer)
	err = binary.Write(headerBuf, binary.LittleEndian, &binHdr)
	if err != nil {
		return nil, err
	}

	// Combine header and payload
	packet := append(headerBuf.Bytes(), payload...)

	// Calculate and append CRC
	crc := CRC32(packet)
	crcBytes := make([]byte, crcLength)
	binary.LittleEndian.PutUint32(crcBytes, crc)
	packet = append(packet, crcBytes...)

	return packet, nil
}
