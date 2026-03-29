package sockrefclock

import (
	"time"
	"unsafe"

	"github.com/jclark/satpulse/gps/ptime"
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
func sockPacket(sys time.Time, offset float64, leap ptime.LeapSecondKind) ([]byte, error) {
	var s sockSample
	initSockSample(&s, sys, offset, leap)
	var buf [sizeofSockSample]byte
	type sockBufPtr *[sizeofSockSample]byte
	buf = *(sockBufPtr)(unsafe.Pointer(&s))
	return buf[:], nil
}

func initSockSample(s *sockSample, sys time.Time, offset float64, leap ptime.LeapSecondKind) {
	s.tv = unix.NsecToTimeval(sys.UnixNano())
	s.offset = offset
	s.leap = leapToSock(leap)
	s.magic = sockMagic
}

func leapToSock(leap ptime.LeapSecondKind) sockLeap {
	switch leap {
	case ptime.LeapSecondPositive:
		return leapInsert
	case ptime.LeapSecondNegative:
		return leapDelete
	default:
		return leapNormal
	}
}
