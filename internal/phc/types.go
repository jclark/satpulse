package phc

import (
	"errors"
	"fmt"
	"time"

	"github.com/jclark/satpulse/internal/ptime"
)

// ErrNotSupported is returned when PHC operations are not supported on the current platform.
var ErrNotSupported = errors.New("PHC not supported on this platform")

// PinFunc represents a pin function type.
type PinFunc uint32

func (f PinFunc) String() string {
	switch f {
	case PinFuncNone:
		return "none"
	case PinFuncExtts:
		return "extts"
	case PinFuncPerout:
		return "perout"
	case PinFuncPhysync:
		return "physync"
	default:
		return fmt.Sprintf("unknown(%d)", f)
	}
}

// PinDesc describes a PHC pin configuration.
type PinDesc struct {
	Name  string
	Index uint32
	Func  PinFunc
	Chan  uint32
}

// DriverFlags represents PHC driver capability flags.
type DriverFlags uint32

const (
	DriverOneEdge DriverFlags = 1 << iota
	DriverBothEdges
	DriverKnown
	DriverPoll4Hz
	DriverCarrier
	DriverEdges DriverFlags = DriverOneEdge | DriverBothEdges
)

func (flags DriverFlags) Edges() int {
	return int(flags & DriverEdges)
}

func (flags DriverFlags) SetEdges(edges int) DriverFlags {
	if edges == 0 {
		return flags
	}
	if edges != 1 && edges != 2 {
		panic(fmt.Sprintf("invalid number of pulse edges %d", edges))
	}
	flags &^= DriverEdges
	return flags | DriverFlags(edges)
}

// PhyID represents a PHY identifier.
type PhyID uint32

// TSEvent represents a timestamp event.
type TSEvent struct {
	T      time.Time
	Seq    uint32
	Index  int
	Offset time.Duration
}

// TSEdge represents timestamp edge selection.
type TSEdge int

const (
	TSRising TSEdge = iota
	TSFalling
	TSBoth
)

// A MultiSample is a series of samples of the PHC and system clocks.
// In the case of a MultiSample returned by SysOffset, the number
// of PHC samples is the one more than the number of system clock samples.
// In the case of a MultiSample returned by SysOffsetExtended,
// the number of system clock samples is twice the number of PHC samples;
// the system clock samples are the system clock times immediately before and after the PHC clock time.
// In the case of a MultiSample returns by SysOffsetPrecise, the number of PHC samples
// and of system clocks samples is both one.
type MultiSample struct {
	PHC []ptime.Time
	Sys []time.Time
}

func (ms MultiSample) Reduce() (ptime.Time, time.Time) {
	return ms.Extract(ms.Select())
}

func (ms MultiSample) Extract(i int) (ptime.Time, time.Time) {
	return ms.PHC[i], ms.SysAverage(i)
}

func (ms MultiSample) Select() int {
	best := 0
	minInterval := ms.SysInterval(0)
	for i := 1; i < len(ms.PHC); i++ {
		if interval := ms.SysInterval(i); interval < minInterval {
			minInterval = interval
			best = i
		}
	}
	return best
}

func (ms MultiSample) SysInterval(i int) time.Duration {
	pre, post := ms.SysPrePost(i)
	return post.Sub(pre)
}

// SysPrePost returns the system times immediately before and after the i-th PHC sample.
func (ms MultiSample) SysPrePost(i int) (pre, post time.Time) {
	phcLen := len(ms.PHC)
	switch len(ms.Sys) {
	case phcLen * 2:
		return ms.Sys[i*2], ms.Sys[i*2+1]
	case phcLen + 1:
		return ms.Sys[i], ms.Sys[i+1]
	case 1:
		return ms.Sys[0], ms.Sys[0]
	default:
		panic("invalid MultiSample sys length")
	}
}

// SysAverage returns the average of the system times for the i-th PHC sample.
func (ms MultiSample) SysAverage(i int) time.Time {
	pre, post := ms.SysPrePost(i)
	return pre.Add(post.Sub(pre) / 2)
}