package as

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
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

// rtcmMSM4IDs and rtcmMSM7IDs list the per-constellation MSM targets,
// index-aligned. Enabling covers every constellation the ids exist
// for; the receiver emits MSM only for constellations it tracks.
var rtcmMSM4IDs = []asbin.MsgID{asbin.RtcmMsm4GpsID, asbin.RtcmMsm4GloID,
	asbin.RtcmMsm4GalID, asbin.RtcmMsm4SbasID, asbin.RtcmMsm4QzssID, asbin.RtcmMsm4BdsID}
var rtcmMSM7IDs = []asbin.MsgID{asbin.RtcmMsm7GpsID, asbin.RtcmMsm7GloID,
	asbin.RtcmMsm7GalID, asbin.RtcmMsm7SbasID, asbin.RtcmMsm7QzssID, asbin.RtcmMsm7BdsID}

// rtcmEphIDs lists the broadcast-ephemeris targets. There is no flag
// to enable them yet (#282), but a request without RTCMMsgOther turns
// them off: the TAU1302 ships with them enabled and emitting.
var rtcmEphIDs = []asbin.MsgID{asbin.RtcmEphGpsID, asbin.RtcmEphGloID,
	asbin.RtcmEphBdsID, asbin.RtcmEphQzssID, asbin.RtcmEphGalID}

// rtcmPropIDs lists the proprietary 0xF8 targets outside the modeled
// group (4065 moving-base reference PVT and its undocumented
// neighbours). A request without RTCMMsgOther turns them off, so the
// request is complete over the receiver's RTCM output.
var rtcmPropIDs = []asbin.MsgID{asbin.RtcmProp4065ID, asbin.RtcmProp4066ID,
	asbin.RtcmProp4068ID, asbin.RtcmProp4069ID}

// generateRTCMReqs configures RTCM output via CFG-MSG 0xF8 targets. A
// wire-format request is complete over the receiver's RTCM output:
// besides the modeled group - MSM4, MSM7, ARP - the broadcast-ephemeris
// and proprietary targets are turned off, unless RTCMMsgOther restricts
// the request to the modeled group and leaves them as found. All requests are
// NAK-tolerant: a receiver without RTCM output (the TAU1201) NAKs
// every 0xF8 target, and the absence of any achieved RTCM output is
// the statement - not an error. ARP (1005) is only emitted while a
// fixed position is set; enabling it without one is accepted and
// silent.
func (c *Configurator) generateRTCMReqs(flags gpsprot.RTCMMsgFlags) {
	msm4 := flags&gpsprot.RTCMMsgMSM4 != 0
	msm7 := flags&gpsprot.RTCMMsgMSM7 != 0
	for i := range rtcmMSM4IDs {
		c.addMsgRate(rtcmMSM4IDs[i], msm4)
		c.addMsgRate(rtcmMSM7IDs[i], msm7)
	}
	c.addMsgRate(asbin.RtcmArpID, flags&gpsprot.RTCMMsgARP != 0)
	if flags&gpsprot.RTCMMsgOther == 0 {
		for _, mid := range rtcmEphIDs {
			c.addMsgRate(mid, false)
		}
		for _, mid := range rtcmPropIDs {
			c.addMsgRate(mid, false)
		}
	}
}

// generateRawReqs configures raw data output. Allystar has a single
// switch, RXM-DUMPRAW, which turns on once-per-second RXM-RAW frames
// in an undocumented format; both kinds of raw information map to it,
// and a request naming neither turns it off (a raw request is
// complete). NAK-tolerant because the TAU951M NAKs the disable while
// still applying it.
func (c *Configurator) generateRawReqs(flags gpsprot.RawMsgFlags) {
	var enable uint8
	if flags&(gpsprot.RawMsgObs|gpsprot.RawMsgNavData) != 0 {
		enable = 1
	}
	c.addReqNakOK(&asbin.RxmDumpRaw{Enable: enable})
}

// generatePVTReqs configures the native messages that deliver the
// requested PVT information via CFG-MSG. The mapping: NAV-POSLLH or
// NAV-POSECEF carries position, NAV-VELNED or NAV-VELECEF velocity,
// NAV-TIMEUTC UTC time, NAV-TIME GNSS time and the current leap
// second, NAV-DOP plus NAV-AUTO solution quality, NAV-SVIN survey
// progress. Time-of-pulse and end-of-epoch information have no
// Allystar carrier (audited: no TIM class, no EOE equivalent), so
// requesting them enables nothing and they show as absence; with the
// after option the solution-time messages stand in for a pulse-time
// message. Without the off flag the request is incremental: unneeded
// messages are left alone.
func (c *Configurator) generatePVTReqs(flags gpsprot.PVTMsgFlags) {
	ecef := flags&gpsprot.PVTMsgECEF != 0
	wantTime := flags&gpsprot.PVTMsgTime != 0 ||
		(flags&gpsprot.PVTMsgTimePulse != 0 && flags&gpsprot.PVTMsgTimePulseAfter != 0)
	wantPos := flags&gpsprot.PVTMsgPos != 0
	wantVel := flags&gpsprot.PVTMsgVel != 0
	wantQual := flags&gpsprot.PVTMsgQuality != 0
	wantNavTime := flags&gpsprot.PVTMsgLeapSecond != 0 ||
		(wantTime && flags&gpsprot.PVTMsgTAI != 0)
	off := flags&gpsprot.PVTMsgOff != 0
	set := func(mid asbin.MsgID, want bool) {
		if want || off {
			c.addMsgRate(mid, want)
		}
	}
	set(asbin.NavPosLlhID, wantPos && !ecef)
	set(asbin.NavPosEcefID, wantPos && ecef)
	set(asbin.NavVelNedID, wantVel && !ecef)
	set(asbin.NavVelEcefID, wantVel && ecef)
	set(asbin.NavTimeUtcID, wantTime && flags&gpsprot.PVTMsgTAI == 0)
	set(asbin.NavTimeID, wantNavTime)
	set(asbin.NavDopID, wantQual)
	set(asbin.NavAutoID, wantQual)
	set(asbin.NavSvinID, flags&gpsprot.PVTMsgSurvey != 0)
}

// generateSatsReqs configures the satellite information messages. A
// satellite request is complete: group messages not carrying requested
// information are turned off. NAV-SVINFO is per-satellite only
// (hardware-verified: dual-band satellites appear once), so per-signal
// information is not deliverable and a signal-only request enables
// nothing - the absence in the output is the statement.
func (c *Configurator) generateSatsReqs(flags gpsprot.SatsMsgFlags) {
	c.addMsgRate(asbin.NavSvInfoID, flags&gpsprot.SatsMsgSat != 0)
}

// addMsgRate requests a message be enabled (at 1Hz output) or disabled
// via CFG-MSG. A NAK is acceptable: the message may not exist on this
// firmware (the TAU951M lacks NAV-SVSTATE, for example), and
// undeliverable information shows as absence.
func (c *Configurator) addMsgRate(mid asbin.MsgID, on bool) {
	c.msgRate(mid, on, true)
}

// generateNMEAReqs sets each standard NMEA sentence on or off (a
// wire-format request is complete). These are not nakOK: every tested
// unit accepts all seven ids, so a NAK is a genuine refusal. GSV goes
// first because it carries the most traffic, so turning it off frees the
// line soonest; disables are sent here, while an enable may be deferred
// to the resolve phase, so GSV-first ordering of the disables holds.
func (c *Configurator) generateNMEAReqs(flags gpsprot.NMEAMsgFlags) {
	msgs := []struct {
		flag gpsprot.NMEAMsgFlags
		mid  asbin.MsgID
	}{
		{gpsprot.NMEAMsgGSV, asbin.NmeaGsvID},
		{gpsprot.NMEAMsgRMC, asbin.NmeaRmcID},
		{gpsprot.NMEAMsgGGA, asbin.NmeaGgaID},
		{gpsprot.NMEAMsgGSA, asbin.NmeaGsaID},
		{gpsprot.NMEAMsgZDA, asbin.NmeaZdaID},
		{gpsprot.NMEAMsgVTG, asbin.NmeaVtgID},
		{gpsprot.NMEAMsgGLL, asbin.NmeaGllID},
	}
	for _, m := range msgs {
		c.msgRate(m.mid, flags&m.flag != 0, false)
	}
	if flags&gpsprot.NMEAMsgOther == 0 {
		for _, mid := range nmeaOtherIDs {
			c.addMsgRate(mid, false)
		}
	}
}

// nmeaOtherIDs lists the rate-controllable NMEA sentences outside the
// modeled vocabulary. A request without NMEAMsgOther turns them off, so
// the request is complete over the receiver's NMEA output; nakOK because
// not every unit carries them (the TAU1302 NAKs DTM, JAM exists only on
// the TAU951M). TXT (0x20) is deliberately excluded: it is spontaneous
// antenna-status output, out of scope in both directions.
var nmeaOtherIDs = []asbin.MsgID{asbin.NmeaGstID, asbin.NmeaGrsID,
	asbin.NmeaDtmID, asbin.NmeaJamID}

// msgRate applies one CFG-MSG target's output request. In planning mode
// (the query-phase dry run) it only records the enable/disable intent.
// A disable is rate-independent and always sent. An enable is a three-way
// decision keyed on the native rate: the Allystar Rate field is a divisor
// of the native cycle, so "1Hz output" means rate = nativeHz, and until
// the estimator knows nativeHz an enable cannot be sent correctly.
//   - already 1Hz by observation: skip - the message is doing the
//     requested thing whatever its stored divisor (no send, no save).
//   - native rate resolved: send at rate = nativeHz. Correct first try.
//   - neither: defer to the resolve phase.
func (c *Configurator) msgRate(mid asbin.MsgID, on bool, nakOK bool) {
	if c.planning {
		c.plan[mid] = on
		return
	}
	if !on {
		c.sendMsgRate(mid, 0, nakOK)
		return
	}
	if iv, ok := c.est.observedInterval(mid); ok && isOneHz(iv) {
		return
	}
	if hz := c.est.nativeHz; hz != 0 {
		c.sendMsgRate(mid, uint8(hz), nakOK)
		return
	}
	c.deferred = append(c.deferred, deferredEnable{mid: mid, nakOK: nakOK})
}

// sendMsgRate appends a CFG-MSG set of mid to the given rate.
func (c *Configurator) sendMsgRate(mid asbin.MsgID, rate uint8, nakOK bool) {
	cls, id := mid.Unpack()
	m := &asbin.CfgMsg{CfgMsgFixed: asbin.CfgMsgFixed{MsgClass: cls, MsgID: id, Rate: rate}}
	if nakOK {
		c.addReqNakOK(m)
	} else {
		c.addReq(m)
	}
}

// plannedEnabled returns the CFG-MSG targets the message phase will
// enable, by running the message generators in planning mode (no
// requests emitted). The query-phase rate poll prefers one of these, as
// an id the request is about to disable may never produce the second
// arrival that turns a polled rate into a native rate.
func (c *Configurator) plannedEnabled() map[asbin.MsgID]bool {
	c.planning = true
	c.plan = make(map[asbin.MsgID]bool)
	c.generateMsgReqs()
	c.planning = false
	keep := make(map[asbin.MsgID]bool)
	for mid, on := range c.plan {
		if on {
			keep[mid] = true
		}
	}
	c.plan = nil
	return keep
}

// generateRatePoll adds one CFG-MSG poll of a flowing id when traffic is
// flowing and some message will be enabled. Its polled rate, combined
// with the observed interval of that id, gives the native rate exactly
// (this is what makes the restart cases free). Its mid 0x06 0x01 is
// distinct from the property polls, so it pipelines with them - zero
// added round trips. Requires the 4-byte CFG-MSG readback parse.
func (c *Configurator) generateRatePoll() {
	if !c.est.sawPeriodic() {
		return
	}
	keep := c.plannedEnabled()
	if len(keep) == 0 {
		return
	}
	mid, ok := c.est.flowingPollID(keep)
	if !ok {
		return
	}
	c.add(&asReq{mid: asbin.CfgMsgID, packet: asbin.PollCfgMsg(mid),
		onData: func(asbin.Msg) {}, optional: true})
}

// generateResolveReqs completes every deferred enable, bounded. If the
// rate resolved in the meantime, the enables go out at once at the right
// divisor. If traffic is flowing but the rate is not yet computed, a
// watch waits for it (at most ~1s) with no wrong-rate transient. If the
// receiver is silent, the enables go out at rate=1 to create the
// evidence, and the watch corrects them if the unit turns out faster or
// concludes 1Hz on the deadline.
func (c *Configurator) generateResolveReqs() {
	if len(c.deferred) == 0 {
		return
	}
	if c.est.nativeHz != 0 {
		c.sendDeferred(uint8(c.est.nativeHz))
		return
	}
	if c.est.sawPeriodic() {
		c.watch = c.addWatchReq(resolveFlowingDelay)
		return
	}
	c.sendDeferredAtRate1()
	c.watchSent = true
	c.watch = c.addWatchReq(resolveSilentDelay)
}

// addWatchReq appends the resolve-phase watch: a request born awaiting a
// response that it never sent, so the director never transmits it. It
// resolves when the estimator computes the rate (onObserve) or when its
// deadline passes (watchTimeout). The deadline is anchored at creation
// time (the estimator's current notion of now) and advances only on
// waited-on arrivals (see onObserve and GetDeadline).
func (c *Configurator) addWatchReq(dur time.Duration) *asReq {
	req := &asReq{state: reqAwaitingAck, watch: true, watchDur: dur,
		watchAnchor: c.est.lastRead, cfg: c}
	c.reqs = append(c.reqs, req)
	return req
}

// onObserve is called after every observed packet is fed to the shared
// estimator. It completes the watch as soon as the rate resolves, and
// otherwise advances the deadline only when a track the watch waits on
// makes progress - the first arrival of an id whose further arrivals can
// still resolve the rate (a polled id, or one we self-set at rate=1).
// Unrelated traffic must not advance it, so the cap stays bounded; and
// only a first arrival advances, so a flood of non-resolving arrivals of a
// waited-on id cannot postpone the cap indefinitely.
func (c *Configurator) onObserve() {
	w := c.watch
	if w == nil || w.state != reqAwaitingAck {
		return
	}
	if !w.watchStarted {
		// Anchor the deadline on the first real packet the watch sees (the
		// enable ACK, or the first flowing arrival). est.lastRead may be
		// the zero time at creation when no traffic preceded the watch, so
		// the anchor cannot be snapshotted then.
		w.watchAnchor = c.est.lastRead
		w.watchStarted = true
	}
	if c.est.nativeHz != 0 {
		c.resolveWatch()
		return
	}
	if !c.est.lastWasArrival {
		return
	}
	mid := c.est.lastArrivalMid
	tr := c.est.tracks[mid]
	if tr == nil || !(tr.hasPoll || tr.selfSet) || w.extended[mid] {
		return
	}
	if w.extended == nil {
		w.extended = make(map[asbin.MsgID]bool)
	}
	w.extended[mid] = true
	w.watchAnchor = c.est.lastRead
}

// resolveWatch completes the watch once the native rate is known: it
// sends the deferred enables at the native divisor, or - if they were
// already sent at rate=1 and the rate is 1 - leaves them as they are.
func (c *Configurator) resolveWatch() {
	if c.watchSent && c.est.nativeHz == 1 {
		c.deferred = nil
	} else {
		c.sendDeferred(uint8(c.est.nativeHz))
	}
	c.watch.state = reqSucceeded
	c.watch = nil
}

// watchTimeout handles the watch deadline. A flowing wait that elapsed
// without a computed rate falls back to the silent path: enable at
// rate=1 to create evidence and extend the deadline. A silent wait that
// elapsed means no fast traffic appeared, so the rate is 1Hz (anchored
// negative, or the cap with a log line when nothing arrived at all);
// rate=1 was already sent and is correct.
func (c *Configurator) watchTimeout(req *asReq) {
	if c.est.nativeHz != 0 {
		c.resolveWatch()
		return
	}
	if !c.watchSent {
		c.sendDeferredAtRate1()
		c.watchSent = true
		req.watchDur = resolveSilentDelay
		req.watchAnchor = c.est.lastRead
		req.extended = nil
		return
	}
	c.est.concludeSilent()
	c.deferred = nil
	req.state = reqSucceeded
	c.watch = nil
}

// sendDeferred issues each deferred enable at rate hz, skipping any that
// is already observed at 1Hz (naturally, or because hz turned out to be
// 1). It clears the deferred list.
func (c *Configurator) sendDeferred(hz uint8) {
	for _, d := range c.deferred {
		if iv, ok := c.est.observedInterval(d.mid); ok && isOneHz(iv) {
			continue
		}
		c.sendMsgRate(d.mid, hz, d.nakOK)
	}
	c.deferred = nil
}

// sendDeferredAtRate1 enables each deferred message at rate=1 to create
// the evidence a silent receiver needs, and marks each self-set so a
// clean interval on it is the native period directly. The deferred list
// is kept so the enables can be corrected if the unit proves faster.
func (c *Configurator) sendDeferredAtRate1() {
	for _, d := range c.deferred {
		c.sendMsgRate(d.mid, 1, d.nakOK)
		c.est.markSelfSet(d.mid)
	}
}
