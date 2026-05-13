package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jclark/satpulse/time/lib/pmc"
)

const configsDir = "../../../configs"

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
		_, _, err := LoadConfig(path)
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

func TestStreamPullConfig(t *testing.T) {
	cfgStr := `[stream.pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
ntrip.username = "u"
ntrip.password = "p"`
	cfg, err := readConfig(strings.NewReader(cfgStr))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Stream.Pull == nil || cfg.Stream.Pull.Ntrip == nil {
		t.Fatalf("expected stream.pull.ntrip to be set, got %+v", cfg.Stream)
	}
	n := cfg.Stream.Pull.Ntrip
	if n.Address != "caster.example.com:2101" || n.Mountpoint != "RTCM" || n.Username != "u" || n.Password != "p" {
		t.Errorf("ntrip = %+v", n)
	}
}

func TestPTPConfig(t *testing.T) {
	cfgStr := `[ptp]
	ptp4l.udsAddress = "/tmp/ptp4l"
	domainNumber = 1
	clockAccuracy = 20
	majorSdoId = 2
	minorSdoId = 12
	offsetScaledLogVariance = 0x8000`
	r := strings.NewReader(cfgStr)
	cfg, err := readConfig(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PTP.PTP4L.UDSAddress != "/tmp/ptp4l" || cfg.PTP.DomainNumber != 1 || cfg.PTP.MajorSdoID != 2 || cfg.PTP.MinorSdoID != 12 || cfg.PTP.ClockAccuracy != 20 {
		t.Fatal("PTP config not parsed correctly")
	}
	if cfg.PTP.OffsetScaledLogVariance != 0x8000 {
		t.Fatalf("OffsetScaledLogVariance = %d, expect %d", cfg.PTP.OffsetScaledLogVariance, 0x8000)
	}
}

func TestPTPConfigClockQuality(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*PTPConfig)
		expect    pmc.ClockQuality
		expectErr bool
	}{
		{
			name:   "defaults",
			modify: func(cfg *PTPConfig) {},
			expect: pmc.ClockQuality{
				ClockClass:              pmc.ClockClassSyncPrimaryRef,
				ClockAccuracy:           pmc.ClockAccuracyWithin250ns,
				OffsetScaledLogVariance: pmc.OffsetScaledLogVarianceUnknown,
			},
		},
		{
			name:   "100ns accuracy",
			modify: func(cfg *PTPConfig) { cfg.ClockAccuracy = 100 },
			expect: pmc.ClockQuality{
				ClockClass:              pmc.ClockClassSyncPrimaryRef,
				ClockAccuracy:           pmc.ClockAccuracyWithin100ns,
				OffsetScaledLogVariance: pmc.OffsetScaledLogVarianceUnknown,
			},
		},
		{
			name:      "zero accuracy",
			modify:    func(cfg *PTPConfig) { cfg.ClockAccuracy = 0 },
			expectErr: true,
		},
		{
			name:      "negative accuracy",
			modify:    func(cfg *PTPConfig) { cfg.ClockAccuracy = -1 },
			expectErr: true,
		},
		{
			name:      "too large accuracy",
			modify:    func(cfg *PTPConfig) { cfg.ClockAccuracy = 20_000_000_000 },
			expectErr: true,
		},
		{
			name:   "explicit offsetScaledLogVariance",
			modify: func(cfg *PTPConfig) { cfg.OffsetScaledLogVariance = 0x8000 },
			expect: pmc.ClockQuality{
				ClockClass:              pmc.ClockClassSyncPrimaryRef,
				ClockAccuracy:           pmc.ClockAccuracyWithin250ns,
				OffsetScaledLogVariance: 0x8000,
			},
		},
		{
			name:   "allanDeviation",
			modify: func(cfg *PTPConfig) { cfg.AllanDeviation = 1e-9 },
			expect: pmc.ClockQuality{
				ClockClass:              pmc.ClockClassSyncPrimaryRef,
				ClockAccuracy:           pmc.ClockAccuracyWithin250ns,
				OffsetScaledLogVariance: pmc.AdevToOffsetScaledLogVariance(1e-9, 1.0),
			},
		},
		{
			name: "both offsetScaledLogVariance and allanDeviation",
			modify: func(cfg *PTPConfig) {
				cfg.OffsetScaledLogVariance = 0x8000
				cfg.AllanDeviation = 1e-9
			},
			expectErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig().PTP
			tt.modify(&cfg)
			got, err := cfg.ClockQuality()
			if (err != nil) != tt.expectErr {
				t.Fatalf("ClockQuality() error = %v, expectErr %v", err, tt.expectErr)
			}
			if !tt.expectErr && got != tt.expect {
				t.Errorf("ClockQuality() = %+v, expect %+v", got, tt.expect)
			}
		})
	}
}
