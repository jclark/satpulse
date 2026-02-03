package phctime

import (
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

type Time struct {
	T   ptime.Time
	Era Era
}

// Sample represents a paired reading of a PHC clock and system clock.
// The Clock field represents the PHC time, and Sys represents the
// corresponding system time (either monotonic or wallclock).
type Sample struct {
	PHC Time
	Sys time.Time
}

// An Era represents a period of time within which a clock has not been stepped.
// When the time of a clock is read, an era is associated with it.
// When a clock is stepped, it may be uncertain whether a particular read of the
// clock happened before or after the step. We handle this by incrementing the
// twice during the step: once before the step is started, and once after we know
// it has taken effect. Even-numbered eras are used to represent the uncertain period
// while the clock is being stepped.
type Era uint64

// Uncertain returns true if the era is uncertain.
// Uncertain eras are even numbered.
func (e Era) Uncertain() bool {
	return (e & 1) == 0
}

func (e Era) StepCount() (count uint64, changing bool) {
	count = uint64(e) >> 1
	changing = e.Uncertain()
	if !changing {
		count++
	}
	return
}
