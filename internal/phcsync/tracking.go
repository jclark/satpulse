package phcsync

import (
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/median"
)

// TrackingConfig contains tunable parameters for tracking mode.
type TrackingConfig struct {
	// Kp is the proportional gain for the PI servo used during tracking mode.
	// Lower than converging mode for stability during normal operation.
	Kp float64 `toml:"kp" check:">0.0,<10.0"`

	// Ki is the integral gain for the PI servo used during tracking mode.
	// Lower than converging mode to prevent overcorrection during stable tracking.
	Ki float64 `toml:"ki" check:">0.0,<10.0"`

	// OutlierThreshold is the minimum absolute offset in nanoseconds for a sample to be
	// considered for MAD-based outlier detection. Samples with |offset| < OutlierThreshold
	// are always accepted. Samples with |offset| >= OutlierThreshold are evaluated using
	// MAD statistics to determine if they are outliers. This ensures samples within the
	// normal range of tracking jitter are not treated as outliers.
	OutlierThreshold int64 `toml:"outlierThreshold" check:">0,<=1000"`

	// MADWindow is the number of samples in the sliding window for MAD-based outlier detection.
	// The window stores offset history used to compute the median and median absolute deviation.
	// Larger windows provide more robust outlier detection but respond more slowly to changing
	// conditions.
	MADWindow int `toml:"madWindow" check:">=3,<100"`

	// MADMultiple is the multiple of MAD (Median Absolute Deviation) used as the outlier threshold.
	// A sample is classified as an outlier if its offset is more than MADMultiple * MAD away from
	// the median offset. Higher values make outlier detection more conservative (fewer rejections).
	MADMultiple float64 `toml:"madMultiple" check:">0.0,<1000.0"`

	// MADMinSamples is the minimum number of samples required in the MAD window before MAD-based
	// outlier detection is active. Until this threshold is reached, only the OutlierThreshold
	// is checked. This ensures MAD statistics are based on sufficient data.
	MADMinSamples int `toml:"madMinSamples" check:">=3,<100"`

	// PreMADOutlierThreshold is the outlier threshold in nanoseconds used before MAD-based
	// detection is ready. During the warmup period (offsetWindow.Len() < MADMinSamples),
	// a sample is rejected if |offset| >= OutlierThreshold AND |offset| >= PreMADOutlierThreshold.
	// This protects the servo from obviously bad outliers during warmup. Once MAD is ready,
	// full statistical detection is used.
	PreMADOutlierThreshold int64 `toml:"preMADOutlierThreshold" check:">0,<=10000"`

	// PulseWidthTolerance is the tolerance in nanoseconds for filtering trailing edges in
	// dual-edge mode based on temporal spacing. An edge is considered a trailing edge (and
	// ignored) if it arrives within PulseWidthTolerance of the expected trailing edge time
	// (lastEdgeTime + PulseWidth). This helps distinguish leading from trailing edges when
	// alignment alone is ambiguous.
	PulseWidthTolerance int64 `toml:"pulseWidthTolerance" check:">0,<=10000"`

	// AlignTolerance is the tolerance in nanoseconds for offset from top of second to
	// immediately accept an edge as a leading edge. Edges with |offset| <= AlignTolerance
	// (where offset is timestamp rounded to nearest second) are assumed to be leading edges
	// and accepted without further checks. This is the primary discriminator for
	// well-synchronized clocks.
	AlignTolerance int64 `toml:"alignTolerance" check:">0,<=10000"`

	// BadSampleLimit is the maximum number of consecutive bad samples (missing or outlier)
	// before transitioning back to reset mode. Bad samples do not contribute to frequency
	// adjustment.
	BadSampleLimit int `toml:"badSampleLimit" check:">=1,<100"`

	// AvgFreqTimeConstant is the time constant in seconds for the exponential moving
	// average of frequency adjustments. This average represents the baseline frequency
	// correction (without phase correction) and is used when samples are missing.
	// Larger values track baseline frequency more smoothly but respond more slowly to
	// oscillator drift. The EMA is updated as: avgFreq = alpha*freq + (1-alpha)*avgFreq
	// where alpha = 1 - exp(-sampleInterval/timeConstant). Set to 0 to disable this
	// feature (no frequency adjustment on missing samples).
	AvgFreqTimeConstant float64 `toml:"avgFreqTimeConstant" check:">=0.0,<1000.0"`

	// IgnoreSawtoothCorrection, when true, disables the use of pulse offset corrections
	// from PrePulse messages. This is primarily for testing to verify that sawtooth
	// correction improves synchronization accuracy. Default: false (use corrections).
	IgnoreSawtoothCorrection bool `toml:"ignoreSawtoothCorrection"`
}

func defaultTrackingConfig() TrackingConfig {
	return TrackingConfig{
		Kp:                     0.5,
		Ki:                     0.1,
		OutlierThreshold:       50,   // 50ns
		MADWindow:              10,   // 10 samples
		MADMultiple:            25.0, // 25 * MAD
		MADMinSamples:          10,   // 10 samples minimum
		PreMADOutlierThreshold: 500,  // 500ns warmup threshold
		PulseWidthTolerance:    200,  // 200ns
		AlignTolerance:         100,  // 100ns
		BadSampleLimit:         5,
		AvgFreqTimeConstant:    30, // 30 seconds
	}
}

type trackingSampleGenerator struct {
	cfg        TrackingConfig
	pt         PulseType
	freq       float64
	maxFreq    float64
	lg         *slog.Logger
	lastSample *Sample // last accepted sample, used for edge filtering
	timeMsgBuf TimeMsgBuffer
}

func newTrackingSampleGenerator(cfg TrackingConfig, pt PulseType, lastSample *Sample, freq, maxFreq float64, timeMsgBuf TimeMsgBuffer, lg *slog.Logger) *trackingSampleGenerator {
	if pt.EdgesPerPulse == 2 && pt.PulseWidth <= 0 {
		panic("tracking mode: PulseWidth must be > 0 when EdgesPerPulse == 2")
	}
	return &trackingSampleGenerator{
		cfg:        cfg,
		pt:         pt,
		freq:       freq,
		maxFreq:    maxFreq,
		lg:         lg,
		lastSample: lastSample,
		timeMsgBuf: timeMsgBuf,
	}
}

// ignoreEdge determines whether to ignore an edge based on dual-edge filtering.
// Returns true if the edge should be ignored (filtered out).
func (g *trackingSampleGenerator) ignoreEdge(edge PulseEdge, edgeIndex uint64) bool {
	// If not dual-edge mode, accept all edges
	if g.pt.EdgesPerPulse != 2 {
		return false
	}

	// Round to nearest second to get offset from top of second
	refTime := edge.Timestamp.T.Round(time.Second)
	offset := edge.Timestamp.T.Sub(refTime)
	offsetAbs := offset.Abs()

	// if alignKeep is true, it indicates we should probably keep this edge
	alignKeep := offsetAbs <= time.Duration(g.cfg.AlignTolerance)

	// Check spacing relative to pulse width
	// Note: PulseWidth is guaranteed to be > 0 when EdgesPerPulse == 2 (validated in constructor)
	// lastSample is always non-nil (tracking mode is entered from converging mode)

	lastEdgeTime := g.lastSample.Ref.Add(g.lastSample.Offset)
	// if widthIgnore is true, it indicates we should probably ignore this edge
	widthIgnore := (edge.Timestamp.T.Sub(lastEdgeTime) - g.pt.PulseWidth).Abs() <= time.Duration(g.cfg.PulseWidthTolerance)

	// parityIgnore is true suggests ignoring this edge, and !parityIgnore suggests keeping it
	parityIgnore := (g.lastSample.EdgeIndex^edgeIndex)&1 != 0

	if alignKeep && !parityIgnore {
		return false
	}

	if widthIgnore && parityIgnore {
		return true
	}

	// this also has the effect of resetting parity
	if alignKeep {
		return false
	}

	if widthIgnore {
		return true
	}

	return parityIgnore
}

func (g *trackingSampleGenerator) pulseEdgeSample(edge PulseEdge, edgeIndex uint64) *Sample {
	// Filter dual edges
	if g.ignoreEdge(edge, edgeIndex) {
		return nil
	}

	// Round to nearest second
	sec := edge.Timestamp.T.Round(time.Second)

	// Apply pulse offset correction if available (unless disabled by config)
	refTime := sec
	if !g.cfg.IgnoreSawtoothCorrection {
		pulseOffset, ok := g.timeMsgBuf.GetPrePulseCorrection(sec)
		if ok {
			// PulseOffset satisfies: true_second = pulse + PulseOffset
			// We want GPS time of pulse: pulse = true_second - PulseOffset
			// sec approximates true_second, so: pulse_GPS_time = sec - PulseOffset
			refTime = sec.Add(-pulseOffset)
			g.lg.Debug("applied pulse offset correction",
				"sec", sec,
				"pulseOffset_ns", pulseOffset.Nanoseconds(),
				"refTime", refTime)
		}
	}
	offset := edge.Timestamp.T.Sub(refTime)

	// Estimate system time: TRead minus the time between reading and timestamp capture
	phcDelta := edge.TRead.Clock.T.Sub(edge.Timestamp.T)
	sys := edge.TRead.Sys.Add(-phcDelta)

	sample := &Sample{
		Kind:      SampleOK,
		Ref:       refTime,
		Offset:    offset,
		Era:       edge.Timestamp.Era,
		EdgeIndex: edgeIndex,
		Sys:       sys,
		// Freq and Mode will be filled in by processSample
	}

	// Update lastSample for next edge comparison
	g.lastSample = sample

	return sample
}

func (g *trackingSampleGenerator) timeMessageSample() *Sample {
	// Tracking mode only uses pulse edges for now
	return nil
}

type trackingSampleProcessor struct {
	servo                 *piServo
	cfg                   TrackingConfig
	consecutiveBadSamples int
	avgFreq               float64 // exponential moving average of frequency
	emaAlpha              float64 // EMA coefficient (1 - exp(-1/timeConstant))
	offsetWindow          *median.Window[time.Duration]
	lg                    *slog.Logger
}

func newTrackingSampleProcessor(cfg TrackingConfig, currentFreq, maxFreq float64, lg *slog.Logger) *trackingSampleProcessor {
	// Calculate EMA alpha from time constant
	// With 1 second sample interval: alpha = 1 - exp(-1/timeConstant)
	// If timeConstant is 0, EMA feature is disabled
	var emaAlpha float64
	if cfg.AvgFreqTimeConstant > 0 {
		emaAlpha = 1.0 - math.Exp(-1.0/cfg.AvgFreqTimeConstant)
	}
	return &trackingSampleProcessor{
		servo:        newPiServo(currentFreq, cfg.Kp, cfg.Ki, maxFreq),
		cfg:          cfg,
		avgFreq:      currentFreq, // initialize with current frequency
		emaAlpha:     emaAlpha,
		offsetWindow: median.New[time.Duration](cfg.MADWindow),
		lg:           lg,
	}
}

func (p *trackingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	action := p.sampleAction(sample)

	// Track consecutive bad samples (missing or outlier - i.e., not sent to servo)
	if action.actionType == phcNoAction {
		p.consecutiveBadSamples++
		// Use EMA feature (adjusting on bad samples), if not disabled by AvgFreqTimeConstant = 0
		if p.consecutiveBadSamples == 1 && p.cfg.AvgFreqTimeConstant > 0 {
			action.actionType = phcAdjustFrequency
			action.freq = p.avgFreq
			p.lg.Debug("first bad sample; switching to average frequency", "avgFreq", p.avgFreq)
		}
	} else {
		p.consecutiveBadSamples = 0
	}
	// Transition to reset mode if too many consecutive bad samples
	if p.consecutiveBadSamples >= p.cfg.BadSampleLimit {
		p.lg.Info("entering reset mode", "consecutiveBadSamples", p.consecutiveBadSamples)
		return action, ModeReset
	}
	return action, ModeTracking
}

func (p *trackingSampleProcessor) sampleAction(sample *Sample) phcAction {
	if sample.Kind == SampleMissing {
		return phcAction{actionType: phcNoAction}
	}

	// Add signed offset to window (including outliers)
	// Median is robust to outliers, so including them doesn't corrupt the statistics
	p.offsetWindow.Add(sample.Offset)

	// During MAD warmup, reject samples exceeding both thresholds
	if p.offsetWindow.Len() < p.cfg.MADMinSamples {
		absOffset := sample.Offset.Abs()
		if absOffset >= time.Duration(p.cfg.OutlierThreshold) && absOffset >= time.Duration(p.cfg.PreMADOutlierThreshold) {
			sample.Kind = SampleOutlier
			p.lg.Debug("outlier rejected during MAD warmup",
				"offset", sample.Offset,
				"outlierThreshold", p.cfg.OutlierThreshold,
				"preMADOutlierThreshold", p.cfg.PreMADOutlierThreshold)
			return phcAction{actionType: phcNoAction}
		}
	} else {
		// MAD is ready, use full MAD-based detection
		if sample.Offset.Abs() >= time.Duration(p.cfg.OutlierThreshold) && p.madIsOutlier(sample.Offset) {
			sample.Kind = SampleOutlier
			p.lg.Debug("outlier detected by MAD",
				"offset", sample.Offset)
			return phcAction{actionType: phcNoAction}
		}
	}

	// Apply PI control for good samples
	freq := p.servo.sample(sample.Offset)

	// Update exponential moving average of frequency
	// avgFreq = alpha * freq + (1-alpha) * avgFreq
	p.avgFreq = p.emaAlpha*freq + (1.0-p.emaAlpha)*p.avgFreq

	return phcAction{
		actionType: phcAdjustFrequency,
		freq:       freq,
	}
}

// madIsOutlier determines if an offset is an outlier using MAD detection.
// Computes the median of offsets (center), then the median of absolute deviations
// from that center (MAD). An offset is an outlier if its deviation from center
// exceeds k * MAD. This correctly handles sustained DC bias (e.g., ionospheric
// delays) while rejecting transient spikes (e.g., multipath errors).
func (p *trackingSampleProcessor) madIsOutlier(offset time.Duration) bool {
	// Need minimum samples for MAD to be meaningful
	if p.offsetWindow.Len() < p.cfg.MADMinSamples {
		return false
	}

	// Compute median of offsets (the center, can be non-zero during transients)
	center := p.offsetWindow.Median()

	// Compute MAD: median of absolute deviations from center
	// Use iterator to avoid materializing intermediate slice unnecessarily
	mad := median.Median(func(yield func(time.Duration) bool) {
		p.offsetWindow.Iterate(func(i int, v time.Duration) bool {
			return yield((v - center).Abs())
		})
	})

	// Check if offset deviates too much from center
	threshold := time.Duration(float64(mad) * p.cfg.MADMultiple)
	deviation := (offset - center).Abs()

	return deviation > threshold
}
