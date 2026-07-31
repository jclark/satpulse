package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// This file provides the shared form-row machinery used by the
// Configuration and Messages views: a form is a []cfgItem navigated
// with up/down, each item rendering one line and handling its own
// keys.

func marker(focused bool) string {
	if focused {
		return "> "
	}
	return "  "
}

// itemStyle dims a row whose enabled predicate reports false.
func itemStyle(enabled func() bool, s string) string {
	if enabled != nil && !enabled() {
		return faintStyle.Render(s)
	}
	return s
}

func itemHeading(text string) cfgItem {
	return cfgItem{render: func(bool) string { return sectionStyle.Render(text) }}
}

func itemSpacer() cfgItem {
	return cfgItem{render: func(bool) string { return "" }}
}

func itemInfo(text func() string) cfgItem {
	return cfgItem{render: func(bool) string { return faintStyle.Render(text()) }}
}

func itemCheck(label string, val *bool, enabled func() bool, onChange func()) cfgItem {
	return cfgItem{
		enabled: enabled,
		render: func(focused bool) string {
			box := "[ ]"
			if *val {
				box = "[x]"
			}
			return marker(focused) + itemStyle(enabled, box+" "+label)
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			if k.String() == "space" || k.String() == "enter" {
				*val = !*val
				if onChange != nil {
					onChange()
				}
			}
			return nil
		},
	}
}

func itemText(label string, ti *textinput.Model, enabled func() bool, onChange func()) cfgItem {
	return cfgItem{
		enabled:  enabled,
		captures: true,
		render: func(focused bool) string {
			if focused {
				ti.Focus()
			} else {
				ti.Blur()
			}
			return marker(focused) + itemStyle(enabled, labelStyle.Render(fmt.Sprintf("%-24s", label))+ti.View())
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			before := ti.Value()
			var cmd tea.Cmd
			*ti, cmd = ti.Update(k)
			if ti.Value() != before && onChange != nil {
				onChange()
			}
			return cmd
		},
	}
}

// itemRadio renders a one-line radio group cycled with left/right;
// optionOK disables individual options.
func itemRadio(label string, options []string, idx *int, enabled func() bool, optionOK func(int) bool, onChange func()) cfgItem {
	return cfgItem{
		enabled: enabled,
		render: func(focused bool) string {
			var b strings.Builder
			b.WriteString(marker(focused))
			b.WriteString(labelStyle.Render(fmt.Sprintf("%-24s", label)))
			for i, o := range options {
				mark := "( ) "
				if i == *idx {
					mark = "(x) "
				}
				s := mark + o
				if optionOK != nil && !optionOK(i) {
					s = faintStyle.Render(s)
				}
				b.WriteString(s)
				if i != len(options)-1 {
					b.WriteString("  ")
				}
			}
			return itemStyle(enabled, b.String())
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			dir := 0
			switch k.String() {
			case "left":
				dir = -1
			case "right", "space", "enter":
				dir = 1
			default:
				return nil
			}
			n := len(options)
			for i := 1; i <= n; i++ {
				j := (*idx + dir*i + n*n) % n
				if optionOK == nil || optionOK(j) {
					if j != *idx {
						*idx = j
						if onChange != nil {
							onChange()
						}
					}
					return nil
				}
			}
			return nil
		},
	}
}

// itemSelect renders a compact select showing only the current
// option, cycled with left/right.
func itemSelect(label string, options func() []string, idx *int, enabled func() bool, onChange func()) cfgItem {
	return cfgItem{
		enabled: enabled,
		render: func(focused bool) string {
			opts := options()
			cur := "-"
			if *idx >= 0 && *idx < len(opts) {
				cur = opts[*idx]
			}
			return marker(focused) + itemStyle(enabled, labelStyle.Render(fmt.Sprintf("%-24s", label))+"< "+cur+" >")
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			opts := options()
			if len(opts) == 0 {
				return nil
			}
			var dir int
			switch k.String() {
			case "left":
				dir = -1
			case "right", "space", "enter":
				dir = 1
			default:
				return nil
			}
			*idx = (*idx + dir + len(opts)) % len(opts)
			if onChange != nil {
				onChange()
			}
			return nil
		},
	}
}

func itemButton(label func() string, enabled func() bool, action func() tea.Cmd) cfgItem {
	return cfgItem{
		enabled: enabled,
		render: func(focused bool) string {
			s := "[ " + label() + " ]"
			if focused {
				s = activeTab.Render(s)
			}
			return marker(focused) + itemStyle(enabled, s)
		},
		key: func(k tea.KeyPressMsg) tea.Cmd {
			if k.String() == "enter" || k.String() == "space" {
				return action()
			}
			return nil
		},
	}
}

// moveItemFocus advances to the next focusable, enabled item.
func moveItemFocus(items []cfgItem, focus *int, dir int) {
	n := len(items)
	if n == 0 {
		return
	}
	for i := 1; i <= n; i++ {
		j := (*focus + dir*i + n*n) % n
		it := &items[j]
		if it.key != nil && (it.enabled == nil || it.enabled()) {
			*focus = j
			return
		}
	}
}

// renderItems renders a form, keeping the focused item visible within
// height; extra lines are appended after the items.
func renderItems(items []cfgItem, focus int, offset *int, width, height int, extra ...string) string {
	lines := make([]string, 0, len(items)+len(extra))
	focusLine := 0
	for i, it := range items {
		if i == focus {
			focusLine = len(lines)
		}
		lines = append(lines, it.render(i == focus))
	}
	lines = append(lines, extra...)
	if focusLine < *offset {
		*offset = focusLine
	}
	if focusLine >= *offset+height {
		// Scrolling down, keep a third of the window below the focus
		// so content attached to it (a decode, a status) stays
		// visible.
		*offset = focusLine - max(height-1-height/3, 0)
	}
	if *offset > len(lines)-height {
		*offset = len(lines) - height
	}
	if *offset < 0 {
		*offset = 0
	}
	end := min(*offset+height, len(lines))
	out := make([]string, 0, end-*offset)
	for _, l := range lines[*offset:end] {
		out = append(out, truncate(l, width))
	}
	return strings.Join(out, "\n")
}
