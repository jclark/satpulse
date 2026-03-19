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
	mh             gpsprot.MsgHandler
	mgr            *gpsprot.NavEpochManager
	curNavEpoch      uint32               // current RunTime (0 means no epoch seen yet)
	curNavEpochMsg   *gpsprot.NavEpochMsg // accumulated NavEpochMsg for current epoch
	curNavEpochStart time.Time            // tRead of first message in current epoch
	pendingNav2Dop *casbin.Nav2Dop       // buffered until FlushNavEpoch (no TOW field)
	lastTimeGNSS   gpsprot.GNSS         // from most recent Nav2TimeUTC
	satAccum       satAccum             // satellite info accumulator
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
		p.curNavEpochMsg = &gpsprot.NavEpochMsg{}
		p.curNavEpochStart = tRead
	}
}

// FlushNavEpoch implements gpsprot.EpochFlusher.
func (p *PacketProcessor) FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority, gpsprot.MsgHandler) {
	p.satAccum.epochChange(p.mh, tRead)
	msg := p.curNavEpochMsg
	p.curNavEpochMsg = nil
	if p.pendingNav2Dop != nil && msg != nil {
		dopNav2Dop(msg, p.pendingNav2Dop)
	}
	p.pendingNav2Dop = nil
	if msg != nil {
		msg.Tag = Tag
	}
	return msg, gpsprot.PriVendorLow, p.mh
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

func (p *PacketProcessor) dispatch(m casbin.Msg, tRead time.Time) bool {
	switch mt := m.(type) {
	case *casbin.NavSol:
		tm := timeNavSol(mt)
		posE := posECEFNavSol(p.curNavEpochMsg, mt)
		velE := velECEFNavSol(p.curNavEpochMsg, mt)
		if p.mh != nil {
			if tm != nil {
				tm.Tag = Tag
				if ne := p.curNavEpochMsg; ne != nil {
					tm.ReadDelay = gpsprot.Duration(tRead.Sub(p.curNavEpochStart))
				}
				p.mh.Time(tm, tRead)
			}
			if posE != nil {
				posE.Tag = Tag
				posE.Priority = gpsprot.PriVendorLow
				p.mh.PosECEF(posE, tRead)
			}
			if velE != nil {
				velE.Tag = Tag
				velE.Priority = gpsprot.PriVendorLow
				p.mh.VelECEF(velE, tRead)
			}
		}
		return true
	case *casbin.NavTimeUTC:
		tm := timeNavTimeUTC(mt)
		if tm == nil {
			return false
		}
		if p.mh != nil {
			tm.Tag = Tag
			if ne := p.curNavEpochMsg; ne != nil {
				tm.ReadDelay = gpsprot.Duration(tRead.Sub(p.curNavEpochStart))
			}
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
	case *casbin.NavPv:
		posG := posGeoNavPv(p.curNavEpochMsg, mt)
		velG := velGeoNavPv(p.curNavEpochMsg, mt)
		if posG == nil && velG == nil {
			return false
		}
		if p.mh != nil {
			if posG != nil {
				posG.Tag = Tag
				posG.Priority = gpsprot.PriVendorLow
				p.mh.PosGeo(posG, tRead)
			}
			if velG != nil {
				velG.Tag = Tag
				velG.Priority = gpsprot.PriVendorLow
				p.mh.VelGeo(velG, tRead)
			}
		}
		return true
	case *casbin.NavDop:
		dopNavDop(p.curNavEpochMsg, mt)
		return true
	case *casbin.Nav2TimeUTC:
		tm := timeNav2TimeUTC(mt)
		if tm == nil {
			return false
		}
		p.lastTimeGNSS = tm.GNSS
		if p.mh != nil {
			tm.Tag = Tag
			if ne := p.curNavEpochMsg; ne != nil {
				tm.ReadDelay = gpsprot.Duration(tRead.Sub(p.curNavEpochStart))
			}
			p.mh.Time(tm, tRead)
		}
		return true
	case *casbin.Nav2Sol:
		tm := timeNav2Sol(mt, p.lastTimeGNSS)
		posE := posECEFNav2Sol(p.curNavEpochMsg, mt)
		velE := velECEFNav2Sol(p.curNavEpochMsg, mt)
		if p.mh != nil {
			if tm != nil {
				tm.Tag = Tag
				if ne := p.curNavEpochMsg; ne != nil {
					tm.ReadDelay = gpsprot.Duration(tRead.Sub(p.curNavEpochStart))
				}
				p.mh.Time(tm, tRead)
			}
			if posE != nil {
				posE.Tag = Tag
				posE.Priority = gpsprot.PriVendorLow
				p.mh.PosECEF(posE, tRead)
			}
			if velE != nil {
				velE.Tag = Tag
				velE.Priority = gpsprot.PriVendorLow
				p.mh.VelECEF(velE, tRead)
			}
		}
		return true
	case *casbin.Nav2Pvh:
		posG := posGeoNav2Pvh(p.curNavEpochMsg, mt)
		velG := velGeoNav2Pvh(p.curNavEpochMsg, mt)
		if posG == nil && velG == nil {
			return false
		}
		if p.mh != nil {
			if posG != nil {
				posG.Tag = Tag
				posG.Priority = gpsprot.PriVendorHigh
				p.mh.PosGeo(posG, tRead)
			}
			if velG != nil {
				velG.Tag = Tag
				velG.Priority = gpsprot.PriVendorHigh
				p.mh.VelGeo(velG, tRead)
			}
		}
		return true
	case *casbin.Nav2Dop:
		p.pendingNav2Dop = mt
		return true
	case *casbin.Nav2Sig:
		msg := satsNav2Sig(mt)
		if msg != nil {
			if ne := p.curNavEpochMsg; ne != nil {
				ne.GNSSUsed |= msg.GNSSUsed()
				ne.BandsUsed |= msg.BandsUsed()
			}
			if p.mh != nil {
				p.mh.Satellites(msg, tRead)
			}
		}
		corrFromNav2Sig(p.curNavEpochMsg, mt)
		return true
	default:
		return false
	}
}
