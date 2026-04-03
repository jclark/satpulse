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
	hasPayload := len(data) > 10
	if mid == casbin.CfgRstID {
		return requestAnalysis{
			expectAck:  ExpectAckNone,
			expectData: expectDataNone,
		}
	}
	if mid == casbin.CfgMsgID {
		if !hasPayload {
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
		if hasPayload {
			a.expectData = expectDataNone
		} else {
			a.expectData = expectDataSingle
			a.dataMatch = casbinDataMatch(corr)
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
	// CFG queries use short payloads (0-1 bytes); sets use longer payloads.
	isQuery := sm.Class == 0x05
	if !isQuery {
		isQuery = payloadLen <= 1
	}
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
