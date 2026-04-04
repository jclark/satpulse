package msgfile

import (
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// CASBINMsg represents a [[casbin]] entry.
type CASBINMsg struct {
	UBXLikeMsg
}

func (cm *CASBINMsg) toRaw() (RawMsg, error) {
	payload, err := cm.Payload.Encode(casbin.Endian)
	if err != nil {
		return RawMsg{}, err
	}
	mid := casbin.MakeMsgID(cm.Class, cm.ID)
	pkt, err := casbin.PackMsg(mid, payload)
	if err != nil {
		return RawMsg{}, err
	}
	delay, err := cm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := cm.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{Bytes: pkt, Delay: delay, WaitLimit: wl, Tag: *cm.Tag}, nil
}

func (cm *CASBINMsg) getTag() string { return *cm.Tag }

// casbinAnalyzer handles response analysis for CASIC binary.
type casbinAnalyzer struct{}

func (casbinAnalyzer) analyzeResponse(data string) responseAnalysis {
	if len(data) < casbin.PacketMinLen {
		return responseAnalysis{kind: responseMaybeData}
	}
	mid := casbin.PacketMsgID(data)
	switch mid {
	case casbin.AckAckID:
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: casbinMsgIDString(casbin.AckMsgID(data)),
		}
	case casbin.AckNakID:
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: casbinMsgIDString(casbin.AckMsgID(data)),
		}
	}
	return responseAnalysis{kind: responseMaybeData}
}

func casbinMsgIDString(mid casbin.MsgID) string {
	cls, id := mid.Unpack()
	return string([]byte{cls, id})
}

func (cm *CASBINMsg) analyzeRequest(data string) requestAnalysis {
	mid := casbin.MakeMsgID(cm.Class, cm.ID)
	corr := casbinMsgIDString(casbin.PacketMsgID(data))
	payloadLen := len(data) - casbin.PacketMinLen
	if mid.CfgClass() && !mid.Ackable() {
		return requestAnalysis{
			ackTag:       gpsreg.TagCASICBin,
			ackCorrelate: corr,
			expectAck:    ExpectAckNakOnly,
			expectData:   expectDataNone,
		}
	}
	if mid == casbin.CfgMsgID {
		if payloadLen <= 0 {
			// All-rates query: ACK + multiple CFG-MSG data responses.
			return requestAnalysis{
				ackTag:       gpsreg.TagCASICBin,
				ackCorrelate: corr,
				expectAck:    ExpectAckOrNak,
				dataTag:      gpsreg.TagCASICBin,
				expectData:   expectDataMultiple,
				dataMatch:    casbinDataMatch(mid),
			}
		}
		// Single-message poll: rate field is 0xFFFF.
		// Payload: target_class(1) + target_id(1) + rate(2).
		payload := data[casbin.HeaderLen : len(data)-casbin.TrailerLen]
		if len(payload) >= 4 && payload[2] == 0xFF && payload[3] == 0xFF {
			polledMid := casbin.MakeMsgID(payload[0], payload[1])
			return requestAnalysis{
				ackTag:       gpsreg.TagCASICBin,
				ackCorrelate: corr,
				expectAck:    ExpectAckOrNak,
				dataTag:      gpsreg.TagCASICBin,
				expectData:   expectDataSingle,
				dataMatch:    casbinDataMatch(polledMid),
			}
		}
		// Regular CFG-MSG set (falls through to CFG set below).
	}
	if mid.CfgClass() {
		a := requestAnalysis{
			ackTag:       gpsreg.TagCASICBin,
			ackCorrelate: corr,
			expectAck:    ExpectAckOrNak,
			dataTag:      gpsreg.TagCASICBin,
		}
		if payloadLen <= casbin.PollPayloadLen(mid) {
			a.expectData = expectDataSingle
			a.dataMatch = casbinDataMatch(mid)
		} else {
			a.expectData = expectDataNone
		}
		return a
	}
	// Non-CFG poll.
	return requestAnalysis{
		expectAck:  ExpectAckNone,
		dataTag:    gpsreg.TagCASICBin,
		expectData: expectDataAmbig,
		dataMatch:  casbinDataMatch(mid),
	}
}

func casbinDataMatch(mid casbin.MsgID) func(string) bool {
	return func(data string) bool {
		return casbin.PacketMsgID(data) == mid
	}
}
