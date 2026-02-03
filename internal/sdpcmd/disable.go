package sdpcmd

import (
	"fmt"
	"log/slog"

	"github.com/jclark/satpulse/time/phc"
)

func disable(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	// Open PHC and resolve pin
	clk, pinIndex, err := phcOpen(cfg.Interface, cfg.Pin)
	if err != nil {
		return nil, err
	}
	defer clk.Close()
	
	// Disable pin by setting function to NONE
	// Channel doesn't matter for NONE function, but we use 0 for consistency
	err = clk.PinSetFunc(pinIndex, phc.PinFuncNone, 0)
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