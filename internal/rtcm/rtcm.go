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
	Payload          string
	MsgType          uint16
}

var msgNames = map[uint16]string{
	1001: "L1-GPS",
	1002: "L1x-GPS",
	1003: "L1L2-GPS",
	1004: "L1L2x-GPS",
	1005: "station-ARP",
	1006: "station-ARP-height",
	1007: "antenna",
	1008: "antenna-serial",
	1009: "L1-GLONASS",
	1010: "L1x-GLONASS",
	1011: "L1L2-GLONASS",
	1012: "L1L2x-GLONASS",
	1019: "eph-GPS",
	1020: "eph-GLONASS",
	1033: "rcvr-antenna",
	1041: "eph-NavIC",
	1042: "eph-BeiDou",
	1044: "eph-QZSS",
	1045: "eph-Galileo-F",
	1046: "eph-Galileo-I",
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

// ExtractChecksum extracts the checksum from the RTCM packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	// RTCM checksum is 3 bytes, so return the last 3 bytes of the buffer
	return pkt[len(pkt)-3:]
}

func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	checksum := crc24q.Checksum(pkt[0:len(pkt)-3])
	return []byte{byte(checksum >> 16), byte(checksum >> 8), byte(checksum)}
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

// ProcessPacket processes an RTCM packet's data and returns any error.
// It assumes checksum has already been verified.
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	msg := ParseMessage(data)
	msgID := msg.ID()
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
		Payload:          payload,
		MsgType:          msgType,
	}
}
