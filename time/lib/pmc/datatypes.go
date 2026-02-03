package pmc

// This file  provides some data types.
// These are defined in Section 5.3 of the standard.

import (
	"fmt"
	"math"
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
	ClockClass              uint8 // in the PTP standard this is not an enumeration
	ClockAccuracy           ClockAccuracy
	OffsetScaledLogVariance uint16
}

type ClockAccuracy uint8

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
	ClockAccuracyWithin1ps ClockAccuracy = 0x17 + iota
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

type scaledTime struct {
	significand uint8
	exponent    uint8
}

var clockAccuracyWithin = []scaledTime{
	{1, 12},
	{25, 13},
	{10, 12},
	{25, 12},
	{100, 12},
	{250, 12},
	{1, 9},
	{25, 10},
	{10, 9},
	{25, 9},
	{100, 9},
	{250, 9},
	{1, 6},
	{25, 7},
	{10, 6},
	{25, 6},
	{100, 6},
	{250, 6},
	{1, 3},
	{25, 4},
	{10, 3},
	{25, 3},
	{100, 3},
	{250, 3},
	{1, 0},
	{10, 0},
}

// DurationToClockAccuracy converts a time.Duration to the appropriate ClockAccuracy value,
// rounding up to the next available accuracy level.
// Returns 0 for durations <= 0 or > 10s.
func DurationToClockAccuracy(d time.Duration) ClockAccuracy {
	if d <= 0 {
		return 0
	}
	nanos := d.Nanoseconds()
	for i, st := range clockAccuracyWithin {
		thresholdNanos := float64(st.significand) * float64(time.Second) * math.Pow10(-int(st.exponent))
		if float64(nanos) <= thresholdNanos {
			return ClockAccuracy(int(ClockAccuracyWithin1ps) + i)
		}
	}
	return 0
}

func (a ClockAccuracy) String() string {
	return fmt.Sprintf("0x%02X", uint8(a))
}

func (a ClockAccuracy) Description() string {
	if len(clockAccuracyWithin) != int(ClockAccuracyGreater10s-ClockAccuracyWithin1ps) {
		panic("clockAccuracyWithing array is not correct")
	}
	if a >= ClockAccuracyWithin1ps && a < ClockAccuracyGreater10s {
		st := clockAccuracyWithin[a-ClockAccuracyWithin1ps]
		units := []string{"s", "ms", "us", "ns", "ps"}
		exp := st.exponent
		if exp%3 != 0 {
			exp++
			return fmt.Sprintf("<=%.1f%s", float64(st.significand)/10, units[exp/3])
		}
		return fmt.Sprintf("<=%d%s", st.significand, units[exp/3])
	}
	switch a {
	case ClockAccuracyUnknown:
		return "unknown"
	case ClockAccuracyGreater10s:
		return ">10s"
	}
	return "reserved"
}

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

// AdevToOffsetScaledLogVariance converts Allan Deviation (dimensionless) to the
// IEEE 1588-2019 "offsetScaledLogVariance" (uint16).
//
// tau is the averaging time in seconds (standard PTP default is 1.0 second);
// it must be finite and positive.
// Allan deviaton is numerically in seconds when tau=1.0 (e.g. of the order of 1e-9 for nanosecond-level stability).
// This if for a non-dynamic estimate of offsetScaledLogVariance,
// so no hysteresis requirement is applied.
func AdevToOffsetScaledLogVariance(adev, tau float64) uint16 {
	if adev <= 0 || math.IsNaN(adev) || math.IsInf(adev, 0) {
		return OffsetScaledLogVarianceUnknown
	}
	if tau <= 0 || math.IsNaN(tau) || math.IsInf(tau, 0) {
		panic("invalid tau value")
	}
	// Calculate PTP Variance from adev and tau
	//    sigma_ptp^2 = (tau^2 / 3) * sigma_adev^2
	ptpVariance := (adev * adev) * (tau * tau) / 3.0

	// Handle Zero/Underflow (Log2(0) is -Inf)
	//    Spec Note 2 says min representable is ~2^-128.
	minVar := math.Pow(2, -128)
	if ptpVariance <= minVar {
		return 0x0000 // Minimum representable (Best possible quality)
	}

	// Log Base 2
	logVar := math.Log2(ptpVariance)

	// Scale by 2^8 (256)
	scaled := logVar * 256.0

	// Convert to an integer
	scaled = math.Round(scaled)

	// Apply 0x8000 offset
	offsetScaled := scaled + 0x8000

	// Clamp to range of uint16
	//    0xFFFF is use for both unknown and values too large to be represented.
	if offsetScaled >= 0xFFFF {
		return 0xFFFF
	}
	if offsetScaled < 0 {
		return 0x0000
	}

	return uint16(offsetScaled)
}