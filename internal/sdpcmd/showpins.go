package sdpcmd

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/jclark/satpulse/time/phc"
)

// PinDesc represents a single pin's configuration (for show with interface)
type PinDesc struct {
	Name     string `json:"name"`
	Index    uint32 `json:"index"`
	Function string `json:"function"` // "none", "extts", "perout", "physync"
	Channel  uint32 `json:"channel"`
}

// Compile-time assertion that PinDesc implements Printer
var _ Printer = (*PinDesc)(nil)

// Print outputs the pin description in human-readable format
func (p *PinDesc) Print(f *os.File) {
	fmt.Fprintf(f, "Pin %s\n", p.Name)
	fmt.Fprintf(f, "  Pin index: %d\n", p.Index)
	fmt.Fprintf(f, "  Function: %s\n", p.Function)
	if p.Function != "none" {
		fmt.Fprintf(f, "  Channel: %d\n", p.Channel)
	}
}

// showAuto implements the default mode with an interface or device: the
// pins of the PHC, or the interface record if the PHC has none
func showAuto(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	if cfg.Device != "" {
		return showDevice(cfg)
	}
	phcIndex, err := ifPHCIndex(cfg.Interface)
	if err != nil {
		return nil, err
	}
	if sysfsPinCount(os.DirFS("/sys"), phcIndex) > 0 {
		return showPinsByPath(cfg, phc.ClockPath(phcIndex))
	}
	return showIface(lg, cfg.Interface, phcIndex)
}

// showDevice shows the pins of the PHC given by its device path. For a PHC
// without pins, the default mode shows interfaces, which is not supported
// with a device.
func showDevice(cfg *FlagConfig) ([]Printer, error) {
	clk, err := phc.Open(cfg.Device)
	if err != nil {
		return nil, fmt.Errorf("failed to open PHC device: %w", err)
	}
	defer clk.Close()
	if !clk.HasPins() {
		return nil, errShowIfacesDevice
	}
	return listPins(cfg, clk)
}

// showPins implements --show-pins mode
func showPins(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	path, err := phcPath(cfg)
	if err != nil {
		return nil, err
	}
	return showPinsByPath(cfg, path)
}

// showPinsByPath shows the pins of the PHC that cfg selects, whose device
// path has already been looked up
func showPinsByPath(cfg *FlagConfig, path string) ([]Printer, error) {
	clk, err := phc.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open PHC device: %w", err)
	}
	defer clk.Close()
	if !clk.HasPins() {
		return nil, noPinsError(cfg)
	}
	return listPins(cfg, clk)
}

// listPins shows the pins of clk, which has pins
func listPins(cfg *FlagConfig, clk *phc.Clock) ([]Printer, error) {
	var result []Printer
	for i := range uint32(clk.PinCount()) {
		pinDesc, err := clk.PinGetFunc(i)
		if err != nil {
			return nil, fmt.Errorf("failed to get pin %d description: %w", i, err)
		}

		if cfg.Pin != "" && !matchesPinDesc(cfg.Pin, pinDesc) {
			continue
		}

		pd := PinDesc{
			Index:    i,
			Name:     pinDesc.Name,
			Function: pinDesc.Func.String(),
			Channel:  pinDesc.Chan,
		}
		result = append(result, &pd)
	}

	if cfg.Pin != "" && len(result) == 0 {
		return nil, fmt.Errorf("pin '%s' not found on %s", cfg.Pin, cfg.phcName())
	}

	return result, nil
}

// matchesPinDesc returns true if the pin matches by index or name
func matchesPinDesc(pin string, desc *phc.PinDesc) bool {
	// Try to parse pin as unsigned integer for index matching
	if idx, err := strconv.ParseUint(pin, 10, 32); err == nil {
		return uint32(idx) == desc.Index
	}
	// Otherwise compare as string name
	return pin == desc.Name
}