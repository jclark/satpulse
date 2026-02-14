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
	// Navigation epoch tracking: Allystar NAV messages contain an iTOW field
	// that identifies which navigation solution they belong to.
	// Invariant: curNavEpochMsg is non-nil iff curNavEpoch is non-zero.
	curNavEpoch    uint32              // current navigation epoch (iTOW + 1 to reserve zero for "no epoch")
	curNavEpochMsg *gpsprot.NavEpochMsg // accumulated NavEpochMsg for current epoch
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
	if nm, ok := m.(asbin.NavMsg); ok {
		p.handleNavEpoch(nm, tRead)
	}
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

func (p *PacketProcessor) handleNavEpoch(nm asbin.NavMsg, tRead time.Time) {
	e := nm.NavEpoch()
	e++ // use zero to represent "no epoch seen"
	if e != p.curNavEpoch {
		p.flushNavEpoch(tRead)
		p.curNavEpoch = e
		p.curNavEpochMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
	}
}

func (p *PacketProcessor) flushNavEpoch(tRead time.Time) {
	if p.curNavEpochMsg != nil && p.mh != nil {
		p.curNavEpochMsg.Tag = Tag
		p.mh.NavEpoch(p.curNavEpochMsg, tRead)
	}
	p.curNavEpochMsg = nil
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
