package tui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/msgfile"
)

// receiverPorts are the port names offered by the Port selector,
// matching the workbench message-file panel.
var receiverPorts = []string{"-", "I2C", "UART1", "UART2", "USB", "SPI"}

// messagesView is the Messages tab: pick a message file from the
// library, send a tag's messages, and watch progress and receiver
// responses.
type messagesView struct {
	sess    *session.Session
	msgDirs []msgfile.Dir
	formats []gpsprot.PacketFormat

	connState session.ConnState
	catalog   []msgfile.Entry
	vendors   []string
	vendorIdx int
	fileIdx   int

	path string
	tags []session.MsgFileTag
	// sel is the selected tag row; armed marks it ready to send.
	sel       int
	armed     bool
	activeTag int // tag of the last send, -1 none

	portIdx     int
	defaultPort string
	save        bool

	respSession int
	sendStatus  string
	sendErr     bool
	responses   []session.ResponseEvent
	respSel     int // selected response, -1 none

	items  []cfgItem
	focus  int
	offset int
}

// msgCatalogMsg delivers the message-file catalog.
type msgCatalogMsg []msgfile.Entry

// msgLoadedMsg delivers a loaded message file.
type msgLoadedMsg struct {
	path string
	tags []session.MsgFileTag
	err  error
}

func newMessagesView(sess *session.Session, msgDirs []msgfile.Dir) *messagesView {
	v := &messagesView{
		sess:      sess,
		msgDirs:   msgDirs,
		formats:   gpsreg.CreatePacketFormats(nil),
		connState: sess.State(),
		activeTag: -1,
		respSel:   -1,
	}
	return v
}

func (v *messagesView) title() string { return "Messages" }

// available reports whether the tab appears: the message-file library
// must offer at least one file.
func (v *messagesView) available() bool { return len(v.catalog) > 0 }

func (v *messagesView) capturesInput() bool { return false }

func (v *messagesView) hints() []string {
	return []string{"up/down move", "left/right change", "enter select/send"}
}

// loadCatalog lists the message-file library.
func (v *messagesView) loadCatalog() tea.Cmd {
	dirs := v.msgDirs
	return func() tea.Msg {
		return msgCatalogMsg(msgfile.ListNames(dirs))
	}
}

func (v *messagesView) handleEvent(ev session.Event) {
	switch ev.Name {
	case session.EventState:
		if st, ok := ev.Data.(session.ConnState); ok {
			v.connState = st
			if st == session.StateConfiguring || st == session.StateDisconnected {
				v.respSession = 0
			}
		}
	case session.EventMsgSend:
		if e, ok := ev.Data.(session.MsgSendEvent); ok {
			v.handleSend(e)
		}
	case session.EventResponse:
		if e, ok := ev.Data.(session.ResponseEvent); ok {
			if e.Session == v.respSession {
				v.responses = append(v.responses, e)
				if v.respSel < 0 && (e.Text != "" || e.Bin != "") && e.Kind != "ack" {
					v.respSel = len(v.responses) - 1
				}
			}
		}
	}
}

func (v *messagesView) handleSend(e session.MsgSendEvent) {
	switch e.Status {
	case "started":
		v.respSession = e.Session
		v.sendStatus = "Sending..."
		v.sendErr = false
	case "sent":
		v.sendStatus = fmt.Sprintf("Sending %d/%d...", e.Current, e.Total)
		v.sendErr = false
	case "done":
		v.sendStatus = fmt.Sprintf("Sent %d messages", e.Total)
		v.sendErr = false
	case "error":
		v.sendStatus = e.Error
		if v.sendStatus == "" {
			v.sendStatus = "Error"
		}
		v.sendErr = true
	case "cancelled":
		v.sendStatus = "Cancelled"
		v.sendErr = false
	}
}

func (v *messagesView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case viewShownMsg:
		return v.loadCatalog()
	case msgCatalogMsg:
		v.setCatalog(msg)
		return nil
	case msgLoadedMsg:
		if msg.err != nil {
			v.sendStatus = msg.err.Error()
			v.sendErr = true
			return nil
		}
		v.applyLoaded(msg.path, msg.tags)
		return nil
	case opResultMsg:
		if msg.op == "send" && msg.err != nil {
			v.sendStatus = msg.err.Error()
			v.sendErr = true
		}
		return nil
	case tea.KeyPressMsg:
		items := v.buildItems()
		switch msg.String() {
		case "up":
			moveItemFocus(items, &v.focus, -1)
			return nil
		case "down":
			moveItemFocus(items, &v.focus, 1)
			return nil
		}
		if v.focus < len(items) {
			it := &items[v.focus]
			if it.key != nil && (it.enabled == nil || it.enabled()) {
				return it.key(msg)
			}
		}
	}
	return nil
}

// setCatalog installs a fresh catalog listing, preserving the current
// selection when it still exists and preselecting the session vendor
// otherwise.
func (v *messagesView) setCatalog(entries []msgfile.Entry) {
	curVendor := v.vendor()
	curFile := v.file()
	v.catalog = entries
	seen := map[string]bool{}
	v.vendors = nil
	for _, e := range entries {
		if !seen[e.Vendor] {
			seen[e.Vendor] = true
			v.vendors = append(v.vendors, e.Vendor)
		}
	}
	slices.Sort(v.vendors)
	v.vendorIdx = 0
	if i := slices.Index(v.vendors, curVendor); i >= 0 {
		v.vendorIdx = i
	} else if pre := v.preselect(); pre != "" {
		if i := slices.Index(v.vendors, pre); i >= 0 {
			v.vendorIdx = i
		}
	}
	files := v.vendorFiles()
	v.fileIdx = 0
	if i := slices.Index(files, curFile); i >= 0 {
		v.fileIdx = i
	}
}

// preselect returns the vendor to preselect: the probed receiver's
// vendor matched case-insensitively against the catalog.
func (v *messagesView) preselect() string {
	r := v.sess.Receiver()
	if !r.Info.IsSet() {
		return ""
	}
	vendor := strings.ToLower(r.Info.Get().Vendor)
	for _, name := range v.vendors {
		if strings.ToLower(name) == vendor {
			return name
		}
	}
	return ""
}

func (v *messagesView) vendor() string {
	if v.vendorIdx < len(v.vendors) {
		return v.vendors[v.vendorIdx]
	}
	return ""
}

func (v *messagesView) vendorFiles() []string {
	var files []string
	for _, e := range v.catalog {
		if e.Vendor == v.vendor() {
			files = append(files, e.File)
		}
	}
	slices.Sort(files)
	return files
}

func (v *messagesView) file() string {
	files := v.vendorFiles()
	if v.fileIdx < len(files) {
		return files[v.fileIdx]
	}
	return ""
}

func (v *messagesView) loadCmd() tea.Cmd {
	name := msgfile.Name{Vendor: v.vendor(), File: v.file()}
	if name.Vendor == "" || name.File == "" {
		return nil
	}
	sess, dirs := v.sess, v.msgDirs
	return func() tea.Msg {
		dir, fname, err := msgfile.FindName(name, dirs)
		if err != nil {
			return msgLoadedMsg{err: err}
		}
		mf, err := dir.Load(fname)
		if err != nil {
			return msgLoadedMsg{err: err}
		}
		return msgLoadedMsg{path: dir.DisplayPath(fname), tags: sess.SetMsgFile(mf)}
	}
}

// applyLoaded installs a loaded file: reset the send state and select
// a lone (or default) tag, as the workbench does.
func (v *messagesView) applyLoaded(path string, tags []session.MsgFileTag) {
	v.respSession = 0
	v.path = path
	v.tags = tags
	v.sendStatus = ""
	v.sendErr = false
	v.responses = nil
	v.respSel = -1
	v.activeTag = -1
	v.sel = 0
	v.armed = len(tags) == 1 || (len(tags) > 0 && tags[0].Tag == "")
}

func (v *messagesView) selTag() *session.MsgFileTag {
	if v.sel >= 0 && v.sel < len(v.tags) {
		return &v.tags[v.sel]
	}
	return nil
}

func (v *messagesView) port() string {
	if v.portIdx == 0 {
		return ""
	}
	return receiverPorts[v.portIdx]
}

// setDefaultPort seeds the port selector from the configuration
// readback without overwriting a user choice.
func (v *messagesView) setDefaultPort(port string) {
	if v.defaultPort == "" && v.portIdx == 0 {
		if i := slices.Index(receiverPorts, strings.ToUpper(port)); i > 0 {
			v.portIdx = i
		}
	}
	v.defaultPort = port
}

// canSend mirrors the workbench Send gating.
func (v *messagesView) canSend() bool {
	t := v.selTag()
	return v.connState == session.StateConnected && v.armed && t != nil &&
		v.path != "" && (!t.NeedsPort || v.port() != "")
}

func (v *messagesView) sendCmd() tea.Cmd {
	t := v.selTag()
	if !v.canSend() {
		return nil
	}
	v.activeTag = v.sel
	v.armed = false
	v.responses = nil
	v.respSel = -1
	v.sendStatus = ""
	port := ""
	if t.NeedsPort {
		port = v.port()
	}
	save := t.SaveAware && v.save
	sess, tag := v.sess, t.Tag
	return func() tea.Msg {
		return opResultMsg{op: "send", err: sess.SendMsgFile(tag, port, save)}
	}
}

// buildItems assembles the form; it is rebuilt per key/render because
// the tag rows and response rows change with state.
func (v *messagesView) buildItems() []cfgItem {
	var items []cfgItem
	items = append(items, itemHeading("Message file"))
	items = append(items, itemSelect("Vendor", func() []string { return v.vendors }, &v.vendorIdx, nil, func() {
		v.fileIdx = 0
	}))
	items = append(items, itemSelect("File", v.vendorFiles, &v.fileIdx, nil, nil))
	items = append(items, itemButton(func() string { return "Load" },
		func() bool { return v.vendor() != "" && v.file() != "" }, v.loadCmd))
	if v.path != "" {
		items = append(items, itemInfo(func() string { return "  " + v.path }))
	}
	if len(v.tags) > 0 {
		items = append(items, itemSpacer(), itemHeading("Tags"))
		for i := range v.tags {
			items = append(items, v.tagRow(i))
		}
		items = append(items, itemSpacer())
		anyPort := v.defaultPort != ""
		anySave := false
		for _, t := range v.tags {
			anyPort = anyPort || t.NeedsPort
			anySave = anySave || t.SaveAware
		}
		if anyPort {
			items = append(items, itemRadio("Port", receiverPorts, &v.portIdx,
				func() bool { t := v.selTag(); return t != nil && t.NeedsPort }, nil, nil))
		}
		if anySave {
			items = append(items, itemCheck("Save to non-volatile memory", &v.save,
				func() bool { t := v.selTag(); return t != nil && t.SaveAware }, nil))
		}
		items = append(items, itemButton(func() string { return "Send" }, v.canSend, v.sendCmd))
	}
	if len(v.responses) > 0 {
		items = append(items, itemSpacer(), itemHeading("Responses"))
		for i := range v.responses {
			items = append(items, v.responseRow(i))
		}
	}
	return items
}

// tagRow renders one tag-table row; enter selects and arms it.
func (v *messagesView) tagRow(i int) cfgItem {
	return cfgItem{
		enabled: func() bool { return true },
		render: func(focused bool) string {
			t := &v.tags[i]
			name := t.Tag
			if name == "" {
				name = "(default)"
			}
			active := " "
			if i == v.activeTag && !v.armed {
				active = "*"
			}
			line := fmt.Sprintf("%-20s %3d msgs  %s", name, t.MsgCount, t.Desc)
			sel := " "
			if i == v.sel && v.armed {
				sel = ">"
			}
			s := marker(focused) + sel + active + " " + line
			if i == v.sel && v.armed {
				return headerStyle.Render(s)
			}
			return s
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			if k.String() == "enter" || k.String() == "space" {
				v.respSession = 0
				v.responses = nil
				v.respSel = -1
				v.sendStatus = ""
				v.activeTag = -1
				v.sel = i
				v.armed = true
			}
			return nil
		},
	}
}

// responseRow renders one receiver response; enter selects it for the
// decode pane below.
func (v *messagesView) responseRow(i int) cfgItem {
	return cfgItem{
		enabled: func() bool { return true },
		render: func(focused bool) string {
			r := &v.responses[i]
			s := marker(focused) + formatResponse(r)
			if i == v.respSel && r.Kind != "ack" {
				s += "  " + faintStyle.Render("[selected]")
			}
			return s
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			if (k.String() == "enter" || k.String() == "space") && v.responses[i].Kind != "ack" {
				v.respSel = i
			}
			return nil
		},
	}
}

// formatResponse mirrors the workbench response-line rendering.
func formatResponse(r *session.ResponseEvent) string {
	if r.Kind == "ack" {
		prefix := "Message"
		if r.MsgCount > 1 {
			prefix = fmt.Sprintf("Message %d", r.ResponseTo+1)
		}
		if r.AckError == "" {
			return prefix + " accepted"
		}
		return errorStyle.Render(prefix + " rejected: " + r.AckError)
	}
	var s string
	switch {
	case r.Text != "":
		s = r.Text
	case r.Tag != "" && r.MsgID != "":
		s = r.Tag + "-" + r.MsgID
	case r.Tag != "":
		s = r.Tag
	case r.Bin != "":
		s = r.Bin
	default:
		s = "Response"
	}
	if r.Kind == "maybe" {
		return faintStyle.Render(s)
	}
	return s
}

func (v *messagesView) render(width, height int) string {
	items := v.buildItems()
	if v.focus >= len(items) {
		v.focus = 0
		moveItemFocus(items, &v.focus, 1)
	}
	var extra []string
	if v.sendStatus != "" {
		s := v.sendStatus
		if v.sendErr {
			s = errorStyle.Render(s)
		} else {
			s = faintStyle.Render(s)
		}
		extra = append(extra, "", s)
	}
	if d := v.renderDecode(); d != "" {
		extra = append(extra, "", labelStyle.Render("Decode"))
		extra = append(extra, strings.Split(d, "\n")...)
	}
	return renderItems(items, v.focus, &v.offset, width, height, extra...)
}

// renderDecode decodes the selected response, as the workbench decode
// pane does.
func (v *messagesView) renderDecode() string {
	if v.respSel < 0 || v.respSel >= len(v.responses) {
		return ""
	}
	r := &v.responses[v.respSel]
	data := []byte(r.Text)
	if r.Bin != "" {
		b, err := hex.DecodeString(r.Bin)
		if err != nil {
			return ""
		}
		data = b
	}
	if len(data) == 0 {
		return ""
	}
	rslt, err := session.DecodePacket(v.formats, data, false)
	if err == nil && rslt != nil {
		var out any = rslt
		if rslt.Header == nil && rslt.CfgData == nil && rslt.Payload != nil {
			out = rslt.Payload
		}
		if b, err := json.MarshalIndent(out, "", "  "); err == nil {
			return string(b)
		}
	}
	return r.Text
}
