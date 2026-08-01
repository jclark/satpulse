// Package serialcmd implements the satpulsetool serial subcommand.
package serialcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/lib/serialenum"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [-j|--jsonl]`

// noPortsError makes an empty enumeration distinguishable from a command
// failure to callers and scripts.
type noPortsError struct{}

func (noPortsError) Error() string { return "no serial ports found" }
func (noPortsError) ExitCode() int { return 2 }

type flags struct {
	jsonl bool
}

// Cmd executes the serial subcommand.
func Cmd(_ io.Writer, _ slog.Level, progName, cmdName string, args []string) (usage string, err error) {
	cfg, help, usageFunc, err := parseFlags(cmdName, args)
	if err != nil {
		return usageFunc(progName), err
	}
	if help {
		return usageFunc(progName), nil
	}
	ports, err := serialenum.List()
	if err != nil {
		return "", err
	}
	if len(ports) == 0 {
		return "", noPortsError{}
	}
	return "", printPorts(os.Stdout, ports, cfg.jsonl)
}

func parseFlags(cmdName string, args []string) (cfg flags, help bool, usageFunc func(string) string, err error) {
	fs := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	fs.BoolVarP(&cfg.jsonl, "jsonl", "j", false, "output one JSON object per serial port")
	fs.BoolVarP(&help, "help", "h", false, "show usage help for the serial command")
	usageFunc = cmd.UsageFunc(cmdName, summary, fs)
	if err = fs.Parse(args); err != nil {
		return
	}
	if fs.NArg() != 0 {
		err = fmt.Errorf("serial enumeration takes no arguments")
	}
	return
}

func printPorts(w io.Writer, ports []serialenum.Port, jsonl bool) error {
	for _, port := range ports {
		if jsonl {
			b, err := json.Marshal(port)
			if err != nil {
				return fmt.Errorf("encoding serial port: %w", err)
			}
			if _, err := fmt.Fprintln(w, string(b)); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintln(w, port.Display); err != nil {
			return err
		}
	}
	return nil
}
