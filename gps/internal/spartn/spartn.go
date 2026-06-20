// Package spartn implements gps/gpsprot abstractions for the SPARTN protocol.
// It uses gps/lib/spartnbin to frame packets and extract envelope metadata.
// SPARTN carries (often encrypted) GNSS correction data; this package scans
// frames and surfaces their plaintext metadata, treating the payload as opaque.
package spartn

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/spartnbin"
)

// Tag for SPARTN packets
const Tag gpsprot.Tag = "SPARTN"

// PacketFormat returns the SPARTN packet format
var PacketFormat gpsprot.PacketFormat = packetFormat{}

// packetFormat implements gpsprot.PacketFormat for SPARTN frames.
type packetFormat struct{}

func (packetFormat) Tag() gpsprot.Tag { return Tag }

func (packetFormat) IsBinary() bool { return true }

// Scan states. The header has a variable length (depending on the time-tag
// size and the encryption flag), so the scanner stays in stateHeader until the
// length-determining fields are in hand, then switches to a per-byte countdown
// that ends at stateExpectN, exactly as the RTCM scanner does.
const (
	stateSync    gpsprot.ScanState = iota + gpsprot.ScanStateSync // 0, looking for preamble
	stateHeader                                                   // reading the transport header
	stateExpectN                                                  // countdown anchor (final state)
)

// PreambleByte is the SPARTN frame preamble byte.
const PreambleByte = spartnbin.Preamble

func (packetFormat) Next(state gpsprot.ScanState, buf []byte, nextScanIndex, packetLen int) gpsprot.ScanState {
	b := buf[nextScanIndex]
	switch state {
	case stateSync:
		if b == PreambleByte {
			return stateHeader
		}
	case stateHeader:
		base := nextScanIndex - packetLen
		// TF006 frame CRC-4 gates recognition: a stray preamble byte is
		// rejected here unless TF002-TF005 carry a valid CRC.
		if packetLen == 3 {
			if !spartnbin.VerifyHeaderCRC(buf[base : base+4]) {
				return stateSync
			}
			return stateHeader
		}
		if packetLen >= 4 {
			if total, ok := spartnbin.FrameLen(buf[base : nextScanIndex+1]); ok {
				return gpsprot.ScanState(int(stateExpectN) + total - 1 - packetLen)
			}
		}
		return stateHeader
	default:
		if state > stateExpectN {
			return state - 1
		}
	}
	return stateSync
}

func (packetFormat) IsFinal(state gpsprot.ScanState) bool {
	return state == stateExpectN
}

func (packetFormat) MsgID(pkt []byte) string {
	return spartnbin.MsgID(pkt)
}

// ExtractChecksum extracts the TF018 message CRC from the SPARTN frame.
// Precondition: the packet must be valid according to Next().
func (packetFormat) ExtractChecksum(pkt []byte) []byte {
	return spartnbin.ExtractChecksum(pkt)
}

func (packetFormat) ComputeChecksum(pkt []byte) []byte {
	return spartnbin.ComputeChecksum(pkt)
}

func (packetFormat) RescanOnBadChecksum(prevPktValid bool, pkt []byte) bool {
	return !prevPktValid || !spartnbin.KnownType(pkt)
}

// PacketProcessor implements gpsprot.PacketProcessor for SPARTN packets.
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
}

var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// NewPacketProcessor creates a new SPARTN packet processor.
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{}
}

// ProcessPacket parses a SPARTN frame's envelope and forwards the resulting
// *spartnbin.Frame to the native message handler. It assumes the checksum has
// already been verified.
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	f, err := spartnbin.Parse([]byte(data))
	if err != nil {
		return "", err
	}
	msgID := f.MsgID()
	if nmh := p.GetNativeMsgHandler(); nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, f, tRead)
	}
	return msgID, nil
}

// NativeOnly returns true since SPARTN only provides correction data.
func (p *PacketProcessor) NativeOnly() bool {
	return true
}
