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
	name, err := findDevice(sysClassPPS, ttyPath)
	if err != nil {
		return "", err
	}
	return filepath.Join("/dev", name), nil
}

// findDevice returns the name of the PPS device whose sysfs path attribute is
// sourcePath. Unreadable entries are skipped because another process's PPS
// source may vanish during the scan.
func findDevice(sysDir, sourcePath string) (string, error) {
	entries, err := os.ReadDir(sysDir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(sysDir, e.Name(), "path"))
		if err != nil {
			continue
		}
		if strings.TrimSuffix(string(b), "\n") == sourcePath {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("%s: no kernel PPS source in %s", sourcePath, sysDir)
}
