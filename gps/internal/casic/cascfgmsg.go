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
}

// generateRateReqs forces the named 1 Hz positioning interval whenever
// the message phase accepted an enable. A CFG-MSG rate is a per-fix
// divisor, not a frequency, so enabled every-fix output only runs at
// 1 Hz when the positioning rate is also 1 Hz. The
// interval (CFG-RATE) persists and may be left at a non-default value,
// so this matches the semantics guarantee of 1 Hz output independent of
// the positioning rate, as the u-blox backend does. msgEnabled is set
// only by an accepted enable (see addMsg), so an invocation that only
// disables output, or whose every enable was refused, sends nothing and
// leaves the interval alone. This is its own phase, after the message
// phase is final, because whether an enable was ACKed is known only
// then. On V6 FixRateHz accompanies the interval; on V5 the trailing
// bytes are reserved and must stay zero. CFG-RATE is documented on both
// families, so a NAK is a genuine refusal (addReq).
func (c *Configurator) generateRateReqs() {
	if !c.msgEnabled {
		return
	}
	rate := &casbin.CfgRate{
		FixIntervalMs: casbin.CfgRateFixInterval1Hz,
		FixRateHz:     casbin.CfgRateFixRateV5Reserved,
	}
	if c.family == familyV6 {
		rate.FixRateHz = casbin.CfgRateFixRate1Hz
	}
	// The probe's CFG-RATE readback makes the usual case a no-op.
	// Res is excluded from the comparison: it is reserved, V6 units
	// report 1 there, and the value written does not matter.
	if c.rate.FixIntervalMs == rate.FixIntervalMs && c.rate.FixRateHz == rate.FixRateHz {
		c.touchNoOp(casbin.CfgCfgSectionNav)
		return
	}
	c.addSetReq(rate, func() { c.rate = rate })
}

// generateRawReqs configures raw data output where the capability is
// declared: RXM2-MEASX carries raw observations, RXM2-SFRBX raw
// navigation subframes, and a raw request is complete (the group
// message not named is turned off). Receivers without the declared
// capability - navigation-class V6 units and all V5 units, which
// acknowledge the enables but never emit the messages - get nothing
// attempted; the flag layer's unsupported-option warning stands.
func (c *Configurator) generateRawReqs(flags gpsprot.RawMsgFlags) {
	if c.ConfigSupport()&gpsprot.ConfigSupportRaw == 0 {
		return
	}
	c.addMsgRate(casbin.Rxm2MeasxID, flags&gpsprot.RawMsgObs != 0)
	c.addMsgRate(casbin.Rxm2SfrbxID, flags&gpsprot.RawMsgNavData != 0)
}

// addMsgRate sets a message's output rate to every fix or off via CFG-MSG. A NAK
// is acceptable: the message may not exist on this firmware, and
// undeliverable information shows as absence.
func (c *Configurator) addMsgRate(mid casbin.MsgID, on bool) {
	rate := casbin.CfgMsgRateOff
	if on {
		rate = casbin.CfgMsgRateEveryFix
	}
	c.addReqNakOK(&casbin.CfgMsg{Target: mid, Rate: rate})
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
		c.addMsgRate(casbin.Nav2SigID, flags&(gpsprot.SatsMsgSat|gpsprot.SatsMsgSignal) != 0)
		return
	}
	on := flags&gpsprot.SatsMsgSat != 0
	for _, mid := range []casbin.MsgID{casbin.NavGPSInfoID, casbin.NavBDSInfoID, casbin.NavGLNInfoID} {
		c.addMsgRate(mid, on)
	}
}

// generatePVTReqs configures the native messages that deliver the
// requested PVT information via CFG-MSG. The mapping (see plan):
// NAV-SOL/NAV2-SOL carries ECEF pos+vel, TAI time, and fix quality;
// NAV-PV/NAV2-PVH carries geodetic pos+vel; NAV-TIMEUTC/NAV2-TIMEUTC
// carries UTC time; NAV-DOP/NAV2-DOP the full DOP set; TIM-TP (V5)
// carries the time of the next pulse and TIM2-TPX (V6) the time of
// the pulse that occurred; TIM2-LS (V6) and MSG-GPSUTC (V5) carry
// leap second events; TIM2-TIMEPOS (V6 only) carries survey progress.
// Epoch markers have no CASIC message. Without the off flag the
// request is incremental: unneeded messages are left alone.
func (c *Configurator) generatePVTReqs(flags gpsprot.PVTMsgFlags) {
	pv, sol, timeUTC, dop := casbin.NavPvID, casbin.NavSolID, casbin.NavTimeUTCID, casbin.NavDopID
	if c.family == familyV6 {
		pv, sol, timeUTC, dop = casbin.Nav2PvhID, casbin.Nav2SolID, casbin.Nav2TimeUTCID, casbin.Nav2DopID
	}
	tp := flags&gpsprot.PVTMsgTimePulse != 0
	// tp+after also wants a time message, for two reasons: on V5 the
	// pulse message TIM-TP is pre-pulse (time of the next pulse), so a
	// time message following the pulse is the only realization of
	// after; on V6 TIM2-TPX is post-pulse and would suffice by itself,
	// but its ACK does not guarantee emission (the F8N and AT372-6P
	// acknowledge it and never emit), so time must not depend on it.
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
	set := func(mid casbin.MsgID, want bool) {
		if want || off {
			c.addMsgRate(mid, want)
		}
	}
	set(sol, wantSol)
	set(pv, wantPv)
	set(timeUTC, wantTimeUTC)
	set(dop, wantDop)
	if c.family == familyV6 {
		set(casbin.Tim2LsID, flags&gpsprot.PVTMsgLeapSecond != 0)
		// Gated on the declared capability (navigation and positioning
		// classes never emit TIM2-TIMEPOS) and on the tmode phase
		// having put the receiver into survey-in mode, matching ubx:
		// the survey flag declares interest in progress (satpulsed
		// sets it unconditionally), and there is no progress to
		// deliver outside a survey.
		if c.ConfigSupport()&gpsprot.ConfigSupportSurveyMsg != 0 {
			set(casbin.Tim2TimePosID, flags&gpsprot.PVTMsgSurvey != 0 && c.survey)
		}
	} else {
		set(casbin.MsgGPSUTCID, flags&gpsprot.PVTMsgLeapSecond != 0)
	}
	c.generateTimTPReqs(tp, off)
}

// generateTimTPReqs enables or disables the family's time-of-pulse
// message: TIM-TP (0x02 0x00) on V5, TIM2-TPX (0x12 0x00) on V6. The
// V6 protocol does not document TIM-TP and every tested V6 unit NAKs
// it, so it is not attempted there. The enable is nakOK: a refusal
// means pulse-time information is not deliverable, which per the
// semantics is shown by its absence, not an error. Acceptance does
// not guarantee delivery either - the F8N and AT372-6P acknowledge
// the TIM2-TPX enable but never emit the message - which is why
// nothing else depends on this enable (see generatePVTReqs).
func (c *Configurator) generateTimTPReqs(tp, off bool) {
	mid := casbin.TimTPID
	if c.family == familyV6 {
		mid = casbin.Tim2TpxID
	}
	if tp || off {
		c.addMsgRate(mid, tp)
	}
}

// generateNMEAReqs sets the rate of each standard NMEA sentence via
// CFG-MSG: named sentences on, the rest off (a wire-format request is
// complete). Unlike binary message enables these are not nakOK: every
// CASIC firmware emits the standard sentences, so a NAK is a genuine
// refusal. GSV goes first because it carries the most traffic, so
// turning it off frees a saturated line soonest. Completeness extends
// past the modeled seven: without NMEAMsgOther the extra output
// sentences the firmware manuals document are disabled too, via
// addMsgRate (nakOK) because a sentence id may be absent in a given
// firmware version and that refusal must not fail the request.
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
		rate := casbin.CfgMsgRateOff
		if flags&m.flag != 0 {
			rate = casbin.CfgMsgRateEveryFix
		}
		c.addReq(&casbin.CfgMsg{Target: m.mid, Rate: rate})
	}
	if flags&gpsprot.NMEAMsgOther != 0 {
		return
	}
	extra := []casbin.MsgID{casbin.NmeaTxtAntID, casbin.NmeaDhvV6ID,
		casbin.NmeaTxtLpsID, casbin.NmeaTxtInsID, casbin.NmeaUtcV6ID,
		casbin.NmeaGstV6ID, casbin.NmeaTxtRfeID}
	if c.family == familyV5 {
		extra = []casbin.MsgID{casbin.NmeaGstID, casbin.NmeaAntID,
			casbin.NmeaLpsID, casbin.NmeaDhvID, casbin.NmeaUtcID}
	}
	for _, mid := range extra {
		c.addMsgRate(mid, false)
	}
}
