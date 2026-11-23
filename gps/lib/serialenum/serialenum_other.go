//go:build !linux && !freebsd

package serialenum

import (
	"fmt"
	"runtime"

	"go.bug.st/serial/enumerator"
)

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
		usb := parseEnumeratorUSBID(p.VID, p.PID)
		result = append(result, Port{
			Device:  p.Name,
			Display: enumeratorDisplay(p.Name, p.Product, usb.VID, usb.PID),
			USB:     usb,
			Serial:  p.SerialNumber,
		})
	}
	return result, nil
}
