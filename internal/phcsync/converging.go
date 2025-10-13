package phcsync

import (
	"log/slog"
	"slices"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
)

// ConvergingConfig contains tunable parameters for converging mode.
type ConvergingConfig struct {
	KP        float64 // proportional gain
	KI        float64 // integral gain
	Window    int     // window size for tracking offsets
	Threshold int64   // max acceptable median offset to exit converging in nanoseconds (e.g. 1000 = 1µs)
}

func defaultConvergingConfig() ConvergingConfig {
	return ConvergingConfig{
		KP:        0.7,
		KI:        0.3,
		Window:    5,
		Threshold: 1000, // 1µs
	}
}

type convergingSampleGenerator struct{}

func newConvergingSampleGenerator() *convergingSampleGenerator {
	return &convergingSampleGenerator{}
}

func (g *convergingSampleGenerator) pulseEdgeSample(edge PulseEdge) *SampleData {
	// Round to nearest second
	refTime := edge.Timestamp.T.Round(time.Second)
	offset := edge.Timestamp.T.Sub(refTime)

	return &SampleData{
		Kind:   SampleOK,
		Ref:    refTime,
		Offset: offset,
		Era:    edge.Timestamp.Era,
	}
}

func (g *convergingSampleGenerator) timeMessageSample() *SampleData {
	// Converging mode only uses pulse edges
	return nil
}

type convergingSampleProcessor struct {
	servo      *piServo
	offsets    *circbuf.Buffer[time.Duration]
	lastMedian time.Duration
	window     int
	threshold  time.Duration
	lg         *slog.Logger
}

func newConvergingSampleProcessor(cfg ConvergingConfig, currentFreq, maxFreq float64, lg *slog.Logger) *convergingSampleProcessor {
	return &convergingSampleProcessor{
		servo:     newPiServo(currentFreq, cfg.KP, cfg.KI, maxFreq),
		offsets:   circbuf.New[time.Duration](cfg.Window),
		window:    cfg.Window,
		threshold: time.Duration(cfg.Threshold),
		lg:        lg,
	}
}

func (p *convergingSampleProcessor) processSample(sample *SampleData) (phcAction, controllerMode) {
	if sample.Kind == SampleMissing {
		// Ignore single missing sample
		return phcAction{actionType: phcNoAction}, modeConverging
	}

	// Add offset to buffer
	p.offsets.Append(sample.Offset)

	// Only check exit condition once we have enough samples
	if p.offsets.Len() >= p.window {
		// Calculate median of absolute offsets
		median := p.computeMedian()

		// Check exit condition: median not improving AND below threshold
		if p.lastMedian != 0 && median >= p.lastMedian && median <= p.threshold {
			p.lg.Info("converging complete",
				"median", median,
				"threshold", p.threshold)
			return phcAction{
				actionType: phcAdjustFrequency,
				freq:       p.servo.sample(sample.Offset),
			}, modeTracking
		}

		// Update last median for next iteration
		p.lastMedian = median
	}

	// Continue converging
	freq := p.servo.sample(sample.Offset)
	return phcAction{
		actionType: phcAdjustFrequency,
		freq:       freq,
	}, modeConverging
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
