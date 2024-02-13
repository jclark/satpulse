package cmd

import (
	"fmt"

	"github.com/spf13/pflag"
)

func UsageFunc(cmdName, summary string, flags *pflag.FlagSet) func(string) string {
	return func(progName string) string {
		return fmt.Sprintf("Usage: %s %s %s\nOptions:\n%s", progName, cmdName, summary, flags.FlagUsages())
	}
}
