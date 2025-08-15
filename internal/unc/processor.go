package unc

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/uncmsg"
)

// BinPacketProcessor implements the gpsprot.PacketProcessor interface for Unicore binary packets
type BinPacketProcessor struct {
	packetProcessor
}

// NewBinPacketProcessor creates a new Unicore binary packet processor
func NewBinPacketProcessor() *BinPacketProcessor {
	return &BinPacketProcessor{}
}

// ProcessPacket processes a Unicore binary packet's data and returns the message ID and any error
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := BinPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagBinary, msgID, uncmsg.ParseBinMsg)
	return msgID, err
}

// AsciiPacketProcessor implements the gpsprot.PacketProcessor interface for Unicore ASCII packets
type AsciiPacketProcessor struct {
	packetProcessor
}

// NewAsciiPacketProcessor creates a new Unicore ASCII packet processor
func NewAsciiPacketProcessor() *AsciiPacketProcessor {
	return &AsciiPacketProcessor{}
}

// ProcessPacket processes a Unicore ASCII packet's data and returns the message ID and any error
func (p *AsciiPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := AsciiPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagAscii, msgID, uncmsg.ParseAsciiMessage)
	return msgID, err
}

// packetProcessor is the common functionality between BinPacketProcessor and AsciiPacketProcessor
type packetProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *packetProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// processPacket is the common packet processing logic for both binary and ASCII packets
func (p *packetProcessor) processPacket(bytes []byte, tRead time.Time, tag gpsprot.Tag, msgID string,
	parser func([]byte) (uncmsg.MessageHeader, uncmsg.Msg, error)) error {
	header, msg, err := parser(bytes)
	if err != nil {
		return err
	}	
	if p.mh != nil {
		handled, err := p.dispatch(header, msg, tRead, tag)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}	
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return nmh.NativeMsg(tag, msgID, msg, tRead)
	}
	return nil
}

// dispatch attempts to convert and dispatch a message as a protocol-agnostic message
// Returns (handled, error) where handled indicates if the message was processed
func (p *packetProcessor) dispatch(header uncmsg.MessageHeader, msg uncmsg.Msg, tRead time.Time, tag gpsprot.Tag) (bool, error) {
	switch m := msg.(type) {
	case *uncmsg.RecTime:
		tm, err := timeRecTime(header, m, tag)
		if err != nil {
			return false, err
		}
		if tm != nil {
			p.mh.Time(tm, tRead)
			return true, nil
		}
	// TODO: Add other message type conversions here
	// case *uncmsg.GPSUTC:
	// case *uncmsg.GALUTC:
	// case *uncmsg.BD3UTC:
	// case *uncmsg.BDSUTC:
	// case *uncmsg.SatsInfo:
	}
	
	return false, nil
}

