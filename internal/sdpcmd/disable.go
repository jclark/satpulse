package sdpcmd

import (
	"fmt"
	"log/slog"

	"github.com/jclark/satpulse/internal/phc"
)

func disable(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	// Get PHC index from interface name
	phcIndex, err := phc.IfPhcIndex(cfg.Interface)
	if err != nil {
		return nil, err
	}
	if phcIndex < 0 {
		return nil, fmt.Errorf("interface %s does not have a PTP hardware clock", cfg.Interface)
	}
	
	// Open PHC device
	clk, err := phc.Open(phc.ClockPath(phcIndex))
	if err != nil {
		return nil, fmt.Errorf("failed to open PHC device: %w", err)
	}
	defer clk.Close()
	
	// Check if interface has pins
	if clk.PinCount() == 0 {
		return nil, fmt.Errorf("interface %s has no software-defined pins", cfg.Interface)
	}
	
	// Parse pin (could be index or name)
	pinIndex, err := resolvePinIndex(clk, cfg.Pin)
	if err != nil {
		return nil, err
	}
	
	// Disable pin by setting function to NONE
	// Channel doesn't matter for NONE function, but we use 0 for consistency
	err = clk.PinSetFunc(uint32(pinIndex), phc.PinFuncNone, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to disable pin %d: %w", pinIndex, err)
	}
	
	// Log success message
	lg.Info("pin disabled",
		"interface", cfg.Interface,
		"pin", pinIndex)
	
	// No data output for disable mode
	return nil, nil
}