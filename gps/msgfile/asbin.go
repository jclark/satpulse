package msgfile

import (
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// ASBINMsg represents a [[asbin]] entry.
type ASBINMsg struct {
	UBXLikeMsg
}

func (am *ASBINMsg) toRaw() (RawMsg, error) {
	payload, err := am.Payload.Encode(asbin.Endian())
	if err != nil {
		return RawMsg{}, err
	}
	mid := asbin.MakeMsgID(am.Class, am.ID)
	pkt, err := asbin.PackMsg(mid, payload)
	if err != nil {
		return RawMsg{}, err
	}
	delay, err := am.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := am.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{Bytes: pkt, Delay: delay, WaitLimit: wl, Tag: *am.Tag}, nil
}

func (am *ASBINMsg) getTag() string { return *am.Tag }

// asbinAnalyzer handles response analysis for Allystar binary.
type asbinAnalyzer struct{}

func (asbinAnalyzer) analyzeResponse(data string) responseAnalysis {
	if len(data) < asbin.PacketMinLen {
		return responseAnalysis{kind: responseMaybeData}
	}
	mid := asbin.PacketMsgId(data)
	switch mid {
	case asbin.AckAckID:
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: asbinMsgIDString(asbin.AckMsgID(data)),
		}
	case asbin.AckNakID:
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: asbinMsgIDString(asbin.AckMsgID(data)),
		}
	}
	return responseAnalysis{kind: responseMaybeData}
}

func asbinMsgIDString(mid asbin.MsgID) string {
	cls, id := mid.Unpack()
	return string([]byte{cls, id})
}

func (am *ASBINMsg) analyzeRequest(data string) requestAnalysis {
	mid := asbin.MakeMsgID(am.Class, am.ID)
	corr := asbinMsgIDString(asbin.PacketMsgId(data))
	payloadLen := len(data) - asbin.PacketMinLen
	if mid.CfgClass() {
		if !mid.Ackable() {
			return requestAnalysis{
				ackTag:       gpsreg.TagAllystarBin,
				ackCorrelate: corr,
				expectAck:    ExpectAckNakOnly,
				expectData:   expectDataNone,
			}
		}
		a := requestAnalysis{
			ackTag:       gpsreg.TagAllystarBin,
			ackCorrelate: corr,
			expectAck:    ExpectAckOrNak,
			dataTag:      gpsreg.TagAllystarBin,
		}
		if payloadLen <= asbin.PollPayloadLen(mid) {
			a.expectData = expectDataSingle
			a.dataMatch = asbinDataMatch(mid)
		} else {
			a.expectData = expectDataNone
		}
		return a
	}
	// Non-CFG poll.
	return requestAnalysis{
		expectAck:  ExpectAckNone,
		dataTag:    gpsreg.TagAllystarBin,
		expectData: expectDataAmbig,
		dataMatch:  asbinDataMatch(mid),
	}
}

func asbinDataMatch(mid asbin.MsgID) func(string) bool {
	return func(data string) bool {
		return asbin.PacketMsgId(data) == mid
	}
}
