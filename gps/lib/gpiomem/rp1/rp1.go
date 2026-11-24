// Package rp1 describes the GPIO controller of the RP1, the I/O chip of the
// Raspberry Pi 5 and Compute Module 5, as the kernel's pinctrl-rp1 driver
// handles it. GPIOs are named GPIO<n>, numbered from 0 across the
// controller's three banks.
package rp1

import (
	"fmt"
	"strconv"

	"github.com/jclark/satpulse/gps/lib/gpiomem/soc"
)

// SoC describes the RP1's GPIO controller.
type SoC struct{}

const gpioCount = 54

// Family returns "rp1".
func (SoC) Family() string {
	return "rp1"
}

// Compatible returns the compatible string of the RP1's GPIO controller.
func (SoC) Compatible() []string {
	return []string{"raspberrypi,rp1-gpio"}
}

// Lookup finds GPIO<n> on the system's one RP1 GPIO controller.
func (SoC) Lookup(name string, ctrls []soc.Controller) (int, int, error) {
	n, ok := soc.GPIONumber(name)
	if !ok {
		return 0, 0, fmt.Errorf("expected a name of the form GPIO<n>")
	}
	if len(ctrls) != 1 {
		return 0, 0, fmt.Errorf("found %d RP1 GPIO controllers, expected one", len(ctrls))
	}
	if n >= gpioCount {
		return 0, 0, fmt.Errorf("out of range; the controller has GPIO0 to GPIO%d", gpioCount-1)
	}
	return 0, n, nil
}

// Name returns GPIO<line>.
func (SoC) Name(ctrls []soc.Controller, ctrl int, line int) (string, bool) {
	if line < 0 || line >= gpioCount {
		return "", false
	}
	return "GPIO" + strconv.Itoa(line), true
}

// Level locates line in the RIO_IN registers, the registers the kernel's
// driver reads for a GPIO's level. They are in the controller's second
// register range, 0x10000 after the first. The GPIOs are in three banks of
// 28, 6 and 20; each bank's registers are 0x4000 apart, with bank 0's
// RIO_IN at offset 8.
func (SoC) Level(ctrl soc.Controller, line int) (int, uint) {
	bank, base := 0, 0
	if line >= 34 {
		bank, base = 2, 34
	} else if line >= 28 {
		bank, base = 1, 28
	}
	return 0x10008 + bank*0x4000, uint(line - base)
}

// GPIOMem returns the gpiomem device of the Raspberry Pi 5, which maps the
// RP1 GPIO controller's registers, and the length of its window.
func (SoC) GPIOMem() (string, int) {
	return "/dev/gpiomem0", 0x30000
}
