//go:build linux && arm64

package gpiomem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/jclark/satpulse/gps/lib/gpiomem/soc"
	"golang.org/x/sys/unix"
)

// Pin is a mapped GPIO whose level can be read.
type Pin struct {
	mapping []byte
	level   *uint32
	mask    uint32
	device  string
}

// Open finds the GPIO called name, such as GPIO18, among the GPIO controllers
// of the running system, maps its controller's registers and returns a
// reader for its level.
func Open(name string) (*Pin, error) {
	ctrls, err := controllers()
	if err != nil {
		return nil, err
	}
	g, err := resolve(name, ctrls)
	if err != nil {
		return nil, err
	}
	offset, bit := g.soc.Level(g.ctrl, g.line)
	device, length := g.soc.GPIOMem()
	f, err := os.OpenFile(device, os.O_RDONLY|unix.O_SYNC, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := unix.Mmap(int(f.Fd()), 0, length, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mapping %s: %w", device, err)
	}
	return &Pin{mapping: b, level: (*uint32)(unsafe.Pointer(&b[offset])), mask: 1 << bit, device: device}, nil
}

const gpioDevices = "/sys/bus/gpio/devices"

// controllers lists the gpiochips that are bound to a device-tree node, with
// that node's compatible strings.
func controllers() ([]soc.Controller, error) {
	entries, err := os.ReadDir(gpioDevices)
	if err != nil {
		return nil, err
	}
	var ctrls []soc.Controller
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "gpiochip") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(gpioDevices, e.Name(), "of_node", "compatible"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if c := compatibles(b); len(c) > 0 {
			ctrls = append(ctrls, soc.Controller{Chip: e.Name(), Compatible: c})
		}
	}
	return ctrls, nil
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
