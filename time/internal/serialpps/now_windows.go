package serialpps

import (
	"time"

	"golang.org/x/sys/windows"
)

// now reads the clocks behind Edge. time.Now is quantized to the shared
// clock page's update (~0.5 ms measured), far coarser than the brackets and
// pacing the polling loop measures, so wall comes from
// GetSystemTimePreciseAsFileTime, which resolves ~100 ns; that reading has
// no monotonic component, so mono stays a time.Now reading for elapsed-time
// arithmetic. The skew between the adjacent readings is bounded by the
// coarse quantum and enters only second labelling, whose tolerance is the
// millisecond-scale delayUncertainty. It is a variable so the synctest
// tests can substitute a reading of the bubble's clock, which the precise
// reading here is outside of.
var now = func() (wall, mono time.Time) {
	var ft windows.Filetime
	windows.GetSystemTimePreciseAsFileTime(&ft)
	return time.Unix(0, ft.Nanoseconds()), time.Now()
}
