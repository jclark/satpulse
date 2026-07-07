package as

import (
	"math"
	"testing"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/asbin"
)

func TestSignals(t *testing.T) {
	tests := []struct {
		name       string
		requested  gpsprot.SignalSet
		expectMask asbin.CfgNavSatMask
	}{
		{
			// the L2C/L1C bits are written but the silicon clamps them
			// away; the verify poll reveals the achieved set
			name:       "gps_only",
			requested:  gpsprot.SigSetGPS,
			expectMask: 0x201, // GPS L1CA + L5
		},
		{
			name:       "everything",
			requested:  gpsprot.SigSetAll,
			expectMask: tau1201Cap,
		},
		{
			name:       "gps_l1ca_alone",
			requested:  gpsprot.SignalSetOf(gpsprot.SigGPSL1CA),
			expectMask: 0x1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rcvr := &testReceiver{monVer: tau1201Ver(),
				navSat: &asbin.CfgNavSat{EnableMask: tau1201Cap}, sigCap: tau1201Cap}
			cp := probe(t, rcvr)
			target := &gpsprot.ConfigTarget{}
			target.Props.SetSignalsEnabled(tc.requested)
			cfg, errCount := configure(t, cp, rcvr, target)
			if errCount != 0 {
				t.Errorf("ErrorCount = %d, want 0", errCount)
			}
			if rcvr.navSat.EnableMask != tc.expectMask {
				t.Errorf("receiver mask = %#x, want %#x", rcvr.navSat.EnableMask, tc.expectMask)
			}
			got, ok := cfg.ConfigProps().GetSignalsEnabled()
			if !ok || got != navSatToSignals(tc.expectMask) {
				t.Errorf("achieved = %v/%v, want %v", got, ok, navSatToSignals(tc.expectMask))
			}
		})
	}
}

func TestSignalsReadback(t *testing.T) {
	// TAU1302 as-found: a selection narrower than its capability
	rcvr := &testReceiver{monVer: tau1201Ver(),
		navSat: &asbin.CfgNavSat{EnableMask: 0x40415}, sigCap: 0x8042437}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{Get: gpsprot.PropIDSignalsEnabled}
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	want := gpsprot.SignalSetOf(gpsprot.SigGPSL1CA, gpsprot.SigGPSL2C,
		gpsprot.SigBDSB1I, gpsprot.SigBDSB2I, gpsprot.SigGALE1)
	if got, ok := cfg.ConfigProps().GetSignalsEnabled(); !ok || got != want {
		t.Errorf("signals = %v/%v, want %v", got, ok, want)
	}
}

func TestMinElev(t *testing.T) {
	// as-found: trk 1 deg, navi 5 deg (all three units)
	asFound := asbin.CfgElev{TrkMask: 0.017453, NaviMask: 0.087266}
	rcvr := &testReceiver{monVer: tau1201Ver(), elev: asFound}
	cp := probe(t, rcvr)
	target := &gpsprot.ConfigTarget{}
	target.Props.SetMinElevation(gpsprot.DegreesFromFloat(10))
	cfg, errCount := configure(t, cp, rcvr, target)
	if errCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", errCount)
	}
	if rcvr.elev.TrkMask != asFound.TrkMask {
		t.Errorf("TrkMask = %v, want preserved %v", rcvr.elev.TrkMask, asFound.TrkMask)
	}
	if deg := float64(rcvr.elev.NaviMask) * 180 / math.Pi; deg < 9.99 || deg > 10.01 {
		t.Errorf("NaviMask = %v deg, want 10", deg)
	}
	if e, ok := cfg.ConfigProps().GetMinElevation(); !ok || e.Degrees() < 9.99 || e.Degrees() > 10.01 {
		t.Errorf("achieved minElev = %v/%v, want ~10 deg", e, ok)
	}
}
