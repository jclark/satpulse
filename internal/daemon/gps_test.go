package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/jclark/satpulse/internal/gpsprot"
)

func TestGPSConfig(t *testing.T) {
	cfgStr := `[gps]
	config = true
	antennaCableLength = 20
	GNSS="galileo"
	fixedPosAcc = 5
	fixedPosECEF = [3978578.17, -8652.15, 4968410.94]`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	target, err := cfg.GPS.target(9600, false)
	if err != nil {
		t.Fatal(err)
	}
	cp := &target.Props
	delay, _ := cp.GetAntennaCableDelay()
	if (delay - time.Nanosecond*20*5).Abs() > time.Nanosecond {
		t.Errorf("antennaCableDelay: got %v, want about 100ns", delay)
	}
	gnss, _ := cp.GetTimeGNSS()
	if gnss != gpsprot.GAL {
		t.Errorf("GNSS: got %v, want %v", gnss, gpsprot.GAL)
	}
	acc, _ := cp.GetFixedPosAcc()
	if acc != gpsprot.Meter*5 {
		t.Errorf("fixedPosAcc: got %v, want 5m", acc)
	}
	ecef, _ := cp.GetFixedPosECEF()
	if ecef.String() != "3978578.17,-8652.15,4968410.94" {
		t.Errorf("fixedPosECEF: got %v, want 3978578.17,-8652.15,4968410.94", ecef)
	}
}

func TestSatellitesInfo(t *testing.T) {
	cfgStr := `[gps]
	satellitesOutput = true`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GPS.SatellitesOutput == nil || *cfg.GPS.SatellitesOutput != true {
		t.Errorf("SatellitesOutputs: got %v, want true", cfg.GPS.SatellitesOutput)
	}
	cfgStr = `[gps]`
	r = strings.NewReader(cfgStr)
	cfg, err = readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GPS.SatellitesOutput != nil {
		t.Errorf("SatellitesOutputs: got %v, want nil", cfg.GPS.SatellitesOutput)
	}
	opt := cfg.GPS.satsMsg(38400, true)
	if opt.Get() != gpsprot.SatsMsgSV {
		t.Errorf("setSatellitesMsg: got %v, want SatellitesMsgSV", opt.Get())
	}
}

func TestFixedPosECEFTimeMode(t *testing.T) {
	// Test that when fixedPosECEF is set, time mode is set to Fixed
	// and survey parameters are not set
	cfgStr := `[gps]
		config = true
		stationary = true
		resurvey = true
		surveyTime = 3000
		surveyAcc = 10
		fixedPosECEF = [3978578.17, -8652.15, 4968410.94]
		fixedPosAcc = 5`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	target, err := cfg.GPS.target(9600, false)
	if err != nil {
		t.Fatal(err)
	}
	cp := &target.Props
	opts := &target.Opts

	// Check that time mode is set to Fixed
	mode, ok := cp.GetTimeMode()
	if !ok {
		t.Error("timeMode should be set")
	}
	if mode != gpsprot.TimeModeFixed {
		t.Errorf("timeMode: got %v, want TimeModeFixed", mode)
	}

	// Check that survey parameters are not set when fixedPosECEF is provided
	if opts.Survey.When != 0 {
		t.Errorf("Survey.When: got %v, want 0 (no survey when fixed position is set)", opts.Survey.When)
	}
	if opts.Survey.MinDur != 0 {
		t.Errorf("Survey.MinDur: got %v, want 0", opts.Survey.MinDur)
	}
	if opts.Survey.AccLimit != 0 {
		t.Errorf("Survey.AccLimit: got %v, want 0", opts.Survey.AccLimit)
	}

	// Verify fixed position is set correctly
	ecef, ok := cp.GetFixedPosECEF()
	if !ok {
		t.Error("fixedPosECEF should be set")
	}
	if ecef.String() != "3978578.17,-8652.15,4968410.94" {
		t.Errorf("fixedPosECEF: got %v, want 3978578.17,-8652.15,4968410.94", ecef)
	}
}
