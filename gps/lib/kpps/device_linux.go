//go:build linux

//go:generate sh -c "go tool cgo -godefs types_linux.go | gofmt > ztypes_linux.go && rm -rf _obj"

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

// sysBusPlatform is the directory of the platform devices.
const sysBusPlatform = "/sys/bus/platform/devices"

// DevicePathForGPIONode returns the path of the kernel PPS device that the
// pps-gpio driver creates for the device-tree node at node, such as "/pps@12".
// The kernel documents no link from a PPS device to the device that feeds it,
// so the device is found by the name the driver gives its source: the name of
// the node's platform device followed by ".-1", the id of a platform device
// created from the device tree, cut to the bytes the driver keeps. This
// relies on the driver's implementation; a kernel that names the source
// differently gives an error, as does a cut name that another platform device
// could also give.
func DevicePathForGPIONode(node string) (string, error) {
	entries, err := os.ReadDir(sysBusPlatform)
	if err != nil {
		return "", err
	}
	pdev, err := platformDevice(entries, filepath.Join("/proc/device-tree", node))
	if err != nil {
		return "", err
	}
	source := pdev + ".-1"
	// The driver formats the name with snprintf into PPS_MAX_NAME_LEN - 1
	// bytes, which hold the terminating NUL.
	if keep := ppsMaxNameLen - 2; len(source) > keep {
		source = source[:keep]
		n := 0
		for _, e := range entries {
			if strings.HasPrefix(e.Name()+".-1", source) {
				n++
			}
		}
		if n > 1 {
			return "", fmt.Errorf("%s: %d platform devices give PPS source name %q", node, n, source)
		}
	}
	name, err := findDevice(sysClassPPS, "name", source)
	if err != nil {
		return "", err
	}
	return filepath.Join("/dev", name), nil
}

// platformDevice returns the name of the platform device among entries, the
// entries of sysBusPlatform, whose device-tree node is the directory node,
// matching its of_node link by file identity.
func platformDevice(entries []os.DirEntry, node string) (string, error) {
	info, err := os.Stat(node)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if i, err := os.Stat(filepath.Join(sysBusPlatform, e.Name(), "of_node")); err == nil && os.SameFile(i, info) {
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
