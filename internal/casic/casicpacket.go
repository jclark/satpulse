package casic

import (
	"encoding/binary"
	"fmt"

	"github.com/jclark/satpulse/internal/gpsprot"
)

// Tag is the identifier for CASIC protocol packets
const Tag gpsprot.Tag = "CASBIN"

const (
	sync1Byte = 0xBA
	sync2Byte = 0xCE
)

// PacketFormat is the CASIC packet format
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
	if len(pkt) < 6 {
		return "?-?"
	}
	cls := pkt[4]
	id := pkt[5]
	clsName := clsNames[cls]
	if clsName == "" {
		clsName = fmt.Sprintf("0x%02X", cls)
	}
	return fmt.Sprintf("%s-0x%02X", clsName, id)
}

// ExtractChecksum extracts the checksum from the CASIC packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ExtractChecksum(pkt []byte) []byte {
	return pkt[len(pkt)-4:]
}

// ComputeChecksum computes the checksum for the CASIC packet.
// Precondition: the packet must be valid according to Next().
func (f packetFormat) ComputeChecksum(pkt []byte) []byte {
	ck := Checksum(pkt)
	return binary.LittleEndian.AppendUint32(nil, ck)
}

func (f packetFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	return false
}

// Checksum computes the CASIC checksum for a complete packet.
// The checksum is a 32-bit cumulative sum of the class/id/length header
// and all 4-byte words in the payload.
func Checksum(pkt []byte) uint32 {
	if len(pkt) < 10 {
		return 0
	}
	payloadLen := int(binary.LittleEndian.Uint16(pkt[2:4]))
	cls := pkt[4]
	id := pkt[5]
	// Per errata: receivers use (id << 24) + (cls << 16) + len
	ck := (uint32(id) << 24) + (uint32(cls) << 16) + uint32(payloadLen)
	payload := pkt[6 : 6+payloadLen]
	// Sum 32-bit little-endian words, padding with zeros if needed
	for i := 0; i < len(payload); i += 4 {
		var word uint32
		remaining := len(payload) - i
		switch {
		case remaining >= 4:
			word = binary.LittleEndian.Uint32(payload[i:])
		case remaining == 3:
			word = uint32(payload[i]) | uint32(payload[i+1])<<8 | uint32(payload[i+2])<<16
		case remaining == 2:
			word = uint32(payload[i]) | uint32(payload[i+1])<<8
		case remaining == 1:
			word = uint32(payload[i])
		}
		ck += word
	}
	return ck
}

var clsNames = map[byte]string{
	0x01: "NAV",
	0x02: "TIM",
	0x03: "RXM",
	0x05: "ACK",
	0x06: "CFG",
	0x08: "MSG",
	0x0A: "MON",
	0x0B: "AID",
	0x4E: "NMEA",
}
