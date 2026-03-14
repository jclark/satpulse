// Package rtcmbin provides RTCM binary field extraction functions.
package rtcmbin

// Bytes constrains types to string or []byte.
type Bytes interface {
	string | []byte
}

func extractMsgType[B Bytes](pkt B) uint16 {
	if len(pkt) <= 6 {
		return 0
	}
	return uint16(pkt[3])<<4 | uint16(pkt[4])>>4
}

func isMSM(mt uint16) bool {
	msm := mt % 10
	return mt >= 1071 && mt <= 1137 && msm >= 1 && msm <= 7
}

// MultipleMessageBit extracts the MSM Multiple Message Bit from an
// RTCM packet.  ok is false for non-MSM messages or packets that are
// too short.  When ok is true, mmb indicates whether more MSM messages
// follow for this epoch.
func MultipleMessageBit[B Bytes](pkt B) (mmb, ok bool) {
	mt := extractMsgType(pkt)
	if !isMSM(mt) || len(pkt) < 10 {
		return false, false
	}
	return pkt[9]&0x02 != 0, true
}

func hasStationID(mt uint16) bool {
	switch mt {
	case 1005, 1006, 1007, 1008, 1033, 1230:
		return true
	}
	return isMSM(mt)
}

// ReferenceStationID extracts the 12-bit reference station ID (DF003)
// from an RTCM packet.  Returns false if the message type does not
// have a station ID at this offset or the packet is too short.
func ReferenceStationID[B Bytes](pkt B) (uint16, bool) {
	mt := extractMsgType(pkt)
	if !hasStationID(mt) || len(pkt) < 6 {
		return 0, false
	}
	return uint16(pkt[4]&0x0F)<<8 | uint16(pkt[5]), true
}
