//go:build linux

package serialenum

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	sysClassTTYDir = "/sys/class/tty"
	devDir         = "/dev"
)

type portInfo struct {
	Port
	name    string
	product string
}

// List enumerates hardware serial ports using sysfs. It never opens a device
// node.
func List() ([]Port, error) {
	ports, err := enumerate()
	if err != nil {
		return nil, fmt.Errorf("enumerating serial ports: %w", err)
	}
	result := make([]Port, len(ports))
	for i := range ports {
		result[i] = ports[i].Port
	}
	return result, nil
}

func enumerate() ([]portInfo, error) {
	entries, err := os.ReadDir(sysClassTTYDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sysClassTTYDir, err)
	}

	sysDir := filepath.Dir(filepath.Dir(filepath.Clean(sysClassTTYDir)))
	devicesDir := filepath.Join(sysDir, "devices")
	virtualDir := filepath.Join(devicesDir, "virtual")
	var ports []portInfo
	for _, entry := range entries {
		portPath, err := filepath.EvalSymlinks(filepath.Join(sysClassTTYDir, entry.Name()))
		if err != nil {
			if isDeviceGone(err) {
				continue
			}
			return nil, fmt.Errorf("resolving tty %s: %w", entry.Name(), err)
		}
		if pathWithin(portPath, virtualDir) && !isRFCOMMName(entry.Name()) {
			continue
		}

		phantom, err := isPhantomPort(portPath)
		if err != nil {
			if isDeviceGone(err) {
				continue
			}
			return nil, fmt.Errorf("checking tty %s: %w", entry.Name(), err)
		}
		if phantom {
			continue
		}

		port, err := readPortInfo(portPath, devicesDir)
		if err != nil {
			if isDeviceGone(err) {
				continue
			}
			return nil, fmt.Errorf("reading details for tty %s: %w", entry.Name(), err)
		}
		ports = append(ports, port)
	}

	ports, err = filterAndCollectAliases(ports)
	if err != nil {
		return nil, err
	}
	portCount := countUSBDevicePorts(ports)
	for i := range ports {
		setDisplay(&ports[i], portCount[ports[i].usbDeviceKey()] > 1)
	}
	return ports, nil
}

func isDeviceGone(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ENODEV)
}

func isRFCOMMName(name string) bool {
	const prefix = "rfcomm"
	if !strings.HasPrefix(name, prefix) || len(name) == len(prefix) {
		return false
	}
	for _, c := range name[len(prefix):] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isPhantomPort(portPath string) (bool, error) {
	portType, err := readOptionalFile(filepath.Join(portPath, "type"))
	if err != nil {
		return false, err
	}
	return portType == "0", nil
}

func readPortInfo(portPath, devicesDir string) (portInfo, error) {
	name := filepath.Base(portPath)
	port := portInfo{
		Port: Port{
			Device: filepath.Join(devDir, name),
		},
		name: name,
	}

	for dir := portPath; pathWithin(dir, devicesDir); dir = filepath.Dir(dir) {
		subsystem, err := readOptionalLink(filepath.Join(dir, "subsystem"))
		if err != nil {
			return portInfo{}, err
		}
		if filepath.Base(filepath.Clean(subsystem)) != "usb" {
			continue
		}
		uevent, err := readOptionalFile(filepath.Join(dir, "uevent"))
		if err != nil {
			return portInfo{}, err
		}
		if hasUeventValue(uevent, "DEVTYPE", "usb_interface") {
			port.Interface, err = readOptionalFile(filepath.Join(dir, "bInterfaceNumber"))
			if err != nil {
				return portInfo{}, fmt.Errorf("reading USB attribute bInterfaceNumber: %w", err)
			}
			continue
		}
		if !hasUeventValue(uevent, "DEVTYPE", "usb_device") {
			continue
		}

		if err := readUSBDetails(dir, &port); err != nil {
			return portInfo{}, err
		}
		break
	}
	return port, nil
}

func readUSBDetails(dir string, port *portInfo) error {
	vid, err := readOptionalFile(filepath.Join(dir, "idVendor"))
	if err != nil {
		return fmt.Errorf("reading USB attribute idVendor: %w", err)
	}
	port.USB.VID, err = parseUSBID(vid)
	if err != nil {
		return fmt.Errorf("parsing USB attribute idVendor %q: %w", vid, err)
	}
	pid, err := readOptionalFile(filepath.Join(dir, "idProduct"))
	if err != nil {
		return fmt.Errorf("reading USB attribute idProduct: %w", err)
	}
	port.USB.PID, err = parseUSBID(pid)
	if err != nil {
		return fmt.Errorf("parsing USB attribute idProduct %q: %w", pid, err)
	}
	port.product, err = readOptionalFile(filepath.Join(dir, "product"))
	if err != nil {
		return fmt.Errorf("reading USB attribute product: %w", err)
	}
	port.Serial, err = readOptionalFile(filepath.Join(dir, "serial"))
	if err != nil {
		return fmt.Errorf("reading USB attribute serial: %w", err)
	}
	return nil
}

func filterAndCollectAliases(ports []portInfo) ([]portInfo, error) {
	entries, err := os.ReadDir(devDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", devDir, err)
	}

	cleanDevDir := filepath.Clean(devDir)
	devEntries := make(map[string]bool, len(entries))
	for _, entry := range entries {
		devEntries[entry.Name()] = true
	}

	present := ports[:0]
	for _, port := range ports {
		if devEntries[port.name] {
			present = append(present, port)
		}
	}
	ports = present

	portByNode := make(map[string]int, len(ports))
	for i := range ports {
		portByNode[filepath.Join(cleanDevDir, ports[i].name)] = i
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		alias := filepath.Join(cleanDevDir, entry.Name())
		target, err := os.Readlink(alias)
		if err != nil {
			if isDeviceGone(err) {
				continue
			}
			return nil, fmt.Errorf("reading symlink %s: %w", alias, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(cleanDevDir, target)
		}
		target = filepath.Clean(target)
		if filepath.Dir(target) != cleanDevDir {
			continue
		}
		portIndex, ok := portByNode[target]
		if !ok || alias == target {
			continue
		}
		ports[portIndex].Aliases = append(ports[portIndex].Aliases, alias)
	}
	for i := range ports {
		sort.Strings(ports[i].Aliases)
	}
	return ports, nil
}

type usbDeviceKey struct {
	USBID
	serial string
}

func (port *portInfo) usbDeviceKey() usbDeviceKey {
	return usbDeviceKey{port.USB, port.Serial}
}

// countUSBDevicePorts counts the ports of each USB device, so that the ports
// of a multi-port device, which share VID, PID and serial, can be told apart
// by interface number.
func countUSBDevicePorts(ports []portInfo) map[usbDeviceKey]int {
	count := make(map[usbDeviceKey]int)
	for i := range ports {
		if ports[i].USB != (USBID{}) {
			count[ports[i].usbDeviceKey()]++
		}
	}
	return count
}

// setDisplay builds the display string. multiport says that other ports in
// the list belong to the same USB device, so the interface number is needed
// to tell them apart.
func setDisplay(port *portInfo, multiport bool) {
	deviceDetail := port.product
	tag := ubloxTag(port.USB.VID, port.USB.PID)
	if tag != "" && tag != "u-blox" {
		// A generation tag is more informative than u-blox's generic USB
		// product string.
		deviceDetail = tag
	} else if deviceDetail == "" {
		deviceDetail = tag
	}

	port.Display = port.Device
	details := append([]string(nil), port.Aliases...)
	if deviceDetail != "" {
		details = append(details, deviceDetail)
	}
	if multiport && port.Interface != "" {
		details = append(details, "if"+port.Interface)
	}
	if len(details) != 0 {
		port.Display += " (" + strings.Join(details, ", ") + ")"
	}
}

func readOptionalFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readOptionalLink(path string) (string, error) {
	target, err := os.Readlink(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return target, nil
}

func hasUeventValue(uevent, key, value string) bool {
	want := key + "=" + value
	for _, line := range strings.Split(uevent, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

func pathWithin(path, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
