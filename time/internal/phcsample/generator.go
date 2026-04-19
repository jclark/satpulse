package phcsample

import (
	"errors"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/phctime"
)

// PulseEdge carries the PHC-side data the Generator needs from a
// single edge event. The dispatcher adapts ts.Event into this, dropping
// Kind / ResumeFunc / TReadWall; Kind filtering (Pause, Resume) and
// stale-era filtering are dispatcher responsibilities.
type PulseEdge struct {
	Timestamp phctime.Time   // PHC timestamp of the pulse edge
	TRead     phctime.Sample // monotonic-bearing PHC/system read sample
}

// IsZero reports whether edge is the zero value. phcWindow uses this
// sentinel to mark entries that pre-admission filtering has rejected,
// preserving positional alignment across dual-edge polarity streams.
func (edge PulseEdge) IsZero() bool {
	return edge == PulseEdge{}
}

// ErrNotReady indicates the Generator does not yet have enough labelled
// edges to answer. This is the expected error during startup and after
// a gap in edges or messages.
var ErrNotReady = errors.New("phcsample: not enough labelled edges")

// pulseCorrector is the interface implemented by timemsg.Buffer's
// phase-2 UTC-keyed pulse-correction accessor. Phase 1 passes nil.
type pulseCorrector interface {
	GetUTCPulseCorrection(refTime time.Time) (float64, bool)
}

// Generator constructs (true time - sys) samples for chrony's SOCK
// refclock in PHC free-running mode. All three entry points are thin
// pass-throughs to internal collaborators.
type Generator struct {
	cfg           Config
	wc            wallClock
	win           phcWindow
	edgesPerPulse int
	lg            *slog.Logger
}

// NewGenerator constructs a Generator. edgesPerPulse is 1 for single-edge
// or 2 for dual-edge mode; it is discovered by the dispatcher the same
// way as for phcsync.
func NewGenerator(cfg Config, edgesPerPulse int, lg *slog.Logger) *Generator {
	return &Generator{
		cfg:           cfg,
		wc:            *newWallClock(&cfg),
		win:           *newPhcWindow(&cfg, edgesPerPulse),
		edgesPerPulse: edgesPerPulse,
		lg:            lg,
	}
}

// NewInstance returns a fresh Generator with the same configuration,
// edgesPerPulse, and logger as g, but with empty wallClock and
// phcWindow state. Used by the dispatcher on PHC era transitions,
// where nothing in the prior instance is worth preserving: the
// wallClock regression re-warms from subsequent MsgUTCTime calls and
// pre-pause phcWindow entries would reference a stepped PHC.
func (g *Generator) NewInstance() *Generator {
	return NewGenerator(g.cfg, g.edgesPerPulse, g.lg)
}

// MsgUTCTime implements the MsgUTCTimer sink: every eligible UTC
// observation from the message stream feeds the wallClock regression.
func (g *Generator) MsgUTCTime(utc time.Time, tRead time.Time, _ ptime.LeapSecondKind) {
	g.wc.Add(tRead, utc)
}

// Pulse records a pulse-edge event. Per plan, edges are buffered cheaply
// here; labelling and admission happen lazily inside Generate.
func (g *Generator) Pulse(edge PulseEdge) {
	g.win.Pulse(edge)
}

// Generate returns the offset (true time - sys) in seconds at the PHC
// cross-sample. Returns ErrNotReady while warming up.
func (g *Generator) Generate(phc ptime.Time, sys time.Time) (float64, error) {
	return g.win.TrueTimeOffset(phc, sys, &g.wc, nil, g.lg)
}
