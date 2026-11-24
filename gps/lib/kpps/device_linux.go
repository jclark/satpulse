//go:build linux

package kpps

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const sysClassPPS = "/sys/class/pps"

// ListDevices returns the kernel PPS devices in /sys/class/pps, in name
// order. A device whose attributes cannot be read, because it was
// unregistered during the scan, is left out.
func ListDevices() ([]Device, error) {
	return listDevices(sysClassPPS)
}

func listDevices(sysDir string) ([]Device, error) {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var devices []Device
	for _, e := range entries {
		d, err := readDevice(sysDir, e.Name())
		if err != nil {
			continue
		}
		devices = append(devices, d)
	}
	return devices, nil
}

func readDevice(sysDir, name string) (Device, error) {
	attr := func(a string) (string, error) {
		b, err := os.ReadFile(filepath.Join(sysDir, name, a))
		return strings.TrimSuffix(string(b), "\n"), err
	}
	d := Device{Path: "/dev/" + name}
	var err error
	if d.Name, err = attr("name"); err != nil {
		return Device{}, err
	}
	if d.SourcePath, err = attr("path"); err != nil {
		return Device{}, err
	}
	s, err := attr("mode")
	if err != nil {
		return Device{}, err
	}
	mode, err := strconv.ParseUint(strings.TrimSpace(s), 16, 32)
	if err != nil {
		return Device{}, fmt.Errorf("mode attribute %q: %w", s, err)
	}
	d.Mode = Mode(mode)
	return d, nil
}

// DevicePathForTTY returns the path of the kernel PPS device fed by the TTY
// open on fd, which the N_PPS line discipline creates when it is attached to
// that TTY. The kernel does not report the number it assigns, and the numbers
// are not stable across reopens, so the device is found by its sysfs path
// attribute rather than by creation order.
func DevicePathForTTY(fd int) (string, error) {
	ttyPath, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return "", err
	}
	return findDevice(sysClassPPS, ttyPath)
}

// DevicePathForGPIONode returns the path of the kernel PPS device that the
// pps-gpio driver creates for the device-tree node at node, such as "/pps@12".
// The kernel documents no link from a PPS device to the device that feeds it,
// so the device is found by the name the driver gives its source: the name of
// the node's platform device followed by ".-1", the id of a platform device
// created from the device tree. This relies on the driver's implementation;
// a kernel that names the source differently gives an error.
func DevicePathForGPIONode(node string) (string, error) {
	pdev, err := platformDevice(filepath.Join("/sys/firmware/devicetree/base", node))
	if err != nil {
		return "", err
	}
	devices, err := listDevices(sysClassPPS)
	if err != nil {
		return "", err
	}
	for _, d := range devices {
		if d.Name == pdev+".-1" {
			return d.Path, nil
		}
	}
	return "", fmt.Errorf("%s: no kernel PPS source in %s", pdev, sysClassPPS)
}

// platformDevice returns the name of the platform device whose device-tree
// node is the sysfs directory node.
func platformDevice(node string) (string, error) {
	const dir = "/sys/bus/platform/devices"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if p, err := filepath.EvalSymlinks(filepath.Join(dir, e.Name(), "of_node")); err == nil && p == node {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("%s: no platform device", node)
}

// findDevice returns the path of the PPS device whose sysfs path attribute is
// sourcePath.
func findDevice(sysDir, sourcePath string) (string, error) {
	devices, err := listDevices(sysDir)
	if err != nil {
		return "", err
	}
	for _, d := range devices {
		if d.SourcePath == sourcePath {
			return d.Path, nil
		}
	}
	return "", fmt.Errorf("%s: no kernel PPS source in %s", sourcePath, sysDir)
}
