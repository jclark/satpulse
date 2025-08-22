package nov

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

// BinPacketProcessor implements the gpsprot.PacketProcessor interface for NovAtel binary packets
type BinPacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
}

// NewBinPacketProcessor creates a new NovAtel binary packet processor
func NewBinPacketProcessor() *BinPacketProcessor {
	return &BinPacketProcessor{}
}

// ProcessPacket processes a NovAtel binary packet's data and returns the message ID and any error
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := BinPacketFormat.MsgID(bytes)
	// For now, just pass the raw bytes without parsing
	// TODO: Implement actual packet parsing when NovAtel message parsing is added
	
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		// Pass the raw bytes as the message
		return msgID, nmh.NativeMsg(TagBinary, msgID, bytes, tRead)
	}
	return msgID, nil
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *BinPacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}