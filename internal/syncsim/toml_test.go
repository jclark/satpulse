package syncsim

import (
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
freqDrift = -163.074
whiteFreqNoise = 21.040
flickerFreqNoise = 8.725
sineFM.amp = 4.782
sineFM.period = 2986.500

[gps]
jitter = 6.087
sawtooth.period = 1.999
sawtooth.amp = 13.778

[[gps.periodic]]
period = 2986.000
amp = 24.547

[[gps.periodic]]
period = 5972.000
amp = 12.353

[[gps.periodic]]
period = 1493.000
amp = 4.488
`,
			expected: HWConfig{
				PHC: PHCConfig{
					FreqOffset:       2033.295,
					FreqDrift:        -163.074,
					WhiteFreqNoise:   21.040,
					FlickerFreqNoise: 8.725,
					SineFM: SineFMConfig{
						Amp:    4.782,
						Period: 2986.500,
					},
				},
				GPS: GPSConfig{
					Jitter: 6.087,
					Sawtooth: SawtoothConfig{
						Period: 1.999,
						Amp:    13.778,
					},
					Periodic: []PeriodicComponent{
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
freqDrift = -8.677
whiteFreqNoise = 5.816
flickerFreqNoise = 1.337
sineFM.amp = 0.249
sineFM.period = 26904.000

[gps]
jitter = 2.276
sawtooth.period = 3.998
sawtooth.amp = 5.951

[[gps.periodic]]
period = 2989.333
amp = 20.463

[[gps.periodic]]
period = 2690.400
amp = 17.349

[[gps.periodic]]
period = 3363.000
amp = 16.005
`,
			expected: HWConfig{
				PHC: PHCConfig{
					FreqOffset:       -21.719,
					FreqDrift:        -8.677,
					WhiteFreqNoise:   5.816,
					FlickerFreqNoise: 1.337,
					SineFM: SineFMConfig{
						Amp:    0.249,
						Period: 26904.000,
					},
				},
				GPS: GPSConfig{
					Jitter: 2.276,
					Sawtooth: SawtoothConfig{
						Period: 3.998,
						Amp:    5.951,
					},
					Periodic: []PeriodicComponent{
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
			if config.PHC.FreqDrift != tt.expected.PHC.FreqDrift {
				t.Errorf("PHC.FreqDrift = %v, want %v", config.PHC.FreqDrift, tt.expected.PHC.FreqDrift)
			}
			if config.PHC.WhiteFreqNoise != tt.expected.PHC.WhiteFreqNoise {
				t.Errorf("PHC.WhiteFreqNoise = %v, want %v", config.PHC.WhiteFreqNoise, tt.expected.PHC.WhiteFreqNoise)
			}
			if config.PHC.FlickerFreqNoise != tt.expected.PHC.FlickerFreqNoise {
				t.Errorf("PHC.FlickerFreqNoise = %v, want %v", config.PHC.FlickerFreqNoise, tt.expected.PHC.FlickerFreqNoise)
			}
			if config.PHC.SineFM.Amp != tt.expected.PHC.SineFM.Amp {
				t.Errorf("PHC.SineFM.Amp = %v, want %v", config.PHC.SineFM.Amp, tt.expected.PHC.SineFM.Amp)
			}
			if config.PHC.SineFM.Period != tt.expected.PHC.SineFM.Period {
				t.Errorf("PHC.SineFM.Period = %v, want %v", config.PHC.SineFM.Period, tt.expected.PHC.SineFM.Period)
			}

			// Verify GPS config
			if config.GPS.Jitter != tt.expected.GPS.Jitter {
				t.Errorf("GPS.Jitter = %v, want %v", config.GPS.Jitter, tt.expected.GPS.Jitter)
			}
			if config.GPS.Sawtooth.Period != tt.expected.GPS.Sawtooth.Period {
				t.Errorf("GPS.Sawtooth.Period = %v, want %v", config.GPS.Sawtooth.Period, tt.expected.GPS.Sawtooth.Period)
			}
			if config.GPS.Sawtooth.Amp != tt.expected.GPS.Sawtooth.Amp {
				t.Errorf("GPS.Sawtooth.Amp = %v, want %v", config.GPS.Sawtooth.Amp, tt.expected.GPS.Sawtooth.Amp)
			}

			// Verify periodic components
			if len(config.GPS.Periodic) != len(tt.expected.GPS.Periodic) {
				t.Fatalf("GPS.Periodic length = %d, want %d", len(config.GPS.Periodic), len(tt.expected.GPS.Periodic))
			}
			for i, p := range config.GPS.Periodic {
				if p.Period != tt.expected.GPS.Periodic[i].Period {
					t.Errorf("GPS.Periodic[%d].Period = %v, want %v", i, p.Period, tt.expected.GPS.Periodic[i].Period)
				}
				if p.Amp != tt.expected.GPS.Periodic[i].Amp {
					t.Errorf("GPS.Periodic[%d].Amp = %v, want %v", i, p.Amp, tt.expected.GPS.Periodic[i].Amp)
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
