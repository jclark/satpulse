// Package serialenum enumerates serial ports with human-readable display names.
package serialenum

import (
	"fmt"
	"runtime"

	"go.bug.st/serial/enumerator"
)

// Port describes a serial port for display in a dropdown or CLI listing.
type Port struct {
	Device  string `json:"device"`  // device path passed to Connect
	Display string `json:"display"` // human-readable label for the dropdown
}

// List enumerates serial ports available on the system.
// On macOS, non-USB ports (Bluetooth, debug-console) are filtered out.
func List() ([]Port, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("enumerating serial ports: %w", err)
	}
	var result []Port
	for _, p := range ports {
		if runtime.GOOS == "darwin" && !p.IsUSB {
			continue
		}
		result = append(result, Port{
			Device:  p.Name,
			Display: display(p),
		})
	}
	return result, nil
}

// display builds a human-readable label from port details.
func display(p *enumerator.PortDetails) string {
	tag := ubloxTag(p.VID, p.PID)
	if p.Product != "" {
		if tag != "" {
			return p.Product + " - " + tag
		}
		return p.Product
	}
	if tag != "" {
		return p.Name + " (" + tag + ")"
	}
	return p.Name
}

// ubloxTag returns a u-blox generation tag from USB VID/PID, or empty string.
func ubloxTag(vid, pid string) string {
	if vid != "1546" {
		return ""
	}
	var pidVal uint16
	if _, err := fmt.Sscanf(pid, "%x", &pidVal); err != nil {
		return "u-blox"
	}
	if pidVal >= 0x01A4 && pidVal <= 0x01AF {
		return fmt.Sprintf("u-blox gen %d", pidVal-0x01A0)
	}
	return "u-blox"
}
