package phcsync

import (
	"log/slog"
	"time"
)

// TrackingConfig contains tunable parameters for tracking mode.
type TrackingConfig struct {
	KP float64 // proportional gain
	KI float64 // integral gain

	// OutlierThreshold is the minimum absolute offset in nanoseconds for a sample to be considered an outlier.
	// Samples with offsets exceeding this threshold are marked as outliers and not used for frequency adjustment.
	OutlierThreshold int64

	// PulseWidthTolerance is the tolerance in nanoseconds for edge spacing when filtering by pulse width in dual-edge mode.
	// When an edge arrives approximately PulseWidth after the last accepted edge (within this tolerance),
	// it is considered a trailing edge and filtered out. This helps distinguish leading from trailing edges
	// based on temporal spacing. Example: 200 = 200ns tolerance.
	PulseWidthTolerance int64

	// TopOfSecondTolerance is the tolerance in nanoseconds for offset from top of second to immediately accept an edge.
	// Edges with offsets (rounded to nearest second) within this tolerance of zero are assumed to be leading edges
	// and accepted without further checks. This is the primary discriminator for well-synchronized clocks.
	// Example: 100 = 100ns tolerance means offsets between -100ns and +100ns are accepted.
	TopOfSecondTolerance int64

	// MaxConsecutiveBadSamples is the maximum number of consecutive bad samples (missing or outlier)
	// before transitioning from tracking mode to lost mode.
	MaxConsecutiveBadSamples int
}

func defaultTrackingConfig() TrackingConfig {
	return TrackingConfig{
		KP:                       0.7,
		KI:                       0.3,
		OutlierThreshold:         1000, // 1µs
		PulseWidthTolerance:      200,  // 200ns
		TopOfSecondTolerance:     100,  // 100ns
		MaxConsecutiveBadSamples: 5,
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
	alignKeep := offsetAbs <= time.Duration(g.cfg.TopOfSecondTolerance) 

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
	servo                  *piServo
	cfg                    TrackingConfig
	consecutiveBadSamples  int
	lg                     *slog.Logger
}

func newTrackingSampleProcessor(cfg TrackingConfig, currentFreq, maxFreq float64, lg *slog.Logger) *trackingSampleProcessor {
	return &trackingSampleProcessor{
		servo: newPiServo(currentFreq, cfg.KP, cfg.KI, maxFreq),
		cfg:   cfg,
		lg:    lg,
	}
}

func (p *trackingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	action := p.sampleAction(sample)

	// Track consecutive bad samples (missing or outlier - i.e., not sent to servo)
	if action.actionType == phcNoAction {
		p.consecutiveBadSamples++
	} else {
		p.consecutiveBadSamples = 0
	}

	// Transition to reset mode if too many consecutive bad samples
	if p.consecutiveBadSamples >= p.cfg.MaxConsecutiveBadSamples {
		p.lg.Info("entering reset mode", "consecutiveBadSamples", p.consecutiveBadSamples)
		return action, ModeReset
	}

	return action, ModeTracking
}

func (p *trackingSampleProcessor) sampleAction(sample *Sample) phcAction {
	if sample.Kind == SampleMissing {
		// Just continue tracking on missing samples
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
	return phcAction{
		actionType: phcAdjustFrequency,
		freq:       freq,
	}
}