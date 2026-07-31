package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/msgfile"
)

// view is one full-screen tab of the TUI. The root model routes
// session events to every view (each keeps only the state it shows)
// and keys plus rendering to the active one.
type view interface {
	title() string
	// handleEvent updates view state from a session event; called for
	// every view, visible or not.
	handleEvent(ev session.Event)
	// update handles input and other messages for the active view.
	update(msg tea.Msg) tea.Cmd
	// render returns the view content, at most height lines, each at
	// most width columns.
	render(width, height int) string
	// hints returns footer key hints for the active view.
	hints() []string
	// capturesInput reports whether a text input in the view has
	// focus, in which case printable keys must reach the view rather
	// than trigger global shortcuts.
	capturesInput() bool
}

// model is the root Bubble Tea model.
type model struct {
	sess    *session.Session
	lg      *slog.Logger
	sink    *sink
	vendors []gpsreg.Vendor
	msgDirs []msgfile.Dir

	width, height int

	// Session state mirrored from events for the header line.
	connState session.ConnState
	receiver  session.ReceiverEvent
	speed     int
	corr      session.CorrEvent

	views  []view
	active int

	// The Messages view joins the tab bar only once a message file is
	// loaded, as in the workbench.
	messages *messagesView

	conn *connectForm // non-nil while the connect overlay is open

	status  string // latest log line, shown above the footer
	statusErr bool
}

// newModel creates the root model. device and speed seed the connect
// form (and are auto-connected by Init when both are set).
func newModel(sess *session.Session, lg *slog.Logger, sink *sink, vendors []gpsreg.Vendor, msgDirs []msgfile.Dir, device string, speed int) *model {
	m := &model{
		sess:      sess,
		lg:        lg,
		sink:      sink,
		vendors:   vendors,
		msgDirs:   msgDirs,
		connState: session.StateDisconnected,
	}
	m.messages = newMessagesView(sess, msgDirs)
	m.views = []view{
		newMonitorView(sess),
		newPacketsView(sess),
		newCorrectionsView(sess),
		newConfigView(sess, m.messages.setDefaultPort),
	}
	m.conn = newConnectForm(sess, vendors, device, speed)
	return m
}

// opResultMsg is the result of an asynchronous session call made from
// a form or view; err is nil on success.
type opResultMsg struct {
	op  string
	err error
}

// viewShownMsg is sent to a view when it becomes the active tab.
type viewShownMsg struct{}

func (m *model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if cmd := m.conn.autoConnect(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, m.messages.loadCatalog())
	return tea.Batch(cmds...)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case eventsMsg:
		for _, ev := range msg.events {
			m.handleEvent(ev)
		}
		for _, ev := range msg.packets {
			m.handleEvent(ev)
		}
		return m, m.closeConnIfWanted()
	case tea.KeyPressMsg:
		cmd := m.handleKey(msg)
		return m, tea.Batch(cmd, m.closeConnIfWanted())
	}
	// Everything else (command results, input ticks) is broadcast:
	// each view filters by message type, and a result must reach its
	// view even if the user switched tabs meanwhile.
	var cmds []tea.Cmd
	if m.conn != nil {
		cmds = append(cmds, m.conn.update(msg))
	}
	for _, v := range m.views {
		cmds = append(cmds, v.update(msg))
	}
	cmds = append(cmds, m.messages.update(msg))
	return m, tea.Batch(cmds...)
}

func (m *model) handleEvent(ev session.Event) {
	switch ev := ev.(type) {
	case session.ConnState:
		m.connState = ev
		if ev == session.StateDisconnected {
			// The session clears its probe and speed on disconnect but
			// emits only the state change; drop the mirrored values so
			// the header does not keep showing the dead connection's
			// receiver and baud rate. Corrections state keeps its own
			// lifecycle, as in the workbench.
			m.receiver = session.ReceiverEvent{}
			m.speed = 0
		}
	case session.ReceiverEvent:
		m.receiver = ev
	case session.SpeedEvent:
		m.speed = int(ev)
	case session.CorrEvent:
		m.corr = ev
	case session.LogEvent:
		m.status = ev.Message
		m.statusErr = ev.Level == slog.LevelError.String()
	}
	for _, v := range m.views {
		v.handleEvent(ev)
	}
	m.messages.handleEvent(ev)
	if m.conn != nil {
		m.conn.handleEvent(ev)
	}
}

func (m *model) activeView() view {
	tabs := m.tabs()
	if m.active >= len(tabs) {
		m.active = 0
	}
	return tabs[m.active]
}

// closeConnIfWanted closes the connection overlay when it asked to be
// closed (esc, or a completed connect), and notifies the view that
// becomes visible again: a connect through the overlay must trigger
// the same on-shown work as switching to the tab (the Configuration
// tab's automatic readback, the Messages catalog refresh).
func (m *model) closeConnIfWanted() tea.Cmd {
	if m.conn == nil || !m.conn.wantClose {
		return nil
	}
	m.conn = nil
	return m.activeView().update(viewShownMsg{})
}

// tabs returns the views currently in the tab bar: the Messages view
// appears only when the message-file library offers something, as the
// workbench shows its Message file tab only with a message-file
// capable transport.
func (m *model) tabs() []view {
	if m.messages.available() {
		return append(m.views[:len(m.views):len(m.views)], m.messages)
	}
	return m.views
}

func (m *model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	tabs := m.tabs()
	switch msg.String() {
	case "ctrl+c":
		m.sess.Disconnect()
		return tea.Quit
	}
	if m.conn != nil {
		return m.conn.update(msg)
	}
	if m.activeView().capturesInput() {
		switch msg.String() {
		case "tab", "shift+tab":
		default:
			return m.activeView().update(msg)
		}
	}
	switch key := msg.String(); key {
	case "q":
		m.sess.Disconnect()
		return tea.Quit
	case "tab":
		return m.switchTab((m.active + 1) % len(tabs))
	case "shift+tab":
		return m.switchTab((m.active + len(tabs) - 1) % len(tabs))
	case "c":
		m.conn = newConnectForm(m.sess, m.vendors, "", 0)
		return m.conn.open()
	case "1", "2", "3", "4", "5":
		if i := int(key[0] - '1'); i < len(tabs) {
			return m.switchTab(i)
		}
		return nil
	}
	return m.activeView().update(msg)
}

// switchTab activates a tab, gates the high-rate gps:packet stream on
// the Packets view being visible, and notifies the shown view.
func (m *model) switchTab(i int) tea.Cmd {
	m.active = i
	_, packets := m.activeView().(*packetsView)
	m.sink.setWantsPackets(packets)
	return m.activeView().update(viewShownMsg{})
}

func (m *model) View() tea.View {
	header := m.renderHeader()
	tabbar := m.renderTabs()
	footer := m.renderFooter()
	statusLine := m.renderStatus()
	bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(tabbar) -
		lipgloss.Height(statusLine) - lipgloss.Height(footer)
	if bodyHeight < 0 {
		bodyHeight = 0
	}
	var body string
	if m.conn != nil {
		body = m.conn.render(m.width, bodyHeight)
	} else {
		body = m.activeView().render(m.width, bodyHeight)
	}
	body = lipgloss.PlaceVertical(bodyHeight, lipgloss.Top, body)
	v := tea.NewView(strings.Join([]string{header, tabbar, body, statusLine, footer}, "\n"))
	v.AltScreen = true
	return v
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true)
	tabStyle      = lipgloss.NewStyle().Padding(0, 1)
	activeTab     = lipgloss.NewStyle().Padding(0, 1).Reverse(true).Bold(true)
	selStyle      = lipgloss.NewStyle().Reverse(true)
	footerStyle   = lipgloss.NewStyle().Faint(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Red)
	faintStyle    = lipgloss.NewStyle().Faint(true)
	labelStyle    = lipgloss.NewStyle().Faint(true)
	sectionStyle  = lipgloss.NewStyle().Bold(true).Underline(true)
)

// renderHeader renders the persistent connection line: state, receiver
// identification, port speed, corrections state.
func (m *model) renderHeader() string {
	parts := []string{string(m.connState)}
	if m.receiver.Info.IsSet() {
		info := m.receiver.Info.Get()
		id := strings.TrimSpace(info.Vendor + " " + info.Hardware)
		if id != "" {
			parts = append(parts, id)
		}
		if info.Firmware != "" {
			parts = append(parts, info.Firmware)
		}
	} else if len(m.receiver.PacketFormats) > 0 {
		parts = append(parts, strings.Join(m.receiver.PacketFormats, ","))
	}
	if m.speed != 0 {
		parts = append(parts, fmt.Sprintf("%d bps", m.speed))
	}
	if m.corr.State != "" && m.corr.State != "stopped" {
		parts = append(parts, "corrections "+m.corr.State)
	}
	return headerStyle.Render(truncate("SatPulse  "+strings.Join(parts, "  |  "), m.width))
}

func (m *model) renderTabs() string {
	var b strings.Builder
	for i, v := range m.tabs() {
		st := tabStyle
		if i == m.active && m.conn == nil {
			st = activeTab
		}
		b.WriteString(st.Render(fmt.Sprintf("%d %s", i+1, v.title())))
	}
	return truncate(b.String(), m.width)
}

func (m *model) renderStatus() string {
	if m.status == "" {
		return ""
	}
	s := truncate(m.status, m.width)
	if m.statusErr {
		return errorStyle.Render(s)
	}
	return faintStyle.Render(s)
}

func (m *model) renderFooter() string {
	var hints []string
	if m.conn != nil {
		hints = m.conn.hints()
	} else {
		hints = append(hints, m.activeView().hints()...)
		hints = append(hints, "tab switch view", "c connection")
	}
	hints = append(hints, "ctrl+c quit")
	return footerStyle.Render(truncate(strings.Join(hints, "   "), m.width))
}
