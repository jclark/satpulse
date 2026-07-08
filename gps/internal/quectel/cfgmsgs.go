package quectel

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/qtmmsg"
)

// nmeaMsgMembers are the modeled members of the NMEA message group,
// with the firmware version that introduced them (zero = always).
var nmeaMsgMembers = []struct {
	flag gpsprot.NMEAMsgFlags
	name string
	min  fwVersion
}{
	{gpsprot.NMEAMsgRMC, "RMC", fwVersion{}},
	{gpsprot.NMEAMsgGGA, "GGA", fwVersion{}},
	{gpsprot.NMEAMsgGSA, "GSA", fwVersion{}},
	{gpsprot.NMEAMsgGSV, "GSV", fwVersion{}},
	{gpsprot.NMEAMsgZDA, "ZDA", fwZDA},
	{gpsprot.NMEAMsgVTG, "VTG", fwVersion{}},
	{gpsprot.NMEAMsgGLL, "GLL", fwVersion{}},
}

// pvtMsgModeled are the PQTM navigation messages the PVT group
// manages; PVTMsgOff turns off the unneeded ones. PQTMTXT is
// deliberately excluded: it carries notices, not PVT data.
var pvtMsgModeled = []string{
	"PQTMPVT", "PQTMNAV", "PQTMVEL", "PQTMEPE", "PQTMDOP", "PQTMEOE",
	"PQTMSVINSTATUS", "PQTMPL", "PQTMODO", "PQTMPPPNAV",
}

// rtcmMSMNames are the per-constellation MSM message families (GPS,
// GLONASS, Galileo, QZSS, BDS, NavIC); the emitted MSM type (4-7)
// comes from CFGRTCM.
var rtcmMSMNames = []string{
	"RTCM3-107X", "RTCM3-108X", "RTCM3-109X", "RTCM3-111X", "RTCM3-112X", "RTCM3-113X",
}

// generateMsgSets appends the message-output set requests: each
// modeled member the target names is set to its desired state. The
// receiver's current message table is not read back, so a set is sent
// even when the message is already in the requested state. A complete
// NMEA or RTCM request that leaves its protocol with no output is
// realized by switching the protocol off in PQTMCFGPROT instead
// (protOutput), skipping that group's per-message disables.
func (c *Configurator) generateMsgSets() {
	want := c.msgWant()
	prot, nmeaOff, rtcmOff := c.protOutput(want)
	if prot != nil {
		c.reqs = append(c.reqs, prot)
	}
	for _, name := range sortedNames(want) {
		if nmeaOff && isStdNMEA(name) || rtcmOff && isRTCMMsg(name) {
			continue
		}
		c.reqs = append(c.reqs, c.msgRateSet(name, c.msgWantState[name]))
	}
	c.generateRTCMTypeSet()
}

// protOutput drives the port's NMEA and RTCM3 output-protocol bits to
// match the target's complete (non-Other) NMEA and RTCM requests: on
// when any of the group's messages is wanted, off when none is. The
// off case switches the whole protocol dark - including sentences the
// model does not name - so per-message disables are redundant; the
// returned nmeaOff/rtcmOff say so. Whole-protocol output toggles live
// via PQTMCFGPROT and need the port's protocol readback; with none
// available nothing is managed and the per-message sets stand.
func (c *Configurator) protOutput(want map[string]bool) (req *request, nmeaOff, rtcmOff bool) {
	if c.found.prot == nil {
		return nil, false, false
	}
	out := c.found.prot.OutputProt
	newOut := out
	opts := &c.target.Opts
	if opts.NMEAMsg.IsSet() && opts.NMEAMsg.Get()&gpsprot.NMEAMsgOther == 0 {
		if anyWanted(want, isStdNMEA) {
			newOut |= qtmmsg.ProtNMEA
		} else {
			newOut &^= qtmmsg.ProtNMEA
			nmeaOff = true
		}
	}
	if opts.RTCMMsg.IsSet() && opts.RTCMMsg.Get()&gpsprot.RTCMMsgOther == 0 {
		if anyWanted(want, isRTCMMsg) {
			newOut |= qtmmsg.ProtRTCM3
		} else {
			newOut &^= qtmmsg.ProtRTCM3
			rtcmOff = true
		}
	}
	if newOut != out {
		m := *c.found.prot
		m.OutputProt = newOut
		req = c.propSet(&m, func() { c.found.prot = &m })
		req.phase = phaseMsg
	}
	return req, nmeaOff, rtcmOff
}

// generateRTCMTypeSet writes CFGRTCM when the requested MSM type
// differs from the as-found one. MSM7 wins when both types are
// requested. The MSM type is stored-only - message enables apply
// live, but the type takes effect only after a save and reset, even
// for freshly enabled messages (hardware-verified) - so the write
// rides the explicit save+reset gate. Without it the stored type is
// emitted; frontends can tell from the observed message types.
func (c *Configurator) generateRTCMTypeSet() {
	flags := c.target.Opts.RTCMMsg
	if !flags.IsSet() || flags.Get()&(gpsprot.RTCMMsgMSM4|gpsprot.RTCMMsgMSM7) == 0 {
		return
	}
	msmType := uint8(4)
	if flags.Get()&gpsprot.RTCMMsgMSM7 != 0 {
		msmType = 7
	}
	if c.found.rtcm != nil && c.found.rtcm.MSMType == msmType {
		return
	}
	if !c.savesAndResets() {
		return
	}
	m := qtmmsg.CfgRtcm{MSMType: msmType, MSMElevThd: -90.0,
		Reserved0: "07", Reserved1: "06", EPHMode: 1}
	if f := c.found.rtcm; f != nil {
		m = *f
		m.MSMType = msmType
	}
	r := c.propSet(&m, func() { c.found.rtcm = &m })
	r.phase = phaseMsg
	c.reqs = append(c.reqs, r)
}

// msgWant computes the desired on/off state per message name from
// the target's message options, and caches it on the Configurator.
func (c *Configurator) msgWant() map[string]bool {
	want := make(map[string]bool)
	opts := &c.target.Opts
	if opts.NMEAMsg.IsSet() {
		flags := opts.NMEAMsg.Get()
		for _, m := range nmeaMsgMembers {
			if c.fw.has(m.min) {
				want[m.name] = flags&m.flag != 0
			}
		}
	}
	if opts.SatsMsg.IsSet() {
		if opts.SatsMsg.Get()&gpsprot.SatsMsgAny != 0 {
			want["GSV"] = true // GSV carries both satellite and signal data
		} else if !want["GSV"] {
			want["GSV"] = false
		}
	}
	if opts.RTCMMsg.IsSet() {
		flags := opts.RTCMMsg.Get()
		msm := flags&(gpsprot.RTCMMsgMSM4|gpsprot.RTCMMsgMSM7) != 0
		for _, name := range rtcmMSMNames {
			want[name] = msm
		}
		want["RTCM3-1005"] = flags&gpsprot.RTCMMsgARP != 0
	}
	if pvt := opts.PVTMsg; pvt.IsSet() {
		if pvt&(gpsprot.PVTMsgPos|gpsprot.PVTMsgVel|gpsprot.PVTMsgTime|gpsprot.PVTMsgLeapSecond) != 0 {
			want["PQTMPVT"] = true
		}
		if pvt&gpsprot.PVTMsgQuality != 0 {
			want["PQTMEPE"] = true
			want["PQTMDOP"] = true
		}
		if pvt&gpsprot.PVTMsgEpoch != 0 && c.fw.has(fwNAV) {
			want["PQTMEOE"] = true
		}
		if pvt&gpsprot.PVTMsgSurvey != 0 {
			want["PQTMSVINSTATUS"] = true
		}
		if pvt&gpsprot.PVTMsgOff != 0 {
			for _, name := range pvtMsgModeled {
				if !want[name] {
					want[name] = false
				}
			}
		}
	}
	c.msgWantState = want
	return want
}

// msgRateSet builds one CFGMSGRATE set request. PQTM messages carry
// their version field. Message sets are NAK-tolerant: the receiver
// answers ERROR,1 for messages the current mode or firmware lacks
// (e.g. PQTMSVINSTATUS outside effective base mode), which is
// absence, not failure.
func (c *Configurator) msgRateSet(name string, on bool) *request {
	rate := 0
	if on {
		rate = 1
	}
	payload := fmt.Sprintf("PQTMCFGMSGRATE,W,%s,%d", name, rate)
	if ver, ok := qtmmsg.PeriodicMsgVer(strings.TrimPrefix(name, "PQTM")); ok && strings.HasPrefix(name, "PQTM") {
		payload += fmt.Sprintf(",%d", ver)
	} else if isMSMName(name) {
		// MSM sets require the time-offset field in both directions.
		payload += ",0"
	}
	return &request{
		phase:    phaseMsg,
		payload:  payload,
		sentence: "PQTMCFGMSGRATE",
		onError:  func(rc qtmmsg.ResponseClass) bool { return true },
	}
}

// isMSMName reports whether a message name is an MSM family entry
// (e.g. "RTCM3-107X"), which carries a time-offset field.
func isMSMName(name string) bool {
	return strings.HasPrefix(name, "RTCM3-1") && strings.HasSuffix(name, "X")
}

// isStdNMEA reports whether a message name is a standard NMEA sentence
// (the NMEA output protocol), as opposed to a PQTM or RTCM3 message.
func isStdNMEA(name string) bool {
	return !strings.HasPrefix(name, "PQTM") && !strings.HasPrefix(name, "RTCM3-")
}

// isRTCMMsg reports whether a message name is an RTCM3 message (the
// RTCM3 output protocol).
func isRTCMMsg(name string) bool {
	return strings.HasPrefix(name, "RTCM3-")
}

// anyWanted reports whether any wanted (on) message name satisfies pred.
func anyWanted(want map[string]bool, pred func(string) bool) bool {
	for name, on := range want {
		if on && pred(name) {
			return true
		}
	}
	return false
}

func sortedNames(m map[string]bool) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
