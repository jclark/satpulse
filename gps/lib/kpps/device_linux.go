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

// Device describes a kernel PPS device as sysfs registers it.
type Device struct {
	Path       string // the device node, /dev/ppsN
	Name       string // the sysfs name attribute: the source name the driver registered
	SourcePath string // the sysfs path attribute: the source's device path, such as a tty; empty for most drivers
	Mode       Mode   // the sysfs mode attribute: the capabilities, as GetCap reports them
}

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
