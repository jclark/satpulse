package uncmsg

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/lib/fieldenc"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// AsciiHdr represents the header fields from a Unicore ASCII message
// Fields are processed by fieldenc in struct field order
type AsciiHdr struct {
	MessageName string // Message name (e.g., "PPSSTATUSA")
	CPUIdle     byte   // CPU idle percentage
	TimingHdr          // Embedded timing fields
}

// UnknownAsciiMsgBody represents an unrecognized ASCII message
type UnknownAsciiMsgBody struct {
	Name    string
	Payload string
}

func (m *UnknownAsciiMsgBody) ID() (MsgID, string) { return 0, m.Name }

// ParseAsciiMessage parses a Unicore ASCII message from bytes.
// It assumes the packet format scanner has validated the packet structure
// and checksums were already verified.
func ParseAsciiMessage(packet []byte) (*Msg, error) {
	asciiMsg := string(packet)

	// Remove # prefix and \r\n suffix
	asciiMsg = asciiMsg[1 : len(asciiMsg)-2]

	// Split header from rest at first semicolon
	hdrPart, rest, _ := strings.Cut(asciiMsg, ";")

	// Extract data part (everything between semicolon and checksum)
	// Packet must have checksum (ends with '*' + hex digits)
	dataPart, _, _ := strings.Cut(rest, "*")

	// Parse header fields
	hdrFields := strings.Split(hdrPart, ",")

	// Parse header using fieldenc
	var asciiHdr AsciiHdr
	err := fieldenc.Decode(hdrFields, &asciiHdr)
	if err != nil {
		return nil, fmt.Errorf("parsing header: %v", err)
	}

	// Convert to MsgHdr
	msgHdr := MsgHdr{
		CPUIdlePercent: asciiHdr.CPUIdle,
		TimingHdr:      asciiHdr.TimingHdr,
	}

	// Look up message constructor by name
	ctor := msgNameMap[asciiHdr.MessageName]
	if ctor == nil {
		// Unknown message - preserve the name and payload
		return &Msg{Hdr: msgHdr, Body: &UnknownAsciiMsgBody{Name: asciiHdr.MessageName, Payload: dataPart}}, nil
	}

	// Create message instance before parsing data fields
	msgBody := ctor()

	// Parse data fields using adaptive splitting
	var expectedFieldCount int
	if quotedMsg, isQuoted := msgBody.(QuotedAsciiMsgBody); isQuoted {
		expectedFieldCount = quotedMsg.QuotedFieldCount()
	} else {
		expectedFieldCount = 0 // No quoting expected
	}
	dataFields := splitDataFields(dataPart, expectedFieldCount)

	err = novmsg.DecodeAsciiChunked(dataFields, msgBody, asciiHdr.MessageName)
	if err != nil {
		return nil, err
	}

	return &Msg{Hdr: msgHdr, Body: msgBody}, nil
}

// xorChecksumBody is a marker interface for ASCII messages that use an 8-bit XOR checksum
// over the full packet (including leading '#') instead of the standard 32-bit CRC32.
// The MODE message uses this checksum style.
type xorChecksumBody interface {
	MsgBody
	usesXorChecksum()
}

func xorSum(data []byte) byte {
	var v byte
	for _, b := range data {
		v ^= b
	}
	return v
}

// QuotedAsciiMsgBody is a marker interface for messages that use CSV-like
// quoted fields in their ASCII representation. Fields may contain commas
// and other special characters when quoted.
type QuotedAsciiMsgBody interface {
	MsgBody
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
		CPUIdle:     msg.Hdr.CPUIdlePercent,
		TimingHdr:   msg.Hdr.TimingHdr,
	}
	hdrFields, err := fieldenc.Encode(asciiHdr)
	if err != nil {
		return nil, fmt.Errorf("encoding header: %v", err)
	}

	// Build the data part for checksum (excludes leading '#')
	var dataBuilder strings.Builder
	dataBuilder.WriteString(strings.Join(hdrFields, ","))
	dataBuilder.WriteByte(';')
	// Serialize message data
	if uMsg, ok := msg.Body.(*UnknownAsciiMsgBody); ok {
		// For unknown messages, use the raw payload
		if uMsg.Payload != "" {
			dataBuilder.WriteString(uMsg.Payload)
		}
	} else {
		dataFields, err := novmsg.EncodeAsciiChunked(msg.Body, asciiMsgName)
		if err != nil {
			return nil, err
		}
		if len(dataFields) > 0 {
			// Check if this message uses quoted fields
			if _, isQuoted := msg.Body.(QuotedAsciiMsgBody); isQuoted {
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
	}

	dataForChecksum := dataBuilder.String()

	// Build final packet with '#' prefix and checksum
	var packet strings.Builder
	packet.WriteByte('#')
	packet.WriteString(dataForChecksum)
	// XOR checksum covers the full packet including '#'; CRC32 excludes '#'
	if _, ok := msg.Body.(xorChecksumBody); ok {
		packet.WriteString(fmt.Sprintf("*%02x", xorSum([]byte(packet.String()))))
	} else {
		packet.WriteString(fmt.Sprintf("*%08x", novmsg.CRC32([]byte(dataForChecksum))))
	}
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
