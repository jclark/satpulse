//go:build ignore

package main

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/logobs"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/syncsim"
	"github.com/spf13/pflag"
)

var startTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

type flagVars struct {
	statsInterval   int
	clockLogPath    string
	simCfg          syncsim.Config
	phcCfg          phcsync.Config
	debug           bool
	toggleDurations []float64
}

func main() {
	vars, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Current simulation time - updated by Simulate as it runs
	curTime := startTime

	// Configure logging with simulated time
	level := slog.LevelInfo
	if vars.debug {
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

	// Run simulation
	stats, err := syncsim.Simulate(observers, vars.phcCfg, vars.simCfg, &curTime, lg)
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

func parseFlags(args []string) (*flagVars, error) {
	vars := flagVars{
		phcCfg: phcsync.DefaultConfig(),
		simCfg: syncsim.DefaultConfig(),
	}

	// Local variables for HW override flags (NaN = not set)
	var hwPath string
	phcFreqOffset := math.NaN()
	phcDrift := math.NaN()
	phcNoise := math.NaN()
	jitter := math.NaN()
	sawtooth := math.NaN()

	flags := pflag.NewFlagSet("syncsim", pflag.ContinueOnError)

	// HW config file flag
	flags.StringVarP(&hwPath, "hw", "h", "", "hardware config TOML file")

	// Non-HW flags (bind directly to struct)
	flags.Float64Var(&vars.simCfg.Duration, "duration", vars.simCfg.Duration, "simulation duration in seconds")

	// HW-related override flags (local variables with NaN default)
	flags.Float64Var(&phcFreqOffset, "phc-freq-offset", phcFreqOffset, "PHC frequency offset in ppb")
	flags.Float64Var(&phcDrift, "phc-drift", phcDrift, "PHC frequency drift in ppb/day")
	flags.Float64Var(&phcNoise, "phc-noise", phcNoise, "PHC frequency noise stddev in ppb")
	flags.Float64Var(&jitter, "jitter", jitter, "PPS timing jitter in nanoseconds")
	flags.Float64Var(&sawtooth, "sawtooth", sawtooth, "sawtooth amplitude in nanoseconds (0 to disable)")
	var sawtoothMsgType string
	flags.StringVar(&sawtoothMsgType, "sawtooth-msgtype", "", "sawtooth message type: prepulse, postpulse, or none")
	flags.Float64Var(&vars.simCfg.MinDelay, "min-delay", vars.simCfg.MinDelay, "minimum pulse delivery delay in seconds")
	flags.Float64Var(&vars.simCfg.MaxDelay, "max-delay", vars.simCfg.MaxDelay, "maximum pulse delivery delay in seconds")
	flags.Float64Var(&vars.simCfg.MsgDelay, "msg-delay", vars.simCfg.MsgDelay, "GPS message delay after pulse in seconds")
	flags.Float64Var(&vars.simCfg.MsgJitter, "msg-jitter", vars.simCfg.MsgJitter, "GPS message delay jitter in seconds")
	flags.Float64VarP(&vars.simCfg.PulseWidth, "pulse-width", "w", vars.simCfg.PulseWidth, "pulse width in seconds (0 for single-edge mode)")
	flags.IntVar(&vars.statsInterval, "stats", 0, "statistics interval in seconds (0 to disable)")
	flags.StringVar(&vars.clockLogPath, "clock-log", "", "path to clock log file (empty to disable)")
	flags.Float64Var(&vars.phcCfg.Track.Kp, "tracking-kp", vars.phcCfg.Track.Kp, "tracking mode proportional gain")
	flags.Float64Var(&vars.phcCfg.Track.Ki, "tracking-ki", vars.phcCfg.Track.Ki, "tracking mode integral gain")
	flags.Float64Var(&vars.phcCfg.Track.AvgFreqTimeConstant, "avg-freq-time-constant", vars.phcCfg.Track.AvgFreqTimeConstant, "tracking mode average frequency time constant in seconds")
	flags.IntVar(&vars.phcCfg.Track.BadSampleLimit, "tracking-bad-sample-limit", vars.phcCfg.Track.BadSampleLimit, "tracking mode bad sample limit before reset")
	flags.Float64SliceVar(&vars.simCfg.Outlier.Times, "outlier-times", vars.simCfg.Outlier.Times, "comma-separated list of seconds at which to inject PPS outliers")
	flags.DurationVar(&vars.simCfg.Outlier.Offset, "outlier-offset", vars.simCfg.Outlier.Offset, "magnitude of outlier phase offset")
	flags.BoolVar(&vars.debug, "debug", false, "enable debug logging")
	flags.Float64SliceVar(&vars.toggleDurations, "toggle", nil, "comma-separated relative durations to toggle pulse delivery (e.g., '10,5' = stop after 10s, restart after 5s more)")
	err := flags.Parse(args)
	if err != nil {
		return nil, err
	}
	if flags.NArg() != 0 {
		return nil, fmt.Errorf("command must not have non-option arguments")
	}

	// Load HW config if specified (replaces PHC and GPS configs)
	if hwPath != "" {
		hw, err := syncsim.LoadHWConfig(hwPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load hardware config: %v", err)
		}
		vars.simCfg.PHC = hw.PHC
		vars.simCfg.GPS = hw.GPS
	}

	// Apply explicit flag overrides (if not NaN)
	if !math.IsNaN(phcFreqOffset) {
		vars.simCfg.PHC.FreqOffset = phcFreqOffset
	}
	if !math.IsNaN(phcDrift) {
		vars.simCfg.PHC.Drift = phcDrift
	}
	if !math.IsNaN(phcNoise) {
		vars.simCfg.PHC.WhiteNoise = phcNoise
	}
	if !math.IsNaN(jitter) {
		vars.simCfg.GPS.Jitter = jitter
	}
	if !math.IsNaN(sawtooth) {
		vars.simCfg.GPS.Sawtooth.Amp = sawtooth
	}
	if sawtoothMsgType != "" {
		switch sawtoothMsgType {
		case "prepulse":
			vars.simCfg.SawtoothMsgType = gpsprot.PrePulse
		case "postpulse":
			vars.simCfg.SawtoothMsgType = gpsprot.PostPulse
		case "none":
			vars.simCfg.SawtoothMsgType = syncsim.NoPulse
		default:
			return nil, fmt.Errorf("invalid sawtooth-msgtype: %s (must be prepulse, postpulse, or none)", sawtoothMsgType)
		}
	}

	// Validate and convert relative toggle durations to absolute times
	if len(vars.toggleDurations) > 0 {
		vars.simCfg.ToggleTimes = make([]float64, len(vars.toggleDurations))
		t := 0.0
		for i, dur := range vars.toggleDurations {
			if dur <= 0 {
				return nil, fmt.Errorf("toggle duration at index %d must be > 0, got %v", i, dur)
			}
			t += dur
			vars.simCfg.ToggleTimes[i] = t
		}
	}
	return &vars, nil
}
