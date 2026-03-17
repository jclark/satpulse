package sdbp

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// PacketProcessor implements gpsprot.PacketProcessor for SDBP packets.
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh  gpsprot.MsgHandler
	mgr *gpsprot.NavEpochManager
}

// NewPacketProcessor creates a new SDBP packet processor.
func NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor {
	return &PacketProcessor{mgr: mgr}
}

// ProcessPacket processes an SDBP packet.
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	m, err := sdbpbin.ParseMsg(data)
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

// SetMsgHandler sets the handler for protocol-agnostic messages.
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

func (p *PacketProcessor) dispatch(m sdbpbin.Msg, tRead time.Time) bool {
	var tm *gpsprot.TimeMsg
	switch mt := m.(type) {
	case *sdbpbin.DatGPST:
		tm = timeDatGPST(mt)
	case *sdbpbin.DatTPPS:
		tm = timeDatTPPS(mt)
	default:
		return false
	}
	if tm == nil {
		return false
	}
	if h := p.mh; h != nil {
		tm.Tag = Tag
		h.Time(tm, tRead)
	}
	return true
}
