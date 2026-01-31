package as

import (
	"math"
	"time"

	"github.com/jclark/satpulse/internal/asbin"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/ptime"
)

// timeNavTime converts asbin.NavTime to gpsprot.TimeMsg.
// Always returns a TimeMsg, but with zero TAITime when the time is invalid.
func timeNavTime(m *asbin.NavTime) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "NAV-TIME"}
	if m.Flags&(asbin.NavTimeFlagWeekValid|asbin.NavTimeFlagSecondValid) !=
		asbin.NavTimeFlagWeekValid|asbin.NavTimeFlagSecondValid {
		return &t
	}
	// Convert time of week: RefTow is in ms, Fractow is in ns
	tow := time.Duration(m.RefTow)*time.Millisecond + time.Duration(m.Fractow)*time.Nanosecond
	if m.Week > math.MaxInt16 {
		return &t
	}
	week := int16(m.Week)
	// Ignore NavSys field: testing with TAU1201 firmware 3.018 shows it always
	// reports GPS time (week and LeapSec) regardless of which constellation is
	// enabled, and NavSys is always 24 (0x18) instead of the documented 0/1/2/3.
	t.TAITime = ptime.GPS(week, tow)
	t.GNSS = gpsprot.GPS
	if (m.Flags & asbin.NavTimeFlagLeapSecValid) != 0 {
		off := int32(m.LeapSec) + ptime.TAIMinusGPS
		if off >= 0 && off <= math.MaxUint8 {
			t.UTCOffset = uint8(off)
		}
	}
	t.Accuracy = time.Duration(m.TimeErr) * time.Nanosecond
	return &t
}
