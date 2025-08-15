package uncmsg

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// First three bytes of a Unicore binary packet
const (
	sync1 byte = 0xAA
	sync2 byte = 0x44
	sync3 byte = 0xB5
)

const headerLength = 24 // Total header length including 3 sync bytes
const crcLength = 4

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

// UnknownBinMsg represents an unrecognized binary message
type UnknownBinMsg struct {
	MsgID   MsgID
	Payload []byte
}

func (m *UnknownBinMsg) ID() (MsgID, string) { return m.MsgID, "" }

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
			return MessageHeader{}, nil, fmt.Errorf("parsing UNCB-%s: %v", msgID.String(), err)
		}
	} else {
		// Use single read for fixed-length messages
		err = binary.Read(r, binary.LittleEndian, msg)
		if err != nil {
			return MessageHeader{}, nil, fmt.Errorf("parsing UNCB-%s: %v", msgID.String(), err)
		}
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
		return nil, fmt.Errorf("unicore-%s payload too long (%d bytes)", msgID.String(), len(payload))
	}

	// Create binary header
	binHeader := BinaryHeader{
		Sync1:          sync1,
		Sync2:          sync2,
		Sync3:          sync3,
		CPUIdlePercent: header.CPUIdlePercent,
		MessageID:      msgID,
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
	crc := CRC32(packet)
	crcBytes := make([]byte, crcLength)
	binary.LittleEndian.PutUint32(crcBytes, crc)
	packet = append(packet, crcBytes...)

	return packet, nil
}
