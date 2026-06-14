package sockrefclock

import (
	"unsafe"

	"github.com/jclark/satpulse/time/lib/ntime"
	"golang.org/x/sys/unix"
)

const sockMagic = 0x534f434b

// Leap is the leap-second status field of a SOCK sample, using the
// values defined by chrony's refclock SOCK protocol.
type Leap int32

const (
	LeapNone Leap = iota
	LeapInsert
	LeapDelete
)

type sockSample struct {
	tv     unix.Timeval // System time of the measurement
	offset float64      // Offset between the true time and the system time (in seconds)
	pulse  int32        // Non-zero if this is PPS only (no seconds)
	leap   Leap
	_      int32
	magic  int32 // must be sockMagic
}

const sizeofSockSample = (64*3 + 32*4) / 8 // timeval is 2 64-bit ints

// sockPacket creates a Chrony refclock SOCK sample packet.
func sockPacket(sys ntime.Time, offset float64, leap Leap) ([]byte, error) {
	var s sockSample
	initSockSample(&s, sys, offset, leap)
	var buf [sizeofSockSample]byte
	type sockBufPtr *[sizeofSockSample]byte
	buf = *(sockBufPtr)(unsafe.Pointer(&s))
	return buf[:], nil
}

func initSockSample(s *sockSample, sys ntime.Time, offset float64, leap Leap) {
	s.tv = unix.NsecToTimeval(int64(sys))
	s.offset = offset
	s.leap = leap
	s.magic = sockMagic
}
