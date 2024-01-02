package daemon

import (
	"fmt"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/spf13/pflag"
)

type flagVars struct {
	verbose      bool
	sdLog        bool
	wait         bool
	configFile   string
	serialDevice string
	inputLogFile string
}

const summary = `[-h|--help] [-V|--version] [-v|--verbose] [-w|--wait] [-s|--systemd-log]
       [-f|--config-file path] [-d|--serial-device device-path] [--input-log-file path]`

// The caller will display the string and exit if the flagVars are nil.
// If returned error is non-nil, then the flagVars should be nil and the string non-empty.
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
	flags.StringVarP(&vars.configFile, "config-file", "f", defaultConfigFile, "configuration file")
	flags.StringVarP(&vars.serialDevice, "serial-device", "d", "", "serial device to configure")
	flags.StringVar(&vars.inputLogFile, "input-log-file", "", "input log file")
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
		return nil, cmd.VersionInfo(), nil
	}
	if flags.NArg() != 0 {
		return nil, usage(), fmt.Errorf("%s command must not have non-option arguments", cmdName)
	}
	return &vars, "", nil
}
