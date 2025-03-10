package ptime

import (
	"fmt"
	"testing"
	"time"
)

func TestLeapSecs(t *testing.T) {
	leaps := LeapSecondOnDate(time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 38)

	nanoCases := []int32{0, 1, 1e9 / 2, 1e9 - 1, -1, 1 - 1e9, -1e9 / 2}
	// test positive leap seconds
	for _, nanos := range nanoCases {
		secs := []UTCTime{
			UTC(2025, 6, 30, 23, 59, 57, nanos),
			UTC(2025, 6, 30, 23, 59, 58, nanos),
			UTC(2025, 6, 30, 23, 59, 59, nanos),
			UTC(2025, 6, 30, 23, 59, 60, nanos),
			UTC(2025, 7, 1, 0, 0, 0, nanos),
			UTC(2025, 7, 1, 0, 0, 1, nanos),
		}
		checkConsecutive(t, &leaps, secs, nanos)
	}
	// test negative leap seconds
	leaps.UTCOffAfter -= 2
	for _, nanos := range nanoCases {
		secs := []UTCTime{
			UTC(2025, 6, 30, 23, 59, 57, nanos),
			UTC(2025, 6, 30, 23, 59, 58, nanos),
			UTC(2025, 7, 1, 0, 0, 0, nanos),
			UTC(2025, 7, 1, 0, 0, 1, nanos),
		}
		checkConsecutive(t, &leaps, secs, nanos)
	}
}

func TestFormat(t *testing.T) {
	leaps := LeapSecondOnDate(time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 38)
	testCases := [][6]uint8{
		{25, 6, 30, 23, 59, 59},
		{25, 6, 30, 23, 59, 60},
		{25, 7, 1, 00, 00, 00},
		{25, 7, 1, 00, 00, 01},
	}
	for _, tc := range testCases {
		ut := UTC(2000+uint16(tc[0]), tc[1], tc[2], tc[3], tc[4], tc[5], 0)
		expect := fmt.Sprintf("20%02d-%02d-%02dT%02d:%02d:%02dZ", tc[0], tc[1], tc[2], tc[3], tc[4], tc[5])
		s := leaps.FormatTime(leaps.UTCtoTime(ut))
		if s != expect {
			t.Fatalf("got %q, expect %q", s, expect)
		}
	}
}

func checkConsecutive(t *testing.T, leaps *LeapSecond, secs []UTCTime, nanos int32) {
	ts := make([]Time, len(secs))
	for i, ut := range secs {
		ts[i] = leaps.UTCtoTime(ut)
		if i > 0 && ts[i] != ts[i-1]+1e9 {
			t.Fatalf("for nanos %d, %+d leapSec, secs[%d] not consecutive: %d, %d", nanos, leaps.UTCOffAfter-leaps.UTCOffBefore, i, ts[i-1], ts[i])
		}
	}
}

type gnssTime struct {
	week    int16
	towSecs uint32
}

func (t gnssTime) tow() time.Duration {
	return time.Duration(t.towSecs) * time.Second
}

type gnssTimes struct {
	ptp           Time
	gps, gal, bds gnssTime
}

var gnssTimeTest []gnssTimes = []gnssTimes{
	{ptp: Time(1696635646e9), gps: gnssTime{2282, 517227}, gal: gnssTime{1258, 517227}, bds: gnssTime{926, 517213}},
}

func TestGNSS(t *testing.T) {
	for _, tt := range gnssTimeTest {
		gpsTime := GPS(tt.gps.week, tt.gps.tow())
		bdsTime := BeiDou(tt.bds.week, tt.bds.tow())
		galTime := Galileo(tt.gal.week, tt.gal.tow())

		if gpsTime != tt.ptp {
			t.Errorf("GPS(%d, %d) = %d, want %v", tt.gps.week, tt.gps.towSecs, gpsTime, tt.ptp)
		}
		if bdsTime != gpsTime {
			t.Errorf("BeiDou(%d, %d) = %d, want %v", tt.bds.week, tt.bds.towSecs, bdsTime, tt.ptp)
		}
		if galTime != gpsTime {
			t.Errorf("Galileo(%d, %d) = %d, want %v", tt.gal.week, tt.gal.towSecs, galTime, tt.ptp)
		}
	}
}

func TestSysTime(t *testing.T) {
	lsDate := time.Date(2016, time.December, 31, 0, 0, 0, 0, time.UTC)
	lsOver := lsDate.AddDate(0, 0, 1)
	ls := LeapSecondOnDate(lsDate, 36, 37)

	testTimes := []time.Time{
		time.Date(2024, time.January, 8, 36, 0, 0, 0, time.UTC),
		time.Date(2023, time.December, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2023, time.December, 31, 23, 59, 59, 1e9-1, time.UTC),
		time.Date(2024, time.January, 1, 1, 0, 0, 0, time.UTC),
		time.Date(2016, time.January, 8, 36, 0, 0, 0, time.UTC),
		time.Date(2015, time.January, 8, 36, 0, 0, 0, time.UTC),
		lsOver,
		lsOver.Add(-time.Second - time.Nanosecond),
	}
	for _, tm := range testTimes {
		sysTimeTest(t, tm, ls, false)
		timeSysTest(t, tm, ls)
	}
	ambigTimes := []time.Time{
		lsOver.Add(-time.Second / 2),
		lsOver.Add(-time.Second),
		lsOver.Add(-time.Nanosecond),
	}
	for _, tm := range ambigTimes {
		sysTimeTest(t, tm, ls, true)
		timeSysTest(t, tm, ls)
	}
}

func sysTimeTest(t *testing.T, tm time.Time, ls LeapSecond, ambig bool) {
	year, month, day := tm.Date()
	hour, min, sec := tm.Clock()
	utm := UTC(uint16(year), uint8(month), uint8(day), uint8(hour), uint8(min), uint8(sec), int32(tm.Nanosecond()))
	ptu := ls.UTCtoTime(utm)
	pts, ok := ls.SysToTime(tm)
	if ok != !ambig {
		t.Fatalf("SysToTime(%v) = %v, want %v", tm, ok, !ambig)
	}
	if pts != ptu {
		t.Fatalf("SysToTime(%v) = %v, want %v", tm, pts, ptu)
	}
}

func timeSysTest(t *testing.T, tm time.Time, ls LeapSecond) {
	year, month, day := tm.Date()
	hour, min, sec := tm.Clock()
	utm := UTC(uint16(year), uint8(month), uint8(day), uint8(hour), uint8(min), uint8(sec), int32(tm.Nanosecond()))
	ptu := ls.UTCtoTime(utm)
	pts := ls.TimeToSys(ptu)
	if pts != tm {
		t.Fatalf("TimeToSys(%v) = %v, want %v", ptu, pts, tm)
	}
}

func TestTimeToSysAmbig(t *testing.T) {
	lsDate := time.Date(2016, time.December, 31, 0, 0, 0, 0, time.UTC)
	ls := LeapSecondOnDate(lsDate, 36, 37)
	t1 := ls.TimeToSys(ls.OffChangeTime.Add(-time.Second * 3 / 2))
	t2 := ls.TimeToSys(ls.OffChangeTime.Add(-time.Second / 2))
	if t1 != t2 {
		t.Fatalf("ambiguous seconds not equal %v, %v", t1, t2)
	}
	t1 = ls.TimeToSys(ls.OffChangeTime.Add(-time.Second * 2))
	t2 = ls.TimeToSys(ls.OffChangeTime.Add(-time.Second))
	if t1 != t2 {
		t.Fatalf("ambiguous seconds not equal %v, %v", t1, t2)
	}
}

func TestMJD(t *testing.T) {
	ls := LeapSecondOnDate(time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 38)
	type testCase struct {
		utc UTCTime
		mjd float64
	}
	testCases := []testCase{
		{UTC(2023, 2, 25, 0, 0, 0, 0), 60000},
		{UTC(2023, 2, 26, 0, 0, 0, 0), 60001},
		{UTC(2023, 2, 25, 12, 0, 0, 0), 60000.5},
		{UTC(2028, 8, 17, 0, 0, 0, 0), 62000},
		{UTC(2028, 8, 17, 12, 0, 0, 0), 62000.5},
		{UTC(2028, 8, 18, 0, 0, 0, 0), 62001},
		{UTC(2025, 6, 30, 0, 0, 0, 0), 60856},
		{UTC(2025, 7, 1, 0, 0, 0, 0), 60857},
		{UTC(2025, 6, 30, 12, 0, 0, 5e8), 60856.5},
		{UTC(2025, 6, 30, 23, 59, 59, 0), 60856 + 86399.0/86401.0},
		{UTC(2025, 6, 30, 23, 59, 60, 0), 60856 + 86400.0/86401.0},
		{UTC(2025, 6, 30, 23, 59, 60, 5e8), 60856 + 86400.5/86401.0},
	}
	for _, tc := range testCases {
		tm := ls.UTCtoTime(tc.utc)
		mjd := ls.TimeToMJD(tm)
		if mjd != tc.mjd {
			t.Errorf("got %f, expect %f", mjd, tc.mjd)
		}
	}
}
