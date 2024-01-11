package sockrefclock

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/jclark/satpulse/internal/ptime"
	"golang.org/x/sys/unix"
)

const sockMagic = 0x534f434b

type sockLeap int32

const (
	leapNormal sockLeap = iota
	leapInsert
	leapDelete
)

type sockSample struct {
	tv     unix.Timeval // System time of the measurement
	offset float64      // Offset between the true time and the system time (in seconds)
	pulse  int32        // Non-zero if this is PPS only (no seconds)
	leap   sockLeap
	_      int32
	magic  int32 // must be sockMagic
}

const sizeofSockSample = (64*3 + 32*4) / 8 // timeval is 2 64-bit ints

// sockPacket creates a Chrony refclock SOCK sample packet.
func sockPacket(sys time.Time, ref ptime.Time, ls ptime.LeapSecond) ([]byte, error) {
	var s sockSample
	if err := initSockSample(&s, sys, ref, ls); err != nil {
		return nil, err
	}
	var buf [sizeofSockSample]byte
	type sockBufPtr *[sizeofSockSample]byte
	buf = *(sockBufPtr)(unsafe.Pointer(&s))
	return buf[:], nil
}

func initSockSample(s *sockSample, sys time.Time, ref ptime.Time, ls ptime.LeapSecond) error {
	sysTAI, ok := ls.SysToTime(sys)
	if !ok {
		return fmt.Errorf("cannot create sample for system time %v because it occurs during a positive leap second", sys)
	}
	s.tv = unix.Timeval{Sec: sys.Unix(), Usec: int64(sys.Nanosecond() / 1000)}
	s.offset = ref.Sub(sysTAI).Seconds()
	s.leap = refSockLeap(ref, ls)
	s.magic = sockMagic
	return nil
}

func refSockLeap(ref ptime.Time, ls ptime.LeapSecond) sockLeap {
	switch ls.StateAt(ref).LeapTonight {
	case ptime.LeapSecondNone:
		return leapNormal
	case ptime.LeapSecondPositive:
		return leapInsert
	case ptime.LeapSecondNegative:
		return leapDelete
	default:
		panic("invalid leap second kind")
	}
}
