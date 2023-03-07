package ubxmsg

import (
	"time"

	"github.com/jclark/gps2phc/internal/ptime"
	"github.com/jclark/gps2phc/internal/ubx"
)

type LeapSecond struct {
	Date time.Time
}

func (m *Message) LeapSecond() *LeapSecond {
	u, ok := m.um.(*ubx.NavTimeLS)
	if !ok {
		return nil
	}
	d := leapSecondDate(u)
	if d.IsZero() {
		return nil
	}
	ls := LeapSecond{Date: d}
	return &ls
}

func leapSecondDate(tls *ubx.NavTimeLS) time.Time {
	z := time.Time{}
	if (tls.Valid & ubx.NavTimeLSValidTimeToLSEvent) == 0 {
		return z
	}
	wd := tls.DateOfLSGPSDN
	switch tls.SrcOfLSChange {
	case ubx.NavTimeLSSrcOfLSChangeBeiDou:
		// BeiDou DN is 0-based
	case ubx.NavTimeLSSrcOfLSChangeGPS, ubx.NavTimeLSSrcOfLSChangeGalileo:
		// GPS and Galileo DN is 1-based
		wd--
	default:
		// No info about meaning of DN for other cases cases
		return z
	}
	t := ptime.GPSDate(tls.DateOfLSGPSWN, time.Weekday(wd))
	if isLastDayOfQuarter(t) {
		return t
	}
	if tls.LSChange == 0 {
		// This is a past change.
		// GPS transmits only the bottom 8-bits of the week number of the leap second
		// So a past leap second can be off by a multiple of 256 weeks.
		for i := 1; i <= 2; i++ {
			t = t.AddDate(0, 0, -7*0x100)
			if isLastDayOfQuarter(t) {
				return t
			}
		}
	}
	return z
}

func isLastDayOfQuarter(t time.Time) bool {
	return t.AddDate(0, 0, 1).Day() == 1 && t.Month()%3 == 0
}
