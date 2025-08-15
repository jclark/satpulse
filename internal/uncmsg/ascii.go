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
	Name    string
	Payload string
}

func (m *UnknownAsciiMsg) ID() (MsgID, string) { return 0, m.Name }

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

	// Look up message constructor by name
	ctor := msgNameMap[asciiHeader.MessageName]
	if ctor == nil {
		// Unknown message - preserve the name and payload
		return msgHeader, &UnknownAsciiMsg{Name: asciiHeader.MessageName, Payload: dataPart}, nil
	}

	// Create message instance before parsing data fields
	msg := ctor()

	// Parse data fields using adaptive splitting
	var expectedFieldCount int
	if quotedMsg, isQuoted := msg.(QuotedAsciiMsg); isQuoted {
		expectedFieldCount = quotedMsg.QuotedFieldCount()
	} else {
		expectedFieldCount = 0 // No quoting expected
	}
	dataFields := splitDataFields(dataPart, expectedFieldCount)

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

// QuotedAsciiMsg is a marker interface for messages that use CSV-like
// quoted fields in their ASCII representation. Fields may contain commas
// and other special characters when quoted.
type QuotedAsciiMsg interface {
	Msg
	quotedAscii()          // unexported marker method
	QuotedFieldCount() int // expected number of fields when quoted
}

// splitDataFields splits data fields using adaptive logic based on expected field count.
// expectedFieldCount of 0 means no quoting expected (simple comma splitting).
// For quoted messages, tries different strategies based on field count.
func splitDataFields(dataPart string, expectedFieldCount int) []string {
	// Handle empty data
	if dataPart == "" {
		return []string{}
	}

	// Start with simple comma splitting
	fields := strings.Split(dataPart, ",")

	// If no quoting expected, return simple split
	if expectedFieldCount == 0 {
		return fields
	}

	if len(fields) > expectedFieldCount {
		// too many fields
		// possible explanation is that some commas are inside double quotes
		fieldSepPos := make([]int, 0, len(fields))
		inQuotes := false
		for i := 0; i < len(dataPart); i++ {
			if dataPart[i] == '"' {
				inQuotes = !inQuotes
			} else if dataPart[i] == ',' && !inQuotes {
				fieldSepPos = append(fieldSepPos, i)
			}
		}
		if len(fieldSepPos)+1 != expectedFieldCount {
			// that explanation isn't correct
			return fields
		}
		// explanation is correct; split on the found positions
		fields = make([]string, expectedFieldCount)
		if expectedFieldCount > 1 {
			fields[0] = dataPart[:fieldSepPos[0]]
			// we guessed right, so split on the found positions
			for i := 1; i < expectedFieldCount-1; i++ {
				fields[i] = dataPart[fieldSepPos[i-1]+1 : fieldSepPos[i]]
			}
			fields[expectedFieldCount-1] = dataPart[fieldSepPos[expectedFieldCount-2]+1:]
		} else {
			fields[0] = dataPart
		}
	}

	// Now remove quotes
	for i, field := range fields {
		if len(field) >= 2 && field[0] == '"' && field[len(field)-1] == '"' {
			fields[i] = field[1 : len(field)-1]
		}
	}
	return fields

}

// SerializeAsciiMsg serializes a Unicore message with header into ASCII format
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
		// Check if this message uses quoted fields
		if _, isQuoted := msg.(QuotedAsciiMsg); isQuoted {
			// Add quotes around each field
			quotedFields := make([]string, len(dataFields))
			for i, field := range dataFields {
				quotedFields[i] = fmt.Sprintf(`"%s"`, field)
			}
			dataBuilder.WriteString(strings.Join(quotedFields, ","))
		} else {
			dataBuilder.WriteString(strings.Join(dataFields, ","))
		}
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
