// Package kpps provides access to kernel pulse-per-second sources.
//
// Its data model follows RFC 2783, but the package does not implement every
// optional facility described by that API.
package kpps

import (
	"errors"
	"time"
)

// ErrNotSupported is returned when kernel PPS is unavailable on the current
// platform.
var ErrNotSupported = errors.New("kernel PPS not supported on this platform")

// Mode is a set of RFC 2783 mode bits. GetCap can return mode bits for which
// this package does not define a name.
type Mode uint32

const (
	// CaptureAssert is the mode bit for assert-edge capture.
	CaptureAssert Mode = 0x01
	// CaptureClear is the mode bit for clear-edge capture.
	CaptureClear Mode = 0x02
)

// Edge describes the most recently captured edge of one polarity.
type Edge struct {
	T        time.Time
	Sequence uint32
}

// Info contains the most recently captured assert and clear edges and the
// mode in effect when they were fetched. Assert and Clear are independent;
// they do not necessarily belong to the same pulse.
type Info struct {
	Assert Edge
	Clear  Edge
	Mode   Mode
}
