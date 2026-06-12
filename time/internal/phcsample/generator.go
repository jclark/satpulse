package phcsample

import (
	"errors"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/lib/ntime"
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
// TAI-keyed pulse-correction accessor. Callers that have no
// correction source leave g.pc nil; the arithmetic path then behaves
// as if every correction were zero.
type pulseCorrector interface {
	GetTAIPulseCorrection(refTime ptime.Time) (float64, bool)
}

// Sampler receives the PHC-referenced refclock samples the Generator
// produces, one per admitted pulse edge. It is a pipe between the
// Generator and the refclock, not an interpreter of the timestamp's
// domain; the dispatcher implements it by forwarding to the refclock.
type Sampler interface {
	PHCSample(phc ntime.Time, offset float64, leap ptime.LeapSecondKind)
}

// Generator constructs PHC-referenced samples for chrony's SOCK
// refclock in free-running mode (phc.sync = false). The PHC is read
// but never stepped or slewed. Each admitted pulse edge becomes its own
// sample: the PHC timestamp of the edge paired with
// (offset = TAI at the edge - PHC reading), emitted to the Sampler as
// soon as the edge can be labelled from the TAI message stream.
type Generator struct {
	cfg           Config
	wc            wallClock
	win           phcWindow
	pc            pulseCorrector // nil if no sawtooth correction source is installed
	sampler       Sampler        // nil until SetSampler
	leap          ptime.LeapSecondKind
	edgesPerPulse int
	lg            *slog.Logger
}

// NewGenerator constructs a Generator. edgesPerPulse is 1 for single-edge
// or 2 for dual-edge mode; it is discovered by the dispatcher the same
// way as for phcsync. The sawtooth-correction source and the sample
// sink are installed separately via SetPulseCorrector and SetSampler.
func NewGenerator(cfg Config, edgesPerPulse int, lg *slog.Logger) *Generator {
	return &Generator{
		cfg:           cfg,
		wc:            *newWallClock(&cfg),
		win:           *newPhcWindow(&cfg, edgesPerPulse),
		edgesPerPulse: edgesPerPulse,
		lg:            lg,
	}
}

// SetPulseCorrector installs the source of TAI-keyed pulse-offset
// corrections used to shift pulse-edge labels to the true top-of-second.
// Typically wired by the dispatcher to the timemsg.Buffer. Pass nil to
// disable sawtooth correction.
func (g *Generator) SetPulseCorrector(pc pulseCorrector) {
	g.pc = pc
}

// SetSampler installs the sink that receives the PHC-referenced samples.
func (g *Generator) SetSampler(s Sampler) {
	g.sampler = s
}

// NewInstance returns a fresh Generator with the same configuration,
// edgesPerPulse, pulse-corrector, sampler, and logger as g, but with
// empty wallClock and phcWindow state. Used by the dispatcher on PHC
// era transitions, where nothing in the prior instance is worth
// preserving: the wallClock regression re-warms from subsequent
// MsgTAITime calls and pre-pause phcWindow entries would reference a
// stepped PHC.
func (g *Generator) NewInstance() *Generator {
	n := NewGenerator(g.cfg, g.edgesPerPulse, g.lg)
	n.pc = g.pc
	n.sampler = g.sampler
	return n
}

// MsgTAITime implements the MsgTAITimer sink: every eligible TAI
// observation from the message stream feeds the wallClock regression
// and may make buffered edges labellable.
func (g *Generator) MsgTAITime(tai ptime.Time, tRead time.Time, leap ptime.LeapSecondKind) {
	g.leap = leap
	g.wc.Add(tRead, ntime.Time(tai))
	g.emit()
}

// Pulse records a pulse-edge event. Edges are buffered cheaply here;
// labelling and admission happen inside emit.
func (g *Generator) Pulse(ts phctime.Time, tr phctime.Sample) {
	g.win.Pulse(pulseEdge{Timestamp: ts, TRead: tr})
	g.emit()
}

// emit sends any newly labellable edges to the Sampler.
func (g *Generator) emit() {
	if g.sampler == nil {
		return
	}
	pc := g.pc
	if g.cfg.IgnoreSawtoothCorrection {
		pc = nil
	}
	samples, err := g.win.Samples(&g.wc, pc)
	if err != nil {
		if !errors.Is(err, ErrNotReady) {
			g.lg.Warn("could not derive offset from PHC", "err", err)
		}
		return
	}
	for _, s := range samples {
		g.sampler.PHCSample(ntime.Time(s.phc), s.offset, g.leap)
	}
}
