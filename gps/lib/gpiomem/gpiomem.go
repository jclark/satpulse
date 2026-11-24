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
// List describes the GPIO controllers of the running system, including the
// GPIOs that the device tree gives to the pps-gpio driver, so a user can find
// the name of the GPIO a PPS signal is on.
//
// The package does not configure the GPIO: it must already be an input, as
// the pps-gpio overlay makes it. Opening or looking up a GPIO and listing
// controllers are implemented for 64-bit Arm Linux only; elsewhere Open, Lookup
// and List return an error wrapping errors.ErrUnsupported.
package gpiomem

import (
	"bytes"
	"encoding/binary"
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

// GPIO identifies a GPIO of the running system, as Lookup finds it.
type GPIO struct {
	Name string // the SoC's name for the GPIO, such as "GPIO18"
	Chip string // the gpiochip of its controller, such as "gpiochip0"
	Line int    // its line on the controller
}

// lookup finds the GPIO called name among ctrls, the GPIO controllers on the
// system, returning the SoC family and the controller that has it. The name
// in the GPIO is the SoC's, whatever the case of name.
func lookup(ctrls []soc.Controller, name string) (soc.SoC, soc.Controller, GPIO, error) {
	upper, err := upperName(name)
	if err != nil {
		return nil, soc.Controller{}, GPIO{}, err
	}
	s, ctrls, err := findSoC(ctrls)
	if err != nil {
		return nil, soc.Controller{}, GPIO{}, err
	}
	i, line, err := s.Lookup(upper, ctrls)
	if err != nil {
		return nil, soc.Controller{}, GPIO{}, fmt.Errorf("%s: %w", name, err)
	}
	canon, ok := s.Name(ctrls, i, line)
	if !ok {
		panic(fmt.Sprintf("gpiomem: %s has no name for line %d it looked up", s.Backend(), line))
	}
	return s, ctrls[i], GPIO{Name: canon, Chip: ctrls[i].Chip, Line: line}, nil
}

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
			if handles(s, c) {
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

// handles reports whether s is the family of ctrl.
func handles(s soc.SoC, ctrl soc.Controller) bool {
	return slices.ContainsFunc(ctrl.Compatible, func(compat string) bool { return slices.Contains(s.Compatible(), compat) })
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

// Controller describes a GPIO controller of the running system, as List
// reports it. Backend, Device and Length are set only for a controller that
// a gpiomem backend handles.
type Controller struct {
	Chip       string   // the gpiochip's device name, such as "gpiochip0"
	Compatible []string // the compatible strings of its device-tree node
	Backend    string   // the name of the backend that handles it, such as "rp1"
	Device     string   // the gpiomem device that maps its registers
	Length     int      // the length of the mapping
	PPS        []PPS    // its GPIOs that pps-gpio nodes of the device tree use
}

// PPS is a GPIO that a pps-gpio node of the device tree uses for its pulses.
// On a controller that a backend handles, the GPIO is given by its Name and
// Line; on any other, by the Cells of its GPIO specifier, whose meaning the
// controller's binding defines.
type PPS struct {
	Name  string   // the SoC's name for the GPIO, such as "GPIO18"
	Line  int      // the GPIO's line on its controller
	Cells []uint32 // the cells of the GPIO specifier
	Node  string   // the path of the pps-gpio node in the device tree
}

// ppsNode is an enabled pps-gpio node of the device tree, with the cells of
// its gpios property.
type ppsNode struct {
	path  string
	gpios []uint32
}

// describe combines ctrls, the GPIO controllers of the system, with the
// backends that handle them and with nodes, its pps-gpio nodes. A node's
// gpios property holds the phandle of a controller followed by the
// controller's #gpio-cells cells of the GPIO specifier. In the binding of
// every backend's controllers the first cell is the line, and each
// device-tree node has one gpiochip.
func describe(ctrls []soc.Controller, nodes []ppsNode) []Controller {
	list := make([]Controller, len(ctrls))
	for i, c := range ctrls {
		list[i] = Controller{Chip: c.Chip, Compatible: c.Compatible}
		for _, n := range nodes {
			if c.Phandle != 0 && c.GPIOCells > 0 && len(n.gpios) >= 1+c.GPIOCells && n.gpios[0] == c.Phandle {
				list[i].PPS = append(list[i].PPS, PPS{Cells: n.gpios[1 : 1+c.GPIOCells], Node: n.path})
			}
		}
	}
	for _, s := range socs {
		var idx []int
		var backendCtrls []soc.Controller
		for i, c := range ctrls {
			if handles(s, c) {
				idx = append(idx, i)
				backendCtrls = append(backendCtrls, c)
			}
		}
		for j, i := range idx {
			c := &list[i]
			c.Backend = s.Backend()
			c.Device, c.Length = s.GPIOMem()
			var pps []PPS
			for _, p := range c.PPS {
				line := int(p.Cells[0])
				if name, ok := s.Name(backendCtrls, j, line); ok {
					pps = append(pps, PPS{Name: name, Line: line, Node: p.Node})
				}
			}
			c.PPS = pps
		}
	}
	return list
}

// cells splits a device-tree property into its big-endian 32-bit cells.
func cells(b []byte) []uint32 {
	c := make([]uint32, len(b)/4)
	for i := range c {
		c[i] = binary.BigEndian.Uint32(b[4*i:])
	}
	return c
}
