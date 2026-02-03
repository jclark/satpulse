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

// showPins implements --show mode when an an interface is specified
func showPins(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	var result []Printer

	phcIndex, err := phc.IfPhcIndex(cfg.Interface)
	if err != nil || phcIndex < 0 {
		return nil, fmt.Errorf("interface %s does not have a PTP hardware clock", cfg.Interface)
	}

	clk, err := phc.Open(fmt.Sprintf("/dev/ptp%d", phcIndex))
	if err != nil {
		return nil, fmt.Errorf("failed to open PHC device for %s: %w", cfg.Interface, err)
	}
	defer clk.Close()

	nPins := clk.PinCount()
	if nPins <= 0 {
		return nil, fmt.Errorf("interface %s has no software-defined pins", cfg.Interface)
	}

	for i := range uint32(nPins) {
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