package pmc

import "testing"

func TestClockAccuracyDescription(t *testing.T) {
	for _, tc := range []struct {
		ca ClockAccuracy
		s  string
	}{
		{ClockAccuracyWithin1ps, "<=1ps"},
		{ClockAccuracyWithin2point5ps, "<=2.5ps"},
		{ClockAccuracyWithin10ps, "<=10ps"},
		{ClockAccuracyWithin25ps, "<=25ps"},
		{ClockAccuracyWithin100ps, "<=100ps"},
		{ClockAccuracyWithin250ps, "<=250ps"},
		{ClockAccuracyWithin1ns, "<=1ns"},
		{ClockAccuracyWithin2point5ns, "<=2.5ns"},
		{ClockAccuracyWithin10ns, "<=10ns"},
		{ClockAccuracyWithin25ns, "<=25ns"},
		{ClockAccuracyWithin100ns, "<=100ns"},
		{ClockAccuracyWithin250ns, "<=250ns"},
		{ClockAccuracyWithin1us, "<=1us"},
		{ClockAccuracyWithin2point5us, "<=2.5us"},
		{ClockAccuracyWithin10us, "<=10us"},
		{ClockAccuracyWithin25us, "<=25us"},
		{ClockAccuracyWithin100us, "<=100us"},
		{ClockAccuracyWithin250us, "<=250us"},
		{ClockAccuracyWithin1ms, "<=1ms"},
		{ClockAccuracyWithin2point5ms, "<=2.5ms"},
		{ClockAccuracyWithin10ms, "<=10ms"},
		{ClockAccuracyWithin25ms, "<=25ms"},
		{ClockAccuracyWithin100ms, "<=100ms"},
		{ClockAccuracyWithin250ms, "<=250ms"},
		{ClockAccuracyWithin1s, "<=1s"},
		{ClockAccuracyWithin10s, "<=10s"},
		{ClockAccuracyGreater10s, ">10s"},
		{ClockAccuracyUnknown, "unknown"},
		{ClockAccuracy(0), "reserved"},
	} {
		desc := tc.ca.Description()
		if desc != tc.s {
			t.Errorf("ClockAccuracy.Description() = %q, want %q", desc, tc.s)
		}
	}

}
