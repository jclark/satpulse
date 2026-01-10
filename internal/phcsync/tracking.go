package phcsync

import (
	"log/slog"
	"math"
	"time"

	"github.com/jclark/satpulse/internal/circbuf"
	"github.com/jclark/satpulse/internal/median"
)

// TrackingConfig contains tunable parameters for tracking mode.
type TrackingConfig struct {
	// Kp is the proportional gain for the PI servo used during tracking mode.
	// Lower than converging mode for stability during normal operation.
	Kp float64 `toml:"kp" check:">0.0,<10.0" comment:"PI servo proportional gain"`

	// Ki is the integral gain for the PI servo used during tracking mode.
	// Lower than converging mode to prevent overcorrection during stable tracking.
	Ki float64 `toml:"ki" check:">0.0,<10.0" comment:"PI servo integral gain"`

	// MADThreshold is the minimum absolute offset in nanoseconds for a sample to be
	// considered for MAD-based outlier detection. Samples with |offset| < MADThreshold
	// are always accepted. Samples with |offset| >= MADThreshold are evaluated using
	// MAD statistics to determine if they are outliers. This ensures samples within the
	// normal range of tracking jitter are not treated as outliers.
	MADThreshold int64 `toml:"madThreshold" check:">0,<=1_000_000" comment:"Min offset to use MAD for outlier detection (ns)"`

	// MADWindow is the number of samples in the sliding window for MAD-based outlier detection.
	// The window stores offset history used to compute the median and median absolute deviation.
	// Larger windows provide more robust outlier detection but respond more slowly to changing
	// conditions.
	MADWindow int `toml:"madWindow" check:">=3,<100" comment:"Number of samples for MAD window"`

	// MADMultiple is the multiple of MAD (Median Absolute Deviation) used as the outlier threshold.
	// A sample is classified as an outlier if its offset is more than MADMultiple * MAD away from
	// the median offset. Higher values make outlier detection more conservative (fewer rejections).
	MADMultiple float64 `toml:"madMultiple" check:">0.0,<1000.0" comment:"MAD multiple for outlier threshold"`

	// MADMinSamples is the minimum number of samples required in the MAD window before MAD-based
	// outlier detection is active. Until this threshold is reached, only the OutlierThreshold
	// is checked. This ensures MAD statistics are based on sufficient data.
	MADMinSamples int `toml:"madMinSamples" check:">=3,<100" comment:"Min samples before MAD detection active"`

	// OutlierThreshold is the absolute offset in nanoseconds above which a sample is unconditionally treated
	// as an outlier. Above this threshold, MAD detection is bypassed and the sample is rejected outright.
	// During MAD warmup, this is the only outlier check performed.
	OutlierThreshold int64 `toml:"outlierThreshold" check:">0,<=1_000_000" comment:"Unconditional outlier threshold (ns)"`

	// PulseWidthTolerance is the tolerance in nanoseconds for filtering trailing edges in
	// dual-edge mode based on temporal spacing. An edge is considered a trailing edge (and
	// ignored) if it arrives within PulseWidthTolerance of the expected trailing edge time
	// (lastEdgeTime + PulseWidth). This helps distinguish leading from trailing edges when
	// alignment alone is ambiguous.
	PulseWidthTolerance int64 `toml:"pulseWidthTolerance" check:">0,<=10000" comment:"Tolerance for trailing edge filter (ns)"`

	// AlignTolerance is the tolerance in nanoseconds for offset from top of second to
	// immediately accept an edge as a leading edge. Edges with |offset| <= AlignTolerance
	// (where offset is timestamp rounded to nearest second) are assumed to be leading edges
	// and accepted without further checks. This is the primary discriminator for
	// well-synchronized clocks.
	AlignTolerance int64 `toml:"alignTolerance" check:">0,<=10000" comment:"Tolerance for leading edge detection (ns)"`

	// BadSampleRunLimit is the maximum number of consecutive bad samples (missing or outlier)
	// allowed while remaining in tracking mode. Exceeding this limit triggers a reset.
	BadSampleRunLimit int `toml:"badSampleRunLimit" check:">=1,<1000" comment:"Max consecutive bad samples before reset"`

	// OutlierRatioLimit is the maximum ratio of MAD window samples that can be outliers
	// before triggering a reset. Only counts samples admitted to the MAD window and later
	// classified as outliers by MAD detection; extreme outliers rejected by OutlierThreshold
	// are not counted. This prevents the median from being corrupted by accumulated outliers.
	OutlierRatioLimit float64 `toml:"outlierRatioLimit" check:">0.0,<=1.0" comment:"Max outlier ratio in MAD window"`

	// BadSampleWindow is the size of the sliding window for tracking bad sample frequency.
	// Used with BadSampleRatioLimit to detect intermittent failures.
	BadSampleWindow int `toml:"badSampleWindow" check:">=1,<1000" comment:"Window size for bad sample frequency"`

	// BadSampleRatioLimit is the maximum ratio of bad samples allowed in the bad sample
	// window before triggering a reset. Detects intermittent failures even when not consecutive.
	BadSampleRatioLimit float64 `toml:"badSampleRatioLimit" check:">0.0,<=1.0" comment:"Max bad sample ratio in window"`

	// AvgFreqTimeConstant is the time constant in seconds for the exponential moving
	// average of frequency adjustments. This average represents the baseline frequency
	// correction (without phase correction) and is used when samples are missing.
	// Larger values track baseline frequency more smoothly but respond more slowly to
	// oscillator drift. The EMA is updated as: avgFreq = alpha*freq + (1-alpha)*avgFreq
	// where alpha = 1 - exp(-sampleInterval/timeConstant). Set to 0 to disable this
	// feature (no frequency adjustment on missing samples).
	AvgFreqTimeConstant float64 `toml:"avgFreqTimeConstant" check:">=0.0,<1000.0" comment:"EMA time constant for avg frequency (s)"`

	// IgnoreSawtoothCorrection, when true, disables the use of pulse offset corrections
	// from PrePulse messages. This is primarily for testing to verify that sawtooth
	// correction improves synchronization accuracy. Default: false (use corrections).
	IgnoreSawtoothCorrection bool `toml:"ignoreSawtoothCorrection" comment:"Ignore sawtooth corrections (for testing)"`

	// PulseCorrectionTimeout is maximum time in seconds to wait for a PostPulse correction
	// message after the pulse occurs.
	// The upper limit is set the 1 second minus the tick interval, which is 0.25s.
	PulseCorrectionTimeout float64 `toml:"pulseCorrectionTimeout" check:">0,<0.75" comment:"Max wait for pulse correction msg (s)"`
}

func defaultTrackingConfig() TrackingConfig {
	return TrackingConfig{
		Kp:                     0.8,
		Ki:                     0.3,
		MADThreshold:           50,   // 50ns
		MADWindow:              20,   // larger window for more robust outlier detection
		MADMultiple:            25.0, // 25 * MAD
		MADMinSamples:          10,   // 10 samples minimum
		OutlierThreshold:       500,  // 500ns
		PulseWidthTolerance:    200,  // 200ns
		AlignTolerance:         100,  // 100ns
		BadSampleRunLimit:      5,    // consider increasing in the future, when tests show it is safe
		OutlierRatioLimit:      0.3,  // >30% outliers triggers reset
		BadSampleWindow:        60,   // 1 minute window
		BadSampleRatioLimit:    0.5,  // >50% bad in window triggers reset
		AvgFreqTimeConstant:    30,   // 30 seconds
		PulseCorrectionTimeout: 0.3,  // 300ms
	}
}

type trackingSampleGenerator struct {
	cfg          TrackingConfig
	pt           PulseType
	freq         float64
	maxFreq      float64
	lg           *slog.Logger
	lastSample   *Sample // last accepted sample, used for edge filtering
	timeMsgBuf   TimeMsgBuffer
	pendingPulse *struct {
		PulseEdge
		edgeIndex uint64
	}
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
	if g.pendingPulse != nil {
		// this shouldn't ever happen, because we have clamped PulseCorrectionTimeout to < 0.75s
		// (1s minus tick interval of 0.25s)
		g.lg.Warn("new pulse arrived while waiting for post-pulse correction")
		g.pendingPulse = nil
	}
	sample := g.pulseSample(edge, edgeIndex, g.cfg.IgnoreSawtoothCorrection)
	if sample == nil {
		pendingPulse := &struct {
			PulseEdge
			edgeIndex uint64
		}{
			PulseEdge: edge,
			edgeIndex: edgeIndex,
		}
		g.pendingPulse = pendingPulse
	}
	return sample
}

func (g *trackingSampleGenerator) timeMessageSample() *Sample {
	if g.pendingPulse == nil {
		return nil
	}
	sample := g.pulseSample(g.pendingPulse.PulseEdge, g.pendingPulse.edgeIndex, g.cfg.IgnoreSawtoothCorrection)
	if sample != nil {
		g.pendingPulse = nil
	}
	return sample
}

func (g *trackingSampleGenerator) tickSample(now time.Time) *Sample {
	if g.pendingPulse == nil {
		return nil
	}
	// if we haven't yet reached the deadline, keep waiting
	if !now.After(g.pendingPulseDeadline()) {
		return nil
	}
	// we have passed the deadline
	sample := g.pulseSample(g.pendingPulse.PulseEdge, g.pendingPulse.edgeIndex, true)
	g.pendingPulse = nil
	return sample
}

func (g *trackingSampleGenerator) pulseSample(edge PulseEdge, edgeIndex uint64, ignoreCorr bool) *Sample {
	// Round to nearest second
	sec := edge.Timestamp.T.Round(time.Second)

	// Apply pulse offset correction if available (unless disabled by config)
	refTime := sec
	if !ignoreCorr {
		pulseOffset, ok := g.timeMsgBuf.GetPulseCorrection(sec)
		if ok {
			// PulseOffset satisfies: true_second = pulse + PulseOffset
			// We want GPS time of pulse: pulse = true_second - PulseOffset
			// sec approximates true_second, so: pulse_GPS_time = sec - PulseOffset
			refTime = sec.Add(-pulseOffset)
			g.lg.Debug("applied pulse offset correction",
				"sec", sec,
				"pulseOffset_ns", pulseOffset.Nanoseconds(),
				"refTime", refTime)
		} else if g.timeMsgBuf.WaitForPulseCorrection(sec) {
			return nil
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

// pendingPulseDeadline returns the system time deadline for processing a pending pulse.
// Returns zero time if no pulse is pending.
// The deadline is computed as the estimated system time of the pulse plus PulseCorrectionTimeout.
func (g *trackingSampleGenerator) pendingPulseDeadline() time.Time {
	if g.pendingPulse == nil {
		return time.Time{}
	}
	// Estimate system time when pulse occurred
	phcDelta := g.pendingPulse.TRead.Clock.T.Sub(g.pendingPulse.Timestamp.T)
	pulseSysTime := g.pendingPulse.TRead.Sys.Add(-phcDelta)
	return pulseSysTime.Add(time.Duration(g.cfg.PulseCorrectionTimeout * float64(time.Second)))
}

type trackingSampleProcessor struct {
	servo                 *piServo
	cfg                   TrackingConfig
	consecutiveBadSamples int
	avgFreq               float64 // exponential moving average of frequency
	emaAlpha              float64 // EMA coefficient (1 - exp(-1/timeConstant))
	madWindow             *madWindow
	badSamples            *circbuf.Buffer[bool]
	badSampleCount        int
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
		servo:      newPiServo(currentFreq, cfg.Kp, cfg.Ki, maxFreq),
		cfg:        cfg,
		avgFreq:    currentFreq, // initialize with current frequency
		emaAlpha:   emaAlpha,
		madWindow:  newMADWindow(cfg.MADWindow),
		badSamples: circbuf.New[bool](cfg.BadSampleWindow),
		lg:         lg,
	}
}

func (p *trackingSampleProcessor) processSample(sample *Sample) (phcAction, Mode) {
	action := p.sampleAction(sample)

	// Track bad samples (missing or outlier - i.e., not sent to servo)
	isBad := action.actionType == phcNoAction
	if isBad {
		p.consecutiveBadSamples++
		// Use EMA feature (adjusting on bad samples), if not disabled by AvgFreqTimeConstant = 0
		if p.consecutiveBadSamples == 1 && p.cfg.AvgFreqTimeConstant > 0 {
			action.actionType = phcAdjustFrequency
			action.freq = p.avgFreq
			p.servo.reset(p.avgFreq)
			p.lg.Debug("first bad sample; switching to average frequency", "avgFreq", p.avgFreq)
		}
	} else {
		p.consecutiveBadSamples = 0
	}

	// Track bad sample frequency in sliding window
	if p.badSamples.Len() == p.badSamples.Cap() {
		if p.badSamples.Last(p.badSamples.Len() - 1) {
			p.badSampleCount--
		}
	}
	p.badSamples.Append(isBad)
	if isBad {
		p.badSampleCount++
	}

	// Check all four limits for transition to reset mode
	// Limit semantics: a limit of N means N is allowed (use > not >=)

	// Limit 1: consecutive bad samples
	if p.consecutiveBadSamples > p.cfg.BadSampleRunLimit {
		p.lg.Info("entering reset mode", "reason", "consecutiveBadSamples", "count", p.consecutiveBadSamples)
		return action, ModeReset
	}

	// Limit 2: outlier ratio in MAD window
	// Only check when MAD detection is active (after warmup)
	if p.madWindow.Len() >= p.cfg.MADMinSamples {
		outlierRatio := float64(p.madWindow.OutlierCount()) / float64(p.madWindow.Len())
		if outlierRatio > p.cfg.OutlierRatioLimit {
			p.lg.Info("entering reset mode", "reason", "outlierRatio", "ratio", outlierRatio)
			return action, ModeReset
		}
	}

	// Limit 3: bad sample ratio in bad sample window
	// Only check when window is full - this limit detects persistent intermittent failures
	if p.badSamples.Len() == p.badSamples.Cap() {
		badRatio := float64(p.badSampleCount) / float64(p.badSamples.Len())
		if badRatio > p.cfg.BadSampleRatioLimit {
			p.lg.Info("entering reset mode", "reason", "badSampleRatio", "ratio", badRatio)
			return action, ModeReset
		}
	}

	return action, ModeTracking
}

func (p *trackingSampleProcessor) sampleAction(sample *Sample) phcAction {
	if sample.Kind == SampleMissing {
		return phcAction{actionType: phcNoAction}
	}

	absOffset := sample.Offset.Abs()

	// Upper gate: unconditionally reject and exclude from MAD window
	if absOffset >= time.Duration(p.cfg.OutlierThreshold) {
		sample.Kind = SampleOutlier
		p.lg.Debug("extreme outlier rejected unconditionally",
			"offset", sample.Offset,
			"outlierThreshold", p.cfg.OutlierThreshold)
		return phcAction{actionType: phcNoAction}
	}

	// Add to MAD window (only samples below OutlierThreshold)
	p.madWindow.Add(sample.Offset)

	// After MAD warmup, use MAD-based detection for samples above MADThreshold
	if p.madWindow.Len() >= p.cfg.MADMinSamples &&
		absOffset >= time.Duration(p.cfg.MADThreshold) &&
		p.madIsOutlier(sample.Offset) {
		p.madWindow.SetLastOutlier()
		sample.Kind = SampleOutlier
		p.lg.Debug("outlier detected by MAD",
			"offset", sample.Offset)
		return phcAction{actionType: phcNoAction}
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
	if p.madWindow.Len() < p.cfg.MADMinSamples {
		return false
	}

	// Compute median of offsets (the center, can be non-zero during transients)
	center := p.madWindow.Median()

	// Compute MAD: median of absolute deviations from center
	// Use iterator to avoid materializing intermediate slice unnecessarily
	mad := median.Median(func(yield func(time.Duration) bool) {
		p.madWindow.Iterate(func(i int, v time.Duration) bool {
			return yield((v - center).Abs())
		})
	})

	// Check if offset deviates too much from center
	threshold := time.Duration(float64(mad) * p.cfg.MADMultiple)
	deviation := (offset - center).Abs()

	return deviation > threshold
}

// madWindow tracks offsets for MAD-based outlier detection and counts
// how many samples in the window are outliers.
type madWindow struct {
	*median.Window[time.Duration]
	outliers     *circbuf.Buffer[bool]
	outlierCount int
}

func newMADWindow(capacity int) *madWindow {
	return &madWindow{
		Window:   median.New[time.Duration](capacity),
		outliers: circbuf.New[bool](capacity),
	}
}

// Add adds an offset to the window. The sample is initially marked as not an outlier.
// Call SetLastOutlier after MAD detection to mark it as an outlier if needed.
func (w *madWindow) Add(offset time.Duration) {
	// Decrement count if evicting an outlier
	if w.outliers.Len() == w.outliers.Cap() {
		if w.outliers.Last(w.outliers.Len() - 1) {
			w.outlierCount--
		}
	}
	w.Window.Add(offset)
	w.outliers.Append(false)
}

// SetLastOutlier marks the most recently added sample as an outlier.
func (w *madWindow) SetLastOutlier() {
	if w.outliers.Len() == 0 {
		return
	}
	w.outliers.SetLast(0, true)
	w.outlierCount++
}

// OutlierCount returns the number of outliers currently in the window.
func (w *madWindow) OutlierCount() int {
	return w.outlierCount
}
