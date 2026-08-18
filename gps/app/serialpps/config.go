package serialpps

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/lib/check"
	"github.com/jclark/satpulse/gps/lib/wakeup"
)

// Config controls how serial PPS edges are detected and associated with
// UTC-labelled receiver messages. Durations are expressed in seconds in
// TOML.
type Config struct {
	// DelayUncertainty is the allowed measurement uncertainty when an
	// inferred post-pulse message delay is slightly negative.
	DelayUncertainty float64 `toml:"delayUncertainty" check:">=0,<1" comment:"Uncertainty in measured pulse-to-message delay (s)"`
	// MaxDelay is the maximum accepted inferred post-pulse message delay.
	MaxDelay float64 `toml:"maxDelay" check:">0,<1" comment:"Maximum post-pulse message delay (s)"`
	// Method selects how edges are detected; unspecified means automatic.
	Method gpsio.PPSMethod `toml:"method" comment:"PPS edge detection method: poll, wait, or kernel; omit for automatic selection"`
	// MaxWakeupLatency optionally limits latency added when a CPU wakes from
	// idle while serial PPS is active.
	MaxWakeupLatency *float64 `toml:"maxWakeupLatency" comment:"Maximum CPU wakeup latency while serial PPS is active (microseconds)"`
}

// DefaultConfig returns the default serial PPS sampling configuration.
func DefaultConfig() Config {
	return Config{
		DelayUncertainty: 0.005,
		MaxDelay:         0.8,
	}
}

// Validate checks that the delay interval is valid and narrower than one
// second, so at most one integral UTC label can satisfy it.
func (cfg Config) Validate() error {
	msgs := check.Validate(cfg)
	if cfg.DelayUncertainty >= 0 && cfg.DelayUncertainty < 1 && cfg.MaxDelay > 0 && cfg.MaxDelay < 1 &&
		!(cfg.DelayUncertainty+cfg.MaxDelay < 1) {
		msgs = append(msgs, fmt.Sprintf("delayUncertainty + maxDelay: must be < 1, got %g", cfg.DelayUncertainty+cfg.MaxDelay))
	}
	if cfg.Method < 0 || cfg.Method > gpsio.PPSMethodKernel {
		msgs = append(msgs, fmt.Sprintf("method: invalid value %d", int(cfg.Method)))
	}
	if cfg.MaxWakeupLatency != nil {
		if _, err := wakeupLatencyDuration(*cfg.MaxWakeupLatency); err != nil {
			msgs = append(msgs, "maxWakeupLatency: "+err.Error())
		}
	}
	switch len(msgs) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("in sample.serial.pps table: %s", msgs[0])
	default:
		msgs = append([]string{"errors in sample.serial.pps table:"}, msgs...)
		return errors.New(strings.Join(msgs, "\n\t"))
	}
}

func wakeupLatencyDuration(microseconds float64) (time.Duration, error) {
	if math.IsNaN(microseconds) || math.IsInf(microseconds, 0) {
		return 0, fmt.Errorf("must be finite, got %g", microseconds)
	}
	if microseconds < 0 {
		return 0, fmt.Errorf("must not be negative, got %g", microseconds)
	}
	if microseconds >= float64(math.MaxInt64)/float64(time.Microsecond) {
		return 0, fmt.Errorf("is too large, got %g", microseconds)
	}
	resolution := float64(wakeup.LatencyResolution) / float64(time.Microsecond)
	if microseconds > 0 && microseconds < resolution {
		return 0, fmt.Errorf("must be 0 or at least %g microseconds on this platform, got %g", resolution, microseconds)
	}
	d := time.Duration(math.Round(microseconds * float64(time.Microsecond)))
	if microseconds > 0 && d == 0 {
		return 0, fmt.Errorf("is too small to represent, got %g", microseconds)
	}
	return d, nil
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
