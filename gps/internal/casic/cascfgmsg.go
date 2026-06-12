package casic

import (
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/casbin"
)

// generateMsgReqs generates the message output requests for the target.
func (c *Configurator) generateMsgReqs() {
	opts := &c.target.Opts
	if opts.PVTMsg.IsSet() {
		c.generatePVTReqs(opts.PVTMsg)
	}
	if opts.NMEAMsg.IsSet() {
		c.generateNMEAReqs(opts.NMEAMsg.Get())
	}
	if opts.SatsMsg.IsSet() {
		c.generateSatsReqs(opts.SatsMsg.Get())
	}
	if opts.RawMsg.IsSet() {
		c.generateRawReqs(opts.RawMsg.Get())
	}
	if opts.RTCMMsg.IsSet() {
		c.generateRTCMReqs(opts.RTCMMsg.Get())
	}
}

// generateRTCMReqs configures RTCM output on V6: CFG-RTCM selects the
// message types and MSM version, and the port's protocol mask gates
// RTCM output as a whole. There is no GLONASS MSM enable in CFG-RTCM,
// so GLONASS corrections are not available. Whether a given unit
// actually emits RTCM is its own affair: the enables are acknowledged
// and emission is checked by observation, like raw output.
func (c *Configurator) generateRTCMReqs(flags gpsprot.RTCMMsgFlags) {
	if c.family != familyV6 {
		return
	}
	var en uint32
	ver := uint8(4)
	if flags&(gpsprot.RTCMMsgMSM4|gpsprot.RTCMMsgMSM7) != 0 {
		en |= casbin.RtcmEnGPSMSM | casbin.RtcmEnGALMSM | casbin.RtcmEnQZSSMSM | casbin.RtcmEnBDSMSM
		if flags&gpsprot.RTCMMsgMSM7 != 0 {
			ver = 7
		}
	}
	if flags&gpsprot.RTCMMsgARP != 0 {
		en |= casbin.RtcmEn1005
	}
	c.addReqNakOK(&casbin.CfgRtcm{MsgEnable: en, MsmVer: ver}, nil)
	if len(c.ports) == 0 {
		return
	}
	base := c.ports[0]
	for _, p := range c.ports {
		if p.PortID == 0 {
			base = p
		}
	}
	mask := base.ProtoMask &^ uint8(casbin.PrtProtoRTCMOut)
	if en != 0 {
		mask = base.ProtoMask | casbin.PrtProtoRTCMOut
	}
	c.addReqNakOK(&casbin.CfgPrt{PortID: casbin.PortCurrent, ProtoMask: mask,
		Mode: base.Mode, BaudRate: base.BaudRate}, nil)
}

// generateRawReqs configures raw data output on V6: RXM2-MEASX carries
// raw observations, RXM2-SFRBX raw navigation subframes. A raw request
// is complete: the group message not named is turned off. The dual-band
// F8N acknowledges these enables without ever emitting the messages - a
// firmware limitation that is deliberately not worked around. V5
// firmware likewise acknowledges its RXM messages but never emits them
// (per the casictool notes), so raw output does not exist there and no
// requests are generated.
func (c *Configurator) generateRawReqs(flags gpsprot.RawMsgFlags) {
	if c.family != familyV6 {
		return
	}
	for _, m := range []struct {
		mid  casbin.MsgID
		flag gpsprot.RawMsgFlags
	}{{casbin.Rxm2MeasxID, gpsprot.RawMsgObs}, {casbin.Rxm2SfrbxID, gpsprot.RawMsgNavData}} {
		var rate uint16
		if flags&m.flag != 0 {
			rate = 1
		}
		c.addReqNakOK(&casbin.CfgMsg{Target: m.mid, Rate: rate}, nil)
	}
}

// generateSatsReqs configures the messages carrying satellite
// information. A satellite request is complete: group messages not
// carrying requested information are turned off. On V6, NAV2-SIG
// carries both per-satellite and per-signal information; on V5 the
// per-constellation NAV-*INFO messages carry satellite information
// only, so a signal-only request enables nothing there (per-signal
// information is not deliverable, which shows as absence).
func (c *Configurator) generateSatsReqs(flags gpsprot.SatsMsgFlags) {
	if c.family == familyV6 {
		var rate uint16
		if flags&(gpsprot.SatsMsgSat|gpsprot.SatsMsgSignal) != 0 {
			rate = 1
		}
		c.addReqNakOK(&casbin.CfgMsg{Target: casbin.Nav2SigID, Rate: rate}, nil)
		return
	}
	var rate uint16
	if flags&gpsprot.SatsMsgSat != 0 {
		rate = 1
	}
	for _, mid := range []casbin.MsgID{casbin.NavGPSInfoID, casbin.NavBDSInfoID, casbin.NavGLNInfoID} {
		c.addReqNakOK(&casbin.CfgMsg{Target: mid, Rate: rate}, nil)
	}
}

// generatePVTReqs configures the native messages that deliver the
// requested PVT information via CFG-MSG. The mapping (see plan):
// NAV-SOL/NAV2-SOL carries ECEF pos+vel, TAI time, and fix quality;
// NAV-PV/NAV2-PVH carries geodetic pos+vel; NAV-TIMEUTC/NAV2-TIMEUTC
// carries UTC time; NAV-DOP/NAV2-DOP the full DOP set; TIM-TP the time
// of the next pulse; TIM2-LS (V6) and MSG-GPSUTC (V5) carry leap
// second events; TIM2-TIMEPOS (V6 only) carries survey progress.
// Epoch markers have no CASIC message. Without the off flag the
// request is incremental: unneeded messages are left alone.
func (c *Configurator) generatePVTReqs(flags gpsprot.PVTMsgFlags) {
	pv, sol, timeUTC, dop := casbin.NavPvID, casbin.NavSolID, casbin.NavTimeUTCID, casbin.NavDopID
	if c.family == familyV6 {
		pv, sol, timeUTC, dop = casbin.Nav2PvhID, casbin.Nav2SolID, casbin.Nav2TimeUTCID, casbin.Nav2DopID
	}
	tp := flags&gpsprot.PVTMsgTimePulse != 0
	wantTime := flags&gpsprot.PVTMsgTime != 0 || (tp && flags&gpsprot.PVTMsgTimePulseAfter != 0)
	var wantSol, wantPv, wantTimeUTC, wantDop bool
	if wantTime {
		if flags&gpsprot.PVTMsgTAI != 0 {
			wantSol = true
		} else {
			wantTimeUTC = true
		}
	}
	if flags&(gpsprot.PVTMsgPos|gpsprot.PVTMsgVel) != 0 {
		if flags&gpsprot.PVTMsgECEF != 0 {
			wantSol = true
		} else {
			wantPv = true
		}
	}
	if flags&gpsprot.PVTMsgQuality != 0 {
		wantSol = true
		wantDop = true
	}
	off := flags&gpsprot.PVTMsgOff != 0
	for _, m := range []struct {
		mid  casbin.MsgID
		want bool
	}{{sol, wantSol}, {pv, wantPv}, {timeUTC, wantTimeUTC}, {dop, wantDop}} {
		if m.want {
			c.addReqNakOK(&casbin.CfgMsg{Target: m.mid, Rate: 1}, nil)
		} else if off {
			c.addReqNakOK(&casbin.CfgMsg{Target: m.mid, Rate: 0}, nil)
		}
	}
	leap, survey := casbin.MsgGPSUTCID, casbin.MsgID(0)
	if c.family == familyV6 {
		leap, survey = casbin.Tim2LsID, casbin.Tim2TimePosID
	}
	for _, m := range []struct {
		mid  casbin.MsgID
		flag gpsprot.PVTMsgFlags
	}{{leap, gpsprot.PVTMsgLeapSecond}, {survey, gpsprot.PVTMsgSurvey}} {
		if m.mid == 0 {
			continue
		}
		if flags&m.flag != 0 {
			c.addReqNakOK(&casbin.CfgMsg{Target: m.mid, Rate: 1}, nil)
		} else if off {
			c.addReqNakOK(&casbin.CfgMsg{Target: m.mid, Rate: 0}, nil)
		}
	}
	c.generateTimTPReqs(tp, off)
}

// generateTimTPReqs enables or disables the time-of-pulse message.
// Hardware divergence within the V6 family: the dual-band ATGM332D-F8N
// NAKs enabling TIM-TP (0x02 0x00) while the AT632 timing receiver
// accepts it; the F8N may also lack TIM2-TPX. So on V6 try TIM-TP
// first and fall back to TIM2-TPX (0x12 0x00) when NAKed; a NAK of the
// fallback means the receiver cannot deliver pulse-time information,
// which per the semantics is shown by its absence, not an error.
func (c *Configurator) generateTimTPReqs(tp, off bool) {
	switch {
	case tp && c.family == familyV6:
		c.addReqNakOK(&casbin.CfgMsg{Target: casbin.TimTPID, Rate: 1}, func() {
			c.addReqNakOK(&casbin.CfgMsg{Target: casbin.Tim2TpxID, Rate: 1}, nil)
		})
	case tp:
		c.addReqNakOK(&casbin.CfgMsg{Target: casbin.TimTPID, Rate: 1}, nil)
	case off:
		c.addReqNakOK(&casbin.CfgMsg{Target: casbin.TimTPID, Rate: 0}, nil)
		if c.family == familyV6 {
			c.addReqNakOK(&casbin.CfgMsg{Target: casbin.Tim2TpxID, Rate: 0}, nil)
		}
	}
}

// generateNMEAReqs sets the rate of each standard NMEA sentence via
// CFG-MSG: named sentences on, the rest off (a wire-format request is
// complete). GSV goes first because it carries the most traffic, so
// turning it off frees a saturated line soonest.
func (c *Configurator) generateNMEAReqs(flags gpsprot.NMEAMsgFlags) {
	zda := casbin.NmeaZdaID
	if c.family == familyV6 {
		zda = casbin.NmeaZdaV6ID
	}
	msgs := []struct {
		flag gpsprot.NMEAMsgFlags
		mid  casbin.MsgID
	}{
		{gpsprot.NMEAMsgGSV, casbin.NmeaGsvID},
		{gpsprot.NMEAMsgRMC, casbin.NmeaRmcID},
		{gpsprot.NMEAMsgGGA, casbin.NmeaGgaID},
		{gpsprot.NMEAMsgGSA, casbin.NmeaGsaID},
		{gpsprot.NMEAMsgZDA, zda},
		{gpsprot.NMEAMsgVTG, casbin.NmeaVtgID},
		{gpsprot.NMEAMsgGLL, casbin.NmeaGllID},
	}
	for _, m := range msgs {
		var rate uint16
		if flags&m.flag != 0 {
			rate = 1
		}
		c.addReq(&casbin.CfgMsg{Target: m.mid, Rate: rate})
	}
}
