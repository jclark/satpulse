package msgfile

import (
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// UBXLikeMsg contains fields shared by UBX, CASBIN, ASBIN, and SDBP message types.
type UBXLikeMsg struct {
	Class   uint8   `toml:"class"`
	ID      uint8   `toml:"id"`
	Payload Payload `toml:"payload"`
	MsgCommon
}

// UBXMsg represents a [[ubx]] entry.
type UBXMsg struct {
	UBXLikeMsg
}

func (um *UBXMsg) toRaw() (RawMsg, error) {
	payload, err := um.Payload.Encode(ubxbin.Endian)
	if err != nil {
		return RawMsg{}, err
	}
	mid := ubxbin.MakeMsgID(um.Class, um.ID)
	pkt, err := ubxbin.PackMsg(mid, payload)
	if err != nil {
		return RawMsg{}, err
	}
	delay, err := um.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := um.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{Bytes: pkt, Delay: delay, WaitLimit: wl, Tag: *um.Tag}, nil
}

func (um *UBXMsg) getTag() string { return *um.Tag }

// ubxAnalyzer handles both request and response analysis for UBX.
type ubxAnalyzer struct{}

func (ubxAnalyzer) analyzeResponse(data string) responseAnalysis {
	if len(data) < ubxbin.PacketMinLen {
		return responseAnalysis{kind: responseMaybeData}
	}
	mid := ubxbin.PacketMsgId(data)
	switch mid {
	case ubxbin.AckAckID:
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: ubxMsgIDString(ubxbin.AckMsgID(data)),
		}
	case ubxbin.AckNakID:
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: ubxMsgIDString(ubxbin.AckMsgID(data)),
		}
	}
	return responseAnalysis{kind: responseMaybeData}
}

func ubxMsgIDString(mid ubxbin.MsgID) string {
	cls, id := mid.Unpack()
	return string([]byte{cls, id})
}

func (um *UBXMsg) analyzeRequest(data string) requestAnalysis {
	mid := ubxbin.MakeMsgID(um.Class, um.ID)
	corr := ubxMsgIDString(ubxbin.PacketMsgId(data))
	payloadLen := len(data) - ubxbin.PacketMinLen
	if mid.CfgClass() {
		if !mid.Ackable() {
			return requestAnalysis{
				ackTag:       gpsreg.TagUBX,
				ackCorrelate: corr,
				expectAck:    ExpectAckNakOnly,
				expectData:   expectDataNone,
			}
		}
		a := requestAnalysis{
			ackTag:       gpsreg.TagUBX,
			ackCorrelate: corr,
			expectAck:    ExpectAckOrNak,
			dataTag:      gpsreg.TagUBX,
		}
		if payloadLen <= ubxbin.PollPayloadLen(mid) {
			a.expectData = expectDataSingle
			a.dataMatch = ubxDataMatch(mid)
		} else {
			a.expectData = expectDataNone
		}
		return a
	}
	// Non-CFG poll. Poll-only messages use expectDataSingle;
	// periodic/polled messages use expectDataAmbig.
	de := expectDataAmbig
	if mid == ubxbin.MonVerID || mid == ubxbin.MonGnssID {
		de = expectDataSingle
	}
	return requestAnalysis{
		expectAck:  ExpectAckNone,
		dataTag:    gpsreg.TagUBX,
		expectData: de,
		dataMatch:  ubxDataMatch(mid),
	}
}

func ubxDataMatch(mid ubxbin.MsgID) func(string) bool {
	return func(data string) bool {
		return ubxbin.PacketMsgId(data) == mid
	}
}
