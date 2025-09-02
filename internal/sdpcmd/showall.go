package sdpcmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jclark/satpulse/internal/phc"
)

// InterfaceInfo represents a network interface with PHC (for list mode)
type InterfaceInfo struct {
	Name              string   `json:"name"`
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

// Print outputs the interface info in human-readable format
func (info *InterfaceInfo) Print(f *os.File) {
	fmt.Fprintf(f, "Interface: %s\n", info.Name)
	fmt.Fprintf(f, "  PTP clock device: /dev/ptp%d\n", info.ClockIndex)
	fmt.Fprintf(f, "  Pins: %s\n", strings.Join(info.Pins, ", "))
	fmt.Fprintf(f, "  External timestamp channels: %d\n", info.NumExttsChannels)
	fmt.Fprintf(f, "  Periodic output channels: %d\n", info.NumPeroutChannels)
}
