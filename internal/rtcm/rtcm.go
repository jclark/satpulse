package rtcm

import (
	"fmt"
	"time"

	"github.com/jclark/crc24q"
	"github.com/jclark/satpulse/internal/gpsprot"
)

// Tag for RTCM packets
const Tag gpsprot.Tag = "RTCM"

// PacketFormat returns the RTCM packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

// Message represents an RTCM message
type Message struct {
	Payload    string
	MsgType    uint16
	ChecksumField uint32
	ComputedChecksum uint32
}

var msgNames = map[uint16]string{
	1005: "station-ARP",
	1006: "station-ARP-height",
    1019: "eph-GPS",
    1020: "eph-GLONASS",
	1041: "eph-NavIC",
    1042: "eph-BeiDou",
    1044: "eph-QZSS",
    1045: "eph-Galileo",
    1046: "eph-Galileo",
    1230: "bias-GLONASS",
}

var gnss = []string{"GPS", "GLONASS", "Galileo", "SBAS", "QZSS", "BeiDou", "NavIC"}

func (m *Message) ID() string {
	g := (int(m.MsgType) / 10) - 107
    n := m.MsgType % 10
    if g >= 0 && g < len(gnss) && n >= 1 && n <= 7 {
        return fmt.Sprintf("MSM%d-%s", n, gnss[g])
    }
    if name, ok := msgNames[m.MsgType]; ok {
        return name
    }
    return fmt.Sprintf("%d", m.MsgType)
}

func (m *Message) ChecksumOK() bool {
	return m.ChecksumField == m.ComputedChecksum
}

// packetFormat implements the gpsprot.PacketFormat interface for RTCM packets
type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

// Constants for RTCM packet scanning (private)
const (
	stateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	stateStarted
	stateExpectN
)

const preambleByte = 0xD3

func (f packetFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	switch state {
	case stateSync:
		if b == preambleByte {
			return stateStarted
		}
	case stateStarted:
		switch packetLen {
		case 2:
			payloadLen := int(b) + int(buf[nextScanIndex-1]&0x3)*0x100
			return gpsprot.ScanState(int(stateExpectN) + payloadLen + 3)
		case 1:
			return stateStarted
		}
	default:
		if state > stateExpectN {
			return state - 1
		}
	}
	return stateSync
}

func (f packetFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == stateExpectN
}

// PacketProcessor implements the gpsprot.PacketProcessor interface for RTCM packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
}

// Ensure PacketProcessor implements gpsprot.PacketProcessor
var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// NewPacketProcessor creates a new RTCM packet processor
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{}
}

// ProcessPacket processes an RTCM packet's data and returns any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	msg := ParseMessage(data)
	msgID := msg.ID()
	if !msg.ChecksumOK() {
		// CRC24 so 24 bits, 6 hex digits
		return msgID, fmt.Errorf("RTCM checksum error: checksum in message 0x%06x, computed %06x", msg.ChecksumField, msg.ComputedChecksum)	
	}
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, msg, tRead)
	}
	return msgID, nil
}

// ParseMessage parses a packet into an RTCM Message
func ParseMessage(packet string) *Message {
	n := len(packet) - 3
	payload := packet[3:n]

	// Default message type is 0 for empty messages
	var msgType uint16 = 0

	// Extract message type for non-empty messages
	if n > 3 {
		msgType = (uint16(payload[0]) << 4) | uint16(payload[1]>>4)
	}

	return &Message{
		Payload:    payload,
		MsgType:    msgType,
		ChecksumField: crc24q.Extract(packet, n),
		ComputedChecksum: crc24q.Checksum(packet[0:n]),
	}
}

