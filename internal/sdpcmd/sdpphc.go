package sdpcmd

import (
	"fmt"
	"strconv"

	"github.com/jclark/satpulse/internal/phc"
)

// phcOpen opens a PHC device for the given interface and resolves the pin
func phcOpen(ifname, pin string) (*phc.Clock, uint32, error) {
	// Get PHC index from interface name
	phcIndex, err := phc.IfPhcIndex(ifname)
	if err != nil {
		return nil, 0, err
	}
	if phcIndex < 0 {
		return nil, 0, fmt.Errorf("interface %s does not have a PTP hardware clock", ifname)
	}

	// Open PHC device
	clk, err := phc.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open PHC device: %w", err)
	}
	defer func() {
		if err != nil {
			clk.Close()
		}
	}()

	// Check if interface has pins
	if clk.PinCount() == 0 {
		err = fmt.Errorf("interface %s has no software-defined pins", ifname)
		return nil, 0, err
	}

	// Resolve pin name/index
	pinIndex, err := resolvePin(clk, ifname, pin)
	if err != nil {
		return nil, 0, err
	}

	return clk, uint32(pinIndex), nil
}

// resolvePin converts a pin name or index string to a pin index
func resolvePin(clk *phc.Clock, ifname, pin string) (int, error) {
	// Try to parse as integer first
	if index, err := strconv.Atoi(pin); err == nil {
		if index < 0 {
			return 0, fmt.Errorf("pin index cannot be negative (got %d)", index)
		}
		maxIndex := clk.PinCount() - 1
		if index > maxIndex {
			return 0, fmt.Errorf("pin index %d exceeds maximum (%d)", index, maxIndex)
		}
		return index, nil
	}

	// Not a number, treat as pin name
	for i := 0; i < clk.PinCount(); i++ {
		desc, err := clk.PinGetFunc(uint32(i))
		if err != nil {
			continue
		}
		if desc.Name == pin {
			return i, nil
		}
	}

	return 0, fmt.Errorf("pin name '%s' not found on interface %s", pin, ifname)
}