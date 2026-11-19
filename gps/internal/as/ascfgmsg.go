package as

import (
	"fmt"
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
	c.startMsgReqs()
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
// the request to the modeled group and leaves them as found. A receiver
// without RTCM output (the TAU1201) NAKs every 0xF8 target, so none is
// sent to it: the absence of any RTCM output is the statement, as for a
// u-blox model without RTCM. All requests are NAK-tolerant. ARP (1005)
// is only emitted while a fixed position is set; enabling it without one
// is accepted and silent.
func (c *Configurator) generateRTCMReqs(flags gpsprot.RTCMMsgFlags) {
	if !c.supportsRTCM() {
		return
	}
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
// progress. Survey progress is scoped to the survey: the flag declares
// interest, and NAV-SVIN is turned on only when the request itself puts
// the receiver into survey mode, as in the other backends. Time-of-pulse and end-of-epoch information have no
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
	set(asbin.NavSvinID, flags&gpsprot.PVTMsgSurvey != 0 && c.surveying)
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

// addMsgRate requests a message be turned on (at one per second) or off
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
// line soonest.
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

// msgRate records one CFG-MSG target's output request. Nothing is sent
// here: a turn-off does not depend on the update rate, but it waits with
// the turn-ons so that it cannot silence a message the Period search is
// watching (see startMsgReqs).
func (c *Configurator) msgRate(mid asbin.MsgID, on bool, nakOK bool) {
	c.msgs = append(c.msgs, msgReq{mid: mid, on: on, nakOK: nakOK})
}

// sendMsgRate appends a CFG-MSG set of mid to the given Period.
func (c *Configurator) sendMsgRate(mid asbin.MsgID, period uint8, nakOK bool) *asReq {
	cls, id := mid.Unpack()
	m := &asbin.CfgMsg{CfgMsgFixed: asbin.CfgMsgFixed{MsgClass: cls, MsgID: id, Rate: period}}
	return c.addMsg(m, asReq{nakOK: nakOK})
}

// msgReq is one requested CFG-MSG target: on at one message per second,
// or off. nakOK carries the id's NAK tolerance (binary NAV and RTCM ids
// may not exist; NMEA ids do).
type msgReq struct {
	mid   asbin.MsgID
	on    bool
	nakOK bool
}

// startMsgReqs acts on the recorded message requests. With nothing to
// turn on, the turn-offs go out at once. Otherwise the Period that gives
// one message per second has to be found first, and everything waits on
// the search.
func (c *Configurator) startMsgReqs() {
	for _, m := range c.msgs {
		if m.on {
			c.search = &periodSearch{ignored: make(map[asbin.MsgID]bool)}
			c.search.watch = c.addWatchReq()
			c.now = c.est.last
			c.searchStep(c.now)
			return
		}
	}
	c.sendMsgOffs()
}

// sendMsgOffs sends the recorded turn-offs, in request order.
func (c *Configurator) sendMsgOffs() {
	for _, m := range c.msgs {
		if !m.on {
			c.sendMsgRate(m.mid, 0, m.nakOK)
		}
	}
}

// periodSearch is the search for the CFG-MSG Period that gives one
// message per second on this receiver (see asrate.go for why it has to
// be found by watching). It runs in stages:
//
//   - look: watch the messages already arriving. When one arrives once
//     per second, poll its Period; 1, 5 or 10 is the answer.
//   - trial: if nothing is arriving once per second, set one NAV message
//     (the probe) to a trial Period and watch it. Arriving once per
//     second, the trial is the answer; clearly not, the next is tried.
//
// With the answer, every requested message is set to it. Without one,
// nothing is turned on and the search fails; the failure suppresses the
// save like any failed request.
type periodSearch struct {
	watch     *asReq               // holds configuration open while the search runs
	ignored   map[asbin.MsgID]bool // types arriving once per second whose Period is not 1, 5 or 10
	pending   *asReq               // the poll or set the search is waiting on
	polling   bool                 // the pending request is a poll
	polled    asbin.MsgID          // the target of the latest poll
	period    uint8                // the Period the latest poll returned
	gotPeriod bool                 // the latest poll was answered for its target
	trialing  bool                 // the look stage is over
	start     time.Time            // start of the current trial
	probe     asbin.MsgID          // the message the trials set
	found     uint8                // the probe's Period before the first trial
	cur       uint8                // the probe's Period now
	wrote     bool                 // cur was set by the current trial
	trials    []uint8              // the Periods to try, in order
	deadline  time.Time            // when the search next needs to look at the clock
}

// stageLimit is how long one stage may run. Every other wait ends by
// itself; this covers messages that keep arriving but never settle. A
// steady receiver needs about 3s in a stage, and this is three times
// that: a judgement about how long to tolerate an unsettled receiver.
const stageLimit = 10 * time.Second

// searchTick is how often the search looks again while it has nothing
// more specific to wait for.
const searchTick = time.Second

// addWatchReq appends the search's watch: a request born awaiting a
// response that it never sent, so the director never transmits it and
// later phases wait for it. Its deadline is whenever the search next
// needs to look at the clock.
func (c *Configurator) addWatchReq() *asReq {
	req := &asReq{state: reqAwaitingAck, watch: true, cfg: c}
	c.reqs = append(c.reqs, req)
	return req
}

// searchStep advances the search as far as what has been seen by now
// allows. It runs after every message from the receiver and whenever the
// watch's deadline passes.
func (c *Configurator) searchStep(now time.Time) {
	s := c.search
	if s == nil || s.watch.state != reqAwaitingAck {
		return
	}
	if now.After(c.now) {
		c.now = now
	}
	for c.searchAdvance() {
	}
	if s.watch.state == reqAwaitingAck {
		s.deadline = c.searchDeadline()
	}
}

// searchAdvance takes one step of the search and reports whether it
// should be called again.
func (c *Configurator) searchAdvance() bool {
	s := c.search
	if s.pending != nil {
		switch s.pending.state {
		case reqSucceeded:
			s.pending = nil
			if s.polling && !s.gotPeriod {
				c.searchEnd(fmt.Errorf("no Period in the reply to a poll of %v", s.polled))
				return false
			}
			s.polling = false
		case reqFailed:
			// the failed request carries the error
			c.searchEnd(nil)
			return false
		default:
			return false
		}
	}
	if c.est.overflow {
		c.searchEnd(fmt.Errorf("receiver reports it is dropping output (BANDWIDTH OVERFLOW)"))
		return false
	}
	if s.trialing {
		return c.trialAdvance()
	}
	if s.gotPeriod {
		s.gotPeriod = false
		if validPeriod(s.period) {
			c.sendMsgOffs()
			c.searchFound(s.period)
			return false
		}
		s.ignored[s.polled] = true
	}
	for _, mid := range c.est.sortedMids() {
		if !s.ignored[mid] && c.est.tracks[mid].oncePerSecond() {
			c.searchPoll(mid)
			return false
		}
	}
	if !c.lookOver() {
		return false
	}
	c.sendMsgOffs()
	c.startTrials()
	return false
}

// validPeriod reports whether p is a Period that gives one message per
// second on a supported receiver.
func validPeriod(p uint8) bool {
	return p == 1 || p == 5 || p == 10
}

// lookOver reports whether the look stage has nothing more to wait for:
// every message type seen has been ruled out and a silent receiver has
// had gapMax to send something, or the stage has run for stageLimit.
func (c *Configurator) lookOver() bool {
	since := c.now.Sub(c.est.first)
	if since >= stageLimit {
		return true
	}
	if since < gapMax {
		return false
	}
	for mid, tr := range c.est.tracks {
		if !c.search.ignored[mid] && !tr.notOncePerSecond(c.now) {
			return false
		}
	}
	return true
}

// searchPoll polls the stored Period of mid. The reply is used only if
// it names mid.
func (c *Configurator) searchPoll(mid asbin.MsgID) {
	s := c.search
	s.polled = mid
	s.polling = true
	s.gotPeriod = false
	s.pending = c.add(&asReq{mid: asbin.CfgMsgID, packet: asbin.PollCfgMsg(mid), onData: func(m asbin.Msg) {
		if cm, ok := m.(*asbin.CfgMsg); ok && asbin.MakeMsgID(cm.MsgClass, cm.MsgID) == mid {
			s.period = cm.Rate
			s.gotPeriod = true
		}
	}})
}

// probeIDs are the NAV messages that can serve as the probe, in order of
// preference. Each carries a time of week that advances even before the
// receiver has the time, which NMEA sentences do not.
var probeIDs = []asbin.MsgID{asbin.NavTimeID, asbin.NavTimeUtcID, asbin.NavPosLlhID,
	asbin.NavPosEcefID, asbin.NavVelNedID, asbin.NavVelEcefID, asbin.NavDopID, asbin.NavSvInfoID}

// startTrials begins the trial stage: it chooses the probe and the order
// of trials and polls the probe's Period, so that it can be put back.
// The probe is a requested message when one will do, since that needs no
// restoring; otherwise NAV-TIME is borrowed. The first trial is what the
// too-fast messages suggest, if anything; after it the order is 10, 5,
// 1, so that a wrong trial sends the probe less often than once per
// second and not more.
func (c *Configurator) startTrials() {
	s := c.search
	s.trialing = true
	s.probe = asbin.NavTimeID
	for _, mid := range probeIDs {
		if c.requestedOn(mid) {
			s.probe = mid
			break
		}
	}
	hint := c.est.fastHint()
	if hint != 0 {
		s.trials = append(s.trials, hint)
	}
	for _, p := range []uint8{10, 5, 1} {
		if p != hint {
			s.trials = append(s.trials, p)
		}
	}
	c.searchPoll(s.probe)
}

func (c *Configurator) requestedOn(mid asbin.MsgID) bool {
	for _, m := range c.msgs {
		if m.mid == mid {
			return m.on
		}
	}
	return false
}

// trialAdvance takes one step of the trial stage.
func (c *Configurator) trialAdvance() bool {
	s := c.search
	if s.gotPeriod {
		// the probe's Period as found
		s.gotPeriod = false
		s.found = s.period
		s.cur = s.period
		return c.nextTrial()
	}
	if s.start.IsZero() {
		// the trial's set has just been acknowledged; only messages
		// after it count
		delete(c.est.tracks, s.probe)
		s.cur = s.trials[0]
		s.wrote = true
		s.start = c.now
		return false
	}
	tr := c.est.tracks[s.probe]
	if tr.oncePerSecond() {
		c.searchFound(s.cur)
		return false
	}
	silent := tr == nil && c.now.Sub(s.start) >= gapMax
	if silent || (tr != nil && tr.notOncePerSecond(c.now)) {
		s.trials = s.trials[1:]
		return c.nextTrial()
	}
	if c.now.Sub(s.start) >= stageLimit {
		c.searchEnd(fmt.Errorf("%v did not settle at Period %d", s.probe, s.cur))
	}
	return false
}

// nextTrial starts the next trial. A trial at the Period the probe
// already has needs no set, and what has been seen of it counts.
func (c *Configurator) nextTrial() bool {
	s := c.search
	if len(s.trials) == 0 {
		c.searchEnd(fmt.Errorf("%v does not arrive once per second at Period 1, 5 or 10", s.probe))
		return false
	}
	if s.trials[0] == s.cur {
		s.start = c.now
		s.wrote = false
		return true
	}
	s.start = time.Time{}
	s.pending = c.sendMsgRate(s.probe, s.trials[0], false)
	return false
}

// searchFound ends the search with the answer and sets every requested
// message to it, whether or not it already appears to arrive once per
// second: what a request names is what gets configured, and saved.
func (c *Configurator) searchFound(period uint8) {
	s := c.search
	for _, m := range c.msgs {
		if m.on && !(s.wrote && m.mid == s.probe) {
			c.sendMsgRate(m.mid, period, m.nakOK)
		}
	}
	c.restoreProbe(true)
	s.watch.state = reqSucceeded
}

// searchEnd ends the search without an answer: the turn-offs still go
// out, the probe is put back, and nothing is turned on. A nil err means
// a request of the search's own failed and carries the error.
func (c *Configurator) searchEnd(err error) {
	s := c.search
	if !s.trialing {
		c.sendMsgOffs()
	}
	c.restoreProbe(false)
	s.watch.state = reqSucceeded
	if err != nil {
		s.watch.state = reqFailed
		s.watch.err = fmt.Errorf("cannot turn messages on: %w", err)
	}
}

// restoreProbe gives the probe the Period it should end with, if it
// does not have it: 0 if the request turns it off, otherwise the Period
// it was found with. A probe that the request turns on already has the
// answer when the search found one.
func (c *Configurator) restoreProbe(found bool) {
	s := c.search
	if !s.trialing || (found && c.requestedOn(s.probe)) {
		return
	}
	want := s.found
	for _, m := range c.msgs {
		if m.mid == s.probe && !m.on {
			want = 0
		}
	}
	if s.cur != want {
		c.sendMsgRate(s.probe, want, true)
	}
}

// searchDeadline returns when the search next needs to look at the
// clock: the earliest moment after now at which time alone could rule a
// message out or end a stage.
func (c *Configurator) searchDeadline() time.Time {
	s := c.search
	d := c.now.Add(searchTick)
	consider := func(t time.Time) {
		if t.After(c.now) && t.Before(d) {
			d = t
		}
	}
	if s.pending != nil {
		return d
	}
	if !s.trialing {
		consider(c.est.first.Add(gapMax))
		consider(c.est.first.Add(stageLimit))
		for _, tr := range c.est.tracks {
			consider(tr.arr.Add(gapMax))
		}
		return d
	}
	consider(s.start.Add(gapMax))
	consider(s.start.Add(stageLimit))
	if tr := c.est.tracks[s.probe]; tr != nil {
		consider(tr.arr.Add(gapMax))
	}
	return d
}
