package uncmsg

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/jclark/satpulse/internal/novmsg"
)

// First three bytes of a Unicore binary packet
const (
	Sync1 byte = 0xAA
	Sync2 byte = 0x44
	Sync3 byte = 0xB5
)

const headerLength = 24 // Total header length including 3 sync bytes
const crcLength = 4

// BinaryHdr represents the 24-byte header of a Unicore binary packet
type BinaryHdr struct {
	Sync1          byte   // 0xAA
	Sync2          byte   // 0x44
	Sync3          byte   // 0xB5
	CPUIdlePercent byte   // CPU idle percentage (0-100)
	MessageID      MsgID  // Message identifier
	MessageLength  uint16 // Length of data payload (not including header or CRC)
	TimingHdr             // Embedded timing header
}

// UnknownBinMsgBody represents an unrecognized binary message
type UnknownBinMsgBody struct {
	MsgID   MsgID
	Payload []byte
}

func (m *UnknownBinMsgBody) ID() (MsgID, string) { return m.MsgID, "" }

// ParseBinMsg parses a Unicore binary message from bytes.
// It assumes the checksums were already verified.
func ParseBinMsg(packet []byte) (*Msg, error) {
	n := len(packet)
	minLen := headerLength + crcLength
	if n < minLen {
		return nil, fmt.Errorf("UNCB message too short (length %d bytes)", n)
	}

	// Parse binary header
	var binHdr BinaryHdr
	hdrReader := bytes.NewReader(packet[:headerLength])
	err := binary.Read(hdrReader, binary.LittleEndian, &binHdr)
	if err != nil {
		return nil, fmt.Errorf("parsing UNCB header: %v", err)
	}

	// Extract message header info
	msgHdr := MsgHdr{
		CPUIdlePercent: binHdr.CPUIdlePercent,
		TimingHdr:      binHdr.TimingHdr,
	}

	// Extract message ID and payload length
	msgID := MsgID(binHdr.MessageID)
	payloadLen := int(binHdr.MessageLength)

	// Calculate expected total length
	expectedLen := headerLength + payloadLen + crcLength
	if n != expectedLen {
		return nil, fmt.Errorf("UNCB message length mismatch: got %d, expected %d", n, expectedLen)
	}

	// Extract payload
	payload := packet[headerLength : headerLength+payloadLen]

	// Look up message constructor
	ctor := msgIDMap[msgID]
	if ctor == nil {
		return &Msg{Hdr: msgHdr, Body: &UnknownBinMsgBody{MsgID: msgID, Payload: payload}}, nil
	}

	// Create and populate message
	msgBody := ctor()
	r := bytes.NewReader(payload)

	err = novmsg.ReadBinChunked(r, msgBody, "UNCB-"+msgID.String())
	if err != nil {
		return nil, err
	}

	// Check for trailing bytes
	_, err = r.ReadByte()
	if err != io.EOF {
		return nil, fmt.Errorf("parsing UNCB-%s: trailing bytes", msgID.String())
	}

	return &Msg{Hdr: msgHdr, Body: msgBody}, nil
}

// SerializeBinMsg serializes a Unicore message with header into binary format
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

		err = novmsg.WriteBinChunked(buf, msg.Body, msgID.String())
		if err != nil {
			return nil, err
		}
		payload = buf.Bytes()
	}

	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("unicore-%s payload too long (%d bytes)", msgID.String(), len(payload))
	}

	// Create binary header
	binHdr := BinaryHdr{
		Sync1:          Sync1,
		Sync2:          Sync2,
		Sync3:          Sync3,
		CPUIdlePercent: msg.Hdr.CPUIdlePercent,
		MessageID:      msgID,
		MessageLength:  uint16(len(payload)),
		TimingHdr:      msg.Hdr.TimingHdr,
	}

	// Serialize header
	hdrBuf := new(bytes.Buffer)
	err = binary.Write(hdrBuf, binary.LittleEndian, &binHdr)
	if err != nil {
		return nil, err
	}

	// Combine header and payload
	packet := append(hdrBuf.Bytes(), payload...)

	// Calculate and append CRC
	crc := novmsg.CRC32(packet)
	crcBytes := make([]byte, crcLength)
	binary.LittleEndian.PutUint32(crcBytes, crc)
	packet = append(packet, crcBytes...)

	return packet, nil
}
