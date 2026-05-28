// Package ntpshm writes samples to the ntpd/NTPsec SHM refclock interface.
package ntpshm

import (
	"errors"
	"math"
	"time"

	"github.com/jclark/satpulse/gps/ptime"
)

// ErrUnsupported is returned when NTP SHM is not supported on this platform.
var ErrUnsupported = errors.New("NTP SHM not supported on this platform")

const minPrecision = -128
const maxPrecision = 127

// Attach reports the result of attaching to an NTP SHM segment.
type Attach struct {
	Unit int
	Key  uint32
}

// Writer writes samples to an NTP SHM segment.
type Writer struct {
	w         shmWriter
	precision int8
}

// New attaches to the NTP SHM segment for unit.
func New(unit uint8) (*Writer, Attach, error) {
	w, a, err := newShmWriter(unit)
	if err != nil {
		return nil, a, err
	}
	return &Writer{w: w}, a, nil
}

// SetPrecision sets the precision reported in subsequent writes.
func (w *Writer) SetPrecision(precision int8) {
	w.precision = precision
}

// Write publishes a clock/receive timestamp pair.
func (w *Writer) Write(clockTime, receiveTime time.Time, leap ptime.LeapSecondKind) {
	w.w.write(clockTime, receiveTime, leap, w.precision)
}

// Close detaches from the NTP SHM segment.
func (w *Writer) Close() error {
	return w.w.close()
}

// Precision returns the NTP log2(seconds) precision exponent for d.
func Precision(d time.Duration) int8 {
	if d <= 0 {
		return minPrecision
	}
	p := math.Ceil(math.Log2(d.Seconds()))
	if p < minPrecision {
		return minPrecision
	}
	if p > maxPrecision {
		return maxPrecision
	}
	return int8(p)
}

func shmKey(unit uint8) uint32 {
	return 0x4e545030 + uint32(unit)
}

func shmLeap(leap ptime.LeapSecondKind) int32 {
	switch leap {
	case ptime.LeapSecondPositive:
		return 1
	case ptime.LeapSecondNegative:
		return 2
	default:
		return 0
	}
}
