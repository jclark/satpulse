package rtcm

import (
	"github.com/jclark/crc24q"
	"github.com/jclark/satpulse/internal/gpsprot"
)

// Tag for RTCM packets
const Tag gpsprot.Tag = "RTCM"

// PacketFormat returns the RTCM packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

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

// RTCMMsg extracts message information from an RTCM packet
func RTCMMsg(packet string) (msg string, checksumOK bool, msgType uint16) {
	n := len(packet) - 3
	checksumOK = crc24q.Checksum(packet[0:n]) == crc24q.Extract(packet, n)
	msg = packet[3:n]
	// treat 0-length message as type 0
	if n != 3 {
		msgType = (uint16(msg[0]) << 4) | uint16(msg[1]>>4)
	}
	return
}
