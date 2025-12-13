package syncsim

import (
	"os"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestLoadConfigFromTOML(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		expected Config
	}{
		{
			name: "i225.in.cm55",
			toml: `[phc]
freqOffset = 9117.374
whiteNoise = 6.942
flickerNoise = 10.171
randomWalk = 2.010

[[phc.sinusoid]]
period = 71603.000
amp = 512.414

[[phc.sinusoid]]
period = 107404.500
amp = 285.132

[[phc.sinusoid]]
period = 53702.250
amp = 243.339

[[phc.sinusoid]]
period = 26851.125
amp = 298.711
`,
			expected: Config{
				PHC: PHCConfig{
					FreqOffset:   9117.374,
					WhiteNoise:   6.942,
					FlickerNoise: 10.171,
					RandomWalk:   2.010,
					Sinusoid: []Sinusoid{
						{Period: 71603.000, Amp: 512.414},
						{Period: 107404.500, Amp: 285.132},
						{Period: 53702.250, Amp: 243.339},
						{Period: 26851.125, Amp: 298.711},
					},
				},
			},
		},
		{
			name: "i225-f9t0.out.sa22c",
			toml: `[phc]
freqOffset = -1007.333
whiteNoise = 8.092
flickerNoise = 7.584
randomWalk = 0.216

[[phc.sinusoid]]
period = 4303.000
amp = 252.321

[[phc.sinusoid]]
period = 5378.750
amp = 102.469

[[phc.sinusoid]]
period = 7171.667
amp = 70.754

[[phc.sinusoid]]
period = 3585.833
amp = 120.340

[[phc.sinusoid]]
period = 10757.500
amp = 26.012

[gps]
jitter = 0.195
sawtooth.amp = 7.855

[[gps.ar1]]
tau = 176.804
sigma = 2.319

[[gps.sinusoid]]
period = 4303.000
amp = 2.253
`,
			expected: Config{
				PHC: PHCConfig{
					FreqOffset:   -1007.333,
					WhiteNoise:   8.092,
					FlickerNoise: 7.584,
					RandomWalk:   0.216,
					Sinusoid: []Sinusoid{
						{Period: 4303.000, Amp: 252.321},
						{Period: 5378.750, Amp: 102.469},
						{Period: 7171.667, Amp: 70.754},
						{Period: 3585.833, Amp: 120.340},
						{Period: 10757.500, Amp: 26.012},
					},
				},
				GPS: GPSConfig{
					Jitter: 0.195,
					AR1: []AR1Config{
						{Tau: 176.804, Sigma: 2.319},
					},
					Sawtooth: SawtoothConfig{
						Amp: 7.855,
					},
					Sinusoid: []Sinusoid{
						{Period: 4303.000, Amp: 2.253},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config Config
			r := strings.NewReader(tt.toml)
			err := toml.NewDecoder(r).DisallowUnknownFields().Decode(&config)
			if err != nil {
				t.Fatalf("failed to decode: %v", err)
			}

			// Verify PHC config
			if config.PHC.FreqOffset != tt.expected.PHC.FreqOffset {
				t.Errorf("PHC.FreqOffset = %v, want %v", config.PHC.FreqOffset, tt.expected.PHC.FreqOffset)
			}
			if config.PHC.WhiteNoise != tt.expected.PHC.WhiteNoise {
				t.Errorf("PHC.WhiteNoise = %v, want %v", config.PHC.WhiteNoise, tt.expected.PHC.WhiteNoise)
			}
			if config.PHC.FlickerNoise != tt.expected.PHC.FlickerNoise {
				t.Errorf("PHC.FlickerNoise = %v, want %v", config.PHC.FlickerNoise, tt.expected.PHC.FlickerNoise)
			}
			if config.PHC.RandomWalk != tt.expected.PHC.RandomWalk {
				t.Errorf("PHC.RandomWalk = %v, want %v", config.PHC.RandomWalk, tt.expected.PHC.RandomWalk)
			}

			// Verify PHC sinusoid components
			if len(config.PHC.Sinusoid) != len(tt.expected.PHC.Sinusoid) {
				t.Fatalf("PHC.Sinusoid length = %d, want %d", len(config.PHC.Sinusoid), len(tt.expected.PHC.Sinusoid))
			}
			for i, s := range config.PHC.Sinusoid {
				if s.Amp != tt.expected.PHC.Sinusoid[i].Amp {
					t.Errorf("PHC.Sinusoid[%d].Amp = %v, want %v", i, s.Amp, tt.expected.PHC.Sinusoid[i].Amp)
				}
				if s.Period != tt.expected.PHC.Sinusoid[i].Period {
					t.Errorf("PHC.Sinusoid[%d].Period = %v, want %v", i, s.Period, tt.expected.PHC.Sinusoid[i].Period)
				}
			}

			// Verify GPS config
			if config.GPS.Jitter != tt.expected.GPS.Jitter {
				t.Errorf("GPS.Jitter = %v, want %v", config.GPS.Jitter, tt.expected.GPS.Jitter)
			}
			if len(config.GPS.AR1) != len(tt.expected.GPS.AR1) {
				t.Fatalf("GPS.AR1 length = %d, want %d", len(config.GPS.AR1), len(tt.expected.GPS.AR1))
			}
			for i, ar1 := range config.GPS.AR1 {
				if ar1.Tau != tt.expected.GPS.AR1[i].Tau {
					t.Errorf("GPS.AR1[%d].Tau = %v, want %v", i, ar1.Tau, tt.expected.GPS.AR1[i].Tau)
				}
				if ar1.Sigma != tt.expected.GPS.AR1[i].Sigma {
					t.Errorf("GPS.AR1[%d].Sigma = %v, want %v", i, ar1.Sigma, tt.expected.GPS.AR1[i].Sigma)
				}
			}
			if config.GPS.Sawtooth.Amp != tt.expected.GPS.Sawtooth.Amp {
				t.Errorf("GPS.Sawtooth.Amp = %v, want %v", config.GPS.Sawtooth.Amp, tt.expected.GPS.Sawtooth.Amp)
			}

			// Verify GPS sinusoid components
			if len(config.GPS.Sinusoid) != len(tt.expected.GPS.Sinusoid) {
				t.Fatalf("GPS.Sinusoid length = %d, want %d", len(config.GPS.Sinusoid), len(tt.expected.GPS.Sinusoid))
			}
			for i, s := range config.GPS.Sinusoid {
				if s.Period != tt.expected.GPS.Sinusoid[i].Period {
					t.Errorf("GPS.Sinusoid[%d].Period = %v, want %v", i, s.Period, tt.expected.GPS.Sinusoid[i].Period)
				}
				if s.Amp != tt.expected.GPS.Sinusoid[i].Amp {
					t.Errorf("GPS.Sinusoid[%d].Amp = %v, want %v", i, s.Amp, tt.expected.GPS.Sinusoid[i].Amp)
				}
			}

			// Verify CreateSimulator methods work
			phcSim := config.PHC.CreateSimulator()
			if phcSim == nil {
				t.Error("PHC.CreateSimulator() returned nil")
			}

			gpsSim := config.GPS.CreateSimulator()
			if gpsSim == nil {
				t.Error("GPS.CreateSimulator() returned nil")
			}
		})
	}
}

func TestInvalidFieldsRejected(t *testing.T) {
	tomlStr := `[gps]
offset = 123.456
jitter = 10.0`

	var config Config
	r := strings.NewReader(tomlStr)
	err := toml.NewDecoder(r).DisallowUnknownFields().Decode(&config)
	if err == nil {
		t.Fatal("expected error for unknown field 'offset', got nil")
	}
	t.Logf("correctly rejected unknown field: %v", err)
}

func TestLoadConfigMergesDefaults(t *testing.T) {
	// Test that LoadConfig() merges TOML values with DefaultConfig()
	// User specifies only sawtooth.amp, should get defaults for other fields

	tomlContent := `[phc]
freqOffset = 100.0

[gps]
jitter = 5.0
sawtooth.amp = 8.0
`

	// Write to temp file
	tmpfile, err := os.CreateTemp("", "hwconfig-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(tomlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Load config
	cfg := DefaultConfig()
	err = LoadConfig(tmpfile.Name(), &cfg)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify user-specified values
	if cfg.PHC.FreqOffset != 100.0 {
		t.Errorf("PHC.FreqOffset = %v, want 100.0", cfg.PHC.FreqOffset)
	}
	if cfg.GPS.Jitter != 5.0 {
		t.Errorf("GPS.Jitter = %v, want 5.0", cfg.GPS.Jitter)
	}
	if cfg.GPS.Sawtooth.Amp != 8.0 {
		t.Errorf("GPS.Sawtooth.Amp = %v, want 8.0", cfg.GPS.Sawtooth.Amp)
	}

	// Verify defaults: PHC noise parameters are zero by default
	if cfg.PHC.Drift != 0 {
		t.Errorf("PHC.Drift = %v, want 0 (zero noise default)", cfg.PHC.Drift)
	}
	if cfg.PHC.WhiteNoise != 0 {
		t.Errorf("PHC.WhiteNoise = %v, want 0 (zero noise default)", cfg.PHC.WhiteNoise)
	}

	// Sawtooth defaults for unspecified fields (sensible non-zero defaults)
	defaults := DefaultConfig()
	if cfg.GPS.Sawtooth.PhaseInit != defaults.GPS.Sawtooth.PhaseInit {
		t.Errorf("GPS.Sawtooth.PhaseInit = %v, want %v (from defaults)",
			cfg.GPS.Sawtooth.PhaseInit, defaults.GPS.Sawtooth.PhaseInit)
	}
	if cfg.GPS.Sawtooth.InternalClock.Amp != defaults.GPS.Sawtooth.InternalClock.Amp {
		t.Errorf("GPS.Sawtooth.InternalClock.Amp = %v, want %v (from defaults)",
			cfg.GPS.Sawtooth.InternalClock.Amp, defaults.GPS.Sawtooth.InternalClock.Amp)
	}
	if cfg.GPS.Sawtooth.InternalClock.Period != defaults.GPS.Sawtooth.InternalClock.Period {
		t.Errorf("GPS.Sawtooth.InternalClock.Period = %v, want %v (from defaults)",
			cfg.GPS.Sawtooth.InternalClock.Period, defaults.GPS.Sawtooth.InternalClock.Period)
	}
	if cfg.GPS.Sawtooth.InternalClock.PhaseInit != defaults.GPS.Sawtooth.InternalClock.PhaseInit {
		t.Errorf("GPS.Sawtooth.InternalClock.PhaseInit = %v, want %v (from defaults)",
			cfg.GPS.Sawtooth.InternalClock.PhaseInit, defaults.GPS.Sawtooth.InternalClock.PhaseInit)
	}

	// Verify Fault defaults to empty (no faults)
	if len(cfg.Fault.Outlier.Times) != 0 {
		t.Errorf("Fault.Outlier.Times = %v, want empty", cfg.Fault.Outlier.Times)
	}
	if cfg.Fault.Outlier.Offset != 0 {
		t.Errorf("Fault.Outlier.Offset = %v, want 0 (user must specify)", cfg.Fault.Outlier.Offset)
	}
}
