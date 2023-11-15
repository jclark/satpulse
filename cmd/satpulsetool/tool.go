package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/spf13/pflag"
)

func main() {
	var verboseLevel int
	var help bool

	flags := pflag.NewFlagSet("satpulsetool", pflag.ContinueOnError)
	flags.SetInterspersed(false)

	flags.CountVarP(&verboseLevel, "verbose", "v", "increase verbosity")
	flags.BoolVarP(&help, "help", "h", false, "show help")
	err := flags.Parse(os.Args[1:])
	progName := os.Args[0]
	if err != nil {
		errPrintln(progName, err)
		os.Exit(2)
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

	level := slog.LevelWarn
	if verboseLevel == 1 {
		level = slog.LevelInfo
	} else if verboseLevel > 1 {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	lg := slog.New(handler)
	slog.SetDefault(lg)

	switch cmdName {
	case "config":
		err = configCmd(lg, progName, cmdName, cmdArgs)
	default:
		errPrintln(progName, "unknown command: "+cmdName)
		usage(progName, flags)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func usage(progName string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, "[global-options] command [options] arg...")
	fmt.Fprintln(os.Stderr, "Commands: config")
	fmt.Fprintln(os.Stderr, "Global options:")
	flags.PrintDefaults()
}

func errPrintln(progName string, arg any) {
	fmt.Fprintln(os.Stderr, progName+":", arg)
}

type intFlag struct {
	value *int
}

var _ pflag.Value = (*intFlag)(nil)

func (i *intFlag) String() string {
	if i.value == nil {
		return ""
	}
	return strconv.Itoa(*i.value)
}

func (i *intFlag) Type() string {
	return "int"
}

func (i *intFlag) Set(s string) error {
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	i.value = &v
	return nil
}
