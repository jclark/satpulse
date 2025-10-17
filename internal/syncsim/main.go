//go:build ignore

package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/internal/logobs"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/syncsim"
	"github.com/spf13/pflag"
)

type flagVars struct {
	duration     float64
	oscDrift     float64
	oscNoise     float64
	ppsJitter    float64
	minDelay     float64
	maxDelay     float64
	msgDelay     float64
	msgJitter    float64
	statsInt     int
	clockLogPath string
	debug        bool
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

	// Create samplers
	statsObs := logobs.NewStatsLogObserver(lg, vars.statsInt)
	clockObs, err := logobs.NewClockLogObserver(lg, vars.clockLogPath, ls)
	if err != nil {
		lg.Error("failed to create clock log observer", "err", err)
		os.Exit(1)
	}
	defer clockObs.Release()

	// Create multi-sampler
	sampler := &multiSampler{samplers: []phcsync.Sampler{statsObs, clockObs}}

	// Create simulation config
	simCfg := syncsim.Config{
		Duration:     vars.duration,
		OscDrift:     vars.oscDrift,
		OscNoise:     vars.oscNoise,
		PPSJitter:    vars.ppsJitter,
		MinDelay:     vars.minDelay,
		MaxDelay:     vars.maxDelay,
		MsgDelay:     vars.msgDelay,
		MsgJitter:    vars.msgJitter,
		GPSStartTime: gpsStartTime,
	}

	// Use default phcsync config
	phcCfg := phcsync.DefaultConfig()

	// Run simulation
	stats, err := syncsim.Simulate(sampler, phcCfg, simCfg, lg)
	if err != nil {
		lg.Error("simulation failed", "err", err)
		os.Exit(1)
	}

	// Final flush of stats
	statsObs.Release()

	lg.Info("simulation complete",
		"samples", stats.SampleCount,
		"finalFreq", stats.FinalFreq,
		"trackingSamples", stats.TrackingSamples,
		"trackingStdDev", stats.TrackingStdDev,
		"clockLog", vars.clockLogPath)
}

func parseFlags(args []string) (*flagVars, error) {
	vars := flagVars{}
	flags := pflag.NewFlagSet("syncsim", pflag.ContinueOnError)
	flags.Float64Var(&vars.duration, "duration", 60.0, "simulation duration in seconds")
	flags.Float64Var(&vars.oscDrift, "drift", 2000.0, "oscillator drift in ppb")
	flags.Float64Var(&vars.oscNoise, "noise", 20.0, "oscillator frequency noise stddev in ppb")
	flags.Float64Var(&vars.ppsJitter, "jitter", 10.0, "PPS timing jitter in nanoseconds")
	flags.Float64Var(&vars.minDelay, "min-delay", 5e-6, "minimum pulse delivery delay in seconds")
	flags.Float64Var(&vars.maxDelay, "max-delay", 250e-6, "maximum pulse delivery delay in seconds")
	flags.Float64Var(&vars.msgDelay, "msg-delay", 0.1, "GPS message delay after pulse in seconds")
	flags.Float64Var(&vars.msgJitter, "msg-jitter", 0.01, "GPS message delay jitter in seconds")
	flags.IntVar(&vars.statsInt, "stats", 10, "statistics interval in seconds (0 to disable)")
	flags.StringVar(&vars.clockLogPath, "clock-log", "/tmp/syncsim-clock.log", "path to clock log file")
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

// multiSampler combines multiple samplers
type multiSampler struct {
	samplers []phcsync.Sampler
}

func (m *multiSampler) Sample(data phcsync.SampleData) {
	for _, s := range m.samplers {
		s.Sample(data)
	}
}
