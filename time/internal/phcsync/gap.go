package phcsync

import (
	"log/slog"
	"slices"
	"time"

	"github.com/jclark/satpulse/time/lib/median"
)

// GapConfig contains tunable parameters for gap mode.
type GapConfig struct {
	// RecoveryThreshold is the number of consecutive missing samples above
	// which returning samples go through MAD recovery (relaxed outlier
	// detection followed by a window shift) instead of resuming tracking
	// directly. Shorter gaps cause too little drift to disturb the MAD
	// window. Must be less than track.badSampleRunLimit, or the gap is
	// abandoned for a reset before recovery can start.
	RecoveryThreshold int `toml:"recoveryThreshold" check:">=1,<1000" comment:"Missing samples before MAD recovery is used"`

	// DriftLimit is the maximum shift in nanoseconds of the post-gap sample
	// median from the pre-gap baseline. A larger shift indicates the clock
	// drifted too far during the gap and triggers a reset. It is also added
	// to the MAD threshold during recovery so that samples reflecting
	// legitimate drift within the limit are not rejected as outliers.
	DriftLimit int64 `toml:"driftLimit" check:">0,<=1_000_000" comment:"Max median shift from pre-gap baseline (ns)"`

	// RecoverySamples is the number of accepted post-gap samples to collect
	// before re-baselining the MAD window.
	RecoverySamples int `toml:"recoverySamples" check:">=1,<100" comment:"Samples to collect before shifting MAD window"`
}

func defaultGapConfig() GapConfig {
	return GapConfig{
		RecoveryThreshold: 3,   // below default badSampleRunLimit (5)
		DriftLimit:        100, // 100ns
		RecoverySamples:   5,
	}
}

// gapSampleProcessor handles samples during a brief loss of PPS signal.
// It is entered from tracking mode on any missing sample and holds the live
// tracking processor so that its state (servo, MAD window, averaged
// frequency, bad-sample bookkeeping) survives the gap and is restored when
// tracking resumes.
type gapSampleProcessor struct {
	cfg             GapConfig
	trackingProc    *trackingSampleProcessor
	missingSamples  int             // consecutive missing samples, including the one that triggered gap mode
	recoveryOffsets []time.Duration // accepted post-gap offsets collected during MAD recovery
	lg              *slog.Logger
}

func newGapSampleProcessor(cfg GapConfig, trackingProc *trackingSampleProcessor, lg *slog.Logger) *gapSampleProcessor {
	return &gapSampleProcessor{cfg: cfg, trackingProc: trackingProc, missingSamples: 1, lg: lg}
}

func (p *gapSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	tp := p.trackingProc
	if sample.Kind == SampleMissing {
		// The PHC is already running at the averaged frequency (switched on
		// the missing sample that triggered gap mode); just keep counting.
		p.missingSamples++
		action := tp.noteBadSample()
		if tp.limitExceeded() {
			return action, ModeReset
		}
		return action, ModeGap
	}
	if p.missingSamples < p.cfg.RecoveryThreshold && len(p.recoveryOffsets) == 0 {
		// Short gap: drift is too small to disturb the MAD window; process
		// the sample normally and resume tracking.
		return tp.processSample(sample)
	}
	return p.recoverySample(sample)
}

// recoverySample processes a returning sample after a long gap. Outlier
// detection is relaxed by DriftLimit so samples reflecting legitimate drift
// are accepted; accepted samples drive the servo and are collected until
// enough are available to re-baseline the MAD window.
func (p *gapSampleProcessor) recoverySample(sample *Sample) (phcAction, Mode) {
	tp := p.trackingProc
	absOffset := sample.Offset.Abs()
	if absOffset >= time.Duration(tp.cfg.OutlierThreshold) ||
		(absOffset >= time.Duration(tp.cfg.MADThreshold) &&
			tp.madIsOutlierSlack(sample.Offset, time.Duration(p.cfg.DriftLimit))) {
		sample.Kind = SampleOutlier
		p.lg.Debug("outlier rejected during gap recovery", "offset", sample.Offset)
		action := tp.noteBadSample()
		if tp.limitExceeded() {
			return action, ModeReset
		}
		return action, ModeGap
	}
	tp.consecutiveBadSamples = 0
	tp.recordSample(false)
	freq := tp.servo.sample(sample.Offset)
	tp.updateAvgFreq(freq)
	action := phcAction{actionType: phcAdjustFrequency, freq: freq}
	p.recoveryOffsets = append(p.recoveryOffsets, sample.Offset)
	if len(p.recoveryOffsets) < p.cfg.RecoverySamples {
		return action, ModeGap
	}
	return action, p.shiftWindow()
}

// shiftWindow re-baselines the MAD window after recovery: the window is
// shifted by the median drift observed across the gap, then the collected
// offsets are added. If the drift exceeds DriftLimit the clock is no longer
// trustworthy and the controller falls back to reset mode.
func (p *gapSampleProcessor) shiftWindow() Mode {
	tp := p.trackingProc
	preMedian := tp.madWindow.Median()
	newMedian := median.Median(slices.Values(p.recoveryOffsets))
	shift := newMedian - preMedian
	if shift.Abs() > time.Duration(p.cfg.DriftLimit) {
		p.lg.Info("entering reset mode", "reason", "gapRecoveryDrift", "shift", shift)
		return ModeReset
	}
	tp.madWindow.ShiftAll(shift)
	for _, off := range p.recoveryOffsets {
		tp.madWindow.Add(off)
	}
	p.lg.Info("gap recovery complete", "shift", shift, "samples", len(p.recoveryOffsets))
	return ModeTracking
}
