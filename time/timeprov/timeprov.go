// Package timeprov sends refclock samples to the Windows Time service
// (w32time) through pipe-timeprov, a w32time time provider DLL that
// receives samples over a named pipe
// (https://github.com/jclark/pipe-timeprov). It is the Windows
// counterpart of the chrony SOCK refclock in time/sockrefclock and
// implements the same sample interface. The wire format uses the units
// and epoch of the w32time time provider interface: 100 ns intervals,
// NT epoch 1601-01-01 UTC.
package timeprov

import (
	"time"

	"github.com/jclark/satpulse/time/lib/ntime"
	"github.com/jclark/satpulse/time/sockrefclock"
)

// DefaultPipe is the pipe path the pipe-timeprov provider listens on
// unless configured otherwise.
const DefaultPipe = `\\.\pipe\pipetimeprov`

type TimeProv struct {
	conn       *pipeConn
	dispersion uint64 // in 100 ns
}

// New creates a TimeProv sending to the named pipe at path; an empty
// path uses DefaultPipe. dispersion is the measurement error reported
// with every sample.
func New(path string, dispersion time.Duration) (*TimeProv, error) {
	if path == "" {
		path = DefaultPipe
	}
	conn, err := newPipeConn(path)
	if err != nil {
		return nil, err
	}
	return &TimeProv{conn: conn, dispersion: uint64(dispersion / 100)}, nil
}

func (c *TimeProv) RemotePath() string {
	return c.conn.path
}

func (c *TimeProv) Sample(sys ntime.Time, offset float64, leap sockrefclock.Leap) error {
	return c.conn.send(record(sys, offset, c.dispersion, leap))
}

func (c *TimeProv) Close() error {
	return c.conn.close()
}
