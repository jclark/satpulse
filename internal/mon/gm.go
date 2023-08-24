package mon

import (
	"time"

	"log/slog"

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
	ls       ptime.LeapSecond
	lg       *slog.Logger
	respCh   chan *GrandmasterProps
	updateCh chan<- GrandmasterUpdateRequest
	lastTime ptime.Time
	sa       *SyncAnalyzer
	inSync   bool
}

type GrandmasterProps struct {
	LeapSecondProps
	ClockClass    uint8
	ClockAccuracy uint8
}

type LeapSecondProps struct {
	UTCOffset   int16
	LeapTonight LeapSecondKind
}

type LeapSecondKind int16

const (
	LeapSecondNone     LeapSecondKind = 0
	LeapSecondPositive LeapSecondKind = 1
	LeapSecondNegative LeapSecondKind = -1
)

func LeapSecondPropsAt(ls ptime.LeapSecond, t ptime.Time) LeapSecondProps {
	var props LeapSecondProps
	if t >= ls.OffChangeTime {
		props.UTCOffset = ls.UTCOffAfter
	} else {
		props.UTCOffset = ls.UTCOffBefore
		if t >= ls.OffChangeTime.Add(-12*time.Hour) {
			props.LeapTonight = LeapSecondKind(ls.UTCOffAfter - ls.UTCOffBefore)
		}
	}
	return props
}

func NewGrandmaster(sa *SyncAnalyzer, ls ptime.LeapSecond, updateCh chan<- GrandmasterUpdateRequest, lg *slog.Logger) *Grandmaster {
	gm := &Grandmaster{sa: sa, lg: lg, ls: ls, updateCh: updateCh}
	gm.target.SetClockInSync(false)
	return gm
}

func (gm *Grandmaster) update() {
	if gm.lastTime.IsZero() {
		return
	}
	gm.target.LeapSecondProps = LeapSecondPropsAt(gm.ls, gm.lastTime)
	inSync := gm.sa.InSync(time.Now())
	if inSync != gm.inSync {
		gm.lg.Info("synchronization status has changed", "inSync", inSync)
		gm.inSync = inSync
	}
	gm.target.SetClockInSync(inSync)
	if gm.updateCh == nil {
		return
	}
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

func (gm *Grandmaster) GPSTime(t ptime.Time) {
	gm.lastTime = t
	gm.update()
}

func (gm *Grandmaster) SetLeapSecond(ls ptime.LeapSecond) {
	if ls == gm.ls {
		return
	}
	gm.ls = ls
	gm.update()
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
		TimeFlags:  pmc.CurrentUTCOffsetValid | pmc.PTPTimescale | pmc.TimeTraceable | leapFlags(props.LeapTonight),
		TimeSource: pmc.TimeSourceGNSS,
	}
}

func leapFlags(leapTonight LeapSecondKind) pmc.TimeFlags {
	switch leapTonight {
	case LeapSecondPositive:
		return pmc.Leap61
	case LeapSecondNegative:
		return pmc.Leap59
	}
	return 0
}
