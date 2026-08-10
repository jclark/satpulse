package serialpps

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/time/lib/check"
)

// Config controls how serial PPS edges are associated with UTC-labelled
// receiver messages. Durations are expressed in seconds in TOML.
type Config struct {
	// DelayUncertainty is the allowed measurement uncertainty when an
	// inferred post-pulse message delay is slightly negative.
	DelayUncertainty float64 `toml:"delayUncertainty" check:">=0,<1" comment:"Uncertainty in measured pulse-to-message delay (s)"`
	// MaxDelay is the maximum accepted inferred post-pulse message delay.
	MaxDelay float64 `toml:"maxDelay" check:">0,<1" comment:"Maximum post-pulse message delay (s)"`
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

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
