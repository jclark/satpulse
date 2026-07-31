package main

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/geopos"
	"github.com/jclark/satpulse/gps/lib/opt"
	"github.com/jclark/satpulse/gps/ptime"
)

// cn0Max is the full-scale CN0 for the signal bar chart, matching the
// workbench signals panel.
const cn0Max = 55

// posRow is one Position table row, keyed by native message ID.
type posRow struct {
	geo   *gpsprot.PosGeoMsg
	ecef  *gpsprot.PosECEFMsg
	epoch int
}

// velRow is one Velocity table row, keyed by native message ID.
type velRow struct {
	geo   *gpsprot.VelGeoMsg
	ecef  *gpsprot.VelECEFMsg
	epoch int
}

// timeRow is one Time table row, keyed by native message ID.
type timeRow struct {
	msg   *gpsprot.TimeMsg
	epoch int
}

// monitorView is the Monitor tab: a scrollable stack of sections
// mirroring the workbench monitor panels, minus map, sky view, and
// scatter plot.
type monitorView struct {
	sess *session.Session

	nav      *gpsprot.NavEpochMsg
	timeMsg  *gpsprot.TimeMsg
	leapOff  int16
	survey   *gpsprot.SurveyMsg
	sats     *gpsprot.SatellitesMsg
	bundle   gpsprot.PVMsgBundle
	pvtEpoch int
	posRows  map[string]*posRow
	velRows  map[string]*velRow
	timeRows map[string]*timeRow
	stats    posStats
	baseARPs map[uint16][3]float64

	offset     int // scroll offset in lines
	lastHeight int
}

func newMonitorView(sess *session.Session) *monitorView {
	m := &monitorView{sess: sess}
	m.clear()
	return m
}

func (m *monitorView) title() string { return "Monitor" }

func (m *monitorView) capturesInput() bool { return false }

func (m *monitorView) hints() []string {
	return []string{"up/down scroll"}
}

func (m *monitorView) clear() {
	m.nav = nil
	m.timeMsg = nil
	m.survey = nil
	m.sats = nil
	m.bundle = gpsprot.PVMsgBundle{}
	m.leapOff = 0
	m.pvtEpoch = 0
	m.posRows = make(map[string]*posRow)
	m.velRows = make(map[string]*velRow)
	m.timeRows = make(map[string]*timeRow)
	m.stats.reset()
	m.baseARPs = make(map[uint16][3]float64)
}

func (m *monitorView) handleEvent(ev session.Event) {
	switch ev.Name {
	case session.EventState:
		if st, ok := ev.Data.(session.ConnState); ok && st == session.StateDisconnected {
			m.clear()
		}
	case session.EventTime:
		if msg, ok := ev.Data.(*gpsprot.TimeMsg); ok {
			m.timeMsg = msg
		}
	case session.EventMsg:
		if me, ok := ev.Data.(session.MsgEvent); ok {
			m.handleMsg(me)
		}
	case session.EventEpochPVT:
		if b, ok := ev.Data.(gpsprot.PVMsgBundle); ok {
			m.handleEpoch(b)
		}
	case session.EventBaseARP:
		if arp, ok := ev.Data.(session.BaseARPEvent); ok {
			m.baseARPs[arp.StationID] = arp.ECEF
		}
	case session.EventCorrections:
		if ce, ok := ev.Data.(session.CorrEvent); ok && (ce.State == "connecting" || ce.State == "stopped") {
			m.baseARPs = make(map[uint16][3]float64)
		}
	}
}

func (m *monitorView) handleMsg(me session.MsgEvent) {
	switch me.Kind {
	case "navEpoch":
		if msg, ok := me.Msg.(*gpsprot.NavEpochMsg); ok {
			m.nav = msg
		}
	case "leapSecond":
		if msg, ok := me.Msg.(*gpsprot.LeapSecondMsg); ok && msg.UTCOffAfter != 0 {
			m.leapOff = msg.UTCOffAfter
		}
	case "survey":
		if msg, ok := me.Msg.(*gpsprot.SurveyMsg); ok {
			m.survey = msg
		}
	case "satellites":
		if msg, ok := me.Msg.(*gpsprot.SatellitesMsg); ok {
			m.sats = msg
		}
	case "time":
		if msg, ok := me.Msg.(*gpsprot.TimeMsg); ok && msg.NativeMsgID != "" {
			m.timeRows[msg.NativeMsgID] = &timeRow{msg: msg, epoch: m.pvtEpoch}
		}
	case "posGeo":
		if msg, ok := me.Msg.(*gpsprot.PosGeoMsg); ok && msg.NativeMsgID != "" {
			m.posRows[msg.NativeMsgID] = &posRow{geo: msg, epoch: m.pvtEpoch}
		}
	case "posECEF":
		if msg, ok := me.Msg.(*gpsprot.PosECEFMsg); ok && msg.NativeMsgID != "" {
			m.posRows[msg.NativeMsgID] = &posRow{ecef: msg, epoch: m.pvtEpoch}
		}
	case "velGeo":
		if msg, ok := me.Msg.(*gpsprot.VelGeoMsg); ok && msg.NativeMsgID != "" {
			m.velRows[msg.NativeMsgID] = &velRow{geo: msg, epoch: m.pvtEpoch}
		}
	case "velECEF":
		if msg, ok := me.Msg.(*gpsprot.VelECEFMsg); ok && msg.NativeMsgID != "" {
			m.velRows[msg.NativeMsgID] = &velRow{ecef: msg, epoch: m.pvtEpoch}
		}
	}
}

// handleEpoch advances the PVT epoch: accumulate the position fix for
// the statistics and evict rows that were not refreshed this epoch,
// as the workbench does per table.
func (m *monitorView) handleEpoch(b gpsprot.PVMsgBundle) {
	m.bundle = b
	m.pvtEpoch++
	if pe := b.PosECEF.Ptr(); pe != nil {
		m.stats.add(geopos.ECEF{pe.Pos[0].Meters(), pe.Pos[1].Meters(), pe.Pos[2].Meters()})
	}
	evict(m.posRows, func(r *posRow) int { return r.epoch })
	evict(m.velRows, func(r *velRow) int { return r.epoch })
	evict(m.timeRows, func(r *timeRow) int { return r.epoch })
}

// evict drops rows whose epoch is older than the newest epoch present
// in the table.
func evict[R any](rows map[string]*R, epoch func(*R) int) {
	max := 0
	for _, r := range rows {
		if epoch(r) > max {
			max = epoch(r)
		}
	}
	if max == 0 {
		return
	}
	maps.DeleteFunc(rows, func(_ string, r *R) bool { return epoch(r) < max })
}

func (m *monitorView) update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up":
			m.offset--
		case "down":
			m.offset++
		case "pgup":
			m.offset -= m.lastHeight
		case "pgdown":
			m.offset += m.lastHeight
		case "home":
			m.offset = 0
		}
	}
	return nil
}

func (m *monitorView) render(width, height int) string {
	m.lastHeight = height
	var lines []string
	add := func(ls ...string) { lines = append(lines, ls...) }
	add(sectionStyle.Render("Summary"))
	add(m.renderSummary()...)
	add("", sectionStyle.Render("Clock"))
	add(m.renderClock()...)
	add("", sectionStyle.Render("PVT messages"))
	add(m.renderPVT()...)
	add("", sectionStyle.Render("Satellite signals"))
	add(m.renderSignals(width)...)
	add("", sectionStyle.Render("Survey"))
	add(m.renderSurvey()...)
	add("", sectionStyle.Render("Position statistics"))
	add(m.renderStats()...)
	if m.offset > len(lines)-height {
		m.offset = len(lines) - height
	}
	if m.offset < 0 {
		m.offset = 0
	}
	end := min(m.offset+height, len(lines))
	out := make([]string, 0, end-m.offset)
	for _, l := range lines[m.offset:end] {
		out = append(out, truncate(l, width))
	}
	return strings.Join(out, "\n")
}

// renderSummary mirrors the workbench summary panel: fix, satellites,
// DOP, and accuracy lines built from the last NavEpochMsg.
func (m *monitorView) renderSummary() []string {
	if m.nav == nil {
		return []string{faintStyle.Render("Waiting for fix")}
	}
	nav := m.nav
	var lines []string
	var fix []string
	if nav.FixLevel != 0 {
		fix = append(fix, fixLevelDisplay(nav.FixLevel))
	}
	if nav.SolutionDim != 0 {
		fix = append(fix, solutionDimDisplay(nav.SolutionDim))
	}
	fix = append(fix, nav.AuxSrc.Items()...)
	for _, name := range nav.Correction.Leaves().Names() {
		fix = append(fix, corrDisplay(name))
	}
	if id := nav.RTCMRefBaseID.Ptr(); id != nil && *id != 0 {
		fix = append(fix, fmt.Sprintf("R%d", *id))
	}
	if age := nav.DiffAge.Ptr(); age != nil {
		fix = append(fix, fmt.Sprintf("%.0fs", age.Seconds()))
	}
	if len(fix) > 0 {
		lines = append(lines, kv(4, "Fix", strings.Join(fix, " ")))
	}
	var sats []string
	if used := nav.NumSVUsed.Ptr(); used != nil {
		if tracked := nav.NumSVTracked.Ptr(); tracked != nil {
			sats = append(sats, fmt.Sprintf("%d/%d", *used, *tracked))
		}
	}
	if !nav.GNSSUsed.IsZero() {
		var names []string
		for _, g := range nav.GNSSUsed.Items() {
			names = append(names, g.String())
		}
		sats = append(sats, strings.Join(names, " "))
	}
	if !nav.BandsUsed.IsZero() {
		sats = append(sats, strings.Join(nav.BandsUsed.Items(), " "))
	}
	if len(sats) > 0 {
		lines = append(lines, kv(4, "Sats", strings.Join(sats, "; ")))
	}
	var dop []string
	addDOP := func(label string, v opt.Val[float64]) {
		if f := v.Ptr(); f != nil {
			dop = append(dop, fmt.Sprintf("%s%.1f", label, *f))
		}
	}
	addDOP("G", nav.DOP.Geom)
	addDOP("P", nav.DOP.Pos)
	addDOP("T", nav.DOP.Time)
	addDOP("H", nav.DOP.Hor)
	addDOP("V", nav.DOP.Vert)
	addDOP("N", nav.DOP.North)
	addDOP("E", nav.DOP.East)
	if len(dop) > 0 {
		lines = append(lines, kv(4, "DOP", strings.Join(dop, " ")))
	}
	var acc []string
	addAcc := func(label string, v opt.Val[gpsprot.Length]) {
		if l := v.Ptr(); l != nil {
			acc = append(acc, fmt.Sprintf("%s%.3f", label, l.Meters()))
		}
	}
	addAcc("P", nav.Acc.Pos)
	addAcc("H", nav.Acc.Hor)
	addAcc("V", nav.Acc.Vert)
	if m.moving() {
		if s := nav.Acc.GroundSpeed.Ptr(); s != nil {
			acc = append(acc, fmt.Sprintf("GS%.3f", s.MetersPerSecond()))
		}
		if c := nav.Acc.Course.Ptr(); c != nil {
			acc = append(acc, fmt.Sprintf("C%.1f", c.Degrees()))
		}
	}
	if len(acc) > 0 {
		lines = append(lines, kv(4, "Acc", strings.Join(acc, " ")))
	}
	if len(lines) == 0 {
		return []string{faintStyle.Render("Waiting for fix")}
	}
	return lines
}

// moving reports whether the receiver is moving, per the workbench's
// 0.1 m/s ground speed threshold.
func (m *monitorView) moving() bool {
	vg := m.bundle.VelGeo.Ptr()
	if vg == nil {
		return false
	}
	gs := vg.GroundSpeed.Ptr()
	return gs != nil && gs.MetersPerSecond() >= 0.1
}

func fixLevelDisplay(f gpsprot.FixLevel) string {
	switch f {
	case gpsprot.FixLevelNotMeasured:
		return "not-measured"
	case gpsprot.FixLevelCarrierFloat:
		return "float"
	case gpsprot.FixLevelCarrierFixed:
		return "fixed"
	}
	return f.String()
}

func solutionDimDisplay(d gpsprot.SolutionDim) string {
	if d == gpsprot.SolutionDimTimeOnly {
		return "time-only"
	}
	return d.String()
}

func corrDisplay(name string) string {
	switch name {
	case "used":
		return "corr"
	case "partialDualFreq":
		return "dual-freq (coarse)"
	case "fullDualFreq":
		return "dual-freq (precise)"
	case "PPPConverging":
		return "PPP converging"
	case "PPPConverged":
		return "PPP converged"
	}
	return name
}

// renderClock shows the receiver time from the last gps:time event:
// local and UTC wall time, TAI, leap seconds, source, and accuracy.
func (m *monitorView) renderClock() []string {
	msg := m.timeMsg
	if msg == nil {
		return []string{faintStyle.Render("Waiting for time")}
	}
	const lw = 12
	var lines []string
	if ut := msg.UTCTime.Ptr(); ut != nil {
		lines = append(lines,
			kv(lw, "Local", ut.SysTime().Local().Format("2006-01-02 15:04:05")),
			kv(lw, "UTC", formatUTC(*ut)))
	}
	if !msg.TAITime.IsZero() {
		lines = append(lines, kv(lw, "TAI", formatTAI(msg.TAITime)))
	}
	if off := m.utcOffset(msg); off != 0 {
		lines = append(lines, kv(lw, "Leap seconds", fmt.Sprintf("%d", off)))
	}
	if msg.Accuracy != 0 {
		lines = append(lines, kv(lw, "Accuracy", fmtNs(msg.Accuracy)))
	}
	if msg.GNSS != 0 {
		lines = append(lines, kv(lw, "Time source", msg.GNSS.String()))
	}
	return lines
}

// utcOffset returns the TAI-UTC offset for a time message: the
// message's own when present, else the last announced one.
func (m *monitorView) utcOffset(msg *gpsprot.TimeMsg) int16 {
	if msg != nil && msg.UTCOffset != 0 {
		return int16(msg.UTCOffset)
	}
	return m.leapOff
}

// formatUTC formats a UTCTime as "YYYY-MM-DD HH:MM:SS", preserving a
// leap second's :60 by deriving the fields from the time of day.
func formatUTC(ut ptime.UTCTime) string {
	tod := ut.TimeOfDay
	h := int(tod / time.Hour)
	tod -= time.Duration(h) * time.Hour
	mi := int(tod / time.Minute)
	tod -= time.Duration(mi) * time.Minute
	s := int(tod / time.Second)
	return fmt.Sprintf("%s %02d:%02d:%02d", ut.Date.Format("2006-01-02"), h, mi, s)
}

// formatTAI formats a TAI time by displaying its seconds-since-epoch
// on the calendar, matching the workbench.
func formatTAI(t ptime.Time) string {
	return time.Unix(0, int64(t)).UTC().Format("2006-01-02 15:04:05")
}

// fmtNs formats a time accuracy with the workbench's unit thresholds.
func fmtNs(d time.Duration) string {
	ns := d.Nanoseconds()
	switch {
	case ns >= 1e9:
		return fmt.Sprintf("%.1f s", float64(ns)/1e9)
	case ns >= 1e6:
		return fmt.Sprintf("%.1f ms", float64(ns)/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.1f us", float64(ns)/1e3)
	}
	return fmt.Sprintf("%d ns", ns)
}

// renderPVT renders the Position, Velocity, and Time tables: one row
// per native message, receiver-reported values bold, derived values
// plain.
func (m *monitorView) renderPVT() []string {
	var lines []string
	lines = append(lines, labelStyle.Render("Position"))
	if len(m.posRows) == 0 {
		lines = append(lines, faintStyle.Render("No position data"))
	} else {
		lines = append(lines, renderTable(
			[]string{"Tag", "Message", "Latitude,Longitude", "Height", "Height MSL", "ECEF"},
			nil, m.posTableRows())...)
	}
	lines = append(lines, "", labelStyle.Render("Velocity"))
	if len(m.velRows) == 0 {
		lines = append(lines, faintStyle.Render("No velocity data"))
	} else {
		lines = append(lines, renderTable(
			[]string{"Tag", "Message", "Speed (m/s)", "Ground speed", "Course (deg)", "North,East,Down", "ECEF (m/s)"},
			nil, m.velTableRows())...)
	}
	lines = append(lines, "", labelStyle.Render("Time"))
	if len(m.timeRows) == 0 {
		lines = append(lines, faintStyle.Render("No time data"))
	} else {
		lines = append(lines, renderTable(
			[]string{"Tag", "Message", "UTC", "TAI", "Leap sec", "TAcc", "GNSS"},
			nil, m.timeTableRows())...)
	}
	return lines
}

func (m *monitorView) posTableRows() [][]cell {
	var rows [][]cell
	for _, id := range slices.Sorted(maps.Keys(m.posRows)) {
		r := m.posRows[id]
		if r.geo != nil {
			g := r.geo
			row := []cell{{text: string(g.Tag)}, {text: id},
				{text: fmt.Sprintf("%.7f,%.7f", g.LatLon[0].Degrees(), g.LatLon[1].Degrees()), bold: true}}
			ecef := cell{text: "-"}
			height := cell{text: "-"}
			if h := g.Height.Ptr(); h != nil {
				height = cell{text: fmt.Sprintf("%.4f", h.Meters()), bold: true}
				p := geopos.WGS84.LLHtoECEF(geopos.LLH{Lat: g.LatLon[0].Degrees(), Lon: g.LatLon[1].Degrees(), Height: h.Meters()})
				ecef = cell{text: fmt.Sprintf("%.4f,%.4f,%.4f", p[0], p[1], p[2])}
			}
			row = append(row, height)
			if h := g.HeightMSL.Ptr(); h != nil {
				row = append(row, cell{text: fmt.Sprintf("%.4f", h.Meters()), bold: true})
			} else {
				row = append(row, cell{text: "-"})
			}
			rows = append(rows, append(row, ecef))
		} else if r.ecef != nil {
			e := r.ecef
			p := geopos.ECEF{e.Pos[0].Meters(), e.Pos[1].Meters(), e.Pos[2].Meters()}
			latlon, height := cell{text: "-"}, cell{text: "-"}
			if llh, err := geopos.WGS84.ECEFtoLLH(p); err == nil {
				latlon = cell{text: fmt.Sprintf("%.7f,%.7f", llh.Lat, llh.Lon)}
				height = cell{text: fmt.Sprintf("%.4f", llh.Height)}
			}
			rows = append(rows, []cell{{text: string(e.Tag)}, {text: id}, latlon, height, {text: "-"},
				{text: fmt.Sprintf("%.4f,%.4f,%.4f", p[0], p[1], p[2]), bold: true}})
		}
	}
	return rows
}

func (m *monitorView) velTableRows() [][]cell {
	var rows [][]cell
	for _, id := range slices.Sorted(maps.Keys(m.velRows)) {
		r := m.velRows[id]
		if r.geo != nil {
			g := r.geo
			speed := cell{text: "-"}
			if s := g.Speed3D.Ptr(); s != nil {
				speed = cell{text: fmt.Sprintf("%.4f", s.MetersPerSecond()), bold: true}
			} else if ned := g.VelNED.Ptr(); ned != nil {
				n, e, d := ned[0].MetersPerSecond(), ned[1].MetersPerSecond(), ned[2].MetersPerSecond()
				speed = cell{text: fmt.Sprintf("%.4f", math.Sqrt(n*n+e*e+d*d))}
			}
			ground := cell{text: "-"}
			if s := g.GroundSpeed.Ptr(); s != nil {
				ground = cell{text: fmt.Sprintf("%.4f", s.MetersPerSecond()), bold: true}
			}
			course := cell{text: "-"}
			if c := g.Course.Ptr(); c != nil {
				course = cell{text: fmt.Sprintf("%.2f", c.Degrees()), bold: true}
			}
			nedCell, ecefCell := cell{text: "-"}, cell{text: "-"}
			if ned := g.VelNED.Ptr(); ned != nil {
				n, e, d := ned[0].MetersPerSecond(), ned[1].MetersPerSecond(), ned[2].MetersPerSecond()
				nedCell = cell{text: fmt.Sprintf("%.4f,%.4f,%.4f", n, e, d), bold: true}
				if v := m.sess.VelNEDtoECEF(n, e, d); v != nil {
					ecefCell = cell{text: fmt.Sprintf("%.4f,%.4f,%.4f", v[0], v[1], v[2])}
				}
			}
			rows = append(rows, []cell{{text: string(g.Tag)}, {text: id}, speed, ground, course, nedCell, ecefCell})
		} else if r.ecef != nil {
			e := r.ecef
			vx, vy, vz := e.Vel[0].MetersPerSecond(), e.Vel[1].MetersPerSecond(), e.Vel[2].MetersPerSecond()
			nedCell := cell{text: "-"}
			if v := m.sess.VelECEFtoNED(vx, vy, vz); v != nil {
				nedCell = cell{text: fmt.Sprintf("%.4f,%.4f,%.4f", v[0], v[1], v[2])}
			}
			rows = append(rows, []cell{{text: string(e.Tag)}, {text: id}, {text: "-"}, {text: "-"}, {text: "-"},
				nedCell, {text: fmt.Sprintf("%.4f,%.4f,%.4f", vx, vy, vz), bold: true}})
		}
	}
	return rows
}

func (m *monitorView) timeTableRows() [][]cell {
	var rows [][]cell
	for _, id := range slices.Sorted(maps.Keys(m.timeRows)) {
		msg := m.timeRows[id].msg
		utcCell, taiCell := cell{text: "-"}, cell{text: "-"}
		ls := m.utcOffset(msg)
		if ut := msg.UTCTime.Ptr(); ut != nil {
			utcCell = cell{text: formatUTC(*ut), bold: true}
			if msg.TAITime.IsZero() && ls > 0 {
				taiCell = cell{text: formatTAI(ptime.Unix(ut.SysTime().Unix()+int64(ls), 0))}
			}
		}
		if !msg.TAITime.IsZero() {
			taiCell = cell{text: formatTAI(msg.TAITime), bold: true}
			if !utcCell.bold && ls > 0 {
				utcCell = cell{text: formatTAI(msg.TAITime.Add(-time.Duration(ls) * time.Second))}
			}
		}
		leapCell := cell{text: "-"}
		if msg.UTCOffset != 0 {
			leapCell = cell{text: fmt.Sprintf("%d", msg.UTCOffset), bold: true}
		} else if m.leapOff != 0 {
			leapCell = cell{text: fmt.Sprintf("%d", m.leapOff)}
		}
		taccCell := cell{text: "-"}
		if msg.Accuracy != 0 {
			taccCell = cell{text: fmtNs(msg.Accuracy), bold: true}
		}
		gnssCell := cell{text: "-"}
		if msg.GNSS != 0 {
			gnssCell = cell{text: msg.GNSS.String(), bold: true}
		}
		rows = append(rows, []cell{{text: string(msg.Tag)}, {text: id}, utcCell, taiCell, leapCell, taccCell, gnssCell})
	}
	return rows
}

// renderSignals renders the satellite table with a text CN0 bar chart:
// one row per satellite signal (or one bare row for a satellite
// reporting no signal detail), sorted by constellation, SV number,
// and signal.
func (m *monitorView) renderSignals(width int) []string {
	if m.sats == nil || len(m.sats.SVs) == 0 {
		return []string{faintStyle.Render("No satellite data")}
	}
	showUsed := m.sats.UsedValidity != gpsprot.SatelliteUsedInvalid
	type sigRow struct {
		sv   *gpsprot.SVInfo
		sig  *gpsprot.SignalInfo
		used bool
	}
	var srows []sigRow
	for i := range m.sats.SVs {
		sv := &m.sats.SVs[i]
		if len(sv.Signals) == 0 {
			srows = append(srows, sigRow{sv: sv, used: sv.Used})
			continue
		}
		for j := range sv.Signals {
			sig := &sv.Signals[j]
			used := sv.Used
			if m.sats.UsedValidity == gpsprot.SatelliteUsedSignal {
				used = sig.Used
			}
			srows = append(srows, sigRow{sv: sv, sig: sig, used: used})
		}
	}
	slices.SortFunc(srows, func(a, b sigRow) int {
		if c := int(a.sv.ID.GNSS) - int(b.sv.ID.GNSS); c != 0 {
			return c
		}
		if c := int(a.sv.ID.Num) - int(b.sv.ID.Num); c != 0 {
			return c
		}
		var as, bs gpsprot.SignalID
		if a.sig != nil {
			as = a.sig.ID
		}
		if b.sig != nil {
			bs = b.sig.ID
		}
		return strings.Compare(string(as), string(bs))
	})
	headers := []string{"GNSS", "SV", "Signal", "Azim", "Elev", "CN0"}
	right := []bool{false, false, false, true, true, true}
	if showUsed {
		headers = slices.Insert(headers, 3, "Used")
		right = slices.Insert(right, 3, false)
	}
	var rows [][]cell
	var cn0s []int
	for _, r := range srows {
		row := []cell{
			{text: r.sv.ID.GNSS.String()},
			{text: fmt.Sprintf("%02d", r.sv.ID.Num)},
		}
		sigName, cn0 := "-", 0
		if r.sig != nil {
			if r.sig.ID != "" {
				sigName = string(r.sig.ID)
			}
			cn0 = int(r.sig.CN0)
		}
		row = append(row, cell{text: sigName})
		if showUsed {
			used := ""
			if r.used {
				used = "*"
			}
			row = append(row, cell{text: used})
		}
		az, el := "", ""
		if la := r.sv.LookAngles.Ptr(); la != nil {
			az = fmt.Sprintf("%d", la.Azimuth)
			el = fmt.Sprintf("%d", la.Elevation)
		}
		cn0Text := ""
		if cn0 > 0 {
			cn0Text = fmt.Sprintf("%d", cn0)
		}
		row = append(row, cell{text: az}, cell{text: el}, cell{text: cn0Text})
		rows = append(rows, row)
		cn0s = append(cn0s, cn0)
	}
	lines := renderTable(headers, right, rows)
	// Append the CN0 bar to each data line, sized to the space left.
	barWidth := width - lipgloss.Width(lines[0]) - 2
	if barWidth > 30 {
		barWidth = 30
	}
	if barWidth >= 4 {
		for i := range rows {
			lines[i+1] += "  " + bar(cn0s[i], cn0Max, barWidth)
		}
	}
	return lines
}

// renderSurvey mirrors the workbench survey panel.
func (m *monitorView) renderSurvey() []string {
	msg := m.survey
	if msg == nil {
		return []string{faintStyle.Render("No survey data")}
	}
	const lw = 16
	status := "-"
	if msg.Valid {
		status = "Valid"
	} else if msg.InProgress {
		status = "In progress"
	}
	lines := []string{kv(lw, "Status", status)}
	if msg.Accuracy != 0 {
		lines = append(lines, kv(lw, "Accuracy", fmt.Sprintf("%.4f m", msg.Accuracy.Meters())))
	}
	if !msg.Position.IsZero() {
		p := geopos.ECEF{msg.Position[0].Meters(), msg.Position[1].Meters(), msg.Position[2].Meters()}
		if llh, err := geopos.WGS84.ECEFtoLLH(p); err == nil {
			lines = append(lines,
				kv(lw, "Coordinates", fmt.Sprintf("%.5f,%.5f", llh.Lat, llh.Lon)),
				kv(lw, "Height", fmt.Sprintf("%.2f m", llh.Height)))
		}
		lines = append(lines, kv(lw, "ECEF", fmt.Sprintf("%.4f, %.4f, %.4f", p[0], p[1], p[2])))
	}
	if msg.ObsCount != 0 {
		lines = append(lines, kv(lw, "Observations", fmt.Sprintf("%d", msg.ObsCount)))
	}
	lines = append(lines, kv(lw, "Observation time", fmt.Sprintf("%.0f s", msg.ObsTime.Seconds())))
	return lines
}

// renderStats shows the position statistics the workbench scatter
// panel computes, without the plot.
func (m *monitorView) renderStats() []string {
	r := m.stats.compute()
	if r.N == 0 || !r.rotOK {
		return []string{faintStyle.Render("Waiting for position")}
	}
	const lw = 16
	lines := []string{
		kv(lw, "50% CEP", formatPrecision(r.CEP50)),
		kv(lw, "95% CEP", formatPrecision(r.CEP95)),
		kv(lw, "RMS Horiz", formatPrecision(r.RMSH)),
		kv(lw, "RMS Vert", formatPrecision(r.RMSV)),
		kv(lw, "RMS 3D", formatPrecision(r.RMS3D)),
		kv(lw, "Points", fmt.Sprintf("%d", r.N)),
		kv(lw, "Mean ECEF", fmt.Sprintf("%.4f, %.4f, %.4f", r.Mean[0], r.Mean[1], r.Mean[2])),
	}
	if len(m.baseARPs) > 0 {
		lines = append(lines, "", labelStyle.Render("Distance from RTCM ARP"))
		var rows [][]cell
		for _, id := range slices.Sorted(maps.Keys(m.baseARPs)) {
			d3, dh, dv := r.arpDist(m.baseARPs[id])
			rows = append(rows, []cell{
				{text: fmt.Sprintf("%d", id)},
				{text: formatPrecision(d3)},
				{text: formatPrecision(dh)},
				{text: formatPrecision(dv)},
			})
		}
		lines = append(lines, renderTable([]string{"Station", "3D", "Horiz", "Vert"},
			[]bool{false, true, true, true}, rows)...)
	}
	return lines
}

// formatPrecision formats a distance with the workbench's precision
// rule: millimetres below one metre, metres otherwise.
func formatPrecision(meters float64) string {
	if meters < 1 {
		return fmt.Sprintf("%.1f mm", meters*1000)
	}
	return fmt.Sprintf("%.3f m", meters)
}
