package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jclark/satpulse/internal/pmc"
)

const configsDir = "../../configs"

func TestLoadConfig(t *testing.T) {
	cfgFiles, err := os.ReadDir(configsDir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, f := range cfgFiles {
		if strings.ToLower(filepath.Ext(f.Name())) != ".toml" {
			continue
		}
		path := filepath.Join(configsDir, f.Name())
		count++
		_, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("error loading %s: %v", path, err)
		}
	}
	if count == 0 {
		t.Fatalf("no config files found in %s", configsDir)
	} else {
		t.Logf("tested %d config files from %s", count, configsDir)
	}
}

func TestPTPConfig(t *testing.T) {
	cfgStr := `[ptp]
	ptp4l.udsAddress = "/tmp/ptp4l"
	domainNumber = 1
	clockAccuracy = 20
	majorSdoId = 2
	minorSdoId = 12`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PTP.PTP4L.UDSAddress != "/tmp/ptp4l" || cfg.PTP.DomainNumber != 1 || cfg.PTP.MajorSdoID != 2 || cfg.PTP.MinorSdoID != 12 || cfg.PTP.ClockAccuracy != 20 {
		t.Fatal("PTP config not parsed correctly")
	}
}

func TestPTPConfigClockQuality(t *testing.T) {
	tests := []struct {
		name          string
		clockAccuracy int
		wantErr       bool
	}{
		{"valid 100ns", 100, false},
		{"valid 1ns", 1, false},
		{"valid 10s", 10_000_000_000, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"too large", 20_000_000_000, true}, // > 10s
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &PTPConfig{ClockAccuracy: tt.clockAccuracy}
			_, err := cfg.ClockQuality()
			if (err != nil) != tt.wantErr {
				t.Errorf("ClockQuality() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
	// Test the returned values for a valid config
	cfg := &PTPConfig{ClockAccuracy: 100} // 100ns
	cq, err := cfg.ClockQuality()
	if err != nil {
		t.Fatal(err)
	}
	if cq.ClockClass != pmc.ClockClassSyncPrimaryRef {
		t.Errorf("ClockClass = %d, want %d", cq.ClockClass, pmc.ClockClassSyncPrimaryRef)
	}
	if cq.ClockAccuracy != pmc.ClockAccuracyWithin100ns {
		t.Errorf("ClockAccuracy = %v, want %v", cq.ClockAccuracy, pmc.ClockAccuracyWithin100ns)
	}
	if cq.OffsetScaledLogVariance != pmc.OffsetScaledLogVarianceUnknown {
		t.Errorf("OffsetScaledLogVariance = %d, want %d", cq.OffsetScaledLogVariance, pmc.OffsetScaledLogVarianceUnknown)
	}
}
