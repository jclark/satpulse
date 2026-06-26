//go:build !linux && !darwin

package ntpshm

import (
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

type shmWriter struct{}

func newShmWriter(segment uint8) (shmWriter, Attach, error) {
	return shmWriter{}, Attach{Segment: int(segment), Key: shmKey(segment)}, ErrUnsupported
}

func (w shmWriter) write(clock, recv time.Time, leap ptime.LeapSecondKind, precision int8) {}

func (w shmWriter) close() error { return nil }
