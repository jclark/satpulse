package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
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
	debug         bool
}

func main() {
	opts, cfg, duration, err := parseArgs(os.Args[1:])
	if err != nil {
		cmd.ErrPrintln("syncsim", err)
		os.Exit(1)
	}

	// Current simulation time - updated by Simulate as it runs
	curTime := startTime

	// Configure logging with simulated time
	level := slog.LevelInfo
	if opts.debug {
		level = slog.LevelDebug
	}
	lg := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
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
			lg.Error("failed to create clock log observer", "err", err)
			os.Exit(1)
		}
		defer clockObs.Release()
		observers = append(observers, clockObs)
	}

	var tsLog io.Writer
	if opts.tsLogPath != "" {
		f, err := os.Create(opts.tsLogPath)
		if err != nil {
			lg.Error("failed to create ts log file", "err", err)
			os.Exit(1)
		}
		defer f.Close()
		tsLog = f
	}

	// Run simulation
	stats, err := syncsim.Simulate(observers, cfg, duration, tsLog, &curTime, lg)
	if err != nil {
		lg.Error("simulation failed", "err", err)
		os.Exit(1)
	}

	// Final flush of stats
	statsObs.Release()

	// Log final stats
	args := stats.Stats.LogArgs()
	args = append(args, "samples", stats.SampleCount)
	args = append(args, "trackingStdDev", stats.TrackingStdDev)
	args = append(args, "trackingAbsMax", stats.TrackingAbsMax)
	args = append(args, "trackingMean", stats.TrackingMean)
	lg.Info("simulation complete", args...)
}

func parseArgs(args []string) (*options, syncsim.Config, time.Duration, error) {
	opts := &options{}
	help := false
	showVersion := false

	flags := pflag.NewFlagSet("syncsim", pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&showVersion, "version", "V", false, "show version information")
	flags.BoolVar(&opts.debug, "debug", false, "enable debug logging")
	flags.IntVar(&opts.statsInterval, "stats", 0, "log stats every N seconds (0 to disable)")
	flags.StringVar(&opts.clockLogPath, "clock-log", "", "write clock offsets to PATH")
	flags.StringVar(&opts.tsLogPath, "ts-log", "", "write PHC timestamps to PATH (JSON Lines)")

	err := flags.Parse(args)
	if err != nil {
		return nil, syncsim.Config{}, 0, err
	}
	if help {
		fmt.Fprintf(os.Stderr, "Usage: syncsim [options] <config.toml> <duration>\n\nOptions:\n%s", flags.FlagUsages())
		os.Exit(0)
	}
	if showVersion {
		fmt.Println(cmd.VersionInfo())
		os.Exit(0)
	}
	if flags.NArg() != 2 {
		return nil, syncsim.Config{}, 0, fmt.Errorf("usage: syncsim [options] <config.toml> <duration>")
	}

	configPath := flags.Arg(0)
	durSec, err := strconv.ParseFloat(flags.Arg(1), 64)
	if err != nil {
		return nil, syncsim.Config{}, 0, fmt.Errorf("invalid duration: %v", err)
	}
	if durSec <= 0 {
		return nil, syncsim.Config{}, 0, fmt.Errorf("duration must be positive")
	}
	duration := time.Duration(durSec * float64(time.Second))

	// Load config from TOML file
	cfg := syncsim.DefaultConfig()
	if err := syncsim.LoadConfig(configPath, &cfg); err != nil {
		return nil, syncsim.Config{}, 0, fmt.Errorf("failed to load config: %v", err)
	}

	return opts, cfg, duration, nil
}
