package ptime

import (
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

func checkConsecutive(t *testing.T, leaps *LeapSecond, secs []UTCTime, nanos int32) {
	ts := make([]Time, len(secs))
	for i, ut := range secs {
		ts[i] = leaps.UTCtoTime(ut)
		if i > 0 && ts[i] != ts[i-1]+1e9 {
			t.Fatalf("for nanos %d, %+d leapSec, secs[%d] not consecutive: %d, %d", nanos, leaps.UTCOffAfter-leaps.UTCOffBefore, i, ts[i-1], ts[i])
		}
	}
}
