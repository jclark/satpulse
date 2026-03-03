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

// Port represents the port address encoding used by NovAtel OEM7 and ByNav.
type Port uint8

const (
	COM1 Port = 0x20
	COM2 Port = 0x40
	COM3 Port = 0x60
)

func (p Port) String() string {
	switch p {
	case COM1:
		return "COM1"
	case COM2:
		return "COM2"
	case COM3:
		return "COM3"
	default:
		return strconv.Itoa(int(p))
	}
}

// MarshalText implements encoding.TextMarshaler for fieldenc support.
func (p Port) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support.
func (p *Port) UnmarshalText(text []byte) error {
	switch string(text) {
	case "COM1":
		*p = COM1
	case "COM2":
		*p = COM2
	case "COM3":
		*p = COM3
	default:
		n, err := strconv.ParseUint(string(text), 10, 8)
		if err != nil {
			return fmt.Errorf("invalid port: %s", text)
		}
		*p = Port(n)
	}
	return nil
}

// UnicorePort represents the port address encoding used by Unicore receivers
// (UM980, etc.) when emitting NovAtel-format packets.
type UnicorePort uint8

const (
	UnicoreCOM1 UnicorePort = 1
	UnicoreCOM2 UnicorePort = 2
	UnicoreCOM3 UnicorePort = 3
)

func (p UnicorePort) String() string {
	switch p {
	case UnicoreCOM1:
		return "COM1"
	case UnicoreCOM2:
		return "COM2"
	case UnicoreCOM3:
		return "COM3"
	default:
		return strconv.Itoa(int(p))
	}
}

// MarshalText implements encoding.TextMarshaler for fieldenc support.
func (p UnicorePort) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support.
func (p *UnicorePort) UnmarshalText(text []byte) error {
	switch string(text) {
	case "COM1":
		*p = UnicoreCOM1
	case "COM2":
		*p = UnicoreCOM2
	case "COM3":
		*p = UnicoreCOM3
	default:
		n, err := strconv.ParseUint(string(text), 10, 8)
		if err != nil {
			return fmt.Errorf("invalid port: %s", text)
		}
		*p = UnicorePort(n)
	}
	return nil
}

// SinoPort represents the port address encoding used by SinoGNSS receivers.
// Same binary byte values as OEM7 (0x20, 0x40, 0x60) but serialized as
// decimal strings in ASCII ("32", "64", "96") instead of "COM1", "COM2", "COM3".
type SinoPort uint8

func (p SinoPort) String() string {
	return strconv.Itoa(int(p))
}

// MarshalText implements encoding.TextMarshaler for fieldenc support.
func (p SinoPort) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support.
func (p *SinoPort) UnmarshalText(text []byte) error {
	n, err := strconv.ParseUint(string(text), 10, 8)
	if err != nil {
		return fmt.Errorf("invalid port: %s", text)
	}
	*p = SinoPort(n)
	return nil
}

// BinaryHdr represents the standard 28-byte header of a NovAtel binary packet.
// Parameterized on port type: Port, UnicorePort, or SinoPort.
type BinaryHdr[P ~uint8] struct {
	Sync1         byte // 0xAA
	Sync2         byte // 0x44
	Sync3         byte // 0x12
	HeaderLength  byte
	MessageID     MsgID
	MessageType   byte // Message type/format
	Port          P
	MessageLength uint16 // Length of payload (not including header or CRC)
	CommonHdr
}

// UnknownBinMsgBody represents an unrecognized binary message
type UnknownBinMsgBody struct {
	MsgID   MsgID
	Payload []byte
}

func (m *UnknownBinMsgBody) ID() (MsgID, string) { return m.MsgID, "" }

// ParseBinMsg parses a NovAtel binary message using OEM7 port encoding
// and the global message registry.
func ParseBinMsg(packet []byte) (*Msg[Port], error) {
	return ParseBinMsgUsing[Port](packet, msgIDMap)
}

// ParseBinMsgUsing parses a NovAtel binary message using a specific port type
// and constructor map.
func ParseBinMsgUsing[P ~uint8](packet []byte, ctors map[MsgID]func() MsgBody) (*Msg[P], error) {
	n := len(packet)
	if n < 4 {
		return nil, fmt.Errorf("NOVB message too short (length %d bytes)", n)
	}
	headerLen := int(packet[headerLengthOffset])
	minLen := headerLen + crcLength
	if n < minLen {
		return nil, fmt.Errorf("NOVB message too short (length %d bytes, need %d)", n, minLen)
	}
	var binHdr BinaryHdr[P]
	headerReader := bytes.NewReader(packet[:headerLen])
	err := binary.Read(headerReader, binary.LittleEndian, &binHdr)
	if err != nil {
		return nil, fmt.Errorf("parsing NOVB header: %v", err)
	}
	msgHdr := MsgHdr[P]{
		Port:      binHdr.Port,
		CommonHdr: binHdr.CommonHdr,
	}
	msgID := MsgID(binHdr.MessageID)
	payloadLen := int(binHdr.MessageLength)
	expectedLen := headerLen + payloadLen + crcLength
	if n != expectedLen {
		return nil, fmt.Errorf("NOVB message length mismatch: got %d, expected %d", n, expectedLen)
	}
	payload := packet[headerLen : headerLen+payloadLen]
	ctor := ctors[msgID]
	if ctor == nil {
		return &Msg[P]{
			Hdr:  msgHdr,
			Body: &UnknownBinMsgBody{MsgID: msgID, Payload: payload},
		}, nil
	}
	body := ctor()
	r := bytes.NewReader(payload)
	err = ReadBinChunked(r, body, "NOVB-"+msgID.String())
	if err != nil {
		return nil, err
	}
	_, err = r.ReadByte()
	if err != io.EOF {
		return nil, fmt.Errorf("parsing NOVB-%s: trailing bytes", msgID.String())
	}
	return &Msg[P]{Hdr: msgHdr, Body: body}, nil
}

// SerializeBinMsg serializes a NovAtel message with header into binary format.
func SerializeBinMsg[P ~uint8](msg *Msg[P]) ([]byte, error) {
	msgID, _ := msg.Body.ID()
	if msgID == 0 {
		return nil, fmt.Errorf("unknown ASCII message cannot be serialized as binary")
	}
	var payload []byte
	var err error
	if uMsg, ok := msg.Body.(*UnknownBinMsgBody); ok {
		payload = uMsg.Payload
	} else {
		buf := new(bytes.Buffer)
		err = WriteBinChunked(buf, msg.Body, msgID.String())
		if err != nil {
			return nil, err
		}
		payload = buf.Bytes()
	}
	if len(payload) > 0xFFFF {
		return nil, fmt.Errorf("novatel-%s payload too long (%d bytes)", msgID.String(), len(payload))
	}
	binHdr := BinaryHdr[P]{
		Sync1:         Sync1,
		Sync2:         Sync2,
		Sync3:         Sync3,
		HeaderLength:  standardHeaderLength,
		MessageID:     msgID,
		Port:          msg.Hdr.Port,
		MessageLength: uint16(len(payload)),
		CommonHdr:     msg.Hdr.CommonHdr,
	}
	headerBuf := new(bytes.Buffer)
	err = binary.Write(headerBuf, binary.LittleEndian, &binHdr)
	if err != nil {
		return nil, err
	}
	packet := append(headerBuf.Bytes(), payload...)
	crc := CRC32(packet)
	crcBytes := make([]byte, crcLength)
	binary.LittleEndian.PutUint32(crcBytes, crc)
	packet = append(packet, crcBytes...)
	return packet, nil
}
