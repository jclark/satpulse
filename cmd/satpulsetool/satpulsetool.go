package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/spf13/pflag"
)

// exitCoder interface for errors that specify their own exit code
type exitCoder interface {
	error
	ExitCode() int
}

func main() {
	var verboseLevel int
	var help bool
	var showVersion bool

	flags := pflag.NewFlagSet("satpulsetool", pflag.ContinueOnError)
	flags.SetInterspersed(false)

	flags.CountVarP(&verboseLevel, "verbose", "v", "increase verbosity")
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&showVersion, "version", "V", false, "show version information")

	err := flags.Parse(os.Args[1:])
	progName := os.Args[0]
	if err != nil {
		cmd.ErrPrintln(progName, err)
		usage(progName, flags)
		os.Exit(2)
	}
	if showVersion {
		fmt.Fprintln(os.Stderr, cmd.VersionInfo())
		os.Exit(0)
	}
	if help {
		usage(progName, flags)
		os.Exit(0)
	}
	if flags.NArg() == 0 {
		usage(progName, flags)
		os.Exit(2)
	}
	cmdName := flags.Arg(0)
	cmdArgs := flags.Args()[1:]

	logLevel := slog.LevelWarn
	if verboseLevel == 1 {
		logLevel = slog.LevelInfo
	} else if verboseLevel > 1 {
		logLevel = slog.LevelDebug
	}
	logWriter := os.Stderr

	exitCode := 0
	exec, ok := commands[cmdName]
	if ok {
		usage, err := exec(logWriter, logLevel, progName, cmdName, cmdArgs)
		if err != nil {
			cmd.ErrPrintlnWithDetail(progName, err)
			// Check if error specifies its own exit code
			var ec exitCoder
			if errors.As(err, &ec) {
				exitCode = ec.ExitCode()
			} else if usage != "" {
				exitCode = 2
			} else {
				exitCode = 1
			}
		}
		fmt.Fprint(os.Stderr, usage)
	} else {
		cmd.ErrPrintln(progName, "unknown command: "+cmdName)
		usage(progName, flags)
		exitCode = 2
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

// descriptions lists every possible subcommand and its one-line description,
// in the order they should appear in usage. usage() only prints entries whose
// name is present in commands, so the help text reflects what this build
// actually supports.
var descriptions = []struct {
	name, desc string
}{
	{"gps", "configure a GPS device"},
	{"sdp", "control software-defined pins on PTP hardware clocks"},
	{"syncsim", "run clock synchronization simulation"},
	{"convobs", "convert raw and JSON observations"},
	{"decode", "decode a GPS packet"},
	{"annotate", "annotate a JSONL packet log with decoded fields"},
	{"pack", "convert a JSONL packet log to a packet byte stream"},
	{"replay", "replay a JSONL packet log through the processing pipeline"},
	{"ntrip", "Ntrip client"},
	{"pmc", "send a PTP management message to ptp4l process"},
}

func usage(progName string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, "[global-options] command [options] arg...")
	fmt.Fprintln(os.Stderr, "Commands:")
	for _, d := range descriptions {
		if _, ok := commands[d.name]; ok {
			fmt.Fprintf(os.Stderr, "  %s - %s\n", d.name, d.desc)
		}
	}
	fmt.Fprintln(os.Stderr, "Global options:")
	flags.PrintDefaults()
}
