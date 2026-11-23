package as

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// Tag is the identifier for Allystar binary protocol packets
const Tag gpsprot.Tag = "ASBIN"

// PacketFormat is the Allystar binary packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

func (f packetFormat) IsBinary() bool {
	return true
}

const (
	sync1Byte = asbin.Sync1
	sync2Byte = asbin.Sync2
)

// State machine for packet scanning.
// Packet format: sync(2) + class(1) + id(1) + len(2) + payload + checksum(2)
const (
	stateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	stateSync2
	stateClass
	stateID
	stateLenLo
	stateLenHi
	stateBody
)

func (f packetFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	switch state {
	case stateSync:
		if b == sync1Byte {
			return stateSync2
		}
	case stateSync2:
		if b == sync2Byte {
			return stateClass
		}
	case stateClass:
		return stateID
	case stateID:
		return stateLenLo
	case stateLenLo:
		return stateLenHi
	case stateLenHi:
		// payload length is little-endian at buf[nextScanIndex-1:nextScanIndex+1]
		// remaining = payload + checksum(2)
		payloadLen := int(buf[nextScanIndex-1]) + int(b)*0x100
		return gpsprot.ScanState(int(stateBody) + payloadLen + 2)
	default:
		if state > stateBody {
			return state - 1
		}
	}
	return stateSync
}

func (f packetFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == stateBody
}

func (f packetFormat) MsgID(pkt []byte) string {
	return asbin.PacketMsgId(pkt).String()
}

// ExtractChecksum extracts the checksum from the Allystar binary packet.
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	return pkt[len(pkt)-2:]
}

// ComputeChecksum computes the checksum for the Allystar binary packet.
// Checksum covers bytes [2:len-2] (class through payload, excluding sync and checksum).
func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	ckA, ckB := asbin.Checksum(pkt[2 : len(pkt)-2])
	return []byte{ckA, ckB}
}

func (f packetFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return false
}
