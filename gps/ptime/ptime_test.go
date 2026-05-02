package ptime

import (
	"encoding/json"
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

func TestUTCTimeSub(t *testing.T) {
	cases := []struct {
		name string
		ut1  UTCTime
		ut2  UTCTime
		want time.Duration
	}{
		{
			name: "same day",
			ut1:  UTC(2025, 5, 1, 12, 30, 0, 0),
			ut2:  UTC(2025, 5, 1, 12, 29, 58, 500000000),
			want: 1500 * time.Millisecond,
		},
		{
			name: "ordinary midnight",
			ut1:  UTC(2025, 5, 2, 0, 0, 1, 0),
			ut2:  UTC(2025, 5, 1, 23, 59, 59, 500000000),
			want: 1500 * time.Millisecond,
		},
		{
			name: "negative time of day",
			ut1: UTCTime{
				Date:      time.Date(2025, time.July, 1, 0, 0, 0, 0, time.UTC),
				TimeOfDay: -1 * time.Hour,
			},
			ut2:  UTC(2025, 6, 30, 22, 0, 0, 0),
			want: time.Hour,
		},
		{
			name: "into positive leap second",
			ut1:  UTC(2025, 6, 30, 23, 59, 60, 250000000),
			ut2:  UTC(2025, 6, 30, 23, 59, 59, 750000000),
			want: 500 * time.Millisecond,
		},
		{
			name: "within positive leap second",
			ut1:  UTC(2025, 6, 30, 23, 59, 60, 750000000),
			ut2:  UTC(2025, 6, 30, 23, 59, 60, 250000000),
			want: 500 * time.Millisecond,
		},
		{
			name: "out of positive leap second",
			ut1:  UTC(2025, 7, 1, 0, 0, 0, 0),
			ut2:  UTC(2025, 6, 30, 23, 59, 60, 0),
			want: time.Second,
		},
		{
			name: "out of positive leap second subsecond",
			ut1:  UTC(2025, 7, 1, 0, 0, 0, 0),
			ut2:  UTC(2025, 6, 30, 23, 59, 60, 250000000),
			want: 750 * time.Millisecond,
		},
		{
			name: "reverse out of positive leap second",
			ut1:  UTC(2025, 6, 30, 23, 59, 60, 0),
			ut2:  UTC(2025, 7, 1, 0, 0, 0, 0),
			want: -time.Second,
		},
		{
			name: "negative result",
			ut1:  UTC(2025, 6, 30, 23, 59, 59, 750000000),
			ut2:  UTC(2025, 6, 30, 23, 59, 60, 250000000),
			want: -500 * time.Millisecond,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.ut1.Sub(tc.ut2)
			if got != tc.want {
				t.Fatalf("%v.Sub(%v) = %v, want %v", tc.ut1, tc.ut2, got, tc.want)
			}
		})
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

func TestTimeToUTC(t *testing.T) {
	posLeap := LeapSecondOnDate(time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 38)
	negLeap := LeapSecondOnDate(time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 36)
	ls2016 := LeapSecond2016()
	type testCase struct {
		name string
		ls   LeapSecond
		utc  UTCTime
	}
	cases := []testCase{
		// Positive leap second: consecutive seconds spanning :60
		{"pos :57", posLeap, UTC(2025, 6, 30, 23, 59, 57, 0)},
		{"pos :58", posLeap, UTC(2025, 6, 30, 23, 59, 58, 0)},
		{"pos :59", posLeap, UTC(2025, 6, 30, 23, 59, 59, 0)},
		{"pos :60", posLeap, UTC(2025, 6, 30, 23, 59, 60, 0)},
		{"pos next day :00", posLeap, UTC(2025, 7, 1, 0, 0, 0, 0)},
		{"pos next day :01", posLeap, UTC(2025, 7, 1, 0, 0, 1, 0)},
		// Negative leap second: :59 is skipped
		{"neg :57", negLeap, UTC(2025, 6, 30, 23, 59, 57, 0)},
		{"neg :58", negLeap, UTC(2025, 6, 30, 23, 59, 58, 0)},
		{"neg next day :00", negLeap, UTC(2025, 7, 1, 0, 0, 0, 0)},
		{"neg next day :01", negLeap, UTC(2025, 7, 1, 0, 0, 1, 0)},
		// Boundary nanoseconds around positive leap second
		{"pos :59.999999999", posLeap, UTC(2025, 6, 30, 23, 59, 59, 999999999)},
		{"pos :60.000000001", posLeap, UTC(2025, 6, 30, 23, 59, 60, 1)},
		{"pos :60.5", posLeap, UTC(2025, 6, 30, 23, 59, 60, 500000000)},
		{"pos :60.999999999", posLeap, UTC(2025, 6, 30, 23, 59, 60, 999999999)},
		// Sub-second precision at key seconds
		{"pos :59 +1ns", posLeap, UTC(2025, 6, 30, 23, 59, 59, 1)},
		{"pos :59 +500ms", posLeap, UTC(2025, 6, 30, 23, 59, 59, 500000000)},
		{"pos :00 +1ns", posLeap, UTC(2025, 7, 1, 0, 0, 0, 1)},
		{"pos :00 +999999999ns", posLeap, UTC(2025, 7, 1, 0, 0, 0, 999999999)},
		// 2016 leap second
		{"2016 before :58", ls2016, UTC(2016, 12, 31, 23, 59, 58, 0)},
		{"2016 before :59", ls2016, UTC(2016, 12, 31, 23, 59, 59, 0)},
		{"2016 :60", ls2016, UTC(2016, 12, 31, 23, 59, 60, 0)},
		{"2016 :60.5", ls2016, UTC(2016, 12, 31, 23, 59, 60, 500000000)},
		{"2016 new year", ls2016, UTC(2017, 1, 1, 0, 0, 0, 0)},
		// Normal times well after 2016 leap second
		{"normal 2024 midnight", ls2016, UTC(2024, 1, 1, 0, 0, 0, 0)},
		{"normal 2024 midday", ls2016, UTC(2024, 6, 15, 12, 30, 45, 123456789)},
		{"normal 2020 nye", ls2016, UTC(2020, 12, 31, 23, 59, 59, 0)},
		// On the day of 2016 leap second, before it
		{"2016 day noon", ls2016, UTC(2016, 12, 31, 12, 0, 0, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tai := tc.ls.UTCtoTime(tc.utc)
			got := tc.ls.TimeToUTC(tai)
			if got != tc.utc {
				t.Errorf("got Date=%v ToD=%v, want Date=%v ToD=%v",
					got.Date, got.TimeOfDay, tc.utc.Date, tc.utc.TimeOfDay)
			}
		})
	}
	// Verify TAI is continuous across leap second boundary
	t.Run("TAI continuity at boundary", func(t *testing.T) {
		leapEnd := posLeap.UTCtoTime(UTC(2025, 6, 30, 23, 59, 60, 999999999))
		afterLeap := posLeap.UTCtoTime(UTC(2025, 7, 1, 0, 0, 0, 0))
		if afterLeap != leapEnd+1 {
			t.Errorf("boundary not 1ns apart: %d, %d", leapEnd, afterLeap)
		}
	})
}

func TestTimeToUTCLeapTimeOfDay(t *testing.T) {
	ls := LeapSecondOnDate(time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 38)
	expectDate := time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		nanos     int32
		expectToD time.Duration
	}{
		{":60.0", 0, 24 * time.Hour},
		{":60.5", 500000000, 24*time.Hour + 500*time.Millisecond},
		{":60.999999999", 999999999, 24*time.Hour + 999999999},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tai := ls.UTCtoTime(UTC(2025, 6, 30, 23, 59, 60, tc.nanos))
			got := ls.TimeToUTC(tai)
			if got.TimeOfDay != tc.expectToD {
				t.Errorf("TimeOfDay = %v, want %v", got.TimeOfDay, tc.expectToD)
			}
			if !got.Date.Equal(expectDate) {
				t.Errorf("Date = %v, want %v", got.Date, expectDate)
			}
		})
	}
}

func TestTimeToUTCFormatConsistency(t *testing.T) {
	ls := LeapSecondOnDate(time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 38)
	cases := []struct {
		utc    UTCTime
		format string
	}{
		{UTC(2025, 6, 30, 23, 59, 59, 0), "2025-06-30T23:59:59Z"},
		{UTC(2025, 6, 30, 23, 59, 60, 0), "2025-06-30T23:59:60Z"},
		{UTC(2025, 7, 1, 0, 0, 0, 0), "2025-07-01T00:00:00Z"},
		{UTC(2025, 7, 1, 0, 0, 1, 0), "2025-07-01T00:00:01Z"},
	}
	for _, tc := range cases {
		tai := ls.UTCtoTime(tc.utc)
		got := ls.TimeToUTC(tai)
		if got != tc.utc {
			t.Errorf("round-trip %s: got %+v, want %+v", tc.format, got, tc.utc)
		}
		if s := ls.FormatTime(tai); s != tc.format {
			t.Errorf("FormatTime for %s: got %q", tc.format, s)
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
	lsDate := leapSecond2016Date
	lsOver := lsDate.AddDate(0, 0, 1)
	ls := LeapSecondOnDate(lsDate, LeapSecond2016UTCOffBefore, LeapSecond2016UTCOffAfter)

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

func TestGPSUTC(t *testing.T) {
	expected := UTC(2025, 5, 27, 3, 38, 53, 0)
	ut := GPSUTC(2368, 185933*time.Second)
	if expected != ut {
		t.Errorf("GPSUTC(2368, 185933) = %v, want %v", ut, expected)
	}
}

// TestGLONASSWeekUTC verifies conversion from GLONASS week/TOW to UTCTime.
// Uses captured data: week 1577, TOW 13796000ms = 2026-03-22 03:49:56 UTC.
func TestGLONASSWeekUTC(t *testing.T) {
	expected := UTC(2026, 3, 22, 3, 49, 56, 0)
	ut := GLONASSWeekUTC(1577, 13796000*time.Millisecond)
	if expected != ut {
		t.Errorf("GLONASSWeekUTC(1577, 13796000ms) = %v, want %v", ut, expected)
	}
}

type leapSecondTestCase struct {
	name      string
	now       Time
	gnss      GNSSLeapSecond
	expect    LeapSecond
	expectErr bool
}

func TestGPSLeapSecond(t *testing.T) {
	// Helper: construct a TAI Time from a UTC instant given current TAI−UTC.
	pastNowUTC := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // comfortably after 2016-12-31
	pastNowTAI := Unix(pastNowUTC.Unix()+37, 0)               // all three systems have 37s (TAI−UTC) since 2017
	nowTAI := func(utc time.Time) Time { return Unix(utc.Unix()+37, 0) }

	const sixMonths = 183 * 24 * time.Hour
	futureDate := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

	testCases := []leapSecondTestCase{
		{
			name:   "GPS past (receiver showed WNLSF=2441, DN=7)",
			now:    pastNowTAI,
			gnss:   GNSSLeapSecond{WNLSF: 2441 % 256, DN: 7, DeltaLS: 18, DeltaLSF: 18}, // 137, Saturday (1-based)
			expect: LeapSecond2016(),
		},
		{
			name:   "GPS future within horizon",
			now:    nowTAI(futureDate.Add(-179 * 24 * time.Hour)),
			gnss:   GNSSLeapSecond{WNLSF: 147, DN: 5, DeltaLS: 18, DeltaLSF: 19}, // Thursday (1-based: 5)
			expect: LeapSecondOnDate(futureDate, TAIMinusGPS+18, TAIMinusGPS+19),
		},
		{
			name:      "GPS future beyond horizon → error",
			now:       nowTAI(futureDate.Add(-(sixMonths + 10*24*time.Hour))),
			gnss:      GNSSLeapSecond{WNLSF: 147, DN: 5, DeltaLS: 18, DeltaLSF: 19},
			expectErr: true,
		},
	}

	runLeapSecondTests(t, testCases, GPSLeapSecond)
}

func TestGalileoLeapSecond(t *testing.T) {
	// Helper: construct a TAI Time from a UTC instant given current TAI−UTC.
	pastNowUTC := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pastNowTAI := Unix(pastNowUTC.Unix()+37, 0)
	nowTAI := func(utc time.Time) Time { return Unix(utc.Unix()+37, 0) }

	const sixMonths = 183 * 24 * time.Hour
	futureDate := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)

	testCases := []leapSecondTestCase{
		{
			name:   "Galileo past (receiver showed WnLSF=1417, Dn=7)",
			now:    pastNowTAI,
			gnss:   GNSSLeapSecond{WNLSF: 1417 % 256, DN: 7, DeltaLS: 18, DeltaLSF: 18}, // 137, Saturday (1-based)
			expect: LeapSecond2016(),
		},
		{
			name:   "Galileo future within horizon",
			now:    nowTAI(futureDate.Add(-170 * 24 * time.Hour)),
			gnss:   GNSSLeapSecond{WNLSF: 147, DN: 5, DeltaLS: 18, DeltaLSF: 19}, // Thursday (1-based: 5)
			expect: LeapSecondOnDate(futureDate, TAIMinusGalileo+18, TAIMinusGalileo+19),
		},
		{
			name:      "Galileo future beyond horizon → error",
			now:       nowTAI(futureDate.Add(-(sixMonths + 1*time.Hour))),
			gnss:      GNSSLeapSecond{WNLSF: 147, DN: 5, DeltaLS: 18, DeltaLSF: 19},
			expectErr: true,
		},
	}

	runLeapSecondTests(t, testCases, GalileoLeapSecond)
}

func TestBeiDouLeapSecond(t *testing.T) {
	// Helper: construct a TAI Time from a UTC instant given current TAI−UTC.
	pastNowUTC := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pastNowTAI := Unix(pastNowUTC.Unix()+37, 0)
	nowTAI := func(utc time.Time) Time { return Unix(utc.Unix()+37, 0) }

	const sixMonths = 183 * 24 * time.Hour
	futureDate := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	lastLeapSecond := LeapSecond2016()

	testCases := []leapSecondTestCase{
		{
			name:   "BeiDou-3 past (receiver showed WnLSF=61, Dn=6, ΔtLS=4, ΔtLSF=4)",
			now:    pastNowTAI,
			gnss:   GNSSLeapSecond{WNLSF: 61, DN: 6, DeltaLS: 4, DeltaLSF: 4}, // Saturday (0-based)
			expect: lastLeapSecond,
		},
		{
			name:   "BeiDou-2 past (receiver showed WnLSF=1085, Dn=6, ΔtLS=4, ΔtLSF=4)",
			now:    pastNowTAI,
			gnss:   GNSSLeapSecond{WNLSF: 1085 % 256, DN: 6, DeltaLS: 4, DeltaLSF: 4}, // 61, Saturday (0-based)
			expect: lastLeapSecond,
		},
		{
			name:   "BeiDou-3 future within horizon",
			now:    nowTAI(futureDate.Add(-150 * 24 * time.Hour)),
			gnss:   GNSSLeapSecond{WNLSF: 71, DN: 4, DeltaLS: 4, DeltaLSF: 5}, // Thursday (0-based: 4)
			expect: LeapSecondOnDate(futureDate, TAIMinusBeiDou+4, TAIMinusBeiDou+5),
		},
		{
			name:      "BeiDou-3 future beyond horizon → error",
			now:       nowTAI(futureDate.Add(-(sixMonths + 24*time.Hour))),
			gnss:      GNSSLeapSecond{WNLSF: 71, DN: 4, DeltaLS: 4, DeltaLSF: 5},
			expectErr: true,
		},
	}

	runLeapSecondTests(t, testCases, BeiDouLeapSecond)
}

func runLeapSecondTests(t *testing.T, testCases []leapSecondTestCase, testFunc func(GNSSLeapSecond, Time) (LeapSecond, error)) {
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ls, err := testFunc(tt.gnss, tt.now)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("leap second conversion error: %v", err)
			}
			if ls != tt.expect {
				t.Errorf("got %+v, expect %+v", ls, tt.expect)
			}
		})
	}
}

func TestUTCTimeTextLeapSeconds(t *testing.T) {
	testCases := []struct {
		name string
		utc  UTCTime
		text string
	}{
		{
			name: "leap second exact second 60",
			utc:  UTC(2025, 6, 30, 23, 59, 60, 0),
			text: "2025-06-30T23:59:60Z",
		},
		{
			name: "leap second with nanoseconds",
			utc:  UTC(2025, 6, 30, 23, 59, 60, 500000000),
			text: "2025-06-30T23:59:60.5Z",
		},
		{
			name: "leap second end (max nanos)",
			utc:  UTC(2025, 6, 30, 23, 59, 60, 999999999),
			text: "2025-06-30T23:59:60.999999999Z",
		},
		{
			name: "second before leap second",
			utc:  UTC(2025, 6, 30, 23, 59, 59, 999999999),
			text: "2025-06-30T23:59:59.999999999Z",
		},
		{
			name: "second after leap second",
			utc:  UTC(2025, 7, 1, 0, 0, 0, 0),
			text: "2025-07-01T00:00:00Z",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name+" marshal", func(t *testing.T) {
			data, err := tc.utc.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText error: %v", err)
			}
			if string(data) != tc.text {
				t.Errorf("got %s, want %s", string(data), tc.text)
			}
		})
		t.Run(tc.name+" unmarshal", func(t *testing.T) {
			var utc UTCTime
			err := utc.UnmarshalText([]byte(tc.text))
			if err != nil {
				t.Fatalf("UnmarshalText error: %v", err)
			}
			if utc != tc.utc {
				t.Errorf("got Date=%v TimeOfDay=%v, want Date=%v TimeOfDay=%v",
					utc.Date, utc.TimeOfDay, tc.utc.Date, tc.utc.TimeOfDay)
			}
		})
		t.Run(tc.name+" json marshal", func(t *testing.T) {
			data, err := json.Marshal(tc.utc)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			want := `"` + tc.text + `"`
			if string(data) != want {
				t.Errorf("got %s, want %s", string(data), want)
			}
		})
		t.Run(tc.name+" json unmarshal", func(t *testing.T) {
			var utc UTCTime
			err := json.Unmarshal([]byte(`"`+tc.text+`"`), &utc)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if utc != tc.utc {
				t.Errorf("got Date=%v TimeOfDay=%v, want Date=%v TimeOfDay=%v",
					utc.Date, utc.TimeOfDay, tc.utc.Date, tc.utc.TimeOfDay)
			}
		})
		t.Run(tc.name+" round-trip", func(t *testing.T) {
			data, err := json.Marshal(tc.utc)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			var utc UTCTime
			err = json.Unmarshal(data, &utc)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if utc != tc.utc {
				t.Errorf("round-trip failed:\ngot  Date=%v TimeOfDay=%v\nwant Date=%v TimeOfDay=%v",
					utc.Date, utc.TimeOfDay, tc.utc.Date, tc.utc.TimeOfDay)
			}
		})
	}
}

func TestUTCTimeTextInvalidLeapSeconds(t *testing.T) {
	invalidCases := []struct {
		name string
		text string
	}{
		{"second 61 (invalid)", "2025-06-30T23:59:61Z"},
		{"second 62 (invalid)", "2025-06-30T23:59:62Z"},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			var utc UTCTime
			err := utc.UnmarshalText([]byte(tc.text))
			if err == nil {
				t.Errorf("expected error for %s, got none", tc.text)
			}
		})
	}
}

func TestUTCStateAt(t *testing.T) {
	ls := LeapSecondOnDate(time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC), 37, 38)
	leapDay := time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC)
	nextDay := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	normalDay := time.Date(2026, time.March, 29, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		ut         UTCTime
		expectLeap LeapSecondKind
		expectOff  int16
	}{
		{
			name:       "normal_day",
			ut:         UTCTime{Date: normalDay, TimeOfDay: 15 * time.Hour},
			expectLeap: LeapSecondNone,
			expectOff:  37,
		},
		{
			name:       "leap_day_in_window",
			ut:         UTCTime{Date: leapDay, TimeOfDay: 20 * time.Hour},
			expectLeap: LeapSecondPositive,
			expectOff:  37,
		},
		{
			name:       "leap_day_at_12h_boundary",
			ut:         UTCTime{Date: leapDay, TimeOfDay: 12 * time.Hour},
			expectLeap: LeapSecondPositive,
			expectOff:  37,
		},
		{
			name:       "leap_day_before_window",
			ut:         UTCTime{Date: leapDay, TimeOfDay: 6 * time.Hour},
			expectLeap: LeapSecondNone,
			expectOff:  37,
		},
		{
			name:       "day_after_leap",
			ut:         UTCTime{Date: nextDay, TimeOfDay: 6 * time.Hour},
			expectLeap: LeapSecondNone,
			expectOff:  38,
		},
		{
			// GLONASS: 21:00 UTC on June 30 as July 1 minus 3h
			name:       "negative_tod_in_window",
			ut:         UTCTime{Date: nextDay, TimeOfDay: -3 * time.Hour},
			expectLeap: LeapSecondPositive,
			expectOff:  37,
		},
		{
			// GLONASS: 03:00 UTC on June 30 as June 30 minus 21h
			// Normalizes to June 29 03:00 -- not the leap day
			name:       "negative_tod_wrong_day",
			ut:         UTCTime{Date: leapDay, TimeOfDay: -21 * time.Hour},
			expectLeap: LeapSecondNone,
			expectOff:  37,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ls.UTCStateAt(tc.ut)
			if got.LeapTonight != tc.expectLeap {
				t.Errorf("LeapTonight = %v, want %v", got.LeapTonight, tc.expectLeap)
			}
			if got.UTCOffset != tc.expectOff {
				t.Errorf("UTCOffset = %v, want %v", got.UTCOffset, tc.expectOff)
			}
		})
	}
}
