package ubx

import (
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

// PacketFormat is the UBX packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

// packetFormat implements the gpsprot.PacketFormat interface for UBX packets
type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

// First two bytes of UBX packet
const (
	sync1Byte = bin.Sync1
	sync2Byte = bin.Sync2
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

func (f packetFormat) MsgID(pkt []byte) string {
	return bin.PacketMsgId(pkt).String()
}

// ExtractChecksum extracts the checksum from the UBX packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	// UBX checksum is 2 bytes, so return the last 2 bytes of the buffer
	return pkt[len(pkt)-2:]
}

// ComputeChecksum computes the checksum for the UBX packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	ckA, ckB := bin.Checksum(pkt[2:len(pkt)-2])
	return []byte{ckA, ckB}
}

func (f packetFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	// XXX not sure if we should rescan
	return false
}