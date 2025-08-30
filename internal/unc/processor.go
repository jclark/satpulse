package unc

import (
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/uncmsg"
)

// BinPacketProcessor implements the gpsprot.PacketProcessor interface for Unicore binary packets
type BinPacketProcessor struct {
	packetProcessor
}

// NewBinPacketProcessor creates a new Unicore binary packet processor
func NewBinPacketProcessor() *BinPacketProcessor {
	return &BinPacketProcessor{}
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
func NewAsciiPacketProcessor() *AsciiPacketProcessor {
	return &AsciiPacketProcessor{}
}

// ProcessPacket processes a Unicore ASCII packet's data and returns the message ID and any error
func (p *AsciiPacketProcessor) ProcessPacket(data string, tRead time.Time) (string, error) {
	bytes := []byte(data)
	msgID := AsciiPacketFormat.MsgID(bytes)
	err := p.processPacket(bytes, tRead, TagAscii, msgID, uncmsg.ParseAsciiMessage)
	return msgID, err
}

// packetProcessor is the common functionality between BinPacketProcessor and AsciiPacketProcessor
type packetProcessor struct {
	gpsprot.DefaultPacketProcessor
	mh gpsprot.MsgHandler
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
	if p.mh != nil {
		handled, err := p.dispatch(msg, tRead, tag)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
	}
	nmh := p.GetNativeMsgHandler()
	if nmh != nil {
		return nmh.NativeMsg(tag, msgID, msg, tRead)
	}
	return nil
}

// dispatch attempts to convert and dispatch a message as a protocol-agnostic message
// Returns (handled, error) where handled indicates if the message was processed
func (p *packetProcessor) dispatch(msg *uncmsg.Msg, tRead time.Time, tag gpsprot.Tag) (bool, error) {
	switch body := msg.Body.(type) {
	case *uncmsg.RecTime:
		tm, err := timeMsgFromRecTime(&msg.Hdr, body, tag)
		if err != nil {
			return false, err
		}
		if tm != nil {
			p.mh.Time(tm, tRead)
			return true, nil
		}
	case *uncmsg.SatsInfo:
		sm, err := satellitesMsgFromSatsInfo(&msg.Hdr, body, tag)
		if err != nil {
			return false, err
		}
		if sm != nil {
			p.mh.Satellites(sm, tRead)
			return true, nil
		}
	case *uncmsg.GPSUTC:
		return dispatchUTC(&msg.Hdr, body, utcConversionParamsFromGPSUTC, gpsprot.GPS, p.mh, tRead)
	case *uncmsg.GALUTC:
		return dispatchUTC(&msg.Hdr, body, utcConversionParamsFromGALUTC, gpsprot.GAL, p.mh, tRead)
	case *uncmsg.BD3UTC:
		return dispatchUTC(&msg.Hdr, body, utcConversionParamsFromBD3UTC, gpsprot.BDS, p.mh, tRead)
	case *uncmsg.BDSUTC:
		return dispatchBDSUTC(&msg.Hdr, body, p.mh, tRead)
	}
	return false, nil
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
