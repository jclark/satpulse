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
