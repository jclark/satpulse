package sdpcmd

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

func parseFlags(cmdName string, args []string) (jsonl bool, help bool, iface string, usageFunc func(string) string, err error) {
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.SetInterspersed(false)

	flags.BoolVarP(&jsonl, "jsonl", "j", false, "output in JSON lines format")
	flags.BoolVarP(&help, "help", "h", false, "show help")

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

	// Check for positional argument (interface name)
	if flags.NArg() > 0 {
		iface = flags.Arg(0)
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