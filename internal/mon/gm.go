package mon

import (
	"github.com/jclark/gps4ptp/internal/pmc"
	"github.com/jclark/gps4ptp/internal/ptime"
)

type GrandmasterUpdateRequest struct {
	props GrandmasterProps
	resp  chan<- *GrandmasterProps
}

type Grandmaster struct {
	target   GrandmasterProps
	actual   *GrandmasterProps
	respCh   chan *GrandmasterProps          // maybe nil
	updateCh chan<- GrandmasterUpdateRequest // never nil
}

type GrandmasterProps struct {
	ptime.LeapSecondState
	ClockClass    uint8
	ClockAccuracy uint8
}

func NewGrandmaster(updateCh chan<- GrandmasterUpdateRequest) *Grandmaster {
	gm := &Grandmaster{updateCh: updateCh}
	gm.target.SetClockInSync(false)
	return gm
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
	respCh := make(chan *GrandmasterProps, 1)
	gm.respCh = respCh
	gm.updateCh <- GrandmasterUpdateRequest{props: gm.target, resp: respCh}
}

func (gm *Grandmaster) handleResponse() {
	if gm.respCh == nil {
		return
	}
	select {
	case props := <-gm.respCh:
		if props != nil {
			gm.actual = props
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
