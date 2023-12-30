package mon

import (
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
}

type GrandmasterProps struct {
	ptime.LeapSecondState
	ClockClass    uint8
	ClockAccuracy uint8
}

func NewGrandmaster() (*Grandmaster, <-chan GrandmasterUpdateRequest) {
	updateCh := make(chan GrandmasterUpdateRequest, 1)
	gm := &Grandmaster{updateCh: updateCh}
	gm.target.SetClockInSync(false)
	return gm, updateCh
}

func (gm *Grandmaster) Close() {
	close(gm.updateCh)
}

func (gm *Grandmaster) Update(inSync bool, leap ptime.LeapSecondState) {
	gm.target.SetClockInSync(inSync)
	gm.target.LeapSecondState = leap

	gm.handleResponse()
	// Don't update if there already is a pending update
	if gm.respCh != nil {
		return
	}
	if gm.actual == nil {
		// First update
		if !inSync {
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

func (props *GrandmasterProps) SetClockInSync(inSync bool) {
	if inSync {
		props.ClockClass = pmc.ClockClassSyncPrimaryRef
		props.ClockAccuracy = pmc.ClockAccuracyWithin100ns
	} else {
		// DegradedA means using PTP timescale, not in sync, but won't be slave using default BMCA
		props.ClockClass = pmc.ClockClassDegradedA
		props.ClockAccuracy = pmc.ClockAccuracyUnknown
	}
}

func (props *GrandmasterProps) ClockInSync() bool {
	return props.ClockClass == pmc.ClockClassSyncPrimaryRef
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
