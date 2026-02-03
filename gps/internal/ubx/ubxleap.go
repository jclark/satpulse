package ubx

import (
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func leapSecond(u *ubxbin.NavTimeLS) *gpsprot.LeapSecondMsg {
	ls := gpsprot.LeapSecondMsg{}
	var date time.Time
	if !leapSecondUTCOffset(u, &ls) || !leapSecondDate(u, &date) {
		return nil
	}
	ls.LeapSecond = ptime.LeapSecondOnDate(date, ls.UTCOffBefore, ls.UTCOffAfter)
	ls.GNSS = leapSecondGNSS(u.SrcOfLSChange)
	return &ls
}

func leapSecondUTCOffset(u *ubxbin.NavTimeLS, ls *gpsprot.LeapSecondMsg) bool {
	if (u.Valid & ubxbin.NavTimeLSValidCurrLS) == 0 {
		return false
	}
	cur := int16(u.CurrLS) + ptime.TAIMinusGPS
	ls.UTCOffBefore = cur
	ls.UTCOffAfter = cur
	switch u.LSChange {
	case ubxbin.NavTimeLSChangePositive:
		ls.UTCOffAfter++
	case ubxbin.NavTimeLSChangeNegative:
		ls.UTCOffAfter--
	case ubxbin.NavTimeLSChangeNone:
		ls.UTCOffBefore-- // assume all past leap seconds are positive
	default:
		return false
	}
	return true
}

func leapSecondDate(tls *ubxbin.NavTimeLS, lsDate *time.Time) bool {
	if (tls.Valid & ubxbin.NavTimeLSValidTimeToLSEvent) == 0 {
		return false
	}
	wd := tls.DateOfLSGPSDN
	switch tls.SrcOfLSChange {
	case ubxbin.NavTimeLSSrcOfLSChangeBeiDou:
		// BeiDou DN is 0-based
		// see https://www.gpsworld.com/beidou-numbering-presents-leap-second-issue/
	case ubxbin.NavTimeLSSrcOfLSChangeGPS, ubxbin.NavTimeLSSrcOfLSChangeGalileo:
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

func leapSecondGNSS(src ubxbin.NavTimeLSSrcOfLSChange) gpsprot.GNSS {
	switch src {
	case ubxbin.NavTimeLSSrcOfLSChangeGPS:
		return gpsprot.GPS
	case ubxbin.NavTimeLSSrcOfLSChangeGalileo:
		return gpsprot.GAL
	case ubxbin.NavTimeLSSrcOfLSChangeBeiDou:
		return gpsprot.BDS
	case ubxbin.NavTimeLSSrcOfLSChangeGLONASS:
		return gpsprot.GLO
	case ubxbin.NavTimeLSSrcOfLSChangeSBAS:
		return gpsprot.SBAS
	case ubxbin.NavTimeLSSrcOfLSChangeNavIC:
		return gpsprot.NAVIC
	default:
		return 0
	}
}
