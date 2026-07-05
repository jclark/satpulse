package septentrio

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// Set requests realize the target's properties. Each set's ack echoes the
// achieved state line, which onReply parses back into nativeProps with the
// same parser the query used: the ack is the readback. A refusal leaves the
// receiver configuration unchanged (verified), so the queried value stands.
// The receiver refuses out-of-range values rather than clamping, so range
// clamping happens here, before the wire.

// gnssTimeScale is the reverse of timeScaleGNSS; other GNSS have no
// TimeScale analogue and leave the argument unset.
var gnssTimeScale = map[gpsprot.GNSS]string{
	gpsprot.GPS: "GPS",
	gpsprot.GAL: "Galileo",
	gpsprot.BDS: "BeiDou",
	gpsprot.GLO: "GLONASS",
}

// generateSetReqs generates the set requests realizing the target's
// properties: signals, the scalars, the PVT mode, then the outputs.
func (c *Configurator) generateSetReqs() {
	c.generateSignalReqs()
	c.generateScalarReqs()
	c.generateModeReqs()
	c.generateOutputReqs()
	c.generateIdentReqs()
	c.generateSaveResetReqs()
}

// generateIdentReqs fetches the receiver identity when no ReceiverSetup
// block has arrived: the Identification internal file is the one ASCII
// carrier of the firmware version, answers on this connection within
// milliseconds, and changes nothing. (The ReceiverSetup one-shot was
// rejected for this: its same-connection delivery requires enabling the
// block on a stream, a side effect whose periodic emissions consumers see.)
// Identity stays best-effort: give-up is success, with identity absent.
func (c *Configurator) generateIdentReqs() {
	if c.rxSetup != nil {
		return
	}
	c.append(&sReq{cmd: "lstInternalFile, Identification", optional: true, onLst: func(content string) {
		if id, err := parseIdent(content); err == nil {
			c.ident = id
		}
	}})
}

// generateSaveResetReqs realizes the NVM operations, last: a save persists
// everything realized before it, and a reset ends the conversation. All
// mappings hardware-verified: eccf Current,Boot saves; eccf Boot,Current
// reloads the SAVED configuration in place (no restart); exeResetReceiver
// replies with a framed STOP>-terminated ack BEFORE the reboot drops the
// connection, so reset requests complete normally - the receiver then goes
// quiet for up to ~30 s while it reboots (Hard boots from the Boot config;
// the erase argument resets the configuration to factory defaults).
func (c *Configurator) generateSaveResetReqs() {
	o := &c.target.Opts
	if o.Save != gpsprot.SaveNone {
		c.addReq("exeCopyConfigFile, Current, Boot", nil)
	}
	switch o.Reset {
	case gpsprot.ResetReload:
		c.addReq("exeCopyConfigFile, Boot, Current", nil)
	case gpsprot.ResetCold:
		c.addReq("exeResetReceiver, Hard, PVTData+SatData", nil)
	case gpsprot.ResetFactory:
		c.addReq("exeResetReceiver, Hard, all", nil)
	}
}

// generateScalarReqs generates the set requests for the scalar properties.
func (c *Configurator) generateScalarReqs() {
	props, np := &c.target.Props, &c.np
	if v, ok := props.GetMinElevation(); ok {
		deg := min(max(int((v+gpsprot.Degrees/2)/gpsprot.Degrees), 0), 90)
		c.addReq(fmt.Sprintf("setElevationMask, PVT, %d", deg), np.parseElevationMask)
	}
	if cmd := c.ppsSetCmd(); cmd != "" {
		c.addReq(cmd, np.parsePPSParameters)
	}
	if v, ok := props.GetAntennaCableDelay(); ok {
		ns := min(max(float64(v)/float64(time.Nanosecond), -10000), 10000)
		c.addReq("setCalibCommonDelay, "+formatFloat(ns), np.parseCalibCommonDelay)
	}
	if v, ok := props.GetRTCMBaseID(); ok {
		c.addReq(fmt.Sprintf("setRTCMv3Formatting, %d", min(v, 4095)), np.parseRTCMv3Formatting)
	}
	if v, ok := props.GetNavMsgAuth(); ok && c.caps.caps["GalOSNMA"] {
		// Loose is the owner-ruled interim level; strict needs trusted time
		// (exeSetTime) and follows the osnma branch. MTRoot is always left
		// alone (blank in live operation); disabling clears MeasLevel too,
		// since an active MeasLevel alone still applies authentication.
		if v == gpsprot.NavMsgAuthOSNMA {
			c.addReq("setGalOSNMAUsage, loose", np.parseGalOSNMAUsage)
		} else {
			c.addReq("setGalOSNMAUsage, off, , off", np.parseGalOSNMAUsage)
		}
	}
}

// sstGNSSNames maps device-independent GNSS values to the constellation
// aliases setSatelliteTracking and setSignalTracking accept.
var sstGNSSNames = map[gpsprot.GNSS]string{
	gpsprot.GPS:   "GPS",
	gpsprot.GLO:   "GLONASS",
	gpsprot.GAL:   "GALILEO",
	gpsprot.BDS:   "BEIDOU",
	gpsprot.QZSS:  "QZSS",
	gpsprot.NAVIC: "NAVIC",
	gpsprot.SBAS:  "SBAS",
}

// generateSignalReqs realizes SignalsEnabled with three commands: the
// constellation gate (sst), signal tracking (snt), and signal usage (snu).
// snt has no "-" removal (verified), so tracking and usage are written as
// explicit full lists: the mapped target signals plus whatever receiver-only
// signals (GALE5, GLOL2P, QZSL1CB) the query phase found present - those have
// no device-independent name and are preserved as found. The achieved value
// comes from the snt ack's own state line.
func (c *Configurator) generateSignalReqs() {
	ss, ok := c.target.Props.GetSignalsEnabled()
	if !ok {
		return
	}
	var names []string
	for sig := range septSignalNames {
		if ss.Contains(sig) {
			names = append(names, septSignalNames[sig])
		}
	}
	slices.Sort(names)
	tracking := append(names, unmappedSignals(c.np.tracking)...)
	gset := ss.GNSSSet()
	var consts []string
	for _, g := range gset.Items() {
		if n, ok := sstGNSSNames[g]; ok {
			consts = append(consts, n)
		}
	}
	constList := strings.Join(consts, "+")
	if gset == gpsprot.AllGNSSSet {
		constList = "all"
	}
	c.addReq("setSatelliteTracking, "+listOrNone(constList), nil)
	c.addReq("setSignalTracking, "+listOrNone(strings.Join(tracking, "+")), c.np.parseSignalTracking)
	// The PVT usage argument's enum has no GEO entries (SBAS ranging is not
	// a PVT input on this receiver; a GEO signal there is refused), so the
	// SBAS signals go only in the NavData list.
	pvt, nav := withoutGEO(names), names
	if u := c.np.usage; u != nil {
		pvt = append(pvt, unmappedSignals(u.pvt)...)
		nav = append(slices.Clone(names), unmappedSignals(u.navData)...)
	}
	c.addReq("setSignalUsage, "+listOrNone(strings.Join(pvt, "+"))+", "+listOrNone(strings.Join(nav, "+")),
		c.np.parseSignalUsage)
}

// withoutGEO returns names with the GEO (SBAS) signals removed.
func withoutGEO(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != "GEOL1" && n != "GEOL5" {
			out = append(out, n)
		}
	}
	return out
}

// unmappedSignals returns the signals in names that have no
// device-independent analogue.
func unmappedSignals(names []string) []string {
	var out []string
	for _, n := range names {
		if _, ok := septSignalFromName[n]; !ok {
			out = append(out, n)
		}
	}
	return out
}

// listOrNone returns the receiver's empty-list spelling for an empty list.
func listOrNone(list string) string {
	if list == "" {
		return "none"
	}
	return list
}

// generateModeReqs realizes the Mode property. A rover mode set omits the
// RoverMode argument, preserving the receiver's current rover-mode list; a
// static position is written to slot 1 before the mode switch references it
// (verified order).
func (c *Configurator) generateModeReqs() {
	m, ok := c.target.Props.GetMode()
	if !ok {
		return
	}
	np := &c.np
	if !m.Static {
		c.addReq("setPVTMode, Rover", np.parsePVTMode)
		return
	}
	switch m.PosType {
	case gpsprot.PosTypeNone:
		c.addReq("setPVTMode, Static, , auto", np.parsePVTMode)
	case gpsprot.PosTypeLLH:
		c.addReq(fmt.Sprintf("setStaticPosGeodetic, Geodetic1, %.9f, %.9f, %.4f",
			float64(m.FixedPosLLH[0])/float64(gpsprot.Degrees),
			float64(m.FixedPosLLH[1])/float64(gpsprot.Degrees),
			m.Height.Meters()), np.parseStaticPos)
		c.addReq("setPVTMode, Static, , Geodetic1", np.parsePVTMode)
	case gpsprot.PosTypeECEF:
		c.addReq(fmt.Sprintf("setStaticPosCartesian, Cartesian1, %.4f, %.4f, %.4f",
			m.FixedPosECEF[0].Meters(), m.FixedPosECEF[1].Meters(), m.FixedPosECEF[2].Meters()),
			np.parseStaticPos)
		c.addReq("setPVTMode, Static, , Cartesian1", np.parsePVTMode)
	}
}

// ppsSetCmd builds the setPPSParameters command for the target's TimePulse
// and TimeGNSS properties, omitting arguments that keep their current value.
// Returns "" when the target sets none of them.
func (c *Configurator) ppsSetCmd() string {
	props, np := &c.target.Props, &c.np
	if w, ok := props.GetTimePulseWidth(); ok && w == 0 {
		return "setPPSParameters, off"
	}
	// Args: Interval, Polarity, Delay, TimeScale, MaxHoldover, PulseWidth.
	// Delay moves only the pulse, not the pseudoranges: never touched
	// (AntennaCableDelay is setCalibCommonDelay).
	args := make([]string, 6)
	if p, ok := props.GetTimePulsePeriod(); ok {
		args[0] = nearestPPSInterval(p)
	}
	if r, ok := props.GetTimePulsePolarityRising(); ok {
		if r {
			args[1] = "Low2High"
		} else {
			args[1] = "High2Low"
		}
	}
	if g, ok := props.GetTimeGNSS(); ok {
		args[3] = gnssTimeScale[g] // unmapped GNSS leaves TimeScale as is
	} else if align, ok := props.GetTimePulseAlignToGNSS(); ok {
		if !align {
			args[3] = "RxClock"
		} else if np.pps == nil || np.pps.timeScale == "RxClock" {
			args[3] = "GPS"
		}
	}
	if locked, ok := props.GetTimePulseOnlyWhenLocked(); ok {
		if locked {
			args[4] = "1"
		} else {
			args[4] = "0"
		}
	}
	if w, ok := props.GetTimePulseWidth(); ok {
		ms := min(max(float64(w)/float64(time.Millisecond), 0.000001), 1000)
		args[5] = formatFloat(ms)
		if args[0] == "" && (c.np.pps == nil || c.np.pps.interval == "off") {
			// A width alone cannot enable a disabled pulse; supply the
			// default 1 s interval.
			args[0] = "sec1"
		}
	}
	last := -1
	for i, a := range args {
		if a != "" {
			last = i
		}
	}
	if last < 0 {
		return ""
	}
	return "setPPSParameters, " + strings.Join(args[:last+1], ", ")
}

// nearestPPSInterval returns the Interval enum value whose period is closest
// to d: the receiver's interval choices are fixed, and a non-representable
// period is realized best-effort with the truth coming from the readback.
func nearestPPSInterval(d time.Duration) string {
	best, bestDiff := "", time.Duration(0)
	for name, period := range ppsIntervals {
		diff := (period - d).Abs()
		if best == "" || diff < bestDiff || (diff == bestDiff && period < ppsIntervals[best]) {
			best, bestDiff = name, diff
		}
	}
	return best
}

// formatFloat formats a float argument in the shortest exact decimal form.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
