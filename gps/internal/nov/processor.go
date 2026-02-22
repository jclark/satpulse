package nov

import (
	"maps"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/novmsg"
)

// Variant selects the NovAtel-compatible vendor whose enum values and
// port encoding are used for parsing.
type Variant int

const (
	VariantOEM7 Variant = iota
	VariantSinoGNSS
	VariantUnicore // NovAtel-format messages on Unicore hardware (undocumented)
	VariantByNav
)

// parseResult holds the port-type-independent parts of a parsed message.
type parseResult struct {
	Common novmsg.CommonHdr
	Body   novmsg.MsgBody
	Msg    any // original *novmsg.Msg[P] for NativeMsg passthrough
}

// BinPacketProcessor implements the gpsprot.PacketProcessor interface for NovAtel binary packets
type BinPacketProcessor struct {
	packetProcessor
	ctors map[novmsg.MsgID]func() novmsg.MsgBody
	parse func([]byte, map[novmsg.MsgID]func() novmsg.MsgBody) (parseResult, error)
}

// NewBinPacketProcessor creates a new NovAtel binary packet processor
func NewBinPacketProcessor(mgr *gpsprot.NavEpochManager) *BinPacketProcessor {
	return &BinPacketProcessor{
		packetProcessor: packetProcessor{mh: &gpsprot.DefaultHandler{}, mgr: mgr},
		ctors:           novmsg.BinRegistry(),
		parse:           binParser[novmsg.Port](),
	}
}

// SetVariant configures the processor for a specific NovAtel-compatible vendor.
// Must be called before ProcessPacket.
func (p *BinPacketProcessor) SetVariant(v Variant) {
	p.ctors, p.parse = binVariant(v)
}

// ProcessPacket processes a NovAtel binary packet's data and returns the message ID and any error
func (p *BinPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	b := []byte(data)
	msgID := BinPacketFormat.MsgID(b)
	res, err := p.parse(b, p.ctors)
	if err != nil {
		return msgID, err
	}
	return msgID, p.processMsg(&res.Common, res.Body, res.Msg, tRead, TagBinary, msgID)
}

// AsciiPacketProcessor implements the gpsprot.PacketProcessor interface for NovAtel ASCII packets
type AsciiPacketProcessor struct {
	packetProcessor
	ctors map[string]func() novmsg.MsgBody
	parse func([]byte, map[string]func() novmsg.MsgBody) (parseResult, error)
}

// NewAsciiPacketProcessor creates a new NovAtel ASCII packet processor
func NewAsciiPacketProcessor(mgr *gpsprot.NavEpochManager) *AsciiPacketProcessor {
	return &AsciiPacketProcessor{
		packetProcessor: packetProcessor{mh: &gpsprot.DefaultHandler{}, mgr: mgr},
		ctors:           novmsg.AsciiRegistry(),
		parse:           asciiParser[novmsg.Port](),
	}
}

// SetVariant configures the processor for a specific NovAtel-compatible vendor.
// Must be called before ProcessPacket.
func (p *AsciiPacketProcessor) SetVariant(v Variant) {
	p.ctors, p.parse = asciiVariant(v)
}

// ProcessPacket processes a NovAtel ASCII packet's data and returns the message ID and any error
func (p *AsciiPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	b := []byte(data)
	msgID := AsciiPacketFormat.MsgID(b)
	res, err := p.parse(b, p.ctors)
	if err != nil {
		return msgID, err
	}
	return msgID, p.processMsg(&res.Common, res.Body, res.Msg, tRead, TagAscii, msgID)
}

// binParser returns a parse closure for binary messages with a specific port type.
func binParser[P ~uint8]() func([]byte, map[novmsg.MsgID]func() novmsg.MsgBody) (parseResult, error) {
	return func(pkt []byte, ctors map[novmsg.MsgID]func() novmsg.MsgBody) (parseResult, error) {
		msg, err := novmsg.ParseBinMsgUsing[P](pkt, ctors)
		if err != nil {
			return parseResult{}, err
		}
		return parseResult{Common: msg.Hdr.CommonHdr, Body: msg.Body, Msg: msg}, nil
	}
}

// asciiParser returns a parse closure for ASCII messages with a specific port type.
func asciiParser[P ~uint8]() func([]byte, map[string]func() novmsg.MsgBody) (parseResult, error) {
	return func(pkt []byte, ctors map[string]func() novmsg.MsgBody) (parseResult, error) {
		msg, err := novmsg.ParseAsciiMsgUsing[P](pkt, ctors)
		if err != nil {
			return parseResult{}, err
		}
		return parseResult{Common: msg.Hdr.CommonHdr, Body: msg.Body, Msg: msg}, nil
	}
}

// binVariant returns the constructor map and parse function for a binary variant.
func binVariant(v Variant) (map[novmsg.MsgID]func() novmsg.MsgBody,
	func([]byte, map[novmsg.MsgID]func() novmsg.MsgBody) (parseResult, error)) {
	reg := novmsg.BinRegistry()
	switch v {
	case VariantSinoGNSS:
		m := copyMap(reg)
		m[novmsg.BestPosID] = func() novmsg.MsgBody { return &novmsg.SinoBestPos{} }
		m[novmsg.PsrPosID] = func() novmsg.MsgBody { return &novmsg.SinoPsrPos{} }
		m[novmsg.PsrVelID] = func() novmsg.MsgBody { return &novmsg.SinoPsrVel{} }
		m[novmsg.BestXYZID] = func() novmsg.MsgBody { return &novmsg.SinoBestXYZ{} }
		return m, binParser[novmsg.SinoPort]()
	case VariantUnicore:
		m := copyMap(reg)
		m[novmsg.UnicoreIonUTCID] = reg[novmsg.IonUTCID]
		delete(m, novmsg.IonUTCID)
		return m, binParser[novmsg.UnicorePort]()
	default: // OEM7, ByNav
		return reg, binParser[novmsg.Port]()
	}
}

// asciiVariant returns the constructor map and parse function for an ASCII variant.
func asciiVariant(v Variant) (map[string]func() novmsg.MsgBody,
	func([]byte, map[string]func() novmsg.MsgBody) (parseResult, error)) {
	reg := novmsg.AsciiRegistry()
	switch v {
	case VariantSinoGNSS:
		m := copyMap(reg)
		m["BESTPOSA"] = func() novmsg.MsgBody { return &novmsg.SinoBestPos{} }
		m["PSRPOSA"] = func() novmsg.MsgBody { return &novmsg.SinoPsrPos{} }
		m["PSRVELA"] = func() novmsg.MsgBody { return &novmsg.SinoPsrVel{} }
		m["BESTXYZA"] = func() novmsg.MsgBody { return &novmsg.SinoBestXYZ{} }
		return m, asciiParser[novmsg.SinoPort]()
	case VariantUnicore:
		return reg, asciiParser[novmsg.UnicorePort]()
	default: // OEM7, ByNav
		return reg, asciiParser[novmsg.Port]()
	}
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	cp := make(map[K]V, len(m))
	maps.Copy(cp, m)
	return cp
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

// processMsg handles a parsed message: dispatches known types and forwards
// unhandled messages to the NativeMsgHandler. The msg parameter is the
// original *Msg[P] for NativeMsg passthrough.
func (p *packetProcessor) processMsg(common *novmsg.CommonHdr, body novmsg.MsgBody, msg any, tRead time.Time, tag gpsprot.Tag, msgID string) error {
	handled, err := p.dispatch(common, body, tRead, tag)
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
func (p *packetProcessor) dispatch(common *novmsg.CommonHdr, body novmsg.MsgBody, tRead time.Time, tag gpsprot.Tag) (bool, error) {
	p.handleEpoch(common, tag, tRead)
	h := p.mh
	switch m := body.(type) {
	case *novmsg.BestPos:
		return posGeoBestPos(h, p.curEpochMsg, &m.Pos, tag, tRead)
	case *novmsg.SinoBestPos:
		return posGeoBestPos(h, p.curEpochMsg, &m.Pos, tag, tRead)
	case *novmsg.BestGNSSPos:
		return posGeoBestPos(h, p.curEpochMsg, &m.Pos, tag, tRead)
	case *novmsg.PsrPos:
		return posGeoBestPos(h, p.curEpochMsg, &m.Pos, tag, tRead)
	case *novmsg.SinoPsrPos:
		return posGeoBestPos(h, p.curEpochMsg, &m.Pos, tag, tRead)
	case *novmsg.BestVel:
		if m.SolStatus != novmsg.SolComputed {
			return false, nil
		}
		velG := VelGeoVel(p.curEpochMsg, &m.Vel, "BESTVEL")
		velG.Tag = tag
		h.VelGeo(velG, tRead)
		return true, nil
	case *novmsg.PsrVel:
		if m.SolStatus != novmsg.SolComputed {
			return false, nil
		}
		velG := VelGeoVel(p.curEpochMsg, &m.Vel, "PSRVEL")
		velG.Tag = tag
		h.VelGeo(velG, tRead)
		return true, nil
	case *novmsg.SinoPsrVel:
		if m.SolStatus != novmsg.SolComputed {
			return false, nil
		}
		velG := VelGeoVel(p.curEpochMsg, &m.Vel, "PSRVEL")
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
		return posVelECEFBestXYZ(h, p.curEpochMsg, &m.XYZ, tag, tRead)
	case *novmsg.SinoBestXYZ:
		return posVelECEFBestXYZ(h, p.curEpochMsg, &m.XYZ, tag, tRead)
	case *novmsg.Time:
		tm, err := timeMsgFromTime(common, m, tag)
		if err != nil {
			return false, err
		}
		if tm != nil {
			h.Time(tm, tRead)
			return true, nil
		}
	case *novmsg.IonUTC:
		return leapSecondIonUTC(common, m, h, tRead)
	case *novmsg.UnicoreIonUTC:
		return leapSecondIonUTC(common, &m.IonUTC, h, tRead)
	}
	return false, nil
}

func (p *packetProcessor) handleEpoch(common *novmsg.CommonHdr, tag gpsprot.Tag, tRead time.Time) {
	e := uint64(common.Week)<<32 | uint64(common.MillisecondsOfWeek)
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
