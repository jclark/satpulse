// Package rtcmbin provides RTCM binary packet parsing, serialization,
// and field extraction.
package rtcmbin

import (
	"fmt"

	"github.com/jclark/crc24q"
	"github.com/jclark/satpulse/gps/lib/bitsenc"
)

// PreambleByte is the RTCM preamble byte.
const PreambleByte = 0xD3

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

// MsgHdrStationID is the header for RTCM messages that carry DF003
// (Reference Station ID).  Embedding this instead of MsgHdr both
// declares the field and satisfies the StationIDer interface, so
// ReferenceStationID can find it without a per-MsgType allow-list.
type MsgHdrStationID struct {
	MsgHdr
	StationID uint16 `bits:"12" json:"stationID"`
}

// RefStationID returns the 12-bit DF003 Reference Station ID.
func (h *MsgHdrStationID) RefStationID() uint16 { return h.StationID }

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
	case 1001:
		return parseBits[MT1001](mt, payload)
	case 1002:
		return parseBits[MT1002](mt, payload)
	case 1003:
		return parseBits[MT1003](mt, payload)
	case 1004:
		return parseBits[MT1004](mt, payload)
	case 1005:
		return parseBits[MT1005](mt, payload)
	case 1006:
		return parseBits[MT1006](mt, payload)
	case 1007:
		return parseBits[MT1007](mt, payload)
	case 1008:
		return parseBits[MT1008](mt, payload)
	case 1009:
		return parseBits[MT1009](mt, payload)
	case 1010:
		return parseBits[MT1010](mt, payload)
	case 1011:
		return parseBits[MT1011](mt, payload)
	case 1012:
		return parseBits[MT1012](mt, payload)
	case 1013:
		return parseBits[MT1013](mt, payload)
	case 1033:
		return parseBits[MT1033](mt, payload)
	case 1230:
		return parseBits[MT1230](mt, payload)
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

// parseBits decodes a fixed RTCM message type T from payload using the
// bitsenc codec.  The PT constraint requires *T to satisfy Msg, so
// embedding MsgHdr (directly or via MsgHdrStationID) is sufficient.
func parseBits[T any, PT interface {
	*T
	Msg
}](mt MsgType, payload string) (Msg, error) {
	var msg T
	pmsg := PT(&msg)
	if err := bitsenc.NewReader([]byte(payload)).Read(pmsg); err != nil {
		return nil, fmt.Errorf("rtcm %d: %w", mt, err)
	}
	return pmsg, nil
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

// SerializeMsg serializes a Msg into a complete RTCM packet.
func SerializeMsg(msg Msg) (string, error) {
	var payload []byte
	if u, ok := msg.(*UnknownMsg); ok {
		payload = []byte(u.Payload)
	} else {
		var err error
		payload, err = bitsenc.Write(msg)
		if err != nil {
			return "", err
		}
	}
	pkt, err := PackMsg(payload)
	if err != nil {
		return "", err
	}
	return string(pkt), nil
}

// PackMsg frames a payload as a complete RTCM packet
// (preamble + 10-bit length + payload + CRC-24Q).
func PackMsg(payload []byte) ([]byte, error) {
	n := len(payload)
	if n > 1023 {
		return nil, fmt.Errorf("rtcmbin: payload too long (%d bytes)", n)
	}
	pkt := make([]byte, 3+n+3)
	pkt[0] = PreambleByte
	pkt[1] = byte(n >> 8)
	pkt[2] = byte(n)
	copy(pkt[3:], payload)
	crc := Checksum(pkt[:3+n])
	pkt[3+n] = crc[0]
	pkt[3+n+1] = crc[1]
	pkt[3+n+2] = crc[2]
	return pkt, nil
}

// Checksum computes the CRC-24Q checksum over data.
func Checksum(data []byte) [3]byte {
	c := crc24q.Checksum(data)
	return [3]byte{byte(c >> 16), byte(c >> 8), byte(c)}
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

// StationIDer is implemented by message types that carry DF003
// (Reference Station ID).  Typed callers that have already parsed a
// packet can type-assert against this interface; ReferenceStationID
// uses a byte-level fast path instead, so it also works for DF003-
// bearing packets whose payload we don't parse into a typed struct.
type StationIDer interface {
	RefStationID() uint16
}

// hasStationID reports whether mt carries DF003 as its second 12-bit
// field.  Keep in sync with the set of message types whose typed struct
// embeds MsgHdrStationID, plus any DF003-bearing types not yet modelled.
func (mt MsgType) hasStationID() bool {
	switch mt {
	case 1001, 1002, 1003, 1004,
		1005, 1006, 1007, 1008,
		1009, 1010, 1011, 1012, 1013,
		1033, 1230:
		return true
	}
	return mt.IsMSM()
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
