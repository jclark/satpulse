package pollsim

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/jclark/satpulse/gps/lib/check"
	"github.com/pelletier/go-toml/v2"
)

// Seconds is a true alias for float64, used for documentation in config
// structs; the simulator converts to time.Duration internally.
type Seconds = float64

// Config holds the simulation parameters.
type Config struct {
	Sim   SimConfig   `toml:"sim" comment:"Simulation parameters"`
	Pulse PulseConfig `toml:"pulse" comment:"The pulse as seen on the pin"`
	Host  HostConfig  `toml:"host" comment:"Timing of the host's state queries, timers and clock"`
	Poll  PollConfig  `toml:"poll" comment:"Poll loop and consumer parameters"`
	Fault FaultConfig `toml:"fault" comment:"Faults injected during the simulation"`
}

// SimConfig configures the run as a whole.
type SimConfig struct {
	// Duration is the simulated time covered.
	Duration Seconds `toml:"duration" check:">0" comment:"Simulated duration (s)"`
	// Seed seeds every random model, so a run is repeatable.
	Seed int64 `toml:"seed" comment:"Random seed"`
}

// PulseConfig configures the pulse. Its leading edges are at integral
// seconds of simulated time plus a Gaussian jitter.
type PulseConfig struct {
	// Width is how long the pin stays in the pulse after a leading edge.
	Width Seconds `toml:"width" check:">0,<1" comment:"Pulse width (s)"`
	// Jitter is the standard deviation of the edge's timing error, as
	// delivered to the pin: receiver error plus any latency between the
	// receiver and the pin that varies from pulse to pulse.
	Jitter Seconds `toml:"jitter" check:">=0,<0.1" comment:"Edge timing jitter stddev (s)"`
}

// HostConfig configures the host: how long a state query takes, how timer
// sleeps behave, and how fast the clock reads.
type HostConfig struct {
	Query QueryConfig `toml:"query" comment:"Modem-state query timing"`
	Timer TimerConfig `toml:"timer" comment:"Timer sleep behaviour"`
	// ClockRead is how long reading the clock takes. It paces the PreWarm
	// busy-wait, which reads the clock continuously.
	ClockRead Seconds `toml:"clockRead" check:">0,<0.001" comment:"Clock read time (s)"`
}

// QueryConfig configures the state query. Its duration is Duration plus a
// Gaussian error of Jitter, never below a quarter of Duration; while the
// host has been idle it is multiplied by the idle factor.
type QueryConfig struct {
	// Duration is the typical query time: ~10 us for a UART, ~200 us for a
	// USB adapter on macOS, ~1-2 ms for one on Linux.
	Duration Seconds `toml:"duration" check:">0,<0.1" comment:"Typical query time (s)"`
	// Jitter is the standard deviation of the query time.
	Jitter Seconds `toml:"jitter" check:">=0,<0.1" comment:"Query time stddev (s)"`
	// Idle models hosts whose queries slow down while the machine idles,
	// the effect PreWarm counters.
	Idle IdleConfig `toml:"idle" comment:"Slowdown after idling"`
}

// IdleConfig models the idle slowdown: after After without activity
// (queries or busy-waiting) queries take Factor times longer, until Recover
// of continuous activity has passed.
type IdleConfig struct {
	After   Seconds `toml:"after" check:">=0,<1" comment:"Idle time before queries slow down (s); 0 disables"`
	Factor  float64 `toml:"factor" check:">=1,<=100" comment:"Query time multiplier while slowed"`
	Recover Seconds `toml:"recover" check:">=0,<1" comment:"Continuous activity that restores full speed (s)"`
}

// TimerConfig configures timer sleeps. A requested sleep is truncated to a
// multiple of Resolution, as the Go runtime does on Linux at one
// millisecond, and a truncated-to-zero sleep returns at once; a sleep that
// does happen overshoots by a Gaussian amount, never negative.
type TimerConfig struct {
	Resolution      Seconds `toml:"resolution" check:">=0,<0.1" comment:"Sleep truncation quantum (s); 0 means sleeps are exact to the nanosecond"`
	Overshoot       Seconds `toml:"overshoot" check:">=0,<0.1" comment:"Mean wakeup overshoot (s)"`
	OvershootJitter Seconds `toml:"overshootJitter" check:">=0,<0.1" comment:"Wakeup overshoot stddev (s)"`
}

// PollConfig configures the poll loop and its consumer.
type PollConfig struct {
	// PreWarm is the pollPreWarm setting.
	PreWarm Seconds `toml:"preWarm" check:">=0,<1" comment:"Busy-wait before each poll window (s); 0 disables"`
	// MinSpacing is the loop's minimum spacing between queries; 0 means the
	// loop's default.
	MinSpacing Seconds `toml:"minSpacing" check:">=0,<0.1" comment:"Minimum spacing between queries (s); 0 means the default"`
	// MaxUncertainty is the consumer's limit: a catch that is not rejected
	// and no more uncertain than this is forwarded to the time daemon.
	MaxUncertainty Seconds `toml:"maxUncertainty" check:">0,<1" comment:"Consumer's uncertainty limit for forwarding an edge (s)"`
}

// FaultConfig configures the faults.
type FaultConfig struct {
	Outage []OutageConfig `toml:"outage" comment:"Periods with no pulse"`
	Stall  []StallConfig  `toml:"stall" comment:"Single stalls of the polling thread"`
	Stalls []StallBurst   `toml:"stalls" comment:"Random stalls of the polling thread"`
}

// OutageConfig is a period during which the pulse is absent.
type OutageConfig struct {
	Start    Seconds `toml:"start" check:">=0" comment:"When the outage begins (s)"`
	Duration Seconds `toml:"duration" check:">=0" comment:"How long it lasts (s)"`
}

// StallConfig is one stall: the polling thread does not run for Duration
// from At, wherever it is at the time: inside a query, in the busy-wait, or
// asleep, in which case only the part after the scheduled wakeup counts.
type StallConfig struct {
	At       Seconds `toml:"at" check:">=0" comment:"When the stall begins (s)"`
	Duration Seconds `toml:"duration" check:">=0,<10" comment:"How long the thread does not run (s)"`
}

// StallBurst is a Poisson process of stalls at Rate per second during
// [Start, Start+Duration), each lasting a log-uniform time between Min and
// Max. It models the host load of the 2026-09-20 incident: several stalls
// of milliseconds to tens of milliseconds over a few seconds.
type StallBurst struct {
	Start    Seconds `toml:"start" check:">=0" comment:"When the burst begins (s)"`
	Duration Seconds `toml:"duration" check:">=0" comment:"How long it lasts (s); 0 means the whole run"`
	Rate     float64 `toml:"rate" check:">=0" comment:"Stalls per second"`
	Min      Seconds `toml:"min" check:">0,<10" comment:"Shortest stall (s)"`
	Max      Seconds `toml:"max" check:">0,<10" comment:"Longest stall (s)"`
}

// DefaultConfig returns a configuration for a USB serial adapter on macOS:
// 200 us queries, exact timers, no faults.
func DefaultConfig() Config {
	return Config{
		Sim:   SimConfig{Duration: 600, Seed: 1},
		Pulse: PulseConfig{Width: 0.1, Jitter: 20e-6},
		Host: HostConfig{
			Query:     QueryConfig{Duration: 200e-6, Jitter: 30e-6, Idle: IdleConfig{Factor: 1}},
			ClockRead: 50e-9,
		},
		Poll:  PollConfig{MaxUncertainty: 1e-3},
		Fault: FaultConfig{Outage: []OutageConfig{{}}, Stall: []StallConfig{{}}, Stalls: []StallBurst{{Min: 1e-3, Max: 1e-3}}},
	}
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	var errs []error
	for _, msg := range check.Validate(c) {
		errs = append(errs, errors.New(msg))
	}
	for i, b := range c.Fault.Stalls {
		if b.Min > b.Max {
			errs = append(errs, fmt.Errorf("fault.stalls[%d]: min %g exceeds max %g", i, b.Min, b.Max))
		}
	}
	if idle := c.Host.Query.Idle; idle.After > 0 && idle.Recover == 0 {
		errs = append(errs, errors.New("host.query.idle: recover must be positive when after is"))
	}
	return errors.Join(errs...)
}

// LoadConfig reads a TOML configuration file into cfg, which should hold the
// defaults, and validates the result.
func LoadConfig(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := toml.NewDecoder(f).DisallowUnknownFields().Decode(cfg); err != nil {
		return err
	}
	return cfg.Validate()
}

// WriteDefaultConfig writes the default configuration as TOML to w, with
// comments from the struct tags.
func WriteDefaultConfig(w io.Writer) error {
	cfg := DefaultConfig()
	return toml.NewEncoder(w).Encode(cfg)
}
