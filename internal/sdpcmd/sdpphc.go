package sdpcmd

import (
	"fmt"
	"strconv"

	"github.com/jclark/satpulse/time/phc"
)

// phcOpen opens the PHC that cfg selects and resolves the pin,
// which defaults to 0. When the PHC has no pins, the pin is unused: the
// channel is used directly, and specifying a pin is an error.
func phcOpen(cfg *FlagConfig) (*phc.Clock, uint32, error) {
	path, err := phcPath(cfg)
	if err != nil {
		return nil, 0, err
	}

	// Open PHC device
	clk, err := phc.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open PHC device: %w", err)
	}
	defer func() {
		if err != nil {
			clk.Close()
		}
	}()

	if cfg.Pin == "" {
		return clk, 0, nil
	}
	if !clk.HasPins() {
		err = noPinsError(cfg)
		return nil, 0, err
	}

	// Resolve pin name/index
	pinIndex, err := resolvePin(clk, cfg.phcName(), cfg.Pin)
	if err != nil {
		return nil, 0, err
	}
	return clk, uint32(pinIndex), nil
}

// phcPath returns the device path of the PHC that cfg selects.
func phcPath(cfg *FlagConfig) (string, error) {
	if cfg.Device != "" {
		return cfg.Device, nil
	}
	index, err := ifPHCIndex(cfg.Interface)
	if err != nil {
		return "", err
	}
	return phc.ClockPath(index), nil
}

// ifPHCIndex resolves an interface's PHC index, requiring a PHC.
func ifPHCIndex(ifname string) (int, error) {
	index, err := phc.IfPHCIndex(ifname)
	if err != nil {
		return 0, err
	}
	if index < 0 {
		return 0, fmt.Errorf("interface %s does not have a PTP hardware clock", ifname)
	}
	return index, nil
}

func noPinsError(cfg *FlagConfig) error {
	return fmt.Errorf("%s has no pins", cfg.phcName())
}

// phcName describes the PHC that cfg selects, for messages.
func (cfg *FlagConfig) phcName() string {
	if cfg.Device != "" {
		return cfg.Device
	}
	return "the PHC of interface " + cfg.Interface
}

// logAttrs returns the log attributes naming the PHC that cfg selects.
func (cfg *FlagConfig) logAttrs() []any {
	if cfg.Device != "" {
		return []any{"device", cfg.Device}
	}
	return []any{"interface", cfg.Interface}
}

// resolvePin converts a pin name or index string to a pin index
func resolvePin(clk *phc.Clock, phcName, pin string) (int, error) {
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

	return 0, fmt.Errorf("pin name '%s' not found on %s", pin, phcName)
}
