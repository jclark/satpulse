package combine

import (
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

type Sampler interface {
	// Sample records a time sample that can be used to adjust the time.
	// ref is the reference time; local is our local time.
	// local is the time of the PHC at which the time pulse from the GPS was received.
	// ref is the time in the PTP time scale that the time pulse was aligned to by the GPS;
	// this the start of the second, but maybe adjusted for a few nanoseconds by applying a correction specified
	// by the GPS (sometimes called sawtooth correction).
	Sample(ref ptime.Time, local ptime.ClockTime, delayed bool)
}

type secMsgState struct {
	sec               ptime.Time
	pulseOff          *time.Duration
	navSolnMsgTRead   time.Time
	prePulseMsgTRead  time.Time
	postPulseMsgTRead time.Time
}

type secMsgList []*secMsgState

type pulseEdge struct {
	ptime.ClockTime
	tRead time.Time
}

type edgeFilter interface {
	// delayed can only be non-nil once
	// and delayed must be nil after include returns true
	include(edge pulseEdge, cfg *Config) (inc bool, delayed []pulseEdge)
}

type sampleData struct {
	sec         ptime.Time
	pulseOffset time.Duration
	pulse       pulseEdge
}

type PulseType struct {
	EdgesPerPulse int
	PulseWidth    time.Duration
}

type Config struct {
	PulsePollInterval  time.Duration // if the ethernet driver polls to detect a time pulse, then this is the interval between polls
	PulseReadDelay     time.Duration // the delay between the pulse and when we read it (not including PulsePollInterval)
	MessageWindowSize  time.Duration
	NavSolnDelay       time.Duration
	SerialDelay        time.Duration
	PulseWidthAccuracy time.Duration // when omitting trailing edges, how much variation in pulse width is allowed
	PulseWidthMax      time.Duration // when detecting trailing edges, what is the maximum pulse width we detect
}

func (cfg *Config) SetDefault(pt PulseType) {
	cfg.PulseReadDelay = 50 * time.Millisecond
	cfg.MessageWindowSize = 900 * time.Millisecond
	cfg.NavSolnDelay = 100 * time.Millisecond
	cfg.SerialDelay = 100 * time.Millisecond
	cfg.PulseWidthAccuracy = 10 * time.Millisecond
	cfg.PulseWidthMax = 400 * time.Millisecond
	cfg.PulsePollInterval = 0
}

func (cfg *Config) Validate() error {
	v := cfg.PulseWidthMax + cfg.PulseReadDelay + cfg.PulseWidthAccuracy
	if v >= 500*time.Millisecond {
		return errors.New("configured pulse width too large for pulse read delay and accuracy")
	}
	return nil
}

type Combiner struct {
	cfg               Config
	sampler           Sampler
	edgeFilter        edgeFilter
	lg                *slog.Logger
	pulses            []pulseEdge
	secMsgList        secMsgList
	waitPulseOffset   bool
	refSample         *sampleData
	lastSample        *sampleData
	PulseDiscardCount int
}

type matchQuality int

const (
	matchNone matchQuality = iota
	matchBad
	matchMedium
	matchGood
	matchGreat
)

type secMatch struct {
	sec ptime.Time
	q   matchQuality
}

func NewCombiner(pt PulseType, sampler Sampler, lg *slog.Logger, cfg Config) (*Combiner, error) {
	c := &Combiner{
		sampler: sampler,
		lg:      lg,
		cfg:     cfg,
	}
	if pt.EdgesPerPulse == 2 {
		if pt.PulseWidth == 0 {
			c.edgeFilter = &detectPulseWidthFilter{}
		} else {
			c.edgeFilter = &knownEdgeFilter{
				pulseWidth: pt.PulseWidth,
			}
		}
	} else if pt.EdgesPerPulse == 1 {
		c.edgeFilter = &noEdgeFilter{}
	} else {
		// XXX implement this
		return nil, errors.New("unsupported number of edges per pulse")
	}
	return c, nil
}

// TimeMsg handles a message from the GPS receiver that gives information about the current time.
func (c *Combiner) TimeMsg(sec ptime.Time, tRead time.Time, pulseOff *time.Duration, ref gpsprot.TimeRef) {
	sl, i, err := c.secMsgList.addSec(sec, len(c.pulses)+3) // we use extra messages in prePulseAdvance
	c.secMsgList = sl
	if err != nil {
		c.lg.Warn(err.Error(), "sec", sec)
		return
	}
	secState := c.secMsgList[i]
	if pulseOff != nil {
		secState.pulseOff = pulseOff
	}
	c.lg.Debug("combiner received time message",
		"sec", sec,
		"tRead", tRead,
		"ref", ref,
		"havePulseOff", pulseOff != nil,
		"pulseOff", secState.pulseOff,
		"index", i,
		"len", len(sl))
	switch ref {
	case gpsprot.NavSolution:
		if secState.navSolnMsgTRead.IsZero() {
			secState.navSolnMsgTRead = tRead
			c.tryUpgradeLastSample(secState)
		}
	case gpsprot.PrePulse:
		secState.prePulseMsgTRead = tRead
	case gpsprot.PostPulse:
		secState.postPulseMsgTRead = tRead
		if pulseOff != nil {
			c.waitPulseOffset = true
		}
	}
	if len(c.pulses) != 0 && ref != gpsprot.PrePulse {
		if c.refSample == nil {
			c.tryEmitFirstSample()
		} else {
			c.tryEmitNextSample()
		}
	}
}

const maxInitPulses = 5

// PulseEdge records the PHC time at which a pulse edge was received.
// tRead is the system time when we received the timestamp event from the kernel.
func (c *Combiner) PulseEdge(tClock ptime.ClockTime, tRead time.Time) {
	edge := pulseEdge{tClock, tRead}
	inc, delayed := c.edgeFilter.include(edge, &c.cfg)
	c.lg.Debug("combiner received pulse edge",
		"t", edge.T,
		"era", edge.Era,
		"tRead", edge.tRead,
		"included", inc,
		"delayedEdges", delayed != nil,
		"pending", len(c.pulses),
		"haveRefSample", c.refSample != nil)
	if c.refSample == nil {
		// limit to maxInitPulses but use all of the delayed edges
		excess := len(c.pulses) + len(delayed) - maxInitPulses
		if inc {
			excess++
		}
		nDiscarded := 0
		if excess >= len(c.pulses) {
			nDiscarded = len(c.pulses)
			c.pulses = delayed
		} else {
			if excess > 0 {
				nDiscarded = excess
				c.pulses = c.pulses[excess:]
			}
			if delayed != nil {
				c.pulses = append(c.pulses, delayed...)
			}
		}
		if nDiscarded > 0 {
			c.lg.Warn("discarded pulses during initialization", "n", nDiscarded)
			c.PulseDiscardCount += nDiscarded
		}
		if !inc {
			return
		}
		c.pulses = append(c.pulses, edge)
		c.tryEmitFirstSample()
	} else {
		if delayed != nil {
			panic("delayed edges should not occur after initialization")
		}
		if !inc {
			return
		}
		for _, p := range c.pulses {
			c.lg.Warn("failed to emit sample for pulse", "t", p.T)
		}
		c.PulseDiscardCount += len(c.pulses)
		c.pulses = c.pulses[:0]
		c.pulses = append(c.pulses, edge)
		c.tryEmitNextSample()
	}
}

func (c *Combiner) tryEmitFirstSample() {
	if len(c.pulses) == 0 {
		return
	}
	pulse := c.pulses[len(c.pulses)-1]
	q, i := c.secMsgList.bestMatch(pulse, &c.cfg)
	if q < matchMedium {
		return
	}
	secMsg := c.secMsgList[i]
	sample := &sampleData{
		sec:         secMsg.sec,
		pulse:       pulse,
		pulseOffset: secMsg.pulseOffset(),
	}
	delayed := c.delayedSamples(i, sample)
	if len(delayed) == 0 {
		// wait for at least one consistent sample
		return
	}
	for _, s := range delayed {
		c.lg.Debug("combiner about to emit delayed sample")
		c.emit(&s, true)
	}
	c.refSample = sample
	c.lastSample = sample
	c.lg.Debug("combiner about to emit first non-delayed sample")
	c.emit(sample, false)
	c.pulses = c.pulses[:0]
}

func (c *Combiner) delayedSamples(msgIndex int, refSample *sampleData) []sampleData {
	pulseIndex := len(c.pulses) - 1
	if pulseIndex == 0 {
		return nil
	}
	samples := make([]sampleData, pulseIndex)
	nextSample := refSample
	for pulseIndex > 0 && msgIndex > 0 {
		pulse := c.pulses[pulseIndex-1]
		secMsg := c.secMsgList[msgIndex-1]
		sample := sampleData{
			sec:         secMsg.sec,
			pulse:       pulse,
			pulseOffset: secMsg.pulseOffset(),
		}
		nextSec, _ := sample.chooseNextSec(c.pulses[pulseIndex], nil, &c.cfg)
		if nextSec != nextSample.sec {
			break
		}
		pulseIndex--
		msgIndex--
		samples[pulseIndex] = sample
		nextSample = &sample
	}
	return samples[pulseIndex:]
}

func (c *Combiner) tryEmitNextSample() {
	if c.refSample == nil {
		panic("tryEmitNextSample when sample is nil")
	}

	if len(c.pulses) == 0 {
		return
	}
	if len(c.pulses) != 1 {
		panic("tryEmitNextSample when number of pulses is more than 1")
	}
	pulse := c.pulses[0]
	sec, useAsRef := c.refSample.chooseNextSec(pulse, c.secMsgList, &c.cfg)
	if sec == 0 {
		return
	}
	off, wait := c.findPulseOffset(sec)
	if wait {
		return
	}
	c.pulses = c.pulses[:0]
	sample := sampleData{
		sec:         sec,
		pulse:       pulse,
		pulseOffset: off,
	}
	c.lastSample = &sample
	if useAsRef {
		c.refSample = &sample
	}
	c.emit(&sample, false)
}

func (c *Combiner) emit(sample *sampleData, delayed bool) {
	c.sampler.Sample(sample.refTime(), sample.pulse.ClockTime, delayed)
	c.lastSample = sample
}

func (c *Combiner) tryUpgradeLastSample(secState *secMsgState) {
	if c.lastSample == nil {
		return
	}
	if c.lastSample == c.refSample {
		return
	}
	if c.lastSample.sec != secState.sec {
		return
	}
	q := c.cfg.navSolnMsgMatchQuality(secState.navSolnMsgTRead.Sub(c.lastSample.pulse.tRead))
	if q < matchMedium {
		return
	}
	c.refSample = c.lastSample
}

func (c *Combiner) findPulseOffset(sec ptime.Time) (off time.Duration, wait bool) {
	i := c.secMsgList.search(sec)
	var found *secMsgState
	if i < len(c.secMsgList) && c.secMsgList[i].sec == sec {
		found = c.secMsgList[i]
	}
	if found != nil && found.pulseOff != nil {
		off = *found.pulseOff
	} else if c.waitPulseOffset && (found == nil || found.postPulseMsgTRead.IsZero()) {
		wait = true
	}
	return
}

// bestMatch finds the second that is the best match for a pulse in a list of seconds.
// It returns the quality of the match and the index of the message.
// It does not consider messages of the kind that are emitted before the pulse.
func (sl secMsgList) bestMatch(pulse pulseEdge, cfg *Config) (matchQuality, int) {
	pt := pulse.tRead
	qualities := make([]matchQuality, len(sl))
	var bestMatchQual = matchNone
	var bestMatchIndex = -1
	for i := len(sl) - 1; i >= 0; i-- {
		q := matchNone
		delay := time.Duration(0)
		if !sl[i].postPulseMsgTRead.IsZero() {
			delay = sl[i].postPulseMsgTRead.Sub(pt)
			q = cfg.postPulseMsgMatchQuality(delay)
		} else if !sl[i].navSolnMsgTRead.IsZero() {
			delay = sl[i].navSolnMsgTRead.Sub(pt)
			q = cfg.navSolnMsgMatchQuality(delay)
		}
		if q > bestMatchQual {
			bestMatchQual = q
			bestMatchIndex = i
		}
		if q < bestMatchQual {
			// past local maximum
			break
		}
		if delay < 0 {
			// delay is going to decrease by about a second on next iteration
			// need to have < 0 here, because delay above is 0 if there is only a PrePulse message
			break
		}
	}
	if bestMatchQual == matchNone || bestMatchIndex > 0 && qualities[bestMatchIndex-1] == bestMatchQual {
		return matchNone, -1
	}
	return bestMatchQual, bestMatchIndex
}

func (sml secMsgList) prePulseMatch(pulse pulseEdge, cfg *Config) (secMatch secMatch) {
	if len(sml) < 3 {
		return
	}
	s := sml[len(sml)-1]
	t := s.prePulseMsgTRead
	if t.IsZero() {
		return
	}
	if !s.postPulseMsgTRead.IsZero() || !s.navSolnMsgTRead.IsZero() {
		return
	}
	advance := sml[:len(sml)-1].predictPrePulseAdvance(cfg)
	if advance == 0 {
		return
	}
	t = t.Add(advance)
	delay := t.Sub(pulse.tRead)
	q := matchNone

	isPostPulse := !sml[len(sml)-2].postPulseMsgTRead.IsZero()
	if isPostPulse {
		q = cfg.postPulseMsgMatchQuality(delay)
	} else {
		q = cfg.navSolnMsgMatchQuality(delay)
	}
	if q == matchNone {
		return
	}
	secMatch.q = q
	secMatch.sec = s.sec
	return
}

func (sml secMsgList) predictPrePulseAdvance(cfg *Config) time.Duration {
	if len(sml) == 0 {
		return 0
	}
	navMsgCount := 0
	advances := make([]time.Duration, len(sml))
	total := time.Duration(0)
	for i, s := range sml {
		t1 := s.prePulseMsgTRead
		if t1.IsZero() {
			return 0
		}
		if i > 0 && s.sec != sml[i-1].sec.Add(time.Second) {
			return 0
		}
		t2 := s.postPulseMsgTRead
		if t2.IsZero() {
			t2 = s.navSolnMsgTRead
			if t2.IsZero() {
				return 0
			}
			navMsgCount++
		}
		advances[i] = t2.Sub(t1)
		total += advances[i]
	}
	if navMsgCount != 0 && navMsgCount != len(sml) {
		return 0
	}
	minAdv := slices.Min(advances)
	maxAdv := slices.Max(advances)
	if maxAdv-minAdv > cfg.SerialDelay {
		return 0
	}
	return total / time.Duration(len(sml))
}

var errOutOfOrderMsg = errors.New("out of order message")

// Add a new message to the list of messages.
// If there's already a message for the same second, then don't add it again.
// Return the possibly modified list, the index of the message,
// and an error if a message for that second should not be possible now.
// discard messages from the front if necessary to keep length <= maxLength
func (sml secMsgList) addSec(sec ptime.Time, maxLength int) (secMsgList, int, error) {
	if maxLength < 1 {
		panic("maxLength must be >= 1")
	}
	i := sml.search(sec)
	for j := i; j < len(sml); j++ {
		if sml[j].happened() {
			return sml, -1, errOutOfOrderMsg
		}
	}
	if i < len(sml) && sml[i].sec == sec {
		return sml, i, nil
	}
	if len(sml) >= maxLength {
		// we want new length now to be maxLength - 1
		// but if we found it, don't throw it out for now
		excess := len(sml) - (maxLength - 1)
		sml = sml[excess:]
		i -= excess
		if i < 0 {
			i = 0
		}
	}
	newSec := secMsgState{sec: sec}
	if i == len(sml) {
		sml = append(sml, &newSec)
		return sml, i, nil
	}
	sml = slices.Insert(sml, i, &newSec)
	return sml, i, nil
}

// search returns the greatest index i such that for all j < i, sl[j].sec < sec
// The return value is >= 0 and <= len(sl).
// Assumes that secStates is in ascending order of sec with no duplicates.
// Note that when sec is greater than all sl[j].sec, then i == len(sl).
// The implementation searchs from the end of the slice, since this will be the common case.
func (sml secMsgList) search(sec ptime.Time) int {
	i := len(sml)
	for i > 0 && sml[i-1].sec >= sec {
		i--
	}
	return i
}

// chooseNextSec chooses the second for a pulse based on an existing sample.
// It returns 0 if no second can be chosen.
// The bool return value says whether the choice is good enough to be used as the reference sample.
func (s *sampleData) chooseNextSec(pulse pulseEdge, sml secMsgList, cfg *Config) (ptime.Time, bool) {
	phcMatch := s.phcMatch(pulse, cfg)
	readMatch := s.readMatch(pulse, cfg)
	phcQ := phcMatch.q
	readQ := readMatch.q
	maxQ := max(phcQ, readQ)
	msgMatch := sml.prePulseMatch(pulse, cfg)
	msgMatchIsAfter := false
	if msgMatch.q == matchNone && len(sml) != 0 {
		q, i := sml.bestMatch(pulse, cfg)
		if q != matchNone {
			msgMatch = secMatch{sml[i].sec, q}
			msgMatchIsAfter = true
		}
	}
	if maxQ >= matchGood && phcMatch.sec == readMatch.sec && min(phcMatch.q, readMatch.q) > matchBad {
		return phcMatch.sec, msgMatch.q > matchBad
	}

	if phcMatch.q == matchNone {
		msgQ := msgMatch.q
		maxQ = max(readQ, msgQ)
		if readMatch.sec == msgMatch.sec && min(readMatch.q, msgMatch.q) >= matchMedium {
			return readMatch.sec, msgMatchIsAfter || maxQ >= matchGood
		}
	}
	// XXX need to add matches from messages

	// if we have a PrePulse message and it's consistent, then we can use as a reference sample right away
	return 0, false
}

func (s *sampleData) readMatch(pulse pulseEdge, cfg *Config) secMatch {
	sec, frac := nextSec(s.sec, pulse.subTRead(s.pulse))
	return secMatch{sec, cfg.readSampleMatchQuality(frac)}
}

func (s *sampleData) phcMatch(pulse pulseEdge, cfg *Config) secMatch {
	t, ok := pulse.subT(s.pulse)
	if !ok {
		return secMatch{}
	}
	sec, frac := nextSec(s.sec, t)
	return secMatch{sec, cfg.phcSampleMatchQuality(frac)}
}

// The time according to the GPS at which the pulse occurred.
func (s *sampleData) refTime() ptime.Time {
	// the pulse offset is the time of the pulse minus the top of the second
	return s.sec.Add(s.pulseOffset)
}

func (s *secMsgState) pulseOffset() time.Duration {
	if s.pulseOff == nil {
		return 0
	}
	return *s.pulseOff
}

func (s *secMsgState) happened() bool {
	return !s.navSolnMsgTRead.IsZero() || !s.postPulseMsgTRead.IsZero()
}

type noEdgeFilter struct{}

func (f *noEdgeFilter) include(_ pulseEdge, _ *Config) (bool, []pulseEdge) {
	return true, nil
}

// knownEdgeFilter filters edges that are the trailing edge of the pulse
// This is for the case where we both know that we will be getting two edges per pulse
// and know the pulse width.
type knownEdgeFilter struct {
	pulseWidth  time.Duration
	prev        pulseEdge
	prevIsFirst bool
}

// include returns true if the pulse edge should be treated as the significant edge of a pulse.
// If we don't include the second edge and do include the third edge, then we return the first edge as delayed along with the third edge.
// If we do include the second edge, then there's no delayed edge.
func (f *knownEdgeFilter) include(edge pulseEdge, cfg *Config) (bool, []pulseEdge) {
	prev := f.prev
	f.prev = edge
	if prev.isZero() {
		f.prevIsFirst = true
		return false, nil
	}
	prevIsFirst := f.prevIsFirst
	f.prevIsFirst = false
	off := edge.tRead.Sub(prev.tRead)
	if (f.pulseWidth - off).Abs() < cfg.PulseWidthAccuracy+cfg.PulseReadDelay {
		var delayed []pulseEdge
		if prevIsFirst {
			delayed = []pulseEdge{prev}
		}
		return false, delayed
	}
	return true, nil
}

type detectPulseWidthFilter struct {
	avgPulseWidth time.Duration
	pulseWidths   []time.Duration
	prev          pulseEdge
	prevIsFirst   bool
}

func (f *detectPulseWidthFilter) include(edge pulseEdge, cfg *Config) (bool, []pulseEdge) {
	prev := f.prev
	f.prev = edge

	off := edge.tRead.Sub(prev.tRead)
	tolerance := cfg.PulseWidthAccuracy + cfg.PulseReadDelay
	if f.avgPulseWidth != 0 {
		// we use tolerance*2 here because our guess of the pulseWidth may be wrong by up to tolerance
		// (unlikely, but possible)
		max := min(f.avgPulseWidth+tolerance*2, cfg.PulseWidthMax+tolerance)
		min := f.avgPulseWidth - tolerance*2
		return !(min <= off && off <= max), nil
	}
	if prev.isZero() {
		f.prevIsFirst = true
		return false, nil
	}
	prevIsFirst := f.prevIsFirst
	f.prevIsFirst = false
	if off <= cfg.PulseWidthMax+tolerance {
		var delayed []pulseEdge
		if prevIsFirst {
			delayed = []pulseEdge{prev}
		}
		f.pulseWidths = append(f.pulseWidths, off)
		f.detectPulseWidth(cfg)
		return false, delayed
	}
	return true, nil
}

func (f *detectPulseWidthFilter) detectPulseWidth(cfg *Config) {
	const usePulseWidths = 9
	const nOutliers = 2
	if len(f.pulseWidths) < usePulseWidths {
		return
	}
	// discard outliers and then use the average of the rest
	slices.Sort(f.pulseWidths)
	widths := f.pulseWidths[nOutliers : len(f.pulseWidths)-nOutliers]
	sum := time.Duration(0)
	for _, w := range widths {
		sum += w
	}
	f.avgPulseWidth = sum / time.Duration(len(widths))
	if f.avgPulseWidth > cfg.PulseWidthMax {
		f.avgPulseWidth = cfg.PulseWidthMax
	}
}

func (p pulseEdge) isZero() bool {
	return p.tRead.IsZero()
}

func (p pulseEdge) subTRead(q pulseEdge) time.Duration {
	return p.tRead.Sub(q.tRead)
}

func (p pulseEdge) subT(q pulseEdge) (time.Duration, bool) {
	if p.Era != q.Era || p.Era.Uncertain() {
		return 0, false
	}
	return p.T.Sub(q.T), true
}

// delay is the tRead of the message minus the tRead of the pulse
func (cfg *Config) navSolnMsgMatchQuality(delay time.Duration) matchQuality {
	return cfg.msgMatchQuality(delay, cfg.NavSolnDelay)
}

// delay is the tRead of the message minus the tRead of the pulse
func (cfg *Config) postPulseMsgMatchQuality(delay time.Duration) matchQuality {
	return cfg.msgMatchQuality(delay, 0)
}

func (cfg *Config) msgMatchQuality(delay time.Duration, extraExpectedDelay time.Duration) matchQuality {
	windowStart := -cfg.PulsePollInterval - cfg.PulseReadDelay
	if delay < windowStart || delay > windowStart+cfg.MessageWindowSize {
		return matchNone
	}
	if delay < windowStart+cfg.SerialDelay+extraExpectedDelay {
		return matchGood
	}
	return matchMedium
}

func (cfg *Config) readSampleMatchQuality(off time.Duration) matchQuality {
	off = off.Abs()
	if off <= time.Millisecond {
		return matchGood
	}
	if off <= cfg.PulsePollInterval+cfg.PulseReadDelay {
		return matchMedium
	}
	return matchBad
}

func (*Config) phcSampleMatchQuality(off time.Duration) matchQuality {
	off = off.Abs()
	if off < time.Microsecond {
		return matchGreat
	}
	if off < 100*time.Millisecond {
		return matchGood
	}
	return matchBad
}

// nextSec returns the time of the following second closest to d after ref.
// ref is assumed to be rounded to a second.
// sec is the the time the following second (rounded to a second).
// frac is in the range [0, 0.5] says how far  sec is from ref + d.
// More precisely: sec + frac == ref + d
func nextSec(ref ptime.Time, d time.Duration) (sec ptime.Time, frac time.Duration) {
	whole := d.Round(time.Second)
	frac = d - whole
	sec = ref.Add(whole)
	// We have
	// frac = d - whole
	// sec = ref + whole
	// Therefore
	// frac + sec = ref + d
	return
}
