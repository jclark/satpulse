package ubx

import (
	"errors"
	"iter"
	"slices"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/rtcm"
	"github.com/jclark/satpulse/internal/ubx/bin"
	ucv "github.com/jclark/satpulse/internal/ubxcfgval"
)

type MsgRate byte

type msgChanges struct {
	rate         map[bin.MsgID]MsgRate
	protoEnable  bin.CfgPrtProtoMask
	protoDisable bin.CfgPrtProtoMask
}

func newMsgChanges() *msgChanges {
	return &msgChanges{
		rate: make(map[bin.MsgID]MsgRate),
	}
}

func (m *msgChanges) rates() iter.Seq2[bin.MsgID, MsgRate] {
	return func(yield func(bin.MsgID, MsgRate) bool) {
		keys := make([]bin.MsgID, 0, len(m.rate))
		for k := range m.rate {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b bin.MsgID) int {
			return int(a) - int(b)
		})
		for _, k := range keys {
			if !yield(k, m.rate[k]) {
				return
			}
		}
	}
}

func (mc *msgChanges) changeOutProtoMask(mask bin.CfgPrtProtoMask) bin.CfgPrtProtoMask {
	return (mask &^ mc.protoDisable) | mc.protoEnable
}

// usesRates returns true if any message rate is non-zero.
// Semantically our rates our in Hz, so if this is true then we
// need to ensure that a 1 here means 1Hz.
func (mc *msgChanges) usesRate() bool {
	for _, rate := range mc.rate {
		if rate != 0 {
			return true
		}
	}
	return false
}

func (m *msgChanges) options(opts *gpsprot.ConfigOptions, ver *Version, enabledGNSS gpsprot.GNSSSet) error {
	if opts.PVTMsg.IsSet() {
		m.pvt(opts.PVTMsg.Get(), ver)
	}
	if opts.SatsMsg.IsSet() {
		m.sats(opts.SatsMsg.Get(), ver)
	}
	if opts.NMEAMsg.IsSet() {
		m.nmea(opts.NMEAMsg.Get(), ver)
	}
	if opts.RawMsg.IsSet() {
		err := m.raw(opts.RawMsg.Get(), ver)
		if err != nil {
			return err
		}
	}
	if opts.RTCMMsg.IsSet() {
		err := m.rtcm(opts.RTCMMsg.Get(), ver, enabledGNSS)
		if err != nil {
			return err
		}
	}
	return nil
}

func (mc *msgChanges) items(port ucv.Port) []ucv.Item {
	items := []ucv.Item{}
	if (mc.protoEnable|mc.protoDisable)&bin.CfgPrtProtoNMEA != 0 {
		ucv.AddItem(&items, portOutprotNmeaKey(port), mc.protoEnable&bin.CfgPrtProtoNMEA != 0)
	}
	if (mc.protoEnable|mc.protoDisable)&bin.CfgPrtProtoRTCM3 != 0 {
		ucv.AddItem(&items, portOutprotRtcm3xKey(port), mc.protoEnable&bin.CfgPrtProtoRTCM3 != 0)
	}
	for mid, rate := range mc.rate {
		km, ok := msgIDKey[mid]
		if !ok {
			continue
		}
		ucv.AddItem(&items, km.KeyU(port), uint64(rate))
	}
	return items
}

func (mc *msgChanges) pvt(flags gpsprot.PVTMsgFlags, ver *Version) {
	off := flags&gpsprot.PVTMsgOff != 0
	navPVTSupported := ver.protVerAtLeast(15, 0)
	fts := ver.ProductCategory() == "FTS"
	// these are the messages we might enable
	timTOS := false
	timTP := false
	navTimeGPS := false
	navTimeUTC := false
	navPosECEF := false
	navPosLLH := false
	navVelECEF := false
	navVelNED := false
	navPVT := false
	navTimeLS := flags&gpsprot.PVTMsgLeapSecond != 0
	if flags&gpsprot.PVTMsgPos != 0 && flags&gpsprot.PVTMsgECEF != 0 {
		navPosECEF = true
		flags &^= gpsprot.PVTMsgPos | gpsprot.PVTMsgECEF
	}
	if flags&gpsprot.PVTMsgVel != 0 && flags&gpsprot.PVTMsgECEF != 0 {
		navVelECEF = true
		flags &^= gpsprot.PVTMsgVel | gpsprot.PVTMsgECEF
	}
	if fts && flags&gpsprot.PVTMsgTimePulse != 0 {
		timTOS = true
		flags &^= gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI
	}
	if flags&gpsprot.PVTMsgTimePulse != 0 {
		timTP = true
		flags &^= gpsprot.PVTMsgTimePulse
		// TimePulseAfter is just like Time, except on FTS
		if flags&gpsprot.PVTMsgTimePulseAfter != 0 {
			flags |= gpsprot.PVTMsgTime
		}
	}
	flags &^= gpsprot.PVTMsgTimePulseAfter
	if flags&gpsprot.PVTMsgTAI != 0 && flags&gpsprot.PVTMsgTime != 0 {
		navTimeGPS = true
		flags &^= gpsprot.PVTMsgTime | gpsprot.PVTMsgTAI
	}
	if navPVTSupported {
		nPVT := 0
		if flags&gpsprot.PVTMsgTime != 0 {
			nPVT++
		}
		if flags&gpsprot.PVTMsgPos != 0 {
			nPVT++
		}
		if flags&gpsprot.PVTMsgVel != 0 {
			nPVT++
		}
		if nPVT >= 2 {
			navPVT = true
			flags &^= gpsprot.PVTMsgTime | gpsprot.PVTMsgPos | gpsprot.PVTMsgVel
		}
	}
	if flags&gpsprot.PVTMsgTime != 0 {
		navTimeUTC = true
		flags &^= gpsprot.PVTMsgTime
	}
	if flags&gpsprot.PVTMsgPos != 0 {
		navPosLLH = true
		flags &^= gpsprot.PVTMsgPos
	}
	if flags&gpsprot.PVTMsgVel != 0 {
		navVelNED = true
		flags &^= gpsprot.PVTMsgVel
	}
	if fts {
		mc.pvtMsg(bin.TimTosID, timTOS, off)
	} else {
		mc.pvtMsg(bin.TimTPID, timTP, off)
	}
	if navPVTSupported {
		mc.pvtMsg(bin.NavPVTID, navPVT, off)
	}
	mc.pvtMsg(bin.NavTimeGPSID, navTimeGPS, off)
	mc.pvtMsg(bin.NavTimeUTCID, navTimeUTC, off)
	mc.pvtMsg(bin.NavPosECEFID, navPosECEF, off)
	mc.pvtMsg(bin.NavPosLLHID, navPosLLH, off)
	mc.pvtMsg(bin.NavVelECEFID, navVelECEF, off)
	mc.pvtMsg(bin.NavVelNEDID, navVelNED, off)
	if ver.protVerAtLeast(18, 0) {
		mc.pvtMsg(bin.NavTimeLSID, navTimeLS, off)
	}
}

func (mc *msgChanges) pvtMsg(msgID bin.MsgID, enable, off bool) {
	rate := MsgRate(0)
	if enable {
		rate = 1
	}
	if rate != 0 || off {
		mc.rate[msgID] = rate
	}
}

func (m *msgChanges) sats(flags gpsprot.SatsMsgFlags, ver *Version) {
	msgID := bin.NavSVInfoID
	// UBX-NAV-SAT first appeared in protocol version 15.00
	if ver.protVerAtLeast(15, 0) {
		msgID = bin.NavSatID
	}
	rate := MsgRate(0)
	if flags&gpsprot.SatsMsgSV != 0 {
		rate = 1
	}
	m.rate[msgID] = rate
}

func (m *msgChanges) nmea(flags gpsprot.NMEAMsgFlags, _ *Version) {
	if flags&gpsprot.NMEAMsgAny == 0 {
		m.protoDisable |= bin.CfgPrtProtoNMEA
		return
	}
	m.protoEnable |= bin.CfgPrtProtoNMEA
	m.rate[bin.NmeaRmcID] = nmeaRate(flags & gpsprot.NMEAMsgRMC)
	m.rate[bin.NmeaGgaID] = nmeaRate(flags & gpsprot.NMEAMsgGGA)
	m.rate[bin.NmeaGsaID] = nmeaRate(flags & gpsprot.NMEAMsgGSA)
	m.rate[bin.NmeaGsvID] = nmeaRate(flags & gpsprot.NMEAMsgGSV)
	m.rate[bin.NmeaZdaID] = nmeaRate(flags & gpsprot.NMEAMsgZDA)
}

func nmeaRate(flags gpsprot.NMEAMsgFlags) MsgRate {
	if flags != 0 {
		return 1
	}
	return 0
}

func (m *msgChanges) raw(flags gpsprot.RawMsgFlags, ver *Version) error {
	rawLevel := ver.rawLevel()
	if rawLevel == 0 {
		if flags != gpsprot.RawMsgNone {
			return errors.New("raw messages not supported by this model")
		}
		return nil
	}
	var obsRate, navRate MsgRate
	obsMsgID := bin.RxmRawxID
	navMsgID := bin.RxmSfrbxID
	if rawLevel == 1 {
		obsMsgID = bin.RxmRawID
		navMsgID = bin.RxmSfrbID
	}
	if flags&gpsprot.RawMsgObs != 0 {
		obsRate = 1
	}
	if flags&gpsprot.RawMsgNavData != 0 {
		navRate = 1
	}
	m.rate[obsMsgID] = obsRate
	m.rate[navMsgID] = navRate
	return nil
}

func (mc *msgChanges) rtcm(flags gpsprot.RTCMMsgFlags, ver *Version, enabledGNSS gpsprot.GNSSSet) error {
	disabledMsgIDs := []bin.MsgID{}
	anyEnabled := false
	supGNSS, supMSM := ver.rtcmSupport()
	if supGNSS == 0 {
		if flags != gpsprot.RTCMMsgNone {
			return errors.New("RTCM message output not supported by this model")
		}
		return nil
	}
	gloEnabled := false
	msmFlags := []gpsprot.RTCMMsgFlags{gpsprot.RTCMMsgMSM4, gpsprot.RTCMMsgMSM7}
	for _, g := range supGNSS.Items() {
		for _, m := range msmFlags {
			if supMSM&m == 0 {
				continue
			}
			enable := false
			if enabledGNSS.Contains(g) && flags&m != 0 {
				enable = true
			}
			msm := 4
			if m == gpsprot.RTCMMsgMSM7 {
				msm = 7
			}
			msgType := rtcm.MSMMsgType(g, msm)
			if msgType == 0 {
				continue
			}
			msgID, ok := bin.RTCMMsgID(int(msgType))
			if !ok {
				continue
			}
			if enable {
				mc.rate[msgID] = 1
				anyEnabled = true
				if g == gpsprot.GLO {
					gloEnabled = true
				}
			} else {
				disabledMsgIDs = append(disabledMsgIDs, msgID)
			}
		}
	}
	if flags&(gpsprot.RTCMMsgMSM4|gpsprot.RTCMMsgMSM7) != 0 && !anyEnabled {
		return errors.New("specified MSM messages not supported with specified GNSS")
	}
	if flags&gpsprot.RTCMMsgARP != 0 {
		mc.rate[bin.Rtcm1005ID] = 1
		anyEnabled = true
	}
	if !anyEnabled {
		mc.protoDisable |= bin.CfgPrtProtoRTCM3
		return nil
	}
	mc.protoEnable |= bin.CfgPrtProtoRTCM3
	if gloEnabled {
		msgID, ok := bin.RTCMMsgID(int(rtcm.GLONASSBiasMsgType))
		if ok {
			mc.rate[msgID] = 1
		}
	}
	for _, mid := range disabledMsgIDs {
		mc.rate[mid] = 0
	}
	return nil
}

var msgIDKey = map[bin.MsgID]ucv.KeyM{
	bin.NavTimeGPSID: ucv.KUbxNavTimegps,
	bin.NavPVTID:     ucv.KUbxNavPvt,
	bin.NavSatID:     ucv.KUbxNavSat,
	//bin.NavSigID: ucv.KUbxNavSig,
	bin.RxmRawxID:  ucv.KUbxRxmRawx,
	bin.RxmSfrbxID: ucv.KUbxRxmSfrbx,
	bin.TimTPID:    ucv.KUbxTimTp,
	// NMEA messages
	bin.NmeaGgaID: ucv.KNmeaIdGga,
	bin.NmeaGllID: ucv.KNmeaIdGll,
	bin.NmeaGsaID: ucv.KNmeaIdGsa,
	bin.NmeaGsvID: ucv.KNmeaIdGsv,
	bin.NmeaRmcID: ucv.KNmeaIdRmc,
	bin.NmeaVtgID: ucv.KNmeaIdVtg,
	bin.NmeaZdaID: ucv.KNmeaIdZda,
	bin.NmeaGnsID: ucv.KNmeaIdGns,
	// RTCM messages
	bin.Rtcm1005ID: ucv.KRtcm3xType1005,
	bin.Rtcm1074ID: ucv.KRtcm3xType1074,
	bin.Rtcm1077ID: ucv.KRtcm3xType1077,
	bin.Rtcm1084ID: ucv.KRtcm3xType1084,
	bin.Rtcm1087ID: ucv.KRtcm3xType1087,
	bin.Rtcm1094ID: ucv.KRtcm3xType1094,
	bin.Rtcm1097ID: ucv.KRtcm3xType1097,
	bin.Rtcm1124ID: ucv.KRtcm3xType1124,
	bin.Rtcm1127ID: ucv.KRtcm3xType1127,
	bin.Rtcm1230ID: ucv.KRtcm3xType1230,
}
