package novmsg

import (
	"fmt"
	"strings"

	"github.com/jclark/satpulse/internal/fieldenc"
)

// AsciiHeader represents the header fields from a NovAtel ASCII message
// Fields are processed by fieldenc in struct field order
type AsciiHeader struct {
	MessageName string // Message name (e.g., "BESTPOSA")
	Port string
	CommonHeader
}

// UnknownAsciiMsg represents an unrecognized ASCII message
type UnknownAsciiMsg struct {
	Name    string
	Payload string
}

func (m *UnknownAsciiMsg) ID() (MsgID, string) { return 0, m.Name }

// ParseAsciiMessage parses a NovAtel ASCII message from bytes.
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

	// Convert to MessageHeader - create from CommonHeader and Port
	msgHeader := MessageHeader{
		Port:         asciiHeader.Port,
		CommonHeader: asciiHeader.CommonHeader,
	}

	// Look up message constructor by name
	ctor := msgNameMap[asciiHeader.MessageName]
	if ctor == nil {
		// Unknown message - preserve the name and payload
		return msgHeader, &UnknownAsciiMsg{Name: asciiHeader.MessageName, Payload: dataPart}, nil
	}

	// Create message instance before parsing data fields
	msg := ctor()

	// Parse data fields - NovAtel doesn't use quoted fields
	var dataFields []string
	if dataPart != "" {
		dataFields = strings.Split(dataPart, ",")
	}

	// Check if this is a chunked message
	if chunkedMsg, ok := msg.(ChunkedMsg); ok {
		// Use the chunks iterator to parse the message
		fieldIndex := 0
		for chunk := range chunkedMsg.Chunks() {
			fieldsConsumed, chunkErr := fieldenc.PartialDecode(dataFields[fieldIndex:], chunk)
			if chunkErr != nil {
				err = chunkErr
				break
			}
			fieldIndex += fieldsConsumed
		}
		if err != nil {
			return MessageHeader{}, nil, fmt.Errorf("parsing %s data: %v", asciiHeader.MessageName, err)
		}
		// Ensure all fields were consumed
		if fieldIndex != len(dataFields) {
			return MessageHeader{}, nil, fmt.Errorf("parsing %s data: expected to consume %d fields, consumed %d", asciiHeader.MessageName, len(dataFields), fieldIndex)
		}
	} else {
		err = fieldenc.Decode(dataFields, msg)
		if err != nil {
			return MessageHeader{}, nil, fmt.Errorf("parsing %s data: %v", asciiHeader.MessageName, err)
		}
	}

	return msgHeader, msg, nil
}

// SerializeAsciiMsg serializes a NovAtel message with header into ASCII format
func SerializeAsciiMsg(header MessageHeader, msg Msg) ([]byte, error) {
	// Get message ID and name
	_, msgName := msg.ID()
	if msgName == "" {
		return nil, fmt.Errorf("unknown binary message cannot be serialized as ASCII")
	}

	// Use the name directly - it already contains the correct wire format
	asciiMsgName := msgName

	// Serialize header using fieldenc
	asciiHeader := AsciiHeader{
		MessageName:  asciiMsgName,
		Port:         header.Port,
		CommonHeader: header.CommonHeader,
	}
	headerFields, err := fieldenc.Encode(asciiHeader)
	if err != nil {
		return nil, fmt.Errorf("encoding header: %v", err)
	}

	// Serialize message data
	var dataFields []string
	if uMsg, ok := msg.(*UnknownAsciiMsg); ok {
		// For unknown messages, use the raw payload
		if uMsg.Payload != "" {
			dataFields = strings.Split(uMsg.Payload, ",")
		}
	} else {
		// Check if this is a chunked message
		if chunkedMsg, ok := msg.(ChunkedMsg); ok {
			// Use the chunks iterator to serialize the message
			for chunk := range chunkedMsg.Chunks() {
				chunkFields, chunkErr := fieldenc.Encode(chunk)
				if chunkErr != nil {
					err = chunkErr
					break
				}
				dataFields = append(dataFields, chunkFields...)
			}
			if err != nil {
				return nil, fmt.Errorf("encoding %s data: %v", asciiMsgName, err)
			}
		} else {
			// Use single encode for fixed-length messages
			dataFields, err = fieldenc.Encode(msg)
			if err != nil {
				return nil, fmt.Errorf("encoding %s data: %v", asciiMsgName, err)
			}
		}
	}

	// Build the data part for checksum (excludes leading '#')
	var dataBuilder strings.Builder
	dataBuilder.WriteString(strings.Join(headerFields, ","))
	dataBuilder.WriteByte(';')
	if len(dataFields) > 0 {
		dataBuilder.WriteString(strings.Join(dataFields, ","))
	}

	dataForChecksum := dataBuilder.String()

	// Calculate CRC32 checksum on data (excluding '#')
	// NovAtel always uses CRC32 for serialization
	checksum := CRC32([]byte(dataForChecksum))
	
	// Build final packet with '#' prefix and 8-digit CRC32 checksum
	var packet strings.Builder
	packet.WriteByte('#')
	packet.WriteString(dataForChecksum)
	packet.WriteString(fmt.Sprintf("*%08x", checksum))
	packet.WriteString("\r\n")
	return []byte(packet.String()), nil
}