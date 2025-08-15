package unc

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/uncmsg"
)

// BinPacketProcessor implements the gpsprot.PacketProcessor interface for Unicore binary packets
type BinPacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
}

// NewBinPacketProcessor creates a new Unicore binary packet processor
func NewBinPacketProcessor() *BinPacketProcessor {
	return &BinPacketProcessor{}
}

// ProcessPacket processes a Unicore binary packet's data and returns the message ID and any error
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	_, msg, err := uncmsg.ParseBinMsg(bytes)
	if err != nil {
		return BinPacketFormat.MsgID(bytes), err
	}
	msgID := msg.ID().String()
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return msgID, nmh.NativeMsg(TagBinary, msgID, msg, tRead)
	}
	return msgID, nil
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *BinPacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// AsciiPacketProcessor implements the gpsprot.PacketProcessor interface for Unicore ASCII packets
type AsciiPacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
}

// NewAsciiPacketProcessor creates a new Unicore ASCII packet processor
func NewAsciiPacketProcessor() *AsciiPacketProcessor {
	return &AsciiPacketProcessor{}
}

// ProcessPacket processes a Unicore ASCII packet's data and returns the message ID and any error
func (p *AsciiPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	_, msg, err := uncmsg.ParseAsciiMessage(bytes)
	if err != nil {
		return AsciiPacketFormat.MsgID(bytes), err
	}
	msgID := msg.ID().String()
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return msgID, nmh.NativeMsg(TagAscii, msgID, msg, tRead)
	}
	return msgID, nil
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *AsciiPacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

