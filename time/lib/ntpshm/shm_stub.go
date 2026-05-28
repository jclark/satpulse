//go:build !linux

package ntpshm

import (
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

type shmWriter struct{}

func newShmWriter(unit uint8) (shmWriter, Attach, error) {
	return shmWriter{}, Attach{Unit: int(unit), Key: shmKey(unit)}, ErrUnsupported
}

func (w shmWriter) write(clock, recv time.Time, leap ptime.LeapSecondKind, precision int8) {}

func (w shmWriter) close() error { return nil }
