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
	gen := func(delayUncertainty, maxDelay float64) pps.GeneratorConfig {
		return pps.GeneratorConfig{DelayUncertainty: delayUncertainty, MaxDelay: maxDelay}
	}
	tests := []struct {
		name    string
		cfg     Config
		errText string
	}{
		{name: "defaults", cfg: DefaultConfig()},
		{name: "zero uncertainty", cfg: Config{GeneratorConfig: gen(0, 0.8)}},
		{name: "poll method", cfg: Config{GeneratorConfig: gen(0, 0.8), Method: gpsio.PPSMethodPoll}},
		{name: "wait method", cfg: Config{GeneratorConfig: gen(0, 0.8), Method: gpsio.PPSMethodWait}},
		{name: "kernel method", cfg: Config{GeneratorConfig: gen(0, 0.8), Method: gpsio.PPSMethodKernel}},
		{name: "zero wakeup latency", cfg: Config{GeneratorConfig: gen(0, 0.8), MaxWakeupLatency: floatPtr(0)}},
		{name: "fractional wakeup latency", cfg: Config{GeneratorConfig: gen(0, 0.8), MaxWakeupLatency: floatPtr(10.5e-6)}},
		{name: "invalid method", cfg: Config{GeneratorConfig: gen(0, 0.8), Method: gpsio.PPSMethodKernel + 1}, errText: "method"},
		{name: "negative wakeup latency", cfg: Config{GeneratorConfig: gen(0, 0.8), MaxWakeupLatency: floatPtr(-1)}, errText: "maxWakeupLatency"},
		{name: "NaN wakeup latency", cfg: Config{GeneratorConfig: gen(0, 0.8), MaxWakeupLatency: floatPtr(math.NaN())}, errText: "maxWakeupLatency"},
		{name: "infinite wakeup latency", cfg: Config{GeneratorConfig: gen(0, 0.8), MaxWakeupLatency: floatPtr(math.Inf(1))}, errText: "maxWakeupLatency"},
		{name: "one-second wakeup latency", cfg: Config{GeneratorConfig: gen(0, 0.8), MaxWakeupLatency: floatPtr(1)}, errText: "maxWakeupLatency"},
		{name: "negative uncertainty", cfg: Config{GeneratorConfig: gen(-0.001, 0.8)}, errText: "delayUncertainty"},
		{name: "zero maximum", cfg: Config{GeneratorConfig: gen(0.005, 0)}, errText: "maxDelay"},
		{name: "prewarm", cfg: Config{GeneratorConfig: gen(0.005, 0.8), PollPreWarm: 0.05}},
		{name: "negative prewarm", cfg: Config{GeneratorConfig: gen(0.005, 0.8), PollPreWarm: -0.01}, errText: "pollPreWarm"},
		{name: "one-second interval", cfg: Config{GeneratorConfig: gen(0.2, 0.8)}, errText: "delayUncertainty + maxDelay"},
		{name: "interval over one second", cfg: Config{GeneratorConfig: gen(0.3, 0.8)}, errText: "delayUncertainty + maxDelay"},
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
