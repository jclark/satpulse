package as

import (
	"time"

	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// Ensure PacketProcessor implements gpsprot.PacketProcessor
var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// PacketProcessor implements the gpsprot.PacketProcessor interface for Allystar binary packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
}

// NewPacketProcessor creates a new Allystar binary packet processor
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{}
}

// ProcessPacket processes an Allystar binary packet's data and returns the message ID and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	m, err := asbin.ParseMsg(data)
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

func (p *PacketProcessor) dispatch(m asbin.Msg, tRead time.Time) bool {
	switch mt := m.(type) {
	case *asbin.NavTime:
		tm := timeNavTime(mt)
		if tm != nil && p.mh != nil {
			tm.Tag = Tag
			p.mh.Time(tm, tRead)
		}
		return true
	case *asbin.NavTimeUTC:
		tm := timeNavTimeUTC(mt)
		if tm != nil && p.mh != nil {
			tm.Tag = Tag
			p.mh.Time(tm, tRead)
		}
		return true
	case *asbin.NavSVInfo:
		sats := satellitesNavSVInfo(mt)
		if sats != nil && p.mh != nil {
			p.mh.Satellites(sats, tRead)
		}
		return true
	case *asbin.NavSvin:
		sv := surveyNavSvin(mt)
		if sv != nil && p.mh != nil {
			p.mh.Survey(sv, tRead)
		}
		return true
	default:
		return false
	}
}
