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

// ErrNotReady indicates the Generator does not yet have enough labelled
// edges to answer. This is the expected error during startup and after
// a gap in edges or messages.
var ErrNotReady = errors.New("phcsample: not enough labelled edges")

// Sampler receives samples emitted by Generator.Generate. Only
// successful calls are reported; ErrNotReady and other errors produce
// no sample.
type Sampler interface {
	NTPSample(sys time.Time, offset float64, leap ptime.LeapSecondKind, phc ptime.Time)
}

// pulseCorrector is the interface implemented by timemsg.Buffer's
// phase-2 UTC-keyed pulse-correction accessor. Phase 1 passes nil.
type pulseCorrector interface {
	GetUTCPulseCorrection(refTime time.Time) (float64, bool)
}

// Generator constructs (true time - sys) samples for chrony's SOCK
// refclock in PHC free-running mode. All three entry points are thin
// pass-throughs to internal collaborators.
type Generator struct {
	cfg  Config
	wc   wallClock
	win  phcWindow
	smp  Sampler
	leap ptime.LeapSecondKind
	lg   *slog.Logger
}

// NewGenerator constructs a Generator. edgesPerPulse is 1 for single-edge
// or 2 for dual-edge mode; it is discovered by the dispatcher the same
// way as for phcsync.
func NewGenerator(cfg Config, smp Sampler, edgesPerPulse int, lg *slog.Logger) *Generator {
	return &Generator{
		cfg: cfg,
		wc:  *newWallClock(&cfg),
		win: *newPhcWindow(&cfg, edgesPerPulse),
		smp: smp,
		lg:  lg,
	}
}

// MsgUTCTime implements the MsgUTCTimer sink. The most recent leap
// value is retained and forwarded on the next NTPSample call.
func (g *Generator) MsgUTCTime(utc time.Time, tRead time.Time, leap ptime.LeapSecondKind) {
	g.wc.Add(tRead, utc)
	g.leap = leap
}

// Pulse records a pulse-edge event. Per plan, edges are buffered cheaply
// here; labelling and admission happen lazily inside Generate.
func (g *Generator) Pulse(edge PulseEdge) {
	g.win.Pulse(edge)
}

// Generate returns the offset (true time - sys) in seconds at the PHC
// cross-sample. On success the sample is also emitted via the Sampler.
// Returns ErrNotReady while warming up.
func (g *Generator) Generate(phc ptime.Time, sys time.Time) (float64, error) {
	off, err := g.win.TrueTimeOffset(phc, sys, &g.wc, nil, g.lg)
	if err != nil {
		return 0, err
	}
	g.smp.NTPSample(sys, off, g.leap, phc)
	return off, nil
}
