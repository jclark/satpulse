package sockrefclock

import (
	"errors"
	"net"

	"github.com/jclark/satpulse/time/lib/ntime"
)

// ErrUnsupported is returned when the chrony SOCK refclock is not supported
// on this platform. chrony does not run on Windows, so the SOCK refclock has
// no peer there; New fails cleanly when [ntp.sock] is configured.
var ErrUnsupported = errors.New("chrony SOCK refclock not supported on this platform")

func sockPacket(sys ntime.Time, offset float64, leap Leap) ([]byte, error) {
	return nil, ErrUnsupported
}

func listenUnixgramUnbound() (*net.UnixConn, error) {
	return nil, ErrUnsupported
}
