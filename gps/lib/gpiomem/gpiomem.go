//go:build linux && arm64

// Package gpiomem reads the level of a Raspberry Pi GPIO directly from the
// GPIO controller's registers, mapped read-only through the gpiomem device
// that Raspberry Pi OS provides for user-space register access. A read is one
// bus transaction, taking about a microsecond on a Raspberry Pi 5, which makes
// it suitable for polling for a pulse edge. The package does not configure
// the GPIO: it must already be an input, as the pps-gpio overlay or pinctrl
// makes it. It is built for 64-bit Linux only.
package gpiomem

import (
	"bytes"
	"fmt"
	"os"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

// A model describes the GPIO controller of a Raspberry Pi SoC: the device
// that maps its registers, the length to map, the number of GPIOs, and the
// location of a GPIO's level bit within the mapping.
type model struct {
	soc    string // the SoC's compatible string in the device tree
	device string
	length int
	count  int
	reg    func(gpio int) (offset int, bit uint)
}

// Pin is a mapped GPIO whose level can be read.
type Pin struct {
	mapping []byte
	level   *uint32
	mask    uint32
	device  string
}

const compatiblePath = "/proc/device-tree/compatible"

// Open maps the registers of the GPIO controller that the device tree
// identifies and returns a reader for the level of GPIO gpio.
func Open(gpio int) (*Pin, error) {
	compatible, err := os.ReadFile(compatiblePath)
	if err != nil {
		return nil, err
	}
	m, ok := modelFor(compatible)
	if !ok {
		return nil, fmt.Errorf("%s: no supported SoC in %q", compatiblePath, compatible)
	}
	if gpio < 0 || gpio >= m.count {
		return nil, fmt.Errorf("GPIO %d is out of range; %s has GPIOs 0 to %d", gpio, m.soc, m.count-1)
	}
	f, err := os.OpenFile(m.device, os.O_RDONLY|unix.O_SYNC, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := unix.Mmap(int(f.Fd()), 0, m.length, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mapping %s: %w", m.device, err)
	}
	offset, bit := m.reg(gpio)
	return &Pin{mapping: b, level: (*uint32)(unsafe.Pointer(&b[offset])), mask: 1 << bit, device: m.device}, nil
}

// models lists the supported SoCs. The 32-bit-only BCM2835 and BCM2836
// share the BCM283x layout but are left out until the package is built for
// 32-bit systems.
var models = []model{
	{soc: "brcm,bcm2712", device: "/dev/gpiomem0", length: 0x30000, count: 54, reg: rp1Reg},
	{soc: "brcm,bcm2711", device: "/dev/gpiomem", length: 0x1000, count: 58, reg: bcm283xReg},
	{soc: "brcm,bcm2837", device: "/dev/gpiomem", length: 0x1000, count: 54, reg: bcm283xReg},
}

// modelFor finds the supported SoC among the device tree's NUL-separated
// compatible strings.
func modelFor(compatible []byte) (model, bool) {
	for _, s := range bytes.Split(compatible, []byte{0}) {
		for _, m := range models {
			if string(s) == m.soc {
				return m, true
			}
		}
	}
	return model{}, false
}

// bcm283xReg locates a GPIO in the BCM2835 family's level registers: 32
// GPIOs per register, the first at 0x34.
func bcm283xReg(gpio int) (int, uint) {
	return 0x34 + 4*(gpio/32), uint(gpio % 32)
}

// rp1Reg locates a GPIO in the RP1's RIO_IN registers on the Raspberry Pi
// 5, the register the kernel's pinctrl-rp1 driver reads for a GPIO's
// level. The GPIOs are in three banks of 28, 6 and 20; each bank's
// registers are 0x4000 apart, with bank 0's RIO_IN at 0x10008.
func rp1Reg(gpio int) (int, uint) {
	bank, base := 0, 0
	if gpio >= 34 {
		bank, base = 2, 34
	} else if gpio >= 28 {
		bank, base = 1, 28
	}
	return 0x10008 + bank*0x4000, uint(gpio - base)
}

// High reports whether the GPIO currently reads high.
func (p *Pin) High() bool {
	return atomic.LoadUint32(p.level)&p.mask != 0
}

// Device returns the path of the gpiomem device the GPIO is read through.
func (p *Pin) Device() string {
	return p.device
}

// Close unmaps the registers. It must not be called while High may still
// run: High has no guard, so a read after Close faults. Calling Close twice
// is a programmer error and panics.
func (p *Pin) Close() error {
	if p.mapping == nil {
		panic("gpiomem: Pin closed twice")
	}
	err := unix.Munmap(p.mapping)
	p.mapping = nil
	return err
}
