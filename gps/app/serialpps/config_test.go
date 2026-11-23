package serialpps

import (
	"math"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/app/pps"
)

func TestConfig(t *testing.T) {
	floatPtr := func(v float64) *float64 { return &v }
	base := pps.GeneratorConfig{MaxDelay: 0.8}
	tests := []struct {
		name    string
		cfg     Config
		errText string
	}{
		{name: "defaults", cfg: DefaultConfig()},
		{name: "zero uncertainty", cfg: Config{GeneratorConfig: base}},
		{name: "poll method", cfg: Config{GeneratorConfig: base, Method: gpsio.PPSMethodPoll}},
		{name: "wait method", cfg: Config{GeneratorConfig: base, Method: gpsio.PPSMethodWait}},
		{name: "kernel method", cfg: Config{GeneratorConfig: base, Method: gpsio.PPSMethodKernel}},
		{name: "zero wakeup latency", cfg: Config{GeneratorConfig: base, MaxWakeupLatency: floatPtr(0)}},
		{name: "fractional wakeup latency", cfg: Config{GeneratorConfig: base, MaxWakeupLatency: floatPtr(10.5e-6)}},
		{name: "invalid method", cfg: Config{GeneratorConfig: base, Method: gpsio.PPSMethodKernel + 1}, errText: "method"},
		{name: "negative wakeup latency", cfg: Config{GeneratorConfig: base, MaxWakeupLatency: floatPtr(-1)}, errText: "maxWakeupLatency"},
		{name: "NaN wakeup latency", cfg: Config{GeneratorConfig: base, MaxWakeupLatency: floatPtr(math.NaN())}, errText: "maxWakeupLatency"},
		{name: "infinite wakeup latency", cfg: Config{GeneratorConfig: base, MaxWakeupLatency: floatPtr(math.Inf(1))}, errText: "maxWakeupLatency"},
		{name: "one-second wakeup latency", cfg: Config{GeneratorConfig: base, MaxWakeupLatency: floatPtr(1)}, errText: "maxWakeupLatency"},
		{name: "negative uncertainty", cfg: Config{GeneratorConfig: pps.GeneratorConfig{DelayUncertainty: -0.001, MaxDelay: 0.8}}, errText: "delayUncertainty"},
		{name: "zero maximum", cfg: Config{GeneratorConfig: pps.GeneratorConfig{DelayUncertainty: 0.005}}, errText: "maxDelay"},
		{name: "prewarm", cfg: Config{GeneratorConfig: pps.DefaultGeneratorConfig(), PollPreWarm: 0.05}},
		{name: "negative prewarm", cfg: Config{GeneratorConfig: pps.DefaultGeneratorConfig(), PollPreWarm: -0.01}, errText: "pollPreWarm"},
		{name: "one-second interval", cfg: Config{GeneratorConfig: pps.GeneratorConfig{DelayUncertainty: 0.2, MaxDelay: 0.8}}, errText: "delayUncertainty + maxDelay"},
		{name: "interval over one second", cfg: Config{GeneratorConfig: pps.GeneratorConfig{DelayUncertainty: 0.3, MaxDelay: 0.8}}, errText: "delayUncertainty + maxDelay"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.errText == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.errText) {
				t.Fatalf("Validate error = %v, want text %q", err, tc.errText)
			}
		})
	}
}
