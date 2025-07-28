package unc

import (
	"encoding/binary"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/uncmsg"
)

// BinPacketFormat is the Unicore binary packet format
var BinPacketFormat gpsprot.PacketFormat = binPacketFormat{}

// binPacketFormat implements the gpsprot.PacketFormat interface for Unicore binary packets
type binPacketFormat struct{}


func (f binPacketFormat) Tag() gpsprot.Tag {
	return "UNCB" // Unicore Binary
}

// First three bytes of a Unicore binary packet
const (
	sync1 byte = 0xAA
	sync2 byte = 0x44
	sync3 byte = 0xB5
)

const (
	// binStateSync is the initial state looking for the first sync byte.
	binStateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	// binStateStarted means we have seen the first sync byte (0xAA).
	binStateStarted
	// binStateExpectN is the base for our countdown state.
	binStateExpectN
)

const headerLength = 24 // Total header length including 3 sync bytes
const crcLength = 4

func (f binPacketFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	switch state {
	case binStateSync:
		if b == sync1 {
			return binStateStarted
		}
	case binStateStarted:
		switch packetLen {
		case 1: // Expect second sync byte
			if b == sync2 {
				return binStateStarted
			}
		case 2: // Expect third sync byte
			if b == sync3 {
				return binStateStarted
			}
		case 8: // We have enough bytes to read the payload length (at offset 6-7 from packet start)
			packetStart := nextScanIndex - packetLen
			payloadLen := int(binary.LittleEndian.Uint16(buf[packetStart+6:packetStart+8]))
			totalLen := headerLength + payloadLen + crcLength

			// We have already read packetLen + 1 bytes (9 bytes).
			// The state represents the number of bytes to read *after* this one.
			remaining := totalLen - (packetLen + 1)
			if remaining < 0 {
				return binStateSync // Invalid length
			}
			return binStateExpectN + gpsprot.ScanState(remaining)
		default:
			// If we haven't reached the length field yet, just keep consuming bytes.
			return binStateStarted
		}
	default:
		if state > binStateExpectN {
			return state - 1
		}
	}
	return binStateSync
}

func (f binPacketFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == binStateExpectN
}

func (f binPacketFormat) MsgID(pkt []byte) string {
	if len(pkt) < 6 {
		return ""
	}
	// Message ID is a USHORT at offset 4
	msgID := uncmsg.MsgID(binary.LittleEndian.Uint16(pkt[4:6]))
	return msgID.String()
}

func (f binPacketFormat) ExtractChecksum(pkt []byte) []byte {
	return pkt[len(pkt)-crcLength:]
}

func (f binPacketFormat) ComputeChecksum(pkt []byte) []byte {
	crc := uncmsg.CRC32(pkt[:len(pkt)-crcLength])
	checksumBytes := make([]byte, crcLength)
	binary.LittleEndian.PutUint32(checksumBytes, crc)
	return checksumBytes
}

func (f binPacketFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	// XXX should we rescan on bad checksum?
	// We have 3 sync bytes, so pretty unlikely it was another kind of packet.
	return false
}
