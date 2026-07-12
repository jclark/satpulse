package as

import (
	"reflect"
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

// tau951mPps returns the TAU951M's as-found CFG-PPS values, factory
// 530 ns offset included.
func tau951mPps() *asbin.CfgPps {
	return &asbin.CfgPps{
		Period:    1000000,
		Offset:    530,
		DutyCycle: 10000,
		Polarity:  asbin.CfgPpsPolarityFallingEdge,
		GPIO:      13,
		Sync:      asbin.CfgPpsSyncOnlyWithFix,
	}
}

func TestTimePulseSet(t *testing.T) {
	tests := []struct {
		name       string
		width      time.Duration
		expectPps  asbin.CfgPps
		expectWant gpsprot.TimePulse
	}{
		{
			// setting the width merges into the readback: period, GPIO,
			// and the factory offset are preserved
			name:  "width_100ms",
			width: 100 * time.Millisecond,
			expectPps: asbin.CfgPps{Period: 1000000, Offset: 530, DutyCycle: 100000,
				Polarity: asbin.CfgPpsPolarityFallingEdge, GPIO: 13,
				Sync: asbin.CfgPpsSyncOnlyWithFix},
		},
		{
			name:  "width_zero_disables",
			width: 0,
			expectPps: asbin.CfgPps{Period: 1000000, Offset: 530, DutyCycle: 0,
				Polarity: asbin.CfgPpsPolarityFallingEdge, GPIO: 13,
				Sync: asbin.CfgPpsSyncOnlyWithFix},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver(), pps: tau951mPps()}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Props.SetTimePulseWidth(tc.width)
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if !reflect.DeepEqual(*rcvr.pps, tc.expectPps) {
				t.Errorf("receiver PPS\ngot  %+v\nwant %+v", *rcvr.pps, tc.expectPps)
			}
			props := cfg.ConfigProps()
			w, ok := props.GetTimePulseWidth()
			if !ok || w != tc.width {
				t.Errorf("achieved width = %v/%v, want %v", w, ok, tc.width)
			}
		})
	}
}

func TestTimePulseZeroDutyIgnored(t *testing.T) {
	// TAU1302 defect: a zero duty cycle can be acknowledged without
	// being stored. Per the semantics the achieved value is what the
	// receiver ACCEPTED - the set exchange reports width 0, with no
	// verify roundtrip to work around the defect. The discrepancy is
	// receiver characterization, visible on a later readback.
	rcvr := &testReceiver{monVer: tau1201Ver(), pps: tau951mPps(), ignoreZeroDuty: true}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetTimePulseWidth(0)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if w, ok := cfg.ConfigProps().GetTimePulseWidth(); !ok || w != 0 {
		t.Errorf("achieved width = %v/%v, want the accepted 0", w, ok)
	}
	if rcvr.pps.DutyCycle == 0 {
		t.Error("fake stored the zero duty; it must model the defect")
	}
}

func TestTimePulseReadback(t *testing.T) {
	rcvr := &testReceiver{monVer: tau1201Ver(), pps: tau951mPps()}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{Get: tpProps}
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	props := cfg.ConfigProps()
	if w, ok := props.GetTimePulseWidth(); !ok || w != 10*time.Millisecond {
		t.Errorf("width = %v/%v, want 10ms", w, ok)
	}
	if p, ok := props.GetTimePulsePeriod(); !ok || p != time.Second {
		t.Errorf("period = %v/%v, want 1s", p, ok)
	}
	if r, ok := props.GetTimePulsePolarityRising(); !ok || r {
		t.Errorf("polarityRising = %v/%v, want false", r, ok)
	}
	if l, ok := props.GetTimePulseOnlyWhenLocked(); !ok || !l {
		t.Errorf("onlyWhenLocked = %v/%v, want true", l, ok)
	}
	if a, ok := props.GetTimePulseAlignToGNSS(); !ok || !a {
		t.Errorf("alignToGNSS = %v/%v, want true (the pulse is always GNSS-aligned)", a, ok)
	}
}

func TestTimePulseWidthSubMsPeriod(t *testing.T) {
	// The width readback must not truncate away periods under 1 ms.
	cfg := newConfigurator(&gpsprot.ConfigTarget{}, tau1201Ver(), newRateEstimator())
	cfg.pps = &asbin.CfgPps{Period: 100, DutyCycle: 500000}
	if w, ok := cfg.ConfigProps().GetTimePulseWidth(); !ok || w != 50*time.Microsecond {
		t.Errorf("width = %v/%v, want 50us", w, ok)
	}
}

func TestTimePulseAbsent(t *testing.T) {
	// A receiver without CFG-PPS is silent to the poll: the property
	// shows as absence - nothing set, nothing reported, no error.
	rcvr := &testReceiver{monVer: tau1201Ver()}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetTimePulseWidth(100 * time.Millisecond)
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if _, ok := cfg.ConfigProps().GetTimePulseWidth(); ok {
		t.Error("width reported for a receiver without CFG-PPS")
	}
}
