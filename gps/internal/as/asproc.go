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
	mh  gpsprot.MsgHandler
	mgr *gpsprot.NavEpochManager
	// Navigation epoch tracking: Allystar NAV messages contain an iTOW field
	// that identifies which navigation solution they belong to.
	// Invariant: curNavEpochMsg is non-nil iff curNavEpoch is non-zero.
	curNavEpoch    uint32              // current navigation epoch (iTOW + 1 to reserve zero for "no epoch")
	curNavEpochMsg *gpsprot.NavEpochMsg // accumulated NavEpochMsg for current epoch
	hadNavAuto     bool                 // true if NAV-AUTO has been seen in the current epoch
}

// NewPacketProcessor creates a new Allystar binary packet processor
func NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor {
	return &PacketProcessor{mgr: mgr}
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
	if !sameEpoch(e, p.curNavEpoch) {
		p.mgr.EpochStarted(p, tRead)
		p.curNavEpoch = e
		p.curNavEpochMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
		p.hadNavAuto = false
	}
}

// sameEpoch reports whether two epoch identifiers (iTOW+1) refer to the same
// navigation epoch. It allows a 1ms tolerance because Allystar NAV-TIMEUTC
// reports iTOW consistently 1ms less than other NAV messages in the same
// solution. The minimum epoch interval is 1000ms (1Hz), so a 1ms tolerance
// cannot merge distinct epochs.
func sameEpoch(a, b uint32) bool {
	if a > b {
		return a-b <= 1
	}
	return b-a <= 1
}

// FlushNavEpoch implements gpsprot.EpochFlusher.
func (p *PacketProcessor) FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority, gpsprot.MsgHandler) {
	msg := p.curNavEpochMsg
	p.curNavEpochMsg = nil
	if msg != nil {
		msg.Tag = Tag
	}
	return msg, gpsprot.PriVendorLow, p.mh
}

func (p *PacketProcessor) dispatch(m asbin.Msg, tRead time.Time) bool {
	var tm *gpsprot.TimeMsg
	var sv *gpsprot.SurveyMsg
	var sats *gpsprot.SatellitesMsg
	var posG *gpsprot.PosGeoMsg
	var posE *gpsprot.PosECEFMsg
	var velG *gpsprot.VelGeoMsg
	var velE *gpsprot.VelECEFMsg
	h := p.mh
	switch mt := m.(type) {
	case *asbin.NavPosEcef:
		posE = posEcefNavPosEcef(p.curNavEpochMsg, mt)
	case *asbin.NavVelEcef:
		velE = velEcefNavVelEcef(p.curNavEpochMsg, mt)
	case *asbin.NavPosLlh:
		posG = posGeoNavPosLlh(p.curNavEpochMsg, mt)
	case *asbin.NavVelNed:
		velG = velGeoNavVelNed(p.curNavEpochMsg, mt)
	case *asbin.NavTime:
		tm = timeNavTime(mt)
	case *asbin.NavTimeUTC:
		tm = timeNavTimeUTC(mt)
	case *asbin.NavSVInfo:
		sats = satellitesNavSVInfo(mt)
	case *asbin.NavSvin:
		sv = surveyNavSvin(mt)
	case *asbin.NavAuto:
		if p.hadNavAuto || p.curNavEpochMsg == nil {
			p.mgr.EpochStarted(p, tRead)
			p.curNavEpochMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
		}
		p.hadNavAuto = true
		qualityNavAuto(p.curNavEpochMsg, mt)
		posG = posGeoNavAuto(mt)
		velG = velGeoNavAuto(mt)
	case *asbin.NavDop:
		dopNavDop(p.curNavEpochMsg, mt)
		return true
	default:
		return false
	}
	if tm == nil && sv == nil && sats == nil && posG == nil && posE == nil && velG == nil && velE == nil {
		return false
	}
	if sats != nil {
		if ne := p.curNavEpochMsg; ne != nil {
			ne.GNSSUsed |= sats.GNSSUsed()
			ne.BandsUsed |= sats.BandsUsed()
		}
	}
	if h != nil {
		if sats != nil {
			h.Satellites(sats, tRead)
		} else if sv != nil {
			h.Survey(sv, tRead)
		} else if tm != nil {
			tm.Tag = Tag
			h.Time(tm, tRead)
		}
		if posG != nil {
			posG.Tag = Tag
			posG.Priority = gpsprot.PriVendorLow
			h.PosGeo(posG, tRead)
		} else if posE != nil {
			posE.Tag = Tag
			posE.Priority = gpsprot.PriVendorLow
			h.PosECEF(posE, tRead)
		}
		if velG != nil {
			velG.Tag = Tag
			velG.Priority = gpsprot.PriVendorLow
			h.VelGeo(velG, tRead)
		} else if velE != nil {
			velE.Tag = Tag
			velE.Priority = gpsprot.PriVendorLow
			h.VelECEF(velE, tRead)
		}
	}
	return true
}
