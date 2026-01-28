package casic

import (
	"encoding/binary"

	"github.com/jclark/satpulse/internal/casic/bin"
	"github.com/jclark/satpulse/internal/gpsprot"
)

// Tag is the identifier for CASIC binary protocol packets
const Tag gpsprot.Tag = "CASBIN"

const (
	sync1Byte = bin.Sync1
	sync2Byte = bin.Sync2
)

// PacketFormat is the CASIC binary packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

const (
	stateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	stateSync2
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
			return stateLenLo
		}
	case stateLenLo:
		return stateLenHi
	case stateLenHi:
		// payload length is little-endian at buf[2:4]
		// packet format: sync(2) + len(2) + class(1) + id(1) + payload + checksum(4)
		// remaining = class(1) + id(1) + payload + checksum(4) = 2 + payloadLen + 4
		payloadLen := int(buf[nextScanIndex-1]) + int(b)*0x100
		return gpsprot.ScanState(int(stateBody) + payloadLen + 2 + 4)
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
	return bin.PacketMsgID(pkt).String()
}

// ExtractChecksum extracts the checksum from the CASIC binary packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	return pkt[len(pkt)-4:]
}

// ComputeChecksum computes the checksum for the CASIC binary packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	ck := bin.Checksum(pkt[:len(pkt)-4])
	return binary.LittleEndian.AppendUint32(nil, ck)
}

func (f packetFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return false
}
