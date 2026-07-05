package septentrio

import (
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// nativeProps holds receiver state in the receiver's own representation,
// filled from query replies and set acks (ack-is-readback); ConfigProps()
// converts it to device-independent properties. A nil/absent entry means the
// value was never read back and the property is not reported.
type nativeProps struct {
	elevMaskPVT  *int // degrees
	pps          *ppsParams
	tracking     []string // snt signal list
	haveTracking bool
	usage        *signalUsage
	pvtMode      *pvtMode
	staticPos    *staticPos
	osnma        *osnmaUsage
	rtcmBaseID   *int
	cableDelayNs *float64
}

// ppsParams is the PPSParameters state line:
//
//	PPSParameters, sec1, Low2High, 0.00, GPS, 60, 5.000000
type ppsParams struct {
	interval    string  // Interval enum (off, msec10..sec60, kHz../MHz..)
	polarity    string  // Low2High / High2Low
	delayNs     float64 // Delay
	timeScale   string  // GPS/Galileo/BeiDou/GLONASS/UTC/RxClock
	maxHoldover int     // seconds; 0 = pulses continue indefinitely
	widthMs     float64 // PulseWidth
}

// signalUsage is the SignalUsage state line: two lists (PVT, NavData).
type signalUsage struct {
	pvt     []string
	navData []string
}

// pvtMode is the PVTMode state line:
//
//	PVTMode, Rover, StandAlone+DGNSS+RTKFixed, auto
type pvtMode struct {
	mode      string // Rover / Static
	roverMode string // RoverMode list (Rover only)
	staticRef string // auto or a GeodeticN/CartesianN slot (Static only)
}

// staticPos is a StaticPosGeodetic or StaticPosCartesian state line.
type staticPos struct {
	slot     string // Geodetic1, Cartesian1, ...
	geodetic bool
	lat, lon float64 // degrees (geodetic)
	height   float64 // m (geodetic)
	x, y, z  float64 // m (cartesian)
}

// osnmaUsage is the GalOSNMAUsage state line: (PVTLevel, MTRoot, MeasLevel).
type osnmaUsage struct {
	pvtLevel  string // off / loose / strict
	measLevel string // off / loose
}

// stateFields splits a state line into trimmed comma-separated fields.
func stateFields(s string) []string {
	fields := strings.Split(s, ",")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}

// findState returns the fields of the first state line led by name, or nil.
func findState(r *Reply, name string) []string {
	for _, s := range r.States {
		if f := stateFields(s); f[0] == name {
			return f
		}
	}
	return nil
}

// signalList splits a +-joined signal list; "none" and "" are empty.
func signalList(s string) []string {
	if s == "" || s == "none" {
		return nil
	}
	return strings.Split(s, "+")
}

func (np *nativeProps) parseElevationMask(r *Reply) {
	f := findState(r, "ElevationMask")
	if len(f) < 3 || f[1] != "PVT" {
		return
	}
	if v, err := strconv.Atoi(f[2]); err == nil {
		np.elevMaskPVT = &v
	}
}

func (np *nativeProps) parsePPSParameters(r *Reply) {
	f := findState(r, "PPSParameters")
	if len(f) < 7 {
		return
	}
	delay, err1 := strconv.ParseFloat(f[3], 64)
	holdover, err2 := strconv.Atoi(f[5])
	width, err3 := strconv.ParseFloat(f[6], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return
	}
	np.pps = &ppsParams{
		interval:    f[1],
		polarity:    f[2],
		delayNs:     delay,
		timeScale:   f[4],
		maxHoldover: holdover,
		widthMs:     width,
	}
}

func (np *nativeProps) parseSignalTracking(r *Reply) {
	f := findState(r, "SignalTracking")
	if len(f) < 2 {
		return
	}
	np.tracking = signalList(f[1])
	np.haveTracking = true
}

func (np *nativeProps) parseSignalUsage(r *Reply) {
	f := findState(r, "SignalUsage")
	if len(f) < 3 {
		return
	}
	np.usage = &signalUsage{pvt: signalList(f[1]), navData: signalList(f[2])}
}

func (np *nativeProps) parsePVTMode(r *Reply) {
	f := findState(r, "PVTMode")
	if len(f) < 2 {
		return
	}
	pm := &pvtMode{mode: f[1]}
	if len(f) > 2 {
		pm.roverMode = f[2]
	}
	if len(f) > 3 {
		pm.staticRef = f[3]
	}
	np.pvtMode = pm
}

func (np *nativeProps) parseStaticPos(r *Reply) {
	if f := findState(r, "StaticPosGeodetic"); len(f) >= 5 {
		lat, err1 := strconv.ParseFloat(f[2], 64)
		lon, err2 := strconv.ParseFloat(f[3], 64)
		height, err3 := strconv.ParseFloat(f[4], 64)
		if err1 == nil && err2 == nil && err3 == nil {
			np.staticPos = &staticPos{slot: f[1], geodetic: true, lat: lat, lon: lon, height: height}
		}
		return
	}
	if f := findState(r, "StaticPosCartesian"); len(f) >= 5 {
		x, err1 := strconv.ParseFloat(f[2], 64)
		y, err2 := strconv.ParseFloat(f[3], 64)
		z, err3 := strconv.ParseFloat(f[4], 64)
		if err1 == nil && err2 == nil && err3 == nil {
			np.staticPos = &staticPos{slot: f[1], x: x, y: y, z: z}
		}
	}
}

func (np *nativeProps) parseGalOSNMAUsage(r *Reply) {
	f := findState(r, "GalOSNMAUsage")
	if len(f) < 4 {
		return
	}
	np.osnma = &osnmaUsage{pvtLevel: f[1], measLevel: f[3]}
}

func (np *nativeProps) parseRTCMv3Formatting(r *Reply) {
	f := findState(r, "RTCMv3Formatting")
	if len(f) < 2 {
		return
	}
	if v, err := strconv.Atoi(f[1]); err == nil {
		np.rtcmBaseID = &v
	}
}

func (np *nativeProps) parseCalibCommonDelay(r *Reply) {
	f := findState(r, "CalibCommonDelay")
	if len(f) < 2 {
		return
	}
	if v, err := strconv.ParseFloat(f[1], 64); err == nil {
		np.cableDelayNs = &v
	}
}

// ppsIntervals maps the setPPSParameters Interval enum to the pulse period.
// "off" is absent: it maps to a zero-width (disabled) time pulse instead.
var ppsIntervals = map[string]time.Duration{
	"MHz10":   100 * time.Nanosecond,
	"MHz1":    time.Microsecond,
	"kHz100":  10 * time.Microsecond,
	"kHz10":   100 * time.Microsecond,
	"kHz5":    200 * time.Microsecond,
	"kHz2":    500 * time.Microsecond,
	"kHz1":    time.Millisecond,
	"msec10":  10 * time.Millisecond,
	"msec20":  20 * time.Millisecond,
	"msec50":  50 * time.Millisecond,
	"msec100": 100 * time.Millisecond,
	"msec200": 200 * time.Millisecond,
	"msec250": 250 * time.Millisecond,
	"msec500": 500 * time.Millisecond,
	"sec1":    time.Second,
	"sec2":    2 * time.Second,
	"sec4":    4 * time.Second,
	"sec5":    5 * time.Second,
	"sec10":   10 * time.Second,
	"sec30":   30 * time.Second,
	"sec60":   60 * time.Second,
}

// timeScaleGNSS maps the TimeScale enum to device-independent GNSS values;
// UTC and RxClock have no analogue.
var timeScaleGNSS = map[string]gpsprot.GNSS{
	"GPS":     gpsprot.GPS,
	"Galileo": gpsprot.GAL,
	"BeiDou":  gpsprot.BDS,
	"GLONASS": gpsprot.GLO,
}

// convertToProps fills props from the native values read back so far.
func (np *nativeProps) convertToProps(props *gpsprot.ConfigProps) {
	if np.elevMaskPVT != nil {
		props.SetMinElevation(gpsprot.Angle(*np.elevMaskPVT) * gpsprot.Degrees)
	}
	if np.pps != nil {
		np.convertTimePulse(props)
	}
	if np.haveTracking {
		props.SetSignalsEnabled(signalSetFromNames(np.tracking))
	}
	if np.pvtMode != nil {
		np.convertMode(props)
	}
	if np.osnma != nil {
		auth := gpsprot.NavMsgAuthNone
		if np.osnma.pvtLevel != "off" || np.osnma.measLevel != "off" {
			auth = gpsprot.NavMsgAuthOSNMA
		}
		props.SetNavMsgAuth(auth)
	}
	if np.rtcmBaseID != nil {
		props.SetRTCMBaseID(uint16(*np.rtcmBaseID))
	}
	if np.cableDelayNs != nil {
		props.SetAntennaCableDelay(time.Duration(*np.cableDelayNs * float64(time.Nanosecond)))
	}
}

func (np *nativeProps) convertTimePulse(props *gpsprot.ConfigProps) {
	pps := np.pps
	tp := gpsprot.TimePulse{
		AlignToGNSS:    pps.timeScale != "RxClock",
		OnlyWhenLocked: pps.maxHoldover != 0,
		PolarityRising: pps.polarity == "Low2High",
	}
	if pps.interval != "off" {
		period, ok := ppsIntervals[pps.interval]
		if !ok {
			return // unknown enum value; report nothing rather than guess
		}
		tp.Period = period
		tp.Width = time.Duration(pps.widthMs * float64(time.Millisecond))
	}
	props.SetTimePulse(tp)
	if g, ok := timeScaleGNSS[pps.timeScale]; ok {
		props.SetTimeGNSS(g)
	}
}

func (np *nativeProps) convertMode(props *gpsprot.ConfigProps) {
	pm := np.pvtMode
	m := gpsprot.Mode{Static: pm.mode == "Static"}
	if m.Static && pm.staticRef != "auto" && pm.staticRef != "" {
		sp := np.staticPos
		if sp == nil || sp.slot != pm.staticRef {
			return // referenced slot not read back; report nothing rather than guess
		}
		if sp.geodetic {
			m.PosType = gpsprot.PosTypeLLH
			m.FixedPosLLH = [2]gpsprot.Angle{gpsprot.DegreesFromFloat(sp.lat), gpsprot.DegreesFromFloat(sp.lon)}
			m.Height = gpsprot.Meters(sp.height)
		} else {
			m.PosType = gpsprot.PosTypeECEF
			m.FixedPosECEF = gpsprot.Point3D{gpsprot.Meters(sp.x), gpsprot.Meters(sp.y), gpsprot.Meters(sp.z)}
		}
	}
	props.SetMode(m)
}

// addReq appends a command request whose reply is parsed by onReply.
func (c *Configurator) addReq(cmd string, onReply func(*Reply)) {
	c.append(&sReq{cmd: cmd, onReply: onReply})
}

// generateQueryReqs generates the get requests for the properties the target
// reads or sets (sets need the current state for read-modify-write).
func (c *Configurator) generateQueryReqs() {
	t, np := c.target, &c.np
	if t.UsesAny(gpsprot.PropIDSignalsEnabled) {
		c.addReq("getSignalTracking", np.parseSignalTracking)
		c.addReq("getSignalUsage", np.parseSignalUsage)
	}
	if t.UsesAny(gpsprot.PropIDTimePulse | gpsprot.PropIDTimeGNSS) {
		c.addReq("getPPSParameters", np.parsePPSParameters)
	}
	if t.UsesAny(gpsprot.PropIDMode) {
		c.addReq("getPVTMode", np.parsePVTMode)
	}
	if t.UsesAny(gpsprot.PropIDMinElevation) {
		c.addReq("getElevationMask, PVT", np.parseElevationMask)
	}
	if t.UsesAny(gpsprot.PropIDNavMsgAuth) && c.caps.caps["GalOSNMA"] {
		c.addReq("getGalOSNMAUsage", np.parseGalOSNMAUsage)
	}
	if t.UsesAny(gpsprot.PropIDRTCMBaseID) {
		c.addReq("getRTCMv3Formatting", np.parseRTCMv3Formatting)
	}
	if t.UsesAny(gpsprot.PropIDAntennaCableDelay) {
		c.addReq("getCalibCommonDelay", np.parseCalibCommonDelay)
	}
}

// generateFollowupQueries adds the static-position query once the PVTMode
// reply names a position slot, and reports whether it added a request.
func (c *Configurator) generateFollowupQueries() bool {
	pm := c.np.pvtMode
	if pm == nil || c.staticQueried {
		return false
	}
	ref := pm.staticRef
	if pm.mode != "Static" || ref == "auto" || ref == "" {
		return false
	}
	c.staticQueried = true
	cmd := "getStaticPosGeodetic, " + ref
	if strings.HasPrefix(ref, "Cartesian") {
		cmd = "getStaticPosCartesian, " + ref
	}
	c.addReq(cmd, c.np.parseStaticPos)
	return true
}
