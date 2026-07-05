package septentrio

import (
	"slices"
	"strings"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// Message output control owns SBF Stream1 and NMEA Stream1 (the streams the
// verified message files and the shipped daemon configuration use); all
// other streams are out-of-group and never touched. Within the owned SBF
// stream, each output option governs its own block CLASS: an option that is
// present replaces its class's blocks (PVT messages replace only with the
// PVTMsgOff flag, and add otherwise, per the flag's contract), an absent
// option leaves its class as found, and blocks shared between classes (e.g.
// MeasEpoch: satellite signal strengths and raw observations) stay enabled
// while any class still wants them.
//
// The block choices are fixed against the conversion layer's decode set
// (gps/internal/septentrio on the septentrio-msg branch): PVTGeodetic/
// PVTCartesian (position, velocity, solution time; PVTCartesian is also the
// SurveyMsg source), xPPSOffset (time-pulse time), GPS/GAL/BDSUtc (leap
// seconds), DOP + DiffCorrIn + BaseStation (quality and corrections),
// EndOfPVT (epoch), ChannelStatus (+MeasEpoch for signal strengths). Raw
// output additionally enables MeasExtra and the navigation-data blocks,
// which the stack does not decode: enabling them is configuration for
// external consumers; their processing is out of scope here.

const ownStream = "Stream1"

// streamState is an SBFOutput or NMEAOutput state line for one stream.
type streamState struct {
	cd       string   // connection descriptor, "none" when unbound
	messages []string // block or sentence list
	interval string   // interval enum, "off" when disabled
}

func (np *nativeProps) parseSBFOutput(r *Reply) {
	np.sbfStream = parseStreamState(r, "SBFOutput")
}

func (np *nativeProps) parseNMEAOutput(r *Reply) {
	np.nmeaStream = parseStreamState(r, "NMEAOutput")
}

func parseStreamState(r *Reply, name string) *streamState {
	f := findState(r, name)
	if len(f) < 5 || f[1] != ownStream {
		return nil
	}
	return &streamState{cd: f[2], messages: signalList(f[3]), interval: f[4]}
}

// pvtBlocks returns the SBF blocks realizing the requested PVT messages.
func pvtBlocks(f gpsprot.PVTMsgFlags) []string {
	var b []string
	pvt := "PVTGeodetic"
	if f&gpsprot.PVTMsgECEF != 0 {
		pvt = "PVTCartesian"
	}
	if f&(gpsprot.PVTMsgPos|gpsprot.PVTMsgVel|gpsprot.PVTMsgTime) != 0 {
		b = append(b, pvt)
	}
	if f&gpsprot.PVTMsgTimePulse != 0 {
		b = append(b, "xPPSOffset")
	}
	if f&(gpsprot.PVTMsgLeapSecond|gpsprot.PVTMsgTAI) != 0 {
		b = append(b, "GPSUtc", "GALUtc", "BDSUtc")
	}
	if f&gpsprot.PVTMsgSurvey != 0 {
		b = append(b, "PVTCartesian")
	}
	if f&gpsprot.PVTMsgQuality != 0 {
		b = append(b, "DOP", "DiffCorrIn", "BaseStation")
	}
	if f&gpsprot.PVTMsgEpoch != 0 {
		b = append(b, "EndOfPVT")
	}
	return b
}

// pvtClass is every block pvtBlocks can enable.
var pvtClass = []string{"PVTGeodetic", "PVTCartesian", "xPPSOffset",
	"GPSUtc", "GALUtc", "BDSUtc", "DOP", "DiffCorrIn", "BaseStation", "EndOfPVT"}

func satsBlocks(f gpsprot.SatsMsgFlags) []string {
	var b []string
	if f&gpsprot.SatsMsgSat != 0 {
		b = append(b, "ChannelStatus")
	}
	if f&gpsprot.SatsMsgSignal != 0 {
		b = append(b, "ChannelStatus", "MeasEpoch")
	}
	return b
}

var satsClass = []string{"ChannelStatus", "MeasEpoch"}

func rawBlocks(f gpsprot.RawMsgFlags) []string {
	var b []string
	if f&gpsprot.RawMsgObs != 0 {
		b = append(b, "MeasEpoch", "MeasExtra")
	}
	if f&gpsprot.RawMsgNavData != 0 {
		b = append(b, "GPSNav", "GPSCNav", "GPSIon", "GPSUtc", "GLONav", "GLOTime",
			"GALNav", "GALIon", "GALUtc", "GEONav", "BDSNav", "BDSIon", "BDSUtc",
			"QZSNav", "NavICLNav")
	}
	return b
}

var rawClass = []string{"MeasEpoch", "MeasExtra", "GPSNav", "GPSCNav", "GPSIon",
	"GPSUtc", "GLONav", "GLOTime", "GALNav", "GALIon", "GALUtc", "GEONav",
	"BDSNav", "BDSIon", "BDSUtc", "QZSNav", "NavICLNav"}

// touchesSBFOutput reports whether the target changes the owned SBF stream.
func (c *Configurator) touchesSBFOutput() bool {
	o := &c.target.Opts
	return o.PVTMsg.IsSet() || o.SatsMsg.IsSet() || o.RawMsg.IsSet()
}

// sbfOutputList computes the owned stream's new block list: touched classes
// contribute their desired blocks, and blocks outside every touched class
// (user additions, untouched classes) are preserved. A touched class's
// disable wins even when an untouched class's universe contains the block:
// presence does not record which option enabled a block, so an explicit
// disable is the only honest signal.
func (c *Configurator) sbfOutputList() []string {
	o := &c.target.Opts
	var cur []string
	if c.np.sbfStream != nil {
		cur = c.np.sbfStream.messages
	}
	pvtDesired := pvtBlocks(o.PVTMsg)
	if o.PVTMsg&gpsprot.PVTMsgOff == 0 {
		// Without the Off flag, PVT messages only get added.
		pvtDesired = append(pvtDesired, intersect(cur, pvtClass)...)
	}
	classes := []struct {
		touched bool
		desired []string
		class   []string
	}{
		{o.PVTMsg.IsSet(), pvtDesired, pvtClass},
		{o.SatsMsg.IsSet(), satsBlocks(o.SatsMsg.Get()), satsClass},
		{o.RawMsg.IsSet(), rawBlocks(o.RawMsg.Get()), rawClass},
	}
	keep := func(block string) bool {
		inTouched, wanted := false, false
		for _, cl := range classes {
			if !cl.touched || !slices.Contains(cl.class, block) {
				continue
			}
			inTouched = true
			if slices.Contains(cl.desired, block) {
				wanted = true
			}
		}
		return !inTouched || wanted
	}
	var list []string
	for _, b := range cur {
		if keep(b) {
			list = append(list, b)
		}
	}
	for _, cl := range classes {
		if !cl.touched {
			continue
		}
		for _, b := range cl.desired {
			if !slices.Contains(list, b) {
				list = append(list, b)
			}
		}
	}
	slices.Sort(list)
	return list
}

// nmeaFlagSentences maps each NMEA message flag to its sentence name.
var nmeaFlagSentences = []struct {
	flag gpsprot.NMEAMsgFlags
	name string
}{
	{gpsprot.NMEAMsgRMC, "RMC"},
	{gpsprot.NMEAMsgGGA, "GGA"},
	{gpsprot.NMEAMsgGSA, "GSA"},
	{gpsprot.NMEAMsgGSV, "GSV"},
	{gpsprot.NMEAMsgZDA, "ZDA"},
	{gpsprot.NMEAMsgVTG, "VTG"},
	{gpsprot.NMEAMsgGLL, "GLL"},
}

// rtcmMessages returns the RTCMv3 message list realizing the flags. The
// MSM4/MSM7 aliases expand receiver-side to the per-constellation message
// set (verified: 1074-1134 for MSM4, including QZSS), so enabled-GNSS
// filtering happens in the receiver; ARP is RTCM1006 (1005 plus antenna
// height, the verified message-file choice); RTCMMsgLax changes nothing
// because the receiver accepts every choice the flags can express.
func rtcmMessages(f gpsprot.RTCMMsgFlags) []string {
	var m []string
	if f&gpsprot.RTCMMsgMSM4 != 0 {
		m = append(m, "MSM4")
	}
	if f&gpsprot.RTCMMsgMSM7 != 0 {
		m = append(m, "MSM7")
	}
	if f&gpsprot.RTCMMsgARP != 0 {
		m = append(m, "RTCM1006")
	}
	return m
}

// generateOutputReqs realizes the message output options on the owned
// streams and this connection's RTCM output list. A stream write is one
// self-contained command (port, complete list, 1 Hz); the acked state line
// is the readback. Actual RTCM emission additionally requires base mode and
// a reference position - observation, not enablement, is the evidence.
func (c *Configurator) generateOutputReqs() {
	np := &c.np
	if c.touchesSBFOutput() {
		// An empty Messages argument keeps the current list (the
		// omitted-argument rule); an empty selection must be spelled none.
		list := listOrNone(strings.Join(c.sbfOutputList(), "+"))
		c.addReq("setSBFOutput, "+ownStream+", "+c.port+", "+list+", sec1",
			np.parseSBFOutput)
	}
	if c.target.Opts.NMEAMsg.IsSet() {
		var names []string
		f := c.target.Opts.NMEAMsg.Get()
		for _, e := range nmeaFlagSentences {
			if f&e.flag != 0 {
				names = append(names, e.name)
			}
		}
		if len(names) == 0 {
			c.addReq("setNMEAOutput, "+ownStream+", , none", np.parseNMEAOutput)
		} else {
			c.addReq("setNMEAOutput, "+ownStream+", "+c.port+", "+strings.Join(names, "+")+", sec1",
				np.parseNMEAOutput)
		}
	}
	if c.target.Opts.RTCMMsg.IsSet() {
		list := rtcmMessages(c.target.Opts.RTCMMsg.Get())
		c.addReq("setRTCMv3Output, "+c.port+", "+listOrNone(strings.Join(list, "+")), nil)
	}
}

// intersect returns the elements of list that are in class.
func intersect(list, class []string) []string {
	var out []string
	for _, v := range list {
		if slices.Contains(class, v) {
			out = append(out, v)
		}
	}
	return out
}
