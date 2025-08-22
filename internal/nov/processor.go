package nov

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/novmsg"
)

// BinPacketProcessor implements the gpsprot.PacketProcessor interface for NovAtel binary packets
type BinPacketProcessor struct {
	packetProcessor
}

// NewBinPacketProcessor creates a new NovAtel binary packet processor
func NewBinPacketProcessor() *BinPacketProcessor {
	return &BinPacketProcessor{}
}

// ProcessPacket processes a NovAtel binary packet's data and returns the message ID and any error
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := BinPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagBinary, msgID, novmsg.ParseBinMsg)
	return msgID, err
}

// AsciiPacketProcessor implements the gpsprot.PacketProcessor interface for NovAtel ASCII packets
type AsciiPacketProcessor struct {
	packetProcessor
}

// NewAsciiPacketProcessor creates a new NovAtel ASCII packet processor
func NewAsciiPacketProcessor() *AsciiPacketProcessor {
	return &AsciiPacketProcessor{}
}

// ProcessPacket processes a NovAtel ASCII packet's data and returns the message ID and any error
func (p *AsciiPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := AsciiPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagAscii, msgID, novmsg.ParseAsciiMessage)
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
	parser func([]byte) (novmsg.MessageHeader, novmsg.Msg, error)) error {
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
func (p *packetProcessor) dispatch(header novmsg.MessageHeader, msg novmsg.Msg, tRead time.Time, tag gpsprot.Tag) (bool, error) {
	switch m := msg.(type) {
	case *novmsg.Time:
		tm, err := timeTime(header, m, tag)
		if err != nil {
			return false, err
		}
		if tm != nil {
			p.mh.Time(tm, tRead)
			return true, nil
		}
	// TODO: Add other message type conversions here
	// case *novmsg.BestPos:
	// case *novmsg.Heading:
	}
	
	return false, nil
}