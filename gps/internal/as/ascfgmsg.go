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
