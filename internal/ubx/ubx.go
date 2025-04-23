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
	mh  gpsprot.MsgHandler
}

// NewPacketProcessor creates a new UBX packet processor
func NewPacketProcessor() *PacketProcessor {
	return &PacketProcessor{}
}

// ProcessPacket processes a UBX packet's data and returns the message ID and any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	m, err := bin.ParseMsg(data)
	msgID := m.ID().String()
	if err != nil {
		return msgID, err
	}
	if Dispatch(m, tRead, p.mh) {
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

func Dispatch(m bin.Msg, tRead time.Time, h gpsprot.MsgHandler) bool {
	var time *gpsprot.TimeMsg
	var sv *gpsprot.SurveyMsg
	var sats *gpsprot.SatellitesMsg
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
		sats = satellitesNavSat(mt)
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
