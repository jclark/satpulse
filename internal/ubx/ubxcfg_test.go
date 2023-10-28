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
		t.Errorf("Inconsistent: %v", bad)
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
