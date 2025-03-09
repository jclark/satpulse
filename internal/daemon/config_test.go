package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
