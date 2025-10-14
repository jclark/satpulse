package phcsync

import (
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
	"github.com/jclark/satpulse/internal/ptime"
)

// InitConfig contains tunable parameters for init mode.
type InitConfig struct {
	Window int // number of pulses to collect (e.g. 5)

	MinStep int64 // minimum offset in nanoseconds to perform a step (e.g. 5000 = 5µs)

	PulseIntervalTolerance float64 // max variation between intervals in PPB (e.g. 500)

	// Delay is the expected midpoint of the pulse-to-message delay window in seconds.
	// Typical GPS receivers send time messages 50-250ms after the pulse, so 0.1 (100ms) is reasonable.
	Delay float64

	// DelayTightness controls how much of the maximum possible delay window to accept (0.0 to 1.0).
	// The maximum possible window is 1.0 second (time until next pulse) with edge timestamping,
	// or 0.5 seconds with 50% duty cycle timestamping. This parameter specifies what fraction
	// of that maximum to accept, centered around Delay. For example, 0.6 with maxWindow=1.0
	// accepts delays from Delay-0.3 to Delay+0.3.
	DelayTightness float64

	// DelaySpread is the maximum acceptable variation between pulse-to-message delays,
	// expressed as a proportion of the maximum window. For example, 0.1 with maxWindow=1.0
	// means (max delay - min delay) must be ≤ 0.1 seconds. This checks consistency: all
	// delays should be similar. Should be significantly smaller than DelayTightness.
	DelaySpread float64
}

func defaultInitConfig() InitConfig {
	return InitConfig{
		Window:                 5,
		MinStep:                5000,  // nanoseconds
		PulseIntervalTolerance: 500.0, // PPB
		Delay:                  0.1,   // seconds
		DelayTightness:         0.6,   // proportion of max window
		DelaySpread:            0.2,   // proportion of max window
	}
}

type initSampleGenerator struct {
	timeMsgBuffer TimeMsgBuffer
	edgeBuf       *circbuf.Buffer[PulseEdge]
	cfg           InitConfig
	lg            *slog.Logger
	maxFreq       float64
	freq          float64
}

func newInitSampleGenerator(timeMsgBuffer TimeMsgBuffer, cfg InitConfig, maxFreq, freq float64, lg *slog.Logger) *initSampleGenerator {
	return &initSampleGenerator{
		timeMsgBuffer: timeMsgBuffer,
		edgeBuf:       circbuf.New[PulseEdge](cfg.Window),
		cfg:           cfg,
		lg:            lg,
		maxFreq:       maxFreq,
		freq:          freq,
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

	avgInterval, err := g.checkPulseIntervals()
	if err != nil {
		logError(g.lg, err)
		return nil
	}

	pulseTimes := g.pulseTimes(avgInterval)
	err = g.checkAlignment(pulseTimes, tRead)
	if err != nil {
		logError(g.lg, err)
		return nil
	}

	lastPulseTimestamp := g.edgeBuf.Last(0).Timestamp
	// offset is local - ref
	offset := lastPulseTimestamp.T.Sub(lastSec)

	return &SampleData{
		Kind:   SampleOK,
		Ref:    lastSec,
		Offset: offset,
		Freq:   0,
		Era:    lastPulseTimestamp.Era,
	}
}

type loggableError interface {
	error
	log(lg *slog.Logger)	
}

type limitError struct {
	msg string
	propName string
	value    any
	limit    any
}

func (e *limitError) Error() string {
	return e.msg
}

func (e *limitError) log(lg *slog.Logger) {
	lg.Info(e.msg,
		"property", e.propName,
		"value", e.value,
		"limit", e.limit)
}

type logMsgError struct {
	msg string
	args []any
}

func (e *logMsgError) Error() string {
	return e.msg
}

func (e *logMsgError) log(lg *slog.Logger) {
	lg.Info(e.msg, e.args...)
}

func logError(lg *slog.Logger, err error) {
	if err == nil {
		return
	}
	if le, ok := err.(loggableError); ok {
		le.log(lg)
	} else if !errors.Is(err, errNotEnoughTimestamps) {
		lg.Info(err.Error())
	}
}

var errNotEnoughTimestamps = errors.New("not enough timestamps")

// checkPulseIntervals validates that PHC pulse timestamps are stable and adjustable.
// It checks that all intervals are close enough to 1 second to be corrected within the PHC's
// frequency adjustment range, and that the intervals are consistent with each other.
func (g *initSampleGenerator) checkPulseIntervals() (avgInterval time.Duration, err error) {
	timestamps := g.pulseTimestamps()
	if len(timestamps) < 2 {
		return 0, errNotEnoughTimestamps
	}

	intervals := g.pulseIntervals(timestamps)
	avgInterval = timestamps[len(timestamps)-1].Sub(timestamps[0]) / time.Duration(len(timestamps)-1)

	err = g.checkPulseIntervalsAdjustable(intervals)
	if err != nil {
		return 0, err
	}

	err = g.checkPulseIntervalsConsistent(intervals)
	if err != nil {
		return 0, err
	}

	return avgInterval, nil
}

// pulseTimes estimates the real (monotonic) time when each pulse occurred.
// It uses avgInterval to scale the delay between when the pulse timestamp was captured
// and when it was read from the PHC, converting from PHC time domain to real time domain.
func (g *initSampleGenerator) pulseTimes(avgInterval time.Duration) []time.Time {
	times := make([]time.Time, g.edgeBuf.Len())
	g.edgeBuf.Iterate(func(i int, edge PulseEdge) bool {
		phcDelta := edge.TReadPHC.T.Sub(edge.Timestamp.T)
		// Scale from PHC time domain to real time domain: avgInterval is how much PHC time equals 1 real second
		// Note this is assuming system clock frequency is reasonably accurate
		realDelta := time.Duration(float64(phcDelta) / avgInterval.Seconds())
		times[i] = edge.TRead.Add(-realDelta)
		return true
	})
	slices.Reverse(times)
	return times
}

// checkAlignment verifies that the estimated pulse times align with time message read times.
// It checks that delays between corresponding pulses and messages are consistent and within expected range.
func (g *initSampleGenerator) checkAlignment(pulseTimes []time.Time, msgReadTimes []time.Time) error {
	delays := g.pulseDelays(pulseTimes, msgReadTimes)

	err := g.checkDelaySpread(delays)
	if err != nil {
		return err
	}

	err = g.checkDelayRange(delays)
	if err != nil {
		return err
	}

	return nil
}

func (g *initSampleGenerator) pulseDelays(pulseTimes []time.Time, msgReadTimes []time.Time) []time.Duration {
	if len(pulseTimes) != len(msgReadTimes) {
		panic("pulseDelays: pulseTimes and msgReadTimes must have same length")
	}
	delays := make([]time.Duration, len(pulseTimes))
	for i := range pulseTimes {
		delays[i] = msgReadTimes[i].Sub(pulseTimes[i])
	}
	return delays
}

func (g *initSampleGenerator) checkDelaySpread(delays []time.Duration) error {
	if len(delays) == 0 {
		panic("checkDelaySpread: need at least 1 delay")
	}

	minDelay, maxDelay := delays[0], delays[0]
	for _, d := range delays {
		if d < minDelay {
			minDelay = d
		}
		if d > maxDelay {
			maxDelay = d
		}
	}

	maxWindow := 1.0
	spread := maxDelay - minDelay
	spreadProportion := spread.Seconds() / maxWindow

	if spreadProportion > g.cfg.DelaySpread {
		return &limitError{
			msg:      "spread in delays between pulse and message too large",
			propName: "DelaySpread",
			value:    spreadProportion,
			limit:    g.cfg.DelaySpread,
		}
	}
	return nil
}

func (g *initSampleGenerator) checkDelayRange(delays []time.Duration) error {
	maxWindow := 1.0
	halfAcceptableWindow := g.cfg.DelayTightness * maxWindow / 2
	minAcceptable := g.cfg.Delay - halfAcceptableWindow
	maxAcceptable := g.cfg.Delay + halfAcceptableWindow

	for _, delay := range delays {
		delaySec := delay.Seconds()
		if delaySec < minAcceptable || delaySec > maxAcceptable {
			return &logMsgError{
				msg: "delay between pulse and message outside acceptable range",
				args: []any{
					"value", delaySec,
					"minAcceptable", minAcceptable,
					"maxAcceptable", maxAcceptable,
					"property", "Delay+DelayTightness",
				},
			}
		}
	}

	return nil
}

func (g *initSampleGenerator) pulseTimestamps() []ptime.Time {
	n := g.edgeBuf.Len()
	timestamps := make([]ptime.Time, n)
	for i := range n {
		timestamps[i] = g.edgeBuf.Last(n - 1 - i).Timestamp.T
	}
	return timestamps
}

func (g *initSampleGenerator) pulseIntervals(timestamps []ptime.Time) []time.Duration {
	if len(timestamps) < 2 {
		panic("pulseIntervals: need at least 2 timestamps")
	}
	intervals := make([]time.Duration, len(timestamps)-1)
	for i := 1; i < len(timestamps); i++ {
		intervals[i-1] = timestamps[i].Sub(timestamps[i-1])
	}
	return intervals
}

func (g *initSampleGenerator) checkPulseIntervalsAdjustable(intervals []time.Duration) error {
	for _, interval := range intervals {
		err := g.checkIntervalAdjustable(interval)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *initSampleGenerator) checkIntervalAdjustable(interval time.Duration) error {
	deviationNanos := interval - time.Second
	deviationPPB := float64(deviationNanos) * 1e9 / float64(time.Second)
	neededFreq := g.freq - deviationPPB
	if neededFreq >= -g.maxFreq && neededFreq <= g.maxFreq {
		return nil
	}
	return &logMsgError{
		msg: "PHC timestamp interval is too far from 1 second",
		args: []any{
			"interval", interval,
			"deviationPPB", deviationPPB,
			"freqPPB", g.freq,
			"maxFreqPPB", g.maxFreq,
		},
	}
}

func (g *initSampleGenerator) checkPulseIntervalsConsistent(intervals []time.Duration) error {
	if len(intervals) == 0 {
		panic("checkPulseIntervalsConsistent: need at least 1 interval")
	}

	minInterval, maxInterval := intervals[0], intervals[0]
	for _, interval := range intervals {
		if interval < minInterval {
			minInterval = interval
		}
		if interval > maxInterval {
			maxInterval = interval
		}
	}

	variationPPB := (float64(maxInterval)/float64(minInterval) - 1.0) * 1e9
	if variationPPB < 0 {
		variationPPB = -variationPPB
	}

	if variationPPB > g.cfg.PulseIntervalTolerance {
		return &limitError{
			msg:      "pulse intervals not consistent",
			propName: "PulseIntervalTolerance",
			value:    variationPPB,
			limit:    g.cfg.PulseIntervalTolerance,
		}
	}
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
