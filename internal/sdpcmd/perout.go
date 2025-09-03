package sdpcmd

import (
	"fmt"
	"log/slog"

	"github.com/jclark/satpulse/internal/phc"
)

func perout(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
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

	// Validate channel index
	maxChan := clk.PeroutChanCount()
	if cfg.Chan < 0 || cfg.Chan >= maxChan {
		return nil, fmt.Errorf("channel %d out of range (max %d)", cfg.Chan, maxChan-1)
	}

	// Configure pin for periodic output
	err = clk.PinSetFunc(uint32(pinIndex), phc.PinFuncPerout, uint32(cfg.Chan))
	if err != nil {
		return nil, fmt.Errorf("failed to configure pin %d for periodic output: %w", pinIndex, err)
	}

	// Enable periodic output with specified parameters
	err = clk.PeroutEnable(uint32(cfg.Chan), cfg.Perout.Period, cfg.Perout.Width)
	if err != nil {
		return nil, fmt.Errorf("failed to enable periodic output: %w", err)
	}

	// Log success message
	if cfg.Perout.Period == 0 {
		lg.Info("periodic output disabled",
			"interface", cfg.Interface,
			"pin", pinIndex,
			"channel", cfg.Chan)
	} else {
		if cfg.Perout.Width > 0 {
			lg.Info("periodic output enabled",
				"interface", cfg.Interface,
				"pin", pinIndex,
				"channel", cfg.Chan,
				"period", cfg.Perout.Period,
				"width", cfg.Perout.Width)
		} else {
			lg.Info("periodic output enabled",
				"interface", cfg.Interface,
				"pin", pinIndex,
				"channel", cfg.Chan,
				"period", cfg.Perout.Period,
				"width", "default")
		}
	}

	// No data output for perout mode
	return nil, nil
}
