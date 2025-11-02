// Package syncsim provides a discrete-event simulator for testing the phcsync controller.
//
// # Architecture
//
// The simulator uses an event-driven architecture built on Go 1.23 iterators:
//
//	generatePulseEvents()    ─┐
//	generateMessageEvents()  ─┼─> mergeEvents() ──> for event := range events
//	generateTickEvents()     ─┘
//
// Three event streams are generated independently and merged chronologically:
//   - Pulse events: GPS PPS edges (rising, and trailing for dual-edge mode)
//   - Message events: GPS time messages containing TAI time
//   - Tick events: System ticks every 250ms (like real daemon)
//
// # Key Design Principles
//
// 1. Event generators are declarative - they describe WHAT events happen and WHEN,
// but NOT which events are delivered. The main loop decides delivery based on outages.
//
// 2. Pulse timestamps are ALWAYS read (even during outages) to track PHC drift during
// signal loss. The decision to deliver to the controller happens separately.
//
// 3. Ticks start immediately at t=0.25, modeling real system behavior. Early ticks are
// safe because the controller returns early when in ModeReset.
//
// 4. Mode tracking uses the observer pattern, which sees samples BEFORE mode transitions.
// This correctly attributes samples to the mode that processed them.
//
// 5. Single-edge and dual-edge modes are handled uniformly - the generator produces
// 1 or 2 pulse events per PPS, and the event loop processes them identically.
package syncsim

import (
	"fmt"
	"iter"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/jclark/satpulse/internal/clocksim"
	"github.com/jclark/satpulse/internal/gpsprot"
	"github.com/jclark/satpulse/internal/obs"
	"github.com/jclark/satpulse/internal/phcsync"
	"github.com/jclark/satpulse/internal/ptime"
	"github.com/jclark/satpulse/internal/statsobs"
	"github.com/jclark/satpulse/internal/timemsg"
	"github.com/jclark/satpulse/internal/ubx"
)

// Config holds simulation parameters
type Config struct {
	Duration      float64       // simulation duration in seconds
	PHCFreqOffset float64       // PHC frequency offset in ppb
	PHCFreqDrift  float64       // PHC frequency drift in ppb/day
	PHCNoise      float64       // PHC frequency noise stddev in ppb
	PPSJitter     float64       // PPS timing jitter in nanoseconds
	MinDelay      float64       // minimum pulse delivery delay in seconds
	MaxDelay      float64       // maximum pulse delivery delay in seconds
	MsgDelay      float64       // GPS message delay after pulse in seconds
	MsgJitter     float64       // GPS message delay jitter in seconds
	PulseWidth    float64       // pulse width in seconds (0 for single-edge mode)
	ToggleTimes   []float64     // absolute simulation times to toggle pulse/message delivery on/off
	Outlier       OutlierConfig // PPS outlier injection configuration
	Shift         ShiftConfig   // PPS phase shift configuration
}

// OutlierConfig configures PPS outlier injection for testing.
// Used to test outlier detection algorithms.
type OutlierConfig struct {
	Times  []float64     // seconds at which to inject outliers (rounded to nearest integer)
	Offset time.Duration // magnitude of outlier phase offset
}

// ShiftConfig configures a temporary PPS phase shift for testing.
// Used to simulate ionospheric disturbances or other gradual timing biases.
type ShiftConfig struct {
	StartTime float64       // start time in seconds
	Ramp      time.Duration // ramp up/down duration (symmetric)
	Duration  time.Duration // total duration (includes ramp up + hold + ramp down)
	Shift     time.Duration // phase shift amount
}

// DefaultConfig returns a Config with sensible default values.
func DefaultConfig() Config {
	return Config{
		Duration:      60.0,
		PHCFreqOffset: 2000.0,
		PHCFreqDrift:  -150.0,
		PHCNoise:      20.0,
		PPSJitter:     10.0,
		MinDelay:      5e-6,
		MaxDelay:      250e-6,
		MsgDelay:      0.1,
		MsgJitter:     0.01,
		PulseWidth:    0, // default to single-edge mode
		Outlier: OutlierConfig{
			Offset: 2000 * time.Nanosecond, // 2µs default outlier magnitude
		},
	}
}

// inOutage returns true if time t falls within an outage period.
// Odd-indexed intervals (1, 3, 5...) are outage periods.
func inOutage(t float64, toggleTimes []float64) bool {
	for i, toggle := range toggleTimes {
		if t < toggle {
			return i%2 == 1
		}
	}
	return len(toggleTimes)%2 == 1
}

const (
	tickInterval = 0.25
)

// Event represents a simulation event with a time
type Event struct {
	Time float64
	Type EventType
	Data any
}

type EventType int

const (
	EventPulse EventType = iota
	EventMessage
	EventTick
)

type PulseEventData struct {
	EdgeIdx int
	PPS     float64
}

type MessageEventData struct {
	PPS float64
}

type TickEventData struct {
}

type pulseTimestamp struct {
	timestamp ptime.ClockTime
	trueTime  float64
	tRead     ptime.Sample
	offset    time.Duration
}

type modeObserver struct {
	obs.DefaultObserver
	initSamples       int
	convergingSamples int
	trackingSamples   int
}

func (m *modeObserver) Sample(s phcsync.Sample) {
	switch s.Mode {
	case phcsync.ModeReset:
		m.initSamples++
	case phcsync.ModeConverging:
		m.convergingSamples++
	case phcsync.ModeTracking:
		m.trackingSamples++
	}
}

// Stats holds simulation results
type Stats struct {
	statsobs.Stats                  // embedded - detailed tracking statistics from observer
	SampleCount       int           // total samples fed to controller
	TrackingStdDev    time.Duration // stddev from true time (simulation-only)
	TrackingMean      time.Duration // mean offset from true time (simulation-only)
	TrackingAbsMax    time.Duration // max absolute offset from true time (simulation-only)
	InitSamples       int           // samples processed in reset mode (includes initial sync and recovery)
	ConvergingSamples int           // samples processed in converging mode
	TrackingSamples   int           // samples processed in tracking mode
}

// Simulate runs a phcsync simulation with the given configuration.
// curTime is updated as the simulation progresses, allowing callers to use it for logging.
// It returns statistics about the simulation run.
func Simulate(observers []obs.Observer, phcCfg phcsync.Config, simCfg Config, curTime *time.Time, lg *slog.Logger) (Stats, error) {
	// Validate phcsync configuration
	if err := phcCfg.Validate(); err != nil {
		return Stats{}, err
	}
	// Create oscillator with offset, drift, and noise
	oscs := []clocksim.OscillatorSimulator{
		clocksim.FreqOffset(simCfg.PHCFreqOffset),
		clocksim.WhiteFreqNoise(simCfg.PHCNoise, 42),
	}
	if simCfg.PHCFreqDrift != 0 {
		oscs = append(oscs, clocksim.FreqDrift(simCfg.PHCFreqDrift))
	}
	osc := clocksim.CombineOscillators(oscs...)

	// PHC starts at epoch (1970-01-01T00:00:00 TAI) - way off from GPS
	raw := clocksim.NewRawClock(osc, 0)

	// Build PPS simulator: start with jitter
	ppsSims := []clocksim.PPSSimulator{
		clocksim.JitterPPS(time.Duration(simCfg.PPSJitter)*time.Nanosecond, 123),
	}

	// Add shift if configured
	if simCfg.Shift.Shift != 0 {
		ppsSims = append(ppsSims, clocksim.ShiftPPS(
			simCfg.Shift.StartTime,
			simCfg.Shift.Ramp,
			simCfg.Shift.Duration,
			simCfg.Shift.Shift,
		))
	}

	// Add outliers if configured
	for _, second := range simCfg.Outlier.Times {
		ppsSims = append(ppsSims, clocksim.SingleOutlierPPS(second, simCfg.Outlier.Offset))
	}

	pps := clocksim.CombinePPS(ppsSims...)

	// Prepare dual-edge mode parameters
	var pulseWidth time.Duration
	var trailingEdgeSim clocksim.PPSSimulator
	var pulseType phcsync.PulseType

	if simCfg.PulseWidth > 0 {
		pulseWidth = time.Duration(simCfg.PulseWidth * 1e9)
		trailingEdgeSim = clocksim.JitterPPS(2*time.Nanosecond, 789)
		pulseType = phcsync.PulseType{
			EdgesPerPulse: 2,
			PulseWidth:    pulseWidth,
		}
	} else {
		pulseType = phcsync.PulseType{
			EdgesPerPulse: 1,
			PulseWidth:    0,
		}
	}

	// Virtual clock starts at t=0, max ±500ppm (like Intel i210)
	vclock := clocksim.NewVirtualClock(raw, pps, 0, 500000, pulseWidth, trailingEdgeSim)

	// Test clock with era tracking
	testClock := clocksim.NewTestClock(vclock)

	// Leap second (current value as of 2017)
	ls := ptime.LeapSecond{
		UTCOffBefore:  37,
		UTCOffAfter:   37,
		OffChangeTime: 1483228800, // 2017-01-01
	}

	// Convert start time to TAI once
	tStart, _ := ls.SysToTime(*curTime)

	// Create timemsg.Buffer
	timeMsgBuf := timemsg.NewBuffer(lg, 5*time.Second, ls, gpsprot.GPS)

	// Create mode observer
	modeObs := &modeObserver{}

	// Create internal stats observer
	statsObs := statsobs.NewStatsObserver()

	// Combine all observers
	allObservers := append([]obs.Observer{statsObs, modeObs}, observers...)
	multiObs := obs.NewMultiObserver(allObservers...)

	// Create controller
	ctrl, err := phcsync.NewController(
		testClock,
		multiObs,
		nil, // no grandmaster
		phcCfg,
		ls,
		pulseType,
		lg,
	)
	if err != nil {
		return Stats{}, err
	}
	ctrl.SetTimeMsgBuffer(timeMsgBuf)
	defer ctrl.Close()

	lg.Info("starting phcsync simulation",
		"duration", simCfg.Duration,
		"pulseDelay", "5µs-250µs",
		"msgDelay", simCfg.MsgDelay,
		"phcFreqOffset", simCfg.PHCFreqOffset,
		"phcFreqDrift", simCfg.PHCFreqDrift,
		"phcNoise", simCfg.PHCNoise,
		"ppsJitter", simCfg.PPSJitter,
		"startTime", curTime.Format(time.RFC3339),
		"phcStartTime", 0.0)

	// Generate event streams
	// Note: ticks start at t=0.25, modeling real system behavior where ticks
	// run continuously from the start. Early ticks are safe - see generateTickEvents.
	pulseGen := generatePulseEvents(simCfg, pulseType.EdgesPerPulse)
	msgGen := generateMessageEvents(simCfg)
	tickGen := generateTickEvents(simCfg)

	// Merge and process events
	events := mergeEvents(pulseGen, msgGen, tickGen)

	sampleCount := 0
	stats := &offsetStats{}

	for event := range events {
		// Update current time for logging
		tCur := tStart.Add(time.Duration(event.Time * 1e9))
		*curTime = ls.TimeToSys(tCur)

		// Advance virtual clock to event time
		vclock.AdvanceTo(event.Time)

		// Dispatch based on event type
		switch event.Type {
		case EventPulse:
			data := event.Data.(PulseEventData)

			// ALWAYS read timestamp and calculate offset (even during outages)
			// This tracks how PHC drifts when signal is lost
			pts, err := readPulseTimestamp(event.Time, data, vclock, testClock, tStart, lg)
			if err != nil {
				return Stats{}, err
			}

			// Track offset for rising edge in tracking mode
			if data.EdgeIdx == 0 && ctrl.Mode() == phcsync.ModeTracking {
				stats.add(pts.offset)
			}

			// Only deliver to controller if NOT in outage
			if !inOutage(data.PPS, simCfg.ToggleTimes) {
				deliverPulseToController(pts, data, ctrl, lg)
			}

		case EventMessage:
			data := event.Data.(MessageEventData)

			// Only deliver to controller if NOT in outage
			if !inOutage(data.PPS, simCfg.ToggleTimes) {
				handleMessageEvent(event.Time, data, timeMsgBuf, ctrl, tStart, lg)
				sampleCount++
			}

		case EventTick:
			// Ticks ALWAYS delivered (no outage filtering)
			handleTickEvent(event.Time, ctrl)
		}
	}

	// Get stats from observers
	trackingStats := statsObs.Stats()

	return Stats{
		Stats:             trackingStats,
		SampleCount:       sampleCount,
		TrackingStdDev:    stats.stdDevRounded(),
		TrackingAbsMax:    stats.absMax,
		TrackingMean:      stats.mean(),
		InitSamples:       modeObs.initSamples,
		ConvergingSamples: modeObs.convergingSamples,
		TrackingSamples:   modeObs.trackingSamples,
	}, nil
}

type offsetStats struct {
	count      int64
	sum        time.Duration
	absMax     time.Duration
	sumSquares int64
}

func (s *offsetStats) add(d time.Duration) {
	ns := d.Nanoseconds()
	s.count++
	s.sum += d
	s.absMax = max(d.Abs(), s.absMax)
	s.sumSquares += ns * ns
}

func (s *offsetStats) stdDevRounded() time.Duration {
	std := s.stdDev()
	return time.Duration(math.Round(std))
}

func (s *offsetStats) mean() time.Duration {
	if s.count == 0 {
		return 0
	}
	return time.Duration(math.Round(float64(s.sum) / float64(s.count)))
}

func (s *offsetStats) stdDev() float64 {
	if s.count <= 1 {
		return 0
	}
	n := float64(s.count)
	sumSq := float64(s.sumSquares)
	sum := float64(s.sum.Nanoseconds())
	variance := (sumSq - (sum*sum)/n) / (n - 1)
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}

// generatePulseEvents creates a push-style iterator that yields pulse edge events.
// Generates ALL pulses - does NOT filter by outages (filtering happens in main loop).
func generatePulseEvents(cfg Config, edgesPerPulse int) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		rng := rand.New(rand.NewSource(999))
		for pps := 1.0; pps < cfg.Duration; pps += 1.0 {
			pulseDelay := cfg.MinDelay + rng.Float64()*(cfg.MaxDelay-cfg.MinDelay)
			risingTime := pps + pulseDelay
			if !yield(Event{
				Time: risingTime,
				Type: EventPulse,
				Data: PulseEventData{EdgeIdx: 0, PPS: pps},
			}) {
				return
			}
			if edgesPerPulse == 2 {
				trailingTime := pps + cfg.PulseWidth + pulseDelay
				if !yield(Event{
					Time: trailingTime,
					Type: EventPulse,
					Data: PulseEventData{EdgeIdx: 1, PPS: pps},
				}) {
					return
				}
			}
		}
	}
}

// generateMessageEvents creates a push-style iterator that yields GPS message events.
// Generates ALL messages - does NOT filter by outages (filtering happens in main loop).
func generateMessageEvents(cfg Config) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		rng := rand.New(rand.NewSource(888))
		for pps := 1.0; pps < cfg.Duration; pps += 1.0 {
			msgDelayTime := cfg.MsgDelay + rng.NormFloat64()*cfg.MsgJitter
			if msgDelayTime < 0 {
				msgDelayTime = 0
			}
			msgTime := pps + msgDelayTime
			if !yield(Event{
				Time: msgTime,
				Type: EventMessage,
				Data: MessageEventData{PPS: pps},
			}) {
				return
			}
		}
	}
}

// generateTickEvents creates a push-style iterator that yields tick events.
// Ticks happen every 250ms regardless of outages, starting immediately at t=0.25.
// Note: Early ticks (before first sample) are safe because controller.Tick()
// returns early when in ModeReset, and mode only exits Reset after first sample
// sets lastSample (controller.go:203).
func generateTickEvents(cfg Config) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		for t := tickInterval; t < cfg.Duration; t += tickInterval {
			if !yield(Event{
				Time: t,
				Type: EventTick,
				Data: TickEventData{},
			}) {
				return
			}
		}
	}
}

// mergeEvents takes three event generators and merges them chronologically
// Returns events in time order until all generators are exhausted
func mergeEvents(
	pulseGen iter.Seq[Event],
	msgGen iter.Seq[Event],
	tickGen iter.Seq[Event],
) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		pulseNext, pulseStop := iter.Pull(pulseGen)
		msgNext, msgStop := iter.Pull(msgGen)
		tickNext, tickStop := iter.Pull(tickGen)
		defer pulseStop()
		defer msgStop()
		defer tickStop()
		pulseEvent, pulseOK := pulseNext()
		msgEvent, msgOK := msgNext()
		tickEvent, tickOK := tickNext()
		for pulseOK || msgOK || tickOK {
			var nextEvent Event
			if pulseOK && (!msgOK || pulseEvent.Time <= msgEvent.Time) && (!tickOK || pulseEvent.Time <= tickEvent.Time) {
				nextEvent = pulseEvent
				pulseEvent, pulseOK = pulseNext()
			} else if msgOK && (!tickOK || msgEvent.Time <= tickEvent.Time) {
				nextEvent = msgEvent
				msgEvent, msgOK = msgNext()
			} else {
				nextEvent = tickEvent
				tickEvent, tickOK = tickNext()
			}
			if !yield(nextEvent) {
				return
			}
		}
	}
}

// readPulseTimestamp reads the timestamp from vclock and calculates offset.
// Called for ALL pulse events (even during outages) to track PHC drift.
// Returns timestamp data that can optionally be delivered to controller.
func readPulseTimestamp(
	eventTime float64,
	data PulseEventData,
	vclock *clocksim.VirtualClock,
	testClock *clocksim.TestClock,
	tStart ptime.Time,
	lg *slog.Logger,
) (*pulseTimestamp, error) {
	if !vclock.TimestampAvailable() {
		return nil, fmt.Errorf("expected timestamp not available at PPS %v", data.PPS)
	}
	timestamp, trueTime, ok := testClock.ReadTimestampWithEra()
	if !ok {
		return nil, fmt.Errorf("failed to read timestamp at PPS %v", data.PPS)
	}
	tSys := time.Unix(0, 0).Add(time.Duration(eventTime * 1e9))
	tReadPHC := testClock.Now()
	tTrue := tStart.Add(time.Duration(trueTime * 1e9))
	taiOffset := timestamp.T.Sub(tTrue)
	lg.Debug("pulse timestamp read",
		"second", int(data.PPS),
		"edgeIdx", data.EdgeIdx,
		"timestamp", timestamp.T,
		"era", timestamp.Era,
		"taiOffset", taiOffset)
	return &pulseTimestamp{
		timestamp: timestamp,
		trueTime:  trueTime,
		tRead: ptime.Sample{
			Clock: tReadPHC,
			Sys:   tSys,
		},
		offset: taiOffset,
	}, nil
}

// deliverPulseToController delivers a pulse edge to the controller.
// Only called for pulses outside outage periods.
func deliverPulseToController(
	pts *pulseTimestamp,
	data PulseEventData,
	ctrl *phcsync.Controller,
	lg *slog.Logger,
) {
	edge := phcsync.PulseEdge{
		Timestamp: pts.timestamp,
		TRead:     pts.tRead,
	}
	lg.Debug("delivering pulse to controller",
		"second", int(data.PPS),
		"edgeIdx", data.EdgeIdx,
		"taiOffset", pts.offset)
	ctrl.PulseEdge(edge)
}

// handleMessageEvent processes a GPS message event
func handleMessageEvent(
	eventTime float64,
	data MessageEventData,
	timeMsgBuf *timemsg.Buffer,
	ctrl *phcsync.Controller,
	tStart ptime.Time,
	lg *slog.Logger,
) {
	tMsg := tStart.Add(time.Duration(data.PPS * 1e9))
	timeMsg := &gpsprot.TimeMsg{
		TAITime:     tMsg,
		GNSS:        gpsprot.GPS,
		Ref:         gpsprot.PostPulse,
		Tag:         ubx.Tag,
		NativeMsgID: "NAV-PVT",
	}
	msgTRead := time.Unix(0, 0).Add(time.Duration(eventTime * 1e9))
	timeMsgBuf.Time(timeMsg, msgTRead)
	lg.Debug("delivering time message",
		"second", int(data.PPS),
		"taiTime", tMsg,
		"msgTRead", msgTRead)
	ctrl.TimeMessage()
}

// handleTickEvent processes a tick event
func handleTickEvent(eventTime float64, ctrl *phcsync.Controller) {
	sys := time.Unix(0, 0).Add(time.Duration(eventTime * 1e9))
	ctrl.Tick(sys)
}
