package daemon

import (
	"fmt"
	"os"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/spf13/pflag"
)

type flagVars struct {
	verbose      bool
	sdLog        bool
	wait         bool
	configFiles  []string
	serialDevice string
}

const summary = `[-h|--help] [-V|--version] [-v|--verbose] [-w|--wait] [-s|--systemd-log]
       [-f|--config-file path] [-d|--serial-device device-path]`

// parseFlags parses the command line flags and returns the flagVars, a string to display, and an error.
// The caller will display the string and exit if the flagVars are nil.
// If returned error is non-nil, then the flagVars should be nil.
func parseFlags(cmdName string, args []string) (*flagVars, string, error) {
	help := false
	showVersion := false
	vars := flagVars{}

	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&vars.verbose, "verbose", "v", false, "log more information")
	flags.BoolVarP(&vars.sdLog, "systemd-log", "s", false, "log to stdout with priorities in systemd-compatible format")
	flags.BoolVarP(&vars.wait, "wait", "w", false, "wait for the network interface to be ready")
	flags.BoolVarP(&showVersion, "version", "V", false, "show version information")
	flags.StringSliceVarP(&vars.configFiles, "config-file", "f", nil, "configuration file")
	flags.StringVarP(&vars.serialDevice, "serial-device", "d", "", "serial device to configure")
	usage := func() string {
		return fmt.Sprintf("Usage: %s %s\nOptions:\n%s", cmdName, summary, flags.FlagUsages())
	}
	err := flags.Parse(args)
	if err != nil {
		return nil, usage(), err
	}
	if help {
		return nil, usage(), nil
	}
	if showVersion {
		return nil, cmd.VersionInfo() + "\n", nil
	}
	if flags.NArg() != 0 {
		return nil, usage(), fmt.Errorf("%s command must not have non-option arguments", cmdName)
	}
	if len(vars.configFiles) == 0 {
		f := os.Getenv(configFileEnvVar)
		if f == "" {
			return nil, "", fmt.Errorf("must specify a config file with -f option or %s environment variable", configFileEnvVar)
		}
		vars.configFiles = []string{f}
	}
	return &vars, "", nil
}
