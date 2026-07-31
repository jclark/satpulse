package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// truncate cuts a styled line to width columns, appending nothing;
// ANSI escapes are preserved.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "")
}

// kv renders a label: value pair for a detail listing, with the label
// padded to labelWidth.
func kv(labelWidth int, label, value string) string {
	return labelStyle.Render(fmt.Sprintf("%*s", labelWidth, label)) + "  " + value
}

// cell is one table cell; bold marks values the receiver reported
// natively, as in the workbench PVT tables (plain means derived).
type cell struct {
	text string
	bold bool
}

// renderTable renders a header row and body rows with columns sized to
// their content. right marks right-aligned columns. Lines are not
// truncated; the caller owns width handling.
func renderTable(headers []string, right []bool, rows [][]cell) []string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && len(c.text) > widths[i] {
				widths[i] = len(c.text)
			}
		}
	}
	pad := func(s string, i int) string {
		if right != nil && i < len(right) && right[i] {
			return fmt.Sprintf("%*s", widths[i], s)
		}
		return fmt.Sprintf("%-*s", widths[i], s)
	}
	var lines []string
	var b strings.Builder
	for i, h := range headers {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(labelStyle.Render(pad(h, i)))
	}
	lines = append(lines, b.String())
	bold := lipgloss.NewStyle().Bold(true)
	for _, r := range rows {
		b.Reset()
		for i, c := range r {
			if i > 0 {
				b.WriteString("  ")
			}
			s := pad(c.text, i)
			if c.bold {
				s = bold.Render(s)
			}
			b.WriteString(s)
		}
		lines = append(lines, b.String())
	}
	return lines
}

// bar renders a text bar chart cell of the given width for a value in
// [0, max].
func bar(value, max, width int) string {
	if max <= 0 || width <= 0 {
		return ""
	}
	n := value * width / max
	if n > width {
		n = width
	}
	if n < 0 {
		n = 0
	}
	return strings.Repeat("#", n) + strings.Repeat(" ", width-n)
}
