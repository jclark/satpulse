package as

import (
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

// rtcmEphIDs are the broadcast-ephemeris RTCM targets. They are not in
// the device-independent RTCM group vocabulary, but they are part of
// the receiver's RTCM output (the TAU1302 ships with them enabled), so
// a complete RTCM request turns them off along with unnamed MSM types.
var rtcmEphIDs = []asbin.MsgID{asbin.RtcmEphGpsID, asbin.RtcmEphGloID,
	asbin.RtcmEphBdsID, asbin.RtcmEphQzssID, asbin.RtcmEphGalID}

// generateRTCMReqs configures RTCM output via CFG-MSG 0xF8 targets. A
// wire-format request is complete: named types on, everything else in
// the group off. All enables are NAK-tolerant: a receiver without RTCM
// output (the TAU1201) NAKs every 0xF8 target, and the absence of any
// achieved RTCM output is the statement - not an error. ARP (1005) is
// only emitted while a fixed position is set; enabling it without one
// is accepted and silent.
func (c *Configurator) generateRTCMReqs(flags gpsprot.RTCMMsgFlags) {
	msm4 := flags&gpsprot.RTCMMsgMSM4 != 0
	msm7 := flags&gpsprot.RTCMMsgMSM7 != 0
	for i := range rtcmMSM4IDs {
		c.addMsgRate(rtcmMSM4IDs[i], msm4)
		c.addMsgRate(rtcmMSM7IDs[i], msm7)
	}
	c.addMsgRate(asbin.RtcmArpID, flags&gpsprot.RTCMMsgARP != 0)
	for _, mid := range rtcmEphIDs {
		c.addMsgRate(mid, false)
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
	c.addReqNakOK(&asbin.RxmDumpRaw{Enable: enable}, nil)
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

// addMsgRate sets a message's output rate to 1 or 0 via CFG-MSG. A NAK
// is acceptable: the message may not exist on this firmware (the
// TAU951M lacks NAV-SVSTATE, for example), and undeliverable
// information shows as absence.
func (c *Configurator) addMsgRate(mid asbin.MsgID, on bool) {
	var rate uint8
	if on {
		rate = 1
	}
	cls, id := mid.Unpack()
	c.addReqNakOK(&asbin.CfgMsg{MsgClass: cls, MsgID: id, Rate: rate}, nil)
}

// generateNMEAReqs sets the rate of each standard NMEA sentence via
// CFG-MSG: named sentences on, the rest off (a wire-format request is
// complete). These are not nakOK: every tested unit accepts all seven
// ids, so a NAK is a genuine refusal. GSV goes first because it
// carries the most traffic, so turning it off frees the line soonest.
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
		var rate uint8
		if flags&m.flag != 0 {
			rate = 1
		}
		cls, id := m.mid.Unpack()
		c.addReq(&asbin.CfgMsg{MsgClass: cls, MsgID: id, Rate: rate})
	}
}
