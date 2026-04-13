package rtcm

import (
	"sort"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/rtcmbin"
)

// Tag for RTCM packets
const Tag gpsprot.Tag = "RTCM"

// PacketFormat returns the RTCM packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

var commonMsgTypes = []rtcmbin.MsgType{
	1001, 1002, 1003, 1004, // legacy GPS observables
	1005, // station ARP
	1006, // station ARP with height
	1007, // antenna
	1008, // antenna with serial number
	1009, 1010, 1011, 1012, // legacy GLONASS observables
	1013, // system parameters
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

const ARPMsgType rtcmbin.MsgType = 1005
const GLONASSBiasMsgType rtcmbin.MsgType = 1230

// MSMMsgType returns the RTCM message type for a given GNSS and MSM number.
// Returns 0 if the mapping cannot be done.
func MSMMsgType(gnss gpsprot.GNSS, msm int) rtcmbin.MsgType {
	if msm < 1 || msm > 7 {
		return 0
	}
	var base rtcmbin.MsgType
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
	return base + rtcmbin.MsgType(msm)
}

func isCommonMsgType(mt rtcmbin.MsgType) bool {
	i := sort.Search(len(commonMsgTypes), func(i int) bool {
		return commonMsgTypes[i] >= mt
	})
	return i < len(commonMsgTypes) && commonMsgTypes[i] == mt
}

// packetFormat implements the gpsprot.PacketFormat interface for RTCM packets
type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

func (f packetFormat) IsBinary() bool {
	return true
}

// Constants for RTCM packet scanning (private)
const (
	stateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	stateStarted
	stateExpectN
)

// PreambleByte is the RTCM preamble byte.
const PreambleByte = rtcmbin.PreambleByte

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
	return rtcmbin.ExtractMsgType(pkt).String()
}

// ExtractChecksum extracts the checksum from the RTCM packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	return pkt[len(pkt)-3:]
}

func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	crc := rtcmbin.Checksum(pkt[:len(pkt)-3])
	return crc[:]
}

func (f packetFormat) RescanOnBadChecksum(prevPktValid bool, pkt []byte) bool {
	return !prevPktValid || !isCommonMsgType(rtcmbin.ExtractMsgType(pkt))
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
	msg, err := rtcmbin.ParseMsg(data)
	if err != nil {
		return "", err
	}
	msgID := rtcmbin.ExtractMsgType(data).String()
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
