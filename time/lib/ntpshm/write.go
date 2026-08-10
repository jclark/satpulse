//go:build linux || darwin || windows

package ntpshm

import (
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/jclark/satpulse/gps/ptime"
)

var _ [expectedSize - unsafe.Sizeof(shmTime{})]byte
var _ [unsafe.Sizeof(shmTime{}) - expectedSize]byte

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
