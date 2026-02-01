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
	taiMinusGNSS := ptime.TAIMinusGPS
	// Treat NavSys fields other than Galileo and BeiDou as GPS.
	// Testing with TAU1201 firmware 3.018 shows that periodic
	// NAV-TIME messages report NavSys as 24 (0x18) instead of the documented 0/1/2/3,
	// though polled responses return the correct value (0 for GPS).
	// I have not found any circumstances in which the NavSys is 1, 2 or 3.
	// But if it is 1 or 2, then the most plausible interpretation of week/tow/leapsec
	// fields is that they are in the indicated GNSS time system.
	// GLONASS doesn't natively use week/tow/leapsec, so when NavSys is 3, it is most
	// plausible that the week/tow/leapsec would be in GPS time.
	// these use GPS time when NavSys is 3.
	switch m.NavSys {
	case asbin.NavTimeSysGalileo:
		taiMinusGNSS = ptime.TAIMinusGalileo
		t.TAITime = ptime.Galileo(week, tow)
		t.GNSS = gpsprot.GAL
	case asbin.NavTimeSysBeiDou:
		taiMinusGNSS = ptime.TAIMinusBeiDou
		t.TAITime = ptime.BeiDou(week, tow)
		t.GNSS = gpsprot.BDS
	default:
		t.TAITime = ptime.GPS(week, tow)
		t.GNSS = gpsprot.GPS
	}
	if (m.Flags & asbin.NavTimeFlagLeapSecValid) != 0 {
		off := int(m.LeapSec) + taiMinusGNSS
		if off >= 0 && off <= math.MaxUint8 {
			t.UTCOffset = uint8(off)
		}
	}
	t.Accuracy = time.Duration(m.TimeErr) * time.Nanosecond
	return &t
}
