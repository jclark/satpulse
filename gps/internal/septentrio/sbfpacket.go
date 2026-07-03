package septentrio

import (
	"encoding/binary"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

// Tag is the identifier for Septentrio SBF packets.
const Tag gpsprot.Tag = "SBF"

// PacketFormat is the Septentrio SBF packet format.
var PacketFormat gpsprot.PacketFormat = packetFormat{}

type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

func (f packetFormat) IsBinary() bool {
	return true
}

const (
	stateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	stateStarted
	stateExpectN
)

func (f packetFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	switch state {
	case stateSync:
		if b == sbfbin.Sync1 {
			return stateStarted
		}
	case stateStarted:
		switch packetLen {
		case 1:
			if b == sbfbin.Sync2 {
				return stateStarted
			}
		case 2, 3, 4, 5, 6:
			return stateStarted
		case 7:
			length := int(buf[nextScanIndex-1]) | int(b)<<8
			bodyLen := length - sbfbin.HeaderLen
			if bodyLen > 0 && bodyLen%4 == 0 {
				return gpsprot.ScanState(int(stateExpectN) + bodyLen)
			}
		default:
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

func (f packetFormat) MsgID(pkt []byte) string {
	return sbfbin.PacketMsgID(pkt).String()
}

func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	return pkt[2:4]
}

func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	crc := sbfbin.CRC16(pkt[4:])
	return binary.LittleEndian.AppendUint16(nil, crc)
}

func (f packetFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return true
}

// PacketProcessor implements gpsprot.PacketProcessor for SBF packets.
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
}

// NewPacketProcessor creates a new SBF packet processor.
func NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor {
	return &PacketProcessor{}
}

// ProcessPacket parses an SBF packet and forwards it to the native message handler.
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	msg, err := sbfbin.ParseMsg(data)
	if err != nil {
		return "", err
	}
	msgID := msg.ID().String()
	if nmh := p.GetNativeMsgHandler(); nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, msg, tRead)
	}
	return msgID, nil
}

// NativeOnly returns true since this phase produces only native messages.
func (p *PacketProcessor) NativeOnly() bool {
	return true
}
