package phcsync

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
	"github.com/jclark/satpulse/internal/ptime"
)

// ResetConfig contains tunable parameters for reset mode.
type ResetConfig struct {
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

	// PulseEdgeAmbig is the minimum difference (in seconds) between the high and low periods
	// of the pulse required to reliably determine which edge is leading. For a 1-second pulse
	// period, this can equivalently be interpreted as a proportion. For example, 0.1 means that
	// if |highPeriod - lowPeriod| < 0.1 seconds (i.e., pulse width between 0.45-0.55 seconds),
	// the pulse is too close to 50% duty cycle to determine edge polarity from timing alone.
	// This is measured with PHC timestamps, so can be quite accurate.
	// In such cases, both edge lists are kept and alignment with time messages is used instead.
	PulseEdgeAmbig float64
}

func defaultResetConfig() ResetConfig {
	return ResetConfig{
		Window:                 5,
		MinStep:                5000,  // nanoseconds
		PulseIntervalTolerance: 500.0, // PPB
		Delay:                  0.1,   // seconds
		DelayTightness:         0.6,   // proportion of max window
		DelaySpread:            0.2,   // proportion of max window
		PulseEdgeAmbig:		 0.1,  // seconds
	}
}

type resetSampleGenerator struct {
	timeMsgBuffer TimeMsgBuffer
	edgeBuf       *circbuf.Buffer[PulseEdge]
	cfg           ResetConfig
	lg            *slog.Logger
	pt            *PulseType
	maxFreq       float64
	freq          float64
	lastEdgeIndex uint64 // stores edgeIndex from most recent pulseEdgeSample call
}

func newResetSampleGenerator(timeMsgBuffer TimeMsgBuffer, cfg ResetConfig, pt *PulseType, freq, maxFreq float64, lg *slog.Logger) *resetSampleGenerator {
	// Buffer needs to hold Window * EdgesPerPulse edges
	bufSize := cfg.Window * pt.EdgesPerPulse
	return &resetSampleGenerator{
		timeMsgBuffer: timeMsgBuffer,
		edgeBuf:       circbuf.New[PulseEdge](bufSize),
		cfg:           cfg,
		lg:            lg,
		pt:            pt,
		maxFreq:       maxFreq,
		freq:          freq,
	}
}

func (g *resetSampleGenerator) pulseEdgeSample(edge PulseEdge, edgeIndex uint64) *Sample {
	g.storeEdge(edge, edgeIndex)
	return g.genSample()
}

// storeEdge appends an edge to the buffer and updates lastEdgeIndex.
// This is used during reset mode to collect edges for analysis.
// Tests can call this directly to populate the edge buffer.
func (g *resetSampleGenerator) storeEdge(edge PulseEdge, edgeIndex uint64) {
	g.edgeBuf.Append(edge)
	g.lastEdgeIndex = edgeIndex
}

func (g *resetSampleGenerator) timeMessageSample() *Sample {
	return g.genSample()
}

type loggableError interface {
	error
	log(lg *slog.Logger)
}

var errNotEnoughTimestamps = errors.New("not enough timestamps")

func (g *resetSampleGenerator) genSample() *Sample {
	// Need enough pulse edges
	// In dual-edge mode, we need EdgesPerPulse * Window edges total
	requiredEdges := g.cfg.Window * g.pt.EdgesPerPulse
	if g.edgeBuf.Len() < requiredEdges {
		return nil
	}

	// Get time messages from buffer
	lastSec, tRead := g.timeMsgBuffer.GetPostTimeMessages(g.cfg.Window)
	if lastSec.IsZero() {
		// Not enough time messages yet
		return nil
	}

	sample, err := g.genSampleForMessages(lastSec, tRead)
	if err != nil {
		if le, ok := err.(loggableError); ok {
			le.log(g.lg)
		} else if !errors.Is(err, errNotEnoughTimestamps) {
			g.lg.Info(err.Error())
		}
		return nil
	}

	return sample
}

// pulseEdgeList is a slice of PulseEdge values with the edgeIndex of the last edge.
type pulseEdgeList struct {
	edges         []PulseEdge
	lastEdgeIndex uint64
}

func (g *resetSampleGenerator) genSampleForMessages(lastSec ptime.Time, tRead []time.Time) (*Sample, error) {
	edgeLists := g.pulseEdgeLists()
	for _, edgeList := range edgeLists {
		err := g.checkPulseIntervals(edgeList)
		if err != nil {
			return nil, err
		}
	}
	
	edgeLists = g.filterEdgeListsByPulseWidth(edgeLists)
	if len(edgeLists) != 1 {
		return nil, errors.New("cannot filter edges")
	}
	// TODO: with 50% duty cycle, we would try the alignement of both edgeLists with a smaller maxWindow, so that at most one succeeds
	edges := edgeLists[0].edges
	avgInterval := edgeLists[0].avgInterval()
	pulseTimes := g.pulseTimes(edges, avgInterval)
	err := g.checkAlignment(pulseTimes, tRead)
	if err != nil {
		return nil, err
	}
	g.lastEdgeIndex = edgeLists[0].lastEdgeIndex

	lastPulseTimestamp := edges[len(edges)-1].Timestamp
	// offset is local - ref
	offset := lastPulseTimestamp.T.Sub(lastSec)

	return &Sample{
		SampleData: &SampleData{
			Kind:   SampleOK,
			Ref:    lastSec,
			Offset: offset,
			Freq:   0,
			Era:    lastPulseTimestamp.Era,
		},
		edgeIndex: g.lastEdgeIndex,
		Sys:       pulseTimes[len(pulseTimes)-1],
	}, nil
}

func (g *resetSampleGenerator) filterEdgeListsByPulseWidth(edgeLists []pulseEdgeList) []pulseEdgeList {
	if len(edgeLists) <= 1 {
		return edgeLists
	}
	totalPulseWidth := time.Duration(0)
	e0 := edgeLists[0].edges
	e1 := edgeLists[1].edges
	// the length of e1 can be up to 1 less than e0
	for i := 0; i < len(e1); i++ {
		totalPulseWidth += e1[i].Timestamp.T.Sub(e0[i].Timestamp.T)
	}
	avgPulseWidth := totalPulseWidth / time.Duration(len(e1))
	avgInterval := (edgeLists[0].avgInterval() + edgeLists[1].avgInterval())/2
	// on the PHC clock, one true second lasts a Duration of avgInterval,
	// so we need to scale the avgPulseWidth (which is from the PHC) to get true time
	truePulseWidth := time.Duration(float64(avgPulseWidth) * (float64(time.Second)/float64(avgInterval)))

	if math.Abs(2*truePulseWidth.Seconds()-1) < g.cfg.PulseEdgeAmbig {
		// Ambiguous pulse width, cannot determine alignment
		return edgeLists
	}
	if truePulseWidth < time.Second/2 {
		g.pt.PulseWidth = truePulseWidth
		return edgeLists[:1]
	}
	g.pt.PulseWidth = time.Second - truePulseWidth
	return edgeLists[1:]
}

type limitError struct {
	msg      string
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
	msg  string
	args []any
}

func (e *logMsgError) Error() string {
	return e.msg
}

func (e *logMsgError) log(lg *slog.Logger) {
	lg.Info(e.msg, e.args...)
}

// checkPulseIntervals validates that PHC pulse timestamps are stable and adjustable.
// It checks that all intervals are close enough to 1 second to be corrected within the PHC's
// frequency adjustment range, and that the intervals are consistent with each other.
func (g *resetSampleGenerator) checkPulseIntervals(edgeList pulseEdgeList) error {
	if edgeList.length() < 2 {
		return errNotEnoughTimestamps
	}
	timestamps := edgeList.timestamps()
	intervals := g.pulseIntervals(timestamps)

	err := g.checkPulseIntervalsAdjustable(intervals)
	if err != nil {
		return err
	}

	err = g.checkPulseIntervalsConsistent(intervals)
	if err != nil {
		return err
	}

	return nil
}

// pulseTimes estimates the real (monotonic) time when each pulse occurred.
// It uses avgInterval to scale the delay between when the pulse timestamp was captured
// and when it was read from the PHC, converting from PHC time domain to real time domain.
func (g *resetSampleGenerator) pulseTimes(edges []PulseEdge, avgInterval time.Duration) []time.Time {
	times := make([]time.Time, len(edges))
	for i, edge := range edges {
		phcDelta := edge.TReadPHC.T.Sub(edge.Timestamp.T)
		// Scale from PHC time domain to real time domain: avgInterval is how much PHC time equals 1 real second
		// Note this is assuming system clock frequency is reasonably accurate
		realDelta := time.Duration(float64(phcDelta) / avgInterval.Seconds())
		times[i] = edge.TRead.Add(-realDelta)
	}
	return times
}

// checkAlignment verifies that the estimated pulse times align with time message read times.
// It checks that delays between corresponding pulses and messages are consistent and within expected range.
func (g *resetSampleGenerator) checkAlignment(pulseTimes []time.Time, msgReadTimes []time.Time) error {
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

func (g *resetSampleGenerator) pulseDelays(pulseTimes []time.Time, msgReadTimes []time.Time) []time.Duration {
	if len(pulseTimes) != len(msgReadTimes) {
		panic("pulseDelays: pulseTimes and msgReadTimes must have same length")
	}
	delays := make([]time.Duration, len(pulseTimes))
	for i := range pulseTimes {
		delays[i] = msgReadTimes[i].Sub(pulseTimes[i])
	}
	return delays
}

func (g *resetSampleGenerator) checkDelaySpread(delays []time.Duration) error {
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

func (g *resetSampleGenerator) checkDelayRange(delays []time.Duration) error {
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

func (g *resetSampleGenerator) pulseEdgeLists() []pulseEdgeList {
	edgeList := g.pulseEdges()

	switch g.pt.EdgesPerPulse {
	case 0:
		panic("unknown edges per pulse is not supported: fix your ethernet driver")
	case 1:
		return []pulseEdgeList{edgeList}
	case 2:
		arr := edgeList.split()
		return arr[:]
	default:
		panic(fmt.Sprintf("unsupported edges per pulse: %d", g.pt.EdgesPerPulse))
	}
}

// pulseEdges extracts all edges from edgeBuf into a pulseEdgeList.
// Edges are ordered oldest to newest (same order as pulseTimestamps).
func (g *resetSampleGenerator) pulseEdges() pulseEdgeList {
	n := g.edgeBuf.Len()
	edges := make([]PulseEdge, n)
	g.edgeBuf.Iterate(func(i int, edge PulseEdge) bool {
		edges[i] = edge
		return true
	})
	slices.Reverse(edges)
	return pulseEdgeList{
		edges:         edges,
		lastEdgeIndex: g.lastEdgeIndex,
	}
}

func (pel pulseEdgeList) length() int {
	return len(pel.edges)
}

// split divides a pulseEdgeList into two lists with alternating edges.
// First edge (index 0) goes to first returned list, second edge to second list, etc.
// The lastEdgeIndex for each returned list is calculated based on which edges it contains.
func (pel pulseEdgeList) split() [2]pulseEdgeList {
	out := [2]pulseEdgeList{}
	for i := range 2 {
		for j := i; j < len(pel.edges); j += 2 {
			out[i].edges = append(out[i].edges, pel.edges[j])
		}
	}
	if len(pel.edges)%2 == 0 {
		out[0].lastEdgeIndex = pel.lastEdgeIndex - 1
		out[1].lastEdgeIndex = pel.lastEdgeIndex
	} else {
		out[0].lastEdgeIndex = pel.lastEdgeIndex
		out[1].lastEdgeIndex = pel.lastEdgeIndex - 1
	}
	return out
}

// edgeTimestamps extracts timestamps from a slice of edges.
func (pel pulseEdgeList) timestamps() []ptime.Time {
	edges := pel.edges
	timestamps := make([]ptime.Time, len(edges))
	for i, edge := range edges {
		timestamps[i] = edge.Timestamp.T
	}
	return timestamps
}

func (pel pulseEdgeList) avgInterval() time.Duration {
	totalInterval := pel.edges[len(pel.edges)-1].Timestamp.T.Sub(pel.edges[0].Timestamp.T)
	return totalInterval / time.Duration(len(pel.edges)-1)
}

func (g *resetSampleGenerator) pulseTimestamps() []ptime.Time {
	n := g.edgeBuf.Len()
	timestamps := make([]ptime.Time, n)
	for i := range n {
		timestamps[i] = g.edgeBuf.Last(n - 1 - i).Timestamp.T
	}
	return timestamps
}

func (g *resetSampleGenerator) pulseIntervals(timestamps []ptime.Time) []time.Duration {
	if len(timestamps) < 2 {
		panic("pulseIntervals: need at least 2 timestamps")
	}
	intervals := make([]time.Duration, len(timestamps)-1)
	for i := 1; i < len(timestamps); i++ {
		intervals[i-1] = timestamps[i].Sub(timestamps[i-1])
	}
	return intervals
}

func (g *resetSampleGenerator) checkPulseIntervalsAdjustable(intervals []time.Duration) error {
	for _, interval := range intervals {
		err := g.checkIntervalAdjustable(interval)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *resetSampleGenerator) checkIntervalAdjustable(interval time.Duration) error {
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

func (g *resetSampleGenerator) checkPulseIntervalsConsistent(intervals []time.Duration) error {
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

type resetSampleProcessor struct {
	minStep time.Duration
	lg      *slog.Logger
}

func newResetSampleProcessor(cfg ResetConfig, lg *slog.Logger) *resetSampleProcessor {
	return &resetSampleProcessor{
		minStep: time.Duration(cfg.MinStep),
		lg:      lg,
	}
}

func (p *resetSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	if sample == nil || sample.Kind == SampleMissing {
		// Keep waiting for a valid sample
		return phcAction{actionType: phcNoAction}, ModeReset
	}

	// Check if offset is small enough to skip stepping
	if sample.Offset.Abs() < p.minStep {
		p.lg.Info("clock already close, skipping step",
			"offset", sample.Offset,
			"minStep", p.minStep)
		return phcAction{actionType: phcNoAction}, ModeConverging
	}

	// Need to step the clock
	p.lg.Info("stepping clock in reset mode",
		"offset", sample.Offset,
		"minStep", p.minStep)
	return phcAction{
		actionType: phcStepClock,
		step:       -sample.Offset, // step by negative offset to correct
	}, ModeConverging
}
