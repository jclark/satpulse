package novmsg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/internal/fieldenc"
)

// AsciiHdr represents the header fields from a NovAtel ASCII message
// Fields are processed by fieldenc in struct field order
type AsciiHdr struct {
	MessageName string // Message name (e.g., "BESTPOSA")
	Port        string
	CommonHdr
}

// UnknownAsciiMsgBody represents an unrecognized ASCII message
type UnknownAsciiMsgBody struct {
	Name    string
	Payload string
}

func (m *UnknownAsciiMsgBody) ID() (MsgID, string) { return 0, m.Name }

// ParseAsciiMessage parses a NovAtel ASCII message from bytes.
// It assumes the packet format scanner has validated the packet structure
// and checksums were already verified.
func ParseAsciiMessage(packet []byte) (*Msg, error) {
	asciiMsg := string(packet)

	// Remove # prefix and \r\n suffix
	asciiMsg = asciiMsg[1 : len(asciiMsg)-2]

	// Split header from rest at first semicolon
	headerPart, rest, _ := strings.Cut(asciiMsg, ";")

	// Extract data part (everything between semicolon and checksum)
	// Packet must have checksum (ends with '*' + hex digits)
	dataPart, _, _ := strings.Cut(rest, "*")

	// Parse header fields
	headerFields := strings.Split(headerPart, ",")

	// Parse header using fieldenc
	var asciiHdr AsciiHdr
	err := fieldenc.Decode(headerFields, &asciiHdr)
	if err != nil {
		return nil, fmt.Errorf("parsing header: %v", err)
	}

	// Convert to MsgHdr - create from CommonHdr and Port
	msgHdr := MsgHdr{
		Port:      asciiHdr.Port,
		CommonHdr: asciiHdr.CommonHdr,
	}

	// Look up message constructor by name
	ctor := msgNameMap[asciiHdr.MessageName]
	if ctor == nil {
		// Unknown message - preserve the name and payload
		return &Msg{
			Hdr:  msgHdr,
			Body: &UnknownAsciiMsgBody{Name: asciiHdr.MessageName, Payload: dataPart},
		}, nil
	}

	// Create message instance before parsing data fields
	body := ctor()

	// Parse data fields - NovAtel doesn't use quoted fields
	var dataFields []string
	if dataPart != "" {
		dataFields = strings.Split(dataPart, ",")
	}

	// Decode the message (handles both chunked and regular messages)
	err = DecodeAsciiChunked(dataFields, body, asciiHdr.MessageName)
	if err != nil {
		return nil, err
	}

	return &Msg{Hdr: msgHdr, Body: body}, nil
}

// SerializeAsciiMsg serializes a NovAtel message with header into ASCII format
func SerializeAsciiMsg(msg *Msg) ([]byte, error) {
	// Get message ID and name
	_, msgName := msg.Body.ID()
	if msgName == "" {
		return nil, fmt.Errorf("unknown binary message cannot be serialized as ASCII")
	}

	// Use the name directly - it already contains the correct wire format
	asciiMsgName := msgName

	// Serialize header using fieldenc
	asciiHdr := AsciiHdr{
		MessageName: asciiMsgName,
		Port:        msg.Hdr.Port,
		CommonHdr:   msg.Hdr.CommonHdr,
	}
	headerFields, err := fieldenc.Encode(asciiHdr)
	if err != nil {
		return nil, fmt.Errorf("encoding header: %v", err)
	}

		// Build the data part for checksum (excludes leading '#')
	var dataBuilder strings.Builder
	dataBuilder.WriteString(strings.Join(headerFields, ","))
	dataBuilder.WriteByte(';')
	// Serialize message data
	if uMsg, ok := msg.Body.(*UnknownAsciiMsgBody); ok {
		// For unknown messages, use the raw payload
		dataBuilder.WriteString(uMsg.Payload)
	} else {
		dataFields, err := EncodeAsciiChunked(msg.Body, asciiMsgName)
		if err != nil {
			return nil, err
		}
		if len(dataFields) > 0 {
			dataBuilder.WriteString(strings.Join(dataFields, ","))
		}
	}

	dataForChecksum := dataBuilder.String()

	// Calculate CRC32 checksum on data (excluding '#')
	checksum := CRC32([]byte(dataForChecksum))

	// Build final packet with '#' prefix and 8-digit CRC32 checksum
	var packet strings.Builder
	packet.WriteByte('#')
	packet.WriteString(dataForChecksum)
	packet.WriteString(fmt.Sprintf("*%08x", checksum))
	packet.WriteString("\r\n")
	return []byte(packet.String()), nil
}
