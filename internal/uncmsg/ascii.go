package uncmsg

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/jclark/satpulse/internal/fieldenc"
)

// AsciiHeader represents the header fields from a Unicore ASCII message
// Fields are processed by fieldenc in struct field order
type AsciiHeader struct {
	MessageName  string // Message name (e.g., "PPSSTATUSA")
	CPUIdle      byte   // CPU idle percentage
	TimingHeader        // Embedded timing fields
}

// UnknownAsciiMsg represents an unrecognized ASCII message
type UnknownAsciiMsg struct {
	MsgID   MsgID
	Payload string
}

func (m *UnknownAsciiMsg) ID() MsgID { return m.MsgID }

// ParseAsciiMessage parses a Unicore ASCII message from bytes.
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

	// Convert to MessageHeader
	msgHeader := MessageHeader{
		CPUIdlePercent: asciiHeader.CPUIdle,
		TimingHeader:   asciiHeader.TimingHeader,
	}

	// Look up message ID by name
	msgID, ok := asciiNameIDMap[asciiHeader.MessageName]
	if !ok {
		return MessageHeader{}, nil, fmt.Errorf("unknown message type: %s", asciiHeader.MessageName)
	}

	// Look up message constructor
	ctor := msgMap[msgID]
	if ctor == nil {
		return msgHeader, &UnknownAsciiMsg{MsgID: msgID, Payload: dataPart}, nil
	}

	// Parse data fields
	// strings.Split("", ",") returns []string{""} but we want []string{} for empty data
	// so that fieldenc.Decode gets no fields instead of one empty field
	var dataFields []string
	if dataPart == "" {
		dataFields = []string{}
	} else {
		dataFields = strings.Split(dataPart, ",")
	}

	// Create and populate message
	msg := ctor()

	// Check if this is a chunked message
	if chunkedMsg, ok := msg.(ChunkedMsg); ok {
		// Use the chunks iterator to parse the message
		fieldIndex := 0
		chunks := chunkedMsg.Chunks()
		chunks(func(chunk any) bool {
			fieldsConsumed, chunkErr := fieldenc.PartialDecode(dataFields[fieldIndex:], chunk)
			if chunkErr != nil {
				err = chunkErr
				return false
			}
			fieldIndex += fieldsConsumed
			return true
		})
		if err != nil {
			return MessageHeader{}, nil, fmt.Errorf("parsing %s data: %v", asciiHeader.MessageName, err)
		}
	} else {
		// Use PartialDecode for fixed-length messages (allows excess fields)
		_, err = fieldenc.PartialDecode(dataFields, msg)
		if err != nil {
			return MessageHeader{}, nil, fmt.Errorf("parsing %s data: %v", asciiHeader.MessageName, err)
		}
	}

	return msgHeader, msg, nil
}

// SerializeAsciiMsg serializes a Unicore message with header into ASCII format
func SerializeAsciiMsg(header MessageHeader, msg Msg) ([]byte, error) {
	// Get message name from ID
	msgName, found := idNameMap[msg.ID()]
	if !found {
		return nil, fmt.Errorf("unknown message ID: %d", msg.ID())
	}
	asciiMsgName := msgName + "A"

	// Serialize header using fieldenc
	asciiHeader := AsciiHeader{
		MessageName:  asciiMsgName,
		CPUIdle:      header.CPUIdlePercent,
		TimingHeader: header.TimingHeader,
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
			chunks := chunkedMsg.Chunks()
			chunks(func(chunk any) bool {
				chunkFields, chunkErr := fieldenc.Encode(chunk)
				if chunkErr != nil {
					err = chunkErr
					return false
				}
				dataFields = append(dataFields, chunkFields...)
				return true
			})
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
	checksum := CRC32([]byte(dataForChecksum))

	// Build final packet with '#' prefix and checksum
	var packet strings.Builder
	packet.WriteByte('#')
	packet.WriteString(dataForChecksum)
	packet.WriteString(fmt.Sprintf("*%08x", checksum))
	packet.WriteString("\r\n")

	return []byte(packet.String()), nil
}

// BinPacketMsgID returns the MsgID of a packet
func BinPacketMsgID(packet []byte) MsgID {
	if len(packet) < 6 {
		return 0
	}
	return MsgID(binary.LittleEndian.Uint16(packet[4:6]))
}
