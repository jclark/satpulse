// Package gpiomem reads the level of a GPIO directly from its controller's
// registers, mapped read-only, which suits polling for a pulse edge: a read
// is one bus transaction, taking about a microsecond on a Raspberry Pi 5.
//
// A GPIO is named as its SoC's documentation names it, such as GPIO18 on a
// Raspberry Pi; the name is case-insensitive ASCII. The GPIO controllers on
// the running system are the gpiochips whose device-tree nodes have a
// compatible string that one of the SoC family packages under this package
// handles; that package resolves the name to a controller and a line and
// locates the line's level register. Currently the registers are mapped
// through the gpiomem devices that Raspberry Pi OS provides.
//
// The package does not configure the GPIO: it must already be an input, as
// the pps-gpio overlay makes it. Opening a GPIO is implemented for 64-bit
// Linux only.
package gpiomem

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jclark/satpulse/gps/lib/gpiomem/bcm2835"
	"github.com/jclark/satpulse/gps/lib/gpiomem/rp1"
	"github.com/jclark/satpulse/gps/lib/gpiomem/soc"
)

// socs lists the supported SoC families.
var socs = []soc.SoC{bcm2835.SoC{}, rp1.SoC{}}

// A gpio is a resolved GPIO: the family of its controller, the controller,
// and its line on that controller.
type gpio struct {
	soc  soc.SoC
	ctrl soc.Controller
	line int
}

// resolve finds the GPIO called name among ctrls, the GPIO controllers on
// the system.
func resolve(name string, ctrls []soc.Controller) (gpio, error) {
	if name == "" {
		return gpio{}, errors.New("empty GPIO name")
	}
	for i := 0; i < len(name); i++ {
		if name[i] >= 0x80 {
			return gpio{}, fmt.Errorf("GPIO name %q: not ASCII", name)
		}
	}
	upper := strings.ToUpper(name)
	var found []gpio
	var errs []error
	for _, s := range socs {
		var matched []soc.Controller
		for _, c := range ctrls {
			if slices.ContainsFunc(c.Compatible, func(compat string) bool { return slices.Contains(s.Compatible(), compat) }) {
				matched = append(matched, c)
			}
		}
		if len(matched) == 0 {
			continue
		}
		i, line, err := s.Resolve(upper, matched)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		found = append(found, gpio{s, matched[i], line})
	}
	if len(found) == 1 {
		return found[0], nil
	}
	if len(found) > 1 {
		return gpio{}, fmt.Errorf("%s: ambiguous; found on %s and %s", name, found[0].ctrl.Chip, found[1].ctrl.Chip)
	}
	if len(errs) == 0 {
		return gpio{}, unsupportedError(ctrls)
	}
	return gpio{}, fmt.Errorf("%s: %w", name, errors.Join(errs...))
}

// unsupportedError reports that none of ctrls is a supported controller,
// listing their compatible strings to identify the hardware.
func unsupportedError(ctrls []soc.Controller) error {
	if len(ctrls) == 0 {
		return errors.New("no GPIO controllers with a device-tree node found")
	}
	var compats []string
	for _, c := range ctrls {
		for _, compat := range c.Compatible {
			if !slices.Contains(compats, compat) {
				compats = append(compats, compat)
			}
		}
	}
	return fmt.Errorf("no supported GPIO controller; found %s", strings.Join(compats, ", "))
}

// compatibles splits the NUL-terminated strings of a device-tree compatible
// property.
func compatibles(b []byte) []string {
	var s []string
	for _, c := range bytes.Split(bytes.TrimRight(b, "\x00"), []byte{0}) {
		if len(c) > 0 {
			s = append(s, string(c))
		}
	}
	return s
}
