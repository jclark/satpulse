//go:build linux || darwin

package ntpshm

import (
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/jclark/satpulse/gps/ptime"
	"golang.org/x/sys/unix"
)

var _ [expectedSize - unsafe.Sizeof(shmTime{})]byte
var _ [unsafe.Sizeof(shmTime{}) - expectedSize]byte

type shmWriter struct {
	t    *shmTime
	data []byte
}

func newShmWriter(segment uint8) (shmWriter, Attach, error) {
	key := shmKey(segment)
	id, err := unix.SysvShmGet(int(key), expectedSize, unix.IPC_CREAT|shmMode(segment))
	a := Attach{Segment: int(segment), Key: key}
	if err != nil {
		return shmWriter{}, a, fmt.Errorf("shmget: %w", err)
	}
	data, err := unix.SysvShmAttach(id, 0, 0)
	if err != nil {
		return shmWriter{}, a, fmt.Errorf("shmat: %w", err)
	}
	if len(data) < expectedSize {
		unix.SysvShmDetach(data)
		return shmWriter{}, a, fmt.Errorf("shmat: segment size %d is smaller than %d", len(data), expectedSize)
	}
	w := shmWriter{t: (*shmTime)(unsafe.Pointer(&data[0])), data: data}
	w.init()
	return w, a, nil
}

func shmMode(segment uint8) int {
	if segment < 2 {
		return 0o600
	}
	return 0o666
}

func (w shmWriter) init() {
	atomic.StoreInt32(&w.t.Valid, 0)
	atomic.AddInt32(&w.t.Count, 1)
	w.t.Mode = 1
	w.t.Nsamples = 1
	atomic.AddInt32(&w.t.Count, 1)
}

func (w shmWriter) write(clock, recv time.Time, leap ptime.LeapSecondKind, precision int8) {
	s := w.t
	atomic.StoreInt32(&s.Valid, 0)
	atomic.AddInt32(&s.Count, 1)
	s.ClockTimeStampSec = shmSec(clock.Unix())
	s.ClockTimeStampNSec = int32(clock.Nanosecond())
	s.ClockTimeStampUSec = int32(clock.Nanosecond() / 1000)
	s.ReceiveTimeStampSec = shmSec(recv.Unix())
	s.ReceiveTimeStampNSec = int32(recv.Nanosecond())
	s.ReceiveTimeStampUSec = int32(recv.Nanosecond() / 1000)
	s.Leap = shmLeap(leap)
	s.Precision = int32(precision)
	atomic.AddInt32(&s.Count, 1)
	atomic.StoreInt32(&s.Valid, 1)
}

func (w shmWriter) close() error {
	if len(w.data) == 0 {
		return nil
	}
	return unix.SysvShmDetach(w.data)
}
