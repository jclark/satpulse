package sockrefclock

import (
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
	s.tv = unix.NsecToTimeval(sys.UnixNano())
	// chrony will ignore measurements in the vicinity of leap seconds, so we don't have to worry about them
	// but I still wonder if we should filter out cases where either of the two times are ambiguous
	s.offset = ls.TimeToSys(ref).Sub(sys).Seconds()
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
