package sdbp

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

// Tag is the identifier for SDBP (SD Binary Protocol) packets.
const Tag gpsprot.Tag = "SDBP"

// PacketFormat is the SDBP packet format.
var PacketFormat gpsprot.PacketFormat = packetFormat{}

type packetFormat struct{}

func (f packetFormat) Tag() gpsprot.Tag {
	return Tag
}

const (
	sync1Byte = sdbpbin.Sync1
	sync2Byte = sdbpbin.Sync2
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
	return sdbpbin.PacketMsgId(pkt).String()
}

// ExtractChecksum extracts the 2-byte checksum from an SDBP packet.
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	return pkt[len(pkt)-2:]
}

// ComputeChecksum computes the UBX-style Fletcher checksum for an SDBP packet.
// Checksum covers class, id, length, and payload (bytes [2:len-2]).
func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	ckA, ckB := sdbpbin.Checksum(pkt[2 : len(pkt)-2])
	return []byte{ckA, ckB}
}

func (f packetFormat) IsBinary() bool {
	return true
}

func (f packetFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return false
}
