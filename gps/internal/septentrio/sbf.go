package septentrio

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/lib/sbfbin"
)

var (
	_ gpsprot.PacketProcessor = (*PacketProcessor)(nil)
	_ gpsprot.EpochFlusher    = (*PacketProcessor)(nil)
)

// PacketProcessor implements gpsprot.PacketProcessor and gpsprot.EpochFlusher
// for Septentrio SBF packets. It parses each packet via sbfbin.ParseMsg,
// converts recognized blocks into gpsprot.Msg values, accumulates one
// NavEpochMsg per PVT-family epoch, runs an independent SatellitesMsg stream,
// and falls back to NativeMsg for every unmapped block.
type PacketProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh  gpsprot.MsgHandler
	mgr *gpsprot.NavEpochManager

	// PVT-family navigation epoch, keyed by the (TOW, WNc) block header.
	curEpochValid bool
	curTOW        uint32
	curWNc        uint16
	curEpochMsg   *gpsprot.NavEpochMsg
	curEpochStart time.Time

	// Independent SatellitesMsg stream, keyed by its own (TOW, WNc).
	satValid bool
	satTOW   uint32
	satWNc   uint16
	satChan  *sbfbin.ChannelStatus
	satMeas  *sbfbin.MeasEpoch
	satVis   *sbfbin.SatVisibility
	satTRead time.Time

	// Last-seen RTCM base station, retained across epochs (BaseStation is
	// event-driven, not epoch-keyed).
	baseStationID   opt.Val[uint16]
	baseStationRTCM bool
}

// NewPacketProcessor creates a new SBF packet processor sharing the given
// NavEpochManager with the receiver's other protocol processors.
func NewPacketProcessor(mgr *gpsprot.NavEpochManager) *PacketProcessor {
	return &PacketProcessor{mgr: mgr}
}

// SetMsgHandler sets the handler for protocol-agnostic messages.
func (p *PacketProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// ProcessPacket parses an SBF packet, converts it, and returns its message ID.
func (p *PacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	b, err := sbfbin.ParseMsg(data)
	if err != nil {
		return PacketFormat.MsgID([]byte(data)), err
	}
	msgID := b.ID().String()
	if p.Dispatch(b, tRead) {
		return msgID, nil
	}
	if nmh := p.GetNativeMsgHandler(); nmh != nil {
		return msgID, nmh.NativeMsg(Tag, msgID, b, tRead)
	}
	return msgID, nil
}

// Dispatch converts a decoded SBF block into protocol-agnostic messages and
// sends them to the message handler. It reports whether the block was handled;
// unhandled blocks fall back to the native message handler.
func (p *PacketProcessor) Dispatch(b *sbfbin.Block, tRead time.Time) bool {
	switch m := b.Params.(type) {
	case *sbfbin.PVTGeodetic:
		ne := p.navEpoch(b.TimeStamp, tRead)
		qualityPVT(ne, pvtGeodeticCommon(m), p.baseStationID)
		p.emitTime(timePVTGeodetic(b, m), tRead)
		p.emitPosGeo(posGeoPVTGeodetic(m), tRead)
		p.emitVelGeo(velGeoPVTGeodetic(m), tRead)
		return true
	case *sbfbin.PVTCartesian:
		ne := p.navEpoch(b.TimeStamp, tRead)
		qualityPVT(ne, pvtCartesianCommon(m), p.baseStationID)
		p.emitTime(timePVTCartesian(b, m), tRead)
		p.emitPosECEF(posECEFPVTCartesian(m), tRead)
		p.emitVelECEF(velECEFPVTCartesian(m), tRead)
		p.emitSurvey(surveyPVTCartesian(m), tRead)
		return true
	case *sbfbin.DOP:
		dopDOP(p.navEpoch(b.TimeStamp, tRead), m)
		return true
	case *sbfbin.PosCovCartesian:
		accPosCovCartesian(p.navEpoch(b.TimeStamp, tRead), m)
		return true
	case *sbfbin.PosCovGeodetic:
		accPosCovGeodetic(p.navEpoch(b.TimeStamp, tRead), m)
		return true
	case *sbfbin.VelCovCartesian:
		accVelCovCartesian(p.navEpoch(b.TimeStamp, tRead), m)
		return true
	case *sbfbin.VelCovGeodetic:
		accVelCovGeodetic(p.navEpoch(b.TimeStamp, tRead), m)
		return true
	case *sbfbin.EndOfPVT:
		p.mgr.EndOfProtocolEpoch(p, tRead)
		return true
	case *sbfbin.ReceiverTime:
		p.emitTime(timeReceiverTime(b, m), tRead)
		return true
	case *sbfbin.XPPSOffset:
		p.emitTime(timeXPPSOffset(b, m), tRead)
		return true
	case *sbfbin.GPSUtc:
		p.emitLeap(leapGPSUtc(b, m), tRead)
		return true
	case *sbfbin.GALUtc:
		p.emitLeap(leapGALUtc(b, m), tRead)
		return true
	case *sbfbin.BDSUtc:
		p.emitLeap(leapBDSUtc(b, m), tRead)
		return true
	case *sbfbin.ChannelStatus:
		p.satBoundary(b.TimeStamp, tRead)
		p.satChan = m
		return true
	case *sbfbin.MeasEpoch:
		p.satBoundary(b.TimeStamp, tRead)
		p.satMeas = m
		return true
	case *sbfbin.SatVisibility:
		p.satBoundary(b.TimeStamp, tRead)
		p.satVis = m
		return true
	case *sbfbin.DiffCorrIn:
		p.emitCorReport(corReportDiffCorrIn(m, p.baseStationID, p.baseStationRTCM), tRead)
		return true
	case *sbfbin.BaseStation:
		p.observeBaseStation(m)
		return false // no gpsprot.Msg; still report as native
	default:
		return false
	}
}

// navEpoch returns the accumulating NavEpochMsg for the epoch identified by
// ts, starting a new epoch (and flushing the previous one) when the (TOW, WNc)
// key changes.
func (p *PacketProcessor) navEpoch(ts sbfbin.TimeStamp, tRead time.Time) *gpsprot.NavEpochMsg {
	if !p.curEpochValid || ts.TOW != p.curTOW || ts.WNc != p.curWNc {
		p.mgr.EpochStarted(p, tRead)
		p.curEpochValid = true
		p.curTOW = ts.TOW
		p.curWNc = ts.WNc
		p.curEpochMsg = &gpsprot.NavEpochMsg{}
		p.curEpochStart = tRead
	}
	return p.curEpochMsg
}

// FlushNavEpoch implements gpsprot.EpochFlusher. It returns the accumulated
// NavEpochMsg for the current epoch (the SatellitesMsg stream is independent
// and is not flushed here). The epoch key is retained so the next PVT block's
// key change is detected.
func (p *PacketProcessor) FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority, gpsprot.MsgHandler) {
	msg := p.curEpochMsg
	p.curEpochMsg = nil
	if msg != nil {
		msg.Tag = Tag
	}
	return msg, gpsprot.PriVendorLow, p.mh
}

// satBoundary flushes the accumulated SatellitesMsg when the sat-block (TOW,
// WNc) changes, then records the new key.
func (p *PacketProcessor) satBoundary(ts sbfbin.TimeStamp, tRead time.Time) {
	if p.satValid && (ts.TOW != p.satTOW || ts.WNc != p.satWNc) {
		p.flushSats()
	}
	p.satValid = true
	p.satTOW = ts.TOW
	p.satWNc = ts.WNc
	if p.satTRead.IsZero() {
		p.satTRead = tRead
	}
}

// flushSats combines the accumulated ChannelStatus/MeasEpoch/SatVisibility of
// the completed sat-epoch into one SatellitesMsg and dispatches it.
func (p *PacketProcessor) flushSats() {
	chn, meas, vis := p.satChan, p.satMeas, p.satVis
	tRead := p.satTRead
	p.satChan, p.satMeas, p.satVis = nil, nil, nil
	p.satTRead = time.Time{}
	if chn == nil && meas == nil && vis == nil {
		return
	}
	if msg := satellitesCombine(chn, meas, vis); msg != nil && p.mh != nil {
		msg.Tag = Tag
		p.mh.Satellites(msg, tRead)
	}
}

// observeBaseStation records the last-seen RTCM base station ID for cross-block
// RTCMRefBaseID fill.
func (p *PacketProcessor) observeBaseStation(m *sbfbin.BaseStation) {
	p.baseStationID = opt.Make(m.BaseStationID)
	p.baseStationRTCM = m.Source == sbfbin.BaseSourceRTCM
}

// Emission helpers stamp Tag/Priority centrally, keeping the per-block
// converters protocol-detail-only.

func (p *PacketProcessor) emitTime(tm *gpsprot.TimeMsg, tRead time.Time) {
	if tm == nil || p.mh == nil {
		return
	}
	tm.Tag = Tag
	if tm.Ref == gpsprot.NavSolution && p.curEpochValid {
		tm.ReadDelay = gpsprot.Duration(tRead.Sub(p.curEpochStart))
	}
	p.mh.Time(tm, tRead)
}

func (p *PacketProcessor) emitLeap(ls *gpsprot.LeapSecondMsg, tRead time.Time) {
	if ls == nil || p.mh == nil {
		return
	}
	p.mh.LeapSecond(ls, tRead)
}

func (p *PacketProcessor) emitPosGeo(m *gpsprot.PosGeoMsg, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	m.Tag = Tag
	m.Priority = gpsprot.PriVendorLow
	p.mh.PosGeo(m, tRead)
}

func (p *PacketProcessor) emitPosECEF(m *gpsprot.PosECEFMsg, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	m.Tag = Tag
	m.Priority = gpsprot.PriVendorLow
	p.mh.PosECEF(m, tRead)
}

func (p *PacketProcessor) emitVelGeo(m *gpsprot.VelGeoMsg, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	m.Tag = Tag
	m.Priority = gpsprot.PriVendorLow
	p.mh.VelGeo(m, tRead)
}

func (p *PacketProcessor) emitVelECEF(m *gpsprot.VelECEFMsg, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	m.Tag = Tag
	m.Priority = gpsprot.PriVendorLow
	p.mh.VelECEF(m, tRead)
}

func (p *PacketProcessor) emitSurvey(m *gpsprot.SurveyMsg, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	p.mh.Survey(m, tRead)
}

func (p *PacketProcessor) emitCorReport(m *gpsprot.CorReportMsg, tRead time.Time) {
	if m == nil || p.mh == nil {
		return
	}
	p.mh.CorReport(m, tRead)
}
