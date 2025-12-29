package phcsync

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
	"github.com/jclark/satpulse/internal/ptime"
)

// ResetConfig contains tunable parameters for reset mode.
type ResetConfig struct {
	// PulseWindow is the number of pulses to collect for alignment analysis during reset mode.
	// A larger window provides more data for statistical checks but delays the initial clock step.
	PulseWindow int `toml:"pulseWindow" check:">=3,<100" comment:"Number of pulses for alignment analysis"`

	// StepThreshold is the minimum absolute offset in nanoseconds required to perform a clock step.
	// If the measured offset is smaller than this threshold, reset mode transitions directly to
	// converging mode without stepping the clock.
	StepThreshold int64 `toml:"stepThreshold" check:">=0,<1_000_000" comment:"Min offset to trigger clock step (ns)"`

	// PulseVariation is the maximum acceptable variation between consecutive pulse intervals,
	// expressed in parts per billion (PPB). This checks clock stability: if the variation
	// between the shortest and longest interval exceeds this limit, the pulses are rejected.
	// The variation is computed as: (maxInterval/minInterval - 1.0) * 1e9.
	PulseVariation float64 `toml:"pulseVariation" check:">=5,<1_000_000" comment:"Max pulse interval variation (ppb)"`

	// ExpectedDelay is the expected pulse-to-message delay in seconds.
	// This represents the typical delay between when a PPS pulse occurs and when the GPS
	// receiver sends the corresponding time message. Most GPS receivers send messages
	// 50-250ms after the pulse.
	ExpectedDelay float64 `toml:"expectedDelay" check:">=0.0,<1.0" comment:"Expected pulse-to-message delay (s)"`

	// DelayConfidenceWindow specifies what fraction of the maximum possible delay window
	// to accept, expressed as a proportion (0.0 to 1.0). The accepted window has width
	// DelayConfidenceWindow*maxWindow and includes ExpectedDelay. If centering that window
	// on ExpectedDelay would make the lower bound negative, it is shifted up so the lower
	// bound is 0 while keeping the window width the same.
	DelayConfidenceWindow float64 `toml:"delayConfidenceWindow" check:">0.0,<=1.0" comment:"Fraction of delay window to accept [0,1]"`

	// DelayVariation is the maximum acceptable spread between pulse-to-message delays,
	// expressed as a proportion of the maximum window (1.0 second). This checks consistency:
	// all delays should be similar. The spread is computed as: (maxDelay - minDelay) / 1.0.
	// Should be significantly smaller than DelayConfidenceWindow to ensure tight clustering.
	DelayVariation float64 `toml:"delayVariation" check:">0.0,<1.0" comment:"Max delay spread as fraction of window"`

	// PulseWidthDetectLimit is the maximum pulse width in seconds that can be automatically
	// detected in dual-edge mode to determine which edge is leading. Pulse widths greater
	// than this value (and by symmetry, less than 1.0 - this value) are too close to 50%
	// duty cycle for reliable auto-detection from timing alone. When detection fails, both
	// edge lists are kept and alignment with time messages is used. Note: pulse widths
	// greater than 0.5 seconds must be explicitly configured via gps.pulseWidth.
	PulseWidthDetectLimit float64 `toml:"pulseWidthDetectLimit" check:">=0.1,<0.5" comment:"Max auto-detectable pulse width (s)"`
}

func defaultResetConfig() ResetConfig {
	return ResetConfig{
		PulseWindow:           5,
		StepThreshold:         5000,  // nanoseconds
		PulseVariation:        500.0, // PPB
		ExpectedDelay:         0.1,   // seconds
		DelayConfidenceWindow: 0.6,   // proportion of max window
		DelayVariation:        0.2,   // proportion of max window
		PulseWidthDetectLimit: 0.45,  // seconds
	}
}

// DelayBounds returns the acceptable delay range in seconds for a given maxWindow.
// The range has width DelayConfidenceWindow*maxWindow, includes ExpectedDelay, and never
// extends below 0 seconds.
func (cfg ResetConfig) DelayBounds(maxWindow float64) (minAcceptable, maxAcceptable float64) {
	halfWindow := cfg.DelayConfidenceWindow * maxWindow / 2
	minAcceptable = cfg.ExpectedDelay - halfWindow
	if minAcceptable < 0 {
		truncated := -minAcceptable
		minAcceptable = 0
		maxAcceptable = cfg.ExpectedDelay + halfWindow + truncated
		return minAcceptable, maxAcceptable
	}
	maxAcceptable = cfg.ExpectedDelay + halfWindow
	return minAcceptable, maxAcceptable
}

type pulseInfo struct {
	pulseWidth  time.Duration // discovered pulse width
	avgInterval time.Duration // average PHC interval between pulses
}

// pulseEdgeList is a slice of PulseEdge values with the edgeIndex of the last edge.
type pulseEdgeList struct {
	edges         []PulseEdge
	lastEdgeIndex uint64
	pulseWidth    time.Duration // discovered pulse width (0 if single-edge mode or unknown)
}

type resetStats struct {
	pulseVariation float64       // actual pulse interval variation in PPB (compare to cfg.PulseVariation)
	delay          float64       // mean pulse-to-message delay in seconds (compare to cfg.ExpectedDelay)
	delayVariation float64       // delay spread as proportion of 1s (compare to cfg.DelayVariation)
	pulseSysOffset time.Duration // mean absolute deviation of pulse times from exact seconds
}

type resetSampleGenerator struct {
	timeMsgBuffer TimeMsgBuffer
	edgeBuf       *circbuf.Buffer[PulseEdge]
	cfg           ResetConfig
	lg            *slog.Logger
	pt            PulseType
	maxFreq       float64
	freq          float64
	lastEdgeIndex uint64        // stores edgeIndex from most recent pulseEdgeSample call
	avgInterval   time.Duration // stored from last successful sample generation
}

func newResetSampleGenerator(timeMsgBuffer TimeMsgBuffer, cfg ResetConfig, pt PulseType, freq, maxFreq float64, lg *slog.Logger) *resetSampleGenerator {
	// Buffer needs to hold Window * EdgesPerPulse edges
	bufSize := cfg.PulseWindow * pt.EdgesPerPulse
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

func (g *resetSampleGenerator) getPulseInfo() pulseInfo {
	return pulseInfo{
		pulseWidth:  g.pt.PulseWidth,
		avgInterval: g.avgInterval,
	}
}

type loggableError interface {
	error
	log(lg *slog.Logger)
}

var errNotEnoughTimestamps = errors.New("not enough timestamps")

// genSample attempts to generate a sample by aligning pulse edges with time messages.
//
// This function is called both when a pulse edge arrives (via pulseEdgeSample) and when
// a time message arrives (via timeMessageSample). We try on both events because the
// arrival order is unpredictable: on some hardware (e.g., Raspberry Pi CM4/CM5), the
// kernel only delivers pulse timestamps every 0.25s, so a pulse that occurred before
// a message was sent may be delivered after that message is received.
//
// The challenge is matching pulses to their corresponding time messages. Pulses are
// timestamped in the PHC time domain; messages are read in the system clock domain.
// To align them, we estimate when each pulse occurred in system clock time:
//  1. Compute the delay from pulse occurrence to read time in PHC time
//  2. Scale that delay to system clock time using avgInterval (how much PHC time
//     equals one real second)
//  3. Subtract from the system clock read time to get estimated pulse time
//
// We then check that the delays between estimated pulse times and message read times
// are consistent and within the expected range. If they are, we have correctly matched
// pulses to their GPS seconds. If not, we must wait for more data.
//
// In dual-edge mode (both rising and falling edges timestamped), we must also determine
// which edge marks the top of the second. If the pulse width is not close to 0.5s, we
// can tell from the time between pulse edges. Otherwise, we try both possibilities and
// use message alignment to disambiguate.
func (g *resetSampleGenerator) genSample() *Sample {
	// Need enough pulse edges
	// In dual-edge mode, we need EdgesPerPulse * Window edges total
	requiredEdges := g.cfg.PulseWindow * g.pt.EdgesPerPulse
	if g.edgeBuf.Len() < requiredEdges {
		return nil
	}

	// Get time messages from buffer
	lastSec, tRead := g.timeMsgBuffer.GetPostTimeMessages(g.cfg.PulseWindow)
	if lastSec.IsZero() {
		// Not enough time messages yet
		return nil
	}

	sample, stats, err := g.genSampleForMessages(lastSec, tRead)
	if err != nil {
		if le, ok := err.(loggableError); ok {
			le.log(g.lg)
		} else if !errors.Is(err, errNotEnoughTimestamps) {
			g.lg.Info(err.Error())
		}
		return nil
	}

	if sample != nil {
		g.lg.Info("reset mode succeeded",
			"pulseVariation", fmt.Sprintf("%.1f", stats.pulseVariation),
			"delay", stats.delay,
			"delayVariation", stats.delayVariation,
			"pulseSysOffset", stats.pulseSysOffset)
	}

	return sample
}

// genSampleForMessages is the core logic of genSample, factored out for unit testing.
// It takes the collected pulse edges and time messages and attempts alignment.
func (g *resetSampleGenerator) genSampleForMessages(lastSec ptime.Time, tRead []time.Time) (*Sample, *resetStats, error) {
	stats := &resetStats{}
	edgeLists := g.pulseEdgeLists()
	for _, edgeList := range edgeLists {
		err := g.checkPulseIntervals(edgeList, stats)
		if err != nil {
			return nil, nil, err
		}
	}

	edgeLists = g.filterEdgeListsByPulseWidth(edgeLists)

	// Choose maxWindow based on number of edge lists
	// With 2 lists (ambiguous ~50% duty cycle), use smaller window to disambiguate
	maxWindow := 1.0
	if len(edgeLists) > 1 {
		maxWindow = 0.5
	}

	// Try aligning each edge list; exactly one should succeed
	var sample *Sample
	var alignErrs []error
	for _, edgeList := range edgeLists {
		tryStats := *stats // copy base stats for this try
		trySample, err := g.tryAlignment(edgeList, lastSec, tRead, maxWindow, &tryStats)
		if err != nil {
			alignErrs = append(alignErrs, err)
			continue
		}
		if sample != nil {
			return nil, nil, errors.New("failed to disambiguate pulse edges")
		}
		sample = trySample
		*stats = tryStats
		g.pt.PulseWidth = edgeList.pulseWidth
	}

	if sample == nil {
		if len(alignErrs) == 1 {
			return nil, nil, alignErrs[0]
		}
		return nil, nil, fmt.Errorf("both possible pulse edge alignments failed: %v; %v", alignErrs[0], alignErrs[1])
	}
	return sample, stats, nil

}

// tryAlignment attempts to align a single edge list with time messages.
// Returns a sample if alignment succeeds, or an error if alignment fails.
// The maxWindow parameter (in seconds) controls the acceptable delay range.
// This function has side effects: it updates g.lastEdgeIndex and g.avgInterval on success.
func (g *resetSampleGenerator) tryAlignment(edgeList pulseEdgeList, lastSec ptime.Time, tRead []time.Time, maxWindow float64, stats *resetStats) (*Sample, error) {
	edges := edgeList.edges
	avgInterval := edgeList.avgInterval()
	pulseTimes := g.pulseTimes(edges, avgInterval)
	err := g.checkAlignment(pulseTimes, tRead, maxWindow, stats)
	if err != nil {
		return nil, err
	}
	g.lastEdgeIndex = edgeList.lastEdgeIndex
	g.avgInterval = avgInterval

	lastPulseTimestamp := edges[len(edges)-1].Timestamp
	// offset is local - ref
	offset := lastPulseTimestamp.T.Sub(lastSec)

	return &Sample{
		Kind:      SampleOK,
		Ref:       lastSec,
		Offset:    offset,
		Freq:      0,
		Era:       lastPulseTimestamp.Era,
		EdgeIndex: g.lastEdgeIndex,
		Sys:       pulseTimes[len(pulseTimes)-1],
		// Mode will be filled in by processSample
	}, nil
}

// filterEdgeListsByPulseWidth tries to filter out an edge list by measuring the time
// between alternating edges.
//
// In dual-edge mode, we receive both rising and falling edges interleaved. When split
// into two edge lists, one contains all the rising edges and the other all the falling
// edges, but we don't know which is which. Only one is aligned to the top of the GPS second.
//
// The time between consecutive edges is either the pulse width or its complement to 1s -
// we don't know which without knowing which edge type came first.
//
// If this measured time is within PulseWidthDetectLimit, we know it's the pulse width
// (since pulse widths > 0.5s must be explicitly configured). If it's greater than
// (1s - PulseWidthDetectLimit), we know it's the complement. In either case, we can
// determine which edge list to use. If the measured time is close to 0.5s, we cannot
// distinguish, so we return both lists and rely on message alignment to disambiguate.
func (g *resetSampleGenerator) filterEdgeListsByPulseWidth(edgeLists []pulseEdgeList) []pulseEdgeList {
	if len(edgeLists) <= 1 {
		return edgeLists
	}

	// Calculate the true pulse width from the two edge lists
	totalPulseWidth := time.Duration(0)
	e0 := edgeLists[0].edges
	e1 := edgeLists[1].edges
	// the length of e1 can be up to 1 less than e0
	n := len(e1)
	if len(e0) < n {
		n = len(e0)
	}
	for i := 0; i < n; i++ {
		totalPulseWidth += e1[i].Timestamp.T.Sub(e0[i].Timestamp.T)
	}
	avgPulseWidth := totalPulseWidth / time.Duration(n)
	avgInterval := (edgeLists[0].avgInterval() + edgeLists[1].avgInterval()) / 2
	// on the PHC clock, one true second lasts a Duration of avgInterval,
	// so we need to scale the avgPulseWidth (which is from the PHC) to get true time
	truePulseWidth := time.Duration(float64(avgPulseWidth) * (float64(time.Second) / float64(avgInterval)))

	pulseWidthDetectLimit := time.Duration(g.cfg.PulseWidthDetectLimit * float64(time.Second))
	if truePulseWidth <= pulseWidthDetectLimit {
		// Short pulse width, use first edge list
		edgeLists[0].pulseWidth = truePulseWidth
		return edgeLists[:1]
	} else if pw := time.Second - truePulseWidth; pw <= pulseWidthDetectLimit {
		// Long pulse width, use second edge list
		edgeLists[1].pulseWidth = pw
		return edgeLists[1:]
	} else {
		// Ambiguous pulse width (close to 50% duty cycle), return both with pulse widths set
		edgeLists[0].pulseWidth = truePulseWidth
		edgeLists[1].pulseWidth = time.Second - truePulseWidth
		return edgeLists
	}
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
func (g *resetSampleGenerator) checkPulseIntervals(edgeList pulseEdgeList, stats *resetStats) error {
	if edgeList.length() < 2 {
		return errNotEnoughTimestamps
	}
	timestamps := edgeList.timestamps()
	intervals := g.pulseIntervals(timestamps)

	err := g.checkPulseIntervalsAdjustable(intervals)
	if err != nil {
		return err
	}

	err = g.checkPulseIntervalsConsistent(intervals, stats)
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
		phcDelta := edge.TRead.Clock.T.Sub(edge.Timestamp.T)
		// Scale from PHC time domain to real time domain: avgInterval is how much PHC time equals 1 real second
		// Note this is assuming system clock frequency is reasonably accurate
		realDelta := time.Duration(float64(phcDelta) / avgInterval.Seconds())
		times[i] = edge.TRead.Sys.Add(-realDelta)
	}
	return times
}

// checkAlignment verifies that the estimated pulse times align with time message read times.
// It checks that delays between corresponding pulses and messages are consistent and within expected range.
// The maxWindow parameter specifies the maximum delay window in seconds (typically 1.0, or 0.5 for 50% duty cycle disambiguation).
func (g *resetSampleGenerator) checkAlignment(pulseTimes []time.Time, msgReadTimes []time.Time, maxWindow float64, stats *resetStats) error {
	delays := g.pulseDelays(pulseTimes, msgReadTimes)

	err := g.checkDelaySpread(delays, maxWindow, stats)
	if err != nil {
		return err
	}

	err = g.checkDelayRange(delays, maxWindow)
	if err != nil {
		return err
	}

	// Calculate mean absolute deviation from exact second boundaries
	var totalOffset time.Duration
	for _, pt := range pulseTimes {
		offset := pt.Sub(pt.Round(time.Second))
		if offset < 0 {
			offset = -offset
		}
		totalOffset += offset
	}
	stats.pulseSysOffset = totalOffset / time.Duration(len(pulseTimes))

	return nil
}

// pulseDelays computes the time from each estimated pulse occurrence to its corresponding
// message read time.
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

func (g *resetSampleGenerator) checkDelaySpread(delays []time.Duration, maxWindow float64, stats *resetStats) error {
	if len(delays) == 0 {
		panic("checkDelaySpread: need at least 1 delay")
	}

	minDelay, maxDelay := delays[0], delays[0]
	sum := time.Duration(0)
	for _, d := range delays {
		sum += d
		if d < minDelay {
			minDelay = d
		}
		if d > maxDelay {
			maxDelay = d
		}
	}

	mean := sum / time.Duration(len(delays))
	stats.delay = mean.Seconds()

	spread := maxDelay - minDelay
	spreadProportion := spread.Seconds() / maxWindow
	stats.delayVariation = spreadProportion

	if spreadProportion > g.cfg.DelayVariation {
		return &limitError{
			msg:      "spread in delays between pulse and message too large",
			propName: "DelaySpread",
			value:    spreadProportion,
			limit:    g.cfg.DelayVariation,
		}
	}
	return nil
}

func (g *resetSampleGenerator) checkDelayRange(delays []time.Duration, maxWindow float64) error {
	minAcceptable, maxAcceptable := g.cfg.DelayBounds(maxWindow)

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

// pulseEdgeLists returns edge lists to try for alignment. In single-edge mode, returns
// one list. In dual-edge mode, returns two lists (one for each edge type) which are
// then filtered by filterEdgeListsByPulseWidth.
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

func (g *resetSampleGenerator) checkPulseIntervalsConsistent(intervals []time.Duration, stats *resetStats) error {
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

	stats.pulseVariation = variationPPB

	if variationPPB > g.cfg.PulseVariation {
		return &limitError{
			msg:      "pulse intervals not consistent",
			propName: "PulseIntervalTolerance",
			value:    variationPPB,
			limit:    g.cfg.PulseVariation,
		}
	}
	return nil
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

type resetSampleProcessor struct {
	minStep time.Duration
	lg      *slog.Logger
}

func newResetSampleProcessor(cfg ResetConfig, lg *slog.Logger) *resetSampleProcessor {
	return &resetSampleProcessor{
		minStep: time.Duration(cfg.StepThreshold),
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
