package tool

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"
)

func Usage(progName, cmdName, summary string, flags *pflag.FlagSet) {
	fmt.Fprintln(os.Stderr, "Usage:", progName, cmdName, summary)
	fmt.Fprintln(os.Stderr, "Options:")
	flags.PrintDefaults()
}
