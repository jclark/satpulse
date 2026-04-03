package msgfile

import (
	"errors"
	"fmt"

	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/casbin"
	"github.com/jclark/satpulse/gps/lib/sdbpbin"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

// BinaryMsg represents a [[binary]] entry or [default.binary].
type BinaryMsg struct {
	Hex string `toml:"hex"`
	MsgCommon
}

func (bm *BinaryMsg) toRaw() (RawMsg, error) {
	if bm.Hex == "" {
		return RawMsg{}, errors.New("hex must not be empty")
	}
	b, err := decodeHex(bm.Hex)
	if err != nil {
		return RawMsg{}, fmt.Errorf("hex: %w", err)
	}
	delay, err := bm.MsgCommon.delay()
	if err != nil {
		return RawMsg{}, err
	}
	wl, err := bm.MsgCommon.waitLimit()
	if err != nil {
		return RawMsg{}, err
	}
	return RawMsg{
		Bytes:     b,
		Delay:     delay,
		WaitLimit: wl,
		Tag:       *bm.Tag,
	}, nil
}

func (bm *BinaryMsg) getTag() string { return *bm.Tag }

// UBXLikeMsg contains fields shared by UBX, CASBIN, and ASBIN message types.
type UBXLikeMsg struct {
	Class   uint8   `toml:"class"`
	ID      uint8   `toml:"id"`
	Payload Payload `toml:"payload"`
	MsgCommon
}

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
	if len(data) < 8 {
		return responseAnalysis{kind: responseMaybeData}
	}
	mid := ubxbin.PacketMsgId(data)
	switch mid {
	case ubxbin.AckAckID:
		return responseAnalysis{
			kind:         responseAck,
			ackCorrelate: ubxAckCorrelate(data),
		}
	case ubxbin.AckNakID:
		return responseAnalysis{
			kind:         responseNak,
			ackCorrelate: ubxAckCorrelate(data),
		}
	}
	return responseAnalysis{kind: responseMaybeData}
}

// ubxAckCorrelate extracts the 2-byte class/ID from a UBX ACK payload.
func ubxAckCorrelate(data string) string {
	if len(data) < 8 {
		return ""
	}
	return data[6:8]
}

// ubxMsgCorrelate extracts the 2-byte class/ID from a UBX packet header.
func ubxMsgCorrelate(data string) string {
	if len(data) < 4 {
		return ""
	}
	return data[2:4]
}

func (um *UBXMsg) analyzeRequest(data string) requestAnalysis {
	mid := ubxbin.MakeMsgID(um.Class, um.ID)
	corr := ubxMsgCorrelate(data)
	hasPayload := len(data) > 8
	if mid == ubxbin.CfgRstID {
		return requestAnalysis{
			expectAck:  ExpectAckNone,
			expectData: expectDataNone,
		}
	}
	if mid.CfgClass() {
		a := requestAnalysis{
			ackTag:       gpsreg.TagUBX,
			ackCorrelate: corr,
			expectAck:    ExpectAckOrNak,
			dataTag:      gpsreg.TagUBX,
		}
		if hasPayload {
			a.expectData = expectDataNone
		} else {
			a.expectData = expectDataSingle
			a.dataMatch = ubxDataMatch(corr)
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
		dataMatch:  ubxDataMatch(corr),
	}
}

func ubxDataMatch(corr string) func(string) bool {
	return func(data string) bool {
		return ubxMsgCorrelate(data) == corr
	}
}
