package as

import (
	"math"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
	"github.com/jclark/satpulse/gps/ptime"
)

// navTowGrid is what the millisecond time of week in NAV-TIME and NAV-TIMEUTC
// is rounded to. This looks wrong, but the receivers do not follow the protocol
// specification here, and different models behave differently. The TAU1201
// rounds the time of week down to the millisecond, so a solution just before
// the second has a time of week ending in 999, whereas the TAU951M-P200 rounds
// it to the nearest millisecond. The sub-millisecond fields, NAV-TIME Fractow
// and NAV-TIMEUTC nano, do not mean what the specification says and are encoded
// differently by different models, so they are ignored.
//
// We need only the nominal time, which is a multiple of the navigation period.
// 10ms is well below the navigation period (the navigation rate is at most
// 10Hz), so rounding to it recovers the nominal time without knowing which
// receiver it is.
const navTowGrid = 10 * time.Millisecond

// timeNavTimeUTC converts asbin.NavTimeUTC to gpsprot.TimeMsg.
// Always returns a TimeMsg, but with nil UTCTime when the time is invalid.
func timeNavTimeUTC(m *asbin.NavTimeUTC) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "NAV-TIMEUTC"}
	const fullyValid = asbin.NavTimeUTCFlagTowValid | asbin.NavTimeUTCFlagWknValid | asbin.NavTimeUTCFlagUtcValid
	if m.ValidFlag&fullyValid != fullyValid {
		return &t
	}
	// hour:min:sec has no fraction of a second, so take the fraction from iTow
	// (see navTowGrid); at 5Hz, this is what distinguishes the solutions within
	// a second. hour:min:sec is the second of the receiver's time rounded to the
	// millisecond, so when rounding iTow carries into the next second,
	// hour:min:sec already includes the carry, provided the solution is less
	// than 0.5ms before the second, as in every capture so far. If it were more,
	// this time would be a second early.
	frac := (time.Duration(m.ITow) * time.Millisecond).Round(navTowGrid) % time.Second
	t.UTCTime.Set(ptime.UTC(m.Year, m.Month, m.Day, m.Hour, m.Min, m.Sec, int32(frac)))
	t.Accuracy = time.Duration(m.TAcc) * time.Nanosecond
	t.GNSS = utcStandardToGNSS(m.ValidFlag.UTCStandard())
	return &t
}

// timeNavTime converts asbin.NavTime to gpsprot.TimeMsg.
// Always returns a TimeMsg, but with zero TAITime when the time is invalid.
func timeNavTime(m *asbin.NavTime) *gpsprot.TimeMsg {
	t := gpsprot.TimeMsg{NativeMsgID: "NAV-TIME"}
	if m.Flags&(asbin.NavTimeFlagWeekValid|asbin.NavTimeFlagSecondValid) !=
		asbin.NavTimeFlagWeekValid|asbin.NavTimeFlagSecondValid {
		return &t
	}
	// Fractow is deliberately ignored; see navTowGrid.
	tow := (time.Duration(m.RefTow) * time.Millisecond).Round(navTowGrid)
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
		t.TAITime = ptime.GPS(int16(ptime.ExtendGPSWeek(m.Week)), tow)
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

func utcStandardToGNSS(std asbin.UTCStandard) gpsprot.GNSS {
	switch std {
	case asbin.UTCStandardNTSC:
		return gpsprot.BDS
	case asbin.UTCStandardUSNO:
		return gpsprot.GPS
	case asbin.UTCStandardEU:
		return gpsprot.GAL
	case asbin.UTCStandardSU:
		return gpsprot.GLO
	default:
		return 0
	}
}
