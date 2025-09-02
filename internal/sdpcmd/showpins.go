package sdpcmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jclark/satpulse/internal/phc"
	"golang.org/x/sys/unix"
)

// PinStatus represents a single pin's configuration (for show with interface)
type PinStatus struct {
	Name     string `json:"name"`
	Index    uint32 `json:"index"`
	Function string `json:"function"` // "none", "extts", "perout", "physync"
	Channel  uint32 `json:"channel"`
}

// Compile-time assertion that PinStatus implements Printer
var _ Printer = (*PinStatus)(nil)

// Print outputs the pin status in human-readable format
func (p *PinStatus) Print(f *os.File) {
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

	ptpPath := fmt.Sprintf("/sys/class/ptp/ptp%d", phcIndex)

	// Check for pins - try n_programmable_pins first, then n_pins
	nPins := readSysfsInt(fmt.Sprintf("%s/n_programmable_pins", ptpPath))
	if nPins <= 0 {
		nPins = readSysfsInt(fmt.Sprintf("%s/n_pins", ptpPath))
	}
	if nPins <= 0 {
		return nil, fmt.Errorf("interface %s has no software-defined pins", cfg.Interface)
	}

	// Try to get detailed pin info (requires root)
	if os.Geteuid() == 0 {
		// Running as root, can open PHC device
		clk, err := phc.Open(fmt.Sprintf("/dev/ptp%d", phcIndex))
		if err == nil {
			defer clk.Close()

			// Query pin configurations
			for i := uint32(0); i < uint32(nPins); i++ {
				pinDesc, err := clk.PinGetFunc(i)
				if err != nil {
					continue
				}

				// Convert function to string
				funcStr := pinFunctionToString(pinDesc.Func)

				// Convert name from byte array to string
				nameBytes := pinDesc.Name[:]
				nameLen := 0
				for j, b := range nameBytes {
					if b == 0 {
						nameLen = j
						break
					}
				}
				name := string(nameBytes[:nameLen])

				status := &PinStatus{
					Index:    i,
					Name:     name,
					Function: funcStr,
					Channel:  pinDesc.Chan,
				}
				result = append(result, status)
			}

			if len(result) > 0 {
				return result, nil
			}
		}
	}

	// Non-root path: read from sysfs pins directory
	pinsDir := fmt.Sprintf("%s/pins", ptpPath)
	entries, err := os.ReadDir(pinsDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read pin information for %s (try running as root)", cfg.Interface)
	}

	for i, entry := range entries {
		if entry.IsDir() {
			continue
		}

		pinPath := fmt.Sprintf("%s/%s", pinsDir, entry.Name())
		data := readSysfsString(pinPath)

		// Parse "function channel" format
		parts := strings.Fields(data)
		funcStr := "unknown"
		channel := uint32(0)

		if len(parts) >= 1 {
			switch parts[0] {
			case "0":
				funcStr = "none"
			case "1":
				funcStr = "extts"
			case "2":
				funcStr = "perout"
			case "3":
				funcStr = "physync"
			}
		}

		if len(parts) >= 2 {
			var ch int
			fmt.Sscanf(parts[1], "%d", &ch)
			channel = uint32(ch)
		}

		status := &PinStatus{
			Index:    uint32(i),
			Name:     entry.Name(),
			Function: funcStr,
			Channel:  channel,
		}
		result = append(result, status)
	}

	return result, nil
}

func pinFunctionToString(f uint32) string {
	switch f {
	case unix.PTP_PF_NONE:
		return "none"
	case unix.PTP_PF_EXTTS:
		return "extts"
	case unix.PTP_PF_PEROUT:
		return "perout"
	case unix.PTP_PF_PHYSYNC:
		return "physync"
	default:
		return fmt.Sprintf("unknown(%d)", f)
	}
}
