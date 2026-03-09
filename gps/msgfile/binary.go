package msgfile

import (
	"errors"
	"fmt"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/lib/casbin"
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
	return RawMsg{
		Bytes: b,
		Delay: delay,
		Tag:   *bm.Tag,
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
	return RawMsg{Bytes: pkt, Delay: delay, Tag: *cm.Tag}, nil
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
	return RawMsg{Bytes: pkt, Delay: delay, Tag: *am.Tag}, nil
}

func (am *ASBINMsg) getTag() string { return *am.Tag }

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
	return RawMsg{Bytes: pkt, Delay: delay, Tag: *um.Tag}, nil
}

func (um *UBXMsg) getTag() string { return *um.Tag }

type ackType int

const (
	notAck ackType = iota
	isAckAck
	isAckNak
)

type packetInfo[M comparable] struct {
	msgID   M
	ack     ackType
	ackedID M // valid only when ack != notAck
}

// ubxLikeMatcher handles UBX, CASBIN, and ASBIN matchers which all follow
// the same pattern: check tag, parse, match ACK class/ID or message class/ID.
// Only CFG class messages receive ACK/NAK replies.
type ubxLikeMatcher[M comparable] struct {
	expectedTag gpsprot.Tag
	sentMsgID   M
	expectAck   bool
	parse       func(string) (packetInfo[M], error)
}

func (m *ubxLikeMatcher[M]) match(tag gpsprot.Tag, data string) (ResponseKind, string) {
	if tag != m.expectedTag {
		return NotResponse, ""
	}
	p, err := m.parse(data)
	if err != nil {
		return NotResponse, ""
	}
	if m.expectAck && p.ackedID == m.sentMsgID {
		switch p.ack {
		case isAckAck:
			return AckResponse, ""
		case isAckNak:
			return AckResponse, AckNak
		}
	}
	if p.msgID == m.sentMsgID {
		return OtherResponse, ""
	}
	return NotResponse, ""
}

func parseUBX(data string) (packetInfo[ubxbin.MsgID], error) {
	if len(data) < 8 {
		return packetInfo[ubxbin.MsgID]{}, fmt.Errorf("too short")
	}
	p := packetInfo[ubxbin.MsgID]{msgID: ubxbin.PacketMsgId(data)}
	msg, _ := ubxbin.ParseMsg(data)
	switch m := msg.(type) {
	case *ubxbin.AckAck:
		p.ack = isAckAck
		p.ackedID = m.MsgID
	case *ubxbin.AckNak:
		p.ack = isAckNak
		p.ackedID = m.MsgID
	}
	return p, nil
}

func parseCASBIN(data string) (packetInfo[casbin.MsgID], error) {
	if len(data) < 10 {
		return packetInfo[casbin.MsgID]{}, fmt.Errorf("too short")
	}
	p := packetInfo[casbin.MsgID]{msgID: casbin.MakeMsgID(data[4], data[5])}
	msg, _ := casbin.ParseMsg(data)
	switch m := msg.(type) {
	case *casbin.AckAck:
		p.ack = isAckAck
		p.ackedID = casbin.MakeMsgID(m.ClsID, m.MsgID)
	case *casbin.AckNak:
		p.ack = isAckNak
		p.ackedID = casbin.MakeMsgID(m.ClsID, m.MsgID)
	}
	return p, nil
}

func parseASBIN(data string) (packetInfo[asbin.MsgID], error) {
	if len(data) < 8 {
		return packetInfo[asbin.MsgID]{}, fmt.Errorf("too short")
	}
	p := packetInfo[asbin.MsgID]{msgID: asbin.MakeMsgID(data[2], data[3])}
	msg, _ := asbin.ParseMsg(data)
	switch m := msg.(type) {
	case *asbin.AckAck:
		p.ack = isAckAck
		p.ackedID = asbin.MakeMsgID(m.MsgClass, m.MsgID)
	case *asbin.AckNak:
		p.ack = isAckNak
		p.ackedID = asbin.MakeMsgID(m.MsgClass, m.MsgID)
	}
	return p, nil
}

func (um *UBXMsg) newMatcher() responseMatcher {
	mid := ubxbin.MakeMsgID(um.Class, um.ID)
	return &ubxLikeMatcher[ubxbin.MsgID]{
		expectedTag: gpsreg.TagUBX,
		sentMsgID:   mid,
		expectAck:   mid.CfgClass(),
		parse:       parseUBX,
	}
}

func (cm *CASBINMsg) newMatcher() responseMatcher {
	mid := casbin.MakeMsgID(cm.Class, cm.ID)
	if mid == casbin.CfgMsgID && len(cm.Payload.Values) >= 3 {
		rate, _ := cm.Payload.Values[2].(int64)
		if rate == 0xFFFF {
			cls, _ := cm.Payload.Values[0].(int64)
			id, _ := cm.Payload.Values[1].(int64)
			return &casbinPollMatcher{
				sentMsgID:   mid,
				polledMsgID: casbin.MakeMsgID(byte(cls), byte(id)),
			}
		}
	}
	return &ubxLikeMatcher[casbin.MsgID]{
		expectedTag: gpsreg.TagCASICBin,
		sentMsgID:   mid,
		expectAck:   mid.CfgClass(),
		parse:       parseCASBIN,
	}
}

// casbinPollMatcher handles CASIC CFG-MSG polls (rate=0xFFFF).
// CASIC polls via CFG-MSG; the response is an ACK followed by the polled message.
type casbinPollMatcher struct {
	sentMsgID   casbin.MsgID // CFG-MSG, for ACK matching
	polledMsgID casbin.MsgID // the message being polled
}

func (m *casbinPollMatcher) match(tag gpsprot.Tag, data string) (ResponseKind, string) {
	if tag != gpsreg.TagCASICBin {
		return NotResponse, ""
	}
	p, err := parseCASBIN(data)
	if err != nil {
		return NotResponse, ""
	}
	if p.ackedID == m.sentMsgID {
		switch p.ack {
		case isAckAck:
			return AckResponse, ""
		case isAckNak:
			return AckResponse, AckNak
		}
	}
	if p.msgID == m.polledMsgID {
		return OtherResponse, ""
	}
	return NotResponse, ""
}

func (am *ASBINMsg) newMatcher() responseMatcher {
	mid := asbin.MakeMsgID(am.Class, am.ID)
	return &ubxLikeMatcher[asbin.MsgID]{
		expectedTag: gpsreg.TagAllystarBin,
		sentMsgID:   mid,
		expectAck:   mid.CfgClass(),
		parse:       parseASBIN,
	}
}

// binaryTags maps binary protocol tags used for response classification.
var binaryTags = map[gpsprot.Tag]bool{
	gpsreg.TagUBX:         true,
	gpsreg.TagCASICBin:    true,
	gpsreg.TagAllystarBin: true,
}

// binaryMatcher handles BinaryMsg responses.
type binaryMatcher struct{}

func (bm *BinaryMsg) newMatcher() responseMatcher {
	return &binaryMatcher{}
}

func (m *binaryMatcher) match(tag gpsprot.Tag, data string) (ResponseKind, string) {
	// Text-based packets are not responses to raw binary.
	switch tag {
	case gpsreg.TagNMEA, gpsreg.TagUnicoreAscii, gpsreg.TagNovAtelAscii:
		return NotResponse, ""
	}
	return MaybeResponse, ""
}
