package serialpps

import (
	"time"

	"golang.org/x/sys/windows"
)

// now reads the clocks behind Edge. time.Now is quantized to the shared
// clock page's update (~0.5 ms measured), far coarser than the brackets and
// the pacing the polling loop measures, so wall comes from
// GetSystemTimePreciseAsFileTime, which resolves ~100 ns; that reading has
// no monotonic component, so mono stays a time.Now reading for elapsed-time
// arithmetic against message read times. The skew between the adjacent
// readings is bounded by the coarse quantum and enters only second
// labelling, whose tolerance is the millisecond-scale delayUncertainty. It
// is a variable so the synctest tests can substitute a reading of the
// bubble's clock, which the precise reading here is outside of.
var now = func() clockReading {
	var ft windows.Filetime
	windows.GetSystemTimePreciseAsFileTime(&ft)
	return clockReading{wall: time.Unix(0, ft.Nanoseconds()), mono: time.Now()}
}

// elapsedSince uses the precise wall readings on Windows. The mono readings
// come from time.Now and are too coarse for modem-state read times.
func (r clockReading) elapsedSince(start clockReading) time.Duration {
	return r.wall.Sub(start.wall)
}
