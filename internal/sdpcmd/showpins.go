package sdpcmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/internal/phc"
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

func showPins(lg *slog.Logger, cfg *FlagConfig) ([]Printer, error) {
	var result []Printer

	// Get PHC index for interface
	phcIndex, err := phc.IfPhcIndex(cfg.Interface)
	if err != nil || phcIndex < 0 {
		return nil, fmt.Errorf("interface %s does not have a PTP hardware clock", cfg.Interface)
	}

	// Open PHC device to get pin info
	clk, err := phc.Open(fmt.Sprintf("/dev/ptp%d", phcIndex))
	if err != nil {
		return nil, fmt.Errorf("failed to open PHC device for %s: %w", cfg.Interface, err)
	}
	defer clk.Close()

	// Check for pins
	nPins := clk.PinCount()
	if nPins <= 0 {
		return nil, fmt.Errorf("interface %s has no software-defined pins", cfg.Interface)
	}

	// Query pin configurations
	for i := uint32(0); i < uint32(nPins); i++ {
		pinDesc, err := clk.PinGetFunc(i)
		if err != nil {
			continue
		}

		// Check if this pin matches the filter (if specified)
		if cfg.Pin != "" {
			indexStr := fmt.Sprintf("%d", i)
			if cfg.Pin != indexStr && cfg.Pin != pinDesc.Name {
				continue // Skip this pin
			}
		}

		pd := PinDesc{
			Index:    i,
			Name:     pinDesc.Name,
			Function: pinDesc.Func.String(),
			Channel:  pinDesc.Chan,
		}
		result = append(result, &pd)
	}

	// Check if we found any matching pins
	if cfg.Pin != "" && len(result) == 0 {
		return nil, fmt.Errorf("pin '%s' not found on interface %s", cfg.Pin, cfg.Interface)
	}

	return result, nil
}

