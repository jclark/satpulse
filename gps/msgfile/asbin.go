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
	if len(data) < 8 {
		return responseAnalysis{kind: responseMaybeData}
	}
	mid := asbin.PacketMsgId(data)
	switch mid {
	case asbin.AckAckID:
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: asbinAckCorrelate(data),
		}
	case asbin.AckNakID:
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: asbinAckCorrelate(data),
		}
	}
	return responseAnalysis{kind: responseMaybeData}
}

// asbinAckCorrelate extracts the 2-byte class/ID from an Allystar ACK payload.
// ASBIN packet: sync(2) + class(1) + id(1) + len(2) + payload + cksum(2).
// ACK payload starts at byte 6: acked_class(1) + acked_id(1).
func asbinAckCorrelate(data string) string {
	if len(data) < 8 {
		return ""
	}
	return data[6:8]
}

// asbinMsgCorrelate extracts the 2-byte class/ID from an Allystar packet header.
func asbinMsgCorrelate(data string) string {
	if len(data) < 4 {
		return ""
	}
	return data[2:4]
}

func (am *ASBINMsg) analyzeRequest(data string) requestAnalysis {
	mid := asbin.MakeMsgID(am.Class, am.ID)
	corr := asbinMsgCorrelate(data)
	hasPayload := len(data) > 8
	if mid == asbin.CfgSimpleRstID {
		return requestAnalysis{
			expectAck:  ExpectAckNone,
			expectData: expectDataNone,
		}
	}
	if mid.CfgClass() {
		a := requestAnalysis{
			ackTag:       gpsreg.TagAllystarBin,
			ackCorrelate: corr,
			expectAck:    ExpectAckOrNak,
			dataTag:      gpsreg.TagAllystarBin,
		}
		if hasPayload {
			a.expectData = expectDataNone
		} else {
			a.expectData = expectDataSingle
			a.dataMatch = asbinDataMatch(corr)
		}
		return a
	}
	// Non-CFG poll.
	return requestAnalysis{
		expectAck:  ExpectAckNone,
		dataTag:    gpsreg.TagAllystarBin,
		expectData: expectDataAmbig,
		dataMatch:  asbinDataMatch(corr),
	}
}

func asbinDataMatch(corr string) func(string) bool {
	return func(data string) bool {
		return asbinMsgCorrelate(data) == corr
	}
}
