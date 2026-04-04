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
	if len(data) < 8 {
		return responseAnalysis{kind: responseMaybeData}
	}
	mid := sdbpbin.PacketMsgId(data)
	switch mid {
	case sdbpbin.PubAckID:
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: sdbpAckCorrelate(data),
		}
	case sdbpbin.PubNakID:
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: sdbpAckCorrelate(data),
		}
	}
	return responseAnalysis{kind: responseMaybeData}
}

// sdbpAckCorrelate extracts the 2-byte class/ID from an SDBP PubAck/PubNak payload.
// SDBP packet: sync(2) + class(1) + id(1) + len(2) + payload + cksum(2).
// PubAck/PubNak payload starts at byte 6: acked_class(1) + acked_id(1).
func sdbpAckCorrelate(data string) string {
	if len(data) < 8 {
		return ""
	}
	return data[6:8]
}

// sdbpMsgCorrelate extracts the 2-byte class/ID from an SDBP packet header.
func sdbpMsgCorrelate(data string) string {
	if len(data) < 4 {
		return ""
	}
	return data[2:4]
}

func (sm *SDBPMsg) analyzeRequest(data string) requestAnalysis {
	mid := sdbpbin.MakeMsgID(sm.Class, sm.ID)
	corr := sdbpMsgCorrelate(data)
	payloadLen := len(data) - 8 // 8 bytes overhead
	switch mid {
	case sdbpbin.CtlRestartID, sdbpbin.CtlStandbyID:
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
			dataMatch:    sdbpDataMatch(corr),
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

func sdbpDataMatch(corr string) func(string) bool {
	return func(data string) bool {
		return sdbpMsgCorrelate(data) == corr
	}
}
