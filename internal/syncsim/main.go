//go:build ignore

package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/jclark/satpulse/internal/logobs"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/syncsim"
)

var (
	duration     = flag.Float64("duration", 60.0, "simulation duration in seconds")
	oscDrift     = flag.Float64("drift", 2000.0, "oscillator drift in ppb")
	oscNoise     = flag.Float64("noise", 20.0, "oscillator frequency noise stddev in ppb")
	ppsJitter    = flag.Float64("jitter", 10.0, "PPS timing jitter in nanoseconds")
	minDelay     = flag.Float64("min-delay", 5e-6, "minimum pulse delivery delay in seconds")
	maxDelay     = flag.Float64("max-delay", 250e-6, "maximum pulse delivery delay in seconds")
	msgDelay     = flag.Float64("msg-delay", 0.1, "GPS message delay after pulse in seconds")
	msgJitter    = flag.Float64("msg-jitter", 0.01, "GPS message delay jitter in seconds")
	statsInt     = flag.Int("stats", 10, "statistics interval in seconds (0 to disable)")
	clockLogPath = flag.String("clock-log", "/tmp/synctest-clock.log", "path to clock log file")
	logLevel     = flag.String("log-level", "INFO", "log level (DEBUG, INFO, WARN, ERROR)")
)

func main() {
	flag.Parse()

	// Configure logging
	level := slog.LevelInfo
	switch *logLevel {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}
	lg := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))

	// GPS time starts near present (2024-10-08 is roughly GPS time ~1.4e9 seconds)
	const gpsStartTime = 1.4e9

	// Leap second (current value as of 2017)
	ls := ptime.LeapSecond{
		UTCOffBefore:  37,
		UTCOffAfter:   37,
		OffChangeTime: 1483228800, // 2017-01-01
	}

	// Create samplers
	statsObs := logobs.NewStatsLogObserver(lg, *statsInt)
	clockObs, err := logobs.NewClockLogObserver(lg, *clockLogPath, ls)
	if err != nil {
		lg.Error("failed to create clock log observer", "err", err)
		os.Exit(1)
	}
	defer clockObs.Release()

	// Create multi-sampler
	sampler := &multiSampler{samplers: []phcsync.Sampler{statsObs, clockObs}}

	// Create simulation config
	simCfg := syncsim.Config{
		Duration:     *duration,
		OscDrift:     *oscDrift,
		OscNoise:     *oscNoise,
		PPSJitter:    *ppsJitter,
		MinDelay:     *minDelay,
		MaxDelay:     *maxDelay,
		MsgDelay:     *msgDelay,
		MsgJitter:    *msgJitter,
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
		"clockLog", *clockLogPath)
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
