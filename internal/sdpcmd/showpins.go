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

// showAuto implements the default mode with an interface: the pins of its
// PHC, or the interface record if the PHC has none
func showAuto(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	phcIndex, err := ifPHCIndex(cfg.Interface)
	if err != nil {
		return nil, err
	}
	if sysfsPinCount(os.DirFS("/sys"), phcIndex) > 0 {
		return showPinsByPHCIndex(cfg, phcIndex)
	}
	return showIface(lg, cfg.Interface, phcIndex)
}

// showPins implements --show-pins mode
func showPins(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	phcIndex, err := ifPHCIndex(cfg.Interface)
	if err != nil {
		return nil, err
	}
	return showPinsByPHCIndex(cfg, phcIndex)
}

// showPinsByPHCIndex shows the pins of the interface's PHC, whose index has
// already been looked up
func showPinsByPHCIndex(cfg *FlagConfig, phcIndex int) ([]Printer, error) {
	var result []Printer

	clk, err := phc.Open(fmt.Sprintf("/dev/ptp%d", phcIndex))
	if err != nil {
		return nil, fmt.Errorf("failed to open PHC device for %s: %w", cfg.Interface, err)
	}
	defer clk.Close()

	if !clk.HasPins() {
		return nil, noPinsError(cfg.Interface)
	}

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
		return nil, fmt.Errorf("pin '%s' not found on interface %s", cfg.Pin, cfg.Interface)
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