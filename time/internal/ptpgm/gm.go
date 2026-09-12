package ptpgm

import (
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/lib/pmc"
)

const updateRetryInterval = 30 * time.Second

type Config struct {
	InSyncClockQuality pmc.ClockQuality
}

type GrandmasterUpdateRequest struct {
	props GrandmasterProps
	resp  chan<- GrandmasterProps
}

type Grandmaster struct {
	target               GrandmasterProps
	lastAttempt          *GrandmasterProps
	lastAttemptConfirmed bool
	respCh               chan GrandmasterProps           // maybe nil
	updateCh             chan<- GrandmasterUpdateRequest // never nil
	clockQual            pmc.ClockQuality
	nextRetry            time.Time
}

type GrandmasterProps struct {
	ptime.LeapSecondState
	pmc.ClockQuality
	TimeFlags pmc.TimeFlags
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

// Close sends the current target after any older update and closes the worker channel.
func (gm *Grandmaster) Close() {
	if gm.respCh != nil && gm.lastAttempt != nil && gm.target == *gm.lastAttempt {
		close(gm.updateCh)
		return
	}
	if gm.respCh != nil {
		gm.waitResponse()
	}
	if gm.needsUpdate() {
		gm.send()
	}
	close(gm.updateCh)
}

// Update sets the target properties and sends them unless a request is in
// flight or the target matches the last attempt. A NoSync target is not sent
// before the first InSync one. A target that is not sent now is sent by a
// later Update or Retry.
func (gm *Grandmaster) Update(state SyncState, leap ptime.LeapSecondState) {
	gm.target.LeapSecondState = leap
	gm.SetClockSync(state)
	gm.handleResponse()
	if gm.respCh != nil {
		return
	}
	if gm.lastAttempt == nil {
		if state == NoSync {
			return
		}
	} else if gm.target == *gm.lastAttempt {
		return
	}
	gm.send()
}

// RetryDue reports whether it is time to retry updating the grandmaster.
func (gm *Grandmaster) RetryDue(now time.Time) bool {
	gm.handleResponse()
	if gm.respCh != nil || !gm.needsUpdate() {
		return false
	}
	if gm.target != *gm.lastAttempt {
		return true
	}
	if gm.nextRetry.IsZero() {
		gm.nextRetry = now.Add(updateRetryInterval)
		return false
	}
	if now.Before(gm.nextRetry) {
		return false
	}
	return true
}

// Retry sends the current target after RetryDue reports that it is due.
func (gm *Grandmaster) Retry() {
	if gm.respCh == nil && gm.needsUpdate() {
		gm.send()
	}
}

func (gm *Grandmaster) needsUpdate() bool {
	return gm.lastAttempt != nil &&
		(gm.target != *gm.lastAttempt || !gm.lastAttemptConfirmed)
}

func (gm *Grandmaster) send() {
	respCh := make(chan GrandmasterProps, 1)
	props := gm.target
	select {
	case gm.updateCh <- GrandmasterUpdateRequest{props: props, resp: respCh}:
	default:
		// callers only send once the previous response has been collected
		panic("grandmaster update channel full")
	}
	gm.lastAttempt = &props
	gm.lastAttemptConfirmed = false
	gm.respCh = respCh
	gm.nextRetry = time.Time{}
}

func (gm *Grandmaster) waitResponse() {
	_, gm.lastAttemptConfirmed = <-gm.respCh
	gm.respCh = nil
}

func (gm *Grandmaster) handleResponse() {
	if gm.respCh == nil {
		return
	}
	select {
	case _, ok := <-gm.respCh:
		gm.lastAttemptConfirmed = ok
		gm.respCh = nil
	default:
	}
}

// SetClockSync sets the target clock quality and time flags for syncState,
// keeping the target's leap second state.
func (gm *Grandmaster) SetClockSync(syncState SyncState) {
	gm.target.SetClock(gm.clockQuality(syncState))
	gm.target.TimeFlags = pmc.CurrentUTCOffsetValid | pmc.PTPTimescale | pmcLeapFlags(gm.target.LeapTonight)
	if syncState == InSync {
		gm.target.TimeFlags |= pmc.TimeTraceable | pmc.FrequencyTraceable
	}
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
		TimeFlags:    props.TimeFlags,
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
