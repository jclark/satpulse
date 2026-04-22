package phcsample

import (
	"errors"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/phctime"
)

// pulseEdge carries the PHC-side data the Generator needs from a
// single edge event. The dispatcher adapts ts.Event into this, dropping
// Kind / ResumeFunc / TReadWall; Kind filtering (Pause, Resume) and
// stale-era filtering are dispatcher responsibilities.
type pulseEdge struct {
	Timestamp phctime.Time   // PHC timestamp of the pulse edge
	TRead     phctime.Sample // monotonic-bearing PHC/system read sample
}

// IsZero reports whether edge is the zero value. phcWindow uses this
// sentinel to mark entries that pre-admission filtering has rejected,
// preserving positional alignment across dual-edge polarity streams.
func (edge pulseEdge) IsZero() bool {
	return edge == pulseEdge{}
}

// ErrNotReady indicates the Generator does not yet have enough labelled
// edges to answer. This is the expected error during startup and after
// a gap in edges or messages.
var ErrNotReady = errors.New("phcsample: not enough labelled edges")

// pulseCorrector is the interface implemented by timemsg.Buffer's
// phase-2 UTC-keyed pulse-correction accessor. Callers that have no
// correction source leave g.pc nil; the arithmetic path then behaves
// as if every correction were zero (the phase-1 fallback).
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
	pc            pulseCorrector // nil if no sawtooth correction source is installed
	edgesPerPulse int
	lg            *slog.Logger
}

// NewGenerator constructs a Generator. edgesPerPulse is 1 for single-edge
// or 2 for dual-edge mode; it is discovered by the dispatcher the same
// way as for phcsync. The sawtooth-correction source is installed
// separately via SetPulseCorrector.
func NewGenerator(cfg Config, edgesPerPulse int, lg *slog.Logger) *Generator {
	return &Generator{
		cfg:           cfg,
		wc:            *newWallClock(&cfg),
		win:           *newPhcWindow(&cfg, edgesPerPulse),
		edgesPerPulse: edgesPerPulse,
		lg:            lg,
	}
}

// SetPulseCorrector installs the source of UTC-keyed pulse-offset
// corrections used to shift pulse-edge labels to the true top-of-second
// during PHC calibration. Typically wired by the dispatcher to the
// timemsg.Buffer. Pass nil to disable sawtooth correction.
func (g *Generator) SetPulseCorrector(pc pulseCorrector) {
	g.pc = pc
}

// NewInstance returns a fresh Generator with the same configuration,
// edgesPerPulse, pulse-corrector, and logger as g, but with empty
// wallClock and phcWindow state. Used by the dispatcher on PHC era
// transitions, where nothing in the prior instance is worth preserving:
// the wallClock regression re-warms from subsequent MsgUTCTime calls
// and pre-pause phcWindow entries would reference a stepped PHC.
func (g *Generator) NewInstance() *Generator {
	n := NewGenerator(g.cfg, g.edgesPerPulse, g.lg)
	n.pc = g.pc
	return n
}

// MsgUTCTime implements the MsgUTCTimer sink: every eligible UTC
// observation from the message stream feeds the wallClock regression.
func (g *Generator) MsgUTCTime(utc time.Time, tRead time.Time, _ ptime.LeapSecondKind) {
	g.wc.Add(tRead, utc)
}

// Pulse records a pulse-edge event. Per plan, edges are buffered cheaply
// here; labelling and admission happen lazily inside Generate. Implements
// gpsevent.PulseReceiver so the dispatcher can deliver edges without
// knowing which runtime mode is active.
func (g *Generator) Pulse(ts phctime.Time, tr phctime.Sample) {
	g.win.Pulse(pulseEdge{Timestamp: ts, TRead: tr})
}

// Generate returns the offset (true time - sys) in seconds at the PHC
// cross-sample. Returns ErrNotReady while warming up.
func (g *Generator) Generate(phc ptime.Time, sys time.Time) (float64, error) {
	pc := g.pc
	if g.cfg.IgnoreSawtoothCorrection {
		pc = nil
	}
	return g.win.TrueTimeOffset(phc, sys, &g.wc, pc, g.lg)
}
