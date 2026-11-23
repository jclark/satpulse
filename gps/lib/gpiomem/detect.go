// Package gpiomem reads the level of a GPIO directly from its controller's
// registers, mapped read-only, which suits polling for a pulse edge: a read
// is one bus transaction, taking about a microsecond on a Raspberry Pi 5.
//
// A GPIO is named as its SoC's documentation names it, such as GPIO18 on a
// Raspberry Pi; the name is case-insensitive ASCII. The SoC is identified
// from the GPIO controllers of the running system: the gpiochips whose
// device-tree nodes have a compatible string that one of the SoC family
// packages under this package handles. That package looks up the controller
// and line the name identifies and locates the line's level register.
// Currently the registers are mapped through the gpiomem devices that
// Raspberry Pi OS provides.
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

// upperName checks that name is a non-empty ASCII string and returns it in
// upper case, the form the SoC family packages parse.
func upperName(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty GPIO name")
	}
	for i := 0; i < len(name); i++ {
		if name[i] >= 0x80 {
			return "", fmt.Errorf("GPIO name %q: not ASCII", name)
		}
	}
	return strings.ToUpper(name), nil
}

// findSoC finds the SoC family whose controllers are among ctrls, the GPIO
// controllers on the system, and returns it with those controllers. A system
// has one SoC, so controllers of more than one family mean something has gone
// badly wrong.
func findSoC(ctrls []soc.Controller) (soc.SoC, []soc.Controller, error) {
	var found soc.SoC
	var foundCtrls []soc.Controller
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
		if found != nil {
			c, d := foundCtrls[0], matched[0]
			return nil, nil, fmt.Errorf("GPIO controllers of more than one SoC family: %s is %s and %s is %s", c.Chip, c.Compatible[0], d.Chip, d.Compatible[0])
		}
		found, foundCtrls = s, matched
	}
	if found == nil {
		return nil, nil, unsupportedError(ctrls)
	}
	return found, foundCtrls, nil
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
