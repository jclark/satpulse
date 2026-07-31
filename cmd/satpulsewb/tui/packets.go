package tui

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
)

const (
	// burstGap bounds the recent entries a row retains: on each new
	// packet, entries older than this relative to it are pruned, so a
	// row keeps the current burst, typically the current epoch.
	// Matches the workbench packet panel's EPOCH_GAP_MS.
	burstGap = 900 * time.Millisecond
	// activeWindow is how recently a row's last packet must have
	// arrived for the row to render as active rather than dimmed.
	activeWindow = 1500 * time.Millisecond
)

// pktRow aggregates the packets of one (protocol, message, direction),
// as in the workbench packet panel: a running count and the entries of
// the current burst.
type pktRow struct {
	tag    string
	msg    string
	out    bool
	count  int
	recent []gpsio.PacketLogEntry // current burst, newest last
}

func pktKey(tag, msg string, out bool) string {
	dir := "i"
	if out {
		dir = "o"
	}
	return tag + ":" + msg + ":" + dir
}

// pktItem addresses one display row: a parent row by key, or one
// burst entry of an expanded row (child >= 0).
type pktItem struct {
	key   string
	child int // -1 for the parent row
}

// snapItem is one entry of a snapshot: the row identity plus the
// captured packet.
type snapItem struct {
	tag string
	msg string
	out bool
	e   gpsio.PacketLogEntry
}

// packetsView is the Packets tab, following the workbench packet
// panel: one aggregated row per (protocol, message, direction) with
// count and recent burst, freeze, snapshot, clear, and a decode pane
// for the selected entry.
type packetsView struct {
	sess    *session.Session
	formats []gpsprot.PacketFormat

	live      map[string]*pktRow
	frozen    map[string]*pktRow // display copy while frozen, else nil
	frozenAt  time.Time
	expanded  map[string]bool
	sel       pktItem // sel.key == "" means no selection
	offset    int
	prevState session.ConnState

	snap    []snapItem // non-nil while the snapshot dialog is open
	snapOff int
}

func newPacketsView(sess *session.Session) *packetsView {
	return &packetsView{
		sess:      sess,
		formats:   gpsreg.CreatePacketFormats(nil),
		live:      make(map[string]*pktRow),
		expanded:  make(map[string]bool),
		sel:       pktItem{child: -1},
		prevState: sess.State(),
	}
}

func (v *packetsView) title() string { return "Packets" }

func (v *packetsView) capturesInput() bool { return false }

func (v *packetsView) hints() []string {
	if v.snap != nil {
		return []string{"up/down scroll", "r refresh", "esc close"}
	}
	freeze := "f freeze"
	if v.frozen != nil {
		freeze = "f unfreeze"
	}
	return []string{"up/down select", "space expand", freeze, "s snapshot", "c clear"}
}

func (v *packetsView) handleEvent(ev session.Event) {
	switch ev.Name {
	case session.EventPacket:
		if e, ok := ev.Data.(gpsio.PacketLogEntry); ok {
			v.addPacket(e)
		}
	case session.EventState:
		if st, ok := ev.Data.(session.ConnState); ok {
			// A new connection starting clears the previous stream,
			// as in the workbench.
			if v.prevState == session.StateDisconnected && st != session.StateDisconnected {
				v.clear()
			}
			v.prevState = st
		}
	}
}

// addPacket accumulates into the live rows; the display copy is
// unaffected while frozen.
func (v *packetsView) addPacket(e gpsio.PacketLogEntry) {
	tag := string(e.Tag)
	key := pktKey(tag, e.Msg, e.Out)
	r := v.live[key]
	if r == nil {
		r = &pktRow{tag: tag, msg: e.Msg, out: e.Out}
		v.live[key] = r
	}
	r.count++
	t := time.Time(e.T)
	for len(r.recent) > 0 && t.Sub(time.Time(r.recent[0].T)) > burstGap {
		r.recent = r.recent[1:]
	}
	r.recent = append(r.recent, e)
}

// rows returns the display map: the frozen copy while frozen, else
// the live map.
func (v *packetsView) rows() map[string]*pktRow {
	if v.frozen != nil {
		return v.frozen
	}
	return v.live
}

// now returns the instant dimming and snapshots are computed against:
// the freeze instant while frozen, so ages stop advancing.
func (v *packetsView) now() time.Time {
	if v.frozen != nil {
		return v.frozenAt
	}
	return time.Now()
}

// sortedKeys returns the display row keys in the workbench order:
// outbound first, then by protocol, then by message.
func (v *packetsView) sortedKeys() []string {
	rows := v.rows()
	keys := slices.Collect(maps.Keys(rows))
	slices.SortFunc(keys, func(a, b string) int {
		ra, rb := rows[a], rows[b]
		if ra.out != rb.out {
			if ra.out {
				return -1
			}
			return 1
		}
		if c := strings.Compare(ra.tag, rb.tag); c != 0 {
			return c
		}
		return strings.Compare(ra.msg, rb.msg)
	})
	return keys
}

// items returns the flattened display rows: each parent, then its
// burst entries when expanded.
func (v *packetsView) items() []pktItem {
	var items []pktItem
	rows := v.rows()
	for _, key := range v.sortedKeys() {
		items = append(items, pktItem{key: key, child: -1})
		if v.expanded[key] && len(rows[key].recent) > 1 {
			for i := range rows[key].recent {
				items = append(items, pktItem{key: key, child: i})
			}
		}
	}
	return items
}

// toggleFreeze freezes a deep copy of the live rows for display, or
// re-syncs to live; accumulation continues in the background either
// way.
func (v *packetsView) toggleFreeze() {
	if v.frozen != nil {
		v.frozen = nil
		return
	}
	frozen := make(map[string]*pktRow, len(v.live))
	for key, r := range v.live {
		c := *r
		c.recent = slices.Clone(r.recent)
		frozen[key] = &c
	}
	v.frozen = frozen
	v.frozenAt = time.Now()
}

// takeSnapshot captures the currently active entries across all
// display rows, sorted by receive time.
func (v *packetsView) takeSnapshot() {
	now := v.now()
	var snap []snapItem
	for _, r := range v.rows() {
		for _, e := range r.recent {
			if now.Sub(time.Time(e.T)) < activeWindow {
				snap = append(snap, snapItem{tag: r.tag, msg: r.msg, out: r.out, e: e})
			}
		}
	}
	slices.SortFunc(snap, func(a, b snapItem) int {
		return time.Time(a.e.T).Compare(time.Time(b.e.T))
	})
	v.snap = snap
	v.snapOff = 0
}

func (v *packetsView) clear() {
	v.live = make(map[string]*pktRow)
	v.frozen = nil
	clear(v.expanded)
	v.sel = pktItem{child: -1}
	v.snap = nil
}

func (v *packetsView) update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if v.snap != nil {
		switch key.String() {
		case "up":
			v.snapOff--
		case "down":
			v.snapOff++
		case "r":
			v.takeSnapshot()
		case "esc", "q":
			v.snap = nil
		}
		return nil
	}
	switch key.String() {
	case "up":
		v.moveSel(-1)
	case "down":
		v.moveSel(1)
	case "space", "right", "left", "enter":
		v.toggleExpand(key.String())
	case "f":
		v.toggleFreeze()
	case "s":
		v.takeSnapshot()
	case "c":
		v.clear()
	case "esc":
		v.sel = pktItem{child: -1}
	}
	return nil
}

func (v *packetsView) moveSel(dir int) {
	items := v.items()
	if len(items) == 0 {
		return
	}
	i := slices.Index(items, v.sel)
	if i < 0 {
		// No or stale selection: enter the list at its ends.
		if dir > 0 {
			v.sel = items[0]
		} else {
			v.sel = items[len(items)-1]
		}
		return
	}
	i += dir
	if i >= 0 && i < len(items) {
		v.sel = items[i]
	}
}

// toggleExpand opens or closes the selected row's burst listing.
func (v *packetsView) toggleExpand(key string) {
	if v.sel.key == "" {
		return
	}
	rowKey := v.sel.key
	switch key {
	case "left":
		if v.expanded[rowKey] {
			delete(v.expanded, rowKey)
			v.sel = pktItem{key: rowKey, child: -1}
		}
	case "right":
		if r := v.rows()[rowKey]; r != nil && len(r.recent) > 1 {
			v.expanded[rowKey] = true
		}
	default: // space, enter
		if v.expanded[rowKey] {
			delete(v.expanded, rowKey)
			v.sel = pktItem{key: rowKey, child: -1}
		} else if r := v.rows()[rowKey]; r != nil && len(r.recent) > 1 {
			v.expanded[rowKey] = true
		}
	}
}

// selEntry returns the packet the selection decodes: a child selects
// its burst entry, a parent its newest entry.
func (v *packetsView) selEntry() *gpsio.PacketLogEntry {
	r := v.rows()[v.sel.key]
	if r == nil || len(r.recent) == 0 {
		return nil
	}
	if v.sel.child >= 0 && v.sel.child < len(r.recent) {
		return &r.recent[v.sel.child]
	}
	return &r.recent[len(r.recent)-1]
}

func (v *packetsView) render(width, height int) string {
	if v.snap != nil {
		return v.renderSnapshot(width, height)
	}
	if len(v.rows()) == 0 {
		return faintStyle.Render("No packets (connect to a receiver to stream packets)")
	}
	// The table takes the upper part, the decode pane of the selected
	// entry the lower, standing in for the workbench's decode pane
	// beside the table.
	decodeH := max(min(height/3, 14), 4)
	tableH := height - decodeH - 1
	if tableH < 3 {
		tableH = height
		decodeH = 0
	}
	lines := v.renderTableLines(width, tableH)
	if decodeH == 0 {
		return strings.Join(lines, "\n")
	}
	out := append(lines, labelStyle.Render(strings.Repeat("-", max(width/2, 1))))
	out = append(out, v.renderDecode(width, decodeH)...)
	return strings.Join(out, "\n")
}

func (v *packetsView) renderTableLines(width, height int) []string {
	rows := v.rows()
	items := v.items()
	now := v.now()
	tagW, msgW, countW := len("Protocol"), len("Message"), len("Count")
	for _, r := range rows {
		tagW = max(tagW, len(r.tag))
		msgW = max(msgW, len(r.msg))
		countW = max(countW, len(fmt.Sprintf("%d", r.count)))
	}
	msgW = min(msgW, 24)
	header := fmt.Sprintf("  %-*s  %-*s  %-2s  %*s  %-12s  %s",
		tagW, "Protocol", msgW, "Message", "", countW, "Count", "Last time", "Last message")
	body := make([]string, 0, len(items))
	selLine := -1
	for _, it := range items {
		r := rows[it.key]
		var line string
		if it.child < 0 {
			last := r.recent[len(r.recent)-1]
			exp := " "
			if len(r.recent) > 1 {
				if v.expanded[it.key] {
					exp = "-"
				} else {
					exp = "+"
				}
			}
			dir := "Rx"
			if r.out {
				dir = "Tx"
			}
			line = fmt.Sprintf("%s %-*s  %-*s  %s  %*d  %s  %s",
				exp, tagW, r.tag, msgW, r.msg, dir, countW, r.count,
				time.Time(last.T).Format("15:04:05.000"), packetPreview(&last))
		} else {
			e := r.recent[it.child]
			line = fmt.Sprintf("      %s  %s", time.Time(e.T).Format("15:04:05.000"), packetPreview(&e))
		}
		line = truncate(line, width)
		switch {
		case it == v.sel:
			selLine = len(body)
			line = selStyle.Render(line)
		case it.child < 0 && now.Sub(time.Time(r.recent[len(r.recent)-1].T)) >= activeWindow:
			line = faintStyle.Render(line)
		}
		body = append(body, line)
	}
	height-- // header line
	if selLine >= 0 {
		if selLine < v.offset {
			v.offset = selLine
		}
		if selLine >= v.offset+height {
			v.offset = selLine - height + 1
		}
	}
	if v.offset > len(body)-height {
		v.offset = len(body) - height
	}
	if v.offset < 0 {
		v.offset = 0
	}
	head := labelStyle.Render(truncate(header, width))
	if v.frozen != nil {
		head += "  " + headerStyle.Render("[frozen]")
	}
	end := min(v.offset+height, len(body))
	return append([]string{head}, body[v.offset:end]...)
}

func (v *packetsView) renderDecode(width, height int) []string {
	e := v.selEntry()
	if e == nil {
		return []string{faintStyle.Render("Select a row to decode")}
	}
	head := fmt.Sprintf("%s %s %s %s", time.Time(e.T).Format("15:04:05.000000"),
		map[bool]string{false: "Rx", true: "Tx"}[e.Out], e.Tag, e.Msg)
	lines := append([]string{headerStyle.Render(truncate(head, width))},
		strings.Split(v.decode(e), "\n")...)
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, l := range lines {
		lines[i] = truncate(l, width)
	}
	return lines
}

func (v *packetsView) renderSnapshot(width, height int) string {
	lines := []string{headerStyle.Render(fmt.Sprintf("Snapshot: %d active packets", len(v.snap)))}
	for _, s := range v.snap {
		dir := "Rx"
		if s.out {
			dir = "Tx"
		}
		lines = append(lines, truncate(fmt.Sprintf("%s  %s  %-8s %-16s %s",
			time.Time(s.e.T).Format("15:04:05.000"), dir, s.tag, s.msg, packetPreview(&s.e)), width))
	}
	if v.snapOff > len(lines)-height {
		v.snapOff = len(lines) - height
	}
	if v.snapOff < 0 {
		v.snapOff = 0
	}
	end := min(v.snapOff+height, len(lines))
	return strings.Join(lines[v.snapOff:end], "\n")
}

// packetPreview renders the packet data for a table cell: cleaned
// ASCII for text packets, hex for binary.
func packetPreview(e *gpsio.PacketLogEntry) string {
	if len(e.Bin) > 0 {
		return fmt.Sprintf("%x", []byte(e.Bin))
	}
	s := strings.TrimRight(e.Ascii, "\r\n")
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
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
