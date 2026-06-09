//go:build linux

package ntpshm

import (
	"errors"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
	"golang.org/x/sys/unix"
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

func TestAttachExisting(t *testing.T) {
	const segment = 254
	if id, err := shmID(segment); err == nil {
		t.Skipf("NTP SHM test segment %d already exists as shmid %d", segment, id)
	} else if !errors.Is(err, unix.ENOENT) {
		t.Fatalf("checking existing segment: %v", err)
	}
	var w1, w2 *Writer
	t.Cleanup(func() {
		if w2 != nil {
			_ = w2.Close()
		}
		if w1 != nil {
			_ = w1.Close()
		}
		if err := removeSHM(segment); err != nil {
			t.Errorf("removing SHM test segment %d: %v", segment, err)
		}
	})
	var a1, a2 Attach
	var err error
	w1, a1, err = New(segment)
	if err != nil {
		t.Fatalf("New first writer: %v", err)
	}
	w2, a2, err = New(segment)
	if err != nil {
		t.Fatalf("New second writer: %v", err)
	}
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

func shmID(segment uint8) (int, error) {
	return unix.SysvShmGet(int(shmKey(segment)), expectedSize, 0)
}

func removeSHM(segment uint8) error {
	id, err := shmID(segment)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = unix.SysvShmCtl(id, unix.IPC_RMID, nil)
	return err
}
