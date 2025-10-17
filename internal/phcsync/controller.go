package phcsync

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/mon"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/servo"
)

// Type aliases for existing interfaces that will eventually move to phcsync
type Clock = servo.Clock
type Sampler = mon.Sampler
type SampleData = mon.SampleData
type SampleKind = mon.SampleKind
type Grandmaster = mon.Grandmaster
type ProxyRefClock = mon.ProxyRefClock

// Sample kind constants
const (
	SampleOK      = mon.SampleOK
	SampleMissing = mon.SampleMissing
	SampleOutlier = mon.SampleOutlier
)

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
	GetPostTimeMessages(n int) (lastSec ptime.Time, tRead []time.Time)
}

// Config contains tunable parameters for the Controller.
type Config struct {
	Init       InitConfig
	Converging ConvergingConfig
	Tracking   TrackingConfig
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		Init:       defaultInitConfig(),
		Converging: defaultConvergingConfig(),
		Tracking:   defaultTrackingConfig(),
	}
}

// controllerMode represents the mode in which the Controller is operating.
type controllerMode int

const (
	modeInvalid controllerMode = iota
	modeInit
	modeConverging
	modeTracking
	modeLost
)

// sampleIntervalMax is the maximum time to wait for a sample before generating a missing sample.
// Same as sampleIntervalMax in mon package.
const sampleIntervalMax = (3 * time.Second) / 2

func (m controllerMode) String() string {
	switch m {
	case modeInit:
		return "init"
	case modeConverging:
		return "converging"
	case modeTracking:
		return "tracking"
	case modeLost:
		return "lost"
	default:
		return fmt.Sprintf("unknown(%d)", m)
	}
}

// Controller coordinates PHC synchronization.
type Controller struct {
	clock          Clock
	timeMsgBuffer  TimeMsgBuffer
	sampler        Sampler
	gm             *Grandmaster
	rc             *ProxyRefClock
	cfg            Config
	leapSecond     ptime.LeapSecond
	mode           controllerMode
	lg             *slog.Logger
	freq           float64 // current frequency adjustment in PPB
	maxFreq        float64 // maximum frequency adjustment in PPB
	sampleGen      sampleGenerator
	sampleProc     sampleProcessor
	lastRefTime    ptime.Time // last reference time from a real sample
	lastSampleTime time.Time  // system time when last real sample was processed
	era            ptime.Era  // current PHC era
}

type sampleGenerator interface {
	pulseEdgeSample(PulseEdge) *SampleData
	timeMessageSample() *SampleData
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
	processSample(*SampleData) (phcAction, controllerMode)
}

// NewController creates a new Controller instance.
func NewController(
	clock Clock,
	timeMsgBuffer TimeMsgBuffer,
	sampler Sampler,
	gm *Grandmaster,
	rc *ProxyRefClock,
	cfg Config,
	leapSecond ptime.LeapSecond,
	lg *slog.Logger,
) (*Controller, error) {
	c := &Controller{
		clock:         clock,
		timeMsgBuffer: timeMsgBuffer,
		sampler:       sampler,
		gm:            gm,
		rc:            rc,
		cfg:           cfg,
		leapSecond:    leapSecond,
		lg:            lg,
	}
	freq, err := clock.FreqOffset()
	if err != nil {
		return nil, err
	}
	c.freq = freq
	c.maxFreq = clock.MaxFreqOffset()
	c.changeMode(modeInit)
	return c, nil
}

type PulseEdge struct {
	Timestamp ptime.ClockTime // PHC clock timestamp for the pulse edge
	TRead     time.Time       // system time immediately after the timestamp event was read
	TReadPHC  ptime.ClockTime // PHC time immediately after the timestamp event was read
}

// PulseEdge handles edge timestamp events from the PHC.
func (c *Controller) PulseEdge(edge PulseEdge) {
	sample := c.sampleGen.pulseEdgeSample(edge)
	c.processPresentSample(sample)
}

// TimeMessage handles notification that a time message occurred.
func (c *Controller) TimeMessage() {
	sample := c.sampleGen.timeMessageSample()
	c.processPresentSample(sample)
}

// processPresentSample handles samples from actual events (not missing).
func (c *Controller) processPresentSample(sample *SampleData) {
	if sample == nil {
		return
	}
	// Update time tracking for real samples
	c.lastRefTime = sample.Ref
	c.lastSampleTime = time.Now()

	// Process through common path
	c.processSample(sample)
}

func (c *Controller) processSample(sample *SampleData) {
	action, mode := c.sampleProc.processSample(sample)
	freq := c.freq
	c.doPHCAction(action)
	sample.Freq = c.freq
	sample.FreqDelta = c.freq - freq
	c.sampler.Sample(*sample)
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
}

// Tick handles regular tick events (0.25s intervals).
func (c *Controller) Tick() {
	// Don't generate missing samples in init mode
	if c.mode == modeInit {
		return
	}

	// Check if we're overdue for a sample
	if time.Since(c.lastSampleTime) <= sampleIntervalMax {
		return
	}

	// Advance time by exactly one second
	c.lastSampleTime = c.lastSampleTime.Add(time.Second)
	c.lastRefTime = c.lastRefTime.Add(time.Second)

	// Create missing sample
	sample := &SampleData{
		Kind:   mon.SampleMissing,
		Ref:    c.lastRefTime,
		Offset: 0,
		Era:    c.era,
		// Freq will be filled in later
	}

	// Missing samples go directly to processSample
	c.processSample(sample)
}

// Close gracefully shuts down the controller.
func (c *Controller) Close() {
	c.lg.Debug("closing phcsync controller")

	if c.gm != nil {
		c.gm.SetClockSync(mon.NoSync)
		c.gm.Close()
	}

	if c.rc != nil {
		c.rc.Close()
	}
}

func (c *Controller) changeMode(mode controllerMode) {
	if c.mode == mode {
		return
	}
	if c.mode != modeInvalid {
		c.lg.Info("changing mode",
			"from", c.mode.String(),
			"to", mode.String(),
		)
	}
	// Initialize sampleGen and sampleProc for the new mode
	switch mode {
	case modeInit:
		c.sampleGen = newInitSampleGenerator(c.timeMsgBuffer, c.cfg.Init, c.maxFreq, c.freq, c.lg)
		c.sampleProc = newInitSampleProcessor(c.cfg.Init, c.lg)
	case modeConverging:
		c.sampleGen = newConvergingSampleGenerator()
		c.sampleProc = newConvergingSampleProcessor(c.cfg.Converging, c.freq, c.maxFreq, c.lg)
	case modeTracking:
		c.sampleGen = newTrackingSampleGenerator()
		c.sampleProc = newTrackingSampleProcessor(c.cfg.Tracking, c.freq, c.maxFreq, c.lg)
	case modeLost:
		c.sampleGen = newLostSampleGenerator()
		c.sampleProc = newLostSampleProcessor()
	default:
		panic("changing to invalid mode")
	}

	c.mode = mode

	if c.gm != nil {
		c.gm.SetClockSync(modeSyncState(mode))
	}
}

func modeSyncState(mode controllerMode) mon.SyncState {
	if mode == modeTracking {
		return mon.InSync
	}
	return mon.NoSync
}

// Tracking reports whether the controller is currently operating in tracking mode.
func (c *Controller) Tracking() bool {
	return c.mode == modeTracking
}
