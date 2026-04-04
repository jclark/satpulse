package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/internal/annotatecmd"
	"github.com/jclark/satpulse/internal/decodecmd"
	"github.com/jclark/satpulse/internal/gpscmd"
	"github.com/jclark/satpulse/internal/pmccmd"
	"github.com/jclark/satpulse/internal/replaycmd"
	"github.com/jclark/satpulse/internal/sdpcmd"
	"github.com/jclark/satpulse/time/app/syncsimcmd"
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
	var exec func(logWriter io.Writer, logLevel slog.Level, progName string, cmdName string, cmdArgs []string) (usage string, err error)
	switch cmdName {
	case "annotate":
		exec = annotatecmd.Cmd
	case "decode":
		exec = decodecmd.Cmd
	case "gps":
		exec = gpscmd.Cmd
	case "pmc":
		exec = pmccmd.Cmd
	case "replay":
		exec = replaycmd.Cmd
	case "sdp":
		exec = sdpcmd.Cmd
	case "syncsim":
		exec = syncsimcmd.Cmd
	}
	if exec != nil {
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

func usage(progName string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, "[global-options] command [options] arg...")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  gps - configure a GPS device")
	fmt.Fprintln(os.Stderr, "  sdp - control software-defined pins on PTP hardware clocks")
	fmt.Fprintln(os.Stderr, "  syncsim - run clock synchronization simulation")
	fmt.Fprintln(os.Stderr, "  decode - decode a GPS packet")
	fmt.Fprintln(os.Stderr, "  annotate - annotate a JSONL packet log with decoded fields")
	fmt.Fprintln(os.Stderr, "  replay - replay a JSONL packet log through the processing pipeline")
	fmt.Fprintln(os.Stderr, "  pmc - send a PTP management message to ptp4l process")
	fmt.Fprintln(os.Stderr, "Global options:")
	flags.PrintDefaults()
}
