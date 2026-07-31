package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
)

// maxPacketRows bounds the retained packet list.
const maxPacketRows = 2000

// packetRow is one retained packet, or a marker for packets dropped
// from the pending queue (dropped > 0).
type packetRow struct {
	entry   gpsio.PacketLogEntry
	dropped int
}

// packetsView is the Packets tab: a scrolling packet list with a
// decode detail view for the selected packet.
type packetsView struct {
	sess    *session.Session
	formats []gpsprot.PacketFormat

	rows   []packetRow
	total  int // rows ever received, for stable selection while trimming
	paused bool
	// sel is the selected row index, or -1 to follow the newest row.
	sel    int
	detail bool // detail mode: the decode result fills the view
	detOff int  // detail scroll offset
	offset int  // list scroll offset
}

func newPacketsView(sess *session.Session) *packetsView {
	return &packetsView{sess: sess, formats: gpsreg.CreatePacketFormats(nil), sel: -1}
}

func (v *packetsView) title() string { return "Packets" }

func (v *packetsView) capturesInput() bool { return false }

func (v *packetsView) hints() []string {
	if v.detail {
		return []string{"up/down scroll", "esc back"}
	}
	pause := "p pause"
	if v.paused {
		pause = "p resume"
	}
	return []string{"up/down select", "enter detail", pause, "c clear", "esc follow"}
}

func (v *packetsView) handleEvent(ev session.Event) {
	switch ev.Name {
	case session.EventPacket:
		if n, ok := ev.Data.(droppedPackets); ok {
			v.append(packetRow{dropped: int(n)})
			return
		}
		if e, ok := ev.Data.(gpsio.PacketLogEntry); ok {
			v.append(packetRow{entry: e})
		}
	case session.EventState:
		if st, ok := ev.Data.(session.ConnState); ok && st == session.StateConnecting {
			// A new connection starting clears the previous stream.
			v.rows = nil
			v.sel = -1
			v.detail = false
		}
	}
}

func (v *packetsView) append(r packetRow) {
	v.rows = append(v.rows, r)
	v.total++
	if len(v.rows) > maxPacketRows {
		trim := len(v.rows) - maxPacketRows
		v.rows = v.rows[trim:]
		if v.sel >= 0 {
			v.sel -= trim
			if v.sel < 0 {
				v.sel = 0
			}
		}
	}
}

func (v *packetsView) update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if v.detail {
		switch key.String() {
		case "up":
			v.detOff--
		case "down":
			v.detOff++
		case "pgup":
			v.detOff -= 10
		case "pgdown":
			v.detOff += 10
		case "esc", "enter":
			v.detail = false
		}
		return nil
	}
	switch key.String() {
	case "up":
		if v.sel < 0 {
			v.sel = len(v.rows) - 1
		}
		if v.sel > 0 {
			v.sel--
		}
	case "down":
		if v.sel >= 0 && v.sel < len(v.rows)-1 {
			v.sel++
		}
	case "esc":
		v.sel = -1
	case "enter":
		if v.selRow() != nil {
			v.detail = true
			v.detOff = 0
		}
	case "p":
		v.paused = !v.paused
	case "c":
		v.rows = nil
		v.sel = -1
	}
	return nil
}

func (v *packetsView) selRow() *packetRow {
	i := v.sel
	if i < 0 {
		i = len(v.rows) - 1
	}
	if i < 0 || i >= len(v.rows) || v.rows[i].dropped > 0 {
		return nil
	}
	return &v.rows[i]
}

func (v *packetsView) render(width, height int) string {
	if v.detail {
		return v.renderDetail(width, height)
	}
	if len(v.rows) == 0 {
		return faintStyle.Render("No packets (connect to a receiver to stream packets)")
	}
	sel := v.sel
	if sel < 0 {
		sel = len(v.rows) - 1
	}
	// Keep the selection in the window; when following and not
	// paused, this pins the view to the newest row.
	follow := v.sel < 0
	if !follow || !v.paused {
		if sel < v.offset {
			v.offset = sel
		}
		if sel >= v.offset+height {
			v.offset = sel - height + 1
		}
	}
	if v.offset > len(v.rows)-height {
		v.offset = len(v.rows) - height
	}
	if v.offset < 0 {
		v.offset = 0
	}
	var b strings.Builder
	end := min(v.offset+height, len(v.rows))
	for i := v.offset; i < end; i++ {
		line := v.renderRow(&v.rows[i])
		if i == sel && v.sel >= 0 {
			line = activeTab.Render(truncate(line, width))
		} else {
			line = truncate(line, width)
		}
		b.WriteString(line)
		if i != end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (v *packetsView) renderRow(r *packetRow) string {
	if r.dropped > 0 {
		return faintStyle.Render(fmt.Sprintf("... %d packets dropped", r.dropped))
	}
	e := &r.entry
	dir := "Rx"
	if e.Out {
		dir = "Tx"
	}
	tag := string(e.Tag)
	if tag == "" {
		tag = "?"
	}
	return fmt.Sprintf("%s %s %-8s %-16s %s",
		time.Time(e.T).Format("15:04:05.000"), dir, tag, e.Msg, packetPreview(e))
}

// packetPreview renders the packet data for the list: cleaned ASCII
// for text packets, hex for binary.
func packetPreview(e *gpsio.PacketLogEntry) string {
	if len(e.Bin) > 0 {
		return fmt.Sprintf("%x", []byte(e.Bin))
	}
	s := strings.TrimRight(e.Ascii, "\r\n")
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
}

func (v *packetsView) renderDetail(width, height int) string {
	r := v.selRow()
	if r == nil {
		v.detail = false
		return ""
	}
	e := &r.entry
	head := fmt.Sprintf("%s %s %s %s", time.Time(e.T).Format("15:04:05.000000"),
		map[bool]string{false: "Rx", true: "Tx"}[e.Out], e.Tag, e.Msg)
	body := v.decode(e)
	lines := append([]string{headerStyle.Render(truncate(head, width)), ""},
		strings.Split(body, "\n")...)
	if v.detOff > len(lines)-height {
		v.detOff = len(lines) - height
	}
	if v.detOff < 0 {
		v.detOff = 0
	}
	end := min(v.detOff+height, len(lines))
	out := make([]string, 0, end-v.detOff)
	for _, l := range lines[v.detOff:end] {
		out = append(out, truncate(l, width))
	}
	return strings.Join(out, "\n")
}

// decode runs the packet through session.DecodePacket and renders the
// result as indented JSON (just the payload when that is the only
// field, as in the workbench); undecodable packets fall back to the
// raw data.
func (v *packetsView) decode(e *gpsio.PacketLogEntry) string {
	data := []byte(e.Bin)
	if len(data) == 0 {
		data = []byte(e.Ascii)
	}
	r, err := session.DecodePacket(v.formats, data, e.Out)
	if err == nil && r != nil {
		var out any = r
		if r.Header == nil && r.CfgData == nil && r.Payload != nil {
			out = r.Payload
		}
		if b, err := json.MarshalIndent(out, "", "  "); err == nil {
			return string(b)
		}
	}
	return packetPreview(e)
}
