package ubx

import (
	"github.com/jclark/satpulse/internal/gpsprot"
)

// Tag for UBX packets
const Tag gpsprot.Tag = "UBX"

// PacketFormat is the UBX packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

// packetFormat implements the gpsprot.PacketFormat interface for UBX packets
type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

// First two bytes of UBX packet
const (
	sync1Byte = 0xB5
	sync2Byte = 0x62
)

const (
	stateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	stateStarted
	stateExpectN
)

func (f packetFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	switch state {
	case stateSync:
		if b == sync1Byte {
			return stateStarted
		}
	case stateStarted:
		switch packetLen {
		case 1:
			if b == sync2Byte {
				return stateStarted
			}
		case 5:
			payloadLen := int(buf[nextScanIndex-1]) + int(b)*0x100
			return gpsprot.ScanState(int(stateExpectN) + payloadLen + 2)
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
