//go:build linux || darwin || windows

package ntpshm

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

func TestWriteRoundTrip(t *testing.T) {
	clock := time.Unix(1_710_000_000, 123_456_789)
	recv := time.Unix(1_710_000_001, 987_654_321)
	for _, precision := range []int8{-128, -23, 7, 127} {
		var s shmTime
		w := &Writer{w: shmWriter{t: &s}}
		w.w.init()
		w.SetPrecision(precision)
		count := s.Count
		w.Write(clock, recv, ptime.LeapSecondPositive)
		if s.Mode != 1 || s.Nsamples != 1 || s.Valid != 1 {
			t.Fatalf("metadata = mode %d nsamples %d valid %d, want 1 1 1", s.Mode, s.Nsamples, s.Valid)
		}
		if s.Count != count+2 {
			t.Fatalf("count = %d, want %d", s.Count, count+2)
		}
		if int64(s.ClockTimeStampSec) != clock.Unix() || s.ClockTimeStampNSec != int32(clock.Nanosecond()) || s.ClockTimeStampUSec != int32(clock.Nanosecond()/1000) {
			t.Fatalf("clock timestamp fields do not match %v: %+v", clock, s)
		}
		if int64(s.ReceiveTimeStampSec) != recv.Unix() || s.ReceiveTimeStampNSec != int32(recv.Nanosecond()) || s.ReceiveTimeStampUSec != int32(recv.Nanosecond()/1000) {
			t.Fatalf("receive timestamp fields do not match %v: %+v", recv, s)
		}
		if s.Leap != 1 || s.Precision != int32(precision) {
			t.Fatalf("leap/precision = %d/%d, want 1/%d", s.Leap, s.Precision, precision)
		}
	}
}
