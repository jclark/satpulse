package ubx

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ubx/bin"
)

// Tag is the identifier for UBX protocol packets
const Tag gpsprot.Tag = "UBX"

// Protocol-specific handler
type ProtHandler interface {
	UBX(msg bin.Msg, tRead time.Time)
}

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

// ProcessPacket processes a UBX packet's data and returns any error
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) error {
	m, err := bin.ParseMsg(data)
	if err != nil {
		return err
	}
	if Dispatch(m, tRead, p.mh) {
		return nil
	}
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		nmh.NativeMsg(Tag, m.ID().String(), m, tRead)
	}
	return nil
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// CreatePacketExchanger creates a PacketExchanger if configuration is supported
// Returns nil if configuration is not supported
func (p *PacketProcessor) CreatePacketExchanger() gpsprot.PacketExchanger {
	// Will implement packet exchanger later
	return nil
}

func Dispatch(m bin.Msg, tRead time.Time, h gpsprot.MsgHandler) bool {
	var time *gpsprot.TimeMsg
	var sv *gpsprot.SurveyMsg
	switch mt := m.(type) {
	case *bin.NavSat:
		sats := satellitesNavSat(mt)
		if sats == nil {
			return false
		}
		if h != nil {
			h.Satellites(sats, tRead)
		}
		return true
	case *bin.NavTimeLS:
		ls := leapSecond(mt)
		if ls == nil {
			return false
		}
		if h != nil {
			h.LeapSecond(ls, tRead)
		}
		return true
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
	if time == nil && sv == nil {
		return false
	}
	if h != nil {
		if sv != nil {
			h.Survey(sv, tRead)
		} else {
			h.Time(time, tRead)
		}
	}
	return true
}
