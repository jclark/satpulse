package sdpcmd

import (
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"os"
	"strconv"
	
	"github.com/jclark/satpulse/internal/phc"
)


type Printer interface {
	Print(f *os.File)
}

var _ Printer = (*InterfaceInfo)(nil)

// sliceToIter converts a slice of Printers to an iterator
func sliceToIter(items []Printer) iter.Seq[Printer] {
	return func(yield func(Printer) bool) {
		for _, item := range items {
			if !yield(item) {
				return
			}
		}
	}
}

func Cmd(lg *slog.Logger, progName string, cmdName string, cmdArgs []string) (usage string, err error) {
	cfg, help, usageFunc, err := parseFlags(cmdName, cmdArgs)
	if err != nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}
	if help {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}

	// Special case for streaming modes vs static modes
	var output iter.Seq[Printer]
	var iterErr error
	
	if cfg.Mode == ModeExtts {
		// Streaming mode - returns iterator directly
		output = extts(lg, cfg, &iterErr)
	} else {
		// Static modes - look up in map and convert slice to iterator
		modeHandlers := map[Mode]func(*slog.Logger, *FlagConfig) ([]Printer, error){
			ModeShowIfaces: showIfaces,
			ModeShowPins:   showPins,
			// ModePerout and ModeDisable will be added in future phases
		}
		
		handler, ok := modeHandlers[cfg.Mode]
		if !ok {
			panic(fmt.Sprintf("mode %v not implemented", cfg.Mode))
		}
		
		sections, err := handler(lg, cfg)
		if err != nil {
			return "", err
		}
		output = sliceToIter(sections)
	}

	// Single output loop for all modes
	for item := range output {
		if cfg.JSONL {
			b, err := json.Marshal(item)
			if err != nil {
				lg.Debug("failed to marshal item", "error", err)
				continue
			}
			fmt.Println(string(b))
		} else {
			item.Print(os.Stdout)
		}
	}

	// Check if iterator had an error
	if iterErr != nil {
		return "", iterErr
	}

	return
}

// resolvePinIndex converts a pin name or index string to a pin index
func resolvePinIndex(clk *phc.Clock, pin string) (int, error) {
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
	
	return 0, fmt.Errorf("pin name '%s' not found on this interface", pin)
}

// validateExttsChannel validates a channel for external timestamp use
func validateExttsChannel(clk *phc.Clock, channel int) error {
	if channel < 0 {
		return fmt.Errorf("channel index cannot be negative (got %d)", channel)
	}
	maxChannel := clk.ExttsChanCount() - 1
	if channel > maxChannel {
		if maxChannel < 0 {
			return fmt.Errorf("this PHC has no external timestamp channels")
		}
		return fmt.Errorf("channel index %d exceeds maximum (%d) for external timestamps", channel, maxChannel)
	}
	return nil
}

// validatePeroutChannel validates a channel for periodic output use
func validatePeroutChannel(clk *phc.Clock, channel int) error {
	if channel < 0 {
		return fmt.Errorf("channel index cannot be negative (got %d)", channel)
	}
	maxChannel := clk.PeroutChanCount() - 1
	if channel > maxChannel {
		if maxChannel < 0 {
			return fmt.Errorf("this PHC has no periodic output channels")
		}
		return fmt.Errorf("channel index %d exceeds maximum (%d) for periodic output", channel, maxChannel)
	}
	return nil
}

