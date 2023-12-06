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
	sec                 ptime.Time
	pulseOff            *time.Duration
	navSolutionMsgTRead time.Time
	nextPulseMsgTRead   time.Time
	prevPulseMsgTRead   time.Time
}

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
}

type Combiner struct {
	cfg             Config
	sampler         Sampler
	edgeFilter      edgeFilter
	lg              *slog.Logger
	pulses          []pulseEdge
	secStates       []*secMsgState
	waitPulseOffset bool
	refSample       *Sample
	lastSample      *Sample
}

type estimateQuality int

const (
	estimateNone estimateQuality = iota
	estimateBad
	estimateMedium
	estimateGood
	estimateGreat
)

type secEstimate struct {
	sec ptime.Time
	q   estimateQuality
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
	i, err := addSecMsgState(&c.secStates, sec)
	if err != nil {
		c.lg.Warn(err.Error(), "sec", sec)
		return false
	}
	secState := c.secStates[i]
	switch ref {
	case gpsprot.NavSolution:
		secState.navSolutionMsgTRead = tRead
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

var errOutOfOrderMsg = errors.New("out of order message")

func addSecMsgState(ssmp *[]*secMsgState, sec ptime.Time) (i int, err error) {
	secStates := *ssmp
	i = search(secStates, sec)
	for j := i; j < len(secStates); j++ {
		if secStates[j].happened() {
			i = -1
			err = errOutOfOrderMsg
			return
		}
	}
	newSec := secMsgState{sec: sec}
	if i == len(secStates) {
		*ssmp = append(secStates, &newSec)
		return
	}
	if secStates[i].sec == sec {
		return
	}
	before := secStates[:i]
	after := secStates[i:]
	secStates = append(before, &newSec)
	*ssmp = append(secStates, after...)
	return
}

// search returns the greatest index i such that for all j < i, secStates[j].sec < sec
// The return value is >= 0 and <= len(secStates).
// Assumes that secStates is in ascending order of sec with no duplicates.
// Note that when sec is greater than all secStates[j].sec, then i == len(secStates).
// The implementation searchs from the end of the slice, since this will be the common case.
func search(secStates []*secMsgState, sec ptime.Time) int {
	i := len(secStates)
	for i > 0 && secStates[i-1].sec >= sec {
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
	// XXX emit

}

func (c *Combiner) findPulseOffset(sec ptime.Time) (pulseOff time.Duration, wait bool) {
	i := search(c.secStates, sec)
	var secState *secMsgState
	if i < len(c.secStates) && c.secStates[i].sec == sec {
		secState = c.secStates[i]
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
	var estimates [nSource]secEstimate
	estimates[sourcePHC] = refSample.phcEstimate(curPulse, c)
	estimates[sourceRead] = refSample.readEstimate(curPulse, c)
	return 0
}

func (s *Sample) readEstimate(pulse pulseEdge, cfg *Config) secEstimate {
	sec, frac := nextSec(s.sec, pulse.subTRead(s.pulse))
	return secEstimate{sec, readEstimateQuality(frac, cfg)}
}

func (s *Sample) phcEstimate(pulse pulseEdge, cfg *Config) secEstimate {
	t, ok := pulse.subT(s.pulse)
	if !ok {
		return secEstimate{}
	}
	sec, frac := nextSec(s.sec, t)
	return secEstimate{sec, phcEstimateQuality(frac, cfg)}
}

func (s *secMsgState) happened() bool {
	return !s.navSolutionMsgTRead.IsZero() || !s.prevPulseMsgTRead.IsZero()
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

func readEstimateQuality(off time.Duration, cfg *Config) estimateQuality {
	off = off.Abs()
	if off <= time.Millisecond {
		return estimateGood
	}
	if off <= cfg.MaxPulseReadDelay+time.Millisecond {
		return estimateMedium
	}
	return estimateBad
}

func phcEstimateQuality(off time.Duration, _ *Config) estimateQuality {
	off = off.Abs()
	if off < time.Microsecond {
		return estimateGreat
	}
	if off < 100*time.Millisecond {
		return estimateGood
	}
	return estimateBad
}

func nextSec(ref ptime.Time, d time.Duration) (sec ptime.Time, frac time.Duration) {
	whole := d.Round(time.Second)
	frac = d - whole
	sec = ref.Add(whole)
	return
}
