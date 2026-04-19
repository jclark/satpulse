package phcsample

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

// phcWindow holds recent pulse edges, labels them via the wallClock at
// query time, admits those that pass pre-admission filtering, fits the
// PHC-to-UTC regression, and combines with the cross-sample's sys
// reading to produce the refclock offset. In phase 1 step 4 this type
// carries only the public surface; TrueTimeOffset's body lands in step
// 6.
type phcWindow struct {
	cfg           *Config
	edgesPerPulse int
	edges         []PulseEdge
}

func newPhcWindow(cfg *Config, edgesPerPulse int) *phcWindow {
	return &phcWindow{
		cfg:           cfg,
		edgesPerPulse: edgesPerPulse,
	}
}

// Pulse records a pulse edge for later processing. Labelling,
// admission, and fitting happen lazily inside TrueTimeOffset.
func (w *phcWindow) Pulse(edge PulseEdge) {
	w.edges = append(w.edges, edge)
}

// TrueTimeOffset returns the refclock offset in seconds:
//
//	offset = true_time_at(phc) - sys
//
// A positive value means true time is ahead of sys; the system clock
// is behind real time and needs to advance by this amount. The sign
// convention matches what chrony expects from a SOCK refclock.
//
// Phase 1 step 4 stubs this to ErrNotReady; step 6 fills in the body.
func (w *phcWindow) TrueTimeOffset(phc ptime.Time, sys time.Time, wc *wallClock, po pulseCorrector, lg *slog.Logger) (float64, error) {
	return 0, ErrNotReady
}

// Reset clears recorded edges.
func (w *phcWindow) Reset() {
	w.edges = nil
}
