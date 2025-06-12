package bin

const (
	// following are used for enabling/disabling messages
	RxmRawID   MsgID = clsRxm | (0x10 << 8)
	RxmSfrbID  MsgID = clsRxm | (0x12 << 8)
	RxmSfrbxID MsgID = clsRxm | (0x13 << 8)
	RxmRawxID  MsgID = clsRxm | (0x15 << 8)
	// it's just the message type - 1000
	Rtcm1005ID MsgID = clsRtcm | (0x05 << 8)
	Rtcm1006ID MsgID = clsRtcm | (0x06 << 8)
	Rtcm1007ID MsgID = clsRtcm | (0x07 << 8)
	Rtcm1074ID MsgID = clsRtcm | (0x4A << 8)
	Rtcm1077ID MsgID = clsRtcm | (0x4D << 8)
	Rtcm1084ID MsgID = clsRtcm | (0x54 << 8)
	Rtcm1087ID MsgID = clsRtcm | (0x57 << 8)
	Rtcm1094ID MsgID = clsRtcm | (0x5E << 8)
	Rtcm1097ID MsgID = clsRtcm | (0x61 << 8)
	Rtcm1124ID MsgID = clsRtcm | (0x7C << 8)
	Rtcm1127ID MsgID = clsRtcm | (0x7F << 8)
	Rtcm1230ID MsgID = clsRtcm | (0xE6 << 8)
	// NMEA message IDs
	NmeaGgaID MsgID = clsNmea | (0x00 << 8)
	NmeaGllID MsgID = clsNmea | (0x01 << 8)
	NmeaGsaID MsgID = clsNmea | (0x02 << 8)
	NmeaGsvID MsgID = clsNmea | (0x03 << 8)
	NmeaRmcID MsgID = clsNmea | (0x04 << 8)
	NmeaVtgID MsgID = clsNmea | (0x05 << 8)
	NmeaZdaID MsgID = clsNmea | (0x08 << 8)
	NmeaGnsID MsgID = clsNmea | (0x0D << 8)
	// NmeaTxtID is intentionally omitted because it is not a navigation message
	// and is emitted spontaneously. Gen9 does not allow configuration of it.
)

func RTCMMsgID(msgType int) (MsgID, bool) {
	if msgType > 1000 && msgType < 1256 {
		return MsgID(clsRtcm | ((msgType - 1000) << 8)), true
	}
	return 0, false
}

func init() {
	// not implemented
	idNameMap[RxmRawID] = "RAW"
	idNameMap[RxmSfrbID] = "SFRB"
	idNameMap[RxmSfrbxID] = "SFRBX"
	idNameMap[RxmRawxID] = "RAWX"
	idNameMap[Rtcm1005ID] = "1005"
	idNameMap[Rtcm1074ID] = "1074"
	idNameMap[Rtcm1077ID] = "1077"
	idNameMap[Rtcm1084ID] = "1084"
	idNameMap[Rtcm1087ID] = "1087"
	idNameMap[Rtcm1124ID] = "1124"
	idNameMap[Rtcm1127ID] = "1127"
	idNameMap[Rtcm1230ID] = "1230"
	idNameMap[NmeaGgaID] = "GGA"
	idNameMap[NmeaGllID] = "GLL"
	idNameMap[NmeaGsaID] = "GSA"
	idNameMap[NmeaGsvID] = "GSV"
	idNameMap[NmeaRmcID] = "RMC"
	idNameMap[NmeaVtgID] = "VTG"
	idNameMap[NmeaZdaID] = "ZDA"
	idNameMap[NmeaGnsID] = "GNS"
}