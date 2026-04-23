// Package phcsample generates chrony SOCK refclock samples when the PHC is
// left free-running. The PHC is read but never stepped or slewed;
// phcsample correlates PHC timestamps and GPS time messages to construct
// an (offset = true time - sys) sample for each cross-sample.
package phcsample

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jclark/satpulse/time/lib/check"
)

// Config contains tunable parameters for phcsample. Some field names
// are shared verbatim with phcsync.ResetConfig where the meaning
// carries over; others are specific to phcsample's continuous
// message-stream regression model and have no reset-mode analogue.
type Config struct {
	// PulseWindow is the number of pulses kept for the PHC calibration
	// window. Carried over verbatim from phcsync.ResetConfig; used only
	// by phcWindow, not by wallClock.
	PulseWindow int `toml:"pulseWindow" check:">=3,<100" comment:"Number of pulses for alignment analysis"`

	// PulseVariation is the maximum acceptable relative variation
	// between stride-edgesPerPulse pulse intervals, expressed in parts
	// per billion (PPB). Carried over verbatim from phcsync.ResetConfig.
	PulseVariation float64 `toml:"pulseVariation" check:">=5,<1_000_000" comment:"Max pulse interval variation (ppb)"`

	// ExpectedDelay is the expected pulse-to-message delay in seconds.
	// In phcsample it is used only as a centering shift subtracted from
	// each MsgUTCTime sample's tRead before feeding the wallClock
	// regression, so predictions centre on the true pulse-mono time
	// rather than on the later message read time.
	ExpectedDelay float64 `toml:"expectedDelay" check:">=0.0,<1.0" comment:"Expected pulse-to-message delay (s)"`

	// PulseWidthDetectLimit is the maximum pulse width in seconds that
	// can be automatically detected in dual-edge mode to determine
	// which edge is leading. Carried over verbatim from
	// phcsync.ResetConfig.
	PulseWidthDetectLimit float64 `toml:"pulseWidthDetectLimit" check:">=0.1,<0.5" comment:"Max auto-detectable pulse width (s)"`

	// MsgWindow is the length of the message history retained for the
	// wallClock's monotonic-to-UTC regression, in seconds. Messages
	// older than this are discarded. Larger windows give a tighter
	// slope estimate, greater tolerance of occasional outliers, and
	// more averaging of per-message jitter, at negligible cost in
	// memory or compute.
	MsgWindow int `toml:"msgWindow" check:">=5,<=300" comment:"Message history retained (s)"`

	// MaxMsgGap is how long phcsample continues answering after the
	// most recent GPS message arrived, in seconds. While messages are
	// flowing the fitted mapping stays fresh; when they stop, the
	// wallClock projects forward until the gap reaches this limit,
	// then stops producing samples.
	MaxMsgGap float64 `toml:"maxMsgGap" check:">=1.0,<=300.0" comment:"Max gap since last message before samples stop (s)"`

	// MinMsgSpan is the minimum elapsed time (seconds) that the
	// observed messages must cover before the wallClock's fit is
	// treated as usable. Larger values give more stable slope
	// estimates but delay the first sample after startup.
	MinMsgSpan float64 `toml:"minMsgSpan" check:">=1.0,<=60.0" comment:"Min message window span for usable fit (s)"`

	// ClockRateLimit bounds the rate mismatch between the system
	// monotonic clock and the UTC time advertised by the GPS message
	// stream, expressed as a dimensionless fraction (e.g. 0.1 means
	// 10%). The two clocks are expected to tick at nearly the same
	// rate; a large deviation typically indicates a broken message
	// stream or a clock malfunction.
	//
	// When the observed rate difference exceeds this limit, phcsample
	// stops producing samples until the mismatch falls back within
	// bounds.
	//
	// This is a safety bound intended to catch clearly pathological
	// conditions. Values should be generous, leaving plenty of
	// headroom above normal crystal drift.
	ClockRateLimit float64 `toml:"clockRateLimit" check:">0.0,<=1.0" comment:"Max clock rate mismatch [0,1]"`

	// MsgTimingVariation bounds the inconsistency in message timing
	// tolerated by the wallClock fit, expressed as a fraction of one
	// second. Measured as the median absolute residual of observations
	// around the fitted line, so up to about half of the messages may
	// be offset (for example by the slower of two interleaved message
	// types, or a burst on a slow serial link) without firing.
	//
	// Deliberately lenient; a safety gate against genuinely broken
	// streams, not a per-sample quality filter.
	MsgTimingVariation float64 `toml:"msgTimingVariation" check:">0.0,<1.0" comment:"Max median residual around fit [0,1]"`

	// EdgeSecondTolerance bounds the distance between the wallClock's
	// fit-predicted UTC of a pulse edge and the nearest integer second,
	// as a fraction of one second. Edges outside this tolerance are
	// dropped so that rounding to the nearest integer second is never
	// ambiguous. Must be well under 0.5 (rounding boundary); typical
	// values are a small multiple of worst-case message-to-pulse
	// delay jitter.
	EdgeSecondTolerance float64 `toml:"edgeSecondTolerance" check:">0.0,<0.5" comment:"Max edge distance from integer second [0,0.5)"`

	// IgnoreSawtoothCorrection, when true, disables the use of pulse offset corrections.
	IgnoreSawtoothCorrection bool `toml:"ignoreSawtoothCorrection" comment:"Ignore sawtooth corrections"`

	// DiscontinuityThreshold is the minimum absolute PHC-interval
	// deviation from the median that triggers a PHC discontinuity
	// restart in the pulse-edge admissibility pass, in seconds. An
	// external process (e.g. chrony disciplining the PHC, or
	// phc_ctl) may step the PHC; a step that crosses this threshold
	// causes consistentEdges to discard pre-step edges and restart
	// from the post-step suffix. Should be set well above ordinary
	// PHC jitter and aligned with the step / don't-step threshold of
	// the external disciplining system, if any. The lower bound is 1
	// ns (the resolution of time.Duration); values below that would
	// round to zero on internal conversion.
	DiscontinuityThreshold float64 `toml:"discontinuityThreshold" check:">=1e-9,<1.0" comment:"PHC discontinuity detection threshold (s)"`
}

// maxExtrapolation is how far past the last admitted edge's PHC
// timestamp phcWindow will extrapolate the PHC-to-UTC fit when
// evaluating Generate. The query is normally at the latest edge's
// PHC timestamp, but pre-admission can reject a trailing run of
// edges (the outlier edge and its immediate neighbour), pushing the
// last admitted entry up to two pulse intervals behind. Three
// seconds accommodates that case for single-edge 1 Hz pulses while
// still catching pathologically stale windows. Held as a method so
// the plan's w.cfg.maxExtrapolation() call-site signature is
// preserved even though the value is not a tunable Config field
// today.
func (cfg Config) maxExtrapolation() time.Duration {
	return 3 * time.Second
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		PulseWindow:           5,
		PulseVariation:        500.0,
		ExpectedDelay:         0.15,
		PulseWidthDetectLimit: 0.45,
		MsgWindow:             30,
		MaxMsgGap:             10.0,
		// Slightly below 3s so normal message-timing variation does not
		// push startup from pulse 5 to pulse 6 when using 1 Hz messages.
		MinMsgSpan:          2.9,
		ClockRateLimit:         0.1,
		MsgTimingVariation:     0.2,
		EdgeSecondTolerance:    0.4,
		DiscontinuityThreshold: 0.001,
	}
}

// Validate checks that all fields satisfy their check tag constraints,
// and that inter-field invariants hold.
func (cfg *Config) Validate() error {
	msgs := check.Validate(cfg)
	if float64(cfg.MsgWindow) <= cfg.MinMsgSpan {
		msgs = append(msgs, fmt.Sprintf("msgWindow (%ds) must be greater than minMsgSpan (%gs); otherwise pruning discards history faster than the fit needs",
			cfg.MsgWindow, cfg.MinMsgSpan))
	}
	switch len(msgs) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("in phcsample table: %s", msgs[0])
	default:
		msgs = append([]string{"errors in phcsample table:"}, msgs...)
		return errors.New(strings.Join(msgs, "\n\t"))
	}
}
