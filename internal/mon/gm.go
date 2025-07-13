package mon

import (
	"fmt"
	"time"

	"github.com/jclark/satpulse/internal/pmc"
	"github.com/jclark/satpulse/internal/ptime"
)

type GrandmasterUpdateRequest struct {
	props GrandmasterProps
	resp  chan<- GrandmasterProps
}

type Grandmaster struct {
	target   GrandmasterProps
	actual   *GrandmasterProps
	respCh   chan GrandmasterProps           // maybe nil
	updateCh chan<- GrandmasterUpdateRequest // never nil
	clockAcc pmc.ClockAccuracy
}

type GrandmasterProps struct {
	ptime.LeapSecondState
	ClockClass    uint8
	ClockAccuracy pmc.ClockAccuracy
}

func NewGrandmaster() (*Grandmaster, <-chan GrandmasterUpdateRequest) {
	updateCh := make(chan GrandmasterUpdateRequest, 1)
	gm := &Grandmaster{updateCh: updateCh}
	gm.SetClockSync(NoSync)
	return gm, updateCh
}

func (gm *Grandmaster) SetClockAccuracy(acc time.Duration) error {
	ca := pmc.DurationToClockAccuracy(acc)
	if ca == 0 {
		return fmt.Errorf("%v: out of range clock accuracy", acc)
	}
	gm.clockAcc = ca
	return nil
}

func (gm *Grandmaster) Close() {
	close(gm.updateCh)
}

func (gm *Grandmaster) Update(state SyncState, leap ptime.LeapSecondState) {
	gm.SetClockSync(state)
	gm.target.LeapSecondState = leap

	gm.handleResponse()
	// Don't update if there already is a pending update
	if gm.respCh != nil {
		return
	}
	if gm.actual == nil {
		// First update
		if state == NoSync {
			return
		}
	} else if gm.target == *gm.actual {
		// Don't update if there is no change
		return
	}
	respCh := make(chan GrandmasterProps, 1)
	select {
	case gm.updateCh <- GrandmasterUpdateRequest{props: gm.target, resp: respCh}:
		// expect a response
		gm.respCh = respCh
	default:
		close(respCh)
	}
}

func (gm *Grandmaster) handleResponse() {
	if gm.respCh == nil {
		return
	}
	select {
	case props, ok := <-gm.respCh:
		if ok {
			gm.actual = &props
		}
		gm.respCh = nil
	default:
	}
}

func (gm *Grandmaster) SetClockSync(syncState SyncState) {
	gm.target.SetClock(gm.clockAccuracy(syncState))
}

const noSyncAccuracy = pmc.ClockAccuracyUnknown

func (gm *Grandmaster) clockAccuracy(syncState SyncState) pmc.ClockAccuracy {
	switch syncState {
	case InSync:
		return gm.clockAcc
	}
	return noSyncAccuracy
}

func (props *GrandmasterProps) SetClock(acc pmc.ClockAccuracy) {
	props.ClockAccuracy = acc
	if acc == pmc.ClockAccuracyUnknown {
		props.ClockClass = pmc.ClockClassDegradedA
	} else {
		props.ClockClass = pmc.ClockClassSyncPrimaryRef
	}
}

func (props *GrandmasterProps) Settings() pmc.GrandmasterSettings {
	return pmc.GrandmasterSettings{
		ClockQuality: pmc.ClockQuality{
			ClockClass:              props.ClockClass,
			ClockAccuracy:           props.ClockAccuracy,
			OffsetScaledLogVariance: pmc.OffsetScaledLogVarianceUnknown,
		},
		UTCOffset:  props.UTCOffset,
		TimeFlags:  pmc.CurrentUTCOffsetValid | pmc.PTPTimescale | pmc.TimeTraceable | pmcLeapFlags(props.LeapTonight),
		TimeSource: pmc.TimeSourceGNSS,
	}
}

func pmcLeapFlags(leapTonight ptime.LeapSecondKind) pmc.TimeFlags {
	switch leapTonight {
	case ptime.LeapSecondPositive:
		return pmc.Leap61
	case ptime.LeapSecondNegative:
		return pmc.Leap59
	}
	return 0
}
