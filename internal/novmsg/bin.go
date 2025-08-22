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
	sync1 byte = 0xAA
	sync2 byte = 0x44
	sync3 byte = 0x12
)

const headerLength = 28 // Total header length including 3 sync bytes
const crcLength = 4

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

// BinaryHeader represents the 28-byte header of a NovAtel binary packet
type BinaryHeader struct {
	Sync1         byte // 0xAA
	Sync2         byte // 0x44
	Sync3         byte // 0x12
	HeaderLength  byte
	MessageID     MsgID
	MessageType   byte // Message type/format
	Port          Port
	MessageLength uint16 // Length of payload (not including header or CRC)
	CommonHeader
}

// UnknownBinMsg represents an unrecognized binary message
type UnknownBinMsg struct {
	MsgID   MsgID
	Payload []byte
}

func (m *UnknownBinMsg) ID() (MsgID, string) { return m.MsgID, "" }

// ParseBinMsg parses a NovAtel binary message from bytes.
// It assumes the checksums were already verified.
func ParseBinMsg(packet []byte) (MessageHeader, Msg, error) {
	n := len(packet)
	minLen := headerLength + crcLength
	if n < minLen {
		return MessageHeader{}, nil, fmt.Errorf("NOVB message too short (length %d bytes)", n)
	}

	// Parse binary header
	var binHeader BinaryHeader
	headerReader := bytes.NewReader(packet[:headerLength])
	err := binary.Read(headerReader, binary.LittleEndian, &binHeader)
	if err != nil {
		return MessageHeader{}, nil, fmt.Errorf("parsing NOVB header: %v", err)
	}

	// Extract message header info - create MessageHeader from CommonHeader and Port
	msgHeader := MessageHeader{
		Port:         binHeader.Port.String(),
		CommonHeader: binHeader.CommonHeader,
	}

	// Extract message ID and payload length
	msgID := MsgID(binHeader.MessageID)
	payloadLen := int(binHeader.MessageLength)

	// Calculate expected total length
	expectedLen := headerLength + payloadLen + crcLength
	if n != expectedLen {
		return MessageHeader{}, nil, fmt.Errorf("NOVB message length mismatch: got %d, expected %d", n, expectedLen)
	}

	// Extract payload
	payload := packet[headerLength : headerLength+payloadLen]

	// Look up message constructor
	ctor := msgIDMap[msgID]
	if ctor == nil {
		return msgHeader, &UnknownBinMsg{MsgID: msgID, Payload: payload}, nil
	}

	// Create and populate message
	msg := ctor()
	r := bytes.NewReader(payload)

	// Check if this is a chunked message
	if chunkedMsg, ok := msg.(ChunkedMsg); ok {
		// Use the chunks iterator to read the message
		for chunk := range chunkedMsg.Chunks() {
			if err = binary.Read(r, binary.LittleEndian, chunk); err != nil {
				break
			}
		}
		if err != nil {
			return MessageHeader{}, nil, fmt.Errorf("parsing NOVB-%s: %v", msgID.String(), err)
		}
	} else {
		// Use single read for fixed-length messages
		err = binary.Read(r, binary.LittleEndian, msg)
		if err != nil {
			return MessageHeader{}, nil, fmt.Errorf("parsing NOVB-%s: %v", msgID.String(), err)
		}
	}

	// Check for trailing bytes
	_, err = r.ReadByte()
	if err != io.EOF {
		return MessageHeader{}, nil, fmt.Errorf("parsing NOVB-%s: trailing bytes", msgID.String())
	}

	return msgHeader, msg, nil
}

// SerializeBinMsg serializes a NovAtel message with header into binary format
func SerializeBinMsg(header MessageHeader, msg Msg) ([]byte, error) {
	// Get message ID
	msgID, _ := msg.ID()
	if msgID == 0 {
		return nil, fmt.Errorf("unknown ASCII message cannot be serialized as binary")
	}

	var payload []byte
	var err error

	if uMsg, ok := msg.(*UnknownBinMsg); ok {
		payload = uMsg.Payload
	} else {
		// Serialize the message payload
		buf := new(bytes.Buffer)

		// Check if this is a chunked message
		if chunkedMsg, ok := msg.(ChunkedMsg); ok {
			// Use the chunks iterator to write the message
			for chunk := range chunkedMsg.Chunks() {
				if err = binary.Write(buf, binary.LittleEndian, chunk); err != nil {
					break
				}
			}
			if err != nil {
				return nil, fmt.Errorf("serializing %s: %v", msgID.String(), err)
			}
		} else {
			// Use single write for fixed-length messages
			err = binary.Write(buf, binary.LittleEndian, msg)
			if err != nil {
				return nil, err
			}
		}
		payload = buf.Bytes()
	}

	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("novatel-%s payload too long (%d bytes)", msgID.String(), len(payload))
	}

	// Parse port from header
	port, err := ParsePort(header.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %v", header.Port, err)
	}

	// Create binary header
	binHeader := BinaryHeader{
		Sync1:         sync1,
		Sync2:         sync2,
		Sync3:         sync3,
		HeaderLength:  headerLength,
		MessageID:     msgID,
		MessageType:   0, // Default message type
		Port:          port,
		MessageLength: uint16(len(payload)),
		CommonHeader:  header.CommonHeader,
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
	crc := CRC32(packet)
	crcBytes := make([]byte, crcLength)
	binary.LittleEndian.PutUint32(crcBytes, crc)
	packet = append(packet, crcBytes...)

	return packet, nil
}
