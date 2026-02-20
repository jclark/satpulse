package casic

import (
	"time"

	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// Ensure PacketProcessor implements gpsprot.PacketProcessor
var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// PacketProcessor implements the gpsprot.PacketProcessor interface for CASIC binary packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh          gpsprot.MsgHandler
	mgr         *gpsprot.NavEpochManager
	curNavEpoch uint32   // current RunTime (0 means no epoch seen yet)
	satAccum    satAccum // satellite info accumulator
}

// NewPacketProcessor creates a new CASIC binary packet processor
func NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor {
	return &PacketProcessor{mgr: mgr}
}

// ProcessPacket processes a CASIC binary packet's data and returns the message ID and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	m, err := casbin.ParseMsg(data)
	if err != nil {
		return PacketFormat.MsgID([]byte(data)), err
	}
	msgID := m.ID().String()
	if nm, ok := m.(casbin.NavMsg); ok {
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
func (p *PacketProcessor) handleNavEpoch(nm casbin.NavMsg, tRead time.Time) {
	e := nm.NavEpoch()
	e++ // use zero to represent "no epoch seen yet"
	if e != p.curNavEpoch {
		p.mgr.EpochStarted(p, tRead)
		p.curNavEpoch = e
	}
}

// FlushNavEpoch implements gpsprot.EpochFlusher.
func (p *PacketProcessor) FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority, gpsprot.MsgHandler) {
	p.satAccum.epochChange(p.mh, tRead)
	return nil, gpsprot.PriVendorLow, p.mh
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

func (p *PacketProcessor) dispatch(m casbin.Msg, tRead time.Time) bool {
	switch mt := m.(type) {
	case *casbin.NavSol:
		tm := timeNavSol(mt)
		if tm == nil {
			return false
		}
		if p.mh != nil {
			tm.Tag = Tag
			tm.Priority = gpsprot.PriVendorLow
			p.mh.Time(tm, tRead)
		}
		return true
	case *casbin.NavTimeUTC:
		tm := timeNavTimeUTC(mt)
		if tm == nil {
			return false
		}
		if p.mh != nil {
			tm.Tag = Tag
			tm.Priority = gpsprot.PriVendorLow
			p.mh.Time(tm, tRead)
		}
		return true
	case *casbin.TimTP:
		tm := timeTimTP(mt)
		if tm == nil {
			return false
		}
		if p.mh != nil {
			tm.Tag = Tag
			tm.Priority = gpsprot.PriVendorLow
			p.mh.Time(tm, tRead)
		}
		return true
	case *casbin.NavGPSInfo:
		p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
		return true
	case *casbin.NavBDSInfo:
		p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
		return true
	case *casbin.NavGLNInfo:
		p.satAccum.accum(&mt.NavSatInfoFixed, mt.SVs, p.mh, tRead)
		return true
	default:
		return false
	}
}
