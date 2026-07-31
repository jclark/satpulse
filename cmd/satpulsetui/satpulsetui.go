// Command satpulsetui is a terminal UI for interactive GPS receiver
// configuration and monitoring: a Bubble Tea shell over
// gps/app/session covering the same ground as SatPulse Workbench
// (cmd/satpulsewb) where a terminal does it well, usable over ssh on
// a headless machine with no browser involved.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/msgfile"
	"github.com/spf13/pflag"
)

const summary = `[-h|--help] [-v|--verbose]
       [-d|--serial-device path] [-s|--device-speed bps] [--vendor name]
       [--packet-log path]`

type flagVars struct {
	device      string
	speed       int
	vendor      gpsreg.Vendor
	packetLog   string
	logLevel    slog.Level
	showVersion bool
}

func main() {
	progName := os.Args[0]
	v, usage, err := parseFlags(os.Args[1:])
	if err != nil {
		cmd.ErrPrintln(progName, err)
		fmt.Fprint(os.Stderr, usage(progName))
		os.Exit(2)
	}
	if v == nil {
		fmt.Fprint(os.Stderr, usage(progName))
		return
	}
	if v.showVersion {
		fmt.Fprintln(os.Stderr, cmd.VersionInfo())
		return
	}
	if err := run(v); err != nil {
		cmd.ErrPrintlnWithDetail(progName, err)
		os.Exit(1)
	}
}

// parseFlags parses and validates the command line. It returns a nil
// flagVars and a nil error when help was requested.
func parseFlags(args []string) (*flagVars, func(string) string, error) {
	var v flagVars
	var vendorStr string
	var verbose int
	var help bool
	flags := pflag.NewFlagSet("satpulsetui", pflag.ContinueOnError)
	flags.StringVarP(&v.device, "serial-device", "d", "", "serial device connected to GPS receiver")
	flags.IntVarP(&v.speed, "device-speed", "s", 0, "serial device baud-rate in `bps`")
	flags.StringVar(&vendorStr, "vendor", "", "GPS receiver `vendor` name")
	flags.StringVar(&v.packetLog, "packet-log", "", "log packets to `path`")
	flags.BoolFuncP("verbose", "v", "increase logging verbosity", func(string) error {
		verbose++
		return nil
	})
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&v.showVersion, "version", "V", false, "show version information")
	usage := func(progName string) string {
		return fmt.Sprintf("Usage: %s %s\nOptions:\n%s", progName, summary, flags.FlagUsages())
	}
	if err := flags.Parse(args); err != nil {
		return nil, usage, err
	}
	if help {
		return nil, usage, nil
	}
	if flags.NArg() != 0 {
		return nil, usage, fmt.Errorf("satpulsetui must not have non-option arguments")
	}
	if flags.Lookup("device-speed").Changed && v.speed <= 0 {
		return nil, usage, fmt.Errorf("--device-speed must be greater than zero")
	}
	var err error
	if v.vendor, err = gpsreg.ParseVendor(vendorStr); err != nil {
		return nil, usage, err
	}
	// Default Info so connect and configure progress reaches the
	// status line.
	v.logLevel = slog.LevelInfo
	if verbose > 0 {
		v.logLevel = slog.LevelDebug
	}
	return &v, usage, nil
}

// run runs the terminal UI until the user quits. The session is used
// in process; slog is routed through session.NewLogHandler with a
// discard base handler so nothing writes to the terminal behind
// Bubble Tea's back (log records reach the UI as gps:log events).
func run(v *flagVars) error {
	vendors, err := cmd.ResolveVendors(v.vendor)
	if err != nil {
		return err
	}
	var packetLog io.Writer
	if v.packetLog != "" {
		f, err := os.Create(v.packetLog)
		if err != nil {
			return err
		}
		defer f.Close()
		packetLog = f
	}
	snk := newSink()
	base := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: v.logLevel})
	lg := slog.New(session.NewLogHandler(snk, base))
	sess := session.New(lg, snk, session.Options{PacketLog: packetLog})
	m := newModel(sess, lg, snk, vendors, msgDirs(), v.device, v.speed)
	p := tea.NewProgram(m)
	go snk.run(p)
	_, err = p.Run()
	snk.stop()
	sess.Disconnect()
	return err
}

// msgDirs returns the message-file library search path: the
// SATPULSE_GPSMSG_PATH directories, when set, followed by the
// built-in library. Same as satpulsewb's.
func msgDirs() []msgfile.Dir {
	return append(msgfile.EnvDirs(), msgfile.Builtin())
}
