package unc

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/lib/uncmsg"
)

// BinPacketProcessor implements the gpsprot.PacketProcessor interface for Unicore binary packets
type BinPacketProcessor struct {
	packetProcessor
}

// NewBinPacketProcessor creates a new Unicore binary packet processor
func NewBinPacketProcessor(mgr *gpsprot.NavEpochManager) *BinPacketProcessor {
	return &BinPacketProcessor{packetProcessor{mh: &gpsprot.DefaultHandler{}, mgr: mgr}}
}

// ProcessPacket processes a Unicore binary packet's data and returns the message ID and any error
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := BinPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagBinary, msgID, uncmsg.ParseBinMsg)
	return msgID, err
}

// AsciiPacketProcessor implements the gpsprot.PacketProcessor interface for Unicore ASCII packets
type AsciiPacketProcessor struct {
	packetProcessor
}

// NewAsciiPacketProcessor creates a new Unicore ASCII packet processor
func NewAsciiPacketProcessor(mgr *gpsprot.NavEpochManager) *AsciiPacketProcessor {
	return &AsciiPacketProcessor{packetProcessor{mh: &gpsprot.DefaultHandler{}, mgr: mgr}}
}

// ProcessPacket processes a Unicore ASCII packet's data and returns the message ID and any error
func (p *AsciiPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := AsciiPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagAscii, msgID, uncmsg.ParseAsciiMessage)
	return msgID, err
}

// satsMsgSet tracks which satellite messages have been seen in an epoch.
type satsMsgSet struct {
	satsInfo bool
	bestSat  bool
}

// packetProcessor is the common functionality between BinPacketProcessor and AsciiPacketProcessor
type packetProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh  gpsprot.MsgHandler        // never nil; initialized to &gpsprot.DefaultHandler{}
	mgr *gpsprot.NavEpochManager
	// Navigation epoch tracking: Unicore messages carry (Week, MillisecondsOfWeek)
	// in the header. Messages with the same pair are part of the same epoch.
	// Invariant: curEpochMsg is non-nil iff curEpoch is non-zero.
	curEpoch    uint64              // (week<<32 | ms) + 1; 0 = no epoch
	curEpochMsg *gpsprot.NavEpochMsg
	curEpochTag gpsprot.Tag
	// Satellite message accumulation: SATSINFO and BESTSAT are combined
	// into a single SatellitesMsg when both are available.
	satsInfoMsg  *gpsprot.SatellitesMsg
	bestSatSats  []uncmsg.BestSatEntry // raw BESTSAT entries for merge
	satsTRead    time.Time             // timestamp of first message in the pair
	curSatsMsgs  satsMsgSet            // which sat messages seen in current epoch
	prevSatsMsgs satsMsgSet            // which sat messages seen in previous complete epoch
	nEpochsSeen  int
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *packetProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// processPacket is the common packet processing logic for both binary and ASCII packets
func (p *packetProcessor) processPacket(bytes []byte, tRead time.Time, tag gpsprot.Tag, msgID string,
	parser func([]byte) (*uncmsg.Msg, error)) error {
	msg, err := parser(bytes)
	if err != nil {
		return err
	}
	handled, err := p.dispatch(msg, tRead, tag)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return nmh.NativeMsg(tag, msgID, msg, tRead)
	}
	return nil
}

// dispatch attempts to convert and dispatch a message as a protocol-agnostic message.
// Returns (handled, error) where handled indicates if the message was processed.
func (p *packetProcessor) dispatch(msg *uncmsg.Msg, tRead time.Time, tag gpsprot.Tag) (bool, error) {
	p.handleEpoch(&msg.Hdr, tag, tRead)
	h := p.mh
	switch body := msg.Body.(type) {
	case *uncmsg.BestNav:
		posG, velG := bestNavPosVel(p.curEpochMsg, body)
		if posG != nil {
			posG.Tag = tag
			h.PosGeo(posG, tRead)
		}
		if velG != nil {
			velG.Tag = tag
			h.VelGeo(velG, tRead)
		}
		return posG != nil || velG != nil, nil
	case *uncmsg.BestNavXYZ:
		posE, velE := bestNavXYZPosVel(p.curEpochMsg, body)
		if posE != nil {
			posE.Tag = tag
			h.PosECEF(posE, tRead)
		}
		if velE != nil {
			velE.Tag = tag
			h.VelECEF(velE, tRead)
		}
		return posE != nil || velE != nil, nil
	case *uncmsg.RecTime:
		tm, err := timeMsgFromRecTime(&msg.Hdr, body, tag)
		if err != nil {
			return false, err
		}
		if tm != nil {
			h.Time(tm, tRead)
			return true, nil
		}
	case *uncmsg.SatsInfo:
		sm, err := satellitesMsgFromSatsInfo(&msg.Hdr, body, tag)
		if err != nil {
			return false, err
		}
		if sm != nil {
			p.satsInfoMsg = sm
			p.curSatsMsgs.satsInfo = true
			if p.satsTRead.IsZero() {
				p.satsTRead = tRead
			}
			p.maybeFlushSats()
			return true, nil
		}
	case *uncmsg.GPSUTC:
		return dispatchUTC(&msg.Hdr, body, utcConversionParamsFromGPSUTC, gpsprot.GPS, h, tRead)
	case *uncmsg.GALUTC:
		return dispatchUTC(&msg.Hdr, body, utcConversionParamsFromGALUTC, gpsprot.GAL, h, tRead)
	case *uncmsg.BD3UTC:
		return dispatchUTC(&msg.Hdr, body, utcConversionParamsFromBD3UTC, gpsprot.BDS, h, tRead)
	case *uncmsg.BDSUTC:
		return dispatchBDSUTC(&msg.Hdr, body, h, tRead)
	case *uncmsg.BestSat:
		// Set semantics: unconditionally overwrites any BESTPOS fill values.
		ne := p.curEpochMsg
		ne.GNSSUsed, ne.BandsUsed = bestSatGNSSBands(body.Sats)
		if body.Sats != nil {
			p.bestSatSats = body.Sats
		} else {
			p.bestSatSats = []uncmsg.BestSatEntry{}
		}
		p.curSatsMsgs.bestSat = true
		if p.satsTRead.IsZero() {
			p.satsTRead = tRead
		}
		p.maybeFlushSats()
		return true, nil
	case *uncmsg.StaDOP:
		staDOP(p.curEpochMsg, body)
		return true, nil
	}
	return false, nil
}

func (p *packetProcessor) handleEpoch(hdr *uncmsg.MsgHdr, tag gpsprot.Tag, tRead time.Time) {
	e := uint64(hdr.Week)<<32 | uint64(hdr.MillisecondsOfWeek)
	e++ // avoid zero
	if e != p.curEpoch {
		p.mgr.EpochStarted(p, tRead)
		p.curEpoch = e
		p.curEpochMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
		p.curEpochTag = tag
		p.prevSatsMsgs = p.curSatsMsgs
		p.curSatsMsgs = satsMsgSet{}
		p.nEpochsSeen++
	}
}

// FlushNavEpoch implements gpsprot.EpochFlusher.
func (p *packetProcessor) FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority, gpsprot.MsgHandler) {
	p.flushSats()
	msg := p.curEpochMsg
	p.curEpochMsg = nil
	if msg != nil {
		msg.Tag = p.curEpochTag
	}
	return msg, gpsprot.PriVendorLow, p.mh
}

// maybeFlushSats decides whether to emit a SatellitesMsg or wait for more data.
// If we have one but not both of SATSINFO and BESTSAT, we decide whether to wait
// for the other based on whether the missing message was seen in the previous epoch.
func (p *packetProcessor) maybeFlushSats() {
	if p.satsInfoMsg == nil && p.bestSatSats == nil {
		return
	}
	if p.satsInfoMsg != nil && p.bestSatSats != nil {
		p.flushSats()
		return
	}
	if p.nEpochsSeen < 3 {
		return
	}
	if p.satsInfoMsg == nil && p.prevSatsMsgs.satsInfo {
		return
	}
	if p.bestSatSats == nil && p.prevSatsMsgs.bestSat {
		return
	}
	p.flushSats()
}

// flushSats merges buffered SATSINFO and BESTSAT into a single SatellitesMsg
// and dispatches it.
func (p *packetProcessor) flushSats() {
	satsInfo := p.satsInfoMsg
	bestSat := p.bestSatSats
	if satsInfo == nil && bestSat == nil {
		return
	}
	p.satsInfoMsg = nil
	p.bestSatSats = nil
	tRead := p.satsTRead
	p.satsTRead = time.Time{}
	combined := satellitesMerge(satsInfo, bestSat)
	if combined == nil {
		return
	}
	if h := p.mh; h != nil {
		h.Satellites(combined, tRead)
	}
}

// dispatchUTC is a generic function that dispatches *UTC messages (GPS, Galileo, BD3)
func dispatchUTC[T any](
	hdr *uncmsg.MsgHdr,
	body T,
	convertFunc func(T, ptime.Time) (*utcConversionParams, error),
	gnss gpsprot.GNSS,
	mh gpsprot.MsgHandler,
	tRead time.Time,
) (bool, error) {
	_, now := msgHdrTime(hdr)
	params, err := convertFunc(body, now)
	if err != nil {
		return false, err
	}
	lsm := &gpsprot.LeapSecondMsg{
		LeapSecond: params.LeapSecond,
		GNSS:       gnss,
	}
	mh.LeapSecond(lsm, tRead)
	return true, nil
}

// dispatchBDSUTC handles the special case of BDSUTC which needs weekStart
func dispatchBDSUTC(hdr *uncmsg.MsgHdr, body *uncmsg.BDSUTC, mh gpsprot.MsgHandler, tRead time.Time) (bool, error) {
	weekStart := msgHdrWeekStart(hdr)
	_, now := msgHdrTime(hdr)
	params, err := utcConversionParamsFromBDSUTC(body, weekStart, now)
	if err != nil {
		return false, err
	}
	lsm := &gpsprot.LeapSecondMsg{
		LeapSecond: params.LeapSecond,
		GNSS:       gpsprot.BDS,
	}
	mh.LeapSecond(lsm, tRead)
	return true, nil
}
