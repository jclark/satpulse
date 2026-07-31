package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
)

// cfgItem is one row of the configuration form.
type cfgItem struct {
	render func(focused bool) string
	// key handles a key while the item has focus; nil means the item
	// is not focusable (headings, info lines).
	key func(k tea.KeyPressMsg) tea.Cmd
	// enabled gates focus and interaction; nil means always enabled.
	enabled func() bool
	// captures reports that a focused text input wants printable keys.
	captures bool
}

// configView is the Configuration tab: read and apply receiver
// configuration, controls gated on the probed ConfigSupport flags as
// in the workbench Config tab.
type configView struct {
	sess   *session.Session
	onPort func(string) // forwards the read-back receiver port

	connState session.ConnState
	receiver  session.ReceiverEvent
	speed     int

	catalog      map[string][]string
	catalogOrder []string

	// Form state. The *Touched flags mark sections to include in
	// Apply, as in the workbench.
	selSignals     map[string]bool // "GNSS:SIG"
	signalsTouched bool
	minElev        textinput.Model
	minElevTouched bool

	ppsPeriod, ppsWidth, cableDelay          textinput.Model
	timeGNSS                                 int
	alignToGNSS, onlyWhenLocked, risingEdge  bool
	timePulseTouched                         bool

	timeMode        int // 0 none, 1 mobile, 2 survey, 3 fixed
	timeModeTouched bool
	staticNoPos     bool
	surveyTime, surveyAcc textinput.Model
	surveyAgain     bool
	surveyReport    bool
	coordSystem     int // 0 ecef, 1 llh
	ecefX, ecefY, ecefZ textinput.Model
	llhLat, llhLon, llhHeight textinput.Model
	fixedAcc        textinput.Model

	nmeaChange, nmeaDisable bool
	nmeaFlags               map[string]*bool
	rtcmChange, rtcmDisable bool
	msm                     int // 0 none, 1 MSM4, 2 MSM7
	rtcmLax, rtcmARP        bool
	pvtChange               bool
	pvtFlags                map[string]*bool
	satsChange              bool
	satsSat, satsSignal     bool
	rawChange               bool
	rawObs, rawNav          bool

	portNotUART  bool
	speedIdx     int
	speedTouched bool
	saveType     int // gpsprot.SaveType
	resetType    int // gpsprot.ResetType

	lastRead *gpsprot.ConfigProps
	readOnce bool
	reading  bool
	applying bool
	applied  bool
	errMsg   string

	items  []cfgItem
	focus  int
	offset int

	// picker is the signal-selection sub-form; non-nil while open.
	picker      []cfgItem
	pickerSel   map[string]bool
	pickerFocus int
}

var nmeaFlagOrder = []string{"GGA", "GLL", "GSA", "GSV", "RMC", "VTG", "ZDA"}

func newConfigView(sess *session.Session, onPort func(string)) *configView {
	v := &configView{sess: sess, onPort: onPort, connState: sess.State()}
	v.catalog = sess.SignalCatalog(0)
	for g := gpsprot.GNSS(1); g <= gpsprot.GNSSLast; g++ {
		if _, ok := v.catalog[g.String()]; ok {
			v.catalogOrder = append(v.catalogOrder, g.String())
		}
	}
	mk := func(placeholder string, width int) textinput.Model {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = placeholder
		ti.SetWidth(width)
		return ti
	}
	v.minElev = mk("e.g. 10", 10)
	v.ppsPeriod = mk("e.g. 1.0", 10)
	v.ppsWidth = mk("e.g. 0.1", 10)
	v.cableDelay = mk("e.g. 50", 10)
	v.surveyTime = mk("2000", 10)
	v.surveyAcc = mk("20", 10)
	v.ecefX = mk("X (m)", 16)
	v.ecefY = mk("Y (m)", 16)
	v.ecefZ = mk("Z (m)", 16)
	v.llhLat = mk("lat (deg)", 16)
	v.llhLon = mk("lon (deg)", 16)
	v.llhHeight = mk("height (m)", 16)
	v.fixedAcc = mk("20", 10)
	v.resetForm()
	v.buildItems()
	v.focus = -1
	moveItemFocus(v.items, &v.focus, 1)
	return v
}

// resetForm restores every control to its default, as the workbench
// does on disconnect.
func (v *configView) resetForm() {
	v.selSignals = make(map[string]bool)
	v.signalsTouched = false
	v.minElev.SetValue("")
	v.minElevTouched = false
	v.ppsPeriod.SetValue("")
	v.ppsWidth.SetValue("")
	v.cableDelay.SetValue("")
	v.timeGNSS = 0
	v.alignToGNSS, v.onlyWhenLocked, v.risingEdge = true, true, true
	v.timePulseTouched = false
	v.timeMode = 0
	v.timeModeTouched = false
	v.staticNoPos = false
	v.surveyTime.SetValue("")
	v.surveyAcc.SetValue("")
	v.surveyAgain = false
	v.surveyReport = true
	v.coordSystem = 0
	for _, ti := range []*textinput.Model{&v.ecefX, &v.ecefY, &v.ecefZ, &v.llhLat, &v.llhLon, &v.llhHeight, &v.fixedAcc} {
		ti.SetValue("")
	}
	v.nmeaChange, v.nmeaDisable = false, false
	// The item closures capture the *bool pointers, so the maps are
	// allocated once and only the values reset.
	if v.nmeaFlags == nil {
		v.nmeaFlags = make(map[string]*bool)
		for _, f := range nmeaFlagOrder {
			v.nmeaFlags[f] = new(bool)
		}
	}
	for f, p := range v.nmeaFlags {
		*p = f == "RMC" || f == "GGA" || f == "GSA" || f == "GSV"
	}
	v.rtcmChange, v.rtcmDisable = false, false
	v.msm = 1
	if !v.has(gpsprot.ConfigSupportRTCMMSM4) && v.has(gpsprot.ConfigSupportRTCMMSM7) {
		v.msm = 2
	}
	v.rtcmLax, v.rtcmARP = true, true
	v.pvtChange = false
	if v.pvtFlags == nil {
		v.pvtFlags = make(map[string]*bool)
		for _, f := range []string{"pos", "vel", "time", "timePulse", "leapSecond", "tai", "ecef", "timePulseAfter", "quality", "epoch", "off"} {
			v.pvtFlags[f] = new(bool)
		}
	}
	for f, p := range v.pvtFlags {
		*p = f == "time" || f == "pos" || f == "quality" || f == "epoch" || f == "off"
	}
	v.satsChange = false
	v.satsSat, v.satsSignal = true, true
	v.rawChange = false
	v.rawObs, v.rawNav = true, false
	v.portNotUART = false
	v.speedIdx = speedIndex(v.speed)
	v.speedTouched = false
	v.saveType = int(gpsprot.SaveNone)
	v.resetType = int(gpsprot.ResetNone)
	v.applied = false
	v.errMsg = ""
}

func speedIndex(speed int) int {
	for i, s := range speeds {
		if s == speed {
			return i
		}
	}
	return 1 // 38400
}

func (v *configView) title() string { return "Configuration" }

func (v *configView) capturesInput() bool {
	items := v.items
	focus := v.focus
	if v.picker != nil {
		items, focus = v.picker, v.pickerFocus
	}
	if focus >= 0 && focus < len(items) {
		it := &items[focus]
		return it.captures && (it.enabled == nil || it.enabled())
	}
	return false
}

func (v *configView) hints() []string {
	if v.picker != nil {
		return []string{"up/down move", "space toggle", "enter OK", "esc cancel"}
	}
	return []string{"up/down move", "space/left/right change", "enter activate"}
}

// connected mirrors the workbench gating: every control requires a
// connected, idle receiver.
func (v *configView) connected() bool {
	return v.connState == session.StateConnected
}

// has reports configuration item support; zero flags mean support is
// unknown and everything stays enabled.
func (v *configView) has(f gpsprot.ConfigSupportFlags) bool {
	cs := v.receiver.ConfigSupport
	return cs == 0 || cs&f != 0
}

func (v *configView) gnssSupported(name string) bool {
	if !v.receiver.Info.IsSet() {
		return true
	}
	gs := v.receiver.Info.Get().SupportedGNSS
	if gs.IsZero() {
		return true
	}
	g, err := gpsprot.ParseGNSS(name)
	return err == nil && gs.Contains(g)
}

func (v *configView) signalSupported(gnss, sig string) bool {
	ss := v.receiver.SignalsSupported
	if len(ss) == 0 {
		return true
	}
	for _, s := range ss[gnss] {
		if s == sig {
			return true
		}
	}
	return false
}

func (v *configView) handleEvent(ev session.Event) {
	switch ev.Name {
	case session.EventState:
		if st, ok := ev.Data.(session.ConnState); ok {
			v.connState = st
			if st == session.StateDisconnected {
				v.resetForm()
				v.lastRead = nil
				v.readOnce = false
				v.reading = false
				v.applying = false
			}
		}
	case session.EventReceiver:
		if r, ok := ev.Data.(session.ReceiverEvent); ok {
			v.receiver = r
		}
	case session.EventSpeed:
		if s, ok := ev.Data.(int); ok && s != 0 {
			v.speed = s
			if !v.speedTouched {
				v.speedIdx = speedIndex(s)
			}
		}
	}
}

// configReadMsg delivers the result of a ReadConfig run.
type configReadMsg struct {
	props *gpsprot.ConfigProps
	err   error
}

// configAppliedMsg delivers the result of an ApplyConfig run.
type configAppliedMsg struct {
	err error
}

func (v *configView) readCmd() tea.Cmd {
	if !v.connected() || v.reading {
		return nil
	}
	v.reading = true
	v.errMsg = ""
	sess := v.sess
	return func() tea.Msg {
		props, err := sess.ReadConfig(context.Background())
		return configReadMsg{props: props, err: err}
	}
}

func (v *configView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case viewShownMsg:
		// First visit while connected reads the configuration back,
		// as the workbench does when the tab first becomes visible.
		if v.connected() && !v.readOnce {
			v.readOnce = true
			return v.readCmd()
		}
		return nil
	case configReadMsg:
		v.reading = false
		if msg.err != nil {
			v.errMsg = "Configuration read failed: " + msg.err.Error()
			return nil
		}
		v.populate(msg.props)
		return nil
	case configAppliedMsg:
		v.applying = false
		if msg.err != nil {
			v.errMsg = "Apply failed: " + msg.err.Error()
			return nil
		}
		v.clearTouched()
		v.applied = true
		return nil
	case tea.KeyPressMsg:
		return v.handleKey(msg)
	}
	return v.updateFocusedInput(msg)
}

func (v *configView) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	items := v.items
	focus := &v.focus
	if v.picker != nil {
		items, focus = v.picker, &v.pickerFocus
	}
	switch msg.String() {
	case "up":
		moveItemFocus(items, focus, -1)
		return nil
	case "down":
		moveItemFocus(items, focus, 1)
		return nil
	case "esc":
		if v.picker != nil {
			v.picker = nil
			return nil
		}
	}
	if *focus >= 0 && *focus < len(items) {
		it := &items[*focus]
		if it.key != nil && (it.enabled == nil || it.enabled()) {
			return it.key(msg)
		}
	}
	return nil
}

func (v *configView) updateFocusedInput(msg tea.Msg) tea.Cmd {
	if v.picker == nil && v.focus >= 0 && v.focus < len(v.items) {
		it := &v.items[v.focus]
		if it.captures && it.key != nil {
			if key, ok := msg.(tea.KeyPressMsg); ok {
				return it.key(key)
			}
		}
	}
	return nil
}

func (v *configView) render(width, height int) string {
	items := v.items
	focus := v.focus
	if v.picker != nil {
		items, focus = v.picker, v.pickerFocus
	}
	// The status line stays fixed above the form so a pending or
	// error state is visible wherever the form is scrolled.
	if s := v.statusLine(); s != "" {
		return truncate(s, width) + "\n" + renderItems(items, focus, &v.offset, width, height-1)
	}
	return renderItems(items, focus, &v.offset, width, height)
}

// statusLine mirrors the workbench pending/status line.
func (v *configView) statusLine() string {
	if v.errMsg != "" {
		return errorStyle.Render(v.errMsg)
	}
	if v.reading {
		return faintStyle.Render("Reading configuration...")
	}
	if v.applying {
		return faintStyle.Render("Applying configuration...")
	}
	if label := v.pendingLabel(); label != "" {
		return faintStyle.Render("Changes pending to " + label)
	}
	if v.applied {
		return faintStyle.Render("Configuration applied")
	}
	return ""
}

func (v *configView) pendingLabel() string {
	var parts []string
	if v.timePulseTouched {
		parts = append(parts, "time pulse")
	}
	if v.timeModeTouched {
		parts = append(parts, "time mode")
	}
	if v.signalsTouched || v.minElevTouched {
		parts = append(parts, "satellites and signals")
	}
	if v.nmeaChange || v.rtcmChange || v.pvtChange || v.satsChange || v.rawChange {
		parts = append(parts, "messages")
	}
	if v.speedTouched {
		parts = append(parts, "serial speed")
	}
	if v.saveType != int(gpsprot.SaveNone) {
		parts = append(parts, "save")
	}
	if v.resetType != int(gpsprot.ResetNone) {
		parts = append(parts, "reset")
	}
	return strings.Join(parts, ", ")
}

func (v *configView) clearTouched() {
	v.signalsTouched = false
	v.minElevTouched = false
	v.timePulseTouched = false
	v.timeModeTouched = false
	v.nmeaChange = false
	v.rtcmChange = false
	v.pvtChange = false
	v.satsChange = false
	v.rawChange = false
	v.speedTouched = false
	v.saveType = int(gpsprot.SaveNone)
	v.resetType = int(gpsprot.ResetNone)
}
