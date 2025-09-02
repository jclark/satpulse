package sdpcmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// Mode represents the operation mode for the sdp command
type Mode int

const (
	ModeShowIfaces Mode = iota // List all interfaces with PHC/pins
	ModeShowPins               // Show pins for specific interface
	ModeExtts                  // External timestamp monitoring (-i)
	ModePerout                 // Periodic output (-o)
	ModeDisable                // Disable pin (--disable)
)

// FlagConfig holds the parsed command-line flags and determined mode
type FlagConfig struct {
	Mode      Mode
	JSONL     bool
	Interface string
	
	// Extts mode flags
	Timeout   time.Duration
	ShowStale bool
	
	// Common pin selection
	Pin  string // Can be name or index
	Chan uint32
	
	// Perout mode flags (for future)
	PeroutWidth  float64
	PeroutPeriod float64
	PeroutPhase  float64
}

func parseFlags(cmdName string, args []string) (cfg *FlagConfig, help bool, usageFunc func(string) string, err error) {
	cfg = &FlagConfig{
		Timeout: 2 * time.Second, // Default timeout
		Chan:    0,                // Default channel
	}
	
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)

	// General flags
	flags.BoolVarP(&cfg.JSONL, "jsonl", "j", false, "output in JSON lines format")
	flags.BoolVarP(&help, "help", "h", false, "show help")
	
	// Mode flags
	var extts, disable, showFlag bool
	var timeoutSec float64 = 2.0
	var peroutWidth float64
	
	flags.BoolVarP(&extts, "extts", "i", false, "monitor external timestamp events (PPS input)")
	flags.Float64VarP(&timeoutSec, "timeout", "t", 2.0, "timeout for monitoring in seconds (0 for unlimited)")
	flags.BoolVar(&cfg.ShowStale, "show-stale", false, "include stale/buffered timestamps in output")
	
	flags.Float64VarP(&peroutWidth, "perout", "o", -1, "enable periodic output with pulse width in seconds")
	flags.Float64Var(&cfg.PeroutPeriod, "period", 1.0, "period for periodic output in seconds")
	flags.Float64Var(&cfg.PeroutPhase, "phase", 0, "phase offset for periodic output in seconds")
	
	flags.BoolVar(&disable, "disable", false, "disable the pin function")
	flags.BoolVar(&showFlag, "show", false, "show current configuration")
	
	// Common pin selection
	flags.StringVar(&cfg.Pin, "pin", "0", "pin index or name")
	var chanInt int
	flags.IntVar(&chanInt, "chan", 0, "channel index")

	err = flags.Parse(args)
	if err != nil {
		usageFunc = func(progName string) string {
			return usageString(progName, cmdName, flags)
		}
		return
	}

	usageFunc = func(progName string) string {
		return usageString(progName, cmdName, flags)
	}
	
	// Convert timeout to Duration
	cfg.Timeout = time.Duration(timeoutSec * float64(time.Second))
	cfg.Chan = uint32(chanInt)
	
	// Validate positional arguments
	if flags.NArg() > 1 {
		err = fmt.Errorf("too many arguments: expected at most 1 interface name, got %d arguments", flags.NArg())
		return
	}
	
	// Check for positional argument (interface name)
	if flags.NArg() > 0 {
		cfg.Interface = flags.Arg(0)
	}
	
	// Determine mode based on flags
	modeCount := 0
	if extts {
		cfg.Mode = ModeExtts
		modeCount++
	}
	if peroutWidth >= 0 {
		cfg.Mode = ModePerout
		cfg.PeroutWidth = peroutWidth
		modeCount++
	}
	if disable {
		cfg.Mode = ModeDisable
		modeCount++
	}
	if showFlag {
		if cfg.Interface != "" {
			cfg.Mode = ModeShowPins
		} else {
			cfg.Mode = ModeShowIfaces
		}
		modeCount++
	}
	
	// Check for conflicting modes
	if modeCount > 1 {
		err = fmt.Errorf("multiple modes specified; use only one of -i, -o, --disable, or --show")
		return
	}
	
	// Default mode based on interface
	if modeCount == 0 {
		if cfg.Interface != "" {
			cfg.Mode = ModeShowPins
		} else {
			cfg.Mode = ModeShowIfaces
		}
	}
	
	// Validate mode-specific requirements
	switch cfg.Mode {
	case ModeExtts, ModePerout, ModeDisable:
		if cfg.Interface == "" {
			err = fmt.Errorf("interface name required for this mode")
			return
		}
	}

	return
}

func usageString(progName string, cmdName string, flags *pflag.FlagSet) string {
	buf := &strings.Builder{}
	fmt.Fprintf(buf, "Usage: %s %s [options] [interface]\n", progName, cmdName)
	fmt.Fprintln(buf, "Options:")
	flags.SetOutput(buf)
	flags.PrintDefaults()
	return buf.String()
}