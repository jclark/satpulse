package sdpcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
)

// NoDataError represents a condition where no data was found (exit code 2)
type NoDataError struct {
	msg string
}

func (e NoDataError) Error() string {
	return e.msg
}

func (e NoDataError) ExitCode() int {
	return 2
}

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

// Cmd executes the sdp subcommand with the given arguments
func Cmd(logWriter io.Writer, logLevel slog.Level, progName string, cmdName string, cmdArgs []string) (usage string, err error) {
	lg := cmd.NewDefaultLogger(logWriter, logLevel)
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
			ModePerout:     perout,
			ModeDisable:    disable,
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


