package septentrio

import (
	"encoding/binary"

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
	return false
}
