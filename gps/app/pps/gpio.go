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
	// OutlierRatio marks a tracking catch an outlier when its bracket
	// exceeds this multiple of the lower quartile of the recent settled
	// brackets, so that an edge whose reads were interrupted by a stall is
	// withheld from timing. Zero disables the check.
	OutlierRatio float64 `toml:"outlierRatio" check:">=0" comment:"Bracket multiple of the recent lower quartile beyond which a polled edge is an outlier; 0 disables"`
}

// DefaultGPIOConfig returns the default GPIO polling configuration: an
// unpinned poller at normal priority, and the outlier ratio serial PPS
// polling uses.
func DefaultGPIOConfig() GPIOConfig {
	return GPIOConfig{OutlierRatio: 3}
}
