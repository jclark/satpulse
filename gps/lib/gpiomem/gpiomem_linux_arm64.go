//go:build linux && arm64

package gpiomem

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// Open identifies the SoC of the running system from its GPIO controllers,
// finds the GPIO the SoC calls name, such as GPIO18, maps its controller's
// registers and returns a reader for its level.
func Open(name string) (*Pin, error) {
	ctrls, err := controllers()
	if err != nil {
		return nil, err
	}
	s, ctrl, gpio, err := lookup(ctrls, name)
	if err != nil {
		return nil, err
	}
	offset, bit := s.Level(ctrl, gpio.Line)
	device, length := s.GPIOMem()
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

// Lookup identifies the SoC of the running system from its GPIO controllers
// and finds the GPIO the SoC calls name, as Open does, without opening it. It
// reads only sysfs, so it needs no privileges.
func Lookup(name string) (GPIO, error) {
	ctrls, err := controllers()
	if err != nil {
		return GPIO{}, err
	}
	_, _, gpio, err := lookup(ctrls, name)
	return gpio, err
}

// List describes the GPIO controllers of the running system that are bound
// to device-tree nodes, in the order of their gpiochip names. It reads only
// sysfs, so it needs no privileges.
func List() ([]Controller, error) {
	ctrls, err := controllers()
	if err != nil {
		return nil, err
	}
	nodes, err := ppsNodes()
	if err != nil {
		return nil, err
	}
	return describe(ctrls, nodes), nil
}

// controllers lists the gpiochips that are bound to a device-tree node, with
// that node's compatible strings and phandle.
func controllers() ([]soc.Controller, error) {
	dir := gpioDevices()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var ctrls []soc.Controller
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "gpiochip") {
			continue
		}
		node := filepath.Join(dir, e.Name(), "of_node")
		b, err := os.ReadFile(filepath.Join(node, "compatible"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		c := compatibles(b)
		if len(c) == 0 {
			continue
		}
		ctrl := soc.Controller{Chip: e.Name(), Compatible: c}
		if b, err := os.ReadFile(filepath.Join(node, "phandle")); err == nil && len(b) == 4 {
			ctrl.Phandle = binary.BigEndian.Uint32(b)
		}
		ctrls = append(ctrls, ctrl)
	}
	return ctrls, nil
}

// gpioDevices returns the directory linking to the gpiochips:
// /sys/subsystem/gpio/devices where the kernel provides /sys/subsystem, which
// sysfs-rules.rst says then replaces /sys/bus.
func gpioDevices() string {
	const subsystem = "/sys/subsystem/gpio/devices"
	if _, err := os.Stat(subsystem); err == nil {
		return subsystem
	}
	return "/sys/bus/gpio/devices"
}

const deviceTree = "/sys/firmware/devicetree/base"

// ppsNodes finds the enabled nodes of the device tree that are compatible
// with pps-gpio, with the cells of their gpios property.
func ppsNodes() ([]ppsNode, error) {
	var nodes []ppsNode
	err := filepath.WalkDir(deviceTree, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "compatible" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		if !slices.Contains(compatibles(b), "pps-gpio") || !enabled(dir) {
			return nil
		}
		if b, err := os.ReadFile(filepath.Join(dir, "gpios")); err == nil {
			nodes = append(nodes, ppsNode{path: strings.TrimPrefix(dir, deviceTree), gpios: cells(b)})
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return nodes, err
}

// enabled reports whether the device-tree node at dir is enabled: it has no
// status property, or its status is "okay" or "ok".
func enabled(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "status"))
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	status := strings.TrimRight(string(b), "\x00")
	return err == nil && (status == "okay" || status == "ok")
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
