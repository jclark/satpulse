package ubx

import (
	"testing"

	"github.com/jclark/satpulse/internal/gpsprot"
	ubxbin "github.com/jclark/satpulse/internal/ubx/bin"
)

func TestTp5(t *testing.T) {
	raw := &CfgOld{tp5: new(ubxbin.CfgTp5)}
	raw.tp5.Flags |= ubxbin.CfgTp5IsLength

	cp := &gpsprot.ConfigProps{}
	cp.SetPPS()

	raw.tp5 = raw.changeTp5(cp)

	ncp := gpsprot.ConfigProps{}
	raw.cookTp5(&ncp)
	bad := cp.Inconsistent(&ncp)
	if !bad.IsEmpty() {
		t.Errorf("tp5 change failed: %v", bad)
	}

	rep := raw.changeTp5(cp)

	if rep != nil {
		t.Errorf("repeated changeTp5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeTp5(new(gpsprot.ConfigProps))
	if rep != nil {
		t.Errorf("changeTp5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestChangeTp5GNSS(t *testing.T) {
	// Create a new RawConfig and ConfigProps
	raw := CfgOld{tp5: new(ubxbin.CfgTp5)}
	cp := gpsprot.ConfigProps{}

	// Call changeTp5GNSS with the empty RawConfig and ConfigProps
	gnss := raw.changeTp5GNSS(&cp)

	// Check that the result is gpsmsg.GPS
	if gnss != gpsprot.GPS {
		t.Errorf("expected gpsmsg.GPS, got %v", gnss)
	}
}

func TestNav5(t *testing.T) {
	raw := &CfgOld{nav5: new(ubxbin.CfgNav5)}

	cp := &gpsprot.ConfigProps{}
	cp.SetPPS()

	raw.nav5 = raw.changeNav5(cp)

	ncp := gpsprot.ConfigProps{}
	raw.cookNav5(&ncp)
	bad := cp.Inconsistent(&ncp)
	if !bad.IsEmpty() {
		t.Errorf("nav5 change failed: %v", bad)
	}

	rep := raw.changeNav5(cp)

	if rep != nil {
		t.Errorf("repeated changeNav5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeNav5(new(gpsprot.ConfigProps))
	if rep != nil {
		t.Errorf("changeNav5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestRate(t *testing.T) {
	raw := &CfgOld{rate: new(ubxbin.CfgRate)}
	raw.rate.NavRate = 1
	ver := new(Version)

	cp := &gpsprot.ConfigProps{}
	cp.SetPPS()

	raw.rate = raw.changeRate(cp, ver)

	ncp := gpsprot.ConfigProps{}
	raw.cookRate(&ncp, ver)
	bad := cp.Inconsistent(&ncp)
	if !bad.IsEmpty() {
		t.Errorf("rate change failed: %v", bad)
	}

	rep := raw.changeRate(cp, ver)

	if rep != nil {
		t.Errorf("repeated changeRate wasn't a no-op: %v", rep)
	}

	rep = raw.changeRate(new(gpsprot.ConfigProps), ver)
	if rep != nil {
		t.Errorf("changeRate with nothing wasn't a no-op: %v", rep)
	}
}

func TestConfiguratorSane(t *testing.T) {
	target := gpsprot.NewConfigTarget(false)
	target.Props.SetPPS()
	target.Get = gpsprot.PropIDTimePulseWidth
	testConfigurator(t, newLegacyReceiver(), target)
}

func TestConfiguratorGPS(t *testing.T) {
	target := gpsprot.NewConfigTarget(false)
	target.Props.SetPPS()
	target.Props.SetPrimaryGNSS(gpsprot.GPS)
	testConfigurator(t, newLegacyReceiver(), target)
}

func TestConfiguratorGalileo(t *testing.T) {
	target := gpsprot.NewConfigTarget(false)
	target.Props.SetPPS()
	target.Props.SetPrimaryGNSS(gpsprot.GAL)
	rcvr := newLegacyReceiver()
	rcvr.raw.gnss.Blocks[0].GNSSID = ubxbin.GAL
	testConfigurator(t, rcvr, target)
}

func testConfigurator(t *testing.T, rcvr *gpsReceiver, target *gpsprot.ConfigTarget) {
	c, naks, err := runConfiguration(rcvr, target)
	if err != nil {
		t.Fatalf("unexpected error from runConfiguration: %v", err)
	}
	if len(naks) > 0 {
		t.Errorf("unexpected naks: %v", naks)
	}
	result := c.ConfigProps()

	bad := target.Props.Inconsistent(result)
	if !bad.IsEmpty() {
		t.Errorf("final configuration is inconsistent: %v", bad)
	}
	missing := result.Missing(&target.Props)
	if !missing.IsEmpty() {
		t.Errorf("final configuration is missing: %v", missing)
	}
}

func TestConfiguratorRecover1(t *testing.T) {
	c := testConfiguratorRecover(t, ubxbin.CfgGNSSID)
	if c.raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA == 0 {
		t.Errorf("expected NMEA to be enabled, but it wasn't")
	}
}

func TestConfiguratorRecover2(t *testing.T) {
	c := testConfiguratorRecover(t, ubxbin.CfgRateID)
	// in this case we got far enough to enable the time message, so we don't need to disable NMEA
	if c.raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA != 0 {
		t.Errorf("expected NMEA not to be enabled, but it was")
	}
}

func testConfiguratorRecover(t *testing.T, nakMsgID ubxbin.MsgID) *Configurator {
	target := gpsprot.NewConfigTarget(false)
	target.Props.SetPPS()
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgNone)
	target.Opts.PVTMsg.Set(gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI)
	// we can't use legacyReceiver here because it doesn't support the UBX-CFG-GNSS
	rcvr := newGpsReceiver(&m8tVersion)
	rcvr.nakPollMsgID = nakMsgID
	c, naks, err := runConfiguration(rcvr, target)
	if err != nil {
		t.Errorf("unexpected error from runConfiguration: %v", err)
	}
	if len(naks) != 1 {
		t.Errorf("expected 1 nak, got %d", len(naks))
	}
	return c
}

func TestDivModRound(t *testing.T) {
	testCases := []struct {
		x, y, q int64
	}{
		{500, 100, 5},
		{550, 100, 6},
		{449, 100, 4},
		{-500, 100, -5},
		{-550, 100, -6},
		{-449, 100, -4},
		{17, 10, 2},
		{-17, 10, -2},
		{17, 1000, 0},
		{-17, 1000, 0},
		{1005, 1000, 1},
		{-1005, 1000, -1},
		{13, 10, 1},
		{15, 10, 2},
		{18, 10, 2},
		{-13, 10, -1},
		{-15, 10, -2},
		{-18, 10, -2},
	}

	for _, tc := range testCases {
		q, r := divModRound(tc.x, tc.y)
		if q != tc.q {
			t.Errorf("divModRound(%d, %d) = (%d, %d), want quotient %d",
				tc.x, tc.y, q, r, tc.q)
		}
		if tc.x != q*tc.y+r {
			t.Errorf("divModRound(%d, %d) = (%d, %d), does not satisfy x = quotient*y + remainder",
				tc.x, tc.y, q, r)
		}
	}
}
