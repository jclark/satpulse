package serialpps

import (
	"time"

	"golang.org/x/sys/windows"
)

// now reads the clocks behind Edge. time.Now is quantized to the shared
// clock page's update (~0.5 ms measured), far coarser than the brackets the
// polling loop measures, so wall comes from GetSystemTimePreciseAsFileTime,
// which resolves ~100 ns; that reading has no monotonic component, so mono
// stays a time.Now reading, pacing the loop and serving elapsed-time
// arithmetic against message read times. The two are adjacent reads rather
// than one atomic reading of both clocks: they differ by the coarse quantum
// plus however long a preemption between them lasts, which nothing here
// bounds. Only second labelling depends on the pairing, and mislabelling
// takes a skew of ~200 ms, the width of the rejection gap that the enforced
// delayUncertainty + maxDelay < 1s leaves between one second's window and
// the next. The precise reading is outside any synctest bubble, so the tests
// that need one do not run here; see runBubble.
func now() clockReading {
	var ft windows.Filetime
	windows.GetSystemTimePreciseAsFileTime(&ft)
	return clockReading{wall: time.Unix(0, ft.Nanoseconds()), mono: time.Now()}
}

// elapsedSince uses the precise wall readings on Windows. The mono readings
// come from time.Now and are too coarse for modem-state read times.
func (r clockReading) elapsedSince(start clockReading) time.Duration {
	return r.wall.Sub(start.wall)
}
