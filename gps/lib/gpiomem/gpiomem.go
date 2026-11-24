// Package gpiomem reads the level of a GPIO directly from its controller's
// registers, mapped read-only, which suits polling for a pulse edge: a read
// is one bus transaction, taking about a microsecond on a Raspberry Pi 5.
//
// A GPIO is named as its SoC's documentation names it, such as GPIO18 on a
// Raspberry Pi; the name is case-insensitive ASCII. The SoC is identified
// from the GPIO controllers of the running system, the enabled nodes of its
// device tree with the gpio-controller property: those with a compatible
// string that one of the backends, the packages under this package, handles. That backend looks up the controller
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

	"github.com/jclark/satpulse/gps/lib/gpiomem/backend"
	"github.com/jclark/satpulse/gps/lib/gpiomem/bcm2835"
	"github.com/jclark/satpulse/gps/lib/gpiomem/rp1"
)

// backends lists the backends.
var backends = []backend.Backend{bcm2835.Backend{}, rp1.Backend{}}

// GPIO identifies a GPIO of the running system, as Lookup finds it.
type GPIO struct {
	Name string // the SoC's name for the GPIO, such as "GPIO18"
	Node string // the device-tree node of its controller, such as "/soc/gpio@7e200000"
	Line int    // its line on the controller
}

// lookup finds the GPIO called name among ctrls, the GPIO controllers on the
// system, returning the backend and the controller that has it. The name
// in the GPIO is the SoC's, whatever the case of name.
func lookup(ctrls []backend.Controller, name string) (backend.Backend, backend.Controller, GPIO, error) {
	upper, err := upperName(name)
	if err != nil {
		return nil, backend.Controller{}, GPIO{}, err
	}
	be, ctrls, err := findBackend(ctrls)
	if err != nil {
		return nil, backend.Controller{}, GPIO{}, err
	}
	i, line, err := be.Lookup(upper, ctrls)
	if err != nil {
		return nil, backend.Controller{}, GPIO{}, fmt.Errorf("%s: %w", name, err)
	}
	canon, ok := be.GPIOName(ctrls, i, line)
	if !ok {
		panic(fmt.Sprintf("gpiomem: %s has no name for line %d it looked up", be.Name(), line))
	}
	return be, ctrls[i], GPIO{Name: canon, Node: ctrls[i].Node, Line: line}, nil
}

// upperName checks that name is a non-empty ASCII string and returns it in
// upper case, the form the backends parse.
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

// findBackend finds the backend whose controllers are among ctrls, the GPIO
// controllers on the system, and returns it with those controllers. A system
// has one SoC, so controllers of more than one backend mean something has
// gone badly wrong.
func findBackend(ctrls []backend.Controller) (backend.Backend, []backend.Controller, error) {
	var found backend.Backend
	var foundCtrls []backend.Controller
	for _, be := range backends {
		var matched []backend.Controller
		for _, c := range ctrls {
			if handles(be, c) {
				matched = append(matched, c)
			}
		}
		if len(matched) == 0 {
			continue
		}
		if found != nil {
			c, d := foundCtrls[0], matched[0]
			return nil, nil, fmt.Errorf("GPIO controllers of more than one backend: %s is %s and %s is %s", c.Node, c.Compatible[0], d.Node, d.Compatible[0])
		}
		found, foundCtrls = be, matched
	}
	if found == nil {
		return nil, nil, unsupportedError(ctrls)
	}
	return found, foundCtrls, nil
}

// handles reports whether be handles ctrl.
func handles(be backend.Backend, ctrl backend.Controller) bool {
	return slices.ContainsFunc(ctrl.Compatible, func(compat string) bool { return slices.Contains(be.Compatible(), compat) })
}

// unsupportedError reports that none of ctrls is a supported controller,
// listing their compatible strings to identify the hardware.
func unsupportedError(ctrls []backend.Controller) error {
	if len(ctrls) == 0 {
		return errors.New("no GPIO controllers found in the device tree")
	}
	var compats []string
	for _, c := range ctrls {
		for _, compat := range c.Compatible {
			if !slices.Contains(compats, compat) {
				compats = append(compats, compat)
			}
		}
	}
	if len(compats) == 0 {
		return errors.New("no supported GPIO controller found")
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
	Node       string   // the path of its device-tree node, such as "/soc/gpio@7e200000"
	Compatible []string // the compatible strings of its node, if it has any
	Chips      []string // the names of its gpiochips that can be found, such as "gpiochip0"
	Backend    string   // the name of the backend that handles it, such as "rp1"
	Device     string   // the gpiomem device that maps its registers
	Length     int      // the length of the mapping
	PPS        []PPS    // its GPIOs that pps-gpio nodes of the device tree use
}

// PPS is a GPIO that a pps-gpio node of the device tree uses for its pulses.
// On a controller that a backend handles, the GPIO is given by its Name and
// Line; on any other, or if the backend has no name for the line, by the
// Cells of its GPIO specifier, whose meaning the controller's binding
// defines.
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
// backends that handle them, with chips, which maps the path of a node to its
// gpiochips, and with nodes, its pps-gpio nodes, whose GPIOs providers
// resolves to controllers. In the binding of every backend's controllers the
// first cell of a GPIO specifier is the line; a line the backend has no name
// for is left as its cells.
func describe(ctrls []backend.Controller, providers map[uint32]provider, chips map[string][]string, nodes []ppsNode) []Controller {
	list := make([]Controller, len(ctrls))
	for i, c := range ctrls {
		list[i] = Controller{Node: c.Node, Compatible: c.Compatible, Chips: chips[c.Node]}
	}
	for _, n := range nodes {
		phandle, spec, ok := resolve(providers, n.gpios)
		if !ok {
			continue
		}
		for i, c := range ctrls {
			if c.Phandle == phandle {
				list[i].PPS = append(list[i].PPS, PPS{Cells: spec, Node: n.path})
			}
		}
	}
	for _, be := range backends {
		var idx []int
		var backendCtrls []backend.Controller
		for i, c := range ctrls {
			if handles(be, c) {
				idx = append(idx, i)
				backendCtrls = append(backendCtrls, c)
			}
		}
		for j, i := range idx {
			c := &list[i]
			c.Backend = be.Name()
			c.Device, c.Length = be.GPIOMem()
			for k, p := range c.PPS {
				if len(p.Cells) == 0 {
					continue
				}
				line := int(p.Cells[0])
				if name, ok := be.GPIOName(backendCtrls, j, line); ok {
					c.PPS[k] = PPS{Name: name, Line: line, Node: p.Node}
				}
			}
		}
	}
	return list
}

// provider is a node of the device tree that a GPIO specifier can name: one
// with the #gpio-cells property. It is a GPIO controller, or a nexus node,
// whose gpio-map translates its specifiers to those of other providers
// (Devicetree Specification, section 2.5).
type provider struct {
	cells     int      // its #gpio-cells
	available bool     // whether its status allows its use
	gpioMap   []uint32 // its gpio-map, or nil if it is not a nexus node
	mask      []uint32 // its gpio-map-mask
	passThru  []uint32 // its gpio-map-pass-thru
}

// resolve finds the GPIO that gpios, a phandle followed by a GPIO specifier,
// names, following nexus nodes as the kernel's of_parse_phandle_with_args_map
// does, and returns the phandle of the GPIO's controller and the cells of its
// specifier there. ok is false if gpios names no GPIO.
func resolve(providers map[uint32]provider, gpios []uint32) (phandle uint32, spec []uint32, ok bool) {
	if len(gpios) == 0 {
		return 0, nil, false
	}
	phandle = gpios[0]
	p, ok := providers[phandle]
	if !ok || len(gpios) < 1+p.cells {
		return 0, nil, false
	}
	spec = gpios[1 : 1+p.cells]
	for hops := 0; p.gpioMap != nil; hops++ {
		// Only a loop of nexus nodes passes through more nodes than there
		// are. The kernel's translation never ends on one.
		if hops == len(providers) {
			return 0, nil, false
		}
		if phandle, spec, ok = p.translate(providers, spec); !ok {
			return 0, nil, false
		}
		p = providers[phandle]
	}
	return phandle, spec, true
}

// translate maps spec, a specifier of the nexus node p, through p's gpio-map,
// as the kernel does. The map's entries are each a child specifier, the
// phandle of a parent provider and a specifier of the parent. The first entry
// whose child specifier equals spec in the bits of gpio-map-mask and whose
// parent is available gives the parent and its specifier, with the bits of
// gpio-map-pass-thru taken from spec.
func (p provider) translate(providers map[uint32]provider, spec []uint32) (uint32, []uint32, bool) {
	m := p.gpioMap
	// As in the kernel, an entry is looked at only while more cells remain
	// than a child specifier and a phandle.
	for len(m) > len(spec)+1 {
		match := true
		for i, c := range spec {
			if (c^m[i])&cellOr(p.mask, i, ^uint32(0)) != 0 {
				match = false
			}
		}
		phandle := m[len(spec)]
		parent, ok := providers[phandle]
		m = m[len(spec)+1:]
		if !ok || len(m) < parent.cells {
			return 0, nil, false
		}
		if match && parent.available {
			out := slices.Clone(m[:parent.cells])
			for i := range min(len(out), len(spec)) {
				pass := cellOr(p.passThru, i, 0)
				out[i] = out[i]&^pass | spec[i]&pass
			}
			return phandle, out, true
		}
		m = m[parent.cells:]
	}
	return 0, nil, false
}

// cellOr returns c[i], or def if c has no cell i, as when an optional
// property is absent.
func cellOr(c []uint32, i int, def uint32) uint32 {
	if i < len(c) {
		return c[i]
	}
	return def
}

// cells splits a device-tree property into its big-endian 32-bit cells.
func cells(b []byte) []uint32 {
	c := make([]uint32, len(b)/4)
	for i := range c {
		c[i] = binary.BigEndian.Uint32(b[4*i:])
	}
	return c
}
