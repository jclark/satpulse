package gpsmsg

import (
	"testing"
	"time"
)

func TestConfigDB(t *testing.T) {
	// Create a new ConfigDB
	cfg := Config{}

	// Set some values using CfgKey
	CfgSolutionPeriod.Set(&cfg, 10*time.Second)
	CfgTimePulseWidth.Set(&cfg, 1*time.Millisecond)
	CfgTimePulseAlignToGNSS.Set(&cfg, true)

	keys := []CfgKey{CfgSolutionPeriod, CfgTimePulseWidth, CfgTimePulseAlignToGNSS}
	nonKeys := []CfgKey{CfgTimePulsePeriod, CfgStationary}

	// Get the values using CfgKey
	solutionPeriod, ok := CfgSolutionPeriod.Get(&cfg)
	if !ok {
		t.Errorf("expected CfgSolutionPeriod to be set")
	}
	if solutionPeriod != 10*time.Second {
		t.Errorf("expected CfgSolutionPeriod to be 10s, got %v", solutionPeriod)
	}

	timePulseWidth, ok := CfgTimePulseWidth.Get(&cfg)
	if !ok {
		t.Errorf("expected CfgTimePulseWidth to be set")
	}
	if timePulseWidth != 1*time.Millisecond {
		t.Errorf("expected CfgTimePulseWidth to be 1ms, got %v", timePulseWidth)
	}

	timePulsePeriod, ok := CfgTimePulsePeriod.Get(&cfg)
	if ok {
		t.Errorf("expected CfgTimePulsePeriod not to be set")
	}
	if timePulsePeriod != 0 {
		t.Errorf("expected CfgTimePulsePeriod to be 0, got %v", timePulsePeriod)
	}

	timePulseGNSS, ok := CfgTimePulseAlignToGNSS.Get(&cfg)
	if !ok {
		t.Errorf("expected CfgTimePulseAlignGNSS to be set")
	}
	if timePulseGNSS != true {
		t.Error("expected CfgTimePulseGNSS to be true, got false")
	}

	for _, k := range keys {
		if !cfg.Contains(k) {
			t.Errorf("expected %v to be set", k)
		}
	}
	for _, k := range nonKeys {
		if cfg.Contains(k) {
			t.Errorf("expected %v not to be set", k)
		}
	}

}
