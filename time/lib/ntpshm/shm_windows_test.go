package ntpshm

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

// TestAttachRoundTrip creates two mappings on the same name and reads a
// sample written through one via the other. Segment 1 keeps the name in
// the Local\ namespace, which is per logon session, so an unprivileged
// test cannot clash with an ntpd service and needs no create-global
// privilege; the mapping is refcounted, so closing both writers destroys
// it and no cleanup beyond Close is needed.
func TestAttachRoundTrip(t *testing.T) {
	const segment = 1
	w1, a1, err := New(segment)
	if err != nil {
		t.Fatalf("New first writer: %v", err)
	}
	defer w1.Close()
	w2, a2, err := New(segment)
	if err != nil {
		t.Fatalf("New second writer: %v", err)
	}
	defer w2.Close()
	if a1.Key != shmKey(segment) || a2.Key != a1.Key {
		t.Fatalf("attach keys = %#x/%#x, want %#x", a1.Key, a2.Key, shmKey(segment))
	}
	clock := time.Unix(1_710_000_000, 111_222_333)
	recv := time.Unix(1_710_000_001, 444_555_666)
	w1.SetPrecision(-17)
	w1.Write(clock, recv, ptime.LeapSecondNegative)
	if int64(w2.w.t.ClockTimeStampSec) != clock.Unix() || w2.w.t.ClockTimeStampNSec != int32(clock.Nanosecond()) {
		t.Fatalf("second attachment did not observe clock write: %+v", *w2.w.t)
	}
	if int64(w2.w.t.ReceiveTimeStampSec) != recv.Unix() || w2.w.t.ReceiveTimeStampNSec != int32(recv.Nanosecond()) {
		t.Fatalf("second attachment did not observe receive write: %+v", *w2.w.t)
	}
	if w2.w.t.Leap != 2 || w2.w.t.Precision != -17 || w2.w.t.Valid != 1 {
		t.Fatalf("second attachment metadata = leap %d precision %d valid %d, want 2 -17 1", w2.w.t.Leap, w2.w.t.Precision, w2.w.t.Valid)
	}
}
