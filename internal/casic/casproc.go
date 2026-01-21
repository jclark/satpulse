package casic

import (
	"time"

	"github.com/jclark/satpulse/internal/casic/bin"
	"github.com/jclark/satpulse/internal/gpsprot"
)

// Ensure PacketProcessor implements gpsprot.PacketProcessor
var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// PacketProcessor implements the gpsprot.PacketProcessor interface for CASIC packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
}

// NewPacketProcessor creates a new CASIC packet processor
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{}
}

// ProcessPacket processes a CASIC packet's data and returns the message ID and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return PacketFormat.MsgID([]byte(data)), err
	}
	msgID := m.ID().String()
	if p.dispatch(m, tRead) {
		return msgID, nil
	}
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, m, tRead)
	}
	return msgID, nil
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

func (p *PacketProcessor) dispatch(m bin.Msg, tRead time.Time) bool {
	var tm *gpsprot.TimeMsg
	switch mt := m.(type) {
	case *bin.NavSol:
		tm = timeNavSol(mt)
	case *bin.NavTimeUTC:
		tm = timeNavTimeUTC(mt)
	case *bin.TimTP:
		tm = timeTimTP(mt)
	default:
		return false
	}
	if tm == nil {
		return false
	}
	if p.mh != nil {
		tm.Tag = Tag
		p.mh.Time(tm, tRead)
	}
	return true
}
