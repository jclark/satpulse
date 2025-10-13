package phcsync

import (
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
)

// InitConfig contains tunable parameters for init mode.
type InitConfig struct {
	Window  int   // number of pulses to collect (e.g. 5)
	MinStep int64 // minimum offset in nanoseconds to perform a step (e.g. 5000 = 5µs)
	// TODO: add more config as needed for alignment
}

func defaultInitConfig() InitConfig {
	return InitConfig{
		Window:  5,
		MinStep: 5000, // 5 microseconds
	}
}

type initSampleGenerator struct {
	timeMsgBuffer TimeMsgBuffer
	edgeBuf       *circbuf.Buffer[PulseEdge]
	cfg           InitConfig
	lg            *slog.Logger
}

func newInitSampleGenerator(timeMsgBuffer TimeMsgBuffer, cfg InitConfig, lg *slog.Logger) *initSampleGenerator {
	return &initSampleGenerator{
		timeMsgBuffer: timeMsgBuffer,
		edgeBuf:       circbuf.New[PulseEdge](cfg.Window),
		cfg:           cfg,
		lg:            lg,
	}
}

func (g *initSampleGenerator) pulseEdgeSample(edge PulseEdge) *SampleData {
	g.edgeBuf.Append(edge)
	return g.genSample()
}

func (g *initSampleGenerator) timeMessageSample() *SampleData {
	return g.genSample()
}

func (g *initSampleGenerator) genSample() *SampleData {
	// Need enough pulse edges
	if g.edgeBuf.Len() < g.cfg.Window {
		return nil
	}

	// Get time messages from buffer
	lastSec, tRead := g.timeMsgBuffer.GetPostTimeMessages(g.cfg.Window)
	if lastSec.IsZero() {
		// Not enough time messages yet
		return nil
	}

	// TODO: Check alignment between pulses and time messages
	// Will use tRead for alignment checking
	_ = tRead
	return nil
}

type initSampleProcessor struct {
	minStep time.Duration
	lg      *slog.Logger
}

func newInitSampleProcessor(cfg InitConfig, lg *slog.Logger) *initSampleProcessor {
	return &initSampleProcessor{
		minStep: time.Duration(cfg.MinStep),
		lg:      lg,
	}
}

func (p *initSampleProcessor) processSample(sample *SampleData) (phcAction, controllerMode) {
	if sample == nil || sample.Kind == SampleMissing {
		// Keep waiting for a valid sample
		return phcAction{actionType: phcNoAction}, modeInit
	}

	// Check if offset is small enough to skip stepping
	if sample.Offset.Abs() < p.minStep {
		p.lg.Info("clock already close, skipping step",
			"offset", sample.Offset,
			"minStep", p.minStep)
		return phcAction{actionType: phcNoAction}, modeConverging
	}

	// Need to step the clock
	p.lg.Info("stepping clock in init mode",
		"offset", sample.Offset,
		"minStep", p.minStep)
	return phcAction{
		actionType: phcStepClock,
		step:       -sample.Offset, // step by negative offset to correct
	}, modeConverging
}