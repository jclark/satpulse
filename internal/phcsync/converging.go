package phcsync

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/median"
	"github.com/jclark/satpulse/internal/ptime"
)

// ConvergingConfig contains tunable parameters for converging mode.
type ConvergingConfig struct {
	// Kp is the proportional gain for the PI servo used during converging mode.
	// Higher values make the servo more responsive but may cause oscillation.
	// Typical value: 0.7.
	Kp float64 `toml:"kp" check:">0.0,<10.0"`

	// Ki is the integral gain for the PI servo used during converging mode.
	// This accumulates error over time to eliminate steady-state offset.
	// Typical value: 0.3.
	Ki float64 `toml:"ki" check:">=0.0,<10.0"`

	// MedianWindow is the number of samples in the sliding window for computing the median
	// of absolute offsets. The median is used to track convergence progress: when it stops
	// decreasing and stabilizes below OffsetLimit, converging mode exits to tracking mode.
	// Typical value: 5.
	MedianWindow int `toml:"medianWindow" check:">=3,<100"`

	// OffsetLimit is the maximum acceptable absolute offset in nanoseconds for declaring
	// convergence complete. Converging mode exits to tracking when both conditions hold:
	// (1) the median of absolute offsets has not decreased for StableWindow consecutive samples,
	// and (2) every sample since the minimum median was observed has absolute offset <= OffsetLimit.
	// If any sample exceeds this limit, the stability counter resets. Typical value: 1000 (1µs).
	OffsetLimit int64 `toml:"offsetLimit" check:">0,<=10000"`

	// StableWindow is the number of consecutive samples for which the minimum median must
	// remain stable (not decrease) before exiting converging mode. This ensures the offset
	// has truly stabilized rather than just momentarily dipping below the threshold.
	// Typical value: 3.
	StableWindow int `toml:"stableWindow" check:">=1,<100"`

	// BadSampleLimit is the maximum number of consecutive missing samples before transitioning
	// back to reset mode. Missing samples indicate loss of PPS signal or time messages.
	// Typical value: 3.
	BadSampleLimit int `toml:"badSampleLimit" check:">=1,<100"`

	// StepCompensate enables compensation for ADJ_SETOFFSET delay at the beginning of
	// converging mode. ADJ_SETOFFSET is implemented in the kernel by calling the driver
	// to read the current PHC time, adding the offset, then calling the driver again to
	// write back the adjusted time. This read-modify-write sequence takes a few microseconds,
	// causing the clock to lag behind by that amount. If enabled, the offset from the first
	// pulse in converging mode is used to apply a compensation step. Typical value: true.
	StepCompensate bool `toml:"stepCompensate"`
}

func defaultConvergingConfig() ConvergingConfig {
	return ConvergingConfig{
		Kp:             0.7,
		Ki:             0.3,
		MedianWindow:   5,
		OffsetLimit:    1000, // 1µs
		StableWindow:   3,
		BadSampleLimit: 3,
		StepCompensate: true,
	}
}

type convergingSampleGenerator struct {
	cfg              ConvergingConfig
	pt               PulseType
	leadingEdgeIndex uint64
	freq             float64
	maxFreq          float64
	lg               *slog.Logger
}

func newConvergingSampleGenerator(cfg ConvergingConfig, pt PulseType, lastSample *Sample, freq, maxFreq float64, lg *slog.Logger) *convergingSampleGenerator {
	return &convergingSampleGenerator{
		cfg:              cfg,
		pt:               pt,
		leadingEdgeIndex: lastSample.EdgeIndex,
		freq:             freq,
		maxFreq:          maxFreq,
		lg:               lg,
	}
}

func (g *convergingSampleGenerator) pulseEdgeSample(edge PulseEdge, edgeIndex uint64) *Sample {
	// Wait for any step to take effect
	if edge.Timestamp.Era.Uncertain() {
		return nil
	}
	// Filter out trailing edges when EdgesPerPulse == 2
	if g.pt.EdgesPerPulse == 2 && (g.leadingEdgeIndex^edgeIndex)&1 != 0 {
		return nil
	}

	// Round to nearest second
	refTime := edge.Timestamp.T.Round(time.Second)
	offset := edge.Timestamp.T.Sub(refTime)

	// Estimate system time
	phcDelta := edge.TRead.Clock.T.Sub(edge.Timestamp.T)
	sys := edge.TRead.Sys.Add(-phcDelta)

	return &Sample{
		Kind:      SampleOK,
		Ref:       refTime,
		Offset:    offset,
		Era:       edge.Timestamp.Era,
		EdgeIndex: edgeIndex,
		Sys:       sys,
		// Freq and Mode will be filled in by processSample
	}
}

func (g *convergingSampleGenerator) timeMessageSample() *Sample {
	// Converging mode only uses pulse edges
	return nil
}

type convergingSampleProcessor struct {
	cfg                      ConvergingConfig
	servo                    *piServo
	offsets                  *median.Window[time.Duration]
	lastSampleEra            ptime.Era
	stepCompensate           bool
	minMedian                time.Duration
	samplesSinceMinDecreased int
	missingSamples           int
	lg                       *slog.Logger
}

func newConvergingSampleProcessor(cfg ConvergingConfig, lastSample *Sample, currentFreq, maxFreq float64, lg *slog.Logger) *convergingSampleProcessor {
	return &convergingSampleProcessor{
		cfg:            cfg,
		servo:          newPiServo(currentFreq, cfg.Kp, cfg.Ki, maxFreq),
		offsets:        median.New[time.Duration](cfg.MedianWindow),
		lastSampleEra:  lastSample.Era,
		stepCompensate: cfg.StepCompensate,
		lg:             lg,
	}
}

func (p *convergingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	if action := p.handleCompensation(sample); action.actionType != phcNoAction {
		return action, ModeConverging
	}

	if sample.Kind == SampleMissing {
		p.missingSamples++
		p.lg.Info("missing sample in converging mode", "missingSamples", p.missingSamples)
		if p.missingSamples >= p.cfg.BadSampleLimit {
			return phcAction{actionType: phcNoAction}, ModeReset
		}
		return phcAction{actionType: phcNoAction}, ModeConverging
	}
	p.offsets.Add(sample.Offset.Abs())
	if p.converged() {
		median := p.offsets.Median()
		p.lg.Info("converging complete",
			"median", median,
			"minMedian", p.minMedian,
			"threshold", time.Duration(p.cfg.OffsetLimit),
			"stableSamples", p.samplesSinceMinDecreased)
		return phcAction{
			actionType: phcAdjustFrequency,
			freq:       p.servo.sample(sample.Offset),
		}, ModeTracking
	}
	freq := p.servo.sample(sample.Offset)
	return phcAction{
		actionType: phcAdjustFrequency,
		freq:       freq,
	}, ModeConverging
}

func (p *convergingSampleProcessor) handleCompensation(sample *Sample) phcAction {
	if !p.stepCompensate {
		return phcAction{}
	}

	// Only try the step compensation on the first sample in converging mode.
	// If that sample is missing, skip the compensation entirely,
	// since the estimate would become increasingly unreliable with more missing samples.
	p.stepCompensate = false

	if sample.Kind == SampleMissing {
		p.lg.Info("skipping ADJ_SETOFFSET compensation due to missing first pulse")
		return phcAction{}
	}

	if sample.Era > p.lastSampleEra {
		// We stepped! Compensate by 2× the measured offset.
		// 2x because one covers error in first adjustment and the other covers expected error in this adjustment.
		compensationStep := -2 * sample.Offset
		p.lg.Info("compensating for ADJ_SETOFFSET delay",
			"measuredOffset", sample.Offset,
			"compensationStep", compensationStep)
		return phcAction{
			actionType: phcStepClock,
			step:       compensationStep,
		}
	}

	return phcAction{}
}

func (p *convergingSampleProcessor) converged() bool {
	if p.offsets.Len() < p.cfg.MedianWindow {
		return false
	}
	// Check if the most recent absolute offset exceeds the limit
	if p.offsets.Last() > time.Duration(p.cfg.OffsetLimit) {
		p.samplesSinceMinDecreased = 0
		return false
	}
	median := p.offsets.Median()
	if p.samplesSinceMinDecreased == 0 {
		p.minMedian = median
		p.samplesSinceMinDecreased = 1

	} else if median < p.minMedian {
		p.minMedian = median
		p.samplesSinceMinDecreased = 1
	} else {
		p.samplesSinceMinDecreased++
	}
	return p.samplesSinceMinDecreased >= p.cfg.StableWindow
}
