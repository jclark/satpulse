package phcsync

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/mon"
)

// TrackingConfig contains tunable parameters for tracking mode.
type TrackingConfig struct {
	KP               float64 // proportional gain
	KI               float64 // integral gain
	OutlierThreshold int64   // minimum absolute offset in nanoseconds for sample to be considered an outlier
}

func defaultTrackingConfig() TrackingConfig {
	return TrackingConfig{
		KP:               0.7,
		KI:               0.3,
		OutlierThreshold: 1000, // 1µs
	}
}

type trackingSampleGenerator struct{}

func newTrackingSampleGenerator() *trackingSampleGenerator {
	return &trackingSampleGenerator{}
}

func (g *trackingSampleGenerator) pulseEdgeSample(edge PulseEdge) *SampleData {
	// Round to nearest second - identical to converging for now
	// Later will handle falling edge detection differently
	refTime := edge.Timestamp.T.Round(time.Second)
	offset := edge.Timestamp.T.Sub(refTime)

	return &SampleData{
		Kind:      SampleOK,
		Ref:       refTime,
		Offset:    offset,
		Era:       edge.Timestamp.Era,
		SyncState: mon.InSync,
	}
}

func (g *trackingSampleGenerator) timeMessageSample() *SampleData {
	// Tracking mode only uses pulse edges for now
	return nil
}

type trackingSampleProcessor struct {
	servo            *piServo
	outlierThreshold time.Duration
	lg               *slog.Logger
}

func newTrackingSampleProcessor(cfg TrackingConfig, currentFreq, maxFreq float64, lg *slog.Logger) *trackingSampleProcessor {
	return &trackingSampleProcessor{
		servo:            newPiServo(currentFreq, cfg.KP, cfg.KI, maxFreq),
		outlierThreshold: time.Duration(cfg.OutlierThreshold),
		lg:               lg,
	}
}

func (p *trackingSampleProcessor) processSample(sample *SampleData) (phcAction, controllerMode) {
	if sample.Kind == SampleMissing {
		// Just continue tracking on missing samples
		return phcAction{actionType: phcNoAction}, modeTracking
	}

	// Check for outlier
	if sample.Offset.Abs() >= p.outlierThreshold {
		// Mark as outlier and don't adjust
		sample.Kind = SampleOutlier
		p.lg.Debug("outlier detected", "offset", sample.Offset, "threshold", p.outlierThreshold)
		return phcAction{actionType: phcNoAction}, modeTracking
	}

	// Apply PI control for good samples
	freq := p.servo.sample(sample.Offset)
	return phcAction{
		actionType: phcAdjustFrequency,
		freq:       freq,
	}, modeTracking
}
