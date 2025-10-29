package phcsync

import (
	"log/slog"
	"slices"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
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
	OffsetLimit int64 `toml:"offsetLimit" check:">0,<1_000_000_000"`

	// StableWindow is the number of consecutive samples for which the minimum median must
	// remain stable (not decrease) before exiting converging mode. This ensures the offset
	// has truly stabilized rather than just momentarily dipping below the threshold.
	// Typical value: 3.
	StableWindow int `toml:"stableWindow" check:">=1,<100"`

	// BadSampleLimit is the maximum number of consecutive missing samples before transitioning
	// back to reset mode. Missing samples indicate loss of PPS signal or time messages.
	// Typical value: 3.
	BadSampleLimit int `toml:"badSampleLimit" check:">=1,<100"`
}

func defaultConvergingConfig() ConvergingConfig {
	return ConvergingConfig{
		Kp:                0.7,
		Ki:                0.3,
		MedianWindow:            5,
		OffsetLimit:         1000, // 1µs
		StableWindow:     3,
		BadSampleLimit: 3,
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
		leadingEdgeIndex: lastSample.edgeIndex,
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
	phcDelta := edge.TReadPHC.T.Sub(edge.Timestamp.T)
	sys := edge.TRead.Add(-phcDelta)

	return &Sample{
		SampleData: &SampleData{
			Kind:   SampleOK,
			Ref:    refTime,
			Offset: offset,
			Era:    edge.Timestamp.Era,
		},
		edgeIndex: edgeIndex,
		Sys:       sys,
	}
}

func (g *convergingSampleGenerator) timeMessageSample() *Sample {
	// Converging mode only uses pulse edges
	return nil
}

type convergingSampleProcessor struct {
	cfg                      ConvergingConfig
	servo                    *piServo
	offsets                  *circbuf.Buffer[time.Duration]
	minMedian                time.Duration
	samplesSinceMinDecreased int
	missingSamples           int
	lg                       *slog.Logger
}

func newConvergingSampleProcessor(cfg ConvergingConfig, currentFreq, maxFreq float64, lg *slog.Logger) *convergingSampleProcessor {
	return &convergingSampleProcessor{
		cfg:     cfg,
		servo:   newPiServo(currentFreq, cfg.Kp, cfg.Ki, maxFreq),
		offsets: circbuf.New[time.Duration](cfg.MedianWindow),
		lg:      lg,
	}
}

func (p *convergingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	if sample.Kind == SampleMissing {
		p.missingSamples++
		p.lg.Info("missing sample in converging mode", "missingSamples", p.missingSamples)
		if p.missingSamples >= p.cfg.BadSampleLimit {
			return phcAction{actionType: phcNoAction}, ModeReset
		}
		return phcAction{actionType: phcNoAction}, ModeConverging
	}
	p.offsets.Append(sample.Offset)
	if p.converged() {
		median := p.computeMedian()
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

func (p *convergingSampleProcessor) converged() bool {
	if p.offsets.Len() < p.cfg.MedianWindow {
		return false
	}
	if p.offsets.Last(0).Abs() > time.Duration(p.cfg.OffsetLimit) {
		p.samplesSinceMinDecreased = 0
		return false
	}
	median := p.computeMedian()
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

func (p *convergingSampleProcessor) computeMedian() time.Duration {
	n := p.offsets.Len()
	if n == 0 {
		return 0
	}

	// Collect absolute values of offsets
	values := make([]time.Duration, 0, n)
	p.offsets.Iterate(func(i int, d time.Duration) bool {
		values = append(values, d.Abs())
		return true
	})

	slices.Sort(values)

	mid := n / 2
	if n%2 != 0 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}
