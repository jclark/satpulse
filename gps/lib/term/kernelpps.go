package term

import (
	"time"

	"github.com/jclark/satpulse/gps/lib/kpps"
)

// kernelPPSSeq turns the independently sequenced assert and clear edges in a
// kpps.Info into pin changes. A fetch reports the most recent edge of each
// polarity, but a Wait returns one change, so only the newest edge is
// reported; anything else the fetch found is counted as missed, along with
// the edges whose timestamps the latches had already overwritten.
type kernelPPSSeq struct {
	lastAssert uint32
	lastClear  uint32
}

func (s *kernelPPSSeq) update(info kpps.Info, tRead time.Time) (ModemControlPinChange, int, bool) {
	// The counters only ever advance, so the unsigned difference is the number
	// of captures since the last fetch however the 32-bit counter wraps.
	assertDelta := info.Assert.Sequence - s.lastAssert
	clearDelta := info.Clear.Sequence - s.lastClear
	s.lastAssert = info.Assert.Sequence
	s.lastClear = info.Clear.Sequence
	if assertDelta == 0 && clearDelta == 0 {
		return ModemControlPinChange{}, 0, false
	}
	// One edge is reported and the rest are missed: the ones whose timestamps
	// the latches overwrote, and, when both polarities are new, the older of
	// the two, since a wait reports one change.
	missed := int(assertDelta) + int(clearDelta) - 1
	assert := ModemControlPinChange{Timestamp: info.Assert.T, TRead: tRead, Asserted: true}
	clear := ModemControlPinChange{Timestamp: info.Clear.T, TRead: tRead, Asserted: false}
	if clearDelta == 0 {
		return assert, missed, true // only the assert is new
	}
	if assertDelta == 0 {
		return clear, missed, true // only the clear is new
	}
	if info.Clear.T.After(info.Assert.T) {
		return clear, missed, true // both are new; the clear is the later one
	}
	return assert, missed, true // both are new; the assert is the later one
}
