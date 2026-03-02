package nov

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// BinPacketProcessor implements the gpsprot.PacketProcessor interface for NovAtel binary packets
type BinPacketProcessor struct {
	packetProcessor
}

// NewBinPacketProcessor creates a new NovAtel binary packet processor
func NewBinPacketProcessor(mgr *gpsprot.NavEpochManager) *BinPacketProcessor {
	return &BinPacketProcessor{packetProcessor{mh: &gpsprot.DefaultHandler{}, mgr: mgr}}
}

// ProcessPacket processes a NovAtel binary packet's data and returns the message ID and any error
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := BinPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagBinary, msgID, novmsg.ParseBinMsg)
	return msgID, err
}

// AsciiPacketProcessor implements the gpsprot.PacketProcessor interface for NovAtel ASCII packets
type AsciiPacketProcessor struct {
	packetProcessor
}

// NewAsciiPacketProcessor creates a new NovAtel ASCII packet processor
func NewAsciiPacketProcessor(mgr *gpsprot.NavEpochManager) *AsciiPacketProcessor {
	return &AsciiPacketProcessor{packetProcessor{mh: &gpsprot.DefaultHandler{}, mgr: mgr}}
}

// ProcessPacket processes a NovAtel ASCII packet's data and returns the message ID and any error
func (p *AsciiPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := AsciiPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagAscii, msgID, novmsg.ParseAsciiMessage)
	return msgID, err
}

// packetProcessor is the common functionality between BinPacketProcessor and AsciiPacketProcessor
type packetProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh  gpsprot.MsgHandler        // never nil; initialized to &gpsprot.DefaultHandler{}
	mgr *gpsprot.NavEpochManager
	// Navigation epoch tracking: NovAtel messages carry (Week, MillisecondsOfWeek)
	// in the header. Messages with the same pair are part of the same epoch.
	// Invariant: curEpochMsg is non-nil iff curEpoch is non-zero.
	curEpoch    uint64              // (week<<32 | ms) + 1; 0 = no epoch
	curEpochMsg *gpsprot.NavEpochMsg
	curEpochTag gpsprot.Tag
}

// SetMsgHandler sets the handler for protocol-agnostic messages
func (p *packetProcessor) SetMsgHandler(handler gpsprot.MsgHandler) {
	p.mh = handler
}

// processPacket is the common packet processing logic for both binary and ASCII packets
func (p *packetProcessor) processPacket(bytes []byte, tRead time.Time, tag gpsprot.Tag, msgID string,
	parser func([]byte) (*novmsg.Msg, error)) error {
	msg, err := parser(bytes)
	if err != nil {
		return err
	}
	handled, err := p.dispatch(&msg.Hdr, msg.Body, tRead, tag)
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
func (p *packetProcessor) dispatch(hdr *novmsg.MsgHdr, body novmsg.MsgBody, tRead time.Time, tag gpsprot.Tag) (bool, error) {
	p.handleEpoch(hdr, tag, tRead)
	h := p.mh
	switch m := body.(type) {
	case *novmsg.BestPos:
		if m.PSolStatus != novmsg.SolComputed {
			return false, nil
		}
		posG := PosGeo(p.curEpochMsg, &m.Pos, "BESTPOS")
		posG.Tag = tag
		h.PosGeo(posG, tRead)
		return true, nil
	case *novmsg.BestVel:
		if m.SolStatus != novmsg.SolComputed {
			return false, nil
		}
		velG := VelGeoVel(p.curEpochMsg, &m.Vel, "BESTVEL")
		velG.Tag = tag
		h.VelGeo(velG, tRead)
		return true, nil
	case *novmsg.BestGNSSVel:
		if m.SolStatus != novmsg.SolComputed {
			return false, nil
		}
		velG := VelGeoVel(p.curEpochMsg, &m.Vel, "BESTGNSSVEL")
		velG.Tag = tag
		h.VelGeo(velG, tRead)
		return true, nil
	case *novmsg.BestXYZ:
		var posE *gpsprot.PosECEFMsg
		var velE *gpsprot.VelECEFMsg
		if m.PSolStatus == novmsg.SolComputed {
			posE = PosECEFXYZ(p.curEpochMsg, &m.XYZ, "BESTXYZ")
		}
		if m.VSolStatus == novmsg.SolComputed {
			velE = VelECEFXYZ(p.curEpochMsg, &m.XYZ, "BESTXYZ")
		}
		if posE != nil {
			posE.Tag = tag
			h.PosECEF(posE, tRead)
		}
		if velE != nil {
			velE.Tag = tag
			h.VelECEF(velE, tRead)
		}
		return posE != nil || velE != nil, nil
	case *novmsg.Time:
		tm, err := timeMsgFromTime(hdr, m, tag)
		if err != nil {
			return false, err
		}
		if tm != nil {
			h.Time(tm, tRead)
			return true, nil
		}
	case *novmsg.IonUTC:
		return dispatchIonUTC(hdr, m, h, tRead)
	}
	return false, nil
}

func (p *packetProcessor) handleEpoch(hdr *novmsg.MsgHdr, tag gpsprot.Tag, tRead time.Time) {
	e := uint64(hdr.Week)<<32 | uint64(hdr.MillisecondsOfWeek)
	e++ // avoid zero
	if e != p.curEpoch {
		p.mgr.EpochStarted(p, tRead)
		p.curEpoch = e
		p.curEpochMsg = &gpsprot.NavEpochMsg{StartTime: tRead}
		p.curEpochTag = tag
	}
}

// FlushNavEpoch implements gpsprot.EpochFlusher.
func (p *packetProcessor) FlushNavEpoch(tRead time.Time) (*gpsprot.NavEpochMsg, gpsprot.MsgPriority, gpsprot.MsgHandler) {
	msg := p.curEpochMsg
	p.curEpochMsg = nil
	if msg != nil {
		msg.Tag = p.curEpochTag
	}
	return msg, gpsprot.PriVendorLow, p.mh
}
