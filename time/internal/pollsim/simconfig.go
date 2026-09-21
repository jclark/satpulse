package pollsim

// Seconds is a true alias for float64, used for documentation in config
// structs; the simulator converts to time.Duration internally.
type Seconds = float64

// Config holds the simulation parameters.
type Config struct {
	Sim   SimConfig
	Pulse PulseConfig
	Host  HostConfig
	Poll  PollConfig
	Fault FaultConfig
}

// SimConfig configures the run as a whole.
type SimConfig struct {
	// Duration is the simulated time covered.
	Duration Seconds
	// Seed seeds every random model, so a run is repeatable.
	Seed int64
}

// PulseConfig configures the pulse. Its leading edges are at integral
// seconds of simulated time plus a Gaussian jitter.
type PulseConfig struct {
	// Width is how long the pin stays in the pulse after a leading edge.
	Width Seconds
	// Jitter is the standard deviation of the edge's timing error, as
	// delivered to the pin: receiver error plus any latency between the
	// receiver and the pin that varies from pulse to pulse.
	Jitter Seconds
}

// HostConfig configures the host: how long a state query takes, how timer
// sleeps behave, and how fast the clock reads.
type HostConfig struct {
	Query QueryConfig
	Timer TimerConfig
	// ClockRead is how long reading the clock takes. It paces the PreWarm
	// busy-wait, which reads the clock continuously.
	ClockRead Seconds
}

// QueryConfig configures the state query. Its duration is Duration plus a
// Gaussian error of Jitter, never below a quarter of Duration; while the
// host has been idle it is multiplied by the idle factor.
type QueryConfig struct {
	// Duration is the typical query time: ~10 us for a UART, ~200 us for a
	// USB adapter on macOS, ~1-2 ms for one on Linux.
	Duration Seconds
	// Jitter is the standard deviation of the query time.
	Jitter Seconds
	// Idle models hosts whose queries slow down while the machine idles,
	// the effect PreWarm counters.
	Idle IdleConfig
}

// IdleConfig models the idle slowdown: after After without activity
// (queries or busy-waiting) queries take Factor times longer, until Recover
// of continuous activity has passed.
type IdleConfig struct {
	After   Seconds // 0 disables idle slowdown
	Factor  float64
	Recover Seconds
}

// TimerConfig configures timer sleeps. A sleep the loop asks to be precise
// ends at its deadline; any other is truncated to a multiple of Resolution,
// as the Go runtime does on Linux at one millisecond, and a truncated-to-zero
// sleep returns at once. A sleep that does happen overshoots by a Gaussian
// amount, never negative.
type TimerConfig struct {
	Resolution      Seconds // sleep truncation quantum; 0 means exact to the nanosecond
	Overshoot       Seconds // mean wakeup overshoot
	OvershootJitter Seconds // wakeup overshoot standard deviation
}

// PollConfig configures the poll loop and its consumer.
type PollConfig struct {
	// PreWarm is the pollPreWarm setting.
	PreWarm Seconds
	// MinSpacing is the loop's minimum spacing between queries; 0 means the
	// loop's default.
	MinSpacing Seconds
}

// FaultConfig configures the faults.
type FaultConfig struct {
	Outage []OutageConfig
	Stall  []StallConfig
	Stalls []StallBurst
	Slow   []SlowConfig
	Slows  []SlowBurst
}

// OutageConfig is a period during which the pulse is absent.
type OutageConfig struct {
	Start    Seconds
	Duration Seconds
}

// StallConfig is one stall: the polling thread does not run for Duration
// from At, wherever it is at the time: inside a query, in the busy-wait, or
// asleep, in which case only the part after the scheduled wakeup counts.
type StallConfig struct {
	At       Seconds
	Duration Seconds
}

// StallBurst is a Poisson process of stalls at Rate per second during
// [Start, Start+Duration), each lasting a log-uniform time between Min and
// Max. It models the host load of the 2026-09-20 incident: several stalls
// of milliseconds to tens of milliseconds over a few seconds.
type StallBurst struct {
	Start    Seconds
	Duration Seconds // 0 means the rest of the run
	Rate     float64
	Min      Seconds
	Max      Seconds
}

// SlowConfig is a period during which every query takes Factor times
// longer: host load that slows the thread rather than stopping it, so
// the two queries around an edge are slowed alike.
type SlowConfig struct {
	Start    Seconds
	Duration Seconds
	Factor   float64
}

// SlowBurst is a Poisson process of slow periods at Rate per second during
// [Start, Start+Duration), each lasting a log-uniform time between Min and
// Max with queries taking Factor times longer.
type SlowBurst struct {
	Start    Seconds
	Duration Seconds // 0 means the rest of the run
	Rate     float64
	Min      Seconds
	Max      Seconds
	Factor   float64
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
	}
}
