package pps

// GPIOConfig controls how a GPIO pin is polled for PPS edges.
type GPIOConfig struct {
	// CPU, if set, is the CPU the poller's thread is pinned to. It should
	// not be a CPU that handles interrupts, above all the GPIO's own, whose
	// handling interrupts the polls that bracket the edge.
	CPU *int `toml:"cpu" check:">=0" comment:"CPU to pin the GPIO poller to; omit to leave it unpinned"`
	// Priority, if nonzero, is the SCHED_FIFO priority the poller's thread
	// runs at, so that its short polling windows preempt ordinary work.
	Priority int `toml:"priority" check:">=0,<100" comment:"SCHED_FIFO priority of the GPIO poller, 1 to 99; 0 leaves normal scheduling"`
	// MaxBracket is the widest polling bracket, in seconds, from which an
	// edge is used as a sample. Zero applies no limit.
	MaxBracket float64 `toml:"maxBracket" check:">=0,<1" comment:"Widest interval between the polls bracketing an edge for it to be used (s); 0 uses every edge"`
}

// DefaultGPIOConfig returns the default GPIO polling configuration: an
// unpinned poller at normal priority, and brackets up to 5 us, a few times
// the register read time on the supported boards.
func DefaultGPIOConfig() GPIOConfig {
	return GPIOConfig{MaxBracket: 5e-6}
}
