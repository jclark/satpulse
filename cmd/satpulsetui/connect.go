package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/serialenum"
)

// speeds are the serial speeds offered by the speed field, matching
// the workbench connection bar (speeds.ts).
var speeds = []int{9600, 38400, 57600, 115200, 230400, 460800, 921600}

// connectForm is the connection overlay: device and speed entry,
// enumerated ports, connect and disconnect. It replaces the
// workbench's fixed connection panel.
type connectForm struct {
	sess    *session.Session
	vendors []gpsreg.Vendor

	device textinput.Model
	speed  textinput.Model
	focus  int // 0 device, 1 speed, 2 buttons
	button int // 0 connect, 1 disconnect

	ports    []serialenum.Port
	portIdx  int // last port filled into the device field
	state    session.ConnState
	err      string
	busy     bool
	auto     bool // auto-connect at startup requested
	wantClose bool
}

// portsMsg delivers the enumerated serial ports.
type portsMsg []serialenum.Port

func newConnectForm(sess *session.Session, vendors []gpsreg.Vendor, device string, speed int) *connectForm {
	f := &connectForm{sess: sess, vendors: vendors, state: sess.State(), portIdx: -1}
	f.device = textinput.New()
	f.device.Prompt = ""
	f.device.Placeholder = "/dev/..."
	f.device.SetWidth(40)
	f.device.SetValue(device)
	f.speed = textinput.New()
	f.speed.Prompt = ""
	f.speed.Placeholder = "speed"
	f.speed.SetWidth(8)
	if speed != 0 {
		f.speed.SetValue(strconv.Itoa(speed))
	}
	f.device.Focus()
	f.auto = device != "" && speed != 0
	return f
}

// open returns the command that populates the port list.
func (f *connectForm) open() tea.Cmd {
	return func() tea.Msg {
		ports, err := serialenum.List()
		if err != nil {
			return portsMsg(nil)
		}
		return portsMsg(ports)
	}
}

// autoConnect starts the startup connection when device and speed
// were both given on the command line, mirroring web-mode
// auto-connect. It also loads the port list.
func (f *connectForm) autoConnect() tea.Cmd {
	if !f.auto {
		return f.open()
	}
	return tea.Batch(f.open(), f.connectCmd())
}

func (f *connectForm) connectCmd() tea.Cmd {
	device := f.device.Value()
	speed, _ := strconv.Atoi(f.speed.Value())
	if device == "" {
		f.err = "device is required"
		return nil
	}
	if speed <= 0 {
		f.err = "speed must be greater than zero"
		return nil
	}
	f.err = ""
	f.busy = true
	sess, vendors := f.sess, f.vendors
	return func() tea.Msg {
		err := sess.Connect(session.SerialOpener{Device: device, Speed: speed}, vendors)
		return opResultMsg{op: "connect", err: err}
	}
}

func (f *connectForm) disconnectCmd() tea.Cmd {
	f.busy = true
	sess := f.sess
	return func() tea.Msg {
		sess.Disconnect()
		return opResultMsg{op: "disconnect", err: nil}
	}
}

func (f *connectForm) handleEvent(ev session.Event) {
	switch ev.Name {
	case session.EventState:
		if st, ok := ev.Data.(session.ConnState); ok {
			if st == session.StateConnected && f.state != st {
				// A completed connect (including auto-connect) closes
				// the overlay; the header shows the result.
				f.wantClose = true
			}
			f.state = st
		}
	case session.EventReceiver:
		if r, ok := ev.Data.(session.ReceiverEvent); ok && r.Error != "" {
			f.err = r.Error
		}
	}
}

func (f *connectForm) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case portsMsg:
		f.ports = msg
		return nil
	case opResultMsg:
		if msg.op != "connect" && msg.op != "disconnect" {
			return nil
		}
		f.busy = false
		if msg.err != nil {
			f.err = msg.err.Error()
		}
		return nil
	case tea.KeyPressMsg:
		return f.handleKey(msg)
	}
	return f.updateFocused(msg)
}

func (f *connectForm) updateFocused(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch f.focus {
	case 0:
		f.device, cmd = f.device.Update(msg)
	case 1:
		f.speed, cmd = f.speed.Update(msg)
	}
	return cmd
}

func (f *connectForm) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		f.wantClose = true
		return nil
	case "tab":
		return f.setFocus((f.focus + 1) % 3)
	case "shift+tab":
		return f.setFocus((f.focus + 2) % 3)
	case "up", "down":
		// On the device field the arrows cycle the enumerated ports;
		// elsewhere they move the focus.
		dir := 1
		if msg.String() == "up" {
			dir = -1
		}
		if f.focus == 0 && len(f.ports) > 0 {
			f.cyclePort(dir)
			return nil
		}
		return f.setFocus((f.focus + dir + 3) % 3)
	case "enter":
		if f.focus == 2 && f.button == 1 {
			return f.disconnectCmd()
		}
		return f.connectCmd()
	case "left":
		if f.focus == 1 {
			f.cycleSpeed(-1)
			return nil
		}
		if f.focus == 2 {
			f.button = 0
			return nil
		}
	case "right":
		if f.focus == 1 {
			f.cycleSpeed(1)
			return nil
		}
		if f.focus == 2 {
			f.button = 1
			return nil
		}
	}
	return f.updateFocused(msg)
}

func (f *connectForm) setFocus(n int) tea.Cmd {
	f.focus = n
	f.device.Blur()
	f.speed.Blur()
	switch n {
	case 0:
		return f.device.Focus()
	case 1:
		return f.speed.Focus()
	}
	return nil
}

// cyclePort steps the device field through the enumerated ports.
func (f *connectForm) cyclePort(dir int) {
	if f.portIdx < 0 && dir < 0 {
		f.portIdx = 0
	}
	f.portIdx = (f.portIdx + dir + len(f.ports)) % len(f.ports)
	f.device.SetValue(f.ports[f.portIdx].Device)
	f.device.CursorEnd()
}

// cycleSpeed steps the speed field through the common serial speeds.
func (f *connectForm) cycleSpeed(dir int) {
	cur, _ := strconv.Atoi(f.speed.Value())
	i := 0
	for j, s := range speeds {
		if s == cur {
			i = j + dir
			break
		}
		if s > cur {
			if dir > 0 {
				i = j
			} else {
				i = j - 1
			}
			break
		}
		i = j + dir
	}
	if i < 0 {
		i = 0
	}
	if i >= len(speeds) {
		i = len(speeds) - 1
	}
	f.speed.SetValue(strconv.Itoa(speeds[i]))
	f.speed.CursorEnd()
}

func (f *connectForm) render(width, height int) string {
	var b strings.Builder
	b.WriteString(sectionStyle.Render("Connection") + "\n\n")
	b.WriteString(kv(8, "State", string(f.state)) + "\n\n")
	b.WriteString(f.renderField(0, "Device", f.device.View()) + "\n")
	b.WriteString(f.renderField(1, "Speed", f.speed.View()) + "\n\n")
	connect := "[ Connect ]"
	disconnect := "[ Disconnect ]"
	if f.focus == 2 {
		if f.button == 0 {
			connect = activeTab.Render(connect)
		} else {
			disconnect = activeTab.Render(disconnect)
		}
	}
	b.WriteString("  " + connect + "  " + disconnect + "\n")
	if f.busy {
		b.WriteString("\n" + faintStyle.Render("working...") + "\n")
	}
	if f.err != "" {
		b.WriteString("\n" + errorStyle.Render(truncate(f.err, width)) + "\n")
	}
	if len(f.ports) > 0 {
		b.WriteString("\n" + labelStyle.Render("Ports") + "\n")
		for _, p := range f.ports {
			line := "  " + p.Device
			if p.Display != "" && p.Display != p.Device {
				line += "  " + faintStyle.Render(p.Display)
			}
			b.WriteString(truncate(line, width) + "\n")
		}
	}
	return b.String()
}

func (f *connectForm) renderField(n int, label, input string) string {
	marker := "  "
	if f.focus == n {
		marker = "> "
	}
	return marker + labelStyle.Render(fmt.Sprintf("%-8s", label)) + input
}

func (f *connectForm) hints() []string {
	return []string{
		"tab/up/down field", "up/down port", "left/right speed",
		"enter connect", "esc close",
	}
}
