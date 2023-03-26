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

const OffsetScaledLogVarianceUnknown = 0xffff

const (
	ClockClassSyncPrimaryRef = 6
	ClockClassHoldover       = 7
	ClockClassDegradedA      = 52
	ClockClassDegradedB      = 187
	ClockClassDefault        = 248
	ClockClassSlaveOnly      = 255
)

const (
	ClockAccuracyWithin1ps = 0x17 + iota
	ClockAccuracyWithin2point5ps
	ClockAccuracyWithin10ps
	ClockAccuracyWithin25ps
	ClockAccuracyWithin100ps
	ClockAccuracyWithin250ps
	ClockAccuracyWithin1ns
	ClockAccuracyWithin2point5ns
	ClockAccuracyWithin10ns
	ClockAccuracyWithin25ns
	ClockAccuracyWithin100ns
	ClockAccuracyWithin250ns
	ClockAccuracyWithin1us
	ClockAccuracyWithin2point5us
	ClockAccuracyWithin10us
	ClockAccuracyWithin25us
	ClockAccuracyWithin100us
	ClockAccuracyWithin250us
	ClockAccuracyWithin1ms
	ClockAccuracyWithin2point5ms
	ClockAccuracyWithin10ms
	ClockAccuracyWithin25ms
	ClockAccuracyWithin100ms
	ClockAccuracyWithin250ms
	ClockAccuracyWithin1s
	ClockAccuracyWithin10s
	ClockAccuracyGreater10s
	ClockAccuracyUnknown = 0xFE
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
