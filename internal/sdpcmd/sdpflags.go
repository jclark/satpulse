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
	Pin  string // Can be name or index, empty string means not specified
	Chan int
	
	// Perout mode flags
	PeroutWidth  float64 // 0 means --width not specified
	PeroutPeriod float64
	PeroutPhase  float64
}

func parseFlags(cmdName string, args []string) (cfg *FlagConfig, help bool, usageFunc func(string) string, err error) {
	cfg = &FlagConfig{
		Mode:    ModeShowPins,     // Default mode
		Timeout: 2 * time.Second, // Default timeout
		Chan:    0,                // Default channel
		Pin:     "",               // Empty means not specified
	}
	
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)

	// General flags
	flags.BoolVarP(&cfg.JSONL, "jsonl", "j", false, "output in JSON lines format instead of human-readable text")
	flags.BoolVarP(&help, "help", "h", false, "show usage help for the sdp command")
	
	// Mode flags
	var extts, disable, showFlag, perout bool
	var timeoutSec float64 = 2.0
	var widthSpecified bool
	
	flags.BoolVarP(&extts, "extts", "i", false, "check input on a pin")
	flags.Float64VarP(&timeoutSec, "timeout", "t", 2.0, "length of time in seconds for which to read timestamp events (0 for unlimited)")
	flags.BoolVar(&cfg.ShowStale, "show-stale", false, "include stale timestamps in the output")
	
	flags.BoolVarP(&perout, "perout", "o", false, "enable periodic output on a pin")
	flags.Float64VarP(&cfg.PeroutWidth, "width", "w", 0, "set the pulse width for periodic output in seconds")
	flags.Float64Var(&cfg.PeroutPeriod, "period", 1.0, "set the pulse period for periodic output in seconds")
	flags.Float64Var(&cfg.PeroutPhase, "phase", 0, "set the pulse phase offset for periodic output in seconds")
	
	flags.BoolVar(&disable, "disable", false, "disable the pin function")
	flags.BoolVar(&showFlag, "show", false, "show information about SDPs")
	
	// Common pin selection
	flags.StringVar(&cfg.Pin, "pin", "", "select the pin to be used (index or name)")
	flags.IntVar(&cfg.Chan, "chan", 0, "select the channel to be used")

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
	
	// Check if --width was specified
	widthSpecified = flags.Lookup("width").Changed
	
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
	if perout {
		cfg.Mode = ModePerout
		modeCount++
	}
	if disable {
		cfg.Mode = ModeDisable
		modeCount++
	}
	if showFlag {
		// Mode is already set to ModeShowPins by default
		modeCount++
	}
	
	// Check for conflicting modes
	if modeCount > 1 {
		err = fmt.Errorf("multiple modes specified; use only one of -i, -o, --disable, or --show")
		return
	}
	
	// Adjust show mode based on interface presence
	if cfg.Mode == ModeShowPins && cfg.Interface == "" {
		cfg.Mode = ModeShowIfaces
	}
	
	// Validate mode-specific requirements
	switch cfg.Mode {
	case ModeExtts, ModePerout, ModeDisable:
		if cfg.Interface == "" {
			err = fmt.Errorf("interface name required for this mode")
			return
		}
		// Set default pin if not specified
		if cfg.Pin == "" && !flags.Lookup("pin").Changed {
			cfg.Pin = "0"
		}
	}
	
	// Check for mode-specific flags used with wrong modes
	// Check for perout-only flags used without perout mode
	if cfg.Mode != ModePerout {
		if widthSpecified {
			err = fmt.Errorf("--width can only be used with -o/--perout")
			return
		}
		if flags.Lookup("period").Changed {
			err = fmt.Errorf("--period can only be used with -o/--perout")
			return
		}
		if flags.Lookup("phase").Changed {
			err = fmt.Errorf("--phase can only be used with -o/--perout")
			return
		}
	}
	
	// Check for extts-only flags used without extts mode
	if cfg.Mode != ModeExtts {
		if flags.Lookup("timeout").Changed && timeoutSec != 2.0 {
			err = fmt.Errorf("--timeout can only be used with -i/--extts")
			return
		}
		if flags.Lookup("show-stale").Changed {
			err = fmt.Errorf("--show-stale can only be used with -i/--extts")
			return
		}
	}

	// Additional validation for perout mode
	if cfg.Mode == ModePerout {
		// Validate perout specific parameters
		if widthSpecified {
			// --width was explicitly specified
			if cfg.PeroutWidth == 0 {
				err = fmt.Errorf("--width cannot be explicitly set to 0; omit the flag instead")
				return
			}
			if cfg.PeroutWidth < 0 {
				err = fmt.Errorf("pulse width cannot be negative (got %f seconds)", cfg.PeroutWidth)
				return
			}
			if cfg.PeroutWidth >= cfg.PeroutPeriod {
				err = fmt.Errorf("pulse width (%f seconds) must be less than period (%f seconds)", cfg.PeroutWidth, cfg.PeroutPeriod)
				return
			}
		}
		// If --width not specified, cfg.PeroutWidth remains 0 (no duty cycle)
		
		if cfg.PeroutPeriod <= 0 {
			err = fmt.Errorf("period must be positive (got %f seconds)", cfg.PeroutPeriod)
			return
		}
		if cfg.PeroutPhase < 0 {
			err = fmt.Errorf("phase offset cannot be negative (got %f seconds)", cfg.PeroutPhase)
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