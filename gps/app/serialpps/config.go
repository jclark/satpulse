package serialpps

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/lib/check"
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
	MaxWakeupLatency *float64 `toml:"maxWakeupLatency" check:">=0,<1" comment:"Maximum CPU wakeup latency while serial PPS is active (s)"`
	// PollPreWarm is how long the poll method busy-waits before each poll
	// window opens, countering hosts whose state queries slow down while
	// the machine idles. Zero disables it.
	PollPreWarm float64 `toml:"pollPreWarm" check:">=0,<1" comment:"CPU busy-wait before each poll window (s); 0 disables"`
	// PollOutlierRatio marks a tracking catch of the poll method an outlier
	// when its bracket exceeds this multiple of the lower quartile of the
	// recent settled brackets, so that an edge whose state read stalled under
	// host load can be withheld from timing. Zero disables the check.
	PollOutlierRatio float64 `toml:"pollOutlierRatio" check:">=0" comment:"Bracket multiple of the recent lower quartile beyond which a poll catch is an outlier; 0 disables"`
}

// DefaultConfig returns the default serial PPS sampling configuration.
func DefaultConfig() Config {
	return Config{
		DelayUncertainty: 0.005,
		MaxDelay:         0.8,
		PollOutlierRatio: 3,
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
	if r := cfg.PollOutlierRatio; r > 0 && r < 1 {
		msgs = append(msgs, fmt.Sprintf("pollOutlierRatio: must be 0 or at least 1, got %g", r))
	}
	if cfg.Method < 0 || cfg.Method > gpsio.PPSMethodKernel {
		msgs = append(msgs, fmt.Sprintf("method: invalid value %d", int(cfg.Method)))
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
