package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// corRow is one correction message summary row, keyed by message ID.
type corRow struct {
	tag       string
	msgID     string
	count     int
	splits    int
	lastEpoch int
	station   int // RTCM reference station ID, -1 unknown
	lastTime  time.Time
	interval  time.Duration
}

// correctionsView is the Corrections tab: the source form, session
// status, and the correction packet summary.
type correctionsView struct {
	sess *session.Session

	mode      int // 0 ntrip, 1 tcp
	host      textinput.Model
	port      textinput.Model
	portNtrip string // per-mode port values, as in the workbench
	portTCP   string
	mount     textinput.Model
	user      textinput.Model
	pass      textinput.Model
	nmeaSend  bool
	focus     int

	connState session.ConnState
	corr      *session.CorrEvent
	corrErr   string
	pending   bool
	nmeaValid bool
	nmeaLat   float64
	nmeaLon   float64

	rows      map[string]*corRow
	epoch     int
	stoppedAt time.Time // freeze instant for ages when not running
}

const (
	corFieldMode = iota
	corFieldHost
	corFieldPort
	corFieldMount
	corFieldUser
	corFieldPass
	corFieldNMEA
	corFieldButton
	corFieldCount
)

func newCorrectionsView(sess *session.Session) *correctionsView {
	v := &correctionsView{sess: sess, connState: sess.State(), rows: make(map[string]*corRow)}
	mk := func(placeholder string, width int) textinput.Model {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = placeholder
		ti.SetWidth(width)
		return ti
	}
	v.host = mk("e.g. 10.0.0.1", 30)
	v.port = mk("", 8)
	v.port.SetValue("2101")
	v.portNtrip = "2101"
	v.mount = mk("mountpoint", 20)
	v.user = mk("", 20)
	v.pass = mk("", 20)
	v.pass.EchoMode = textinput.EchoPassword
	v.focus = corFieldHost
	v.host.Focus()
	return v
}

func (v *correctionsView) title() string { return "Corrections" }

func (v *correctionsView) capturesInput() bool {
	switch v.focus {
	case corFieldHost, corFieldPort, corFieldMount, corFieldUser, corFieldPass:
		return !v.locked()
	}
	return false
}

func (v *correctionsView) hints() []string {
	return []string{"up/down field", "left/right mode", "space toggle", "enter start/stop", "x clear table"}
}

// running reports whether a correction session is active.
func (v *correctionsView) running() bool {
	if v.corr == nil {
		return false
	}
	switch v.corr.State {
	case "connecting", "connected", "reconnecting":
		return true
	}
	return false
}

// locked reports whether the form fields are editable. Editing and
// starting are different: preparing the source details is fine in any
// receiver state, including disconnected, so only a running
// correction session or a pending start/stop locks the fields.
// Starting is gated separately on exactly StateConnected (startCmd),
// which is what StartCorrections requires.
func (v *correctionsView) locked() bool {
	return v.running() || v.pending
}

func (v *correctionsView) ntrip() bool { return v.mode == 0 }

func (v *correctionsView) handleEvent(ev session.Event) {
	switch ev := ev.(type) {
	case session.ConnState:
		v.connState = ev
		if ev == session.StateDisconnected {
			v.corr = nil
			v.corrErr = ""
			v.pending = false
			v.nmeaValid = false
			v.clearRows()
		}
	case session.CorrEvent:
		v.corr = &ev
		v.pending = false
		switch ev.State {
		case "reconnecting", "failed":
			v.corrErr = ev.Error
			if v.corrErr == "" {
				if ev.State == "failed" {
					v.corrErr = "connection failed"
				} else {
					v.corrErr = "connection lost"
				}
			}
		case "connecting":
			v.corrErr = ""
		case "stopped":
			v.stoppedAt = time.Now()
		}
	case session.NMEAPositionEvent:
		v.nmeaValid = ev.Valid
		v.nmeaLat, v.nmeaLon = ev.Lat, ev.Lon
	case session.CorrPacketEvent:
		v.handleCorPacket(ev.CorReportMsg)
	}
}

// handleCorPacket updates the per-message summary, counting logical
// messages: for fragmentable messages (RTCM MSM), the fragments of
// one epoch count once and extras count as splits.
func (v *correctionsView) handleCorPacket(msg *gpsprot.CorReportMsg) {
	now := time.Now()
	r := v.rows[msg.MsgID]
	if r == nil {
		r = &corRow{tag: string(msg.Tag), msgID: msg.MsgID, count: 1, station: -1, lastTime: now, lastEpoch: -1}
		if msg.FinalFragment.IsSet() {
			r.lastEpoch = v.epoch
		}
		v.rows[msg.MsgID] = r
	} else if msg.FinalFragment.IsSet() {
		if r.lastEpoch != v.epoch {
			r.count++
			r.lastEpoch = v.epoch
			r.interval = now.Sub(r.lastTime)
			r.lastTime = now
		} else {
			r.splits++
		}
	} else {
		r.count++
		r.interval = now.Sub(r.lastTime)
		r.lastTime = now
	}
	if id := msg.RTCMRefBaseID.Ptr(); id != nil {
		r.station = int(*id)
	}
	if ff := msg.FinalFragment.Ptr(); ff != nil && *ff {
		v.epoch++
	}
}

func (v *correctionsView) clearRows() {
	v.rows = make(map[string]*corRow)
	v.epoch = 0
}

func (v *correctionsView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case opResultMsg:
		if msg.op == "corrections" {
			v.pending = false
			if msg.err != nil {
				v.corrErr = msg.err.Error()
				if v.corr != nil && v.corr.State == "connecting" {
					v.corr = &session.CorrEvent{State: "failed"}
				}
			}
		}
		return nil
	case tea.KeyPressMsg:
		return v.handleKey(msg)
	}
	return v.updateFocused(msg)
}

func (v *correctionsView) updateFocused(msg tea.Msg) tea.Cmd {
	if v.locked() {
		return nil
	}
	var cmd tea.Cmd
	switch v.focus {
	case corFieldHost:
		v.host, cmd = v.host.Update(msg)
	case corFieldPort:
		v.port, cmd = v.port.Update(msg)
	case corFieldMount:
		v.mount, cmd = v.mount.Update(msg)
	case corFieldUser:
		v.user, cmd = v.user.Update(msg)
	case corFieldPass:
		v.pass, cmd = v.pass.Update(msg)
	}
	return cmd
}

func (v *correctionsView) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "up":
		return v.moveFocus(-1)
	case "down":
		return v.moveFocus(1)
	case "left", "right":
		if v.focus == corFieldMode && !v.locked() {
			v.setMode(1 - v.mode)
			return nil
		}
	case "space":
		if v.focus == corFieldNMEA && !v.locked() && v.ntrip() {
			v.nmeaSend = !v.nmeaSend
			return nil
		}
	case "enter":
		if v.running() {
			return v.stopCmd()
		}
		return v.startCmd()
	case "x":
		// Clear the correction message table, unless a text field
		// has focus and the x belongs to it.
		if !v.capturesInput() {
			v.clearRows()
			return nil
		}
	}
	return v.updateFocused(msg)
}

// setMode switches between ntrip and tcp, keeping a per-mode port
// value as the workbench does.
func (v *correctionsView) setMode(mode int) {
	if v.ntrip() {
		v.portNtrip = v.port.Value()
	} else {
		v.portTCP = v.port.Value()
	}
	v.mode = mode
	if v.ntrip() {
		v.port.SetValue(v.portNtrip)
	} else {
		v.port.SetValue(v.portTCP)
	}
}

// moveFocus moves to the next focusable field, skipping the
// ntrip-only fields in tcp mode.
func (v *correctionsView) moveFocus(dir int) tea.Cmd {
	for {
		v.focus = (v.focus + dir + corFieldCount) % corFieldCount
		if v.ntrip() {
			break
		}
		if v.focus != corFieldMount && v.focus != corFieldUser && v.focus != corFieldPass && v.focus != corFieldNMEA {
			break
		}
	}
	for _, ti := range []*textinput.Model{&v.host, &v.port, &v.mount, &v.user, &v.pass} {
		ti.Blur()
	}
	switch v.focus {
	case corFieldHost:
		return v.host.Focus()
	case corFieldPort:
		return v.port.Focus()
	case corFieldMount:
		return v.mount.Focus()
	case corFieldUser:
		return v.user.Focus()
	case corFieldPass:
		return v.pass.Focus()
	}
	return nil
}

// canStart mirrors the workbench's start gating.
func (v *correctionsView) canStart() bool {
	port, err := strconv.Atoi(v.port.Value())
	if err != nil || port <= 0 || port > 65535 {
		return false
	}
	if strings.TrimSpace(v.host.Value()) == "" {
		return false
	}
	if v.ntrip() && strings.TrimSpace(v.mount.Value()) == "" {
		return false
	}
	if v.ntrip() && v.nmeaSend && !v.nmeaValid {
		return false
	}
	return true
}

func (v *correctionsView) startCmd() tea.Cmd {
	if v.connState != session.StateConnected || v.running() || v.pending || !v.canStart() {
		return nil
	}
	mode := "tcp"
	if v.ntrip() {
		mode = "ntrip"
	}
	port, _ := strconv.Atoi(v.port.Value())
	src := session.CorrectionSource{
		Mode: mode,
		Host: strings.TrimSpace(v.host.Value()),
		Port: port,
	}
	if v.ntrip() {
		src.Mountpoint = strings.TrimSpace(v.mount.Value())
		src.Username = v.user.Value()
		src.Password = v.pass.Value()
		src.NMEASend = v.nmeaSend
	}
	v.pending = true
	v.corrErr = ""
	v.clearRows()
	sess := v.sess
	return func() tea.Msg {
		return opResultMsg{op: "corrections", err: sess.StartCorrections(src)}
	}
}

func (v *correctionsView) stopCmd() tea.Cmd {
	if v.pending {
		return nil
	}
	v.pending = true
	sess := v.sess
	return func() tea.Msg {
		return opResultMsg{op: "corrections", err: sess.StopCorrections()}
	}
}

func (v *correctionsView) render(width, height int) string {
	var b strings.Builder
	b.WriteString(sectionStyle.Render("Correction source") + "\n")
	modeName := "NTRIP"
	if !v.ntrip() {
		modeName = "TCP"
	}
	b.WriteString(v.field(corFieldMode, "Mode", "< "+modeName+" >") + "\n")
	b.WriteString(v.field(corFieldHost, "Host", v.host.View()) + "\n")
	b.WriteString(v.field(corFieldPort, "Port", v.port.View()) + "\n")
	if v.ntrip() {
		b.WriteString(v.field(corFieldMount, "Mountpoint", v.mount.View()) + "\n")
		b.WriteString(v.field(corFieldUser, "User", v.user.View()) + "\n")
		b.WriteString(v.field(corFieldPass, "Password", v.pass.View()) + "\n")
		check := "[ ]"
		if v.nmeaSend {
			check = "[x]"
		}
		pos := ""
		if v.nmeaValid {
			pos = fmt.Sprintf("   Position: %.7f, %.7f", v.nmeaLat, v.nmeaLon)
		}
		b.WriteString(v.field(corFieldNMEA, "", check+" Send position as NMEA"+pos) + "\n")
	}
	label := "[ Start ]"
	if v.running() {
		label = "[ Stop ]"
	}
	if v.focus == corFieldButton {
		label = activeTab.Render(label)
	}
	b.WriteString("  " + label + "\n\n")
	b.WriteString(v.renderStatus(width) + "\n\n")
	b.WriteString(sectionStyle.Render("Correction messages") + "\n")
	for _, l := range v.renderRows() {
		b.WriteString(truncate(l, width) + "\n")
	}
	return b.String()
}

func (v *correctionsView) field(n int, label, value string) string {
	marker := "  "
	if v.focus == n {
		marker = "> "
	}
	if label != "" {
		return marker + labelStyle.Render(fmt.Sprintf("%-12s", label)) + value
	}
	return marker + value
}

func (v *correctionsView) renderStatus(width int) string {
	state := "stopped"
	if v.corr != nil {
		state = v.corr.State
	}
	s := kv(6, "State", state)
	if v.corrErr != "" {
		s += "  " + errorStyle.Render(truncate(v.corrErr, width))
	}
	return s
}

func (v *correctionsView) renderRows() []string {
	if len(v.rows) == 0 {
		return []string{faintStyle.Render("No correction messages")}
	}
	rows := make([]*corRow, 0, len(v.rows))
	for _, r := range v.rows {
		rows = append(rows, r)
	}
	// Sort by tag, then message ID with numeric awareness (RTCM types
	// are integers, SPARTN types are "type.subtype").
	sortCorRows(rows)
	now := time.Now()
	if !v.running() && !v.stoppedAt.IsZero() {
		now = v.stoppedAt
	}
	var cells [][]cell
	for _, r := range rows {
		msm, desc := "", ""
		if r.tag == "RTCM" {
			msm, desc = rtcmInfo(r.msgID)
		} else if r.tag == "SPARTN" {
			desc = spartnInfo(r.msgID)
		}
		// Cap the description so the count columns stay on screen.
		if len(desc) > 44 {
			desc = desc[:41] + "..."
		}
		station := ""
		if r.station >= 0 {
			station = strconv.Itoa(r.station)
		}
		splits := ""
		if r.splits > 0 {
			splits = strconv.Itoa(r.splits)
		}
		age := ""
		if s := int(now.Sub(r.lastTime).Seconds()); s >= 0 {
			age = strconv.Itoa(s) + "s"
		}
		cells = append(cells, []cell{
			{text: r.tag}, {text: station}, {text: r.msgID}, {text: msm},
			{text: desc}, {text: strconv.Itoa(r.count)}, {text: splits}, {text: age},
		})
	}
	lines := renderTable(
		[]string{"Tag", "Station ID", "Type", "MSM", "Description", "Count", "Splits", "Age"},
		[]bool{false, false, false, false, false, true, true, true}, cells)
	// Dim stale rows with the workbench's adaptive threshold: a row
	// with only one message seen is never stale.
	for i, r := range rows {
		if r.interval > 0 && now.Sub(r.lastTime) >= max(10*time.Second, r.interval*5/2) {
			lines[i+1] = faintStyle.Render(lines[i+1])
		}
	}
	return lines
}

func sortCorRows(rows []*corRow) {
	numCmp := func(a, b string) int {
		na, ea := strconv.ParseFloat(a, 64)
		nb, eb := strconv.ParseFloat(b, 64)
		if ea == nil && eb == nil {
			return cmp.Compare(na, nb)
		}
		return strings.Compare(a, b)
	}
	slices.SortFunc(rows, func(a, b *corRow) int {
		if c := strings.Compare(a.tag, b.tag); c != 0 {
			return c
		}
		return numCmp(a.msgID, b.msgID)
	})
}
