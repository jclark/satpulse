package phcsync

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jclark/satpulse/internal/combine"
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
type PulseType = combine.PulseType

// Sample kind constants
const (
	SampleOK      = mon.SampleOK
	SampleMissing = mon.SampleMissing
	SampleOutlier = mon.SampleOutlier
)

// Sample represents a sample with associated edge index.
// The edgeIndex tracks which edge produced this sample (odd/even).
type Sample struct {
	*SampleData
	edgeIndex uint64
	Sys       time.Time // estimated monotonic system time of pulse
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
	GetPostTimeMessages(n int) (lastSec ptime.Time, tRead []time.Time)
}

// Config contains tunable parameters for the Controller.
type Config struct {
	Reset      ResetConfig
	Converging ConvergingConfig
	Tracking   TrackingConfig
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		Reset:      defaultResetConfig(),
		Converging: defaultConvergingConfig(),
		Tracking:   defaultTrackingConfig(),
	}
}

// Mode represents the mode in which the Controller is operating.
type Mode int

const (
	ModeInvalid Mode = iota
	ModeReset
	ModeConverging
	ModeTracking
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

// Controller coordinates PHC synchronization.
type Controller struct {
	clock          Clock
	timeMsgBuffer  TimeMsgBuffer
	sampler        Sampler
	gm             *Grandmaster
	rc             *ProxyRefClock
	cfg            Config
	leapSecond     ptime.LeapSecond
	pulseWidthSpec time.Duration // configured pulse width, immutable
	pt             PulseType     // discovered/working, mutable
	mode           Mode
	lg             *slog.Logger
	freq           float64 // current frequency adjustment in PPB
	estimatedFreq  float64 // estimated correct frequency in PPB (from reset mode)
	maxFreq        float64 // maximum frequency adjustment in PPB
	edgeIndex      uint64  // increments on each PulseEdge call, tracks odd/even
	sampleGen      sampleGenerator
	sampleProc     sampleProcessor
	lastRefTime    ptime.Time // last reference time from a real sample
	lastSample     *Sample    // last real sample
	era            ptime.Era  // current PHC era
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
func NewController(
	clock Clock,
	timeMsgBuffer TimeMsgBuffer,
	sampler Sampler,
	gm *Grandmaster,
	rc *ProxyRefClock,
	cfg Config,
	leapSecond ptime.LeapSecond,
	pt PulseType,
	lg *slog.Logger,
) (*Controller, error) {
	c := &Controller{
		clock:          clock,
		timeMsgBuffer:  timeMsgBuffer,
		sampler:        sampler,
		gm:             gm,
		rc:             rc,
		cfg:            cfg,
		leapSecond:     leapSecond,
		pulseWidthSpec: pt.PulseWidth,
		pt:             pt,
		lg:             lg,
	}
	freq, err := clock.FreqOffset()
	if err != nil {
		return nil, err
	}
	c.freq = freq
	c.maxFreq = clock.MaxFreqOffset()
	c.changeMode(ModeReset)
	return c, nil
}

type PulseEdge struct {
	Timestamp ptime.ClockTime // PHC clock timestamp for the pulse edge
	TRead     time.Time       // system time immediately after the timestamp event was read
	TReadPHC  ptime.ClockTime // PHC time immediately after the timestamp event was read
}

// PulseEdge handles edge timestamp events from the PHC.
func (c *Controller) PulseEdge(edge PulseEdge) {
	sample := c.sampleGen.pulseEdgeSample(edge, c.edgeIndex)
	c.edgeIndex++
	c.processPresentSample(sample)
}

// TimeMessage handles notification that a time message occurred.
func (c *Controller) TimeMessage() {
	sample := c.sampleGen.timeMessageSample()
	c.processPresentSample(sample)
}

// processPresentSample handles samples from actual events (not missing).
func (c *Controller) processPresentSample(sample *Sample) {
	if sample == nil {
		return
	}
	// Update time tracking for real samples
	c.lastRefTime = sample.Ref
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
	c.sampler.Sample(*sample.SampleData)
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
func (c *Controller) Tick(now time.Time) {
	// Don't generate missing samples in reset mode
	if c.mode == ModeReset {
		return
	}

	// Check if we're overdue for a sample
	if now.Sub(c.lastSample.Sys) <= sampleIntervalMax {
		return
	}

	// Advance time by exactly one second from last sample
	sys := c.lastSample.Sys.Add(time.Second)
	c.lastRefTime = c.lastRefTime.Add(time.Second)

	// Create missing sample
	sample := &Sample{
		SampleData: &SampleData{
			Kind:   mon.SampleMissing,
			Ref:    c.lastRefTime,
			Offset: 0,
			Era:    c.era,
			// Freq will be filled in later
		},
		edgeIndex: 0, // Missing samples don't have an edge index
		Sys:       sys,
	}

	// Update lastSample to the missing sample so we don't generate duplicates
	c.lastSample = sample

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

func (c *Controller) changeMode(mode Mode) {
	if c.mode == mode {
		return
	}
	if c.mode != ModeInvalid {
		c.lg.Info("changing mode",
			"from", c.mode.String(),
			"to", mode.String(),
		)
	}

	// Extract pulse info when leaving reset mode
	if c.mode == ModeReset {
		if rsg, ok := c.sampleGen.(*resetSampleGenerator); ok {
			c.notePulseInfo(rsg.getPulseInfo())
		}
	}

	// Initialize sampleGen and sampleProc for the new mode
	switch mode {
	case ModeReset:
		// Reset pulse width to configured value when entering reset mode
		c.pt.PulseWidth = c.pulseWidthSpec
		c.sampleGen = newResetSampleGenerator(c.timeMsgBuffer, c.cfg.Reset, c.pt, c.freq, c.maxFreq, c.lg)
		c.sampleProc = newResetSampleProcessor(c.cfg.Reset, c.lg)
	case ModeConverging:
		c.sampleGen = newConvergingSampleGenerator(c.cfg.Converging, c.pt, c.lastSample, c.freq, c.maxFreq, c.lg)
		c.sampleProc = newConvergingSampleProcessor(c.cfg.Converging, c.freq, c.maxFreq, c.lg)
	case ModeTracking:
		c.sampleGen = newTrackingSampleGenerator(c.cfg.Tracking, c.pt, c.lastSample, c.freq, c.maxFreq, c.lg)
		c.sampleProc = newTrackingSampleProcessor(c.cfg.Tracking, c.estimatedFreq, c.maxFreq, c.lg)
	default:
		panic("changing to invalid mode")
	}

	c.mode = mode

	if c.gm != nil {
		c.gm.SetClockSync(modeSyncState(mode))
	}
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

func modeSyncState(mode Mode) mon.SyncState {
	if mode == ModeTracking {
		return mon.InSync
	}
	return mon.NoSync
}

// Mode returns the current operating mode of the controller.
func (c *Controller) Mode() Mode {
	return c.mode
}
