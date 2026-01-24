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
	mh          gpsprot.MsgHandler
	curNavEpoch uint32   // current RunTime (0 means no epoch seen yet)
	satAccum    satAccum // satellite info accumulator
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
	if nm, ok := m.(bin.NavMsg); ok {
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

// handleNavEpoch tracks navigation epochs via the RunTime field.
// When a new epoch is detected, it flushes any accumulated satellite data.
func (p *PacketProcessor) handleNavEpoch(nm bin.NavMsg, tRead time.Time) {
	e := nm.NavEpoch()
	if e != p.curNavEpoch {
		if p.curNavEpoch != 0 {
			p.flushNavEpoch(tRead)
		}
		p.curNavEpoch = e
	}
}

func (p *PacketProcessor) flushNavEpoch(tRead time.Time) {
	p.satAccum.epochChange(p.mh, tRead)
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

func (p *PacketProcessor) dispatch(m bin.Msg, tRead time.Time) bool {
	switch mt := m.(type) {
	case *bin.NavSol:
		tm := timeNavSol(mt)
		if tm == nil {
			return false
		}
		if p.mh != nil {
			tm.Tag = Tag
			p.mh.Time(tm, tRead)
		}
		return true
	case *bin.NavTimeUTC:
		tm := timeNavTimeUTC(mt)
		if tm == nil {
			return false
		}
		if p.mh != nil {
			tm.Tag = Tag
			p.mh.Time(tm, tRead)
		}
		return true
	case *bin.TimTP:
		tm := timeTimTP(mt)
		if tm == nil {
			return false
		}
		if p.mh != nil {
			tm.Tag = Tag
			p.mh.Time(tm, tRead)
		}
		return true
	case *bin.NavGPSInfo:
		p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
		return true
	case *bin.NavBDSInfo:
		p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
		return true
	case *bin.NavGLNInfo:
		p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
		return true
	default:
		return false
	}
}
