// Package kpps provides access to kernel pulse-per-second sources.
//
// Its data model follows RFC 2783, but the package does not implement every
// optional facility described by that API.
package kpps

import "time"

// Device describes a kernel PPS device as Linux registers it in sysfs.
type Device struct {
	Path       string // the device node, /dev/ppsN
	Name       string // the sysfs name attribute: the source name the driver registered
	SourcePath string // the sysfs path attribute: the source's device path, such as a tty; empty for most drivers
	Mode       Mode   // the sysfs mode attribute: the capabilities, as GetCap reports them
}

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
// mode that was in effect when the source last processed an event. Mode is
// zero until the first event, except on FreeBSD, where it is the mode in
// effect when the information was fetched. Assert and Clear are independent;
// they do not necessarily belong to the same pulse.
type Info struct {
	Assert Edge
	Clear  Edge
	Mode   Mode
}
