package ptpgm

import (
	"github.com/jclark/satpulse/internal/pmc"
	"github.com/jclark/satpulse/internal/ptime"
)

type Config struct {
	InSyncClockQuality pmc.ClockQuality
}

type GrandmasterUpdateRequest struct {
	props GrandmasterProps
	resp  chan<- GrandmasterProps
}

type Grandmaster struct {
	target    GrandmasterProps
	actual    *GrandmasterProps
	respCh    chan GrandmasterProps           // maybe nil
	updateCh  chan<- GrandmasterUpdateRequest // never nil
	clockQual pmc.ClockQuality
}

type GrandmasterProps struct {
	ptime.LeapSecondState
	pmc.ClockQuality
}

// SyncState represents the current synchronization status
type SyncState int

const (
	NoSync SyncState = iota // Clock is not synchronized
	InSync                  // Clock is synchronized
)

// String returns a human-readable representation of the sync state
func (s SyncState) String() string {
	switch s {
	case NoSync:
		return "out of sync"
	case InSync:
		return "in sync"
	default:
		return "unknown"
	}
}

// NewGrandmaster creates a new Grandmaster with the given configuration.
func NewGrandmaster(cfg Config) (*Grandmaster, <-chan GrandmasterUpdateRequest) {
	updateCh := make(chan GrandmasterUpdateRequest, 1)
	gm := &Grandmaster{
		updateCh:  updateCh,
		clockQual: cfg.InSyncClockQuality,
	}
	gm.SetClockSync(NoSync)
	return gm, updateCh
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
	gm.target.SetClock(gm.clockQuality(syncState))
}

var noSyncClockQuality = pmc.ClockQuality{
	ClockClass:              pmc.ClockClassDegradedA,
	ClockAccuracy:           pmc.ClockAccuracyUnknown,
	OffsetScaledLogVariance: pmc.OffsetScaledLogVarianceUnknown,
}

func (gm *Grandmaster) clockQuality(syncState SyncState) pmc.ClockQuality {
	if syncState == InSync {
		return gm.clockQual
	}
	return noSyncClockQuality
}

func (props *GrandmasterProps) SetClock(cq pmc.ClockQuality) {
	props.ClockQuality = cq
}

func (props *GrandmasterProps) Settings() pmc.GrandmasterSettings {
	return pmc.GrandmasterSettings{
		ClockQuality: props.ClockQuality,
		UTCOffset:    props.UTCOffset,
		TimeFlags:    pmc.CurrentUTCOffsetValid | pmc.PTPTimescale | pmc.TimeTraceable | pmcLeapFlags(props.LeapTonight),
		TimeSource:   pmc.TimeSourceGNSS,
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
