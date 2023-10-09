package ubx

import (
	"time"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ptime"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

func leapSecond(u *bin.NavTimeLS) *gpsmsg.LeapSecond {
	ls := gpsmsg.LeapSecond{NavEpoch: iTOWEpoch(u.ITOW)}
	var date time.Time
	if !leapSecondUTCOffset(u, &ls) || !leapSecondDate(u, &date) {
		return nil
	}
	ls.LeapSecond = ptime.LeapSecondOnDate(date, ls.UTCOffBefore, ls.UTCOffAfter)
	return &ls
}

func leapSecondUTCOffset(u *bin.NavTimeLS, ls *gpsmsg.LeapSecond) bool {
	if (u.Valid & bin.NavTimeLSValidCurrLS) == 0 {
		return false
	}
	cur := int16(u.CurrLS) + ptime.TAIMinusGPS
	ls.UTCOffBefore = cur
	ls.UTCOffAfter = cur
	switch u.LSChange {
	case bin.NavTimeLSChangePositive:
		ls.UTCOffAfter++
	case bin.NavTimeLSChangeNegative:
		ls.UTCOffAfter--
	case bin.NavTimeLSChangeNone:
		ls.UTCOffBefore-- // assume all past leap seconds are positive
	default:
		return false
	}
	return true
}

func leapSecondDate(tls *bin.NavTimeLS, lsDate *time.Time) bool {
	if (tls.Valid & bin.NavTimeLSValidTimeToLSEvent) == 0 {
		return false
	}
	wd := tls.DateOfLSGPSDN
	switch tls.SrcOfLSChange {
	case bin.NavTimeLSSrcOfLSChangeBeiDou:
		// BeiDou DN is 0-based
		// see https://www.gpsworld.com/beidou-numbering-presents-leap-second-issue/
	case bin.NavTimeLSSrcOfLSChangeGPS, bin.NavTimeLSSrcOfLSChangeGalileo:
		// GPS and Galileo DN is 1-based
		wd--
	default:
		// No info about meaning of DN for other cases cases
		return false
	}
	// XXX Does this use GPS week numbers even if the source is Galileo or BeiDou?
	t := ptime.GPSDate(tls.DateOfLSGPSWN, time.Weekday(wd))
	if isLastDayOfQuarter(t) {
		*lsDate = t
		return true
	}
	if tls.LSChange == 0 {
		// This is a past change.
		// GPS, Galileo and BeiDou transmi only the bottom 8-bits of the week number of the leap second
		// So a past leap second can be off by a multiple of 256 weeks.
		// I've seen this for GPS.
		for i := 1; i <= 2; i++ {
			t = t.AddDate(0, 0, -7*0x100)
			if isLastDayOfQuarter(t) {
				*lsDate = t
				return true
			}
		}
	}
	return false
}

func isLastDayOfQuarter(t time.Time) bool {
	return t.AddDate(0, 0, 1).Day() == 1 && t.Month()%3 == 0
}
