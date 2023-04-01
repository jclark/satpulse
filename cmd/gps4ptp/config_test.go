package main

import (
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	_, err := loadConfig(defaultConfigFile)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPTPConfig(t *testing.T) {
	cfgStr := `[ptp]
	udsAddress = "/tmp/ptp4l"
	domainNumber = 1
	majorSdoId = 2
	minorSdoId = 12
	impl = "none"`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PTP.UDSAddress != "/tmp/ptp4l" || cfg.PTP.DomainNumber != 1 || cfg.PTP.MajorSdoID != 2 || cfg.PTP.MinorSdoID != 12 || cfg.PTP.Impl != PTPImplNone {
		t.Fatal("PTP config not parsed correctly")
	}
}
