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
		expected HWConfig
	}{
		{
			name: "lbe1421-cm4-m8t",
			toml: `[phc]
freqOffset = 2033.295
drift = -163.074
whiteNoise = 21.040
flickerNoise = 8.725

[[phc.sinusoid]]
amp = 4.782
period = 2986.500

[gps]
jitter = 6.087
sawtooth.amp = 13.778

[[gps.sinusoid]]
period = 2986.000
amp = 24.547

[[gps.sinusoid]]
period = 5972.000
amp = 12.353

[[gps.sinusoid]]
period = 1493.000
amp = 4.488
`,
			expected: HWConfig{
				PHC: PHCConfig{
					FreqOffset:   2033.295,
					Drift:        -163.074,
					WhiteNoise:   21.040,
					FlickerNoise: 8.725,
					Sinusoid: []Sinusoid{
						{Amp: 4.782, Period: 2986.500},
					},
				},
				GPS: GPSConfig{
					Jitter: 6.087,
					Sawtooth: SawtoothConfig{
						Amp:       13.778, // from TOML
						PhaseInit: 0.5,    // from DefaultConfig
						InternalClock: SineFMConfig{
							Amp:    2.0,    // from DefaultConfig
							Period: 600.0,  // from DefaultConfig
							Phase:  1.0472, // from DefaultConfig (π/3)
						},
					},
					Sinusoid: []Sinusoid{
						{Period: 2986.000, Amp: 24.547},
						{Period: 5972.000, Amp: 12.353},
						{Period: 1493.000, Amp: 4.488},
					},
				},
			},
		},
		{
			name: "lbe1421-timehat-f9t",
			toml: `[phc]
freqOffset = -21.719
drift = -8.677
whiteNoise = 5.816
flickerNoise = 1.337

[[phc.sinusoid]]
amp = 0.249
period = 26904.000

[gps]
jitter = 2.276
sawtooth.amp = 5.951

[[gps.sinusoid]]
period = 2989.333
amp = 20.463

[[gps.sinusoid]]
period = 2690.400
amp = 17.349

[[gps.sinusoid]]
period = 3363.000
amp = 16.005
`,
			expected: HWConfig{
				PHC: PHCConfig{
					FreqOffset:   -21.719,
					Drift:        -8.677,
					WhiteNoise:   5.816,
					FlickerNoise: 1.337,
					Sinusoid: []Sinusoid{
						{Amp: 0.249, Period: 26904.000},
					},
				},
				GPS: GPSConfig{
					Jitter: 2.276,
					Sawtooth: SawtoothConfig{
						Amp: 5.951,
						InternalClock: SineFMConfig{
							Amp:    2.0,
							Period: 600.0,
						},
					},
					Sinusoid: []Sinusoid{
						{Period: 2989.333, Amp: 20.463},
						{Period: 2690.400, Amp: 17.349},
						{Period: 3363.000, Amp: 16.005},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config HWConfig
			r := strings.NewReader(tt.toml)
			err := toml.NewDecoder(r).DisallowUnknownFields().Decode(&config)
			if err != nil {
				t.Fatalf("failed to decode: %v", err)
			}

			// Verify PHC config
			if config.PHC.FreqOffset != tt.expected.PHC.FreqOffset {
				t.Errorf("PHC.FreqOffset = %v, want %v", config.PHC.FreqOffset, tt.expected.PHC.FreqOffset)
			}
			if config.PHC.Drift != tt.expected.PHC.Drift {
				t.Errorf("PHC.Drift = %v, want %v", config.PHC.Drift, tt.expected.PHC.Drift)
			}
			if config.PHC.WhiteNoise != tt.expected.PHC.WhiteNoise {
				t.Errorf("PHC.WhiteNoise = %v, want %v", config.PHC.WhiteNoise, tt.expected.PHC.WhiteNoise)
			}
			if config.PHC.FlickerNoise != tt.expected.PHC.FlickerNoise {
				t.Errorf("PHC.FlickerNoise = %v, want %v", config.PHC.FlickerNoise, tt.expected.PHC.FlickerNoise)
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

	var config HWConfig
	r := strings.NewReader(tomlStr)
	err := toml.NewDecoder(r).DisallowUnknownFields().Decode(&config)
	if err == nil {
		t.Fatal("expected error for unknown field 'offset', got nil")
	}
	t.Logf("correctly rejected unknown field: %v", err)
}

func TestLoadHWConfigMergesDefaults(t *testing.T) {
	// Test that LoadHWConfig() merges TOML values with DefaultConfig()
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
	hw, err := LoadHWConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadHWConfig failed: %v", err)
	}

	// Verify user-specified values
	if hw.PHC.FreqOffset != 100.0 {
		t.Errorf("PHC.FreqOffset = %v, want 100.0", hw.PHC.FreqOffset)
	}
	if hw.GPS.Jitter != 5.0 {
		t.Errorf("GPS.Jitter = %v, want 5.0", hw.GPS.Jitter)
	}
	if hw.GPS.Sawtooth.Amp != 8.0 {
		t.Errorf("GPS.Sawtooth.Amp = %v, want 8.0", hw.GPS.Sawtooth.Amp)
	}

	// Verify defaults were merged for unspecified fields
	defaults := DefaultConfig()

	// PHC defaults for unspecified fields
	if hw.PHC.Drift != defaults.PHC.Drift {
		t.Errorf("PHC.Drift = %v, want %v (from defaults)", hw.PHC.Drift, defaults.PHC.Drift)
	}
	if hw.PHC.WhiteNoise != defaults.PHC.WhiteNoise {
		t.Errorf("PHC.WhiteNoise = %v, want %v (from defaults)", hw.PHC.WhiteNoise, defaults.PHC.WhiteNoise)
	}

	// Sawtooth defaults for unspecified fields
	if hw.GPS.Sawtooth.PhaseInit != defaults.GPS.Sawtooth.PhaseInit {
		t.Errorf("GPS.Sawtooth.PhaseInit = %v, want %v (from defaults)",
			hw.GPS.Sawtooth.PhaseInit, defaults.GPS.Sawtooth.PhaseInit)
	}
	if hw.GPS.Sawtooth.InternalClock.Amp != defaults.GPS.Sawtooth.InternalClock.Amp {
		t.Errorf("GPS.Sawtooth.InternalClock.Amp = %v, want %v (from defaults)",
			hw.GPS.Sawtooth.InternalClock.Amp, defaults.GPS.Sawtooth.InternalClock.Amp)
	}
	if hw.GPS.Sawtooth.InternalClock.Period != defaults.GPS.Sawtooth.InternalClock.Period {
		t.Errorf("GPS.Sawtooth.InternalClock.Period = %v, want %v (from defaults)",
			hw.GPS.Sawtooth.InternalClock.Period, defaults.GPS.Sawtooth.InternalClock.Period)
	}
	if hw.GPS.Sawtooth.InternalClock.Phase != defaults.GPS.Sawtooth.InternalClock.Phase {
		t.Errorf("GPS.Sawtooth.InternalClock.Phase = %v, want %v (from defaults)",
			hw.GPS.Sawtooth.InternalClock.Phase, defaults.GPS.Sawtooth.InternalClock.Phase)
	}
}
