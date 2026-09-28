package daemon

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/time/lib/pmc"
)

const configsDir = "../../../configs"

func TestLoadConfig(t *testing.T) {
	cfgFiles, err := os.ReadDir(configsDir)
	if err != nil {
		t.Fatal(err)
	}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	count := 0
	for _, f := range cfgFiles {
		if strings.ToLower(filepath.Ext(f.Name())) != ".toml" {
			continue
		}
		path := filepath.Join(configsDir, f.Name())
		count++
		cfg, _, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("error loading %s: %v", path, err)
		}
		if err := cfg.Validate(lg); err != nil {
			t.Fatalf("error validating %s: %v", path, err)
		}
	}
	if count == 0 {
		t.Fatalf("no config files found in %s", configsDir)
	} else {
		t.Logf("tested %d config files from %s", count, configsDir)
	}
}

// TestConfigValidate covers the daemon-level config validations
// that bridge top-level users with per-feature references (currently
// Ntrip mountpoint auth.users).  Internal ntrip validation is
// covered in gps/app/ntrip; this test focuses on cross-table checks
// that only the daemon Config can perform.
func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		toml      string
		expectErr bool
	}{
		{
			name: "no users, no ntrip auth",
			toml: `
[[ntrip.mountpoint]]
name = "BKK"
`,
		},
		{
			name: "valid user + mountpoint auth.users",
			toml: `
[[user]]
name = "rover1"
password = "p1"

[[ntrip.mountpoint]]
name = "BKK"
auth.users = ["rover1"]
`,
		},
		{
			name: "user without name",
			toml: `
[[user]]
password = "p"
`,
			expectErr: true,
		},
		{
			name: "duplicate user names",
			toml: `
[[user]]
name = "rover1"
[[user]]
name = "rover1"
`,
			expectErr: true,
		},
		{
			name: "mountpoint references undefined user",
			toml: `
[[user]]
name = "rover1"

[[ntrip.mountpoint]]
name = "BKK"
auth.users = ["ghost"]
`,
			expectErr: true,
		},
		{
			name: "auth.anyUser does not need any top-level user",
			toml: `
[[ntrip.mountpoint]]
name = "BKK"
auth.anyUser = true
`,
		},
	}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := readConfig(strings.NewReader(tc.toml))
			if err != nil {
				t.Fatalf("readConfig: %v", err)
			}
			err = cfg.Validate(lg)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStreamPullConfig(t *testing.T) {
	cfgStr := `[stream.pull]
ntrip.address = "caster.example.com:2101"
ntrip.mountpoint = "RTCM"
ntrip.username = "u"
ntrip.password = "p"
ntrip.nmeaSend = true`
	cfg, err := readConfig(strings.NewReader(cfgStr))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Stream.Pull.Ntrip == nil {
		t.Fatalf("expected stream.pull.ntrip to be set, got %+v", cfg.Stream)
	}
	n := cfg.Stream.Pull.Ntrip
	if n.Address != "caster.example.com:2101" || n.Mountpoint != "RTCM" || n.Username != "u" || n.Password != "p" || !n.NMEASend {
		t.Errorf("ntrip = %+v", n)
	}
}

func TestSerialPPSConfig(t *testing.T) {
	tests := []struct {
		pin       string
		expect    gpsio.SerialPin
		expectErr bool
	}{
		{pin: "cts", expect: gpsio.SerialPinCTS},
		{pin: "CTS", expect: gpsio.SerialPinCTS},
		{pin: "dcd", expect: gpsio.SerialPinDCD},
		{pin: "dsr", expect: gpsio.SerialPinDSR},
		{pin: "ri", expect: gpsio.SerialPinRI},
		{pin: "Ri", expect: gpsio.SerialPinRI},
		{pin: "", expectErr: true},
		{pin: "rts", expectErr: true},
	}
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range tests {
		t.Run(tc.pin, func(t *testing.T) {
			cfg, err := readConfig(strings.NewReader("[serial.pps]\npin = \"" + tc.pin + "\""))
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("readConfig: %v", err)
			}
			if cfg.Serial.PPS == nil || cfg.Serial.PPS.Pin != tc.expect {
				t.Fatalf("serial PPS config = %+v", cfg.Serial.PPS)
			}
			if err := cfg.Validate(lg); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestSerialPPSConfigRequiresPin(t *testing.T) {
	cfg, err := readConfig(strings.NewReader("[serial.pps]\ninvertPolarity = true"))
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	err = cfg.Validate(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "pps.pin in the [serial] table must be specified") {
		t.Fatalf("Validate error = %v, want missing pps.pin", err)
	}
}

func TestSerialPPSConfigInvertPolarity(t *testing.T) {
	cfg, err := readConfig(strings.NewReader("[serial.pps]\npin = \"cts\"\ninvertPolarity = true"))
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if cfg.Serial.PPS == nil || !cfg.Serial.PPS.InvertPolarity {
		t.Fatalf("serial PPS config = %+v, want inverted polarity", cfg.Serial.PPS)
	}
}

func TestSerialPPSConfigRejectsPHC(t *testing.T) {
	cfg, err := readConfig(strings.NewReader(`
[serial.pps]
pin = "cts"

[phc]
interface = "eth0"
`))
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Validate(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "pps.pin in the [serial] table cannot be used with interface in the [phc] table") {
		t.Fatalf("Validate error = %v, want serial PPS/PHC conflict", err)
	}
}

func TestSerialPPSKernelMethodRequiresDCD(t *testing.T) {
	cfg, err := readConfig(strings.NewReader(`
[serial.pps]
pin = "cts"

[sample.serial.pps]
method = "kernel"
`))
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Validate(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), `requires pps.pin = "DCD"`) {
		t.Fatalf("Validate error = %v, want the kernel method to require DCD", err)
	}
	cfg.Serial.PPS.Pin = gpsio.SerialPinDCD
	if err := cfg.Validate(slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("Validate with dcd: %v", err)
	}
}

func TestSerialPPSSampleConfig(t *testing.T) {
	cfg := defaultConfig()
	if got := cfg.Sample.Serial.PPS.Method; got != 0 {
		t.Errorf("default method = %v, want automatic selection", got)
	}
	if got := cfg.Sample.Serial.PPS.DelayUncertainty; got != 0.005 {
		t.Errorf("default delayUncertainty = %v, want 0.005", got)
	}
	if got := cfg.Sample.Serial.PPS.MaxDelay; got != 0.8 {
		t.Errorf("default maxDelay = %v, want 0.8", got)
	}
	if got := cfg.Sample.Serial.PPS.MaxWakeupLatency; got != nil {
		t.Errorf("default maxWakeupLatency = %v, want nil", got)
	}

	cfg, err := readConfig(strings.NewReader(`
[sample.serial.pps]
method = "kernel"
delayUncertainty = 0.01
maxDelay = 0.7
maxWakeupLatency = 10e-6
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sample.Serial.PPS.DelayUncertainty; got != 0.01 {
		t.Errorf("configured delayUncertainty = %v, want 0.01", got)
	}
	if got := cfg.Sample.Serial.PPS.MaxDelay; got != 0.7 {
		t.Errorf("configured maxDelay = %v, want 0.7", got)
	}
	if got := cfg.Sample.Serial.PPS.Method; got != gpsio.PPSMethodKernel {
		t.Errorf("configured method = %v, want %v", got, gpsio.PPSMethodKernel)
	}
	if got := cfg.Sample.Serial.PPS.MaxWakeupLatency; got == nil || *got != 10e-6 {
		t.Errorf("configured maxWakeupLatency = %v, want 10e-6", got)
	}
	if err := cfg.Validate(slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	cfg, err = readConfig(strings.NewReader(`
[sample.serial.pps]
maxWakeupLatency = 0
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Sample.Serial.PPS.MaxWakeupLatency; got == nil || *got != 0 {
		t.Errorf("explicit zero maxWakeupLatency = %v, want pointer to zero", got)
	}
}

func TestSerialPPSSampleConfigRejectsInvalidMethod(t *testing.T) {
	_, err := readConfig(strings.NewReader(`
[sample.serial.pps]
method = "sideband"
`))
	if err == nil {
		t.Fatal("readConfig succeeded with invalid PPS method")
	}
}

func TestSerialPPSSampleConfigRejectsWideInterval(t *testing.T) {
	cfg, err := readConfig(strings.NewReader(`
[sample.serial.pps]
delayUncertainty = 0.2
maxDelay = 0.8
`))
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Validate(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "delayUncertainty + maxDelay") {
		t.Fatalf("Validate error = %v, want interval-width error", err)
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

func TestNTPShmConfig(t *testing.T) {
	cfgStr := `[ntp]
shm.segment = 0
shm.precision = -23`
	cfg, err := readConfig(strings.NewReader(cfgStr))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NTP.SHM == nil || cfg.NTP.SHM.Segment == nil || *cfg.NTP.SHM.Segment != 0 {
		t.Fatalf("NTP SHM segment = %+v, want explicit 0", cfg.NTP.SHM)
	}
	if cfg.NTP.SHM.Precision == nil || *cfg.NTP.SHM.Precision != -23 {
		t.Fatalf("NTP SHM precision = %+v, want -23", cfg.NTP.SHM.Precision)
	}
}

func TestConfigSHMFixedPrecision(t *testing.T) {
	cfg := defaultConfig()
	cfg.PHC.Interface = "eth0"
	if got := cfg.shmFixedPrecision(); got != nil {
		t.Fatalf("PHC-mode default SHM fixed precision = %v, want nil", *got)
	}
	cfg.PHC.Interface = ""
	got := cfg.shmFixedPrecision()
	if got == nil || *got != serialSHMPrecision {
		t.Fatalf("serial-mode SHM fixed precision = %v, want %d", got, serialSHMPrecision)
	}
	cfg.Serial.PPS = &SerialPPSConfig{Pin: gpsio.SerialPinCTS}
	got = cfg.shmFixedPrecision()
	if got == nil || *got != serialPPSSHMPrecision {
		t.Fatalf("serial-PPS SHM fixed precision = %v, want %d", got, serialPPSSHMPrecision)
	}
	cfg.Serial.PPS = nil
	cfg, err := readConfig(strings.NewReader(`[ntp]
shm.segment = 2
shm.precision = -23`))
	if err != nil {
		t.Fatal(err)
	}
	got = cfg.shmFixedPrecision()
	if got == nil || *got != -23 {
		t.Fatalf("configured serial-mode SHM fixed precision = %v, want -23", got)
	}
	cfg.PHC.Interface = "eth0"
	got = cfg.shmFixedPrecision()
	if got == nil || *got != -23 {
		t.Fatalf("configured PHC-mode SHM fixed precision = %v, want -23", got)
	}
	cfg.NTP.SHM.Precision = nil
	got = cfg.shmFixedPrecision()
	if got != nil {
		t.Fatalf("default PHC precision = %v, want nil", *got)
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
