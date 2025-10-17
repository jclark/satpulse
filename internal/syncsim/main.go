//go:build ignore

package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/internal/logobs"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/syncsim"
	"github.com/spf13/pflag"
)

type flagVars struct {
	statsInterval int
	clockLogPath  string
	simCfg        syncsim.Config
	phcCfg        phcsync.Config
	debug         bool
}

func main() {
	vars, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Configure logging
	level := slog.LevelInfo
	if vars.debug {
		level = slog.LevelDebug
	}
	lg := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))

	// GPS time starts near present (2024-10-08 is roughly GPS time ~1.4e9 seconds)
	const gpsStartTime = 1.4e9

	ls := ptime.LeapSecond2016()

	// Create observers
	var observers []obs.Observer
	statsObs := logobs.NewStatsLogObserver(lg, vars.statsInterval)
	observers = append(observers, statsObs)

	if vars.clockLogPath != "" {
		clockObs, err := logobs.NewClockLogObserver(lg, vars.clockLogPath, ls)
		if err != nil {
			lg.Error("failed to create clock log observer", "err", err)
			os.Exit(1)
		}
		defer clockObs.Release()
		observers = append(observers, clockObs)
	}

	// Set GPS start time
	vars.simCfg.GPSStartTime = gpsStartTime

	// Run simulation
	stats, err := syncsim.Simulate(observers, vars.phcCfg, vars.simCfg, lg)
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
	lg.Info("simulation complete", args...)
}

func parseFlags(args []string) (*flagVars, error) {
	vars := flagVars{
		phcCfg: phcsync.DefaultConfig(),
	}
	flags := pflag.NewFlagSet("syncsim", pflag.ContinueOnError)
	flags.Float64Var(&vars.simCfg.Duration, "duration", 60.0, "simulation duration in seconds")
	flags.Float64Var(&vars.simCfg.OscDrift, "drift", 2000.0, "oscillator drift in ppb")
	flags.Float64Var(&vars.simCfg.OscNoise, "noise", 20.0, "oscillator frequency noise stddev in ppb")
	flags.Float64Var(&vars.simCfg.PPSJitter, "jitter", 10.0, "PPS timing jitter in nanoseconds")
	flags.Float64Var(&vars.simCfg.MinDelay, "min-delay", 5e-6, "minimum pulse delivery delay in seconds")
	flags.Float64Var(&vars.simCfg.MaxDelay, "max-delay", 250e-6, "maximum pulse delivery delay in seconds")
	flags.Float64Var(&vars.simCfg.MsgDelay, "msg-delay", 0.1, "GPS message delay after pulse in seconds")
	flags.Float64Var(&vars.simCfg.MsgJitter, "msg-jitter", 0.01, "GPS message delay jitter in seconds")
	flags.IntVar(&vars.statsInterval, "stats", 0, "statistics interval in seconds (0 to disable)")
	flags.StringVar(&vars.clockLogPath, "clock-log", "", "path to clock log file (empty to disable)")
	flags.Float64Var(&vars.phcCfg.Tracking.KP, "tracking-kp", 0.7, "tracking mode proportional gain")
	flags.Float64Var(&vars.phcCfg.Tracking.KI, "tracking-ki", 0.3, "tracking mode integral gain")
	flags.BoolVar(&vars.debug, "debug", false, "enable debug logging")
	err := flags.Parse(args)
	if err != nil {
		return nil, err
	}
	if flags.NArg() != 0 {
		return nil, fmt.Errorf("command must not have non-option arguments")
	}
	return &vars, nil
}
