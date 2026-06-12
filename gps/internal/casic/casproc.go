package casic

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/ptime"
)

// PacketProcessor implements the gpsprot.PacketProcessor interface for CASIC binary packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh  gpsprot.MsgHandler
	mgr *gpsprot.NavEpochManager
	// Navigation epoch tracking: CASIC navigation messages carry a RunTime
	// value that identifies which navigation solution they belong to.
	// Invariant: curNavEpochMsg is non-nil iff curNavEpoch is non-zero.
	curNavEpoch      uint32               // current RunTime (0 means no epoch seen yet)
	curNavEpochMsg   *gpsprot.NavEpochMsg // accumulated NavEpochMsg for current epoch
	curNavEpochStart time.Time            // tRead of first message in current epoch
	pendingNav2Dop   *casbin.Nav2Dop      // buffered until FlushNavEpoch (no TOW field)
	lastTimeGNSS     gpsprot.GNSS         // from most recent valid Nav2TimeUTC
	satAccum         satAccum             // satellite info accumulator
}

var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

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
	p.curNavEpoch = 0
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
		p.emitTime(timeNavSol(mt), tRead)
		p.emitPosECEF(posECEFNavSol(p.curNavEpochMsg, mt), gpsprot.PriVendorLow, tRead)
		p.emitVelECEF(velECEFNavSol(p.curNavEpochMsg, mt), gpsprot.PriVendorLow, tRead)
		return true
	case *casbin.NavTimeUTC:
		p.emitTime(timeNavTimeUTC(mt), tRead)
		return true
	case *casbin.TimTP:
		p.emitTime(timeTimTP(mt), tRead)
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
		p.emitPosGeo(posG, gpsprot.PriVendorLow, tRead)
		p.emitVelGeo(velG, gpsprot.PriVendorLow, tRead)
		return true
	case *casbin.NavDop:
		dopNavDop(p.curNavEpochMsg, mt)
		return true
	case *casbin.Nav2TimeUTC:
		tm := timeNav2TimeUTC(mt)
		if tm.GNSS != 0 {
			p.lastTimeGNSS = tm.GNSS
		}
		p.emitTime(tm, tRead)
		return true
	case *casbin.Nav2Sol:
		p.emitTime(timeNav2Sol(mt, p.lastTimeGNSS), tRead)
		p.emitPosECEF(posECEFNav2Sol(p.curNavEpochMsg, mt), gpsprot.PriVendorLow, tRead)
		p.emitVelECEF(velECEFNav2Sol(p.curNavEpochMsg, mt), gpsprot.PriVendorLow, tRead)
		return true
	case *casbin.Nav2Pvh:
		posG := posGeoNav2Pvh(p.curNavEpochMsg, mt)
		velG := velGeoNav2Pvh(p.curNavEpochMsg, mt)
		if posG == nil && velG == nil {
			return false
		}
		p.emitPosGeo(posG, gpsprot.PriVendorHigh, tRead)
		p.emitVelGeo(velG, gpsprot.PriVendorHigh, tRead)
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
	case *casbin.Tim2Ls:
		p.emitLeap(leapTim2Ls(mt, tRead), tRead)
		return true
	case *casbin.Tim2TimePos:
		if p.mh != nil {
			p.mh.Survey(surveyTim2TimePos(mt), tRead)
		}
		return true
	case *casbin.MsgGPSUTC:
		p.emitLeap(leapMsgUTC(mt.Dtls, mt.Dtlsf, mt.Wnlsf, mt.Dn, mt.Valid,
			gpsprot.GPS, ptime.TAIMinusGPS, 1, tRead), tRead)
		return true
	case *casbin.MsgBDSUTC:
		p.emitLeap(leapMsgUTC(mt.Dtls, mt.Dtlsf, mt.Wnlsf, mt.Dn, mt.Valid,
			gpsprot.BDS, ptime.TAIMinusBeiDou, 0, tRead), tRead)
		return true
	case *casbin.Tim2Tpx:
		p.emitTime(timeTim2Tpx(mt), tRead)
		return true
	case *casbin.Tim2TimeGPS:
		return p.dispatchTim2Time(&mt.Tim2TimeGNSS, gpsprot.GPS, ptime.GPS, ptime.TAIMinusGPS, "TIM2-TIMEGPS", tRead)
	case *casbin.Tim2TimeBDS:
		return p.dispatchTim2Time(&mt.Tim2TimeGNSS, gpsprot.BDS, ptime.BeiDou, ptime.TAIMinusBeiDou, "TIM2-TIMEBDS", tRead)
	case *casbin.Tim2TimeGLN:
		// GLONASS time tracks UTC; use dedicated conversion that produces UTCTime
		p.emitTime(timeTim2TimeGLN(&mt.Tim2TimeGNSS), tRead)
		return true
	case *casbin.Tim2TimeGAL:
		return p.dispatchTim2Time(&mt.Tim2TimeGNSS, gpsprot.GAL, ptime.Galileo, ptime.TAIMinusGalileo, "TIM2-TIMEGAL", tRead)
	default:
		return false
	}
}

func (p *PacketProcessor) dispatchTim2Time(m *casbin.Tim2TimeGNSS, gnss gpsprot.GNSS, toTAI func(int16, time.Duration) ptime.Time, taiMinusGNSS int16, msgID string, tRead time.Time) bool {
	// Dispatch leap second before time so consumers have it available
	p.emitLeap(leapTim2TimeGNSS(m, gnss, taiMinusGNSS), tRead)
	p.emitTime(timeTim2TimeGNSS(m, gnss, toTAI, taiMinusGNSS, msgID), tRead)
	return true
}

// emitTime stamps and delivers a time message, recording how long after
// the start of the current navigation epoch it was read.
func (p *PacketProcessor) emitTime(tm *gpsprot.TimeMsg, tRead time.Time) {
	if tm == nil || p.mh == nil {
		return
	}
	tm.Tag = Tag
	if p.curNavEpochMsg != nil {
		tm.ReadDelay = gpsprot.Duration(tRead.Sub(p.curNavEpochStart))
	}
	p.mh.Time(tm, tRead)
}

// emitLeap delivers a leap second message when one was extracted.
func (p *PacketProcessor) emitLeap(ls *gpsprot.LeapSecondMsg, tRead time.Time) {
	if ls == nil || p.mh == nil {
		return
	}
	p.mh.LeapSecond(ls, tRead)
}

// emitPosECEF delivers an ECEF position message at the given priority.
func (p *PacketProcessor) emitPosECEF(m *gpsprot.PosECEFMsg, pri gpsprot.MsgPriority, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	m.Tag = Tag
	m.Priority = pri
	p.mh.PosECEF(m, tRead)
}

// emitVelECEF delivers an ECEF velocity message at the given priority.
func (p *PacketProcessor) emitVelECEF(m *gpsprot.VelECEFMsg, pri gpsprot.MsgPriority, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	m.Tag = Tag
	m.Priority = pri
	p.mh.VelECEF(m, tRead)
}

// emitPosGeo delivers a geodetic position message at the given priority.
func (p *PacketProcessor) emitPosGeo(m *gpsprot.PosGeoMsg, pri gpsprot.MsgPriority, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	m.Tag = Tag
	m.Priority = pri
	p.mh.PosGeo(m, tRead)
}

// emitVelGeo delivers a geodetic velocity message at the given priority.
func (p *PacketProcessor) emitVelGeo(m *gpsprot.VelGeoMsg, pri gpsprot.MsgPriority, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	m.Tag = Tag
	m.Priority = pri
	p.mh.VelGeo(m, tRead)
}
