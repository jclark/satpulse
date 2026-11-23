// Package soc defines the interface between gpiomem and the packages that
// describe the GPIO controllers of a family of SoCs: how the name of a GPIO
// identifies a controller and a line, and where among the controller's
// registers the line's level is read. Each family package is pure: it reads
// nothing from the system, and is given what gpiomem finds there.
package soc

import (
	"strconv"
	"strings"
)

// Controller is a GPIO controller on the running system: a gpiochip and the
// device-tree node it is bound to.
type Controller struct {
	Chip       string   // the gpiochip's device name, e.g. "gpiochip0"
	Compatible []string // the compatible strings of its device-tree node
}

// SoC describes the GPIO controllers of a family of SoCs, following the
// kernel driver that handles them.
type SoC interface {
	// Compatible returns the device-tree compatible strings of the GPIO
	// controllers the family handles.
	Compatible() []string
	// Lookup finds the GPIO with the given name, which is upper-case
	// ASCII, among ctrls, the controllers on the system that the family
	// handles. It returns the index of the controller in ctrls and the
	// GPIO's line on that controller.
	Lookup(name string, ctrls []Controller) (ctrl int, line int, err error)
	// Level locates the register holding the level of line on ctrl: its
	// offset from the start of the controller's first register range, and
	// the line's bit in it.
	Level(ctrl Controller, line int) (offset int, bit uint)
	// GPIOMem returns the path of the gpiomem device that maps the
	// controller's registers from the start of its first register range,
	// and the length of the device's window, which a mapping must cover.
	GPIOMem() (device string, length int)
}

// GPIONumber parses a name of the form GPIO<n>, used by SoCs that number
// their GPIOs from 0 across all banks, such as the Raspberry Pi's. The name
// is upper-case and n is decimal without leading zeros.
func GPIONumber(name string) (int, bool) {
	s, ok := strings.CutPrefix(name, "GPIO")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || strconv.Itoa(n) != s {
		return 0, false
	}
	return n, true
}
