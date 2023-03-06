package ptime

import (
	"testing"
	"time"
)

func TestLeapSecs(t *testing.T) {
	leaps := LeapSecond{
		LastDate:  time.Date(2025, time.June, 30, 0, 0, 0, 0, time.UTC),
		OffBefore: 37,
		OffAfter:  38,
	}

	testLeapNanos(t, &leaps, 0)
	testLeapNanos(t, &leaps, 1)
	testLeapNanos(t, &leaps, 1e9/2)
	testLeapNanos(t, &leaps, 1e9-1)
	testLeapNanos(t, &leaps, -1)
	testLeapNanos(t, &leaps, 1-1e9)
	testLeapNanos(t, &leaps, -1e9/2)
}

func testLeapNanos(t *testing.T, leaps *LeapSecond, nanos int32) {
	secs := []UTCTime{
		UTC(2025, 6, 30, 23, 59, 59, nanos),
		UTC(2025, 6, 30, 23, 59, 60, nanos),
		UTC(2025, 7, 1, 0, 0, 0, nanos),
		UTC(2025, 7, 1, 0, 0, 1, nanos),
	}
	ts := make([]Time, len(secs))
	for i, ut := range secs {
		ts[i] = leaps.UTCtoTime(&ut)
		if i > 0 && ts[i] != ts[i-1]+1e9 {
			t.Fatalf("for nanos %d, secs[%d] not consecutive: %d, %d", nanos, i, ts[i-1], ts[i])
		}
	}

}
