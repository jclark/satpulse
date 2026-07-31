package tui

import (
	"slices"
	"strconv"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/lib/serialenum"
)

// speeds are the serial speeds offered by the speed select, matching
// the workbench connection bar (speeds.ts).
var speeds = []int{9600, 38400, 57600, 115200, 230400, 460800, 921600}

// connectForm is the connection overlay: device and speed entry,
// enumerated ports, connect and disconnect. It replaces the
// workbench's fixed connection panel and is built on the shared form
// row machinery.
type connectForm struct {
	sess    *session.Session
	vendors []gpsreg.Vendor

	device    textinput.Model
	speedOpts []string
	speedIdx  int

	items  []cfgItem
	focus  int
	offset int

	ports     []serialenum.Port
	state     session.ConnState
	err       string
	busy      bool
	auto      bool // auto-connect at startup requested
	wantClose bool
}

// portsMsg delivers the enumerated serial ports.
type portsMsg []serialenum.Port

func newConnectForm(sess *session.Session, vendors []gpsreg.Vendor, device string, speed int) *connectForm {
	f := &connectForm{sess: sess, vendors: vendors, state: sess.State()}
	f.device = textinput.New()
	f.device.Prompt = ""
	f.device.Placeholder = "/dev/..."
	f.device.SetWidth(40)
	f.device.SetValue(device)
	// Auto-connect needs both flags, judged before the speed default.
	f.auto = device != "" && speed != 0
	if speed == 0 {
		// The workbench's default speed selection.
		speed = 9600
	}
	for i, s := range speeds {
		f.speedOpts = append(f.speedOpts, strconv.Itoa(s))
		if s == speed {
			f.speedIdx = i
		}
	}
	if !slices.Contains(f.speedOpts, strconv.Itoa(speed)) {
		// A -s value outside the standard list is still offered.
		f.speedOpts = append(f.speedOpts, strconv.Itoa(speed))
		f.speedIdx = len(f.speedOpts) - 1
	}
	f.buildItems()
	f.focus = -1
	moveItemFocus(f.items, &f.focus, 1)
	return f
}

// connectedNow reports whether a connection exists or is being made:
// the workbench disables the device and speed controls then, and the
// action button toggles to Disconnect.
func (f *connectForm) connectedNow() bool {
	return f.state != session.StateDisconnected
}

// buildItems assembles the overlay's rows; rebuilt when the port
// enumeration arrives.
func (f *connectForm) buildItems() {
	edit := func() bool { return !f.connectedNow() }
	items := []cfgItem{
		itemHeading("Connection"),
		itemInfo(func() string {
			label := stateLabels[f.state]
			if label == "" {
				label = string(f.state)
			}
			return "  " + label
		}),
		itemSpacer(),
		itemText("Device", &f.device, edit, nil, nil),
		itemSelect("Speed", func() []string { return f.speedOpts }, &f.speedIdx, edit, nil),
		itemButton(func() string {
			if f.connectedNow() {
				return "Disconnect"
			}
			return "Connect"
		}, nil, func() tea.Cmd {
			if f.connectedNow() {
				return f.disconnectCmd()
			}
			return f.connectCmd()
		}),
		itemInfo(func() string {
			if f.busy {
				return faintStyle.Render("  working...")
			}
			if f.err != "" {
				return errorStyle.Render("  " + f.err)
			}
			return ""
		}),
	}
	if len(f.ports) > 0 {
		items = append(items, itemSpacer(), itemHeading("Ports"))
		for _, p := range f.ports {
			items = append(items, f.portItem(p))
		}
	}
	f.items = items
}

// portItem is one enumerated port; selecting it fills the device
// field.
func (f *connectForm) portItem(p serialenum.Port) cfgItem {
	edit := func() bool { return !f.connectedNow() }
	return cfgItem{
		enabled: edit,
		render: func(focused bool) string {
			line := p.Device
			if p.Display != "" && p.Display != p.Device {
				line += "  " + faintStyle.Render(p.Display)
			}
			return marker(focused) + itemStyle(edit, line)
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			if k.String() == "enter" || k.String() == "space" {
				f.device.SetValue(p.Device)
				f.device.CursorEnd()
			}
			return nil
		},
	}
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
	speed, _ := strconv.Atoi(f.speedOpts[f.speedIdx])
	if device == "" {
		f.err = "device is required"
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
	switch ev := ev.(type) {
	case session.ConnState:
		if ev == session.StateConnected && f.state != ev {
			// A completed connect (including auto-connect) closes
			// the overlay; the header shows the result.
			f.wantClose = true
		}
		f.state = ev
	case session.ReceiverEvent:
		if ev.Error != "" {
			f.err = ev.Error
		}
	}
}

func (f *connectForm) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case portsMsg:
		f.ports = msg
		// A sole enumerated port fills an empty device field, as the
		// workbench preselects it.
		if f.device.Value() == "" && len(f.ports) == 1 {
			f.device.SetValue(f.ports[0].Device)
			f.device.CursorEnd()
		}
		f.buildItems()
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
		switch msg.String() {
		case "esc":
			f.wantClose = true
			return nil
		case "up", "shift+tab":
			moveItemFocus(f.items, &f.focus, -1)
			return nil
		case "down", "tab":
			moveItemFocus(f.items, &f.focus, 1)
			return nil
		}
		if f.focus >= 0 && f.focus < len(f.items) {
			it := &f.items[f.focus]
			if it.key != nil && (it.enabled == nil || it.enabled()) {
				return it.key(msg)
			}
		}
	}
	return nil
}

func (f *connectForm) render(width, height int) string {
	return renderItems(f.items, f.focus, &f.offset, width, height)
}

func (f *connectForm) hints() []string {
	return []string{"up/down field", "left/right speed", "enter select/connect", "esc close"}
}
