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
	if len(data) < 10 {
		return responseAnalysis{kind: responseMaybeData}
	}
	mid := casbin.PacketMsgID(data)
	switch mid {
	case casbin.AckAckID:
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: casbinAckCorrelate(data),
		}
	case casbin.AckNakID:
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: casbinAckCorrelate(data),
		}
	}
	return responseAnalysis{kind: responseMaybeData}
}

// casbinAckCorrelate extracts the 2-byte class/ID from a CASIC ACK payload.
// CASIC packet: sync(2) + len(2) + class(1) + id(1) + payload + cksum(4).
// ACK payload starts at byte 6: acked_class(1) + acked_id(1) + reserved(2).
func casbinAckCorrelate(data string) string {
	if len(data) < 8 {
		return ""
	}
	return data[6:8]
}

// casbinMsgCorrelate extracts the 2-byte class/ID from a CASIC packet header.
func casbinMsgCorrelate(data string) string {
	if len(data) < 6 {
		return ""
	}
	return data[4:6]
}

func (cm *CASBINMsg) analyzeRequest(data string) requestAnalysis {
	mid := casbin.MakeMsgID(cm.Class, cm.ID)
	corr := casbinMsgCorrelate(data)
	payloadLen := len(data) - 10
	if mid.CfgClass() && !mid.Ackable() {
		return requestAnalysis{
			expectAck:  ExpectAckNone,
			expectData: expectDataNone,
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
				dataMatch:    casbinDataMatch(corr),
			}
		}
		// Single-message poll: rate field is 0xFFFF.
		// Payload: target_class(1) + target_id(1) + rate(2).
		payload := data[6 : len(data)-4]
		if len(payload) >= 4 && payload[2] == 0xFF && payload[3] == 0xFF {
			polledCorr := data[6:8] // target class/ID from payload
			return requestAnalysis{
				ackTag:       gpsreg.TagCASICBin,
				ackCorrelate: corr,
				expectAck:    ExpectAckOrNak,
				dataTag:      gpsreg.TagCASICBin,
				expectData:   expectDataSingle,
				dataMatch:    casbinDataMatch(polledCorr),
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
			a.dataMatch = casbinDataMatch(corr)
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
		dataMatch:  casbinDataMatch(corr),
	}
}

func casbinDataMatch(corr string) func(string) bool {
	return func(data string) bool {
		return casbinMsgCorrelate(data) == corr
	}
}
