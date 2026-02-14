package ubx

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// Tag is the identifier for UBX protocol packets
const Tag gpsprot.Tag = "UBX"

// Ensure PacketProcessor implements gpsprot.PacketProcessor
var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// PacketProcessor implements the gpsprot.PacketProcessor interface for UBX packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
	// Navigation epoch tracking: UBX navigation messages (NAV-*) contain an iTOW field
	// that identifies which navigation solution they belong to. Messages with the same
	// iTOW are part of the same epoch and can be grouped together.
	curNavEpoch    uint32              // Current navigation epoch (iTOW + 1 to handle zero)
	curNavMsgs     byteSet             // NAV class message IDs seen in current epoch
	prevNavMsgs    byteSet             // NAV class message IDs seen in previous epoch
	nEpochsSeen    int                 // Number of distinct epochs seen
	curNavEpochMsg gpsprot.NavEpochMsg // accumulated NavEpochMsg for current epoch
	// Satellite message accumulation: NAV-SAT and NAV-SIG messages are combined
	// into a single SatellitesMsg when both are available
	satMsg      *gpsprot.SatellitesMsg
	sigMsg      *gpsprot.SatellitesMsg
	satSigTRead time.Time // Timestamp of first message in the pair
}

// NewPacketProcessor creates a new UBX packet processor
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{}
}

// ProcessPacket processes a UBX packet's data and returns the message ID and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	m, err := ubxbin.ParseMsg(data)
	if err != nil {
		return PacketFormat.MsgID([]byte(data)), err
	}
	msgID := m.ID().String()
	if nm, ok := m.(ubxbin.NavMsg); ok {
		p.handleNavEpoch(nm, tRead)
	}
	if p.Dispatch(m, tRead) {
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

// handleNavEpoch tracks which messages we receive for each navigation epoch.
// This allows us to know whether we should wait for additional messages
// (e.g., wait for NAV-SIG if we saw it in the previous epoch).
func (p *PacketProcessor) handleNavEpoch(nm ubxbin.NavMsg, tRead time.Time) {
	e := nm.NavEpoch()
	e++ // use zero to represent invalid epoch
	if e != p.curNavEpoch {
		// New epoch starting - flush any pending messages from previous epoch
		p.flushNavEpoch(tRead)
		p.curNavEpoch = e
		p.curNavEpochMsg.StartTime = tRead
		p.prevNavMsgs = p.curNavMsgs
		p.curNavMsgs.clear()
		p.nEpochsSeen++
	}
	_, id := nm.ID().Unpack()
	p.curNavMsgs.add(id)
}

func (p *PacketProcessor) flushNavEpoch(tRead time.Time) {
	p.flushSats()
	if p.curNavEpoch != 0 && p.mh != nil {
		ne := p.curNavEpochMsg
		ne.Tag = Tag
		p.mh.NavEpoch(&ne, tRead)
	}
	p.curNavEpochMsg = gpsprot.NavEpochMsg{}
}

// maybeFlushSats decides whether to emit a SatellitesMsg or wait for more data.
// If we have one but not both of NAV-SAT and NAV-SIG, we decide whether to wait
// for the other based on whether the missing message was seen in the previous epoch.
func (p *PacketProcessor) maybeFlushSats() {
	var missing ubxbin.MsgID
	if p.satMsg == nil {
		if p.sigMsg == nil {
			return
		}
		missing = ubxbin.NavSatID
	} else if p.sigMsg == nil {
		missing = ubxbin.NavSigID
	} else {
		p.flushSats()
	}
	if p.nEpochsSeen < 3 {
		// We need to see at least 3 epochs before we can make reliable decisions:
		// - Epoch 1: May be incomplete (we might have started listening mid-epoch)
		// - Epoch 2: First complete epoch, but prevNavMsgs still contains epoch 1
		// - Epoch 3: Now prevNavMsgs contains the complete epoch 2, so we can
		//   use it to decide whether to wait for NAV-SIG or NAV-SAT
		return
	}
	_, missingNavMsg := missing.Unpack()
	// If the prevNavMsgs contains the missing message, then we should expect it in this epoch, so wait
	if p.prevNavMsgs.contains(missingNavMsg) {
		return
	}
	p.flushSats()
}

// flushSats combines accumulated NAV-SAT and NAV-SIG messages into a single
// SatellitesMsg and sends it to the message handler.
func (p *PacketProcessor) flushSats() {
	satMsg := p.satMsg
	sigMsg := p.sigMsg
	if satMsg == nil && sigMsg == nil {
		return // nothing to flush
	}
	p.satMsg = nil
	p.sigMsg = nil
	p.mh.Satellites(satellitesCombine(satMsg, sigMsg), p.satSigTRead)
	p.satSigTRead = time.Time{}
}

func (p *PacketProcessor) Dispatch(m ubxbin.Msg, tRead time.Time) bool {
	var time *gpsprot.TimeMsg
	var sv *gpsprot.SurveyMsg
	var sats *gpsprot.SatellitesMsg
	var posG *gpsprot.PosGeoMsg
	var posE *gpsprot.PosECEFMsg
	var velG *gpsprot.VelGeoMsg
	var velE *gpsprot.VelECEFMsg
	h := p.mh
	switch mt := m.(type) {
	case *ubxbin.NavTimeLS:
		ls := leapSecond(mt)
		if ls == nil {
			return false
		}
		if h != nil {
			h.LeapSecond(ls, tRead)
		}
		return true
	case *ubxbin.NavSat:
		p.satMsg = satellitesNavSat(mt)
		if p.satSigTRead.IsZero() {
			p.satSigTRead = tRead
		}
		p.maybeFlushSats()
		return true
	case *ubxbin.NavSig:
		p.sigMsg = satellitesNavSig(mt)
		if p.satSigTRead.IsZero() {
			p.satSigTRead = tRead
		}
		p.maybeFlushSats()
		return true
	case *ubxbin.NavSVInfo:
		sats = satellitesNavSVInfo(mt)
	case *ubxbin.NavPosECEF:
		posE = posECEFNavPosECEF(mt)
		navEpochNavPosECEF(&p.curNavEpochMsg, mt)
	case *ubxbin.NavVelECEF:
		velE = velECEFNavVelECEF(mt)
		navEpochNavVelECEF(&p.curNavEpochMsg, mt)
	case *ubxbin.NavPosLLH:
		posG = posGeoNavPosLLH(mt)
		navEpochNavPosLLH(&p.curNavEpochMsg, mt)
	case *ubxbin.NavVelNED:
		velG = velGeoNavVelNED(mt)
		navEpochNavVelNED(&p.curNavEpochMsg, mt)
	case *ubxbin.NavTimeGPS:
		time = timeNavTimeGPS(mt)
		navEpochTimeAcc(&p.curNavEpochMsg, mt.TAcc)
	case *ubxbin.NavTimeBDS:
		time = timeNavTimeBDS(mt)
		navEpochTimeAcc(&p.curNavEpochMsg, mt.TAcc)
	case *ubxbin.NavTimeGal:
		time = timeNavTimeGal(mt)
		navEpochTimeAcc(&p.curNavEpochMsg, mt.TAcc)
	case *ubxbin.NavTimeGLO:
		time = timeNavTimeGLO(mt)
		navEpochTimeAcc(&p.curNavEpochMsg, mt.TAcc)
	case *ubxbin.NavTimeUTC:
		time = timeNavTimeUTC(mt)
		navEpochTimeAcc(&p.curNavEpochMsg, mt.TAcc)
	case *ubxbin.NavPVT:
		time = timeNavPVT(mt)
		posG = posGeoNavPVT(mt)
		velG = velGeoNavPVT(mt)
		navEpochNavPVT(&p.curNavEpochMsg, mt)
	case *ubxbin.TimTP:
		time = timeTimTP(mt)
	case *ubxbin.TimTos:
		time = timeTimTos(mt)
	case *ubxbin.NavSvin:
		sv = surveyNavSvin(mt)
	case *ubxbin.TimSvin:
		sv = surveyTimSvin(mt)
	default:
		return false
	}
	if time == nil && sv == nil && sats == nil && posG == nil && posE == nil && velG == nil && velE == nil {
		return false
	}
	if h != nil {
		if sats != nil {
			h.Satellites(sats, tRead)
		} else if sv != nil {
			h.Survey(sv, tRead)
		} else if time != nil {
			time.Tag = Tag
			h.Time(time, tRead)
		}
		if posG != nil {
			posG.Tag = Tag
			h.PosGeo(posG, tRead)
		} else if posE != nil {
			posE.Tag = Tag
			h.PosECEF(posE, tRead)
		}
		if velG != nil {
			velG.Tag = Tag
			h.VelGeo(velG, tRead)
		} else if velE != nil {
			velE.Tag = Tag
			h.VelECEF(velE, tRead)
		}
	}
	return true
}
