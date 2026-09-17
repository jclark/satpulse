package sdpcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jaypipes/pcidb"
	"github.com/jclark/satpulse/time/phc"
)

// InterfaceInfo represents a network interface with PHC (for list mode)
type InterfaceInfo struct {
	Name              string   `json:"name"`
	Status            string   `json:"status"`             // Interface status (up/down, carrier/no-carrier)
	Driver            string   `json:"driver"`             // Network driver name
	PCISlot           string   `json:"pci_slot,omitempty"` // PCI slot (e.g., "04:00.0")
	Vendor            string   `json:"vendor,omitempty"`   // PCI vendor name
	Device            string   `json:"device,omitempty"`   // PCI device name
	Revision          uint32   `json:"revision,omitempty"` // PCI revision
	ClockIndex        int      `json:"clock_index"`        // PHC index from ethtool
	Pins              []string `json:"pins"`               // From /sys/class/ptp/ptpX/pins/ directory listing
	NumExttsChannels  int      `json:"n_extts_channels"`   // From /sys/class/ptp/ptpX/n_external_timestamps
	NumPeroutChannels int      `json:"n_perout_channels"`  // From /sys/class/ptp/ptpX/n_periodic_outputs
}

// Print outputs the interface info in human-readable format
func (info *InterfaceInfo) Print(f *os.File) {
	fmt.Fprintf(f, "Interface: %s\n", info.Name)
	fmt.Fprintf(f, "  Status: %s\n", info.Status)
	if info.Driver != "" {
		fmt.Fprintf(f, "  Driver: %s\n", info.Driver)
	}
	if info.PCISlot != "" {
		fmt.Fprintf(f, "  PCI: %s\n", info.PCISlot)
	}
	if info.Vendor != "" {
		fmt.Fprintf(f, "  Vendor: %s\n", info.Vendor)
	}
	if info.Device != "" {
		fmt.Fprintf(f, "  Device: %s\n", info.Device)
	}
	if info.Revision > 0 {
		fmt.Fprintf(f, "  Revision: %02x\n", info.Revision)
	}
	fmt.Fprintf(f, "  PTP clock device: /dev/ptp%d\n", info.ClockIndex)
	fmt.Fprintf(f, "  Pins: %s\n", strings.Join(info.Pins, ", "))
	fmt.Fprintf(f, "  External timestamp channels: %d\n", info.NumExttsChannels)
	fmt.Fprintf(f, "  Periodic output channels: %d\n", info.NumPeroutChannels)
}

// showIfaces implements --show mode when no interface is specified
func showIfaces(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	var result []Printer

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	fsys := os.DirFS("/sys")

	for _, iface := range interfaces {
		phcIndex, err := phc.IfPhcIndex(iface.Name)
		if err != nil || phcIndex < 0 {
			// No PHC on this interface
			continue
		}

		info, err := getInterfaceInfo(fsys, iface.Name, phcIndex)
		if err != nil {
			lg.Warn("PHC info error",
				"interface", iface.Name,
				"phc", phcIndex,
				"error", err)
		}
		if info == nil {
			continue // No usable data (no pins or fatal error)
		}

		// Add non-testable fields
		info.Status = formatStatus(iface.Flags)
		info.Driver, _ = phc.IfDriverName(iface.Name)

		pciSlot, vendor, device, revision := getPCIInfo(iface.Name)
		info.PCISlot = pciSlot
		info.Vendor = vendor
		info.Device = device
		info.Revision = revision

		result = append(result, info)
	}

	if len(result) == 0 {
		return nil, NoDataError{msg: "no interfaces with software-defined pins found"}
	}
	return result, nil
}

func formatStatus(flags net.Flags) string {
	up := flags&net.FlagUp != 0
	carrier := flags&net.FlagRunning != 0

	if up {
		if carrier {
			return "up, carrier"
		}
		return "up, no-carrier"
	}
	return "down"
}

// getInterfaceInfo gathers PHC information from sysfs (testable)
// Returns nil, nil if interface has no software-defined pins (not an error)
// May return non-nil info AND error if data is incomplete but usable
func getInterfaceInfo(fsys fs.FS, ifaceName string, phcIndex int) (*InterfaceInfo, error) {
	ptpPath := fmt.Sprintf("class/ptp/ptp%d", phcIndex)

	// Check for pins
	nPins := readSysfsInt(fsys, path.Join(ptpPath, "n_programmable_pins"))
	if nPins <= 0 {
		nPins = readSysfsInt(fsys, path.Join(ptpPath, "n_pins"))
	}
	if nPins <= 0 {
		return nil, nil // No software-defined pins - not an error
	}

	pins, err := readPinNames(fsys, ptpPath)
	if err != nil {
		return nil, fmt.Errorf("reading pin names for %s: %w", ifaceName, err)
	}

	info := &InterfaceInfo{
		Name:              ifaceName,
		ClockIndex:        phcIndex,
		Pins:              pins,
		NumExttsChannels:  readSysfsInt(fsys, path.Join(ptpPath, "n_external_timestamps")),
		NumPeroutChannels: readSysfsInt(fsys, path.Join(ptpPath, "n_periodic_outputs")),
	}

	// Check for inconsistency
	if len(pins) != nPins {
		return info, fmt.Errorf("pin count mismatch: expected %d, found %d", nPins, len(pins))
	}

	return info, nil
}


func readSysfsInt(fsys fs.FS, name string) int {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return n
}

func readHex(name string) (uint32, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(b))
	var v uint32
	_, err = fmt.Sscanf(s, "0x%x", &v)
	return v, err
}

// readPinNames reads pin names from the pins directory
func readPinNames(fsys fs.FS, ptpPath string) ([]string, error) {
	var pins []string
	pinsDir := path.Join(ptpPath, "pins")
	entries, err := fs.ReadDir(fsys, pinsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil // Directory doesn't exist, return empty
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			pins = append(pins, entry.Name())
		}
	}
	return pins, nil
}

func getPCIInfo(ifname string) (slot string, vendor string, device string, revision uint32) {
	link := "/sys/class/net/" + ifname + "/device"
	devPath, err := filepath.EvalSymlinks(link)
	if err != nil {
		return
	}
	slot = filepath.Base(devPath)

	vid, err := readHex(filepath.Join(devPath, "vendor"))
	if err != nil {
		return
	}
	did, err := readHex(filepath.Join(devPath, "device"))
	if err != nil {
		return
	}
	revision, _ = readHex(filepath.Join(devPath, "revision"))

	// Load PCI database
	db, err := pcidb.New()
	if err != nil {
		// Fall back to hex IDs if can't load database
		vendor = fmt.Sprintf("Vendor %04x", vid)
		device = fmt.Sprintf("Device %04x", did)
		return
	}

	// Look up vendor name
	if v := db.Vendors[fmt.Sprintf("%04x", vid)]; v != nil {
		vendor = v.Name
		// Look up device name
		for _, p := range v.Products {
			if p.ID == fmt.Sprintf("%04x", did) {
				device = p.Name
				break
			}
		}
		if device == "" {
			device = fmt.Sprintf("Device %04x", did)
		}
	} else {
		vendor = fmt.Sprintf("Vendor %04x", vid)
		device = fmt.Sprintf("Device %04x", did)
	}

	return
}

