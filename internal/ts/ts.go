package ts

import (
	"sync/atomic"
	"time"

	"github.com/jclark/satpulse/internal/phc"
	"github.com/jclark/satpulse/internal/ptime"
)

type Clock struct {
	phc.Clock
	eraCounter atomicEra
}

type atomicEra atomic.Uint64

type Event struct {
	Ts        ptime.ClockTime
	TRead     time.Time
	TReadPHC  ptime.ClockTime
	ChanIndex uint32
	Err       error
}

func Open(path string) (*Clock, error) {
	pc, err := phc.Open(path)
	if err != nil {
		return nil, err
	}
	clk := &Clock{Clock: *pc}
	// We start off with an era that is certain.
	// Zero era represent stale PHC clock readings.
	clk.eraCounter.inc()
	return clk, nil
}

func (clk *Clock) AdjTime(d time.Duration) (ptime.Era, error) {
	clk.eraCounter.inc()
	err := clk.Clock.AdjTime(d)
	era := clk.eraCounter.inc()
	if err != nil {
		era = 0
	}
	return era, err
}

const StaleEra ptime.Era = ptime.Era(0)

func (clk *Clock) ReadWorker(done <-chan struct{}, tsEvents chan<- Event, timeout time.Duration) {
	defer close(tsEvents)
	era := StaleEra
	for {
		select {
		case <-done:
			return
		default:
		}
		// The idea is that if we poll and there are no pending events, then any step to the clock
		// that we have made with adjtimex will be in effect for the next read.
		if !clk.ExttsAvailable(timeout) {
			era = clk.eraCounter.load()
			continue
		}
		event := Event{}
		t, err := clk.ReadExtts()
		if err != nil {
			event.Err = err
		} else {
			tClock := ptime.ClockTime{
				T:   t,
				Era: clk.eraCounter.load(),
			}
			if tClock.Era != era && !tClock.Era.Uncertain() {
				if era.Uncertain() {
					tClock.Era = era
				} else {
					// Make the era uncertain.
					// We cannot be sure that the adjtimex is in effect now.
					// We have to wait for a poll that does not return any events.
					tClock.Era = era + 1
				}
			}
			event.TReadPHC, event.TRead, event.Err = clk.sample()
			event.Ts = tClock
		}
		tsEvents <- event
	}
}

// sample reads the PHC and system clocks and returns the results.
// This can be called only from ReadWorker.
func (clk *Clock) sample() (tClock ptime.ClockTime, tSys time.Time, err error) {
	eraPre := clk.eraCounter.load()
	ms, err := clk.SysOffsetExtended(6)
	if err != nil {
		return
	}
	eraPost := clk.eraCounter.load()
	if eraPre == eraPost || eraPre.Uncertain() {
		tClock.Era = eraPre
	} else if eraPost.Uncertain() {
		tClock.Era = eraPost
	} else {
		tClock.Era = eraPre + 1
	}
	tClock.T, tSys = ms.Reduce()
	return
}

func (c *atomicEra) inc() ptime.Era {
	return c.add(1)
}

func (c *atomicEra) add(n uint64) ptime.Era {
	return ptime.Era((*atomic.Uint64)(c).Add(n))
}

func (c *atomicEra) load() ptime.Era {
	return ptime.Era((*atomic.Uint64)(c).Load())
}
