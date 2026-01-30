package syncsimcmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/logobs"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/syncsim"
	"github.com/spf13/pflag"
)

var startTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

type options struct {
	statsInterval int
	clockLogPath  string
	tsLogPath     string
	generate      bool
}

// Cmd executes the syncsim subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	opts, cfg, usageFunc, err := parseArgs(cmdName, args)
	if err != nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}
	if opts == nil {
		// help or show-default-config was handled
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}

	// Handle --generate mode
	if opts.generate {
		if err = syncsim.Generate(cfg, os.Stdout); err != nil {
			return
		}
		return
	}

	// Current simulation time - updated by Simulate as it runs
	curTime := startTime

	// Create logger with simulated time
	lg := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Time(slog.TimeKey, curTime)
			}
			return a
		},
	}))

	ls := ptime.LeapSecond2016()

	// Create observers
	var observers []obs.Observer
	statsObs := logobs.NewStatsLogObserver(lg, opts.statsInterval)
	observers = append(observers, statsObs)

	if opts.clockLogPath != "" {
		clockObs, err := logobs.NewClockLogObserver(lg, opts.clockLogPath, ls)
		if err != nil {
			return "", fmt.Errorf("failed to create clock log observer: %w", err)
		}
		defer clockObs.Release()
		observers = append(observers, clockObs)
	}

	var tsLog io.Writer
	if opts.tsLogPath != "" {
		f, err := os.Create(opts.tsLogPath)
		if err != nil {
			return "", fmt.Errorf("failed to create ts log file: %w", err)
		}
		defer f.Close()
		tsLog = f
	}

	// Run simulation
	stats, err := syncsim.Simulate(observers, cfg, tsLog, &curTime, lg)
	if err != nil {
		return "", err
	}

	// Final flush of stats
	statsObs.Release()

	// Output stats
	fmt.Print(stats.String())
	return
}

func parseArgs(cmdName string, args []string) (*options, syncsim.Config, func(string) string, error) {
	opts := &options{}
	help := false
	showDefaultConfig := false

	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&showDefaultConfig, "show-default-config", "C", false, "print default config as TOML to stdout and exit")
	flags.IntVar(&opts.statsInterval, "stats", 0, "log stats every N seconds (0 to disable)")
	flags.StringVar(&opts.clockLogPath, "clock-log", "", "write clock offsets to `path`")
	flags.StringVar(&opts.tsLogPath, "ts-log", "", "write PHC timestamps to `path` in JSONL format")
	flags.BoolVarP(&opts.generate, "generate", "g", false, "generate timestamps to stdout in JSONL format")

	summary := "[-h|--help] [-C|--show-default-config]\n" +
		"            [--stats N] [--clock-log PATH] [--ts-log PATH] [-g|--generate] <config.toml>"
	usageFunc := cmd.UsageFunc(cmdName, summary, flags)

	err := flags.Parse(args)
	if err != nil {
		return nil, syncsim.Config{}, usageFunc, err
	}
	if help {
		return nil, syncsim.Config{}, usageFunc, nil
	}
	if showDefaultConfig {
		if err := syncsim.WriteDefaultConfig(os.Stdout); err != nil {
			return nil, syncsim.Config{}, nil, err
		}
		return nil, syncsim.Config{}, nil, nil
	}
	if flags.NArg() != 1 {
		return nil, syncsim.Config{}, usageFunc, fmt.Errorf("expected config file argument")
	}

	configPath := flags.Arg(0)

	// Load config from TOML file
	cfg := syncsim.DefaultConfig()
	if err := syncsim.LoadConfig(configPath, &cfg); err != nil {
		if s, ok := err.(fmt.Stringer); ok {
			return nil, syncsim.Config{}, nil, fmt.Errorf("failed to load config: %v\n%s", err, s.String())
		}
		return nil, syncsim.Config{}, nil, fmt.Errorf("failed to load config: %v", err)
	}

	// Check mutual exclusion for --generate
	if opts.generate && (opts.statsInterval != 0 || opts.clockLogPath != "" || opts.tsLogPath != "") {
		return nil, syncsim.Config{}, nil, fmt.Errorf("--generate cannot be used with --stats, --clock-log, or --ts-log")
	}

	return opts, cfg, nil, nil
}
