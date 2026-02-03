package rtcm

import (
	"fmt"
	"sort"
	"time"

	"github.com/jclark/crc24q"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// Tag for RTCM packets
const Tag gpsprot.Tag = "RTCM"

// PacketFormat returns the RTCM packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

type MsgType uint16

// Message represents an RTCM message
type Message struct {
	Payload string
	MsgType MsgType
}

var commomMsgTypes = []MsgType{
	1005, // station ARP
	1006, // station ARP with height
	1007, // antenna
	1008, // antenna with serial number
	1033, // receiver and antenna descriptor
	// MSM 4 and 7
	1074, 1077, // GPS
	1084, 1087, // GLONASS
	1094, 1097, // Galileo
	1104, 1107, // SBAS
	1114, 1117, // QZSS
	1124, 1127, // BeiDou
	1134, 1137, // NavIC (IRNSS)
	1230, // GLONASS bias
}

const ARPMsgType MsgType = 1005
const GLONASSBiasMsgType MsgType = 1230	

// MSMMsgType returns the RTCM message type for a given GNSS and MSM number.
// Returns 0 if the mapping cannot be done.
func MSMMsgType(gnss gpsprot.GNSS, msm int) MsgType {
	if msm < 1 || msm > 7 {
		return 0
	}
	var base MsgType
	switch gnss {
	case gpsprot.GPS:
		base = 1070
	case gpsprot.GLO:
		base = 1080
	case gpsprot.GAL:
		base = 1090
	case gpsprot.BDS:
		base = 1120
	case gpsprot.NAVIC:
		base = 1130
	default:
		return 0
	}
	return base + MsgType(msm)
}

func isCommonMsgType(msgType MsgType) bool {
	// Use binary search directly since the slice is already sorted
	i := sort.Search(len(commomMsgTypes), func(i int) bool {
		return commomMsgTypes[i] >= msgType
	})

	// Check if we found the message type
	return i < len(commomMsgTypes) && commomMsgTypes[i] == msgType
}

func (mt MsgType) String() string {
	return fmt.Sprintf("%d", mt)
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

// PreambleByte is the RTCM preamble byte
const PreambleByte = 0xD3

func (f packetFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	switch state {
	case stateSync:
		if b == PreambleByte {
			return stateStarted
		}
	case stateStarted:
		switch packetLen {
		case 2:
			payloadLen := int(b) + int(buf[nextScanIndex-1]&0x3)*0x100
			return gpsprot.ScanState(int(stateExpectN) + payloadLen + 3)
		case 1:
			if b&^0x3 == 0 {
				return stateStarted
			}
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

func (f packetFormat) MsgID(pkt []byte) string {
	return extractMsgType(pkt).String()
}

// ExtractChecksum extracts the checksum from the RTCM packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	// RTCM checksum is 3 bytes, so return the last 3 bytes of the buffer
	return pkt[len(pkt)-3:]
}

func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	checksum := crc24q.Checksum(pkt[0 : len(pkt)-3])
	return []byte{byte(checksum >> 16), byte(checksum >> 8), byte(checksum)}
}

func (f packetFormat) RescanOnBadChecksum(prevPktValid bool, pkt []byte) bool {
	return !prevPktValid || !isCommonMsgType(extractMsgType(pkt))
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
	msgID := msg.MsgType.String()
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, msg, tRead)
	}
	return msgID, nil
}

// NativeOnly returns true since RTCM only provides correction data
func (p *PacketProcessor) NativeOnly() bool {
	return true
}

// ParseMessage parses a packet into an RTCM Message
func ParseMessage(packet string) *Message {
	return &Message{
		Payload: packet[3 : len(packet)-3],
		MsgType: extractMsgType(packet),
	}
}

type Bytes interface {
	string | []byte
}

func extractMsgType[B Bytes](packet B) MsgType {
	if len(packet) <= 6 {
		return 0
	}
	return (MsgType(packet[3]) << 4) | MsgType(packet[4]>>4)
}
