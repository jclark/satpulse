package phcsync

import (
	"log/slog"
	"slices"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
)

// ConvergingConfig contains tunable parameters for converging mode.
type ConvergingConfig struct {
	// KP is the proportional gain for the PI servo.
	KP float64
	// KI is the integral gain for the PI servo.
	KI float64
	// Window is the number of samples in the sliding window for computing median of absolute offsets.
	Window int
	// Threshold is the maximum acceptable absolute offset in nanoseconds (e.g. 1000 = 1µs).
	// Converging mode exits to tracking when all of the following conditions hold:
	// - the median of absolute offsets in the window has not decreased for StableSamples consecutive samples
	// - the absolute offset of every sample since the minimum median was observed is <= Threshold
	Threshold int64
	// StableSamples is the number of consecutive samples for which the minimum median must remain stable before exiting.
	// The minimum median is considered stable when it has not decreased.
	// If any sample's absolute offset exceeds Threshold, the stability counter resets to 0.
	StableSamples int
	// MaxMissingSamples is the maximum number of missing samples before transitioning back to reset mode.
	MaxMissingSamples int
}

func defaultConvergingConfig() ConvergingConfig {
	return ConvergingConfig{
		KP:                0.7,
		KI:                0.3,
		Window:            5,
		Threshold:         1000, // 1µs
		StableSamples:     3,
		MaxMissingSamples: 3,
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
		servo:   newPiServo(currentFreq, cfg.KP, cfg.KI, maxFreq),
		offsets: circbuf.New[time.Duration](cfg.Window),
		lg:      lg,
	}
}

func (p *convergingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	if sample.Kind == SampleMissing {
		p.missingSamples++
		p.lg.Info("missing sample in converging mode", "missingSamples", p.missingSamples)
		if p.missingSamples >= p.cfg.MaxMissingSamples {
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
			"threshold", time.Duration(p.cfg.Threshold),
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
	if p.offsets.Len() < p.cfg.Window {
		return false
	}
	if p.offsets.Last(0).Abs() > time.Duration(p.cfg.Threshold) {
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
	return p.samplesSinceMinDecreased >= p.cfg.StableSamples
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
