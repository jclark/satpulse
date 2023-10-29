package ubx

import (
	"testing"

	"github.com/jclark/gps4ptp/internal/gpsmsg"
	"github.com/jclark/gps4ptp/internal/ubx/bin"
)

func TestTp5(t *testing.T) {
	raw := &RawConfig{tp5: new(bin.CfgTp5)}
	raw.tp5.Flags |= bin.CfgTp5IsLength

	cfg := &gpsmsg.Config{}
	cfg.SetSane()

	raw.tp5 = raw.changeTp5(cfg)

	ncfg := gpsmsg.Config{}
	raw.cookTp5(&ncfg)
	bad := cfg.Inconsistent(&ncfg)
	if !bad.IsEmpty() {
		t.Errorf("tp5 change failed: %v", bad)
	}

	rep := raw.changeTp5(cfg)

	if rep != nil {
		t.Errorf("repeated changeTp5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeTp5(new(gpsmsg.Config))
	if rep != nil {
		t.Errorf("changeTp5 with nothing wasn't a no-op: %v", rep)
	}
}


func TestNav5(t *testing.T) {
	raw := &RawConfig{nav5: new(bin.CfgNav5)}

	cfg := &gpsmsg.Config{}
	cfg.SetSane()

	raw.nav5 = raw.changeNav5(cfg)

	ncfg := gpsmsg.Config{}
	raw.cookNav5(&ncfg)
	bad := cfg.Inconsistent(&ncfg)
	if !bad.IsEmpty() {
		t.Errorf("nav5 change failed: %v", bad)
	}

	rep := raw.changeNav5(cfg)

	if rep != nil {
		t.Errorf("repeated changeNav5 wasn't a no-op: %v", rep)
	}

	rep = raw.changeNav5(new(gpsmsg.Config))
	if rep != nil {
		t.Errorf("changeNav5 with nothing wasn't a no-op: %v", rep)
	}
}

func TestRate(t *testing.T) {
	raw := &RawConfig{rate: new(bin.CfgRate)}
	raw.rate.NavRate = 1
	ver := new(Version)

	cfg := &gpsmsg.Config{}
	cfg.SetSane()

	raw.rate = raw.changeRate(cfg, ver)

	ncfg := gpsmsg.Config{}
	raw.cookRate(&ncfg, ver)
	bad := cfg.Inconsistent(&ncfg)
	if !bad.IsEmpty() {
		t.Errorf("rate change failed: %v", bad)
	}

	rep := raw.changeRate(cfg, ver)

	if rep != nil {
		t.Errorf("repeated changeRate wasn't a no-op: %v", rep)
	}

	rep = raw.changeRate(new(gpsmsg.Config), ver)
	if rep != nil {
		t.Errorf("changeRate with nothing wasn't a no-op: %v", rep)
	}
}
