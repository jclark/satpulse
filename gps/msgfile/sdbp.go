package msgfile

import (
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
)

// SDBPMsg represents a [[sdbp]] entry.
type SDBPMsg struct {
	UBXLikeMsg
}

func (sm *SDBPMsg) toRaw() (RawMsg, error) {
	payload, err := sm.Payload.Encode(sdbpbin.Endian)
	if err != nil {
		return RawMsg{}, err
	}
	mid := sdbpbin.MakeMsgID(sm.Class, sm.ID)
	pkt, err := sdbpbin.PackMsg(mid, payload)
	if err != nil {
		return RawMsg{}, err
	}
	delay, err := sm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := sm.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{Bytes: pkt, Delay: delay, WaitLimit: wl, Tag: *sm.Tag}, nil
}

func (sm *SDBPMsg) getTag() string { return *sm.Tag }

// sdbpAnalyzer handles response analysis for SDBP.
type sdbpAnalyzer struct{}

func (sdbpAnalyzer) analyzeResponse(data string) responseAnalysis {
	if len(data) < sdbpbin.PacketMinLen {
		return responseAnalysis{kind: responseMaybeData}
	}
	mid := sdbpbin.PacketMsgId(data)
	switch mid {
	case sdbpbin.PubAckID:
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: sdbpMsgIDString(sdbpbin.AckMsgID(data)),
		}
	case sdbpbin.PubNakID:
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: sdbpMsgIDString(sdbpbin.AckMsgID(data)),
		}
	}
	return responseAnalysis{kind: responseMaybeData}
}

func sdbpMsgIDString(mid sdbpbin.MsgID) string {
	cls, id := mid.Unpack()
	return string([]byte{cls, id})
}

func (sm *SDBPMsg) analyzeRequest(data string) requestAnalysis {
	mid := sdbpbin.MakeMsgID(sm.Class, sm.ID)
	corr := sdbpMsgIDString(sdbpbin.PacketMsgId(data))
	payloadLen := len(data) - sdbpbin.PacketMinLen
	if !mid.Ackable() {
		return requestAnalysis{
			ackTag:       gpsreg.TagSDBP,
			ackCorrelate: corr,
			expectAck:    ExpectAckNakOnly,
			expectData:   expectDataNone,
		}
	}
	// QUE class (0x05) messages are always queries.
	isQuery := sm.Class == 0x05 || payloadLen <= sdbpbin.PollPayloadLen(mid)
	if isQuery {
		return requestAnalysis{
			ackTag:       gpsreg.TagSDBP,
			ackCorrelate: corr,
			expectAck:    ExpectAckOrNak,
			dataTag:      gpsreg.TagSDBP,
			expectData:   expectDataSingle,
			dataMatch:    sdbpDataMatch(mid),
		}
	}
	// Set command.
	return requestAnalysis{
		ackTag:       gpsreg.TagSDBP,
		ackCorrelate: corr,
		expectAck:    ExpectAckOrNak,
		expectData:   expectDataNone,
	}
}

func sdbpDataMatch(mid sdbpbin.MsgID) func(string) bool {
	return func(data string) bool {
		return sdbpbin.PacketMsgId(data) == mid
	}
}
