//go:build linux && arm64

package gpiomem

import (
	"cmp"
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

	"github.com/jclark/satpulse/gps/lib/gpiomem/backend"
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
	ctrls, _, err := scan()
	if err != nil {
		return nil, err
	}
	be, ctrl, gpio, err := lookup(ctrls, name)
	if err != nil {
		return nil, err
	}
	offset, bit := be.Level(ctrl, gpio.Line)
	device, length := be.GPIOMem()
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
// reads only the device tree, so it needs no privileges.
func Lookup(name string) (GPIO, error) {
	ctrls, _, err := scan()
	if err != nil {
		return GPIO{}, err
	}
	_, _, gpio, err := lookup(ctrls, name)
	return gpio, err
}

// List describes the GPIO controllers of the running system, sorted by the
// paths of their device-tree nodes. It reads only the device tree, /dev and
// sysfs, so it needs no privileges.
func List() ([]Controller, error) {
	ctrls, nodes, err := scan()
	if err != nil {
		return nil, err
	}
	return describe(ctrls, gpiochips(ctrls), nodes), nil
}

// deviceTree is the path through which sysfs-firmware-ofw says userspace
// must reach the device tree: a symlink, whose target is not stable ABI.
const deviceTree = "/proc/device-tree"

// scan walks the device tree for its GPIO controllers, the enabled nodes with
// the gpio-controller property, and for its enabled pps-gpio nodes, with the
// cells of their gpios property. A node below a disabled node is not enabled,
// since the kernel does not create its device.
func scan() ([]backend.Controller, []ppsNode, error) {
	root, err := filepath.EvalSymlinks(deviceTree)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var ctrls []backend.Controller
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
			ok, err := enabled(path)
			if err != nil {
				return err
			}
			if !ok {
				return fs.SkipDir
			}
			return nil
		}
		dir := filepath.Dir(path)
		switch d.Name() {
		case "gpio-controller":
			// A property of another meaning, such as an alias, can have this
			// name; the controller's property is empty.
			b, err := property(dir, "gpio-controller")
			if err != nil || len(b) != 0 {
				return err
			}
			ctrl, err := controller(dir)
			if err != nil {
				return err
			}
			ctrl.Node = strings.TrimPrefix(dir, root)
			ctrls = append(ctrls, ctrl)
		case "compatible":
			b, err := property(dir, "compatible")
			if err != nil || !slices.Contains(compatibles(b), "pps-gpio") {
				return err
			}
			b, err = property(dir, "gpios")
			if err != nil {
				return err
			}
			nodes = append(nodes, ppsNode{path: strings.TrimPrefix(dir, root), gpios: cells(b)})
		}
		return nil
	})
	return ctrls, nodes, err
}

// enabled reports whether the device-tree node at dir is enabled: it has no
// status property, or its status is "okay" or "ok".
func enabled(dir string) (bool, error) {
	b, err := property(dir, "status")
	if err != nil {
		return false, err
	}
	if b == nil {
		return true, nil
	}
	status := strings.TrimRight(string(b), "\x00")
	return status == "okay" || status == "ok", nil
}

// controller reads the properties of the GPIO controller node at dir.
func controller(dir string) (backend.Controller, error) {
	var ctrl backend.Controller
	b, err := property(dir, "compatible")
	if err != nil {
		return ctrl, err
	}
	ctrl.Compatible = compatibles(b)
	b, err = property(dir, "phandle")
	if err == nil && b == nil {
		b, err = property(dir, "linux,phandle")
	}
	if err != nil {
		return ctrl, err
	}
	if len(b) == 4 {
		ctrl.Phandle = binary.BigEndian.Uint32(b)
	}
	b, err = property(dir, "#gpio-cells")
	if err != nil {
		return ctrl, err
	}
	if len(b) == 4 {
		ctrl.GPIOCells = int(binary.BigEndian.Uint32(b))
	}
	return ctrl, nil
}

// property reads the property called name of the device-tree node at dir,
// returning nil if the node does not have it. A child node of that name is
// not the property. A property whose node an overlay removes while it is
// being read gives ENODEV.
func property(dir, name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, unix.EISDIR) || errors.Is(err, unix.ENODEV) {
		return nil, nil
	}
	return b, err
}

// gpiochips finds the gpiochips of ctrls, mapping the path of a controller's
// node to the names of its gpiochips in numeric order. It finds them by the
// route the kernel documents as ABI: the character devices /dev/gpiochipN
// (ABI/testing/gpio-cdev), the sysfs directory of each device's number
// (ABI/testing/sysfs-dev), and its of_node link to its node
// (ABI/stable/sysfs-devices), which is matched with a controller's node by
// file identity. A symlink, such as the /dev/gpiochip4 Raspberry Pi OS adds
// on a Pi 5, is not a gpiochip of its own. The gpiochips only inform, so one
// that cannot be inspected is left out.
func gpiochips(ctrls []backend.Controller) map[string][]string {
	nodes := make([]fs.FileInfo, len(ctrls))
	for i, c := range ctrls {
		nodes[i], _ = os.Stat(filepath.Join(deviceTree, c.Node))
	}
	// The pattern is valid, so Glob returns no error.
	paths, _ := filepath.Glob("/dev/gpiochip[0-9]*")
	// With the digits checked below, shorter is smaller.
	slices.SortFunc(paths, func(a, b string) int { return cmp.Or(cmp.Compare(len(a), len(b)), strings.Compare(a, b)) })
	chips := make(map[string][]string)
	for _, path := range paths {
		chip := filepath.Base(path)
		if strings.Trim(strings.TrimPrefix(chip, "gpiochip"), "0123456789") != "" {
			continue
		}
		var st unix.Stat_t
		if unix.Lstat(path, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFCHR {
			continue
		}
		info, err := os.Stat(fmt.Sprintf("/sys/dev/char/%d:%d/of_node", unix.Major(st.Rdev), unix.Minor(st.Rdev)))
		if err != nil {
			continue
		}
		for i, n := range nodes {
			// SameFile is false for a node whose Stat failed.
			if os.SameFile(n, info) {
				chips[ctrls[i].Node] = append(chips[ctrls[i].Node], chip)
			}
		}
	}
	return chips
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
