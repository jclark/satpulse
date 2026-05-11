package obs

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/phcsync"
)

// Observer provides unified observability interface
type Observer interface {
	phcsync.Observer
	gpsprot.MsgHandler

	// Tick delivers a filled TimeMsg (TAI+UTC populated, rounded to ms)
	// once per epoch, as soon as the first valid TimeMsg arrives.
	Tick(msg *gpsprot.TimeMsg, tRead time.Time)

	// NavEpochPV delivers the accumulated PVMsgBundle at each navigation epoch.
	// Relative order with respect to NavEpoch is undefined;
	// observers should use one or the other, not both.
	NavEpochPV(msg *gpsprot.NavEpochMsg, pv *gpsprot.PVMsgBundle, tRead time.Time)

	// NTPSample reports a sample sent to the refclock (chrony SOCK).
	// sys is the CLOCK_REALTIME reading paired with the sample; offset
	// is (true time - sys) in seconds; leap is the leap-second state
	// passed to the refclock; phc is the PHC timestamp the offset was
	// computed at, or the zero value when no PHC is in play (serial
	// timing mode).
	NTPSample(sys time.Time, offset float64, leap ptime.LeapSecondKind, phc ptime.Time)

	// NativeMsg reports a parsed protocol-specific message that did not map
	// to a protocol-independent gpsprot.MsgHandler message. It returns true
	// if the observer recognized or used the message.
	NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) bool

	// ReopenLog handles log rotation (e.g., on SIGHUP signal)
	ReopenLog()

	// Release releases any resources used by the observer
	Release()
}

// MultiObserver fans out calls to multiple observers
type MultiObserver struct {
	*gpsprot.MultiHandler
}

// NewMultiObserver creates a new MultiObserver that fans out to multiple observers
func NewMultiObserver(observers ...Observer) *MultiObserver {
	handlers := make([]gpsprot.MsgHandler, len(observers))
	for i, obs := range observers {
		handlers[i] = obs
	}

	return &MultiObserver{
		MultiHandler: gpsprot.NewMultiHandler(handlers...),
	}
}

// Sample implements phcsync.Observer by type-asserting handlers to Observer
func (m *MultiObserver) Sample(data phcsync.Sample) {
	for h := range m.Handlers() {
		if o, ok := h.(phcsync.Observer); ok {
			o.Sample(data)
		}
	}
}

// ModeChanged implements phcsync.Observer by type-asserting handlers to Observer
func (m *MultiObserver) ModeChanged(oldMode, newMode phcsync.Mode) {
	for h := range m.Handlers() {
		if o, ok := h.(phcsync.Observer); ok {
			o.ModeChanged(oldMode, newMode)
		}
	}
}

// Tick implements Observer by type-asserting handlers to Observer
func (m *MultiObserver) Tick(msg *gpsprot.TimeMsg, tRead time.Time) {
	for h := range m.Handlers() {
		if obs, ok := h.(Observer); ok {
			obs.Tick(msg, tRead)
		}
	}
}

// NavEpochPV implements Observer by type-asserting handlers to Observer
func (m *MultiObserver) NavEpochPV(msg *gpsprot.NavEpochMsg, pv *gpsprot.PVMsgBundle, tRead time.Time) {
	for h := range m.Handlers() {
		if obs, ok := h.(Observer); ok {
			obs.NavEpochPV(msg, pv, tRead)
		}
	}
}

// NTPSample implements Observer by type-asserting handlers to Observer
func (m *MultiObserver) NTPSample(sys time.Time, offset float64, leap ptime.LeapSecondKind, phc ptime.Time) {
	for h := range m.Handlers() {
		if obs, ok := h.(Observer); ok {
			obs.NTPSample(sys, offset, leap, phc)
		}
	}
}

// NativeMsg implements Observer by type-asserting handlers to Observer
func (m *MultiObserver) NativeMsg(tag gpsprot.Tag, msgID string, msg any, tRead time.Time) bool {
	handled := false
	for h := range m.Handlers() {
		if obs, ok := h.(Observer); ok {
			handled = obs.NativeMsg(tag, msgID, msg, tRead) || handled
		}
	}
	return handled
}

// ReopenLog implements Observer by type-asserting handlers to Observer
func (m *MultiObserver) ReopenLog() {
	for h := range m.Handlers() {
		if obs, ok := h.(Observer); ok {
			obs.ReopenLog()
		}
	}
}

// Release implements Observer by type-asserting handlers to Observer
func (m *MultiObserver) Release() {
	for h := range m.Handlers() {
		if obs, ok := h.(Observer); ok {
			obs.Release()
		}
	}
}

// DefaultObserver is a no-op implementation of Observer
type DefaultObserver struct {
	gpsprot.DefaultHandler
}

// Sample implements phcsync.Observer as a no-op
func (o *DefaultObserver) Sample(data phcsync.Sample) {}

// ModeChanged implements phcsync.Observer as a no-op
func (o *DefaultObserver) ModeChanged(_, _ phcsync.Mode) {}

// Tick implements Observer as a no-op
func (o *DefaultObserver) Tick(_ *gpsprot.TimeMsg, _ time.Time) {}

// NavEpochPV implements Observer as a no-op
func (o *DefaultObserver) NavEpochPV(_ *gpsprot.NavEpochMsg, _ *gpsprot.PVMsgBundle, _ time.Time) {}

// NTPSample implements Observer as a no-op
func (o *DefaultObserver) NTPSample(_ time.Time, _ float64, _ ptime.LeapSecondKind, _ ptime.Time) {}

// NativeMsg implements Observer as a no-op
func (o *DefaultObserver) NativeMsg(_ gpsprot.Tag, _ string, _ any, _ time.Time) bool { return false }

// ReopenLog implements Observer as a no-op
func (o *DefaultObserver) ReopenLog() {}

// Release implements Observer as a no-op
func (o *DefaultObserver) Release() {}
