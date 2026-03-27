package ubx

import (
	"testing"
	"time"

	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/lib/ubxbin"
)

func TestTp5(t *testing.T) {
	raw := &CfgOld{tp5: new(ubxbin.CfgTp5)}
	raw.tp5.Flags |= ubxbin.CfgTp5IsLength

	cp := &gpsprot.ConfigProps{}
	cp.SetPPS(100 * time.Millisecond)

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

func TestNav5MinElev(t *testing.T) {
	raw := &CfgOld{nav5: new(ubxbin.CfgNav5)}
	ver := &Version{}

	target := &gpsprot.ConfigTarget{}
	target.Props.SetMinElevation(10 * gpsprot.Degrees)

	raw.nav5 = raw.changeNav5(target, ver)
	if raw.nav5 == nil {
		t.Fatal("changeNav5 returned nil")
	}

	ncp := gpsprot.ConfigProps{}
	raw.cookNav5(&ncp, ver)
	bad := target.Props.Inconsistent(&ncp)
	if !bad.IsEmpty() {
		t.Errorf("nav5 MinElev change failed: %v", bad)
	}

	rep := raw.changeNav5(target, ver)
	if rep != nil {
		t.Errorf("repeated changeNav5 wasn't a no-op: %v", rep)
	}

	// out-of-range value should be skipped (no change)
	outOfRange := &gpsprot.ConfigTarget{}
	outOfRange.Props.SetMinElevation(200 * gpsprot.Degrees)
	skip := raw.changeNav5(outOfRange, ver)
	if skip != nil {
		t.Errorf("out-of-range MinElev should be skipped, got %v", skip)
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

	target := &gpsprot.ConfigTarget{}
	ver := &Version{}
	cp := &target.Props
	cp.SetPPS(100 * time.Millisecond)

	raw.nav5 = raw.changeNav5(target, ver)

	ncp := gpsprot.ConfigProps{}
	raw.cookNav5(&ncp, ver)
	bad := cp.Inconsistent(&ncp)
	if !bad.IsEmpty() {
		t.Errorf("nav5 change failed: %v", bad)
	}

	rep := raw.changeNav5(target, ver)

	if rep != nil {
		t.Errorf("repeated changeNav5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeNav5(new(gpsprot.ConfigTarget), ver)
	if rep != nil {
		t.Errorf("changeNav5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestConfiguratorSane(t *testing.T) {
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(100 * time.Millisecond)
	target.Get = gpsprot.PropIDTimePulseWidth
	testConfigurator(t, newLegacyReceiver(), target)
}

func TestConfiguratorGPS(t *testing.T) {
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(100 * time.Millisecond)
	target.Props.SetTimeGNSS(gpsprot.GPS)
	testConfigurator(t, newLegacyReceiver(), target)
}

func TestConfiguratorGalileo(t *testing.T) {
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(100 * time.Millisecond)
	target.Props.SetTimeGNSS(gpsprot.GAL)
	rcvr := newLegacyReceiver()
	rcvr.raw.gnss.Blocks[0].GNSSID = ubxbin.GAL
	testConfigurator(t, rcvr, target)
}

// TestConfiguratorGalileoFromGPS tests changing time GNSS from GPS to Galileo.
// This reproduces #244: when TP5 grid bits already have GPS set,
// changing to Galileo must clear the old grid before setting the new one.
func TestConfiguratorGalileoFromGPS(t *testing.T) {
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(100 * time.Millisecond)
	target.Props.SetTimeGNSS(gpsprot.GAL)
	rcvr := newLegacyReceiver()
	rcvr.raw.tp5.Flags |= ubxbin.CfgTp5GridGPS
	rcvr.raw.gnss.Blocks[0].GNSSID = ubxbin.GAL
	testConfigurator(t, rcvr, target)
}

// TestConfiguratorTimeGNSSOnly tests --time-gnss without --pps.
// When TP5 is already aligned to GPS, --time-gnss gal alone should
// change the grid to Galileo without requiring --pps.
func TestConfiguratorTimeGNSSOnly(t *testing.T) {
	target := gpsprot.NewConfigTarget()
	target.Props.SetTimeGNSS(gpsprot.GAL)
	rcvr := newLegacyReceiver()
	rcvr.raw.tp5.Flags |= ubxbin.CfgTp5GridGPS
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
	c := testConfiguratorRecover(t, ubxbin.CfgMsgID)
	if c == nil {
		t.Fatal("configurator is nil")
	}
	if c.raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA == 0 {
		t.Errorf("expected NMEA to be enabled, but it wasn't")
	}
}

func TestConfiguratorRecover2(t *testing.T) {
	c := testConfiguratorRecover(t, ubxbin.CfgRateID)
	if c == nil {
		t.Fatal("configurator is nil")
	}
	// in this case we got far enough to enable the time message, so we don't need to disable NMEA
	if c.raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA != 0 {
		t.Errorf("expected NMEA not to be enabled, but it was")
	}
}

func testConfiguratorRecover(t *testing.T, nakMsgID ubxbin.MsgID) *Configurator {
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(100 * time.Millisecond)
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

func testConfiguratorAbort(t *testing.T, abortMsgID ubxbin.MsgID) *Configurator {
	target := gpsprot.NewConfigTarget()
	target.Props.SetPPS(100 * time.Millisecond)
	target.Opts.NMEAMsg.Set(gpsprot.NMEAMsgNone)
	target.Opts.PVTMsg.Set(gpsprot.PVTMsgTimePulse | gpsprot.PVTMsgTAI)
	// we can't use legacyReceiver here because it doesn't support the UBX-CFG-GNSS
	rcvr := newGpsReceiver(&m8tVersion)
	rcvr.abortMsgID = abortMsgID
	c, naks, err := runConfiguration(rcvr, target)
	if err != nil {
		t.Errorf("unexpected error from runConfiguration: %v", err)
	}
	// No NAKs expected since abort simulates timeout/corruption, not negative acknowledgment
	if len(naks) != 0 {
		t.Errorf("expected 0 naks, got %d", len(naks))
	}
	return c
}

func TestConfiguratorAbort1(t *testing.T) {
	c := testConfiguratorAbort(t, ubxbin.CfgMsgID)
	if c.raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA == 0 {
		t.Errorf("expected NMEA to be enabled after abort during message config, but it wasn't")
	}
}

func TestConfiguratorAbort2(t *testing.T) {
	c := testConfiguratorAbort(t, ubxbin.CfgRateID)
	// in this case we got far enough to enable the time message, so we don't need to re-enable NMEA
	if c.raw.prt.OutProtoMask&ubxbin.CfgPrtProtoNMEA != 0 {
		t.Errorf("expected NMEA not to be enabled after abort during rate config, but it was")
	}
}
