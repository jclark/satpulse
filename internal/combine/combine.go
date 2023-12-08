package combine

import (
	"errors"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

type pulseEdge struct {
	ptime.ClockTime
	tRead time.Time
}

type Sampler interface {
	// Sample records a time sample that can be used to adjust the time.
	// ref is the reference time; local is our local time.
	// local is the time of the PHC at which the time pulse from the GPS was received.
	// ref is the time in the PTP time scale that the time pulse was aligned to by the GPS;
	// this the start of the second, but maybe adjusted for a few nanoseconds by applying a correction specified
	// by the GPS (sometimes called sawtooth correction).
	Sample(ref ptime.Time, local ptime.ClockTime, delayed bool)
}

type edgeFilter interface {
	// delayed can only be non-nil once
	// and delayed must be nil after include returns true
	include(edge pulseEdge) (inc bool, delayed []pulseEdge)
}

type noEdgeFilter struct{}

func (f *noEdgeFilter) include(edge pulseEdge) (bool, []pulseEdge) {
	return true, nil
}

type knownEdgeFilter struct {
	pulseWidth  time.Duration
	prev        pulseEdge
	prevIsFirst bool
}

const pulseWidthAccuracy = 10 * time.Millisecond

// include returns true if the pulse edge should be treated as the significant edge of a pulse.
// If we don't include the second edge and do include the third edge, then we return the first edge as delayed along with the third edge.
// If we do include the second edge, then there's no delayed edge.
func (f *knownEdgeFilter) include(edge pulseEdge) (bool, []pulseEdge) {
	prev := f.prev
	f.prev = edge
	if prev.isZero() {
		f.prevIsFirst = true
		return false, nil
	}
	prevIsFirst := f.prevIsFirst
	f.prevIsFirst = false
	off := edge.tRead.Sub(prev.tRead)
	if (f.pulseWidth - off).Abs() < pulseWidthAccuracy {
		var delayed []pulseEdge
		if prevIsFirst {
			delayed = []pulseEdge{prev}
		}
		return false, delayed
	}
	return true, nil
}

type secMsgState struct {
	sec               ptime.Time
	optPulseOffset    *time.Duration
	navSolnMsgTRead   time.Time
	prePulseMsgTRead  time.Time
	postPulseMsgTRead time.Time
}

type secMsgList []*secMsgState

type Sample struct {
	sec         ptime.Time
	pulseOffset time.Duration
	pulse       pulseEdge
}

type Config struct {
	EdgesPerPulse     int
	PulseWidth        time.Duration
	MaxPulseReadDelay time.Duration
	NavSolnDelay      time.Duration
	SerialDelay       time.Duration
}

type Combiner struct {
	cfg             Config
	sampler         Sampler
	edgeFilter      edgeFilter
	lg              *slog.Logger
	pulses          []pulseEdge
	secMsgList      secMsgList
	waitPulseOffset bool
	refSample       *Sample
	lastSample      *Sample
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

func NewCombiner(cfg Config, sampler Sampler, lg *slog.Logger) *Combiner {
	c := &Combiner{
		sampler: sampler,
		lg:      lg,
		cfg:     cfg,
	}
	if cfg.EdgesPerPulse == 2 {
		c.edgeFilter = &knownEdgeFilter{pulseWidth: cfg.PulseWidth}
	} else {
		c.edgeFilter = &noEdgeFilter{}
	}
	return c
}

// TimeMsg handles a message from the GPS receiver that gives information about the current time.
func (c *Combiner) TimeMsg(sec ptime.Time, tRead time.Time, pulseOff *time.Duration, ref gpsprot.TimeRef) {
	sl, i, err := c.secMsgList.addSec(sec)
	c.secMsgList = sl
	if err != nil {
		c.lg.Warn(err.Error(), "sec", sec)
		return
	}
	secState := c.secMsgList[i]
	if pulseOff != nil {
		secState.optPulseOffset = pulseOff
	}
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
	// XXX need to limit number of messages stored
	if len(c.pulses) != 0 {
		if c.refSample == nil {
			c.tryEmitFirstSample()
		} else {
			c.tryEmitNextSample()
		}
	}
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

// PulseEdge records the PHC time at which a pulse edge was received.
// tRead is the system time when we received the timestamp event from the kernel.
func (c *Combiner) PulseEdge(tClock ptime.ClockTime, tRead time.Time) {
	edge := pulseEdge{tClock, tRead}
	inc, delayed := c.edgeFilter.include(edge)
	if c.refSample == nil {
		if delayed != nil {
			c.pulses = append(c.pulses, delayed...)
		}
		if !inc {
			return
		}
		c.pulses = append(c.pulses, edge)
		// XXX limit total number of pulses kept
		c.tryEmitFirstSample()
	} else {
		if delayed != nil {
			panic("delayed edges should not occur after initialization")
		}
		if !inc {
			return
		}
		if len(c.pulses) == 0 {
			c.lg.Warn("failed to emit sample for pulse")
		}
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
	if q == matchNone {
		return
	}
	secMsg := c.secMsgList[i]
	// XXX emit delayed pulses
	sample := &Sample{
		sec:         secMsg.sec,
		pulse:       pulse,
		pulseOffset: secMsg.pulseOffset(),
	}
	c.refSample = sample
	c.lastSample = sample
	c.emit(sample, false)
	c.pulses = c.pulses[:0]
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
	sec, useAsRef := c.refSample.chooseNextSec(pulse, &c.cfg)
	if sec == 0 {
		return
	}
	pulseOff, wait := c.findPulseOffset(sec)
	if wait {
		return
	}
	c.pulses = c.pulses[:0]
	sample := Sample{
		sec:         sec,
		pulse:       pulse,
		pulseOffset: pulseOff,
	}
	c.lastSample = &sample
	if useAsRef {
		c.refSample = &sample
	}
	c.emit(&sample, false)
}

func (c *Combiner) emit(sample *Sample, delayed bool) {
	c.sampler.Sample(sample.masterTime(), sample.pulse.ClockTime, false)
	c.lastSample = sample
}

func (c *Combiner) findPulseOffset(sec ptime.Time) (pulseOff time.Duration, wait bool) {
	i := c.secMsgList.search(sec)
	var secState *secMsgState
	if i < len(c.secMsgList) && c.secMsgList[i].sec == sec {
		secState = c.secMsgList[i]
	}
	if secState != nil && secState.optPulseOffset != nil {
		pulseOff = *secState.optPulseOffset
	} else if c.waitPulseOffset && (secState == nil || secState.postPulseMsgTRead.IsZero()) {
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
		if !sl[i].postPulseMsgTRead.IsZero() {
			q = cfg.postPulseMsgMatchQuality(sl[i].postPulseMsgTRead.Sub(pt))
		} else if !sl[i].navSolnMsgTRead.IsZero() {
			q = cfg.navSolnMsgMatchQuality(sl[i].navSolnMsgTRead.Sub(pt))
		}
		if q > bestMatchQual {
			bestMatchQual = q
			bestMatchIndex = i
		}
		if q < bestMatchQual {
			// past local maximum
			break
		}
	}
	if bestMatchQual == matchNone || bestMatchIndex > 0 && qualities[bestMatchIndex-1] == bestMatchQual {
		return matchNone, -1
	}
	return bestMatchQual, bestMatchIndex
}

var errOutOfOrderMsg = errors.New("out of order message")

func (sml secMsgList) addSec(sec ptime.Time) (secMsgList, int, error) {
	i := sml.search(sec)
	for j := i; j < len(sml); j++ {
		if sml[j].happened() {
			return sml, -1, errOutOfOrderMsg
		}
	}
	newSec := secMsgState{sec: sec}
	if i == len(sml) {
		sml = append(sml, &newSec)
		return sml, i, nil
	}
	if sml[i].sec == sec {
		return sml, i, nil
	}
	before := sml[:i]
	after := sml[i:]
	sml = append(before, &newSec)
	sml = append(sml, after...)
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
func (s *Sample) chooseNextSec(pulse pulseEdge, cfg *Config) (ptime.Time, bool) {
	const (
		sourceRead = iota
		sourcePHC
		sourceMsg
		nSource
	)
	var matches [nSource]secMatch
	matches[sourcePHC] = s.phcMatch(pulse, cfg)
	matches[sourceRead] = s.readMatch(pulse, cfg)
	// XXX: need to add matches from messages
	return 0, false
}

func (s *Sample) readMatch(pulse pulseEdge, cfg *Config) secMatch {
	sec, frac := nextSec(s.sec, pulse.subTRead(s.pulse))
	return secMatch{sec, cfg.readSampleMatchQuality(frac)}
}

func (s *Sample) phcMatch(pulse pulseEdge, cfg *Config) secMatch {
	t, ok := pulse.subT(s.pulse)
	if !ok {
		return secMatch{}
	}
	sec, frac := nextSec(s.sec, t)
	return secMatch{sec, cfg.phcSampleMatchQuality(frac)}
}

func (s *Sample) masterTime() ptime.Time {
	return s.sec.Add(-s.pulseOffset)
}

func (s *secMsgState) pulseOffset() time.Duration {
	if s.optPulseOffset == nil {
		return 0
	}
	return *s.optPulseOffset
}

func (s *secMsgState) happened() bool {
	return !s.navSolnMsgTRead.IsZero() || !s.postPulseMsgTRead.IsZero()
}

func (p pulseEdge) isZero() bool {
	return p.tRead.IsZero()
}

func (p pulseEdge) subTRead(q pulseEdge) time.Duration {
	return p.tRead.Sub(q.tRead)
}

func (p pulseEdge) subT(q pulseEdge) (time.Duration, bool) {
	if p.Era != q.Era {
		return 0, false
	}
	return p.T.Sub(q.T), true
}

// delay is the tRead of the message minus the tRead of the pulse
func (cfg *Config) navSolnMsgMatchQuality(delay time.Duration) matchQuality {
	// XXX
	return matchNone
}

// delay is the tRead of the message minus the tRead of the pulse
func (cfg *Config) postPulseMsgMatchQuality(delay time.Duration) matchQuality {
	// XXX
	return matchNone
}
func (cfg *Config) readSampleMatchQuality(off time.Duration) matchQuality {
	off = off.Abs()
	if off <= time.Millisecond {
		return matchGood
	}
	if off <= cfg.MaxPulseReadDelay+time.Millisecond*10 {
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
