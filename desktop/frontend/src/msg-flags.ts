// Source: gps/gpsprot/configtarget.go -- keep in sync.

// NMEA
export const NMEAMsgRMC   = 1 << 0
export const NMEAMsgGGA   = 1 << 1
export const NMEAMsgGSA   = 1 << 2
export const NMEAMsgGSV   = 1 << 3
export const NMEAMsgZDA   = 1 << 4
export const NMEAMsgVTG   = 1 << 5
export const NMEAMsgGLL   = 1 << 6
export const NMEAMsgOther = 1 << 15

// RTCM
export const RTCMMsgMSM4  = 1 << 0
export const RTCMMsgMSM7  = 1 << 3
export const RTCMMsgARP   = 1 << 4
export const RTCMMsgLax   = 1 << 5
export const RTCMMsgOther = 1 << 15

// PVT
export const PVTMsgPos            = 1 << 0
export const PVTMsgVel            = 1 << 1
export const PVTMsgTime           = 1 << 2
export const PVTMsgTimePulse      = 1 << 3
export const PVTMsgLeapSecond     = 1 << 4
export const PVTMsgSurvey         = 1 << 5
export const PVTMsgTAI            = 1 << 6
export const PVTMsgECEF           = 1 << 7
export const PVTMsgTimePulseAfter = 1 << 8
export const PVTMsgOff            = 1 << 9

// Sats
export const SatsMsgSat    = 1 << 0
export const SatsMsgSignal = 1 << 1

// Raw
export const RawMsgObs     = 1 << 0
export const RawMsgNavData = 1 << 1
