// Package rtcmbin provides RTCM binary field extraction functions.
package rtcmbin

import "fmt"

// Bytes constrains types to string or []byte.
type Bytes interface {
	string | []byte
}

// MsgType is an RTCM message type number.
type MsgType uint16

func (mt MsgType) String() string {
	return fmt.Sprintf("%d", mt)
}

// IsMSM reports whether mt is a Multiple Signal Message type
// (1071-1077, 1081-1087, ..., 1131-1137).
func (mt MsgType) IsMSM() bool {
	msm := mt % 10
	return mt >= 1071 && mt <= 1137 && msm >= 1 && msm <= 7
}

func (mt MsgType) hasStationID() bool {
	switch mt {
	case 1005, 1006, 1007, 1008, 1033, 1230:
		return true
	}
	return mt.IsMSM()
}

// ExtractMsgType returns the 12-bit message type from a packet.
func ExtractMsgType[B Bytes](pkt B) MsgType {
	if len(pkt) <= 6 {
		return 0
	}
	return MsgType(pkt[3])<<4 | MsgType(pkt[4])>>4
}

// Message represents a parsed RTCM message.
type Message struct {
	Payload string
	MsgType MsgType
}

// ParseMessage parses a packet into an RTCM Message.
func ParseMessage(packet string) *Message {
	return &Message{
		Payload: packet[3 : len(packet)-3],
		MsgType: ExtractMsgType(packet),
	}
}

// MultipleMessageBit extracts the MSM Multiple Message Bit from an
// RTCM packet.  ok is false for non-MSM messages or packets that are
// too short.  When ok is true, mmb indicates whether more MSM messages
// follow for this epoch.
func MultipleMessageBit[B Bytes](pkt B) (mmb, ok bool) {
	mt := ExtractMsgType(pkt)
	if !mt.IsMSM() || len(pkt) < 10 {
		return false, false
	}
	return pkt[9]&0x02 != 0, true
}

// ReferenceStationID extracts the 12-bit reference station ID (DF003)
// from an RTCM packet.  Returns false if the message type does not
// have a station ID at this offset or the packet is too short.
func ReferenceStationID[B Bytes](pkt B) (uint16, bool) {
	mt := ExtractMsgType(pkt)
	if !mt.hasStationID() || len(pkt) < 6 {
		return 0, false
	}
	return uint16(pkt[4]&0x0F)<<8 | uint16(pkt[5]), true
}
