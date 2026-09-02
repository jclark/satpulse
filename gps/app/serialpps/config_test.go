package serialpps

import (
	"math"
	"strings"
	"testing"

	"github.com/jclark/satpulse/gps/app/gpsio"
)

func TestConfig(t *testing.T) {
	floatPtr := func(v float64) *float64 { return &v }
	tests := []struct {
		name    string
		cfg     Config
		errText string
	}{
		{name: "defaults", cfg: DefaultConfig()},
		{name: "zero uncertainty", cfg: Config{MaxDelay: 0.8}},
		{name: "poll method", cfg: Config{MaxDelay: 0.8, Method: gpsio.PPSMethodPoll}},
		{name: "wait method", cfg: Config{MaxDelay: 0.8, Method: gpsio.PPSMethodWait}},
		{name: "kernel method", cfg: Config{MaxDelay: 0.8, Method: gpsio.PPSMethodKernel}},
		{name: "zero wakeup latency", cfg: Config{MaxDelay: 0.8, MaxWakeupLatency: floatPtr(0)}},
		{name: "fractional wakeup latency", cfg: Config{MaxDelay: 0.8, MaxWakeupLatency: floatPtr(10.5e-6)}},
		{name: "invalid method", cfg: Config{MaxDelay: 0.8, Method: gpsio.PPSMethodKernel + 1}, errText: "method"},
		{name: "negative wakeup latency", cfg: Config{MaxDelay: 0.8, MaxWakeupLatency: floatPtr(-1)}, errText: "maxWakeupLatency"},
		{name: "NaN wakeup latency", cfg: Config{MaxDelay: 0.8, MaxWakeupLatency: floatPtr(math.NaN())}, errText: "maxWakeupLatency"},
		{name: "infinite wakeup latency", cfg: Config{MaxDelay: 0.8, MaxWakeupLatency: floatPtr(math.Inf(1))}, errText: "maxWakeupLatency"},
		{name: "one-second wakeup latency", cfg: Config{MaxDelay: 0.8, MaxWakeupLatency: floatPtr(1)}, errText: "maxWakeupLatency"},
		{name: "negative uncertainty", cfg: Config{DelayUncertainty: -0.001, MaxDelay: 0.8}, errText: "delayUncertainty"},
		{name: "zero maximum", cfg: Config{DelayUncertainty: 0.005}, errText: "maxDelay"},
		{name: "prewarm", cfg: Config{DelayUncertainty: 0.005, MaxDelay: 0.8, PollPreWarm: 0.05}},
		{name: "negative prewarm", cfg: Config{DelayUncertainty: 0.005, MaxDelay: 0.8, PollPreWarm: -0.01}, errText: "pollPreWarm"},
		{name: "outlier check disabled", cfg: Config{MaxDelay: 0.8}},
		{name: "outlier ratio below one", cfg: Config{MaxDelay: 0.8, PollOutlierRatio: 0.5}, errText: "pollOutlierRatio"},
		{name: "negative outlier ratio", cfg: Config{MaxDelay: 0.8, PollOutlierRatio: -3}, errText: "pollOutlierRatio"},
		{name: "NaN outlier ratio", cfg: Config{MaxDelay: 0.8, PollOutlierRatio: math.NaN()}, errText: "pollOutlierRatio"},
		{name: "one-second interval", cfg: Config{DelayUncertainty: 0.2, MaxDelay: 0.8}, errText: "delayUncertainty + maxDelay"},
		{name: "interval over one second", cfg: Config{DelayUncertainty: 0.3, MaxDelay: 0.8}, errText: "delayUncertainty + maxDelay"},
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
