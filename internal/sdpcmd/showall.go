package sdpcmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jaypipes/pcidb"
	"github.com/jclark/satpulse/internal/phc"
)

// InterfaceInfo represents a network interface with PHC (for list mode)
type InterfaceInfo struct {
	Name              string   `json:"name"`
	Driver            string   `json:"driver"`            // Network driver name
	PCISlot           string   `json:"pci_slot,omitempty"`   // PCI slot (e.g., "04:00.0")
	Vendor            string   `json:"vendor,omitempty"`     // PCI vendor name
	Device            string   `json:"device,omitempty"`     // PCI device name
	Revision          uint32   `json:"revision,omitempty"`   // PCI revision
	ClockIndex        int      `json:"clock_index"`       // PHC index from ethtool
	Pins              []string `json:"pins"`              // From /sys/class/ptp/ptpX/pins/ directory listing
	NumExttsChannels  int      `json:"n_extts_channels"`  // From /sys/class/ptp/ptpX/n_external_timestamps
	NumPeroutChannels int      `json:"n_perout_channels"` // From /sys/class/ptp/ptpX/n_periodic_outputs
}

func showAll() ([]Printer, error) {
	var result []Printer

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		phcIndex, err := phc.IfPhcIndex(iface.Name)
		if err != nil || phcIndex < 0 {
			// No PHC on this interface
			continue
		}

		// Get driver name
		driverName, _ := phc.IfDriverName(iface.Name)

		// Get PCI info
		pciSlot, vendor, device, revision := getPCIInfo(iface.Name)

		ptpPath := fmt.Sprintf("/sys/class/ptp/ptp%d", phcIndex)

		// Check for pins - try n_programmable_pins first, then n_pins
		nPins := readSysfsInt(filepath.Join(ptpPath, "n_programmable_pins"))
		if nPins <= 0 {
			nPins = readSysfsInt(filepath.Join(ptpPath, "n_pins"))
		}
		if nPins <= 0 {
			// No software-defined pins
			continue
		}

		// Read other PHC info
		nExtts := readSysfsInt(filepath.Join(ptpPath, "n_external_timestamps"))
		nPerout := readSysfsInt(filepath.Join(ptpPath, "n_periodic_outputs"))

		// List pin names from pins directory
		var pins []string
		pinsDir := filepath.Join(ptpPath, "pins")
		entries, err := os.ReadDir(pinsDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					pins = append(pins, entry.Name())
				}
			}
		}

		// Check consistency between n_pins and actual pin count
		if len(pins) != nPins {
			fmt.Fprintf(os.Stderr, "warning: %s: expected %d pins but found %d in /sys/class/ptp/ptp%d/pins/\n",
				iface.Name, nPins, len(pins), phcIndex)
		}

		info := InterfaceInfo{
			Name:              iface.Name,
			Driver:            driverName,
			PCISlot:           pciSlot,
			Vendor:            vendor,
			Device:            device,
			Revision:          revision,
			ClockIndex:        phcIndex,
			Pins:              pins,
			NumExttsChannels:  nExtts,
			NumPeroutChannels: nPerout,
		}
		result = append(result, &info)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no interfaces with software-defined pins found")
	}
	return result, nil
}

func readSysfsString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readSysfsInt(path string) int {
	s := readSysfsString(path)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func readHex(path string) (uint32, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(b))
	var v uint32
	_, err = fmt.Sscanf(s, "0x%x", &v)
	return v, err
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

// Print outputs the interface info in human-readable format
func (info *InterfaceInfo) Print(f *os.File) {
	fmt.Fprintf(f, "Interface: %s\n", info.Name)
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
