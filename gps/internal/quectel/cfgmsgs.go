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

// generateMsgSets appends the message-output set requests: the
// difference between the desired output state and the as-found
// LSTMSG table. Without a complete dump the sets are sent
// unconditionally (no unmodeled-member turn-off is possible then,
// since what is on cannot be known).
func (c *Configurator) generateMsgSets() {
	for _, name := range sortedNames(c.msgWant()) {
		on := c.msgWantState[name]
		if c.found.lstOK && c.msgEnabled(name) == on {
			continue
		}
		c.reqs = append(c.reqs, c.msgRateSet(name, on))
	}
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
		// A complete request turns unmodeled group members off;
		// NMEAMsgOther preserves them.
		if flags&gpsprot.NMEAMsgOther == 0 && c.found.lstOK {
			for name := range c.found.msgRates {
				if _, modeled := want[name]; !modeled && isStdNMEA(name) {
					want[name] = false
				}
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

// msgEnabled reports whether the as-found table has the message
// output on.
func (c *Configurator) msgEnabled(name string) bool {
	line := c.found.msgRates[name]
	return line != nil && line.Rate > 0
}

// msgRateSet builds one CFGMSGRATE set request. PQTM messages carry
// their version field. Message sets are NAK-tolerant: the receiver
// answers ERROR,1 for messages the current mode or firmware lacks
// (e.g. PQTMSVINSTATUS outside effective base mode), which is
// absence, not failure. The ACK records the assumed state.
func (c *Configurator) msgRateSet(name string, on bool) *request {
	rate := 0
	if on {
		rate = 1
	}
	payload := fmt.Sprintf("PQTMCFGMSGRATE,W,%s,%d", name, rate)
	if ver, ok := qtmmsg.PeriodicMsgVer(strings.TrimPrefix(name, "PQTM")); ok && strings.HasPrefix(name, "PQTM") {
		payload += fmt.Sprintf(",%d", ver)
	}
	return &request{
		phase:    phaseMsg,
		payload:  payload,
		sentence: "PQTMCFGMSGRATE",
		onOK: func() {
			if c.found.msgRates == nil {
				c.found.msgRates = make(map[string]*qtmmsg.LstMsgLine)
			}
			if on {
				c.found.msgRates[name] = &qtmmsg.LstMsgLine{PortType: 1, PortID: c.found.uart.Index, MsgName: name, Rate: 1}
			} else {
				delete(c.found.msgRates, name)
			}
		},
		onError: func(rc qtmmsg.ResponseClass) bool { return true },
	}
}

// isStdNMEA reports whether an LSTMSG entry names a standard NMEA
// sentence (the NMEA group), as opposed to a PQTM or RTCM3 message.
func isStdNMEA(name string) bool {
	return !strings.HasPrefix(name, "PQTM") && !strings.HasPrefix(name, "RTCM3-")
}

func sortedNames(m map[string]bool) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
