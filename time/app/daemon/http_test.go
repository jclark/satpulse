package daemon

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jclark/satpulse/time/lib/sse"
)

func TestConfigHTTPFeatureChecks(t *testing.T) {
	tests := []struct {
		name           string
		http           []HTTPConfig
		wantGUI        bool
		wantMetrics    bool
		wantPosition   bool
		wantSatellites bool
	}{
		{
			name: "none",
		},
		{
			name:           "defaults",
			http:           []HTTPConfig{{Listen: "127.0.0.1:0"}},
			wantGUI:        true,
			wantMetrics:    true,
			wantPosition:   true,
			wantSatellites: true,
		},
		{
			name: "metrics only",
			http: []HTTPConfig{{
				Listen:   "127.0.0.1:0",
				GUI:      httpBool(false),
				Metrics:  httpBool(true),
				Position: httpBool(false),
			}},
			wantMetrics:    true,
			wantSatellites: true,
		},
		{
			name: "position only",
			http: []HTTPConfig{{
				Listen:   "127.0.0.1:0",
				GUI:      httpBool(false),
				Metrics:  httpBool(false),
				Position: httpBool(true),
			}},
			wantPosition: true,
		},
		{
			name: "pprof only",
			http: []HTTPConfig{{
				Listen:   "127.0.0.1:0",
				PProf:    true,
				GUI:      httpBool(false),
				Metrics:  httpBool(false),
				Position: httpBool(false),
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{HTTP: tc.http}
			if got := cfg.anyHTTP(HTTPConfig.gui); got != tc.wantGUI {
				t.Errorf("anyHTTP(HTTPConfig.gui) = %t, want %t", got, tc.wantGUI)
			}
			if got := cfg.anyHTTP(HTTPConfig.metrics); got != tc.wantMetrics {
				t.Errorf("anyHTTP(HTTPConfig.metrics) = %t, want %t", got, tc.wantMetrics)
			}
			if got := cfg.anyHTTP(HTTPConfig.position); got != tc.wantPosition {
				t.Errorf("anyHTTP(HTTPConfig.position) = %t, want %t", got, tc.wantPosition)
			}
			if got := cfg.httpWantsSatellites(); got != tc.wantSatellites {
				t.Errorf("httpWantsSatellites() = %t, want %t", got, tc.wantSatellites)
			}
		})
	}
}

func TestHTTPObserverFactoriesUseFeatureChecks(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{HTTP: []HTTPConfig{{
		Listen:   "127.0.0.1:0",
		GUI:      httpBool(false),
		Metrics:  httpBool(true),
		Position: httpBool(false),
	}}}
	if obs := newSSEObserver(cfg, nil, lg, nil); obs != nil {
		t.Fatalf("newSSEObserver() = %v, want nil", obs)
	}
	if obs := newPrometheusObserver(cfg); obs == nil {
		t.Fatalf("newPrometheusObserver() = nil")
	}
	if obs := newPositionObserver(cfg); obs != nil {
		t.Fatalf("newPositionObserver() = %v, want nil", obs)
	}

	cfg = &Config{HTTP: []HTTPConfig{{
		Listen:   "127.0.0.1:0",
		GUI:      httpBool(true),
		Metrics:  httpBool(false),
		Position: httpBool(false),
	}}}
	sseCh := make(chan sse.Event, 1)
	obs := newSSEObserver(cfg, sseCh, lg, nil)
	if obs == nil {
		t.Fatalf("newSSEObserver() = nil")
	}
	obs.Release()
	if obs := newPrometheusObserver(cfg); obs != nil {
		t.Fatalf("newPrometheusObserver() = %v, want nil", obs)
	}
	if obs := newPositionObserver(cfg); obs != nil {
		t.Fatalf("newPositionObserver() = %v, want nil", obs)
	}
}

func httpBool(v bool) *bool { return &v }
