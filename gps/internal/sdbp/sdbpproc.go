package sdbp

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

var _ gpsprot.PacketProcessor = (*PacketProcessor)(nil)

// PacketProcessor implements gpsprot.PacketProcessor for SDBP packets.
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh             gpsprot.MsgHandler
	mgr            *gpsprot.NavEpochManager
	curNavEpoch    uint32               // current LocalTimestamp+1 (0 means no epoch seen)
	curNavEpochMsg *gpsprot.NavEpochMsg // accumulated NavEpochMsg for current epoch
}

// NewPacketProcessor creates a new SDBP packet processor.
func NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor {
	return &PacketProcessor{mgr: mgr}
}

// ProcessPacket processes an SDBP packet.
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	m, err := sdbpbin.ParseMsg(data)
	if err != nil {
		return PacketFormat.MsgID([]byte(data)), err
	}
	msgID := m.ID().String()
	if nm, ok := m.(sdbpbin.NavMsg); ok {
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

// handleNavEpoch tracks navigation epochs via the LocalTimestamp field.
func (p *PacketProcessor) handleNavEpoch(nm sdbpbin.NavMsg, tRead time.Time) {
	e := nm.NavEpoch()
	e++ // use zero to represent "no epoch seen yet"
	if e != p.curNavEpoch {
		p.mgr.EpochStarted(p, tRead)
		p.curNavEpoch = e
		p.curNavEpochMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
	}
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

// SetMsgHandler sets the handler for protocol-agnostic messages.
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

func (p *PacketProcessor) dispatch(m sdbpbin.Msg, tRead time.Time) bool {
	switch mt := m.(type) {
	case *sdbpbin.DatBDST:
		p.emitTime(timeDatBDST(mt), tRead)
	case *sdbpbin.DatGPST:
		p.emitTime(timeDatGPST(mt), tRead)
	case *sdbpbin.DatGALT:
		p.emitTime(timeDatGALT(mt), tRead)
	case *sdbpbin.DatTPPS:
		p.emitTime(timeDatTPPS(mt), tRead)
	case *sdbpbin.DatUTCT2:
		p.emitTime(timeDatUTCT2(mt), tRead)
		p.emitLeapSecond(leapSecondDatUTCT2(mt), tRead)
	case *sdbpbin.DatGPSU:
		p.emitLeapSecond(leapSecondDatGNSSU(&mt.DatGNSSU, gpsprot.GPS), tRead)
	case *sdbpbin.DatGALU:
		p.emitLeapSecond(leapSecondDatGNSSU(&mt.DatGNSSU, gpsprot.GAL), tRead)
	case *sdbpbin.DatBDSU:
		p.emitLeapSecond(leapSecondDatGNSSU(&mt.DatGNSSU, gpsprot.BDS), tRead)
	case *sdbpbin.DatLLA3:
		ne := p.curNavEpochMsg
		posG := posGeoDatLLA3(ne, mt)
		velG := velGeoDatLLA3(ne, mt)
		if h := p.mh; h != nil {
			if posG != nil {
				posG.Tag = Tag
				posG.Priority = gpsprot.PriVendorLow
				h.PosGeo(posG, tRead)
			}
			if velG != nil {
				velG.Tag = Tag
				velG.Priority = gpsprot.PriVendorLow
				h.VelGeo(velG, tRead)
			}
		}
	case *sdbpbin.DatECEF2:
		ne := p.curNavEpochMsg
		posE := posECEFDatECEF2(ne, mt)
		velE := velECEFDatECEF2(ne, mt)
		if h := p.mh; h != nil {
			if posE != nil {
				posE.Tag = Tag
				posE.Priority = gpsprot.PriVendorLow
				h.PosECEF(posE, tRead)
			}
			if velE != nil {
				velE.Tag = Tag
				velE.Priority = gpsprot.PriVendorLow
				h.VelECEF(velE, tRead)
			}
		}
	case *sdbpbin.DatNED3:
		velG := velGeoDatNED3(p.curNavEpochMsg, mt)
		if velG != nil {
			if h := p.mh; h != nil {
				velG.Tag = Tag
				velG.Priority = gpsprot.PriVendorLow
				h.VelGeo(velG, tRead)
			}
		}
	case *sdbpbin.DatDOP:
		dopDatDOP(p.curNavEpochMsg, mt)
	case *sdbpbin.DatSAT:
		msg := satsDatSAT(mt)
		if msg != nil {
			if ne := p.curNavEpochMsg; ne != nil {
				ne.GNSSUsed |= msg.GNSSUsed()
				ne.BandsUsed |= msg.BandsUsed()
			}
			if h := p.mh; h != nil {
				h.Satellites(msg, tRead)
			}
		}
	case *sdbpbin.DatTSURV:
		sm := surveyDatTSURV(mt)
		if sm != nil {
			if h := p.mh; h != nil {
				h.Survey(sm, tRead)
			}
		}
	default:
		return false
	}
	return true
}

// surveyDatTSURV converts DatTSURV to SurveyMsg.
// Returns nil when status is idle.
func surveyDatTSURV(m *sdbpbin.DatTSURV) *gpsprot.SurveyMsg {
	if m.Status == 0 {
		return nil
	}
	return &gpsprot.SurveyMsg{
		Position: gpsprot.Point3D{
			gpsprot.Length(m.AvgX) * gpsprot.Centimeter,
			gpsprot.Length(m.AvgY) * gpsprot.Centimeter,
			gpsprot.Length(m.AvgZ) * gpsprot.Centimeter,
		},
		Accuracy:   gpsprot.Length(math.Sqrt(float64(m.AvgVariance)*1e-4) * float64(gpsprot.Meter)),
		ObsTime:    gpsprot.Duration(m.ObsTime) * gpsprot.Second,
		ObsCount:   m.ObsCount,
		Valid:      m.Status == 2,
		InProgress: m.Status == 1,
	}
}

func (p *PacketProcessor) emitTime(tm *gpsprot.TimeMsg, tRead time.Time) {
	if tm == nil {
		return
	}
	if h := p.mh; h != nil {
		tm.Tag = Tag
		if ne := p.curNavEpochMsg; ne != nil && tm.Ref == gpsprot.NavSolution {
			tm.ReadDelay = gpsprot.Duration(tRead.Sub(ne.StartTime))
		}
		h.Time(tm, tRead)
	}
}

func (p *PacketProcessor) emitLeapSecond(lm *gpsprot.LeapSecondMsg, tRead time.Time) {
	if lm == nil {
		return
	}
	if h := p.mh; h != nil {
		h.LeapSecond(lm, tRead)
	}
}
