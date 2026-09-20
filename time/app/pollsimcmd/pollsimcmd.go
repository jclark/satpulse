// Package pollsimcmd implements the pollsim subcommand of satpulsetool,
// which simulates the serial PPS polling loop under a modelled host.
package pollsimcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/time/internal/pollsim"
	"github.com/spf13/pflag"
)

// Cmd executes the pollsim subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	help, showDefaultConfig := false, false
	var edgeLogPath string
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&showDefaultConfig, "show-default-config", "C", false, "print default config as TOML to stdout and exit")
	flags.StringVar(&edgeLogPath, "edge-log", "", "write every candidate edge with its true error to `path` in JSON Lines format")
	usageFunc := cmd.UsageFunc(cmdName, "[-h|--help] [-C|--show-default-config] [--edge-log PATH] <config.toml>", flags)
	if err = flags.Parse(args); err != nil || help {
		return usageFunc(progName), err
	}
	if showDefaultConfig {
		return "", pollsim.WriteDefaultConfig(os.Stdout)
	}
	if flags.NArg() != 1 {
		return usageFunc(progName), fmt.Errorf("expected config file argument")
	}
	cfg := pollsim.DefaultConfig()
	if err = pollsim.LoadConfig(flags.Arg(0), &cfg); err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}
	var record func(pollsim.EdgeRecord)
	if edgeLogPath != "" {
		f, err := os.Create(edgeLogPath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		record = func(e pollsim.EdgeRecord) {
			if err := enc.Encode(&e); err != nil {
				cmd.ErrPrintln(progName, "writing edge log: "+err.Error())
			}
		}
	}
	lg := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: logLevel}))
	stats, err := pollsim.Simulate(cfg, lg, record)
	if err != nil {
		return "", err
	}
	fmt.Print(stats.String())
	return "", nil
}
