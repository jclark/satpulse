package pmc

// This file  provides some data types.
// These are defined in Section 5.3 of the standard.

import (
	"fmt"
	"time"
)

type ClockIdentity [8]byte

func AnyClockIdentity() ClockIdentity {
	return ClockIdentity([8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
}

type PortIdentity struct {
	ClockIdentity ClockIdentity
	PortNumber    uint16
}

func AnyPortIdentity() PortIdentity {
	return PortIdentity{AnyClockIdentity(), 0xffff}
}

type ClockQuality struct {
	ClockClass              uint8
	ClockAccuracy           uint8
	OffsetScaledLogVariance uint16
}

const (
	ClockClassSyncPrimaryRef = 6
	ClockClassHoldover       = 7
	ClockClassDegradedA      = 52
	ClockClassDegradedB      = 187
	ClockClassDefault        = 248
	ClockClassSlaveOnly      = 255
)

type TimeInterval int64

func (ti TimeInterval) Duration() time.Duration {
	scaledNanos := int64(ti)
	nanos := scaledNanos >> 16
	frac := scaledNanos & 0xffff
	if frac > 0x8000 {
		nanos++
	}
	return time.Duration(nanos)
}

func (ti TimeInterval) Nanos() float64 {
	return float64(int64(ti)) / 65536.0
}

func (ti TimeInterval) String() string {
	return fmt.Sprintf("%0.1f", ti.Nanos())
}
