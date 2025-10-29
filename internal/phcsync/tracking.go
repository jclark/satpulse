package phcsync

import (
	"log/slog"
	"math"
	"time"
)

// TrackingConfig contains tunable parameters for tracking mode.
type TrackingConfig struct {
	// Kp is the proportional gain for the PI servo used during tracking mode.
	// Lower than converging mode for stability during normal operation.
	// Typical value: 0.5.
	Kp float64 `toml:"kp" check:">0.0,<10.0"`

	// Ki is the integral gain for the PI servo used during tracking mode.
	// Lower than converging mode to prevent overcorrection during stable tracking.
	// Typical value: 0.1.
	Ki float64 `toml:"ki" check:">0.0,<10.0"`

	// OutlierThreshold is the minimum absolute offset in nanoseconds for classifying a sample
	// as an outlier. Samples with |offset| >= OutlierThreshold are marked as outliers and
	// excluded from frequency adjustment. This prevents transient disturbances from affecting
	// the servo. Typical value: 1000 (1µs).
	OutlierThreshold int64 `toml:"outlierThreshold" check:">0,<1_000_000_000"`

	// PulseWidthTolerance is the tolerance in nanoseconds for filtering trailing edges in
	// dual-edge mode based on temporal spacing. An edge is considered a trailing edge (and
	// ignored) if it arrives within PulseWidthTolerance of the expected trailing edge time
	// (lastEdgeTime + PulseWidth). This helps distinguish leading from trailing edges when
	// alignment alone is ambiguous. Typical value: 200 ns.
	PulseWidthTolerance int64 `toml:"pulseWidthTolerance" check:">0,<1_000_000_000"`

	// AlignTolerance is the tolerance in nanoseconds for offset from top of second to
	// immediately accept an edge as a leading edge. Edges with |offset| <= AlignTolerance
	// (where offset is timestamp rounded to nearest second) are assumed to be leading edges
	// and accepted without further checks. This is the primary discriminator for
	// well-synchronized clocks. Typical value: 100 ns.
	AlignTolerance int64 `toml:"alignTolerance" check:">0,<1_000_000_000"`

	// BadSampleLimit is the maximum number of consecutive bad samples (missing or outlier)
	// before transitioning back to reset mode. Bad samples do not contribute to frequency
	// adjustment. Typical value: 5.
	BadSampleLimit int `toml:"badSampleLimit" check:">=1,<100"`

	// AvgFreqTimeConstant is the time constant in seconds for the exponential moving
	// average of frequency adjustments. This average represents the baseline frequency
	// correction (without phase correction) and is used when samples are missing.
	// Larger values track baseline frequency more smoothly but respond more slowly to
	// oscillator drift. The EMA is updated as: avgFreq = alpha*freq + (1-alpha)*avgFreq
	// where alpha = 1 - exp(-sampleInterval/timeConstant). Set to 0 to disable this
	// feature (no frequency adjustment on missing samples). Typical value: 120.
	AvgFreqTimeConstant float64 `toml:"avgFreqTimeConstant" check:">=0.0,<1000.0"`
}

func defaultTrackingConfig() TrackingConfig {
	return TrackingConfig{
		Kp:                  0.5,
		Ki:                  0.1,
		OutlierThreshold:    1000, // 1µs
		PulseWidthTolerance: 200,  // 200ns
		AlignTolerance:      100,  // 100ns
		BadSampleLimit:      5,
		AvgFreqTimeConstant: 30, // 30 seconds
	}
}

type trackingSampleGenerator struct {
	cfg        TrackingConfig
	pt         PulseType
	freq       float64
	maxFreq    float64
	lg         *slog.Logger
	lastSample *Sample // last accepted sample, used for edge filtering
}

func newTrackingSampleGenerator(cfg TrackingConfig, pt PulseType, lastSample *Sample, freq, maxFreq float64, lg *slog.Logger) *trackingSampleGenerator {
	if pt.EdgesPerPulse == 2 && pt.PulseWidth <= 0 {
		panic("tracking mode: PulseWidth must be > 0 when EdgesPerPulse == 2")
	}
	return &trackingSampleGenerator{
		cfg:        cfg,
		pt:         pt,
		freq:       freq,
		maxFreq:    maxFreq,
		lg:         lg,
		lastSample: lastSample,
	}
}

// ignoreEdge determines whether to ignore an edge based on dual-edge filtering.
// Returns true if the edge should be ignored (filtered out).
func (g *trackingSampleGenerator) ignoreEdge(edge PulseEdge, edgeIndex uint64) bool {
	// If not dual-edge mode, accept all edges
	if g.pt.EdgesPerPulse != 2 {
		return false
	}

	// Round to nearest second to get offset from top of second
	refTime := edge.Timestamp.T.Round(time.Second)
	offset := edge.Timestamp.T.Sub(refTime)
	offsetAbs := offset.Abs()

	// if alignKeep is true, it indicates we should probably keep this edge
	alignKeep := offsetAbs <= time.Duration(g.cfg.AlignTolerance)

	// Check spacing relative to pulse width
	// Note: PulseWidth is guaranteed to be > 0 when EdgesPerPulse == 2 (validated in constructor)
	// lastSample is always non-nil (tracking mode is entered from converging mode)

	lastEdgeTime := g.lastSample.Ref.Add(g.lastSample.Offset)
	// if widthIgnore is true, it indicates we should probably ignore this edge
	widthIgnore := (edge.Timestamp.T.Sub(lastEdgeTime) - g.pt.PulseWidth).Abs() <= time.Duration(g.cfg.PulseWidthTolerance)

	// parityIgnore is true suggests ignoring this edge, and !parityIgnore suggests keeping it
	parityIgnore := (g.lastSample.edgeIndex^edgeIndex)&1 != 0

	if alignKeep && !parityIgnore {
		return false
	}

	if widthIgnore && parityIgnore {
		return true
	}

	// this also has the effect of resetting parity
	if alignKeep {
		return false
	}

	if widthIgnore {
		return true
	}

	return parityIgnore
}

func (g *trackingSampleGenerator) pulseEdgeSample(edge PulseEdge, edgeIndex uint64) *Sample {
	// Filter dual edges
	if g.ignoreEdge(edge, edgeIndex) {
		return nil
	}

	// Round to nearest second
	refTime := edge.Timestamp.T.Round(time.Second)
	offset := edge.Timestamp.T.Sub(refTime)

	// Estimate system time: TRead minus the time between reading and timestamp capture
	phcDelta := edge.TReadPHC.T.Sub(edge.Timestamp.T)
	sys := edge.TRead.Add(-phcDelta)

	sample := &Sample{
		SampleData: &SampleData{
			Kind:      SampleOK,
			Ref:       refTime,
			Offset:    offset,
			Era:       edge.Timestamp.Era,
			SyncState: InSync,
		},
		edgeIndex: edgeIndex,
		Sys:       sys,
	}

	// Update lastSample for next edge comparison
	g.lastSample = sample

	return sample
}

func (g *trackingSampleGenerator) timeMessageSample() *Sample {
	// Tracking mode only uses pulse edges for now
	return nil
}

type trackingSampleProcessor struct {
	servo                 *piServo
	cfg                   TrackingConfig
	consecutiveBadSamples int
	avgFreq               float64 // exponential moving average of frequency
	emaAlpha              float64 // EMA coefficient (1 - exp(-1/timeConstant))
	lg                    *slog.Logger
}

func newTrackingSampleProcessor(cfg TrackingConfig, currentFreq, maxFreq float64, lg *slog.Logger) *trackingSampleProcessor {
	// Calculate EMA alpha from time constant
	// With 1 second sample interval: alpha = 1 - exp(-1/timeConstant)
	// If timeConstant is 0, EMA feature is disabled
	var emaAlpha float64
	if cfg.AvgFreqTimeConstant > 0 {
		emaAlpha = 1.0 - math.Exp(-1.0/cfg.AvgFreqTimeConstant)
	}
	return &trackingSampleProcessor{
		servo:    newPiServo(currentFreq, cfg.Kp, cfg.Ki, maxFreq),
		cfg:      cfg,
		avgFreq:  currentFreq, // initialize with current frequency
		emaAlpha: emaAlpha,
		lg:       lg,
	}
}

func (p *trackingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	action := p.sampleAction(sample)

	// Track consecutive bad samples (missing or outlier - i.e., not sent to servo)
	if action.actionType == phcNoAction {
		p.consecutiveBadSamples++
		// Use EMA feature (adjusting on bad samples), if not disabled by AvgFreqTimeConstant = 0
		if p.consecutiveBadSamples == 1 && p.cfg.AvgFreqTimeConstant > 0 {
			action.actionType = phcAdjustFrequency
			action.freq = p.avgFreq
			p.lg.Debug("first bad sample; switching to average frequency", "avgFreq", p.avgFreq)
		}
	} else {
		p.consecutiveBadSamples = 0
	}
	// Transition to reset mode if too many consecutive bad samples
	if p.consecutiveBadSamples >= p.cfg.BadSampleLimit {
		p.lg.Info("entering reset mode", "consecutiveBadSamples", p.consecutiveBadSamples)
		return action, ModeReset
	}
	return action, ModeTracking
}

func (p *trackingSampleProcessor) sampleAction(sample *Sample) phcAction {
	if sample.Kind == SampleMissing {
		return phcAction{actionType: phcNoAction}
	}
	// Check for outlier
	outlierThreshold := time.Duration(p.cfg.OutlierThreshold)
	if sample.Offset.Abs() >= outlierThreshold {
		// Mark as outlier and don't adjust
		sample.Kind = SampleOutlier
		p.lg.Debug("outlier detected", "offset", sample.Offset, "threshold", outlierThreshold)
		return phcAction{actionType: phcNoAction}
	}

	// Apply PI control for good samples
	freq := p.servo.sample(sample.Offset)

	// Update exponential moving average of frequency
	// avgFreq = alpha * freq + (1-alpha) * avgFreq
	p.avgFreq = p.emaAlpha*freq + (1.0-p.emaAlpha)*p.avgFreq

	return phcAction{
		actionType: phcAdjustFrequency,
		freq:       freq,
	}
}
