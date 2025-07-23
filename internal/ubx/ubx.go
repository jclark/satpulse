package ubx

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

// Tag is the identifier for UBX protocol packets
const Tag gpsprot.Tag = "UBX"

// Ensure PacketProcessor implements gpsprot.PacketProcessor
var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// PacketProcessor implements the gpsprot.PacketProcessor interface for UBX packets
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh                gpsprot.MsgHandler
	// Navigation epoch tracking: UBX navigation messages (NAV-*) contain an iTOW field
	// that identifies which navigation solution they belong to. Messages with the same
	// iTOW are part of the same epoch and can be grouped together.
	curNavEpoch       uint32    // Current navigation epoch (iTOW + 1 to handle zero)
	curNavMsgs        byteSet   // NAV class message IDs seen in current epoch
	prevNavMsgs       byteSet   // NAV class message IDs seen in previous epoch
	curNavEpochTStart time.Time // When current epoch started
	nEpochsSeen       int       // Number of distinct epochs seen
	// Satellite message accumulation: NAV-SAT and NAV-SIG messages are combined
	// into a single SatellitesMsg when both are available
	satMsg            *gpsprot.SatellitesMsg
	sigMsg            *gpsprot.SatellitesMsg
	satSigTRead       time.Time // Timestamp of first message in the pair
}

// NewPacketProcessor creates a new UBX packet processor
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{}
}

// ProcessPacket processes a UBX packet's data and returns the message ID and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return PacketFormat.MsgID([]byte(data)), err
	}
	msgID := m.ID().String()
	if nm, ok := m.(bin.NavMsg); ok {
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

// CreatePacketExchanger creates a PacketExchanger if configuration is supported
// Returns nil if configuration is not supported
func (p *PacketProcessor) CreatePacketExchanger() gpsprot.PacketExchanger {
	px := newPacketExchanger(p.GetNativeMsgHandler())
	p.SetNativeMsgHandler(px)
	return px
}

// handleNavEpoch tracks which messages we receive for each navigation epoch.
// This allows us to know whether we should wait for additional messages
// (e.g., wait for NAV-SIG if we saw it in the previous epoch).
func (p *PacketProcessor) handleNavEpoch(nm bin.NavMsg, tRead time.Time) {
	e := nm.NavEpoch()
	e++ // use zero to represent invalid epoch
	if e != p.curNavEpoch {
		// New epoch starting - flush any pending messages from previous epoch
		p.flushNavEpoch()
		p.curNavEpoch = e
		p.curNavEpochTStart = tRead
		p.prevNavMsgs = p.curNavMsgs
		p.curNavMsgs.clear()
		p.nEpochsSeen++
	}
	_, id := nm.ID().Unpack()
	p.curNavMsgs.add(id)
}

func (p *PacketProcessor) flushNavEpoch() {
	p.flushSats()
}

// maybeFlushSats decides whether to emit a SatellitesMsg or wait for more data.
// If we have one but not both of NAV-SAT and NAV-SIG, we decide whether to wait
// for the other based on whether the missing message was seen in the previous epoch.
func (p *PacketProcessor) maybeFlushSats() {
	var missing bin.MsgID 
	if p.satMsg == nil {
		if p.sigMsg == nil {
			return
		}
		missing = bin.NavSatID
	} else if p.sigMsg == nil {
		missing = bin.NavSigID
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

func (p *PacketProcessor) Dispatch(m bin.Msg, tRead time.Time) bool {
	var time *gpsprot.TimeMsg
	var sv *gpsprot.SurveyMsg
	var sats *gpsprot.SatellitesMsg
	h := p.mh
	switch mt := m.(type) {
	case *bin.NavTimeLS:
		ls := leapSecond(mt)
		if ls == nil {
			return false
		}
		if h != nil {
			h.LeapSecond(ls, tRead)
		}
		return true
	case *bin.NavSat:
		p.satMsg = satellitesNavSat(mt)
		if p.satSigTRead.IsZero() {
			p.satSigTRead = tRead
		}
		p.maybeFlushSats()
		return true
	case *bin.NavSig:
		p.sigMsg = satellitesNavSig(mt)
		if p.satSigTRead.IsZero() {
			p.satSigTRead = tRead
		}
		p.maybeFlushSats()
		return true
	case *bin.NavSVInfo:
		sats = satellitesNavSVInfo(mt)
	case *bin.NavTimeGPS:
		time = timeNavTimeGPS(mt)
	case *bin.NavTimeBDS:
		time = timeNavTimeBDS(mt)
	case *bin.NavTimeGal:
		time = timeNavTimeGal(mt)
	case *bin.NavTimeGLO:
		time = timeNavTimeGLO(mt)
	case *bin.NavTimeUTC:
		time = timeNavTimeUTC(mt)
	case *bin.NavPVT:
		time = timeNavPVT(mt)
	case *bin.TimTP:
		time = timeTimTP(mt)
	case *bin.TimTos:
		time = timeTimTos(mt)
	case *bin.NavSvin:
		sv = surveyNavSvin(mt)
	case *bin.TimSvin:
		sv = surveyTimSvin(mt)
	default:
		return false
	}
	if time == nil && sv == nil && sats == nil {
		return false
	}
	if h != nil {
		if sats != nil {
			h.Satellites(sats, tRead)
		} else if sv != nil {
			h.Survey(sv, tRead)
		} else {
			time.Tag = Tag
			h.Time(time, tRead)
		}
	}
	return true
}
