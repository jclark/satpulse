package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// This file builds the configuration form items and implements the
// populate (read) and apply flows.

func (v *configView) buildItems() {
	add := func(it cfgItem) { v.items = append(v.items, it) }
	conn := v.connected
	add(itemHeading("Satellites and signals"))
	for _, name := range v.catalogOrder {
		add(v.gnssCheck(name))
	}
	if len(v.catalogOrder) > 0 {
		add(itemButton(func() string { return "Edit signals..." }, conn, func() tea.Cmd {
			v.openPicker()
			return nil
		}))
	}
	add(itemText("Min elevation (deg)", &v.minElev, conn, func() { v.minElevTouched = true }))
	add(itemSpacer())
	add(itemHeading("Time pulse"))
	tpTouch := func() { v.timePulseTouched = true }
	add(itemText("Period (s)", &v.ppsPeriod, conn, tpTouch))
	add(itemText("Pulse width (s)", &v.ppsWidth, conn, tpTouch))
	add(itemRadio("Time GNSS", []string{"--", "GPS", "Galileo", "BeiDou", "GLONASS"}, &v.timeGNSS, conn, nil, tpTouch))
	add(itemText("Cable delay (ns)", &v.cableDelay, conn, tpTouch))
	add(itemCheck("Align to GNSS", &v.alignToGNSS, conn, tpTouch))
	add(itemCheck("Only when locked", &v.onlyWhenLocked, conn, tpTouch))
	add(itemCheck("Rising edge", &v.risingEdge, conn, tpTouch))
	add(itemSpacer())
	add(itemHeading("Time mode"))
	add(itemRadio("Mode", []string{"--", "Mobile", "Survey-in", "Fixed position"}, &v.timeMode, conn,
		func(i int) bool {
			switch i {
			case 2:
				return v.has(gpsprot.ConfigSupportSurvey)
			case 3:
				return v.has(gpsprot.ConfigSupportFixedPos)
			}
			return true
		},
		func() { v.timeModeTouched = true; v.staticNoPos = false }))
	add(itemInfo(func() string {
		if v.staticNoPos && v.timeMode == 0 {
			return "  Receiver is in stationary mode."
		}
		return ""
	}))
	surveyOn := func() bool { return conn() && v.has(gpsprot.ConfigSupportSurvey) && v.timeMode == 2 }
	add(itemText("Survey time (s)", &v.surveyTime, surveyOn, nil))
	add(itemText("Survey accuracy (m)", &v.surveyAcc,
		func() bool { return surveyOn() && v.has(gpsprot.ConfigSupportSurveyAcc) }, nil))
	add(itemCheck("Do a new survey", &v.surveyAgain, surveyOn, nil))
	add(itemCheck("Report survey progress", &v.surveyReport,
		func() bool { return surveyOn() && v.has(gpsprot.ConfigSupportSurveyMsg) }, nil))
	fixedOn := func() bool { return conn() && v.has(gpsprot.ConfigSupportFixedPos) && v.timeMode == 3 }
	add(itemRadio("Coordinates", []string{"ECEF", "Lat/Lon/Height"}, &v.coordSystem, fixedOn, nil, nil))
	ecefOn := func() bool { return fixedOn() && v.coordSystem == 0 }
	llhOn := func() bool { return fixedOn() && v.coordSystem == 1 }
	add(itemText("X (m)", &v.ecefX, ecefOn, nil))
	add(itemText("Y (m)", &v.ecefY, ecefOn, nil))
	add(itemText("Z (m)", &v.ecefZ, ecefOn, nil))
	add(itemText("Latitude (deg)", &v.llhLat, llhOn, nil))
	add(itemText("Longitude (deg)", &v.llhLon, llhOn, nil))
	add(itemText("Height (m)", &v.llhHeight, llhOn, nil))
	add(itemText("Position accuracy (m)", &v.fixedAcc,
		func() bool { return fixedOn() && v.has(gpsprot.ConfigSupportFixedPosAcc) }, nil))
	add(itemSpacer())
	add(itemHeading("Messages"))
	add(itemButton(func() string { return "Preset: Minimum" }, conn, func() tea.Cmd { v.presetMinimum(); return nil }))
	add(itemButton(func() string { return "Preset: NTP" }, conn, func() tea.Cmd { v.presetTiming(false); return nil }))
	add(itemButton(func() string { return "Preset: PTP" }, conn, func() tea.Cmd { v.presetTiming(true); return nil }))
	add(itemCheck("NMEA: change", &v.nmeaChange, conn, nil))
	nmeaOn := func() bool { return conn() && v.nmeaChange }
	add(itemCheck("  Disable protocol", &v.nmeaDisable, nmeaOn, nil))
	nmeaFlagOn := func() bool { return nmeaOn() && !v.nmeaDisable }
	for _, f := range nmeaFlagOrder {
		add(itemCheck("  "+f, v.nmeaFlags[f], nmeaFlagOn, nil))
	}
	rtcmSupported := func() bool { return v.has(gpsprot.ConfigSupportRTCMMSM4) || v.has(gpsprot.ConfigSupportRTCMMSM7) }
	add(itemCheck("RTCM: change", &v.rtcmChange, func() bool { return conn() && rtcmSupported() }, nil))
	rtcmOn := func() bool { return conn() && v.rtcmChange }
	add(itemCheck("  Disable protocol", &v.rtcmDisable, rtcmOn, nil))
	rtcmMsgOn := func() bool { return rtcmOn() && !v.rtcmDisable }
	add(itemRadio("  MSM", []string{"No MSM", "MSM4", "MSM7"}, &v.msm, rtcmMsgOn,
		func(i int) bool {
			switch i {
			case 1:
				return v.has(gpsprot.ConfigSupportRTCMMSM4)
			case 2:
				return v.has(gpsprot.ConfigSupportRTCMMSM7)
			}
			return true
		}, nil))
	add(itemCheck("  Allow MSM fallback", &v.rtcmLax, func() bool { return rtcmMsgOn() && v.msm != 0 }, nil))
	add(itemCheck("  Antenna reference point (ARP)", &v.rtcmARP, rtcmMsgOn, nil))
	add(itemCheck("PVT: change", &v.pvtChange, conn, nil))
	pvtOn := func() bool { return conn() && v.pvtChange }
	pvt := func(label, flag string, on func() bool) cfgItem {
		if on == nil {
			on = pvtOn
		}
		return itemCheck("  "+label, v.pvtFlags[flag], on, nil)
	}
	add(pvt("Turn off unselected", "off", nil))
	add(pvt("Navigation time", "time", nil))
	add(pvt("Prefer TAI", "tai", func() bool {
		return pvtOn() && (*v.pvtFlags["time"] || *v.pvtFlags["timePulse"])
	}))
	add(pvt("Time-pulse time", "timePulse", nil))
	add(pvt("Ensure message after pulse", "timePulseAfter", func() bool {
		return pvtOn() && *v.pvtFlags["timePulse"]
	}))
	add(pvt("Leap second", "leapSecond", nil))
	add(pvt("Position", "pos", nil))
	add(pvt("Prefer ECEF", "ecef", func() bool {
		return pvtOn() && (*v.pvtFlags["pos"] || *v.pvtFlags["vel"])
	}))
	add(pvt("Velocity", "vel", nil))
	add(pvt("Solution quality", "quality", nil))
	add(pvt("End of epoch", "epoch", nil))
	add(itemCheck("Satellites: change", &v.satsChange, conn, nil))
	satsOn := func() bool { return conn() && v.satsChange }
	add(itemCheck("  Satellite positions", &v.satsSat, satsOn, nil))
	add(itemCheck("  Signals", &v.satsSignal, satsOn, nil))
	add(itemCheck("Raw: change", &v.rawChange, func() bool { return conn() && v.has(gpsprot.ConfigSupportRaw) }, nil))
	rawOn := func() bool { return conn() && v.rawChange }
	add(itemCheck("  Observations (RINEX .obs)", &v.rawObs, rawOn, nil))
	add(itemCheck("  Navigation data (RINEX .nav)", &v.rawNav, rawOn, nil))
	add(itemSpacer())
	add(itemHeading("Serial speed"))
	add(itemInfo(func() string {
		if v.portNotUART {
			return "  Current port is not a UART: baud rate not applicable"
		}
		return ""
	}))
	speedLabels := make([]string, len(speeds))
	for i, s := range speeds {
		speedLabels[i] = strconv.Itoa(s)
	}
	add(itemRadio("Baud rate", speedLabels, &v.speedIdx,
		func() bool { return conn() && v.has(gpsprot.ConfigSupportSpeed) && !v.portNotUART },
		nil, func() { v.speedTouched = true }))
	add(itemSpacer())
	add(itemHeading("Persistent operations"))
	add(itemRadio("Save", []string{"Nothing", "Changes", "All"}, &v.saveType, conn, nil, nil))
	add(itemRadio("Reset", []string{"None", "Reload", "Cold start", "Factory reset"}, &v.resetType, conn, nil, nil))
	add(itemSpacer())
	add(itemButton(func() string {
		if v.reading {
			return "Reading..."
		}
		return "Read"
	}, func() bool { return conn() && !v.reading }, v.readCmd))
	add(itemButton(func() string {
		if v.applying {
			return "Applying..."
		}
		return "Apply"
	}, func() bool { return conn() && !v.applying && v.pendingLabel() != "" }, v.applyCmd))
	add(itemButton(func() string { return "Discard" },
		func() bool { return conn() && v.pendingLabel() != "" }, func() tea.Cmd {
			v.discard()
			return nil
		}))
}

// gnssCheck is the per-constellation checkbox: checked when any of
// the constellation's signals is selected; toggling selects or clears
// every supported signal.
func (v *configView) gnssCheck(name string) cfgItem {
	enabled := func() bool { return v.connected() && v.gnssSupported(name) }
	selected := func() bool {
		for _, sig := range v.catalog[name] {
			if v.selSignals[name+":"+sig] {
				return true
			}
		}
		return false
	}
	return cfgItem{
		enabled: enabled,
		render: func(focused bool) string {
			box := "[ ]"
			if selected() {
				box = "[x]"
			}
			return marker(focused) + itemStyle(enabled, box+" "+name)
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			if k.String() != "space" && k.String() != "enter" {
				return nil
			}
			on := !selected()
			for _, sig := range v.catalog[name] {
				if !v.signalSupported(name, sig) {
					continue
				}
				if on {
					v.selSignals[name+":"+sig] = true
				} else {
					delete(v.selSignals, name+":"+sig)
				}
			}
			v.signalsTouched = true
			return nil
		},
	}
}

// Presets, mirroring the workbench Messages preset buttons.

func (v *configView) presetMinimum() {
	v.nmeaChange, v.nmeaDisable = true, false
	for f, p := range v.nmeaFlags {
		*p = f == "RMC"
	}
	if v.has(gpsprot.ConfigSupportRTCMMSM4) || v.has(gpsprot.ConfigSupportRTCMMSM7) {
		v.rtcmChange, v.rtcmDisable = true, true
	}
	v.pvtChange = true
	for f, p := range v.pvtFlags {
		*p = f == "off"
	}
	v.satsChange = true
	v.satsSat, v.satsSignal = false, false
	if v.has(gpsprot.ConfigSupportRaw) {
		v.rawChange = true
		v.rawObs, v.rawNav = false, false
	}
}

func (v *configView) presetTiming(ptp bool) {
	v.nmeaChange, v.nmeaDisable = true, true
	v.pvtChange = true
	flags := map[string]bool{"time": true, "leapSecond": true, "off": true, "quality": true, "epoch": true, "pos": true}
	if ptp {
		flags = map[string]bool{"timePulse": true, "timePulseAfter": true, "tai": true,
			"leapSecond": true, "off": true, "quality": true, "epoch": true, "pos": true}
	}
	for f, p := range v.pvtFlags {
		*p = flags[f]
	}
	if v.speed >= 38400 {
		v.satsChange = true
		v.satsSat, v.satsSignal = true, true
	}
}

// openPicker opens the signal-selection sub-form on a working copy of
// the selection.
func (v *configView) openPicker() {
	v.pickerSel = make(map[string]bool)
	for k, on := range v.selSignals {
		if on {
			v.pickerSel[k] = true
		}
	}
	var items []cfgItem
	items = append(items, itemHeading("Select GNSS signals"))
	for _, name := range v.catalogOrder {
		items = append(items, v.pickerGNSS(name))
		for _, sig := range v.catalog[name] {
			items = append(items, v.pickerSignal(name, sig))
		}
	}
	items = append(items, itemSpacer(),
		itemButton(func() string { return "OK" }, nil, func() tea.Cmd {
			v.selSignals = v.pickerSel
			v.signalsTouched = true
			v.picker = nil
			return nil
		}),
		itemButton(func() string { return "Cancel" }, nil, func() tea.Cmd {
			v.picker = nil
			return nil
		}))
	v.picker = items
	v.pickerFocus = 0
	v.offset = 0
	moveItemFocus(v.picker, &v.pickerFocus, 1)
}

func (v *configView) pickerGNSS(name string) cfgItem {
	enabled := func() bool { return v.gnssSupported(name) }
	all := func() bool {
		for _, sig := range v.catalog[name] {
			if v.signalSupported(name, sig) && !v.pickerSel[name+":"+sig] {
				return false
			}
		}
		return true
	}
	return cfgItem{
		enabled: enabled,
		render: func(focused bool) string {
			box := "[ ]"
			if all() {
				box = "[x]"
			}
			return marker(focused) + itemStyle(enabled, headerStyle.Render(box+" "+name))
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			if k.String() != "space" && k.String() != "enter" {
				return nil
			}
			on := !all()
			for _, sig := range v.catalog[name] {
				if !v.signalSupported(name, sig) {
					continue
				}
				if on {
					v.pickerSel[name+":"+sig] = true
				} else {
					delete(v.pickerSel, name+":"+sig)
				}
			}
			return nil
		},
	}
}

func (v *configView) pickerSignal(gnss, sig string) cfgItem {
	key := gnss + ":" + sig
	enabled := func() bool { return v.gnssSupported(gnss) && v.signalSupported(gnss, sig) }
	return cfgItem{
		enabled: enabled,
		render: func(focused bool) string {
			box := "[ ]"
			if v.pickerSel[key] {
				box = "[x]"
			}
			return marker(focused) + "  " + itemStyle(enabled, box+" "+sig)
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			if k.String() == "space" || k.String() == "enter" {
				if v.pickerSel[key] {
					delete(v.pickerSel, key)
				} else {
					v.pickerSel[key] = true
				}
			}
			return nil
		},
	}
}

// populate fills the form from a configuration readback, then clears
// the touched flags, as the workbench populateFromConfig does.
func (v *configView) populate(props *gpsprot.ConfigProps) {
	v.lastRead = props
	v.resetForm()
	if props == nil {
		return
	}
	if mode, ok := props.GetMode(); ok {
		if !mode.Static {
			v.timeMode = 1
		} else {
			switch mode.PosType {
			case gpsprot.PosTypeECEF:
				v.timeMode = 3
				v.coordSystem = 0
				v.ecefX.SetValue(formatFloat(mode.FixedPosECEF[0].Meters()))
				v.ecefY.SetValue(formatFloat(mode.FixedPosECEF[1].Meters()))
				v.ecefZ.SetValue(formatFloat(mode.FixedPosECEF[2].Meters()))
			case gpsprot.PosTypeLLH:
				v.timeMode = 3
				v.coordSystem = 1
				v.llhLat.SetValue(formatFloat(mode.FixedPosLLH[0].Degrees()))
				v.llhLon.SetValue(formatFloat(mode.FixedPosLLH[1].Degrees()))
				v.llhHeight.SetValue(formatFloat(mode.Height.Meters()))
			default:
				// Static with no stored position: shown as an
				// informational line, no mode preselected.
				v.timeMode = 0
				v.staticNoPos = true
			}
			if mode.FixedPosAcc != 0 {
				v.fixedAcc.SetValue(formatFloat(mode.FixedPosAcc.Meters()))
			}
		}
	}
	if p, ok := props.GetTimePulsePeriod(); ok {
		v.ppsPeriod.SetValue(formatFloat(p.Seconds()))
	}
	if w, ok := props.GetTimePulseWidth(); ok {
		v.ppsWidth.SetValue(formatFloat(w.Seconds()))
	}
	if b, ok := props.GetTimePulseAlignToGNSS(); ok {
		v.alignToGNSS = b
	}
	if b, ok := props.GetTimePulseOnlyWhenLocked(); ok {
		v.onlyWhenLocked = b
	}
	if b, ok := props.GetTimePulsePolarityRising(); ok {
		v.risingEdge = b
	}
	if g, ok := props.GetTimeGNSS(); ok {
		switch g {
		case gpsprot.GPS:
			v.timeGNSS = 1
		case gpsprot.GAL:
			v.timeGNSS = 2
		case gpsprot.BDS:
			v.timeGNSS = 3
		case gpsprot.GLO:
			v.timeGNSS = 4
		}
	}
	if d, ok := props.GetAntennaCableDelay(); ok {
		v.cableDelay.SetValue(strconv.FormatInt(d.Nanoseconds(), 10))
	}
	if a, ok := props.GetMinElevation(); ok {
		v.minElev.SetValue(formatFloat(a.Degrees()))
	}
	if b, ok := props.GetBaudRate(); ok {
		if b == 0 {
			v.portNotUART = true
		} else {
			v.speedIdx = speedIndex(int(b))
		}
	}
	if ss, ok := props.GetSignalsEnabled(); ok {
		for gnss, sigs := range ss.GNSSSignalMap() {
			for _, sig := range sigs {
				v.selSignals[gnss+":"+sig] = true
			}
		}
	}
	if port, ok := props.GetPort(); ok && v.onPort != nil {
		v.onPort(port)
	}
	v.clearTouched()
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// applyCmd validates the touched fields, builds the ConfigTarget the
// workbench Apply builds, and runs ApplyConfig.
func (v *configView) applyCmd() tea.Cmd {
	target, err := v.buildTarget()
	if err != nil {
		v.errMsg = err.Error()
		return nil
	}
	v.applying = true
	v.applied = false
	v.errMsg = ""
	v.saveType = int(gpsprot.SaveNone)
	v.resetType = int(gpsprot.ResetNone)
	sess := v.sess
	return func() tea.Msg {
		return configAppliedMsg{err: sess.ApplyConfig(context.Background(), target)}
	}
}

func (v *configView) buildTarget() (*gpsprot.ConfigTarget, error) {
	target := gpsprot.NewConfigTarget()
	parse := func(ti *textinput.Model, what string, minOK func(f float64) bool) (float64, bool, error) {
		s := strings.TrimSpace(ti.Value())
		if s == "" {
			return 0, false, nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || (minOK != nil && !minOK(f)) {
			return 0, false, fmt.Errorf("invalid %s", what)
		}
		return f, true, nil
	}
	if v.signalsTouched {
		m := make(map[string][]string)
		for key, on := range v.selSignals {
			if !on {
				continue
			}
			gnss, sig, _ := strings.Cut(key, ":")
			m[gnss] = append(m[gnss], sig)
		}
		ss, err := gpsprot.ParseSignalMap(m)
		if err != nil {
			return nil, err
		}
		target.Props.SetSignalsEnabled(ss)
	}
	if v.minElevTouched {
		if f, ok, err := parse(&v.minElev, "min elevation", func(f float64) bool { return f >= 0 && f <= 90 }); err != nil {
			return nil, err
		} else if ok {
			target.Props.SetMinElevation(gpsprot.DegreesFromFloat(f))
		}
	}
	if v.timePulseTouched {
		period, hasPeriod, err := parse(&v.ppsPeriod, "period", func(f float64) bool { return f >= 0 })
		if err != nil {
			return nil, err
		}
		width, hasWidth, err := parse(&v.ppsWidth, "pulse width", func(f float64) bool { return f >= 0 && f <= 1 })
		if err != nil {
			return nil, err
		}
		if hasPeriod && hasWidth && width >= period && period > 0 {
			return nil, fmt.Errorf("pulse width must be less than the period")
		}
		target.Props.SetTimePulse(gpsprot.TimePulse{
			Period:         time.Duration(period * float64(time.Second)),
			Width:          time.Duration(width * float64(time.Second)),
			AlignToGNSS:    v.alignToGNSS,
			OnlyWhenLocked: v.onlyWhenLocked,
			PolarityRising: v.risingEdge,
		})
		if v.timeGNSS != 0 {
			g := []gpsprot.GNSS{0, gpsprot.GPS, gpsprot.GAL, gpsprot.BDS, gpsprot.GLO}[v.timeGNSS]
			target.Props.SetTimeGNSS(g)
		}
		if d, ok, err := parse(&v.cableDelay, "cable delay", nil); err != nil {
			return nil, err
		} else if ok {
			target.Props.SetAntennaCableDelay(time.Duration(d) * time.Nanosecond)
		}
	}
	if v.timeModeTouched {
		switch v.timeMode {
		case 1:
			target.Props.SetMode(gpsprot.Mode{Static: false})
		case 2:
			target.Props.SetMode(gpsprot.Mode{Static: true})
			dur, hasDur, err := parse(&v.surveyTime, "survey time", func(f float64) bool { return f > 0 })
			if err != nil {
				return nil, err
			}
			if !hasDur {
				dur = 2000
			}
			acc, hasAcc, err := parse(&v.surveyAcc, "survey accuracy", func(f float64) bool { return f >= 0.001 })
			if err != nil {
				return nil, err
			}
			if !hasAcc {
				acc = 20
			}
			var flags gpsprot.SurveyFlags
			if v.surveyAgain {
				flags |= gpsprot.SurveyAgain
			}
			target.Opts.Survey = gpsprot.Survey{
				Flags:    flags,
				MinDur:   time.Duration(dur * float64(time.Second)),
				AccLimit: gpsprot.Meters(acc),
			}
			if v.surveyReport {
				target.Opts.PVTMsg |= gpsprot.PVTMsgSurvey
			}
		case 3:
			acc, hasAcc, err := parse(&v.fixedAcc, "position accuracy", func(f float64) bool { return f >= 0.001 })
			if err != nil {
				return nil, err
			}
			if !hasAcc {
				acc = 20
			}
			mode := gpsprot.Mode{Static: true, FixedPosAcc: gpsprot.Meters(acc)}
			if v.coordSystem == 0 {
				var xyz [3]float64
				for i, ti := range []*textinput.Model{&v.ecefX, &v.ecefY, &v.ecefZ} {
					f, ok, err := parse(ti, "ECEF coordinate", nil)
					if err != nil || !ok {
						return nil, fmt.Errorf("invalid ECEF coordinates")
					}
					xyz[i] = f
				}
				mode.PosType = gpsprot.PosTypeECEF
				mode.FixedPosECEF = gpsprot.Point3D{gpsprot.Meters(xyz[0]), gpsprot.Meters(xyz[1]), gpsprot.Meters(xyz[2])}
			} else {
				lat, okLat, err1 := parse(&v.llhLat, "latitude", func(f float64) bool { return f >= -90 && f <= 90 })
				lon, okLon, err2 := parse(&v.llhLon, "longitude", func(f float64) bool { return f >= -180 && f <= 180 })
				h, okH, err3 := parse(&v.llhHeight, "height", nil)
				if err1 != nil || err2 != nil || err3 != nil || !okLat || !okLon || !okH {
					return nil, fmt.Errorf("invalid lat/lon/height")
				}
				mode.PosType = gpsprot.PosTypeLLH
				mode.FixedPosLLH = [2]gpsprot.Angle{gpsprot.DegreesFromFloat(lat), gpsprot.DegreesFromFloat(lon)}
				mode.Height = gpsprot.Meters(h)
			}
			target.Props.SetMode(mode)
		}
	}
	if v.nmeaChange {
		var flags gpsprot.NMEAMsgFlags
		if !v.nmeaDisable {
			bits := map[string]gpsprot.NMEAMsgFlags{
				"RMC": gpsprot.NMEAMsgRMC, "GGA": gpsprot.NMEAMsgGGA, "GSA": gpsprot.NMEAMsgGSA,
				"GSV": gpsprot.NMEAMsgGSV, "ZDA": gpsprot.NMEAMsgZDA, "VTG": gpsprot.NMEAMsgVTG,
				"GLL": gpsprot.NMEAMsgGLL,
			}
			for name, p := range v.nmeaFlags {
				if *p {
					flags |= bits[name]
				}
			}
			flags |= gpsprot.NMEAMsgOther
		}
		target.Opts.NMEAMsg.Set(flags)
	}
	if v.rtcmChange {
		var flags gpsprot.RTCMMsgFlags
		if !v.rtcmDisable {
			flags = gpsprot.RTCMMsgOther
			switch v.msm {
			case 1:
				flags |= gpsprot.RTCMMsgMSM4
			case 2:
				flags |= gpsprot.RTCMMsgMSM7
			}
			if v.rtcmLax && v.msm != 0 {
				flags |= gpsprot.RTCMMsgLax
			}
			if v.rtcmARP {
				flags |= gpsprot.RTCMMsgARP
			}
		}
		target.Opts.RTCMMsg.Set(flags)
	}
	if v.pvtChange {
		bits := map[string]gpsprot.PVTMsgFlags{
			"pos": gpsprot.PVTMsgPos, "vel": gpsprot.PVTMsgVel, "time": gpsprot.PVTMsgTime,
			"timePulse": gpsprot.PVTMsgTimePulse, "leapSecond": gpsprot.PVTMsgLeapSecond,
			"tai": gpsprot.PVTMsgTAI, "ecef": gpsprot.PVTMsgECEF,
			"timePulseAfter": gpsprot.PVTMsgTimePulseAfter, "quality": gpsprot.PVTMsgQuality,
			"epoch": gpsprot.PVTMsgEpoch, "off": gpsprot.PVTMsgOff,
		}
		for name, p := range v.pvtFlags {
			if *p {
				target.Opts.PVTMsg |= bits[name]
			}
		}
	}
	if v.satsChange {
		var flags gpsprot.SatsMsgFlags
		if v.satsSat {
			flags |= gpsprot.SatsMsgSat
		}
		if v.satsSignal {
			flags |= gpsprot.SatsMsgSignal
		}
		target.Opts.SatsMsg.Set(flags)
	}
	if v.rawChange {
		var flags gpsprot.RawMsgFlags
		if v.rawObs {
			flags |= gpsprot.RawMsgObs
		}
		if v.rawNav {
			flags |= gpsprot.RawMsgNavData
		}
		target.Opts.RawMsg.Set(flags)
	}
	if v.speedTouched {
		target.Props.SetBaudRate(uint32(speeds[v.speedIdx]))
	}
	target.Opts.Save = gpsprot.SaveType(v.saveType)
	target.Opts.Reset = gpsprot.ResetType(v.resetType)
	return target, nil
}

// discard re-populates from the last readback and clears every
// touched flag.
func (v *configView) discard() {
	v.populate(v.lastRead)
}
