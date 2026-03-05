package novmsg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/lib/fieldenc"
)

// AsciiHdr represents the header fields from a NovAtel ASCII message.
// Parameterized on port type for correct port encoding.
type AsciiHdr[P ~uint8] struct {
	MessageName string // Message name (e.g., "BESTPOSA")
	Port        P
	CommonHdr
}

// UnknownAsciiMsgBody represents an unrecognized ASCII message
type UnknownAsciiMsgBody struct {
	Name    string
	Payload string
}

func (m *UnknownAsciiMsgBody) ID() (MsgID, string) { return 0, m.Name }

// ParseAsciiMessage parses a NovAtel ASCII message using OEM7 port encoding
// and the global message registry.
func ParseAsciiMessage(packet []byte) (*Msg[Port], error) {
	return ParseAsciiMsgUsing[Port](packet, msgNameMap)
}

// ParseAsciiMsgUsing parses a NovAtel ASCII message using a specific port type
// and constructor map.
func ParseAsciiMsgUsing[P ~uint8](packet []byte, ctors map[string]func() MsgBody) (*Msg[P], error) {
	asciiMsg := string(packet)
	asciiMsg = asciiMsg[1 : len(asciiMsg)-2]
	headerPart, rest, _ := strings.Cut(asciiMsg, ";")
	dataPart, _, _ := strings.Cut(rest, "*")
	headerFields := strings.Split(headerPart, ",")
	var asciiHdr AsciiHdr[P]
	err := fieldenc.Decode(headerFields, &asciiHdr)
	if err != nil {
		return nil, fmt.Errorf("parsing header: %v", err)
	}
	msgHdr := MsgHdr[P]{
		Port:      asciiHdr.Port,
		CommonHdr: asciiHdr.CommonHdr,
	}
	ctor := ctors[asciiHdr.MessageName]
	if ctor == nil {
		return &Msg[P]{
			Hdr:  msgHdr,
			Body: &UnknownAsciiMsgBody{Name: asciiHdr.MessageName, Payload: dataPart},
		}, nil
	}
	body := ctor()
	var dataFields []string
	if dataPart != "" {
		dataFields = strings.Split(dataPart, ",")
	}
	err = DecodeAsciiChunked(dataFields, body, asciiHdr.MessageName)
	if err != nil {
		return nil, err
	}
	return &Msg[P]{Hdr: msgHdr, Body: body}, nil
}

// SerializeAsciiMsg serializes a NovAtel message with header into ASCII format.
func SerializeAsciiMsg[P ~uint8](msg *Msg[P]) ([]byte, error) {
	_, msgName := msg.Body.ID()
	if msgName == "" {
		return nil, fmt.Errorf("unknown binary message cannot be serialized as ASCII")
	}
	asciiHdr := AsciiHdr[P]{
		MessageName: msgName,
		Port:        msg.Hdr.Port,
		CommonHdr:   msg.Hdr.CommonHdr,
	}
	headerFields, err := fieldenc.Encode(asciiHdr)
	if err != nil {
		return nil, fmt.Errorf("encoding header: %v", err)
	}
	var dataBuilder strings.Builder
	dataBuilder.WriteString(strings.Join(headerFields, ","))
	dataBuilder.WriteByte(';')
	if uMsg, ok := msg.Body.(*UnknownAsciiMsgBody); ok {
		dataBuilder.WriteString(uMsg.Payload)
	} else {
		dataFields, err := EncodeAsciiChunked(msg.Body, msgName)
		if err != nil {
			return nil, err
		}
		if len(dataFields) > 0 {
			dataBuilder.WriteString(strings.Join(dataFields, ","))
		}
	}
	dataForChecksum := dataBuilder.String()
	checksum := CRC32([]byte(dataForChecksum))
	var packet strings.Builder
	packet.WriteByte('#')
	packet.WriteString(dataForChecksum)
	packet.WriteString(fmt.Sprintf("*%08x", checksum))
	packet.WriteString("\r\n")
	return []byte(packet.String()), nil
}
