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

func (pe pulseEdge) isZero() bool {
	return pe.tRead.IsZero()
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
	pulseOff          *time.Duration
	navSolnMsgTRead   time.Time
	nextPulseMsgTRead time.Time
	prevPulseMsgTRead time.Time
}

type secList []*secMsgState

type Sample struct {
	sec         ptime.Time
	secState    *secMsgState // can be nil if we don't have a message for this second
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
	secList         secList
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

func (c *Combiner) TimeMsg(sec ptime.Time, tRead time.Time, pulseOff *time.Duration, ref gpsprot.TimeRef) {
	if !c.addTimeMsg(sec, tRead, pulseOff, ref) {
		return
	}
	if len(c.pulses) != 0 {
		if c.refSample == nil {
			c.tryEmitFirstSample()
		} else {
			c.tryEmitNextSample()
		}
	}
}

func (c *Combiner) addTimeMsg(sec ptime.Time, tRead time.Time, pulseOff *time.Duration, ref gpsprot.TimeRef) bool {
	sl, i, err := c.secList.addSec(sec)
	c.secList = sl
	if err != nil {
		c.lg.Warn(err.Error(), "sec", sec)
		return false
	}
	secState := c.secList[i]
	switch ref {
	case gpsprot.NavSolution:
		secState.navSolnMsgTRead = tRead
	case gpsprot.NextPulse:
		secState.nextPulseMsgTRead = tRead
	case gpsprot.LastPulse:
		secState.prevPulseMsgTRead = tRead
	}
	if pulseOff != nil {
		secState.pulseOff = pulseOff
	}
	return true
}

func (sl secList) bestMatch(pulse pulseEdge, cfg *Config) secMatch {
	pt := pulse.tRead
	// this is our estimate of the system time at which this pulse occurred
	pt = pt.Add(-cfg.MaxPulseReadDelay / 2)
	//var lastOff *time.Duration
	for i := len(sl) - 1; i >= 0; i-- {
		secState := sl[i]
		mt := secState.prevPulseMsgTRead
		if mt.IsZero() {
			mt = secState.navSolnMsgTRead
			if mt.IsZero() {
				//lastOff = nil
				continue
			}
			mt = mt.Add(-cfg.NavSolnDelay)
		}
		mt = mt.Add(-cfg.SerialDelay)
		// mt decreases in this loop, so off increases
		off := pt.Sub(mt)
		if off > 0 {
			// we've gone past the pulse
			break
		}
		//lastOff = &off
	}
	return secMatch{q: matchNone}
}

var errOutOfOrderMsg = errors.New("out of order message")

func (sl secList) addSec(sec ptime.Time) (secStates secList, i int, err error) {
	secStates = sl
	i = secStates.search(sec)
	for j := i; j < len(secStates); j++ {
		if secStates[j].happened() {
			i = -1
			err = errOutOfOrderMsg
			return
		}
	}
	newSec := secMsgState{sec: sec}
	if i == len(secStates) {
		secStates = append(secStates, &newSec)
		return
	}
	if secStates[i].sec == sec {
		return
	}
	before := secStates[:i]
	after := secStates[i:]
	secStates = append(before, &newSec)
	secStates = append(secStates, after...)
	return
}

// search returns the greatest index i such that for all j < i, sl[j].sec < sec
// The return value is >= 0 and <= len(sl).
// Assumes that secStates is in ascending order of sec with no duplicates.
// Note that when sec is greater than all sl[j].sec, then i == len(sl).
// The implementation searchs from the end of the slice, since this will be the common case.
func (sl secList) search(sec ptime.Time) int {
	i := len(sl)
	for i > 0 && sl[i-1].sec >= sec {
		i--
	}
	return i
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

// tryEmitFirstSample attempts to emit a sample for the current pulse in the case when we have not emitted any samples yet.
// If latest pulse is for second N, possibilities for last secMsgState (when everything is OK) are:
//  1. it has a NextPulse message for second N + 1, and no other messages; this is usually the case just after the NextPulse message
//     was received;
//  2. it has a NavSolution or LastPulse message for second N; this will usually be the case just after the message was received
//  3. it has a NavSolution or LastPulse message for second N - 1; this will usually be the case just after the pulse was received
//  4. it has a NavSolution or LastPulse message for second N + 1; this is unusual, but can happen just after the message was
//     was received, if the pulse was delayed (i.e. on the CM4)
func (c *Combiner) tryEmitFirstSample() {

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
	sec := c.cfg.chooseNextSec(c.refSample, pulse)
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
	c.refSample = &sample
	c.emit(&sample, false)
}

func (s *Sample) masterTime() ptime.Time {
	return s.sec.Add(-s.pulseOffset)
}

func (c *Combiner) emit(sample *Sample, delayed bool) {
	c.sampler.Sample(sample.masterTime(), sample.pulse.ClockTime, false)
	c.lastSample = sample
}

func (c *Combiner) findPulseOffset(sec ptime.Time) (pulseOff time.Duration, wait bool) {
	i := c.secList.search(sec)
	var secState *secMsgState
	if i < len(c.secList) && c.secList[i].sec == sec {
		secState = c.secList[i]
	}
	if secState != nil && secState.pulseOff != nil {
		pulseOff = *secState.pulseOff
	} else if c.waitPulseOffset && (secState == nil || secState.prevPulseMsgTRead.IsZero()) {
		wait = true
	}
	return
}

func (c *Config) chooseNextSec(refSample *Sample, curPulse pulseEdge) ptime.Time {
	const (
		sourceRead = iota
		sourcePHC
		sourceMsg
		nSource
	)
	var matches [nSource]secMatch
	matches[sourcePHC] = refSample.phcMatch(curPulse, c)
	matches[sourceRead] = refSample.readMatch(curPulse, c)
	// XXX: need to add matches from messages
	return 0
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

func (s *secMsgState) happened() bool {
	return !s.navSolnMsgTRead.IsZero() || !s.prevPulseMsgTRead.IsZero()
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
	return matchNone
}

func (cfg *Config) readSampleMatchQuality(off time.Duration) matchQuality {
	off = off.Abs()
	if off <= time.Millisecond {
		return matchGood
	}
	if off <= cfg.MaxPulseReadDelay+time.Millisecond {
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
