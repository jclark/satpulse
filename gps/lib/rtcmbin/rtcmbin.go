// Package rtcmbin provides RTCM binary packet parsing and field extraction.
package rtcmbin

import (
	"fmt"

	"github.com/jclark/satpulse/gps/lib/bitsenc"
)

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

// Msg is the interface implemented by all parsed RTCM messages.
type Msg interface {
	MsgType() MsgType
}

// MsgHdr is the common header for all RTCM messages.
// Embed it to satisfy the Msg interface.
type MsgHdr struct {
	MsgNum MsgType `bits:"12" json:"msgNum"`
}

// MsgType returns the message type number.
func (h *MsgHdr) MsgType() MsgType { return h.MsgNum }

// UnknownMsg represents an RTCM message with an unrecognized message type.
type UnknownMsg struct {
	MsgHdr
	Payload string `json:"payload"`
}

// ParseMsg parses an RTCM packet into a typed message struct.
// Returns *UnknownMsg for unrecognized message types.
func ParseMsg(packet string) (Msg, error) {
	if len(packet) < 8 {
		return nil, fmt.Errorf("rtcmbin: packet too short")
	}
	payload := packet[3 : len(packet)-3]
	mt := ExtractMsgType(packet)
	switch mt {
	case 1005:
		var msg MT1005
		if err := bitsenc.Read([]byte(payload), &msg); err != nil {
			return nil, fmt.Errorf("rtcm 1005: %w", err)
		}
		return &msg, nil
	case 1006:
		var msg MT1006
		if err := bitsenc.Read([]byte(payload), &msg); err != nil {
			return nil, fmt.Errorf("rtcm 1006: %w", err)
		}
		return &msg, nil
	case 1230:
		var msg MT1230
		if err := bitsenc.NewReader([]byte(payload)).Read(&msg); err != nil {
			return nil, fmt.Errorf("rtcm 1230: %w", err)
		}
		return &msg, nil
	default:
		if mt.IsMSM() {
			return parseMSM(mt, payload)
		}
		return &UnknownMsg{
			MsgHdr:  MsgHdr{MsgNum: mt},
			Payload: payload,
		}, nil
	}
}

func parseMSM(mt MsgType, payload string) (Msg, error) {
	r := bitsenc.NewReader([]byte(payload))
	if int(mt%10) >= 6 {
		var msg MSMHiRes
		if err := r.Read(&msg); err != nil {
			return nil, fmt.Errorf("rtcm MSM %d: %w", mt, err)
		}
		return &msg, nil
	}
	var msg MSM
	if err := r.Read(&msg); err != nil {
		return nil, fmt.Errorf("rtcm MSM %d: %w", mt, err)
	}
	return &msg, nil
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
