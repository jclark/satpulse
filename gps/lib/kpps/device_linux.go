//go:build linux

package kpps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sysClassPPS = "/sys/class/pps"

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
	name, err := findDevice(sysClassPPS, "path", ttyPath)
	if err != nil {
		return "", err
	}
	return filepath.Join("/dev", name), nil
}

// DevicePathForGPIONode returns the path of the kernel PPS device that the
// pps-gpio driver creates for the device-tree node at node, such as "/pps@12".
// The kernel documents no link from a PPS device to the device that feeds it,
// so the device is found by the name the driver gives its source: the name of
// the node's platform device followed by ".-1", the id of a platform device
// created from the device tree, cut to the 30 bytes the driver keeps. This
// relies on the driver's implementation; a kernel that names the source
// differently gives an error.
func DevicePathForGPIONode(node string) (string, error) {
	pdev, err := platformDevice(filepath.Join("/proc/device-tree", node))
	if err != nil {
		return "", err
	}
	source := pdev + ".-1"
	if len(source) > maxGPIOSourceName {
		source = source[:maxGPIOSourceName]
	}
	name, err := findDevice(sysClassPPS, "name", source)
	if err != nil {
		return "", err
	}
	return filepath.Join("/dev", name), nil
}

// maxGPIOSourceName is the length to which the pps-gpio driver cuts its
// source's name: it formats the name into PPS_MAX_NAME_LEN - 1 bytes,
// including the terminating NUL.
const maxGPIOSourceName = 30

// platformDevice returns the name of the platform device whose device-tree
// node is the directory node, matching its of_node link by file identity.
func platformDevice(node string) (string, error) {
	info, err := os.Stat(node)
	if err != nil {
		return "", err
	}
	const dir = "/sys/bus/platform/devices"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if i, err := os.Stat(filepath.Join(dir, e.Name(), "of_node")); err == nil && os.SameFile(i, info) {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("%s: no platform device", node)
}

// findDevice returns the name of the one PPS device whose sysfs attribute
// attr is value. Unreadable entries are skipped because another process's PPS
// source may vanish during the scan.
func findDevice(sysDir, attr, value string) (string, error) {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return "", err
	}
	var found string
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(sysDir, e.Name(), attr))
		if err != nil || strings.TrimSuffix(string(b), "\n") != value {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("%s: more than one kernel PPS source in %s", value, sysDir)
		}
		found = e.Name()
	}
	if found == "" {
		return "", fmt.Errorf("%s: no kernel PPS source in %s", value, sysDir)
	}
	return found, nil
}
