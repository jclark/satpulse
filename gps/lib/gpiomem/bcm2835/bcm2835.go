// Package bcm2835 is the gpiomem backend for the GPIO controller of the
// Broadcom BCM2835 family, as the kernel's pinctrl-bcm2835 driver handles it:
// the BCM2837 of the Raspberry Pi 3 and the BCM2711 of the Raspberry Pi 4 and
// Compute Module 4. GPIOs are named GPIO<n>, numbered from 0 across the controller.
package bcm2835

import (
	"fmt"
	"strconv"

	"github.com/jclark/satpulse/gps/lib/gpiomem/backend"
)

// Backend handles the BCM2835 family's GPIO controller.
type Backend struct{}

// variants lists the compatible strings the family handles, with the number
// of GPIOs each has. The BCM2837 uses the BCM2835's compatible string.
var variants = []struct {
	compatible string
	count      int
}{
	{"brcm,bcm2835-gpio", 54},
	{"brcm,bcm2711-gpio", 58},
}

// Name returns "bcm2835".
func (Backend) Name() string {
	return "bcm2835"
}

// Compatible returns the compatible strings of the family's controllers.
func (Backend) Compatible() []string {
	s := make([]string, len(variants))
	for i, v := range variants {
		s[i] = v.compatible
	}
	return s
}

// Lookup finds GPIO<n> on the system's one BCM2835-family controller.
func (Backend) Lookup(name string, ctrls []backend.Controller) (int, int, error) {
	n, ok := backend.GPIONumber(name)
	if !ok {
		return 0, 0, fmt.Errorf("expected a name of the form GPIO<n>")
	}
	if len(ctrls) != 1 {
		return 0, 0, fmt.Errorf("found %d BCM2835 GPIO controllers, expected one", len(ctrls))
	}
	if count := gpioCount(ctrls[0]); n >= count {
		return 0, 0, fmt.Errorf("out of range; the controller has GPIO0 to GPIO%d", count-1)
	}
	return 0, n, nil
}

// GPIOName returns GPIO<line>.
func (Backend) GPIOName(ctrls []backend.Controller, ctrl int, line int) (string, bool) {
	if len(ctrls) != 1 || line < 0 || line >= gpioCount(ctrls[ctrl]) {
		return "", false
	}
	return "GPIO" + strconv.Itoa(line), true
}

// gpioCount returns the number of GPIOs of the variant ctrl is.
func gpioCount(ctrl backend.Controller) int {
	for _, c := range ctrl.Compatible {
		for _, v := range variants {
			if c == v.compatible {
				return v.count
			}
		}
	}
	panic("bcm2835: controller not of the family")
}

// Level locates line in the GPLEV level registers: 32 GPIOs per register,
// the first at 0x34.
func (Backend) Level(ctrl backend.Controller, line int) (int, uint) {
	return 0x34 + 4*(line/32), uint(line % 32)
}

// GPIOMem returns the gpiomem device of the Raspberry Pi 3 and 4, which maps
// the GPIO controller's registers, and the length of its window.
func (Backend) GPIOMem() (string, int) {
	return "/dev/gpiomem", 0x1000
}
