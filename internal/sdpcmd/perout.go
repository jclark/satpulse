package sdpcmd

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/phc"
)

// perout implements --perout/-o mode
func perout(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	clk, pinIndex, err := phcOpen(cfg.Interface, cfg.Pin)
	if err != nil {
		return nil, err
	}
	defer clk.Close()

	chanIndex, err := validatePeroutChannel(clk, cfg.Chan)
	if err != nil {
		return nil, err
	}

	err = clk.PinSetFunc(pinIndex, phc.PinFuncPerout, chanIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to configure pin %d for periodic output: %w", pinIndex, err)
	}

	driver, err := phc.IfDriverName(cfg.Interface)
	if err != nil {
		return nil, fmt.Errorf("failed to get driver name for interface %s: %w", cfg.Interface, err)
	}
	startOff := time.Duration(0)
	switch driver {
	case "igb", "igc":
		// igb and igc always use 50% duty cycle
		// The start in PTP_PEROUT_ENABLE determines the start of the *low* part i.e. the falling edge.
		// But we want the rising edge to be aligned with the start of the second.
		startOff = cfg.Perout.Period / 2
	}
	err = clk.PeroutEnable(chanIndex, cfg.Perout.Period, cfg.Perout.Width, startOff)
	if err != nil {
		return nil, fmt.Errorf("failed to enable periodic output: %w", err)
	}

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

	return nil, nil
}

// validatePeroutChannel validates a channel for periodic output use
func validatePeroutChannel(clk *phc.Clock, channel int) (uint32, error) {
	if channel < 0 {
		return 0, fmt.Errorf("channel index cannot be negative (got %d)", channel)
	}
	maxChannel := clk.PeroutChanCount() - 1
	if channel > maxChannel {
		if maxChannel < 0 {
			return 0, fmt.Errorf("this PHC has no periodic output channels")
		}
		return 0, fmt.Errorf("channel index %d exceeds maximum (%d) for periodic output", channel, maxChannel)
	}
	return uint32(channel), nil
}
