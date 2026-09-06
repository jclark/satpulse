package phcsync

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/lib/check"
	"github.com/jclark/satpulse/gps/ptime"
	"github.com/jclark/satpulse/time/internal/ptpgm"
	"github.com/jclark/satpulse/time/phctime"
)

// Clock interface represents a PHC (PTP Hardware Clock) that can be adjusted.
type Clock interface {
	SetFreqOffset(float64) error
	FreqOffset() (float64, error)
	MaxFreqOffset() float64
	AdjTime(d time.Duration) (phctime.Era, error)
}

// PulseType describes the pulse characteristics.
type PulseType struct {
	EdgesPerPulse int
	PulseWidth    time.Duration
}

// TimeMsgBuffer is an interface for the Controller to access time messages from the receiver.
type TimeMsgBuffer interface {
	// GetPostTimeMessages retrieves n time messages.
	// It returns the reference time of the last message and
	// the read times of all the messages in chronological order.
	// The read times must have a valid monotonic part.
	// The messages must be for consecutive seconds;
	// the reference time of each time message is one greater than the previous one.
	// The last message must not be stale i.e. there must not be a time message of the same type with a later reference time.
	// The messages must be the same GNSS message type, which must be of a type that follows the time pulse.
	// If n such messages are not available, the slice will be empty and lastSec will be zero.
	// detectLeap is a closure for the invalid-leap-second check on the
	// selected message type; non-nil only when those messages are
	// UTC-source.
	GetPostTimeMessages(n int) (lastSec ptime.Time, tRead []time.Time, detectLeap func(minDelta time.Duration) bool)
	// GetPulseCorrection retrieves the pulse offset correction (PulseOffset) for a given reference time.
	// The returned correction satisfies: true_time_of_second = pulse_time + correction
	// Returns (correction, true) if available, (0, false) otherwise.
	GetPulseCorrection(refTime ptime.Time) (time.Duration, bool)
	WaitForPulseCorrection(refTime ptime.Time) bool
}

// Config contains tunable parameters for the Controller.
type Config struct {
	Reset    ResetConfig      `toml:"reset" comment:"Reset mode parameters"`
	Converge ConvergingConfig `toml:"converge" comment:"Converging mode parameters"`
	Track    TrackingConfig   `toml:"track" comment:"Tracking mode parameters"`
	Share    SharedConfig     `toml:"share" comment:"Cross-mode parameters"`
}

// SharedConfig contains tunable parameters for cross-mode checks.
type SharedConfig struct {
	// DetectInvalidLeap enables the invalid-leap-second detector that
	// runs on UTC-source time messages after reset mode completes.
	// When false the detector is skipped.
	DetectInvalidLeap bool `toml:"detectInvalidLeap" comment:"Enable detection of invalid leap seconds after reset mode completes"`

	// MinInvalidRepeatInterval is the minimum read-time interval (s)
	// between two same-type messages reporting identical UTC for the
	// repeat to be declared an invalid leap-second event (a stale-
	// firmware correction) rather than a receiver glitch.
	MinInvalidRepeatInterval float64 `toml:"minInvalidRepeatInterval" check:">0,<1.0" comment:"Min repeat interval to declare invalid leap (s)"`
}

func defaultSharedConfig() SharedConfig {
	return SharedConfig{
		DetectInvalidLeap:        true,
		MinInvalidRepeatInterval: 0.8,
	}
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		Reset:    defaultResetConfig(),
		Converge: defaultConvergingConfig(),
		Track:    defaultTrackingConfig(),
		Share:    defaultSharedConfig(),
	}
}

// Mode represents the mode in which the Controller is operating.
type Mode int

const (
	ModeInvalid Mode = iota
	ModeReset
	ModeConverging
	ModeTracking
	NModes
)

// sampleIntervalMax is the maximum time to wait for a sample before generating a missing sample.
// Same as sampleIntervalMax in mon package.
const sampleIntervalMax = (3 * time.Second) / 2

func (m Mode) String() string {
	switch m {
	case ModeReset:
		return "reset"
	case ModeConverging:
		return "converging"
	case ModeTracking:
		return "tracking"
	default:
		return fmt.Sprintf("unknown(%d)", m)
	}
}

// InSync returns true if the mode represents a synchronized state
func (m Mode) InSync() bool {
	return m == ModeTracking
}

// Controller coordinates PHC synchronization.
type Controller struct {
	clock         Clock
	timeMsgBuffer TimeMsgBuffer
	observer      Observer
	gm            *ptpgm.Grandmaster
	cfg           Config
	leapSecond    ptime.LeapSecond
	pt            PulseType // discovered/working, mutable
	mode          Mode
	lg            *slog.Logger
	freq          float64 // current frequency adjustment in PPB
	estimatedFreq float64 // estimated correct frequency in PPB (from reset mode)
	maxFreq       float64 // maximum frequency adjustment in PPB
	edgeIndex     uint64  // increments on each PulseEdge call, tracks odd/even
	sampleGen     sampleGenerator
	sampleProc    sampleProcessor
	lastSample    *Sample                  // last sample (real or missing)
	era           phctime.Era              // current PHC era
	detectLeap    func() bool              // installed on reset->converging with the configured threshold baked in; nil while in reset mode, when the selected time messages are TAI-source, or when DetectInvalidLeap is disabled
}

type sampleGenerator interface {
	pulseEdgeSample(PulseEdge, uint64) *Sample
	timeMessageSample() *Sample
}

type phcActionType int

const (
	phcNoAction phcActionType = iota
	phcAdjustFrequency
	phcStepClock
)

type phcAction struct {
	actionType phcActionType
	step       time.Duration // add this to the time, valid if actionType is phcStepClock
	freq       float64       // change freq adjustment to this, valid if actionType is phcAdjustFrequency
}

type sampleProcessor interface {
	// processSample processes a sample and returns the action to take on the PHC and the mode to be in.
	processSample(*Sample) (phcAction, Mode)
}

// NewController creates a new Controller instance.
// The Config must be validated before calling this function.
func NewController(
	clock Clock,
	observer Observer,
	gm *ptpgm.Grandmaster,
	cfg Config,
	leapSecond ptime.LeapSecond,
	edgesPerPulse int,
	lg *slog.Logger,
) (*Controller, error) {
	switch edgesPerPulse {
	case 0:
		return nil, errors.New("ethernet driver does not properly support PTP_EXTTS_REQUEST2; report to driver maintainer")
	case 1, 2:
	default:
		panic(fmt.Sprintf("unexpected edges per pulse: %d", edgesPerPulse))
	}
	c := &Controller{
		clock:      clock,
		observer:   observer,
		gm:         gm,
		cfg:        cfg,
		leapSecond: leapSecond,
		pt:         PulseType{EdgesPerPulse: edgesPerPulse},
		lg:         lg,
	}
	freq, err := clock.FreqOffset()
	if err != nil {
		return nil, err
	}
	c.freq = freq
	c.maxFreq = clock.MaxFreqOffset()
	// Mode will be set to ModeReset when SetTimeMsgBuffer is called
	return c, nil
}

type PulseEdge struct {
	Timestamp phctime.Time   // PHC clock timestamp for the pulse edge
	TRead     phctime.Sample // PHC and system time immediately after the timestamp event was read
}

// SetTimeMsgBuffer sets the time message buffer for the controller.
// This must be called exactly once after construction, before any PulseEdge or TimeMessage calls.
func (c *Controller) SetTimeMsgBuffer(buf TimeMsgBuffer) {
	if c.mode != ModeInvalid {
		panic("SetTimeMsgBuffer called after controller already initialized")
	}
	c.timeMsgBuffer = buf
	// Initialize to Reset mode now that we have the timeMsgBuffer
	c.changeMode(ModeReset)
}

// PulseEdge handles edge timestamp events from the PHC.
func (c *Controller) PulseEdge(edge PulseEdge) {
	sample := c.sampleGen.pulseEdgeSample(edge, c.edgeIndex)
	c.edgeIndex++
	c.processPresentSample(sample)
}

// TimeMessage handles notification that a time message occurred.
func (c *Controller) TimeMessage() {
	// The reset triggered here would be rejected by the drift-rate check against
	// the tracking-mode persisted sample (see TrackingConfig.PersistThreshold),
	// since a stale-firmware leap correction looks like ~1s of drift. The default
	// PersistThreshold (900s) exceeds the 12.5-minute GPS leap-subframe worst case,
	// so the correction is detected before any sample is persisted.
	if c.detectLeap != nil && c.detectLeap() {
		c.changeMode(ModeReset)
		return
	}
	sample := c.sampleGen.timeMessageSample()
	c.processPresentSample(sample)
}

// processPresentSample handles samples from actual events (not missing).
func (c *Controller) processPresentSample(sample *Sample) {
	if sample == nil {
		return
	}

	c.lastSample = sample

	// Process through common path
	c.processSample(sample)
}

func (c *Controller) processSample(sample *Sample) {
	c.lg.Debug("processing sample", "mode", c.mode, "kind", sample.Kind, "ref", sample.Ref, "offset", sample.Offset)
	action, mode := c.sampleProc.processSample(sample)
	freq := c.freq
	c.doPHCAction(action)
	sample.Freq = c.freq
	sample.FreqDelta = c.freq - freq
	// Set sample mode to current mode (c.mode), not the next mode (mode).
	// Rationale: The sample represents the state of synchronization at the time it occurred.
	// Even if this sample triggers a mode transition, it should be attributed to the mode
	// that was active when it occurred. This is important for tracking metrics: when we're
	// in tracking mode serving synchronized time to clients, any samples (including missing
	// or outlier samples that cause us to leave tracking) represent the quality of time
	// that clients were receiving while we were still in tracking mode.
	sample.Mode = c.mode
	c.observer.Sample(*sample)
	if mode != c.mode {
		c.changeMode(mode)
	}
}

func (c *Controller) doPHCAction(action phcAction) {
	switch action.actionType {
	case phcStepClock:
		era, err := c.clock.AdjTime(action.step)
		if err != nil {
			c.lg.Error("failed to step clock", "step", action.step, "err", err)
		} else {
			c.era = era
			c.lg.Info("stepped clock", "step", action.step, "era", era)
		}
	case phcAdjustFrequency:
		err := c.clock.SetFreqOffset(action.freq)
		if err != nil {
			c.lg.Error("failed to adjust frequency", "freq", action.freq, "err", err)
		} else {
			c.freq = action.freq
			c.lg.Debug("adjusted frequency", "freq", action.freq)
		}
	case phcNoAction:
		// nothing to do
	}
}

// LeapSecond handles leap second information updates.
func (c *Controller) LeapSecond(ls ptime.LeapSecond) {
	c.leapSecond = ls
	c.lg.Debug("leap second updated", "leapSecond", ls)
	c.gmUpdate()
}

// Pause handles pause events from the timestamp worker.
// This is called when the network interface loses carrier.
// It resets sync state and transitions back to reset mode.
func (c *Controller) Pause() {
	c.lg.Info("controller paused (carrier lost), transitioning to reset mode")
	c.changeMode(ModeReset)
}

// Tick handles regular tick events (0.25s intervals).
func (c *Controller) Tick(now time.Time) {
	defer c.gmRetry(now)
	// Don't generate missing samples in reset mode
	if c.mode == ModeReset {
		return
	}

	// Check for pending pulse timeout in tracking mode
	if c.mode == ModeTracking {
		if tsg, ok := c.sampleGen.(*trackingSampleGenerator); ok {
			sample := tsg.tickSample(now)
			if sample != nil {
				c.processPresentSample(sample)
				return
			}
		}
	}

	// Check if we're overdue for a sample
	if now.Sub(c.lastSample.Sys) <= sampleIntervalMax {
		return
	}

	// Create missing sample
	sample := &Sample{
		Kind:      SampleMissing,
		Ref:       c.lastSample.Ref.Add(time.Second),
		Offset:    0,
		Era:       c.era,
		EdgeIndex: 0, // Missing samples don't have an edge index
		// Advance system time by exactly one second from last sample
		Sys: c.lastSample.Sys.Add(time.Second),
		// Freq and Mode will be filled in later by processSample
	}

	// Update lastSample to the missing sample so we don't generate duplicates
	c.lastSample = sample

	// Missing samples go directly to processSample
	c.processSample(sample)
}

// Close gracefully shuts down the controller.
func (c *Controller) Close() {
	c.lg.Debug("closing phcsync controller")

	// Set mode to reset first, then update grandmaster to NoSync
	c.mode = ModeReset
	c.gmUpdate()

	if c.gm != nil {
		c.gm.Close()
	}
}

func (c *Controller) changeMode(mode Mode) {
	if c.mode == mode {
		return
	}
	old := c.mode
	if old != ModeInvalid {
		c.lg.Info("changing mode",
			"from", old.String(),
			"to", mode.String(),
		)
	}

	// Extract pulse info and the leap-detect closure when leaving reset mode.
	if c.mode == ModeReset {
		if rsg, ok := c.sampleGen.(*resetSampleGenerator); ok {
			c.notePulseInfo(rsg.getPulseInfo())
			c.detectLeap = c.getDetectLeap()
		}
	}

	// Extract persist sample when leaving tracking mode
	var persistSample *phctime.Sample
	if c.mode == ModeTracking {
		if tsp, ok := c.sampleProc.(*trackingSampleProcessor); ok {
			persistSample = tsp.getPersistSample()
		}
	}

	// Initialize sampleGen and sampleProc for the new mode
	switch mode {
	case ModeReset:
		// Reset pulse width when entering reset mode; will be auto-detected
		c.pt.PulseWidth = 0
		c.detectLeap = nil
		c.sampleGen = newResetSampleGenerator(c.timeMsgBuffer, c.cfg.Reset, c.pt.EdgesPerPulse, c.freq, c.maxFreq, c.lg)
		c.sampleProc = newResetSampleProcessor(c.cfg.Reset, persistSample, c.lg)
	case ModeConverging:
		c.sampleGen = newConvergingSampleGenerator(c.cfg.Converge, c.pt, c.lastSample, c.freq, c.maxFreq, c.lg)
		c.sampleProc = newConvergingSampleProcessor(c.cfg.Converge, c.lastSample, c.freq, c.maxFreq, c.lg)
	case ModeTracking:
		c.sampleGen = newTrackingSampleGenerator(c.cfg.Track, c.pt, c.lastSample, c.freq, c.maxFreq, c.timeMsgBuffer, c.lg)
		c.sampleProc = newTrackingSampleProcessor(c.cfg.Track, c.estimatedFreq, c.maxFreq, c.lg)
	default:
		panic("changing to invalid mode")
	}

	c.mode = mode

	c.gmUpdate()
	c.observer.ModeChanged(old, mode)
}

// getDetectLeap builds the parameter-less leap-detection closure to
// install when leaving reset mode. Returns nil when the user has
// disabled the detector, when the active sampleGen is not a
// resetSampleGenerator (defensive), or when the reset generator's
// captured closure is nil (TAI-source lock). The configured threshold
// is baked in so TimeMessage does not need to re-read config.
func (c *Controller) getDetectLeap() func() bool {
	if !c.cfg.Share.DetectInvalidLeap {
		return nil
	}
	rsg, ok := c.sampleGen.(*resetSampleGenerator)
	if !ok {
		return nil
	}
	inner := rsg.getDetectLeap()
	if inner == nil {
		return nil
	}
	minDelta := time.Duration(c.cfg.Share.MinInvalidRepeatInterval * float64(time.Second))
	return func() bool { return inner(minDelta) }
}

func (c *Controller) notePulseInfo(pi pulseInfo) {
	c.pt.PulseWidth = pi.pulseWidth
	// Calculate corrected frequency using the formula from old servo:
	// freqOff = (1e9 + freqOff) * (refPeriod / localPeriod) - 1e9
	ratio := float64(time.Second) / float64(pi.avgInterval)
	c.estimatedFreq = (1e9+c.freq)*ratio - 1e9
	c.lg.Info("estimated correct frequency from reset mode",
		"estimatedFreq", c.estimatedFreq,
		"currentFreq", c.freq,
		"avgInterval", pi.avgInterval,
		"pulseWidth", pi.pulseWidth)
}

// gmRetry runs after the rest of the tick so that a retry does not send
// settings that a mode change in the same tick has just invalidated.
func (c *Controller) gmRetry(now time.Time) {
	if c.gm != nil && c.gm.RetryDue(now) {
		c.gmUpdate()
		c.gm.Retry()
	}
}

func (c *Controller) gmUpdate() {
	if c.gm == nil {
		return
	}
	if c.lastSample == nil {
		return
	}
	ref := c.lastSample.Ref
	if ref.IsZero() {
		return
	}
	c.gm.Update(gmSyncState(c.mode), c.leapSecond.StateAt(ref))
}

func gmSyncState(mode Mode) ptpgm.SyncState {
	if mode == ModeTracking {
		return ptpgm.InSync
	}
	return ptpgm.NoSync
}

// Mode returns the current operating mode of the controller.
func (c *Controller) Mode() Mode {
	return c.mode
}

// RequiredMsgWindow returns the minimum time-message buffer window the
// controller needs based on its current config. Reset mode requests
// PulseWindow consecutive messages at 1 Hz; the +2 s margin absorbs
// message-rate jitter so PulseWindow entries are reliably on hand.
func (c *Controller) RequiredMsgWindow() time.Duration {
	return time.Duration(c.cfg.Reset.PulseWindow+2) * time.Second
}

func (cfg *Config) Validate() error {
	msgs := check.Validate(cfg)
	switch len(msgs) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("in sync table: %s", msgs[0])
	default:
		msgs = append([]string{"errors in sync table:"}, msgs...)
		return errors.New(strings.Join(msgs, "\n\t"))
	}
}
