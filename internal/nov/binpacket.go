package nov

import (
	"encoding/binary"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/novmsg"
)

// TagBinary is the identifier for NovAtel binary packets
const TagBinary gpsprot.Tag = "NOVB"

// BinPacketFormat is the Unicore binary packet format
var BinPacketFormat gpsprot.PacketFormat = binPacketFormat{}

// binPacketFormat implements the gpsprot.PacketFormat interface for Unicore binary packets
type binPacketFormat struct{}


func (f binPacketFormat) Tag() gpsprot.Tag {
	return TagBinary
}

// First three bytes of a NovAtel binary packet
const (
	sync1 = novmsg.Sync1
	sync2 = novmsg.Sync2
	sync3 = novmsg.Sync3
)

const (
	// binStateSync is the initial state looking for the first sync byte.
	binStateSync gpsprot.ScanState = iota + gpsprot.ScanStateSync
	// binStateStarted means we have seen the first sync byte (0xAA).
	binStateStarted
	// binStateExpectN is the base for our countdown state.
	binStateExpectN
)

const (
	crcLength           = 4
	headerLengthOffset  = 3
	msgIDOffset         = 4
	payloadLengthOffset = 8
)

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
		case payloadLengthOffset + 2:
			packetStart := nextScanIndex - packetLen
			headerLen := int(buf[packetStart+headerLengthOffset])
			payloadLen := int(binary.LittleEndian.Uint16(buf[packetStart+payloadLengthOffset:packetStart+payloadLengthOffset+2]))
			totalLen := headerLen + payloadLen + crcLength

			// We have already read packetLen + 1 bytes (11 bytes).
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
	if len(pkt) < msgIDOffset+2 {
		return ""
	}
	// Message ID is a USHORT at offset 4
	msgID := novmsg.MsgID(binary.LittleEndian.Uint16(pkt[msgIDOffset:msgIDOffset+2]))
	return msgID.String()
}

func (f binPacketFormat) ExtractChecksum(pkt []byte) []byte {
	return pkt[len(pkt)-crcLength:]
}

func (f binPacketFormat) ComputeChecksum(pkt []byte) []byte {
	crc := novmsg.CRC32(pkt[:len(pkt)-crcLength])
	checksumBytes := make([]byte, crcLength)
	binary.LittleEndian.PutUint32(checksumBytes, crc)
	return checksumBytes
}

func (f binPacketFormat) RescanOnBadChecksum(_ bool, _ []byte) bool {
	// XXX should we rescan on bad checksum?
	// We have 3 sync bytes, so pretty unlikely it was another kind of packet.
	return false
}
