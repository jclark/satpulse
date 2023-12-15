package combine

import (
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

type pulseEdge struct {
	ptime.ClockTime
	tRead time.Time
}

type edgeFilter interface {
	// delayed can only be non-nil once
	// and delayed must be nil after include returns true
	include(edge pulseEdge) (inc bool, delayed []pulseEdge)
}

type noEdgeFilter struct{}

func (f *noEdgeFilter) include(edge pulseEdge) (bool, []pulseEdge) {
	return true, nil
}

// knownEdgeFilter filters edges that are the trailing edge of the pulse
// This is for the case where we both know that we will be getting two edges per pulse
// and know the pulse width.
type knownEdgeFilter struct {
	pulseWidth          time.Duration
	pulseWidthTolerance time.Duration
	prev                pulseEdge
	prevIsFirst         bool
}

// include returns true if the pulse edge should be treated as the significant edge of a pulse.
// If we don't include the second edge and do include the third edge, then we return the first edge as delayed along with the third edge.
// If we do include the second edge, then there's no delayed edge.
func (f *knownEdgeFilter) include(edge pulseEdge) (bool, []pulseEdge) {
	prev := f.prev
	f.prev = edge
	if prev.isZero() {
		f.prevIsFirst = true
		return false, nil
	}
	prevIsFirst := f.prevIsFirst
	f.prevIsFirst = false
	off := edge.tRead.Sub(prev.tRead)
	if (f.pulseWidth - off).Abs() < f.pulseWidthTolerance {
		var delayed []pulseEdge
		if prevIsFirst {
			delayed = []pulseEdge{prev}
		}
		return false, delayed
	}
	return true, nil
}
