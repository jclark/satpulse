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
// sysfs and the device tree, so it needs no privileges.
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
// that node's compatible strings and phandle. It finds them by the route the
// kernel documents as ABI: the character devices /dev/gpiochipN
// (ABI/testing/gpio-cdev), the sysfs directory of each device's number
// (ABI/testing/sysfs-dev), and its of_node link (ABI/stable/sysfs-devices).
// A symlink, such as the /dev/gpiochip4 Raspberry Pi OS adds on a Pi 5, is
// not a gpiochip of its own.
func controllers() ([]soc.Controller, error) {
	paths, err := filepath.Glob("/dev/gpiochip[0-9]*")
	if err != nil {
		return nil, err
	}
	var ctrls []soc.Controller
	for _, path := range paths {
		chip := filepath.Base(path)
		if strings.Trim(strings.TrimPrefix(chip, "gpiochip"), "0123456789") != "" {
			continue
		}
		var st unix.Stat_t
		if err := unix.Lstat(path, &st); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if st.Mode&unix.S_IFMT != unix.S_IFCHR {
			continue
		}
		node := fmt.Sprintf("/sys/dev/char/%d:%d/of_node", unix.Major(st.Rdev), unix.Minor(st.Rdev))
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
		ctrl := soc.Controller{Chip: chip, Compatible: c}
		b, err = os.ReadFile(filepath.Join(node, "phandle"))
		if errors.Is(err, fs.ErrNotExist) {
			b, err = os.ReadFile(filepath.Join(node, "linux,phandle"))
		}
		if err == nil && len(b) == 4 {
			ctrl.Phandle = binary.BigEndian.Uint32(b)
		}
		if b, err := os.ReadFile(filepath.Join(node, "#gpio-cells")); err == nil && len(b) == 4 {
			ctrl.GPIOCells = int(binary.BigEndian.Uint32(b))
		}
		ctrls = append(ctrls, ctrl)
	}
	return ctrls, nil
}

// deviceTree is the path through which sysfs-firmware-ofw says userspace
// must reach the device tree: a symlink, whose target is not stable ABI.
const deviceTree = "/proc/device-tree"

// ppsNodes finds the enabled nodes of the device tree that are compatible
// with pps-gpio, with the cells of their gpios property. A node below a
// disabled node is not enabled, since the kernel does not create its device.
func ppsNodes() ([]ppsNode, error) {
	root, err := filepath.EvalSymlinks(deviceTree)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var nodes []ppsNode
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		// An overlay can be removed during the walk.
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !enabled(path) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "compatible" {
			return nil
		}
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		if !slices.Contains(compatibles(b), "pps-gpio") {
			return nil
		}
		if b, err := os.ReadFile(filepath.Join(dir, "gpios")); err == nil {
			nodes = append(nodes, ppsNode{path: strings.TrimPrefix(dir, root), gpios: cells(b)})
		}
		return nil
	})
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
