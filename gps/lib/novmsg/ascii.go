package novmsg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/gps/lib/fieldenc"
)

// AsciiHeader is the constraint for the types of an ASCII message header:
// AsciiHdr or UnicoreAsciiHdr. They differ in the encoding of the receiver
// status and reserved fields.
type AsciiHeader[P ~uint8, H any] interface {
	*H
	msgHdr() (string, MsgHdr[P])
	setMsgHdr(name string, hdr MsgHdr[P])
}

// AsciiHdr represents the header fields of a NovAtel ASCII message as OEM7,
// ByNav and SinoGNSS receivers write them, with the receiver status and
// reserved fields in hex.
type AsciiHdr struct {
	MessageName        string // Message name (e.g., "BESTPOSA")
	Port               Port
	Sequence           uint16
	IdleTime           Percentage
	TimeStatus         TimeStatus
	Week               uint16
	MillisecondsOfWeek GPSec
	RecvStatus         HexUint32
	Reserved           HexUint16
	Version            uint16
}

func (h *AsciiHdr) msgHdr() (string, MsgHdr[Port]) {
	return h.MessageName, MsgHdr[Port]{
		MessageType: MsgFormatASCII,
		Port:        h.Port,
		CommonHdr: CommonHdr{
			Sequence:           h.Sequence,
			IdleTime:           h.IdleTime,
			TimeStatus:         h.TimeStatus,
			Week:               h.Week,
			MillisecondsOfWeek: h.MillisecondsOfWeek,
			RecvStatus:         uint32(h.RecvStatus),
			Reserved:           uint16(h.Reserved),
			Version:            h.Version,
		},
	}
}

func (h *AsciiHdr) setMsgHdr(name string, hdr MsgHdr[Port]) {
	c := hdr.CommonHdr
	*h = AsciiHdr{
		MessageName:        name,
		Port:               hdr.Port,
		Sequence:           c.Sequence,
		IdleTime:           c.IdleTime,
		TimeStatus:         c.TimeStatus,
		Week:               c.Week,
		MillisecondsOfWeek: c.MillisecondsOfWeek,
		RecvStatus:         HexUint32(c.RecvStatus),
		Reserved:           HexUint16(c.Reserved),
		Version:            c.Version,
	}
}

// UnicoreAsciiHdr represents the header fields of a NovAtel-format ASCII
// message from a Unicore receiver. Unicore fills the header with values of
// its own and writes them all in decimal, including the receiver status and
// reserved fields, which NovAtel writes in hex.
type UnicoreAsciiHdr struct {
	MessageName string // Message name (e.g., "BESTPOSA")
	Port        UnicorePort
	CommonHdr
}

func (h *UnicoreAsciiHdr) msgHdr() (string, MsgHdr[UnicorePort]) {
	return h.MessageName, MsgHdr[UnicorePort]{
		MessageType: MsgFormatASCII,
		Port:        h.Port,
		CommonHdr:   h.CommonHdr,
	}
}

func (h *UnicoreAsciiHdr) setMsgHdr(name string, hdr MsgHdr[UnicorePort]) {
	*h = UnicoreAsciiHdr{MessageName: name, Port: hdr.Port, CommonHdr: hdr.CommonHdr}
}

// HexUint32 is a uint32 that is 8 hex digits in ASCII.
type HexUint32 uint32

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (h *HexUint32) UnmarshalText(text []byte) error {
	v, err := strconv.ParseUint(string(text), 16, 32)
	if err != nil {
		return fmt.Errorf("invalid hex value: %s", text)
	}
	*h = HexUint32(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (h HexUint32) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%08x", uint32(h)), nil
}

// HexUint16 is a uint16 that is 4 hex digits in ASCII.
type HexUint16 uint16

// UnmarshalText implements encoding.TextUnmarshaler for fieldenc support
func (h *HexUint16) UnmarshalText(text []byte) error {
	v, err := strconv.ParseUint(string(text), 16, 16)
	if err != nil {
		return fmt.Errorf("invalid hex value: %s", text)
	}
	*h = HexUint16(v)
	return nil
}

// MarshalText implements encoding.TextMarshaler for fieldenc support
func (h HexUint16) MarshalText() ([]byte, error) {
	return fmt.Appendf(nil, "%04x", uint16(h)), nil
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
	return ParseAsciiMsgUsing[Port, AsciiHdr](packet, msgNameMap)
}

// ParseAsciiMsgUsing parses a NovAtel ASCII message using a specific port
// type, header type and constructor map.
func ParseAsciiMsgUsing[P ~uint8, H any, PH AsciiHeader[P, H]](packet []byte, ctors map[string]func() MsgBody) (*Msg[P], error) {
	asciiMsg := string(packet)
	asciiMsg = asciiMsg[1 : len(asciiMsg)-2]
	headerPart, rest, _ := strings.Cut(asciiMsg, ";")
	dataPart, _, _ := strings.Cut(rest, "*")
	headerFields := strings.Split(headerPart, ",")
	asciiHdr := PH(new(H))
	err := fieldenc.Decode(headerFields, asciiHdr)
	if err != nil {
		return nil, fmt.Errorf("parsing header: %v", err)
	}
	msgName, msgHdr := asciiHdr.msgHdr()
	ctor := ctors[msgName]
	if ctor == nil {
		return &Msg[P]{
			Hdr:  msgHdr,
			Body: &UnknownAsciiMsgBody{Name: msgName, Payload: dataPart},
		}, nil
	}
	body := ctor()
	var dataFields []string
	if dataPart != "" {
		dataFields = strings.Split(dataPart, ",")
	}
	err = DecodeAsciiChunked(dataFields, body, msgName)
	if err != nil {
		return nil, err
	}
	return &Msg[P]{Hdr: msgHdr, Body: body}, nil
}

// SerializeAsciiMsg serializes a NovAtel message with header into ASCII format,
// using header type H.
func SerializeAsciiMsg[P ~uint8, H any, PH AsciiHeader[P, H]](msg *Msg[P]) ([]byte, error) {
	_, msgName := msg.Body.ID()
	if msgName == "" {
		return nil, fmt.Errorf("unknown binary message cannot be serialized as ASCII")
	}
	asciiHdr := PH(new(H))
	asciiHdr.setMsgHdr(msgName, msg.Hdr)
	headerFields, err := fieldenc.Encode(*asciiHdr)
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
