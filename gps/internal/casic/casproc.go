package casic

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/ptime"
)

// Ensure PacketProcessor implements gpsprot.PacketProcessor
var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

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
	lastTimeGNSS     gpsprot.GNSS         // from most recent Nav2TimeUTC
	lastReceiverTime receiverTime         // most recent receiver-derived time reference
	satAccum         satAccum             // satellite info accumulator
}

type receiverTime struct {
	tRead time.Time
	tai   ptime.Time
}

const tim2LsNowMaxAge = time.Hour

func (t receiverTime) recent(tRead time.Time) (ptime.Time, bool) {
	age := tRead.Sub(t.tRead)
	if t.tai.IsZero() || age < 0 || age > tim2LsNowMaxAge {
		return 0, false
	}
	return t.tai, true
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
		tm := timeNavSol(mt)
		posE := posECEFNavSol(p.curNavEpochMsg, mt)
		velE := velECEFNavSol(p.curNavEpochMsg, mt)
		p.dispatchTime(tm, tRead, true)
		if p.mh != nil {
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
		p.dispatchTime(tm, tRead, true)
		return true
	case *casbin.TimTP:
		tm := timeTimTP(mt)
		if tm == nil {
			return false
		}
		p.dispatchTime(tm, tRead, false)
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
		p.dispatchTime(tm, tRead, true)
		return true
	case *casbin.Nav2Sol:
		tm := timeNav2Sol(mt, p.lastTimeGNSS)
		posE := posECEFNav2Sol(p.curNavEpochMsg, mt)
		velE := velECEFNav2Sol(p.curNavEpochMsg, mt)
		p.dispatchTime(tm, tRead, true)
		if p.mh != nil {
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
	case *casbin.Tim2Tpx:
		tm := timeTim2Tpx(mt)
		p.dispatchTime(tm, tRead, true)
		return true
	case *casbin.Tim2TimeGPS:
		return p.dispatchTim2Time(&mt.Tim2TimeGNSS, gpsprot.GPS, ptime.GPS, ptime.TAIMinusGPS, "TIM2-TIMEGPS", tRead)
	case *casbin.Tim2TimeBDS:
		return p.dispatchTim2Time(&mt.Tim2TimeGNSS, gpsprot.BDS, ptime.BeiDou, ptime.TAIMinusBeiDou, "TIM2-TIMEBDS", tRead)
	case *casbin.Tim2TimeGLN:
		// GLONASS time tracks UTC; use dedicated conversion that produces UTCTime
		tm := timeTim2TimeGLN(&mt.Tim2TimeGNSS)
		p.dispatchTime(tm, tRead, true)
		return true
	case *casbin.Tim2TimeGAL:
		return p.dispatchTim2Time(&mt.Tim2TimeGNSS, gpsprot.GAL, ptime.Galileo, ptime.TAIMinusGalileo, "TIM2-TIMEGAL", tRead)
	case *casbin.Tim2Ls:
		if now, ok := p.lastReceiverTime.recent(tRead); ok {
			ls := leapTim2Ls(mt, now)
			if ls != nil && p.mh != nil {
				p.mh.LeapSecond(ls, tRead)
			}
		}
		return true
	case *casbin.Tim2TimePos:
		if p.mh != nil {
			p.mh.Survey(surveyTim2TimePos(mt), tRead)
		}
		return true
	default:
		return false
	}
}

func (p *PacketProcessor) dispatchTim2Time(m *casbin.Tim2TimeGNSS, gnss gpsprot.GNSS, toTAI func(int16, time.Duration) ptime.Time, taiMinusGNSS int16, msgID string, tRead time.Time) bool {
	// Dispatch leap second before time so consumers have it available
	ls := leapTim2TimeGNSS(m, gnss, taiMinusGNSS)
	if ls != nil && p.mh != nil {
		p.mh.LeapSecond(ls, tRead)
	}
	tm := timeTim2TimeGNSS(m, gnss, toTAI, taiMinusGNSS, msgID)
	p.dispatchTime(tm, tRead, true)
	return true
}

func (p *PacketProcessor) dispatchTime(tm *gpsprot.TimeMsg, tRead time.Time, readDelay bool) {
	if tm == nil {
		return
	}
	if tai, ok := tm.ComputeTAITime(ptime.LeapSecond2016()); ok {
		p.lastReceiverTime = receiverTime{tRead: tRead, tai: tai}
	}
	if p.mh == nil {
		return
	}
	tm.Tag = Tag
	if readDelay {
		if ne := p.curNavEpochMsg; ne != nil {
			tm.ReadDelay = gpsprot.Duration(tRead.Sub(p.curNavEpochStart))
		}
	}
	p.mh.Time(tm, tRead)
}
