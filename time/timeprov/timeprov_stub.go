//go:build !windows

package timeprov

import (
	"errors"

	"github.com/jclark/satpulse/time/lib/ntime"
	"github.com/jclark/satpulse/time/sockrefclock"
)

// ErrUnsupported is returned when the w32time refclock is not
// supported on this platform. w32time exists only on Windows, so New
// fails cleanly when [ntp.timeprov] is configured elsewhere.
var ErrUnsupported = errors.New("w32time timeprov refclock not supported on this platform")

// record is never called off Windows: New fails on newPipeConn before
// a TimeProv exists. It exists only so the shared Sample compiles; the
// wire format lives in the windows-tagged record (from wire.h).
func record(sys ntime.Time, offset float64, dispersion uint64, leap sockrefclock.Leap) []byte {
	return nil
}

type pipeConn struct {
	path string
}

func newPipeConn(path string) (*pipeConn, error) {
	return nil, ErrUnsupported
}

func (c *pipeConn) send(pkt []byte) error {
	return ErrUnsupported
}

func (c *pipeConn) close() error {
	return ErrUnsupported
}
