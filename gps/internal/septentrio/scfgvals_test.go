package septentrio

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
)

// TestConvertTimePulse checks the OnlyWhenLocked readback: a nonzero
// MaxHoldover stops the pulses after holdover, except with RxClock, where the
// receiver ignores MaxHoldover and the pulses continue indefinitely.
func TestConvertTimePulse(t *testing.T) {
	tests := []struct {
		name   string
		pps    ppsParams
		expect gpsprot.TimePulse
	}{
		{
			name:   "GPS with holdover limit",
			pps:    ppsParams{interval: "sec1", polarity: "Low2High", timeScale: "GPS", maxHoldover: 60, widthMs: 5},
			expect: gpsprot.TimePulse{Width: 5 * time.Millisecond, Period: time.Second, AlignToGNSS: true, OnlyWhenLocked: true, PolarityRising: true},
		},
		{
			name:   "GPS without holdover limit",
			pps:    ppsParams{interval: "sec1", polarity: "Low2High", timeScale: "GPS", maxHoldover: 0, widthMs: 5},
			expect: gpsprot.TimePulse{Width: 5 * time.Millisecond, Period: time.Second, AlignToGNSS: true, OnlyWhenLocked: false, PolarityRising: true},
		},
		{
			name:   "sub-microsecond width rounds to the nearest nanosecond",
			pps:    ppsParams{interval: "sec1", polarity: "Low2High", timeScale: "GPS", maxHoldover: 0, widthMs: 0.000249},
			expect: gpsprot.TimePulse{Width: 249 * time.Nanosecond, Period: time.Second, AlignToGNSS: true, OnlyWhenLocked: false, PolarityRising: true},
		},
		{
			name:   "RxClock ignores holdover limit",
			pps:    ppsParams{interval: "sec1", polarity: "Low2High", timeScale: "RxClock", maxHoldover: 60, widthMs: 5},
			expect: gpsprot.TimePulse{Width: 5 * time.Millisecond, Period: time.Second, AlignToGNSS: false, OnlyWhenLocked: false, PolarityRising: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			np := &nativeProps{pps: &tc.pps}
			props := &gpsprot.ConfigProps{}
			np.convertTimePulse(props)
			got, ok := props.GetTimePulse()
			if !ok {
				t.Fatalf("TimePulse not set")
			}
			if !reflect.DeepEqual(got, tc.expect) {
				t.Errorf("got  %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

// TestParseSatelliteTracking checks the constellations read from a
// per-satellite list, including a malformed list with an empty element.
func TestParseSatelliteTracking(t *testing.T) {
	tests := []struct {
		name   string
		state  string
		expect gpsprot.GNSSSet
	}{
		{
			name:   "satellites of two constellations",
			state:  "SatelliteTracking, G01+G02+E01",
			expect: gpsprot.GNSSSetOf(gpsprot.GPS) | gpsprot.GNSSSetOf(gpsprot.GAL),
		},
		{
			name:   "none",
			state:  "SatelliteTracking, none",
			expect: 0,
		},
		{
			name:   "empty element",
			state:  "SatelliteTracking, G01++C03",
			expect: gpsprot.GNSSSetOf(gpsprot.GPS) | gpsprot.GNSSSetOf(gpsprot.BDS),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var np nativeProps
			np.parseSatelliteTracking(&Reply{States: []string{tc.state}})
			if np.satGNSS == nil || *np.satGNSS != tc.expect {
				t.Fatalf("got %v, want %v", np.satGNSS, tc.expect)
			}
		})
	}
}
